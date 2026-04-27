package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// RemoveTemplateMemberResult captures one successful harness-template member remove.
type RemoveTemplateMemberResult struct {
	ManifestPath       string
	TemplatePath       string
	RemovedPaths       []string
	RemovedAgentsBlock bool
	ZeroMemberTemplate bool
	Manifest           ManifestFile
	TemplateManifest   TemplateManifest
}

// RemoveTemplateMember removes one orbit member from the current harness_template worktree.
func RemoveTemplateMember(ctx context.Context, repoRoot string, orbitID string) (RemoveTemplateMemberResult, error) {
	source, branchManifest, err := loadCurrentTemplateSource(ctx, repoRoot)
	if err != nil {
		return RemoveTemplateMemberResult{}, err
	}
	if err := ensureTemplateRemovePrecheckPathsClean(ctx, repoRoot, orbitID, source); err != nil {
		return RemoveTemplateMemberResult{}, err
	}

	ownership, err := AnalyzeTemplateMemberOwnership(source, orbitID)
	if err != nil {
		return RemoveTemplateMemberResult{}, err
	}

	nextMembers := make([]TemplateMember, 0, len(source.Manifest.Members))
	removed := false
	for _, member := range source.Manifest.Members {
		if member.OrbitID == orbitID {
			removed = true
			continue
		}
		nextMembers = append(nextMembers, member)
	}
	if !removed {
		return RemoveTemplateMemberResult{}, fmt.Errorf("template member %q not found", orbitID)
	}

	removedPaths := make([]string, 0, len(ownership.ExclusivePaths)+2)
	removedPathSet := make(map[string]struct{}, len(ownership.ExclusivePaths)+2)
	for _, path := range ownership.ExclusivePaths {
		removedPathSet[path] = struct{}{}
		removedPaths = append(removedPaths, path)
	}

	definitionPath, err := OrbitSpecRepoPath(orbitID)
	if err != nil {
		return RemoveTemplateMemberResult{}, err
	}
	removedPathSet[definitionPath] = struct{}{}
	removedPaths = append(removedPaths, definitionPath)

	snapshotPath, err := TemplateMemberSnapshotRepoPath(orbitID)
	if err != nil {
		return RemoveTemplateMemberResult{}, err
	}
	removedPathSet[snapshotPath] = struct{}{}
	removedPaths = append(removedPaths, snapshotPath)

	remainingFiles := make([]orbittemplate.CandidateFile, 0, len(source.Files))
	removedAgentsBlock := false
	for _, file := range source.Files {
		if _, ok := removedPathSet[file.Path]; ok {
			continue
		}
		if file.Path != rootAgentsPath {
			remainingFiles = append(remainingFiles, file)
			continue
		}

		updatedAgents, blockRemoved, err := orbittemplate.RemoveRuntimeAgentsBlockData(file.Content, orbitID)
		if err != nil {
			return RemoveTemplateMemberResult{}, fmt.Errorf("remove template AGENTS block: %w", err)
		}
		if !blockRemoved {
			remainingFiles = append(remainingFiles, file)
			continue
		}

		removedAgentsBlock = true
		if len(updatedAgents) == 0 {
			removedPathSet[rootAgentsPath] = struct{}{}
			removedPaths = append(removedPaths, rootAgentsPath)
			continue
		}

		remainingFiles = append(remainingFiles, orbittemplate.CandidateFile{
			Path:    file.Path,
			Content: updatedAgents,
			Mode:    file.Mode,
		})
	}

	variableBindings := templateVariableBindingsFromManifest(source.Manifest.Variables)
	nextVariables, err := buildTemplateSaveVariableSpecs(memberIDsFromTemplateMembers(nextMembers), remainingFiles, variableBindings)
	if err != nil {
		return RemoveTemplateMemberResult{}, fmt.Errorf("rebuild harness template variables: %w", err)
	}

	nextTemplateManifest := source.Manifest
	nextTemplateManifest.Members = nextMembers
	nextTemplateManifest.Variables = nextVariables
	nextTemplateManifest.Template.IncludesRootAgents = includesRootAgentsFile(remainingFiles)

	nextBranchManifest := branchManifest
	nextBranchManifest.Members = manifestMembersFromTemplateMembers(nextMembers)
	nextBranchManifest.IncludesRootAgents = nextTemplateManifest.Template.IncludesRootAgents

	touchedPaths := append([]string{ManifestRepoPath(), TemplateRepoPath()}, removedPaths...)
	if removedAgentsBlock {
		touchedPaths = append(touchedPaths, rootAgentsPath)
	}
	if err := ensureTemplateRemovePathsClean(ctx, repoRoot, orbitID, touchedPaths); err != nil {
		return RemoveTemplateMemberResult{}, err
	}

	if err := applyTemplateMemberRemovalFileChanges(repoRoot, source.Files, remainingFiles, removedPathSet); err != nil {
		return RemoveTemplateMemberResult{}, err
	}

	templatePath, err := WriteTemplateManifest(repoRoot, nextTemplateManifest)
	if err != nil {
		return RemoveTemplateMemberResult{}, fmt.Errorf("write harness template manifest: %w", err)
	}
	manifestPath, err := WriteManifestFile(repoRoot, nextBranchManifest)
	if err != nil {
		return RemoveTemplateMemberResult{}, fmt.Errorf("write harness branch manifest: %w", err)
	}

	sort.Strings(removedPaths)
	return RemoveTemplateMemberResult{
		ManifestPath:       manifestPath,
		TemplatePath:       templatePath,
		RemovedPaths:       removedPaths,
		RemovedAgentsBlock: removedAgentsBlock,
		ZeroMemberTemplate: len(nextMembers) == 0,
		Manifest:           nextBranchManifest,
		TemplateManifest:   nextTemplateManifest,
	}, nil
}

