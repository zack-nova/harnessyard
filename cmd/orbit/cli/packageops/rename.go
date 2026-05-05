package packageops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"gopkg.in/yaml.v3"
)

const manifestRelativePath = ".harness/manifest.yaml"

var runtimeGuidanceMarkerLinePattern = regexp.MustCompile(`^<!-- orbit:(begin|end) workflow="([^"]+)" -->$`)

// RenameHostedOrbitPackageResult summarizes a hosted orbit package rename.
type RenameHostedOrbitPackageResult struct {
	RepoRoot          string        `json:"repo_root"`
	OldPackage        string        `json:"old_package"`
	NewPackage        string        `json:"new_package"`
	OldDefinitionPath string        `json:"old_definition_path"`
	NewDefinitionPath string        `json:"new_definition_path"`
	ManifestPath      string        `json:"manifest_path,omitempty"`
	ManifestChanged   bool          `json:"manifest_changed"`
	RenamedPaths      []RenamedPath `json:"renamed_paths,omitempty"`
	UpdatedFiles      []string      `json:"updated_files,omitempty"`
}

// RenamedPath records one repo-relative package-owned path moved during rename.
type RenamedPath struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type manifestRenamePlan struct {
	path    string
	changed bool
	file    harness.ManifestFile
}

type fileMutation struct {
	Path     string
	Original []byte
	Next     []byte
}

