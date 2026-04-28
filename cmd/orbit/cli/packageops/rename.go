package packageops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

const manifestRelativePath = ".harness/manifest.yaml"

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

	if _, err := orbit.WriteHostedOrbitSpec(repoRoot, renamedSpec); err != nil {
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
		spec.Members[index].Paths = renamedOrbitMemberPaths(spec.Members[index].Paths, oldPackage, newPackage, &pathRenames)
	}
	for index := range spec.Content {
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
		for _, member := range file.Members {
			if manifestOrbitName(member.Package, member.OrbitID) == oldPackage {
				return manifestRenamePlan{}, fmt.Errorf("renaming runtime or harness-template members is not supported yet; remove or reinstall package %q instead", oldPackage)
			}
		}
	}

	return plan, nil
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