func loadCurrentTemplateSource(ctx context.Context, repoRoot string) (LocalTemplateInstallSource, ManifestFile, error) {
	branchManifest, err := LoadManifestFile(repoRoot)
	if err != nil {
		return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("load harness branch manifest: %w", err)
	}
	if branchManifest.Kind != ManifestKindHarnessTemplate {
		return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("harness root must contain a harness_template manifest at %s", repoRoot)
	}

	templateManifest, err := LoadTemplateManifest(repoRoot)
	if err != nil {
		return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("load harness template manifest: %w", err)
	}
	if err := validateTemplateInstallBranchManifest(branchManifest, templateManifest); err != nil {
		return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("load current harness template: %w", err)
	}

	trackedPaths, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("load tracked harness template files: %w", err)
	}
	sort.Strings(trackedPaths)

	memberSet := make(map[string]struct{}, len(templateManifest.Members))
	for _, member := range templateManifest.Members {
		memberSet[member.OrbitID] = struct{}{}
	}

	source := LocalTemplateInstallSource{
		Ref:             "WORKTREE",
		Manifest:        templateManifest,
		MemberSnapshots: map[string]TemplateMemberSnapshot{},
		DefinitionFiles: []orbittemplate.CandidateFile{},
		Files:           []orbittemplate.CandidateFile{},
	}
	seenDefinitions := make(map[string]struct{}, len(templateManifest.Members))
	seenSnapshots := make(map[string]struct{}, len(templateManifest.Members))

	for _, path := range trackedPaths {
		data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, path)
		if err != nil {
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("read harness template file %s: %w", path, err)
		}
		mode, err := gitpkg.TrackedFileModeWorktreeOrHEAD(ctx, repoRoot, path)
		if err != nil {
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("read harness template file mode %s: %w", path, err)
		}

		switch {
		case path == TemplateRepoPath():
			continue
		case path == ManifestRepoPath():
			continue
		case path == FrameworksRepoPath():
			file, err := ParseFrameworksFileData(data)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template frameworks file %s: %w", path, err)
			}
			source.Frameworks = file
			continue
		case path == ".harness/frameworks.yaml":
			file, err := ParseFrameworksFileData(data)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template frameworks file %s: %w", path, err)
			}
			source.Frameworks = file
			continue
		case path == AgentConfigRepoPath():
			file, err := ParseAgentConfigFileData(data)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template agent config file %s: %w", path, err)
			}
			source.AgentConfig = &file
			continue
		case strings.HasPrefix(path, AgentOverlaysDirRepoPath()+"/"):
			agentID, ok, err := parseAgentOverlayRepoPath(path)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template agent overlay path %s: %w", path, err)
			}
			if !ok {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains unsupported agent overlay path %s", path)
			}
			file, err := ParseAgentOverlayFileData(data)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template agent overlay %s: %w", path, err)
			}
			if source.AgentOverlays == nil {
				source.AgentOverlays = make(map[string]AgentOverlayFile)
			}
			source.AgentOverlays[agentID] = file
			continue
		case path == ".orbit/template.yaml":
			continue
		case path == ".orbit/source.yaml":
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains forbidden path %s", path)
		case path == ".orbit/config.yaml":
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains forbidden path %s", path)
		case strings.HasPrefix(path, ".harness/orbits/"):
			spec, err := orbitpkg.ParseHostedOrbitSpecData(data, filepath.Join(repoRoot, filepath.FromSlash(path)))
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template definition %s: %w", path, err)
			}
			if _, ok := memberSet[spec.ID]; !ok {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains untracked member definition %s", path)
			}
			seenDefinitions[spec.ID] = struct{}{}
			source.DefinitionFiles = append(source.DefinitionFiles, orbittemplate.CandidateFile{
				Path:    path,
				Content: data,
				Mode:    mode,
			})
			continue
		case strings.HasPrefix(path, TemplateMembersDirRepoPath()+"/"):
			snapshot, err := ParseTemplateMemberSnapshotData(data)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("parse harness template member snapshot %s: %w", path, err)
			}
			expectedPath, err := TemplateMemberSnapshotRepoPath(snapshot.OrbitID)
			if err != nil {
				return LocalTemplateInstallSource{}, ManifestFile{}, err
			}
			if expectedPath != path {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains untracked member snapshot %s", path)
			}
			if _, ok := memberSet[snapshot.OrbitID]; !ok {
				return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains untracked member snapshot %s", path)
			}
			seenSnapshots[snapshot.OrbitID] = struct{}{}
			source.MemberSnapshots[snapshot.OrbitID] = snapshot
			continue
		case strings.HasPrefix(path, ".harness/"):
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains forbidden path %s", path)
		case strings.HasPrefix(path, ".git/orbit/state/"):
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains forbidden path %s", path)
		case strings.HasPrefix(path, ".orbit/"):
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template contains forbidden path %s", path)
		}

		source.Files = append(source.Files, orbittemplate.CandidateFile{
			Path:    path,
			Content: data,
			Mode:    mode,
		})
	}

	for _, member := range templateManifest.Members {
		if _, ok := seenDefinitions[member.OrbitID]; !ok {
			return LocalTemplateInstallSource{}, ManifestFile{}, fmt.Errorf("current harness template is missing member definition for %q", member.OrbitID)
		}
	}

	return source, branchManifest, nil
}

