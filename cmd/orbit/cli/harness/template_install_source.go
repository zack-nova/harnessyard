package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"gopkg.in/yaml.v3"
)

// LocalTemplateInstallSource is one validated harness template branch snapshot.
type LocalTemplateInstallSource struct {
	Ref             string
	Commit          string
	Manifest        TemplateManifest
	Frameworks      FrameworksFile
	AgentConfig     *AgentConfigFile
	AgentOverlays   map[string]AgentOverlayFile
	MemberSnapshots map[string]TemplateMemberSnapshot
	DefinitionFiles []orbittemplate.CandidateFile
	Files           []orbittemplate.CandidateFile
}

// FilePaths returns the stable file list that a harness template install preview can expose.
func (source LocalTemplateInstallSource) FilePaths() []string {
	paths := make([]string, 0, len(source.DefinitionFiles)+len(source.Files))
	for _, file := range source.DefinitionFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range source.Files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}

// MemberIDs returns the stable harness template member id list.
func (source LocalTemplateInstallSource) MemberIDs() []string {
	memberIDs := make([]string, 0, len(source.Manifest.Members))
	for _, member := range source.Manifest.Members {
		memberIDs = append(memberIDs, member.OrbitID)
	}
	sort.Strings(memberIDs)
	return memberIDs
}

// LocalTemplateInstallSourceNotFoundError reports that a revision is not a harness template branch.
type LocalTemplateInstallSourceNotFoundError struct {
	Ref string
}

func (err *LocalTemplateInstallSourceNotFoundError) Error() string {
	return fmt.Sprintf("template source %q is not a valid harness template branch", err.Ref)
}

// RemoteTemplateInstallCandidate is one valid remote harness template branch candidate.
type RemoteTemplateInstallCandidate struct {
	RepoURL  string
	Branch   string
	Ref      string
	Commit   string
	Manifest TemplateManifest
}

// RemoteTemplateInstallNotFoundError reports that no remote harness template source matched.
type RemoteTemplateInstallNotFoundError struct {
	RepoURL      string
	RequestedRef string
}

func (err *RemoteTemplateInstallNotFoundError) Error() string {
	if strings.TrimSpace(err.RequestedRef) != "" {
		return fmt.Sprintf("remote harness template ref %q from %q is not a valid harness template branch", err.RequestedRef, err.RepoURL)
	}
	return fmt.Sprintf("no valid remote harness template branches found in %q", err.RepoURL)
}

// RemoteTemplateInstallAmbiguityError reports multiple valid harness template sources with no unique winner.
type RemoteTemplateInstallAmbiguityError struct {
	RepoURL    string
	Candidates []RemoteTemplateInstallCandidate
}

func (err *RemoteTemplateInstallAmbiguityError) Error() string {
	names := make([]string, 0, len(err.Candidates))
	for _, candidate := range err.Candidates {
		names = append(names, candidate.Branch)
	}
	return fmt.Sprintf("remote harness template source %q is ambiguous; candidates: %s", err.RepoURL, strings.Join(names, ", "))
}

// ResolveLocalTemplateInstallSource loads and validates one local harness template branch.
func ResolveLocalTemplateInstallSource(ctx context.Context, repoRoot string, ref string) (LocalTemplateInstallSource, error) {
	trimmedRef := strings.TrimSpace(ref)
	if trimmedRef == "" {
		return LocalTemplateInstallSource{}, fmt.Errorf("template source ref must not be empty")
	}

	exists, err := gitpkg.RevisionExists(ctx, repoRoot, trimmedRef)
	if err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("check harness template source %q: %w", trimmedRef, err)
	}
	if !exists {
		return LocalTemplateInstallSource{}, fmt.Errorf("template source %q not found", trimmedRef)
	}

	return loadTemplateInstallSourceAtRevision(ctx, repoRoot, trimmedRef, trimmedRef, "")
}