// RenameHostedOrbitPackage renames the authored hosted OrbitSpec identity and
// the current branch manifest identity when it points at the same orbit package.
func RenameHostedOrbitPackage(ctx context.Context, repoRoot string, oldPackage string, newPackage string) (RenameHostedOrbitPackageResult, error) {
	oldPackage = strings.TrimSpace(oldPackage)
	newPackage = strings.TrimSpace(newPackage)
	if err := ids.ValidateOrbitID(oldPackage); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("validate old package: %w", err)
	}
	if err := ids.ValidateOrbitID(newPackage); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("validate new package: %w", err)
	}
	if oldPackage == newPackage {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("new package must differ from old package %q", oldPackage)
	}

	oldRelativePath, err := orbit.HostedDefinitionRelativePath(oldPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("build old definition path: %w", err)
	}
	newRelativePath, err := orbit.HostedDefinitionRelativePath(newPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("build new definition path: %w", err)
	}
	oldPath := filepath.Join(repoRoot, filepath.FromSlash(oldRelativePath))
	newPath := filepath.Join(repoRoot, filepath.FromSlash(newRelativePath))

	if _, err := os.Stat(oldPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q not found", oldPackage)
		}
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("stat old orbit package %q: %w", oldPackage, err)
	}
	if _, err := os.Stat(newPath); err == nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q already exists", newPackage)
	} else if !errors.Is(err, os.ErrNotExist) {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("stat new orbit package %q: %w", newPackage, err)
	}

	config, err := orbit.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("load hosted orbit config: %w", err)
	}
	if _, found := config.OrbitByID(oldPackage); !found {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q not found", oldPackage)
	}
	if _, found := config.OrbitByID(newPackage); found {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("orbit package %q already exists", newPackage)
	}

	manifestPlan, err := planManifestRename(repoRoot, oldPackage, newPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, err
	}

	spec, err := orbit.LoadHostedOrbitSpec(ctx, repoRoot, oldPackage)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	renamedSpec, pathRenames, err := renamedHostedSpec(spec, oldPackage, newPackage, newRelativePath)
	if err != nil {
		return RenameHostedOrbitPackageResult{}, err
	}
	appliedPathRenames, err := applyPathRenames(repoRoot, pathRenames)
	if err != nil {
		if len(appliedPathRenames) > 0 {
			if rollbackErr := rollbackPathRenames(repoRoot, appliedPathRenames); rollbackErr != nil {
				return RenameHostedOrbitPackageResult{}, fmt.Errorf("rename package paths: %w; rollback renamed paths: %w", err, rollbackErr)
			}
		}
		return RenameHostedOrbitPackageResult{}, err
	}
	fileMutations, err := planRenameFileMutations(ctx, repoRoot, renamedSpec, appliedPathRenames, oldPackage, newPackage)
	if err != nil {
		if rollbackErr := rollbackPathRenames(repoRoot, appliedPathRenames); rollbackErr != nil {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("plan renamed package file updates: %w; rollback renamed paths: %w", err, rollbackErr)
		}
		return RenameHostedOrbitPackageResult{}, err
	}
	appliedFileMutations, err := applyFileMutations(repoRoot, fileMutations)
	if err != nil {
		if rollbackErr := rollbackFileMutations(repoRoot, appliedFileMutations); rollbackErr != nil {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("update renamed package files: %w; rollback updated files: %w", err, rollbackErr)
		}
		if rollbackErr := rollbackPathRenames(repoRoot, appliedPathRenames); rollbackErr != nil {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("update renamed package files: %w; rollback renamed paths: %w", err, rollbackErr)
		}
		return RenameHostedOrbitPackageResult{}, err
	}

	if _, err := orbit.WriteHostedOrbitSpec(repoRoot, renamedSpec); err != nil {
		if rollbackErr := rollbackFileMutations(repoRoot, appliedFileMutations); rollbackErr != nil {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed orbit package: %w; rollback updated files: %w", err, rollbackErr)
		}
		if rollbackErr := rollbackPathRenames(repoRoot, appliedPathRenames); rollbackErr != nil {
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed orbit package: %w; rollback renamed paths: %w", err, rollbackErr)
		}
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed orbit package: %w", err)
	}
	if manifestPlan.changed {
		if _, err := harness.WriteManifestFile(repoRoot, manifestPlan.file); err != nil {
			if cleanupErr := os.Remove(newPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w; rollback new orbit package %q: %w", err, newPackage, cleanupErr)
			}
			if rollbackErr := rollbackFileMutations(repoRoot, appliedFileMutations); rollbackErr != nil {
				return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w; rollback updated files: %w", err, rollbackErr)
			}
			if rollbackErr := rollbackPathRenames(repoRoot, appliedPathRenames); rollbackErr != nil {
				return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w; rollback renamed paths: %w", err, rollbackErr)
			}
			return RenameHostedOrbitPackageResult{}, fmt.Errorf("write renamed manifest package: %w", err)
		}
	}
	if err := os.Remove(oldPath); err != nil {
		return RenameHostedOrbitPackageResult{}, fmt.Errorf("remove old orbit package %q: %w", oldPackage, err)
	}

	return RenameHostedOrbitPackageResult{
		RepoRoot:          repoRoot,
		OldPackage:        oldPackage,
		NewPackage:        newPackage,
		OldDefinitionPath: oldRelativePath,
		NewDefinitionPath: newRelativePath,
		ManifestPath:      manifestPlan.path,
		ManifestChanged:   manifestPlan.changed,
		RenamedPaths:      appliedPathRenames,
		UpdatedFiles:      mutationPaths(appliedFileMutations),
	}, nil
}