func applyTemplateMemberRemovalFileChanges(
	repoRoot string,
	currentFiles []orbittemplate.CandidateFile,
	remainingFiles []orbittemplate.CandidateFile,
	removedPaths map[string]struct{},
) error {
	remainingByPath := make(map[string]orbittemplate.CandidateFile, len(remainingFiles))
	for _, file := range remainingFiles {
		remainingByPath[file.Path] = file
	}

	for _, file := range currentFiles {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		nextFile, stillPresent := remainingByPath[file.Path]
		if stillPresent {
			if candidateFilesEqual(file, nextFile) {
				continue
			}
			perm, err := gitpkg.FilePermForMode(nextFile.Mode)
			if err != nil {
				return fmt.Errorf("resolve file mode for %s: %w", nextFile.Path, err)
			}
			if err := contractutil.AtomicWriteFileMode(filename, nextFile.Content, perm); err != nil {
				return fmt.Errorf("write updated template file %s: %w", nextFile.Path, err)
			}
			continue
		}
		if _, ok := removedPaths[file.Path]; !ok {
			continue
		}
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove template file %s: %w", file.Path, err)
		}
	}

	for repoPath := range removedPaths {
		if repoPath == rootAgentsPath {
			continue
		}
		if _, ok := remainingByPath[repoPath]; ok {
			continue
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove template file %s: %w", repoPath, err)
		}
	}

	return nil
}

func templateVariableBindingsFromManifest(variables map[string]TemplateVariableSpec) map[string]bindings.VariableBinding {
	bindingsByName := make(map[string]bindings.VariableBinding, len(variables))
	for name, spec := range variables {
		bindingsByName[name] = bindings.VariableBinding{
			Description: spec.Description,
		}
	}

	return bindingsByName
}

func ensureTemplateRemovePrecheckPathsClean(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	source LocalTemplateInstallSource,
) error {
	touchedPaths := []string{ManifestRepoPath(), TemplateRepoPath()}

	definitionPath, err := OrbitSpecRepoPath(orbitID)
	if err != nil {
		return err
	}
	touchedPaths = append(touchedPaths, definitionPath)

	snapshotPath, err := TemplateMemberSnapshotRepoPath(orbitID)
	if err != nil {
		return err
	}
	touchedPaths = append(touchedPaths, snapshotPath)

	if snapshot, ok := source.MemberSnapshots[orbitID]; ok {
		touchedPaths = append(touchedPaths, snapshot.Snapshot.ExportedPaths...)
	}
	for _, file := range source.Files {
		if file.Path == rootAgentsPath {
			touchedPaths = append(touchedPaths, rootAgentsPath)
			break
		}
	}

	return ensureTemplateRemovePathsClean(ctx, repoRoot, orbitID, touchedPaths)
}

func ensureTemplateRemovePathsClean(ctx context.Context, repoRoot string, orbitID string, touchedPaths []string) error {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load template remove worktree status: %w", err)
	}

	statusByPath := make(map[string]string, len(statusEntries))
	for _, entry := range statusEntries {
		statusByPath[entry.Path] = entry.Code
	}

	dirtyPaths := make([]string, 0)
	for _, path := range sortedUniqueStrings(touchedPaths) {
		code, ok := statusByPath[path]
		if !ok {
			continue
		}
		dirtyPaths = append(dirtyPaths, fmt.Sprintf("%s (%s)", path, code))
	}
	if len(dirtyPaths) == 0 {
		return nil
	}

	return fmt.Errorf(
		"cannot remove template member %q with uncommitted changes on touched paths: %s",
		orbitID,
		strings.Join(dirtyPaths, ", "),
	)
}