// ResolveRemoteTemplateInstallSource resolves one remote harness template candidate and materializes its snapshot.
func ResolveRemoteTemplateInstallSource(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	requestedRef string,
) (RemoteTemplateInstallCandidate, LocalTemplateInstallSource, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	trimmedRef := strings.TrimSpace(requestedRef)

	if trimmedRef != "" {
		candidate, source, err := resolveRemoteTemplateInstallSourceSnapshot(ctx, repoRoot, trimmedURL, trimmedRef)
		if err != nil {
			return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, err
		}
		return candidate, source, nil
	}

	candidates, err := EnumerateRemoteTemplateInstallSources(ctx, repoRoot, trimmedURL)
	if err != nil {
		return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, err
	}
	candidate, err := selectRemoteTemplateInstallCandidate(trimmedURL, "", candidates)
	if err != nil {
		return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, err
	}

	source, err := ResolveRemoteTemplateInstallCandidateSnapshot(ctx, repoRoot, candidate)
	if err != nil {
		return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, err
	}

	return candidate, source, nil
}

// EnumerateRemoteTemplateInstallSources lists remote heads and keeps only valid harness template manifests.
func EnumerateRemoteTemplateInstallSources(ctx context.Context, repoRoot string, remoteURL string) ([]RemoteTemplateInstallCandidate, error) {
	heads, err := gitpkg.ListRemoteHeads(ctx, repoRoot, remoteURL)
	if err != nil {
		return nil, fmt.Errorf("enumerate remote heads: %w", err)
	}

	candidates := make([]RemoteTemplateInstallCandidate, 0, len(heads))
	for _, head := range heads {
		branchManifestData, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, remoteURL, head.Ref, ManifestRepoPath())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read remote harness template branch manifest from %q: %w", head.Ref, err)
		}
		branchManifest, err := parseTemplateInstallBranchManifestData(branchManifestData)
		if err != nil {
			continue
		}

		manifestData, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, remoteURL, head.Ref, TemplateRepoPath())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read remote harness template manifest from %q: %w", head.Ref, err)
		}

		manifest, err := ParseTemplateManifestData(manifestData)
		if err != nil {
			continue
		}
		if err := validateTemplateInstallBranchManifest(branchManifest, manifest); err != nil {
			continue
		}
		if err := validateTemplateInstallableManifest(manifest, head.Ref); err != nil {
			continue
		}
		manifest.Template.DefaultTemplate = branchManifest.Template.DefaultTemplate

		candidates = append(candidates, RemoteTemplateInstallCandidate{
			RepoURL:  strings.TrimSpace(remoteURL),
			Branch:   head.Name,
			Ref:      head.Ref,
			Commit:   head.Commit,
			Manifest: manifest,
		})
	}

	return candidates, nil
}

// ResolveRemoteTemplateInstallCandidateSnapshot fetches and validates one chosen remote harness template branch.
func ResolveRemoteTemplateInstallCandidateSnapshot(
	ctx context.Context,
	repoRoot string,
	candidate RemoteTemplateInstallCandidate,
) (LocalTemplateInstallSource, error) {
	var source LocalTemplateInstallSource
	if err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, candidate.RepoURL, candidate.Ref, func(tempRef string) error {
		resolved, err := loadTemplateInstallSourceAtRevision(ctx, repoRoot, tempRef, candidate.Branch, candidate.Commit)
		if err != nil {
			return err
		}
		source = resolved
		return nil
	}); err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("resolve remote harness template source %q from %q: %w", candidate.Branch, candidate.RepoURL, err)
	}

	return source, nil
}

func loadTemplateInstallSourceAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
	sourceCommit string,
) (LocalTemplateInstallSource, error) {
	trimmedRef := strings.TrimSpace(sourceRef)
	if trimmedRef == "" {
		return LocalTemplateInstallSource{}, fmt.Errorf("template source ref must not be empty")
	}

	manifestExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, TemplateRepoPath())
	if err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("check harness template manifest at %q: %w", trimmedRef, err)
	}
	if !manifestExists {
		return LocalTemplateInstallSource{}, &LocalTemplateInstallSourceNotFoundError{Ref: trimmedRef}
	}

	manifestData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, TemplateRepoPath())
	if err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("read harness template manifest from %q: %w", trimmedRef, err)
	}
	manifest, err := ParseTemplateManifestData(manifestData)
	if err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("parse harness template manifest from %q: %w", trimmedRef, err)
	}
	branchManifest, err := loadTemplateInstallBranchManifestAtRevision(ctx, repoRoot, revision, trimmedRef)
	if err != nil {
		return LocalTemplateInstallSource{}, err
	}
	if err := validateTemplateInstallBranchManifest(branchManifest, manifest); err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q: %w", trimmedRef, err)
	}
	if err := validateTemplateInstallableManifest(manifest, trimmedRef); err != nil {
		return LocalTemplateInstallSource{}, err
	}
	frameworksFile, err := loadOptionalTemplateInstallFrameworksAtRevision(ctx, repoRoot, revision, trimmedRef)
	if err != nil {
		return LocalTemplateInstallSource{}, err
	}

	definitionPaths := make(map[string]struct{}, len(manifest.Members))
	definitionFiles := make([]orbittemplate.CandidateFile, 0, len(manifest.Members))
	memberSnapshots := make(map[string]TemplateMemberSnapshot, len(manifest.Members))
	memberSnapshotPaths := make(map[string]struct{}, len(manifest.Members))
	for _, member := range manifest.Members {
		definitionFile, err := loadTemplateInstallDefinitionAtRevision(ctx, repoRoot, revision, trimmedRef, member.OrbitID)
		if err != nil {
			return LocalTemplateInstallSource{}, err
		}
		definitionPaths[definitionFile.Path] = struct{}{}
		definitionFiles = append(definitionFiles, definitionFile)

		snapshotPath, err := TemplateMemberSnapshotRepoPath(member.OrbitID)
		if err != nil {
			return LocalTemplateInstallSource{}, err
		}
		memberSnapshotPaths[snapshotPath] = struct{}{}

		snapshotExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, snapshotPath)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("check template member snapshot %s from %q: %w", snapshotPath, trimmedRef, err)
		}
		if !snapshotExists {
			continue
		}

		snapshotData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, snapshotPath)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("read template member snapshot %s from %q: %w", snapshotPath, trimmedRef, err)
		}
		snapshot, err := ParseTemplateMemberSnapshotData(snapshotData)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("parse template member snapshot %s from %q: %w", snapshotPath, trimmedRef, err)
		}
		if snapshot.OrbitID != member.OrbitID {
			return LocalTemplateInstallSource{}, fmt.Errorf("template member snapshot %s from %q must match orbit %q", snapshotPath, trimmedRef, member.OrbitID)
		}
		memberSnapshots[member.OrbitID] = snapshot
	}
	sort.Slice(definitionFiles, func(left, right int) bool {
		return definitionFiles[left].Path < definitionFiles[right].Path
	})

	allPaths, err := gitpkg.ListAllFilesAtRev(ctx, repoRoot, revision)
	if err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("list harness template source files from %q: %w", trimmedRef, err)
	}
	agentConfig, hasAgentConfig, err := loadOptionalTemplateInstallAgentConfigAtRevision(ctx, repoRoot, revision, trimmedRef)
	if err != nil {
		return LocalTemplateInstallSource{}, err
	}
	agentOverlays, err := loadTemplateInstallAgentOverlaysAtRevision(ctx, repoRoot, revision, trimmedRef, allPaths)
	if err != nil {
		return LocalTemplateInstallSource{}, err
	}
	allowedAgentOverlayPaths := make(map[string]struct{}, len(agentOverlays))
	for agentID := range agentOverlays {
		allowedAgentOverlayPaths[AgentOverlayRepoPath(agentID)] = struct{}{}
	}

	files := make([]orbittemplate.CandidateFile, 0, len(allPaths))
	for _, path := range allPaths {
		switch {
		case path == TemplateRepoPath():
			continue
		case path == ManifestRepoPath():
			continue
		case path == ".orbit/template.yaml":
			continue
		case path == FrameworksRepoPath():
			continue
		case path == ".harness/frameworks.yaml":
			continue
		case path == AgentConfigRepoPath():
			continue
		case strings.HasPrefix(path, AgentOverlaysDirRepoPath()+"/"):
			if _, ok := allowedAgentOverlayPaths[path]; ok {
				continue
			}
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains unsupported agent overlay path %s", trimmedRef, path)
		case path == ".orbit/source.yaml":
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains forbidden path %s", trimmedRef, path)
		case path == ".orbit/config.yaml":
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains forbidden path %s", trimmedRef, path)
		case strings.HasPrefix(path, ".harness/orbits/"):
			if _, ok := definitionPaths[path]; ok {
				continue
			}
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains untracked member definition %s", trimmedRef, path)
		case strings.HasPrefix(path, ".orbit/orbits/"):
			if _, ok := definitionPaths[path]; ok {
				continue
			}
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains untracked member definition %s", trimmedRef, path)
		case strings.HasPrefix(path, TemplateMembersDirRepoPath()+"/"):
			if _, ok := memberSnapshotPaths[path]; ok {
				continue
			}
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains untracked member snapshot %s", trimmedRef, path)
		case strings.HasPrefix(path, ".harness/"):
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains forbidden path %s", trimmedRef, path)
		case strings.HasPrefix(path, ".git/orbit/state/"):
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains forbidden path %s", trimmedRef, path)
		case strings.HasPrefix(path, ".orbit/"):
			return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q contains forbidden path %s", trimmedRef, path)
		}

		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, path)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("read harness template file %s from %q: %w", path, trimmedRef, err)
		}
		mode, err := gitpkg.FileModeAtRev(ctx, repoRoot, revision, path)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("read harness template file mode %s from %q: %w", path, trimmedRef, err)
		}
		files = append(files, orbittemplate.CandidateFile{
			Path:    path,
			Content: data,
			Mode:    mode,
		})
	}

	scanResult := orbittemplate.ScanVariables(files, templateManifestVariableSpecs(manifest.Variables))
	if len(scanResult.Undeclared) > 0 {
		return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q references undeclared variables: %s", trimmedRef, strings.Join(scanResult.Undeclared, ", "))
	}

	commit := strings.TrimSpace(sourceCommit)
	if commit == "" {
		commit, err = gitpkg.ResolveRevision(ctx, repoRoot, revision)
		if err != nil {
			return LocalTemplateInstallSource{}, fmt.Errorf("resolve harness template source commit for %q: %w", trimmedRef, err)
		}
	}

	source := LocalTemplateInstallSource{
		Ref:             trimmedRef,
		Commit:          commit,
		Manifest:        manifest,
		Frameworks:      frameworksFile,
		AgentOverlays:   agentOverlays,
		MemberSnapshots: memberSnapshots,
		DefinitionFiles: definitionFiles,
		Files:           files,
	}
	if hasAgentConfig {
		source.AgentConfig = &agentConfig
	}

	if err := validateTemplateInstallSourceCapabilities(source); err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q: %w", trimmedRef, err)
	}

	return validateLoadedTemplateMemberSnapshots(source, trimmedRef)
}

func loadOptionalTemplateInstallFrameworksAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
) (FrameworksFile, error) {
	exists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, FrameworksRepoPath())
	if err != nil {
		return FrameworksFile{}, fmt.Errorf("check harness template frameworks file from %q: %w", sourceRef, err)
	}
	if !exists {
		legacyExists, legacyErr := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, ".harness/frameworks.yaml")
		if legacyErr != nil {
			return FrameworksFile{}, fmt.Errorf("check legacy harness template frameworks file from %q: %w", sourceRef, legacyErr)
		}
		if !legacyExists {
			return FrameworksFile{SchemaVersion: frameworksSchemaVersion}, nil
		}
		data, readErr := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, ".harness/frameworks.yaml")
		if readErr != nil {
			return FrameworksFile{}, fmt.Errorf("read legacy harness template frameworks file from %q: %w", sourceRef, readErr)
		}
		file, parseErr := ParseFrameworksFileData(data)
		if parseErr != nil {
			return FrameworksFile{}, fmt.Errorf("parse legacy harness template frameworks file from %q: %w", sourceRef, parseErr)
		}
		return file, nil
	}

	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, FrameworksRepoPath())
	if err != nil {
		return FrameworksFile{}, fmt.Errorf("read harness template frameworks file from %q: %w", sourceRef, err)
	}
	file, err := ParseFrameworksFileData(data)
	if err != nil {
		return FrameworksFile{}, fmt.Errorf("parse harness template frameworks file from %q: %w", sourceRef, err)
	}

	return file, nil
}

func loadOptionalTemplateInstallAgentConfigAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
) (AgentConfigFile, bool, error) {
	exists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, AgentConfigRepoPath())
	if err != nil {
		return AgentConfigFile{}, false, fmt.Errorf("check harness template agent config from %q: %w", sourceRef, err)
	}
	if !exists {
		return AgentConfigFile{}, false, nil
	}

	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, AgentConfigRepoPath())
	if err != nil {
		return AgentConfigFile{}, false, fmt.Errorf("read harness template agent config from %q: %w", sourceRef, err)
	}
	file, err := ParseAgentConfigFileData(data)
	if err != nil {
		return AgentConfigFile{}, false, fmt.Errorf("parse harness template agent config from %q: %w", sourceRef, err)
	}

	return file, true, nil
}

func loadTemplateInstallAgentOverlaysAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
	allPaths []string,
) (map[string]AgentOverlayFile, error) {
	overlays := make(map[string]AgentOverlayFile)
	for _, path := range allPaths {
		if !strings.HasPrefix(path, AgentOverlaysDirRepoPath()+"/") {
			continue
		}
		agentID, ok, err := parseAgentOverlayRepoPath(path)
		if err != nil {
			return nil, fmt.Errorf("parse harness template agent overlay path %s from %q: %w", path, sourceRef, err)
		}
		if !ok {
			continue
		}
		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, path)
		if err != nil {
			return nil, fmt.Errorf("read harness template agent overlay %s from %q: %w", path, sourceRef, err)
		}
		file, err := ParseAgentOverlayFileData(data)
		if err != nil {
			return nil, fmt.Errorf("parse harness template agent overlay %s from %q: %w", path, sourceRef, err)
		}
		overlays[agentID] = file
	}

	return overlays, nil
}

func parseAgentOverlayRepoPath(path string) (string, bool, error) {
	if !strings.HasPrefix(path, AgentOverlaysDirRepoPath()+"/") {
		return "", false, nil
	}
	base := strings.TrimPrefix(path, AgentOverlaysDirRepoPath()+"/")
	if strings.Contains(base, "/") || filepath.Ext(base) != ".yaml" {
		return "", false, fmt.Errorf("overlay path must be %s/<agent>.yaml", AgentOverlaysDirRepoPath())
	}
	agentID := strings.TrimSuffix(base, filepath.Ext(base))
	if err := validateAgentOverlayID(agentID); err != nil {
		return "", false, err
	}

	return agentID, true, nil
}

func validateTemplateInstallSourceCapabilities(source LocalTemplateInstallSource) error {
	if len(source.DefinitionFiles) == 0 {
		return nil
	}

	trackedFiles := make([]string, 0, len(source.Files))
	fileContents := make(map[string][]byte, len(source.Files))
	for _, file := range source.Files {
		trackedFiles = append(trackedFiles, file.Path)
		fileContents[file.Path] = append([]byte(nil), file.Content...)
	}

	for _, definitionFile := range source.DefinitionFiles {
		spec, err := orbit.ParseHostedOrbitSpecData(definitionFile.Content, definitionFile.Path)
		if err != nil {
			definition, legacyErr := orbit.ParseDefinitionData(definitionFile.Content, definitionFile.Path)
			if legacyErr != nil {
				return fmt.Errorf("parse template member definition %s: %w", definitionFile.Path, err)
			}
			spec = orbit.OrbitSpecFromDefinition(definition)
		}
		if spec.Capabilities == nil && spec.AgentAddons == nil {
			continue
		}

		definition, err := orbit.CompatibilityDefinitionFromOrbitSpec(spec)
		if err != nil {
			return fmt.Errorf("build compatibility definition for orbit %q: %w", spec.ID, err)
		}
		plan, err := orbit.ResolveProjectionPlan(orbit.RepositoryConfig{
			Global: orbit.DefaultGlobalConfig(),
			Orbits: []orbit.Definition{definition},
		}, spec, trackedFiles)
		if err != nil {
			return fmt.Errorf("resolve capability projection plan for orbit %q: %w", spec.ID, err)
		}
		if err := orbit.PreflightResolvedCapabilitiesFromFiles(spec, trackedFiles, plan.ExportPaths, fileContents); err != nil {
			return fmt.Errorf("preflight capabilities for orbit %q: %w", spec.ID, err)
		}
	}

	return nil
}

func validateLoadedTemplateMemberSnapshots(
	source LocalTemplateInstallSource,
	sourceRef string,
) (LocalTemplateInstallSource, error) {
	if _, err := validateTemplateMemberSnapshots(source); err != nil {
		return LocalTemplateInstallSource{}, fmt.Errorf("harness template source %q: %w", sourceRef, err)
	}

	return source, nil
}

func loadTemplateInstallDefinitionAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
	orbitID string,
) (orbittemplate.CandidateFile, error) {
	hostedPath, err := orbit.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("build hosted harness template definition path for %q: %w", orbitID, err)
	}
	legacyPath, err := orbit.DefinitionRelativePath(orbitID)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("build legacy harness template definition path for %q: %w", orbitID, err)
	}

	hostedExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, hostedPath)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("check harness template definition %s from %q: %w", hostedPath, sourceRef, err)
	}
	if hostedExists {
		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, hostedPath)
		if err != nil {
			return orbittemplate.CandidateFile{}, fmt.Errorf("read harness template definition %s from %q: %w", hostedPath, sourceRef, err)
		}
		spec, err := orbit.ParseHostedOrbitSpecData(data, hostedPath)
		if err != nil {
			return orbittemplate.CandidateFile{}, fmt.Errorf("parse harness template definition %s from %q: %w", hostedPath, sourceRef, err)
		}
		data, err = stableInstallOrbitSpecData(spec)
		if err != nil {
			return orbittemplate.CandidateFile{}, fmt.Errorf("normalize harness template definition %s from %q: %w", hostedPath, sourceRef, err)
		}
		mode, err := gitpkg.FileModeAtRev(ctx, repoRoot, revision, hostedPath)
		if err != nil {
			return orbittemplate.CandidateFile{}, fmt.Errorf("read harness template definition mode %s from %q: %w", hostedPath, sourceRef, err)
		}

		return orbittemplate.CandidateFile{
			Path:    hostedPath,
			Content: data,
			Mode:    mode,
		}, nil
	}

	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, legacyPath)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("read harness template definition %s from %q: %w", legacyPath, sourceRef, err)
	}
	definition, err := orbit.ParseDefinitionData(data, legacyPath)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("parse harness template definition %s from %q: %w", legacyPath, sourceRef, err)
	}
	data, err = stableInstallDefinitionData(definition)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("normalize harness template definition %s from %q: %w", legacyPath, sourceRef, err)
	}
	mode, err := gitpkg.FileModeAtRev(ctx, repoRoot, revision, legacyPath)
	if err != nil {
		return orbittemplate.CandidateFile{}, fmt.Errorf("read harness template definition mode %s from %q: %w", legacyPath, sourceRef, err)
	}

	return orbittemplate.CandidateFile{
		Path:    legacyPath,
		Content: data,
		Mode:    mode,
	}, nil
}

func stableInstallDefinitionData(definition orbit.Definition) ([]byte, error) {
	stable := definition
	if stable.Exclude == nil {
		stable.Exclude = []string{}
	}

	data, err := yaml.Marshal(stable)
	if err != nil {
		return nil, fmt.Errorf("marshal orbit definition: %w", err)
	}

	return append(data, '\n'), nil
}

func stableInstallOrbitSpecData(spec orbit.OrbitSpec) ([]byte, error) {
	stable := spec
	stable.SourcePath = ""

	data, err := yaml.Marshal(stable)
	if err != nil {
		return nil, fmt.Errorf("marshal orbit spec: %w", err)
	}

	return append(data, '\n'), nil
}