func renamedHostedSpec(spec orbit.OrbitSpec, oldPackage string, newPackage string, newRelativePath string) (orbit.OrbitSpec, []RenamedPath, error) {
	version := ""
	if spec.Package != nil {
		version = spec.Package.Version
	}
	identity := ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: newPackage, Version: version}
	if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, "package"); err != nil {
		return orbit.OrbitSpec{}, nil, fmt.Errorf("validate renamed orbit package: %w", err)
	}

	var pathRenames []RenamedPath
	spec.Package = &identity
	spec.ID = newPackage
	spec.SourcePath = ""
	spec.Include = renamedPathPatterns(spec.Include, oldPackage, newPackage, &pathRenames)
	spec.Exclude = renamedPathPatterns(spec.Exclude, oldPackage, newPackage, &pathRenames)
	if spec.Meta != nil {
		spec.Meta.File = newRelativePath
	}
	for index := range spec.Members {
		spec.Members[index] = renamedOrbitMemberIdentity(spec.Members[index], oldPackage, newPackage)
		spec.Members[index].Paths = renamedOrbitMemberPaths(spec.Members[index].Paths, oldPackage, newPackage, &pathRenames)
	}
	for index := range spec.Content {
		spec.Content[index] = renamedOrbitMemberIdentity(spec.Content[index], oldPackage, newPackage)
		spec.Content[index].Paths = renamedOrbitMemberPaths(spec.Content[index].Paths, oldPackage, newPackage, &pathRenames)
	}
	if spec.Capabilities != nil {
		if spec.Capabilities.Commands != nil {
			spec.Capabilities.Commands.Paths = renamedOrbitMemberPaths(spec.Capabilities.Commands.Paths, oldPackage, newPackage, &pathRenames)
		}
		if spec.Capabilities.Skills != nil && spec.Capabilities.Skills.Local != nil {
			spec.Capabilities.Skills.Local.Paths = renamedOrbitMemberPaths(spec.Capabilities.Skills.Local.Paths, oldPackage, newPackage, &pathRenames)
		}
	}
	if spec.AgentAddons != nil && spec.AgentAddons.Hooks != nil {
		for index := range spec.AgentAddons.Hooks.Entries {
			spec.AgentAddons.Hooks.Entries[index].ID = renamedIdentifier(spec.AgentAddons.Hooks.Entries[index].ID, oldPackage, newPackage)
			nextPath, renames := renamedPathPattern(spec.AgentAddons.Hooks.Entries[index].Handler.Path, oldPackage, newPackage)
			spec.AgentAddons.Hooks.Entries[index].Handler.Path = nextPath
			pathRenames = append(pathRenames, renames...)
		}
	}

	mergedPathRenames, err := mergeRenamedPaths(pathRenames)
	if err != nil {
		return orbit.OrbitSpec{}, nil, err
	}

	return spec, mergedPathRenames, nil
}

func planManifestRename(repoRoot string, oldPackage string, newPackage string) (manifestRenamePlan, error) {
	file, err := harness.LoadManifestFile(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return manifestRenamePlan{}, nil
		}
		return manifestRenamePlan{}, fmt.Errorf("load %s: %w", manifestRelativePath, err)
	}

	plan := manifestRenamePlan{path: manifestRelativePath, file: file}
	switch file.Kind {
	case harness.ManifestKindSource:
		current := manifestOrbitName(file.Source.Package, file.Source.OrbitID)
		if current != oldPackage {
			return manifestRenamePlan{}, fmt.Errorf("source package %q must match old package %q", current, oldPackage)
		}
		plan.file.Source.Package = renamedOrbitIdentity(file.Source.Package, newPackage)
		plan.file.Source.OrbitID = newPackage
		plan.changed = true
	case harness.ManifestKindOrbitTemplate:
		current := manifestOrbitName(file.Template.Package, file.Template.OrbitID)
		if current != oldPackage {
			return manifestRenamePlan{}, fmt.Errorf("template package %q must match old package %q", current, oldPackage)
		}
		plan.file.Template.Package = renamedOrbitIdentity(file.Template.Package, newPackage)
		plan.file.Template.OrbitID = newPackage
		plan.changed = true
	case harness.ManifestKindRuntime, harness.ManifestKindHarnessTemplate:
		oldMemberFound := false
		for _, member := range file.Members {
			current := manifestOrbitName(member.Package, member.OrbitID)
			if current == newPackage {
				return manifestRenamePlan{}, fmt.Errorf("%s member package %q already exists", manifestKindLabel(file.Kind), newPackage)
			}
			if current == oldPackage {
				oldMemberFound = true
			}
		}
		if oldMemberFound {
			return manifestRenamePlan{}, fmt.Errorf("renaming runtime or harness-template members is not supported yet; remove or reinstall package %q instead", oldPackage)
		}
	}

	return plan, nil
}

func manifestKindLabel(kind string) string {
	return strings.ReplaceAll(kind, "_", "-")
}

