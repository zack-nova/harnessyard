package orbit

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func definitionControlPath(definition Definition) (string, error) {
	if repoRelative, ok := sourcePathRepoRelativePath(definition.SourcePath); ok {
		return repoRelative, nil
	}

	return HostedDefinitionRelativePath(definition.ID)
}

func specControlPath(spec OrbitSpec) (string, error) {
	if spec.Meta != nil && strings.TrimSpace(spec.Meta.File) != "" {
		normalized, err := ids.NormalizeRepoRelativePath(spec.Meta.File)
		if err != nil {
			return "", fmt.Errorf("normalize meta.file: %w", err)
		}
		return normalized, nil
	}
	if repoRelative, ok := sourcePathRepoRelativePath(spec.SourcePath); ok {
		return repoRelative, nil
	}

	return HostedDefinitionRelativePath(spec.ID)
}

func sourcePathRepoRelativePath(sourcePath string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(sourcePath), `\`, `/`)
	for _, prefix := range []string{hostedOrbitsRelativeDir + "/", orbitsRelativeDir + "/"} {
		index := strings.Index(normalized, prefix)
		if index < 0 {
			continue
		}

		repoRelative, err := ids.NormalizeRepoRelativePath(normalized[index:])
		if err != nil {
			return "", false
		}
		return repoRelative, true
	}

	return "", false
}

// ScopeSet captures the internal scope breakdown for a single orbit.
type ScopeSet struct {
	ControlReadPaths     []string
	OwnedPaths           []string
	ProjectionOnlyPaths  []string
	UserDataPaths        []string
	CompanionPaths       []string
	ScopedOperationPaths []string
	ProjectionPaths      []string
}

// ResolveScopeSet computes the full internal scope breakdown for an orbit.
func ResolveScopeSet(config RepositoryConfig, definition Definition, trackedFiles []string) (ScopeSet, error) {
	scopeSet, err := ResolveScopeSetForSpec(config, OrbitSpecFromDefinition(definition), trackedFiles)
	if err != nil {
		return ScopeSet{}, err
	}

	return scopeSet, nil
}

func resolveUserDataPaths(config GlobalConfig, definition Definition, trackedFiles []string) ([]string, []string, error) {
	if err := ValidateGlobalConfig(config); err != nil {
		return nil, nil, fmt.Errorf("validate global config: %w", err)
	}
	if err := ValidateDefinition(definition); err != nil {
		return nil, nil, fmt.Errorf("validate orbit definition: %w", err)
	}

	includePatterns, err := normalizePatterns(definition.Include)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize include patterns: %w", err)
	}
	excludePatterns, err := normalizePatterns(definition.Exclude)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize exclude patterns: %w", err)
	}
	sharedScopePatterns, err := normalizePatterns(config.SharedScope)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize shared scope patterns: %w", err)
	}
	projectionVisiblePatterns, err := normalizePatterns(config.ProjectionVisible)
	if err != nil {
		return nil, nil, fmt.Errorf("normalize projection-visible patterns: %w", err)
	}

	ownedSet := make(map[string]struct{}, len(trackedFiles))
	projectionOnlySet := make(map[string]struct{}, len(trackedFiles))

	for _, trackedFile := range trackedFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(trackedFile)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize tracked file %q: %w", trackedFile, err)
		}
		if isControlPlanePath(normalizedPath) {
			continue
		}

		included, err := matchNormalizedAny(includePatterns, normalizedPath)
		if err != nil {
			return nil, nil, fmt.Errorf("match include patterns for %q: %w", normalizedPath, err)
		}
		inSharedScope, err := matchNormalizedAny(sharedScopePatterns, normalizedPath)
		if err != nil {
			return nil, nil, fmt.Errorf("match shared scope patterns for %q: %w", normalizedPath, err)
		}
		excluded, err := matchNormalizedAny(excludePatterns, normalizedPath)
		if err != nil {
			return nil, nil, fmt.Errorf("match exclude patterns for %q: %w", normalizedPath, err)
		}
		if !excluded && (included || inSharedScope) {
			ownedSet[normalizedPath] = struct{}{}
		}

		projectionVisible, err := matchNormalizedAny(projectionVisiblePatterns, normalizedPath)
		if err != nil {
			return nil, nil, fmt.Errorf("match projection-visible patterns for %q: %w", normalizedPath, err)
		}
		if !excluded && projectionVisible {
			if _, owned := ownedSet[normalizedPath]; !owned {
				projectionOnlySet[normalizedPath] = struct{}{}
			}
		}
	}

	owned := make([]string, 0, len(ownedSet))
	for path := range ownedSet {
		owned = append(owned, path)
	}
	sort.Strings(owned)

	projectionOnly := make([]string, 0, len(projectionOnlySet))
	for path := range projectionOnlySet {
		projectionOnly = append(projectionOnly, path)
	}
	sort.Strings(projectionOnly)

	return owned, projectionOnly, nil
}

func controlReadPaths(config RepositoryConfig) ([]string, error) {
	paths := []string{}
	if config.HasLegacyGlobalConfig {
		paths = append(paths, configRelativePath)
	}

	for _, definition := range config.Orbits {
		relativePath, err := definitionControlPath(definition)
		if err != nil {
			return nil, fmt.Errorf("definition %q: %w", definition.ID, err)
		}

		paths = append(paths, relativePath)
	}

	return mergeSortedUniquePaths(paths), nil
}

func mergeSortedUniquePaths(groups ...[]string) []string {
	mergedSet := make(map[string]struct{})

	for _, group := range groups {
		for _, value := range group {
			if value == "" {
				continue
			}
			mergedSet[value] = struct{}{}
		}
	}

	merged := make([]string, 0, len(mergedSet))
	for value := range mergedSet {
		merged = append(merged, value)
	}

	sort.Strings(merged)

	return merged
}

func isControlPlanePath(normalizedPath string) bool {
	return normalizedPath == configRelativePath || isOrbitDefinitionPath(normalizedPath)
}

func isOrbitDefinitionPath(normalizedPath string) bool {
	if path.Ext(normalizedPath) != ".yaml" {
		return false
	}

	return strings.HasPrefix(normalizedPath, orbitsRelativeDir+"/") ||
		strings.HasPrefix(normalizedPath, hostedOrbitsRelativeDir+"/")
}