func resolveRemoteTemplateInstallSourceSnapshot(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	requestedRef string,
) (RemoteTemplateInstallCandidate, LocalTemplateInstallSource, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	trimmedRef := strings.TrimSpace(requestedRef)
	branchName, fullRef := normalizeRemoteInstallRequestedRef(trimmedRef)

	var manifest TemplateManifest
	var source LocalTemplateInstallSource
	notTemplate := false

	if err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, trimmedURL, trimmedRef, func(tempRef string) error {
		manifestExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, tempRef, TemplateRepoPath())
		if err != nil {
			return fmt.Errorf("check remote harness template manifest at %q: %w", trimmedRef, err)
		}
		if !manifestExists {
			notTemplate = true
			return nil
		}

		manifestData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, tempRef, TemplateRepoPath())
		if err != nil {
			return fmt.Errorf("read remote harness template manifest at %q: %w", trimmedRef, err)
		}
		parsedManifest, err := ParseTemplateManifestData(manifestData)
		if err != nil {
			return fmt.Errorf("parse remote harness template manifest at %q: %w", trimmedRef, err)
		}
		manifest = parsedManifest

		resolved, err := loadTemplateInstallSourceAtRevision(ctx, repoRoot, tempRef, branchName, "")
		if err != nil {
			return err
		}
		source = resolved
		return nil
	}); err != nil {
		if isMissingRemoteInstallRefError(err) {
			return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, &RemoteTemplateInstallNotFoundError{
				RepoURL:      trimmedURL,
				RequestedRef: trimmedRef,
			}
		}
		return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, fmt.Errorf(
			"resolve remote harness template source %q from %q: %w",
			trimmedRef,
			trimmedURL,
			err,
		)
	}

	if notTemplate {
		return RemoteTemplateInstallCandidate{}, LocalTemplateInstallSource{}, &RemoteTemplateInstallNotFoundError{
			RepoURL:      trimmedURL,
			RequestedRef: trimmedRef,
		}
	}

	return RemoteTemplateInstallCandidate{
		RepoURL:  trimmedURL,
		Branch:   branchName,
		Ref:      fullRef,
		Commit:   source.Commit,
		Manifest: manifest,
	}, source, nil
}

func normalizeRemoteInstallRequestedRef(requestedRef string) (branch string, ref string) {
	trimmedRef := strings.TrimSpace(requestedRef)
	if strings.HasPrefix(trimmedRef, "refs/heads/") {
		return strings.TrimPrefix(trimmedRef, "refs/heads/"), trimmedRef
	}
	return trimmedRef, "refs/heads/" + trimmedRef
}

func isMissingRemoteInstallRefError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "couldn't find remote ref") ||
		strings.Contains(message, "invalid refspec")
}

func selectRemoteTemplateInstallCandidate(
	remoteURL string,
	requestedRef string,
	candidates []RemoteTemplateInstallCandidate,
) (RemoteTemplateInstallCandidate, error) {
	trimmedRef := strings.TrimSpace(requestedRef)
	if trimmedRef != "" {
		for _, candidate := range candidates {
			if candidate.Branch == trimmedRef || candidate.Ref == trimmedRef {
				return candidate, nil
			}
		}

		return RemoteTemplateInstallCandidate{}, &RemoteTemplateInstallNotFoundError{
			RepoURL:      remoteURL,
			RequestedRef: trimmedRef,
		}
	}

	switch len(candidates) {
	case 0:
		return RemoteTemplateInstallCandidate{}, &RemoteTemplateInstallNotFoundError{RepoURL: remoteURL}
	case 1:
		return candidates[0], nil
	}

	defaultCandidates := make([]RemoteTemplateInstallCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Manifest.Template.DefaultTemplate {
			defaultCandidates = append(defaultCandidates, candidate)
		}
	}
	if len(defaultCandidates) == 1 {
		return defaultCandidates[0], nil
	}

	return RemoteTemplateInstallCandidate{}, &RemoteTemplateInstallAmbiguityError{
		RepoURL:    remoteURL,
		Candidates: slices.Clone(candidates),
	}
}

func templateManifestVariableSpecs(values map[string]TemplateVariableSpec) map[string]orbittemplate.VariableSpec {
	specs := make(map[string]orbittemplate.VariableSpec, len(values))
	for name, value := range values {
		specs[name] = orbittemplate.VariableSpec{
			Description: value.Description,
			Required:    value.Required,
		}
	}
	return specs
}

func loadTemplateInstallBranchManifestAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	sourceRef string,
) (ManifestFile, error) {
	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, ManifestRepoPath())
	if err != nil {
		return ManifestFile{}, fmt.Errorf(
			"template source %q is not a valid harness template branch: read %s: %w",
			sourceRef,
			ManifestRepoPath(),
			err,
		)
	}

	manifest, err := parseTemplateInstallBranchManifestData(data)
	if err != nil {
		return ManifestFile{}, fmt.Errorf(
			"template source %q is not a valid harness template branch: parse %s: %w",
			sourceRef,
			ManifestRepoPath(),
			err,
		)
	}

	return manifest, nil
}