func manifestOrbitName(identity ids.PackageIdentity, fallback string) string {
	if strings.TrimSpace(identity.Name) != "" {
		return identity.Name
	}
	return strings.TrimSpace(fallback)
}

func renamedOrbitIdentity(identity ids.PackageIdentity, newPackage string) ids.PackageIdentity {
	identity.Type = ids.PackageTypeOrbit
	identity.Name = newPackage
	return identity
}

func renamedOrbitMemberIdentity(member orbit.OrbitMember, oldPackage string, newPackage string) orbit.OrbitMember {
	member.Key = renamedIdentifier(member.Key, oldPackage, newPackage)
	member.Name = renamedIdentifier(member.Name, oldPackage, newPackage)
	return member
}

func renamedIdentifier(value string, oldPackage string, newPackage string) string {
	switch {
	case value == oldPackage:
		return newPackage
	case strings.HasPrefix(value, oldPackage+"-"):
		return newPackage + strings.TrimPrefix(value, oldPackage)
	case strings.HasPrefix(value, oldPackage+"_"):
		return newPackage + strings.TrimPrefix(value, oldPackage)
	default:
		return value
	}
}

func renamedOrbitMemberPaths(paths orbit.OrbitMemberPaths, oldPackage string, newPackage string, pathRenames *[]RenamedPath) orbit.OrbitMemberPaths {
	paths.Include = renamedPathPatterns(paths.Include, oldPackage, newPackage, pathRenames)
	paths.Exclude = renamedPathPatterns(paths.Exclude, oldPackage, newPackage, pathRenames)
	return paths
}

func renamedPathPatterns(patterns []string, oldPackage string, newPackage string, pathRenames *[]RenamedPath) []string {
	if len(patterns) == 0 {
		return patterns
	}
	next := make([]string, len(patterns))
	for index, pattern := range patterns {
		nextPattern, renames := renamedPathPattern(pattern, oldPackage, newPackage)
		next[index] = nextPattern
		*pathRenames = append(*pathRenames, renames...)
	}
	return next
}

func renamedPathPattern(pattern string, oldPackage string, newPackage string) (string, []RenamedPath) {
	normalized := strings.ReplaceAll(pattern, `\`, `/`)
	segments := strings.Split(normalized, "/")
	nextSegments := make([]string, len(segments))
	pathRenames := []RenamedPath{}
	seenGlob := false
	for index, segment := range segments {
		nextSegment := renamedPathSegment(segment, oldPackage, newPackage)
		nextSegments[index] = nextSegment
		if !seenGlob && nextSegment != segment && !containsGlobMeta(segment) {
			oldPathSegments := append([]string(nil), nextSegments[:index+1]...)
			oldPathSegments[index] = segment
			oldPath := strings.Join(oldPathSegments, "/")
			newPath := strings.Join(nextSegments[:index+1], "/")
			normalizedOldPath, oldErr := ids.NormalizeRepoRelativePath(oldPath)
			normalizedNewPath, newErr := ids.NormalizeRepoRelativePath(newPath)
			if oldErr == nil && newErr == nil {
				pathRenames = append(pathRenames, RenamedPath{OldPath: normalizedOldPath, NewPath: normalizedNewPath})
			}
		}
		if containsGlobMeta(segment) {
			seenGlob = true
		}
	}

	return strings.Join(nextSegments, "/"), pathRenames
}

func renamedPathSegment(segment string, oldPackage string, newPackage string) string {
	if segment == oldPackage {
		return newPackage
	}
	extension := path.Ext(segment)
	if extension != "" && strings.TrimSuffix(segment, extension) == oldPackage {
		return newPackage + extension
	}
	return segment
}

func containsGlobMeta(segment string) bool {
	return strings.ContainsAny(segment, "*?[")
}

func mergeRenamedPaths(pathRenames []RenamedPath) ([]RenamedPath, error) {
	renamedByOldPath := make(map[string]string, len(pathRenames))
	for _, pathRename := range pathRenames {
		oldPath, err := ids.NormalizeRepoRelativePath(pathRename.OldPath)
		if err != nil {
			return nil, fmt.Errorf("normalize old renamed path %q: %w", pathRename.OldPath, err)
		}
		newPath, err := ids.NormalizeRepoRelativePath(pathRename.NewPath)
		if err != nil {
			return nil, fmt.Errorf("normalize new renamed path %q: %w", pathRename.NewPath, err)
		}
		if oldPath == newPath {
			continue
		}
		if existingNewPath, ok := renamedByOldPath[oldPath]; ok && existingNewPath != newPath {
			return nil, fmt.Errorf("path %q would be renamed to both %q and %q", oldPath, existingNewPath, newPath)
		}
		renamedByOldPath[oldPath] = newPath
	}

	merged := make([]RenamedPath, 0, len(renamedByOldPath))
	for oldPath, newPath := range renamedByOldPath {
		merged = append(merged, RenamedPath{OldPath: oldPath, NewPath: newPath})
	}
	sortRenamedPathsForApply(merged)

	withoutChildren := make([]RenamedPath, 0, len(merged))
	for _, pathRename := range merged {
		if renamedPathCoveredByParent(pathRename, withoutChildren) {
			continue
		}
		withoutChildren = append(withoutChildren, pathRename)
	}
	sortRenamedPathsForApply(withoutChildren)

	return withoutChildren, nil
}

func sortRenamedPathsForApply(pathRenames []RenamedPath) {
	sort.Slice(pathRenames, func(left, right int) bool {
		leftDepth := strings.Count(pathRenames[left].OldPath, "/")
		rightDepth := strings.Count(pathRenames[right].OldPath, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		if pathRenames[left].OldPath != pathRenames[right].OldPath {
			return pathRenames[left].OldPath < pathRenames[right].OldPath
		}
		return pathRenames[left].NewPath < pathRenames[right].NewPath
	})
}

func renamedPathCoveredByParent(pathRename RenamedPath, parents []RenamedPath) bool {
	for _, parent := range parents {
		if pathRename.OldPath == parent.OldPath {
			return true
		}
		if !strings.HasPrefix(pathRename.OldPath, parent.OldPath+"/") {
			continue
		}
		suffix := strings.TrimPrefix(pathRename.OldPath, parent.OldPath+"/")
		if path.Join(parent.NewPath, suffix) == pathRename.NewPath {
			return true
		}
	}
	return false
}

func applyPathRenames(repoRoot string, pathRenames []RenamedPath) ([]RenamedPath, error) {
	applied := make([]RenamedPath, 0, len(pathRenames))
	for _, pathRename := range pathRenames {
		oldPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.OldPath))
		newPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.NewPath))
		if _, err := os.Stat(oldPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("stat path %q before rename: %w", pathRename.OldPath, err)
		}
		if _, err := os.Stat(newPath); err == nil {
			return nil, fmt.Errorf("cannot rename path %q to %q: destination already exists", pathRename.OldPath, pathRename.NewPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat rename destination %q: %w", pathRename.NewPath, err)
		}
	}

	for _, pathRename := range pathRenames {
		oldPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.OldPath))
		newPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.NewPath))
		if _, err := os.Stat(oldPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return applied, fmt.Errorf("stat path %q before rename: %w", pathRename.OldPath, err)
		}
		if _, err := os.Stat(newPath); err == nil {
			return applied, fmt.Errorf("cannot rename path %q to %q: destination already exists", pathRename.OldPath, pathRename.NewPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return applied, fmt.Errorf("stat rename destination %q: %w", pathRename.NewPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
			return applied, fmt.Errorf("create parent directory for %q: %w", pathRename.NewPath, err)
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return applied, fmt.Errorf("rename path %q to %q: %w", pathRename.OldPath, pathRename.NewPath, err)
		}
		applied = append(applied, pathRename)
	}

	return applied, nil
}

func rollbackPathRenames(repoRoot string, pathRenames []RenamedPath) error {
	for index := len(pathRenames) - 1; index >= 0; index-- {
		pathRename := pathRenames[index]
		oldPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.OldPath))
		newPath := filepath.Join(repoRoot, filepath.FromSlash(pathRename.NewPath))
		if _, err := os.Stat(newPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat rollback path %q: %w", pathRename.NewPath, err)
		}
		if _, err := os.Stat(oldPath); err == nil {
			return fmt.Errorf("rollback path %q to %q: destination already exists", pathRename.NewPath, pathRename.OldPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat rollback destination %q: %w", pathRename.OldPath, err)
		}
		if err := os.MkdirAll(filepath.Dir(oldPath), 0o750); err != nil {
			return fmt.Errorf("create rollback parent directory for %q: %w", pathRename.OldPath, err)
		}
		if err := os.Rename(newPath, oldPath); err != nil {
			return fmt.Errorf("rollback path %q to %q: %w", pathRename.NewPath, pathRename.OldPath, err)
		}
	}

	return nil
}

func planRenameFileMutations(
	ctx context.Context,
	repoRoot string,
	spec orbit.OrbitSpec,
	pathRenames []RenamedPath,
	oldPackage string,
	newPackage string,
) ([]fileMutation, error) {
	candidatePaths := map[string]struct{}{
		"AGENTS.md":    {},
		"HUMANS.md":    {},
		"BOOTSTRAP.md": {},
	}

	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("list worktree files: %w", err)
	}
	for _, file := range worktreeFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(file)
		if err != nil {
			return nil, fmt.Errorf("normalize worktree file %q: %w", file, err)
		}
		if !isMemberHintFile(normalizedPath) {
			continue
		}
		owned, err := packageRenameShouldUpdateMemberHint(spec, pathRenames, normalizedPath)
		if err != nil {
			return nil, err
		}
		if owned {
			candidatePaths[normalizedPath] = struct{}{}
		}
	}

	paths := make([]string, 0, len(candidatePaths))
	for path := range candidatePaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	mutations := make([]fileMutation, 0, len(paths))
	for _, candidatePath := range paths {
		mutation, ok, err := planRenameFileMutation(repoRoot, candidatePath, oldPackage, newPackage)
		if err != nil {
			return nil, err
		}
		if ok {
			mutations = append(mutations, mutation)
		}
	}

	return mutations, nil
}

func packageRenameShouldUpdateMemberHint(spec orbit.OrbitSpec, pathRenames []RenamedPath, normalizedPath string) (bool, error) {
	if pathUnderAnyRenamedPath(normalizedPath, pathRenames) {
		return true, nil
	}
	if len(spec.Include) > 0 {
		matches, err := orbit.PathMatchesOrbit(orbit.GlobalConfig{}, spec.LegacyDefinition(), normalizedPath)
		if err != nil {
			return false, fmt.Errorf("match member hint file %q against legacy include paths: %w", normalizedPath, err)
		}
		if matches {
			return true, nil
		}
	}
	if spec.HasMemberSchema() {
		for _, member := range spec.Members {
			matches, err := orbit.MemberMatchesPath(member, normalizedPath)
			if err != nil {
				return false, fmt.Errorf("match member hint file %q against member %q: %w", normalizedPath, member.Name, err)
			}
			if matches {
				return true, nil
			}
		}
		return false, nil
	}

	matches, err := orbit.PathMatchesOrbit(orbit.GlobalConfig{}, spec.LegacyDefinition(), normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match member hint file %q against legacy orbit paths: %w", normalizedPath, err)
	}
	return matches, nil
}

func pathUnderAnyRenamedPath(normalizedPath string, pathRenames []RenamedPath) bool {
	for _, pathRename := range pathRenames {
		if normalizedPath == pathRename.NewPath || strings.HasPrefix(normalizedPath, pathRename.NewPath+"/") {
			return true
		}
	}
	return false
}

func isMemberHintFile(normalizedPath string) bool {
	if path.Base(normalizedPath) == "SKILL.md" ||
		strings.HasPrefix(normalizedPath, "commands/") ||
		strings.HasPrefix(normalizedPath, "skills/") {
		return false
	}
	return path.Base(normalizedPath) == ".orbit-member.yaml" || path.Ext(normalizedPath) == ".md"
}

func planRenameFileMutation(repoRoot string, repoPath string, oldPackage string, newPackage string) (fileMutation, bool, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	//nolint:gosec // repoPath is normalized repo-relative input selected from Git-visible files or fixed root guidance names.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileMutation{}, false, nil
		}
		return fileMutation{}, false, fmt.Errorf("read rename candidate %q: %w", repoPath, err)
	}

	var next []byte
	var changed bool
	switch {
	case repoPath == "AGENTS.md" || repoPath == "HUMANS.md" || repoPath == "BOOTSTRAP.md":
		next, changed, err = renamedRuntimeGuidanceMarkerData(data, repoPath, oldPackage, newPackage)
	case path.Base(repoPath) == ".orbit-member.yaml":
		next, changed, err = renamedMemberHintYAMLData(data, repoPath, oldPackage, newPackage)
	case path.Ext(repoPath) == ".md":
		next, changed, err = renamedMarkdownMemberHintData(data, repoPath, oldPackage, newPackage)
	default:
		return fileMutation{}, false, nil
	}
	if err != nil {
		return fileMutation{}, false, err
	}
	if !changed || bytes.Equal(data, next) {
		return fileMutation{}, false, nil
	}

	return fileMutation{Path: repoPath, Original: data, Next: next}, true, nil
}

func renamedRuntimeGuidanceMarkerData(data []byte, label string, oldPackage string, newPackage string) ([]byte, bool, error) {
	if !bytes.Contains(data, []byte("<!-- orbit:")) {
		return data, false, nil
	}
	document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
	if err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", label, err)
	}
	oldFound := false
	newFound := false
	for _, segment := range document.Segments {
		if segment.Kind != orbittemplate.AgentsRuntimeSegmentBlock {
			continue
		}
		if segment.OwnerKind != orbittemplate.OwnerKindOrbit {
			continue
		}
		switch segment.WorkflowID {
		case oldPackage:
			oldFound = true
		case newPackage:
			newFound = true
		}
	}
	if !oldFound {
		return data, false, nil
	}
	if newFound {
		return nil, false, fmt.Errorf("%s already contains orbit block %q while renaming %q", label, newPackage, oldPackage)
	}

	var output bytes.Buffer
	for _, line := range splitLinesPreserveNewline(data) {
		body, ending := splitLineEnding(line)
		matches := runtimeGuidanceMarkerLinePattern.FindStringSubmatch(strings.TrimSpace(string(body)))
		if matches != nil && matches[2] == oldPackage {
			if _, err := fmt.Fprintf(&output, "<!-- orbit:%s workflow=%q -->", matches[1], newPackage); err != nil {
				return nil, false, fmt.Errorf("write renamed %s marker: %w", label, err)
			}
			output.Write(ending)
			continue
		}
		output.Write(line)
	}

	next := output.Bytes()
	if _, err := orbittemplate.ParseRuntimeAgentsDocument(next); err != nil {
		return nil, false, fmt.Errorf("parse renamed %s: %w", label, err)
	}

	return next, true, nil
}

func splitLinesPreserveNewline(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	lines := make([][]byte, 0, bytes.Count(data, []byte{'\n'})+1)
	start := 0
	for index, value := range data {
		if value != '\n' {
			continue
		}
		lines = append(lines, append([]byte(nil), data[start:index+1]...))
		start = index + 1
	}
	if start < len(data) {
		lines = append(lines, append([]byte(nil), data[start:]...))
	}
	return lines
}

func splitLineEnding(line []byte) ([]byte, []byte) {
	if !bytes.HasSuffix(line, []byte{'\n'}) {
		return line, nil
	}
	body := line[:len(line)-1]
	if bytes.HasSuffix(body, []byte{'\r'}) {
		return body[:len(body)-1], []byte("\r\n")
	}
	return body, []byte{'\n'}
}

func renamedMarkdownMemberHintData(data []byte, hintPath string, oldPackage string, newPackage string) ([]byte, bool, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return data, false, nil
	}

	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, false, fmt.Errorf("%s frontmatter must terminate with ---", hintPath)
	}

	frontmatterContent := []byte(rest[:end])
	body := rest[end+len("\n---\n"):]
	nextFrontmatter, changed, err := renamedMemberHintYAMLData(frontmatterContent, hintPath, oldPackage, newPackage)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return data, false, nil
	}

	var output bytes.Buffer
	output.WriteString("---\n")
	output.Write(nextFrontmatter)
	output.WriteString("---\n")
	output.WriteString(body)

	return output.Bytes(), true, nil
}

func renamedMemberHintYAMLData(data []byte, hintPath string, oldPackage string, newPackage string) ([]byte, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, false, fmt.Errorf("%s member hint is invalid YAML: %w", hintPath, err)
	}
	if len(document.Content) == 0 {
		return data, false, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return data, false, nil
	}

	changed, found, err := renameMemberHintNameInRoot(root, oldPackage, newPackage)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", hintPath, err)
	}
	if !found || !changed {
		return data, false, nil
	}

	next, err := yaml.Marshal(&document)
	if err != nil {
		return nil, false, fmt.Errorf("marshal renamed member hint %q: %w", hintPath, err)
	}
	return next, true, nil
}

func renameMemberHintNameInRoot(root *yaml.Node, oldPackage string, newPackage string) (bool, bool, error) {
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		if keyNode.Value != "orbit_member" {
			continue
		}
		if valueNode.Kind != yaml.MappingNode {
			return false, true, fmt.Errorf("orbit_member must be a mapping")
		}
		changed, err := renameMemberHintNameInMapping(valueNode, oldPackage, newPackage)
		return changed, true, err
	}

	if !isFlatMemberHintRoot(root) {
		return false, false, nil
	}
	changed, err := renameMemberHintNameInMapping(root, oldPackage, newPackage)
	return changed, true, err
}

func renameMemberHintNameInMapping(mapping *yaml.Node, oldPackage string, newPackage string) (bool, error) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		keyNode := mapping.Content[index]
		valueNode := mapping.Content[index+1]
		if keyNode.Value != "name" {
			continue
		}
		if valueNode.Kind != yaml.ScalarNode {
			return false, fmt.Errorf("name must be a scalar")
		}
		next := renamedIdentifier(valueNode.Value, oldPackage, newPackage)
		if next == valueNode.Value {
			return false, nil
		}
		if err := ids.ValidateOrbitID(next); err != nil {
			return false, fmt.Errorf("renamed member hint name %q: %w", next, err)
		}
		valueNode.Value = next
		return true, nil
	}
	return false, nil
}

func isFlatMemberHintRoot(root *yaml.Node) bool {
	hasName := false
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		switch keyNode.Value {
		case "name":
			hasName = true
		case "description", "role", "lane", "scopes":
		default:
			return false
		}
	}
	return hasName
}

func applyFileMutations(repoRoot string, mutations []fileMutation) ([]fileMutation, error) {
	applied := make([]fileMutation, 0, len(mutations))
	for _, mutation := range mutations {
		filename := filepath.Join(repoRoot, filepath.FromSlash(mutation.Path))
		if err := contractutil.AtomicWriteFile(filename, mutation.Next); err != nil {
			return applied, fmt.Errorf("write renamed file %q: %w", mutation.Path, err)
		}
		applied = append(applied, mutation)
	}
	return applied, nil
}

func rollbackFileMutations(repoRoot string, mutations []fileMutation) error {
	for index := len(mutations) - 1; index >= 0; index-- {
		mutation := mutations[index]
		filename := filepath.Join(repoRoot, filepath.FromSlash(mutation.Path))
		if err := contractutil.AtomicWriteFile(filename, mutation.Original); err != nil {
			return fmt.Errorf("restore renamed file %q: %w", mutation.Path, err)
		}
	}
	return nil
}

func mutationPaths(mutations []fileMutation) []string {
	paths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		paths = append(paths, mutation.Path)
	}
	sort.Strings(paths)
	return paths
}