func parseTemplateInstallBranchManifestData(data []byte) (ManifestFile, error) {
	manifest, err := ParseManifestFileData(data)
	if err != nil {
		return ManifestFile{}, err
	}
	if manifest.Kind != ManifestKindHarnessTemplate {
		return ManifestFile{}, fmt.Errorf("kind must be %q", ManifestKindHarnessTemplate)
	}
	if manifest.Template == nil || strings.TrimSpace(manifest.Template.HarnessID) == "" {
		return ManifestFile{}, fmt.Errorf("template.harness_id must not be empty")
	}

	return manifest, nil
}

func validateTemplateInstallBranchManifest(branchManifest ManifestFile, manifest TemplateManifest) error {
	if branchManifest.Template == nil {
		return fmt.Errorf("%s template must be present", ManifestRepoPath())
	}
	if branchManifest.Template.HarnessID != manifest.Template.HarnessID {
		return fmt.Errorf(
			"%s template.harness_id %q must match %s template.harness_id %q",
			ManifestRepoPath(),
			branchManifest.Template.HarnessID,
			TemplateRepoPath(),
			manifest.Template.HarnessID,
		)
	}
	if branchManifest.Template.DefaultTemplate != manifest.Template.DefaultTemplate {
		return fmt.Errorf(
			"%s template.default_template %t must match %s template.default_template %t",
			ManifestRepoPath(),
			branchManifest.Template.DefaultTemplate,
			TemplateRepoPath(),
			manifest.Template.DefaultTemplate,
		)
	}
	if branchManifest.Template.CreatedFromBranch != manifest.Template.CreatedFromBranch {
		return fmt.Errorf(
			"%s template.created_from_branch %q must match %s template.created_from_branch %q",
			ManifestRepoPath(),
			branchManifest.Template.CreatedFromBranch,
			TemplateRepoPath(),
			manifest.Template.CreatedFromBranch,
		)
	}
	if branchManifest.Template.CreatedFromCommit != manifest.Template.CreatedFromCommit {
		return fmt.Errorf(
			"%s template.created_from_commit %q must match %s template.created_from_commit %q",
			ManifestRepoPath(),
			branchManifest.Template.CreatedFromCommit,
			TemplateRepoPath(),
			manifest.Template.CreatedFromCommit,
		)
	}
	if !branchManifest.Template.CreatedAt.Equal(manifest.Template.CreatedAt) {
		return fmt.Errorf(
			"%s template.created_at %s must match %s template.created_at %s",
			ManifestRepoPath(),
			branchManifest.Template.CreatedAt.Format(time.RFC3339),
			TemplateRepoPath(),
			manifest.Template.CreatedAt.Format(time.RFC3339),
		)
	}
	if branchManifest.IncludesRootAgents != manifest.Template.IncludesRootAgents {
		return fmt.Errorf(
			"%s includes_root_agents %t must match %s template.includes_root_agents %t",
			ManifestRepoPath(),
			branchManifest.IncludesRootAgents,
			TemplateRepoPath(),
			manifest.Template.IncludesRootAgents,
		)
	}
	if !equalTemplateInstallMemberIDs(branchManifest.Members, manifest.Members) {
		return fmt.Errorf("%s members must match %s members", ManifestRepoPath(), TemplateRepoPath())
	}

	return nil
}

func validateTemplateInstallableManifest(manifest TemplateManifest, sourceRef string) error {
	if len(manifest.Members) > 0 {
		return nil
	}

	trimmedRef := strings.TrimSpace(sourceRef)
	if trimmedRef == "" {
		return fmt.Errorf("zero-member harness template cannot be installed")
	}

	return fmt.Errorf("zero-member harness template %q cannot be installed", trimmedRef)
}

func equalTemplateInstallMemberIDs(branchMembers []ManifestMember, templateMembers []TemplateMember) bool {
	if len(branchMembers) != len(templateMembers) {
		return false
	}

	branchIDs := make([]string, 0, len(branchMembers))
	for _, member := range branchMembers {
		branchIDs = append(branchIDs, member.OrbitID)
	}
	sort.Strings(branchIDs)

	templateIDs := make([]string, 0, len(templateMembers))
	for _, member := range templateMembers {
		templateIDs = append(templateIDs, member.OrbitID)
	}
	sort.Strings(templateIDs)

	return slices.Equal(branchIDs, templateIDs)
}
