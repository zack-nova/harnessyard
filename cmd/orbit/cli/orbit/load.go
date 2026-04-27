package orbit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ControlLoader reads the Orbit control plane independently from the current sparse view.
type ControlLoader interface {
	LoadGlobalConfig(ctx context.Context, repoRoot string) (GlobalConfig, error)
	ListDefinitions(ctx context.Context, repoRoot string) ([]Definition, error)
	LoadDefinition(ctx context.Context, repoRoot string, orbitID string) (Definition, error)
}

// GitControlLoader reads Orbit control files from the worktree when visible,
// and falls back to HEAD when sparse-checkout currently hides them.
type GitControlLoader struct{}

// NewGitControlLoader returns the default control-plane loader for Orbit.
func NewGitControlLoader() GitControlLoader {
	return GitControlLoader{}
}

// LoadRepositoryConfig loads the versioned Orbit configuration from the repository control plane.
func LoadRepositoryConfig(ctx context.Context, repoRoot string) (RepositoryConfig, error) {
	loader := NewGitControlLoader()

	globalConfig, err := loader.LoadGlobalConfig(ctx, repoRoot)
	if err != nil {
		return RepositoryConfig{}, fmt.Errorf("load global config: %w", err)
	}

	definitions, err := loader.ListDefinitions(ctx, repoRoot)
	if err != nil {
		return RepositoryConfig{}, fmt.Errorf("load orbit definitions: %w", err)
	}

	return RepositoryConfig{
		Global:                globalConfig,
		Orbits:                definitions,
		HasLegacyGlobalConfig: true,
	}, nil
}

// LoadRuntimeRepositoryConfig loads the runtime repo control plane using legacy
// global config plus hosted orbit definitions.
func LoadRuntimeRepositoryConfig(ctx context.Context, repoRoot string) (RepositoryConfig, error) {
	definitions, err := LoadHostedDefinitions(ctx, repoRoot)
	if err != nil {
		return RepositoryConfig{}, fmt.Errorf("load orbit definitions: %w", err)
	}

	globalConfig, hasLegacyGlobalConfig, err := loadRuntimeGlobalConfig(ctx, repoRoot, len(definitions) > 0)
	if err != nil {
		return RepositoryConfig{}, err
	}

	return RepositoryConfig{
		Global:                globalConfig,
		Orbits:                definitions,
		HasLegacyGlobalConfig: hasLegacyGlobalConfig,
	}, nil
}

// LoadGlobalConfig loads .orbit/config.yaml and applies documented defaults.
func LoadGlobalConfig(ctx context.Context, repoRoot string) (GlobalConfig, error) {
	return NewGitControlLoader().LoadGlobalConfig(ctx, repoRoot)
}

// LoadDefinitions loads all orbit YAML files from the Orbit control plane.
func LoadDefinitions(ctx context.Context, repoRoot string) ([]Definition, error) {
	return NewGitControlLoader().ListDefinitions(ctx, repoRoot)
}

// LoadHostedRepositoryConfig loads hosted orbit definitions from .harness/orbits/ with default global config.
func LoadHostedRepositoryConfig(ctx context.Context, repoRoot string) (RepositoryConfig, error) {
	definitions, err := LoadHostedDefinitions(ctx, repoRoot)
	if err != nil {
		return RepositoryConfig{}, fmt.Errorf("load hosted orbit definitions: %w", err)
	}

	return RepositoryConfig{
		Global:                DefaultGlobalConfig(),
		Orbits:                definitions,
		HasLegacyGlobalConfig: false,
	}, nil
}

func loadRuntimeGlobalConfig(ctx context.Context, repoRoot string, allowDefault bool) (GlobalConfig, bool, error) {
	globalConfig, err := LoadGlobalConfig(ctx, repoRoot)
	if err == nil {
		return globalConfig, true, nil
	}
	if allowDefault && errors.Is(err, os.ErrNotExist) {
		return DefaultGlobalConfig(), false, nil
	}

	return GlobalConfig{}, false, fmt.Errorf("load global config: %w", err)
}

// LoadHostedDefinitions loads all hosted orbit YAML files from .harness/orbits/.
func LoadHostedDefinitions(ctx context.Context, repoRoot string) ([]Definition, error) {
	return listDefinitionsAtHost(ctx, repoRoot, hostedOrbitsRelativeDir, func(data []byte, sourcePath string) (OrbitSpec, error) {
		return parseOrbitSpecDataWithPathBuilder(data, sourcePath, false, HostedDefinitionRelativePath)
	})
}

// LoadHostedOrbitSpecs loads all hosted orbit control documents from .harness/orbits/.
func LoadHostedOrbitSpecs(ctx context.Context, repoRoot string) ([]OrbitSpec, error) {
	return listOrbitSpecsAtHost(ctx, repoRoot, hostedOrbitsRelativeDir, func(data []byte, sourcePath string) (OrbitSpec, error) {
		return parseOrbitSpecDataWithPathBuilder(data, sourcePath, false, HostedDefinitionRelativePath)
	})
}

// LoadHostedOrbitSpec loads one hosted orbit control document using the strict hosted parser.
func LoadHostedOrbitSpec(ctx context.Context, repoRoot string, orbitID string) (OrbitSpec, error) {
	return loadOrbitSpecAtHost(ctx, repoRoot, orbitID, HostedDefinitionRelativePath, ParseHostedOrbitSpecData)
}

// DiscoverHostedDefinitions lists parseable hosted orbit definitions for discovery-oriented commands.
func DiscoverHostedDefinitions(ctx context.Context, repoRoot string) ([]Definition, error) {
	return discoverDefinitionsAtHost(ctx, repoRoot, hostedOrbitsRelativeDir, func(data []byte, sourcePath string) (OrbitSpec, error) {
		return parseOrbitSpecDataWithPathBuilder(data, sourcePath, false, HostedDefinitionRelativePath)
	})
}

// LoadOrbitSpec loads one orbit control document using the strict compatibility parser.
func LoadOrbitSpec(ctx context.Context, repoRoot string, orbitID string) (OrbitSpec, error) {
	return loadOrbitSpecAtHost(ctx, repoRoot, orbitID, DefinitionRelativePath, ParseOrbitSpecData)
}

// DiscoverDefinitions lists parseable orbit definitions for discovery-oriented commands.
func DiscoverDefinitions(ctx context.Context, repoRoot string) ([]Definition, error) {
	return discoverDefinitionsAtHost(ctx, repoRoot, orbitsRelativeDir, ParseOrbitSpecData)
}

// LoadGlobalConfig implements the default control-plane loader.
func (GitControlLoader) LoadGlobalConfig(ctx context.Context, repoRoot string) (GlobalConfig, error) {
	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, configRelativePath)
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("read %s: %w", ConfigPath(repoRoot), err)
	}

	config, err := ParseGlobalConfigData(data)
	if err != nil {
		return GlobalConfig{}, fmt.Errorf("parse %s: %w", ConfigPath(repoRoot), err)
	}

	return config, nil
}

// ListDefinitions implements the default control-plane loader.
func (GitControlLoader) ListDefinitions(ctx context.Context, repoRoot string) ([]Definition, error) {
	return listDefinitionsAtHost(ctx, repoRoot, orbitsRelativeDir, ParseOrbitSpecData)
}

// LoadDefinition implements the default control-plane loader.
func (GitControlLoader) LoadDefinition(ctx context.Context, repoRoot string, orbitID string) (Definition, error) {
	return loadDefinitionAtHost(ctx, repoRoot, orbitID, DefinitionRelativePath, ParseOrbitSpecData)
}

func discoverDefinitionsAtHost(
	ctx context.Context,
	repoRoot string,
	relativeDir string,
	parser func([]byte, string) (OrbitSpec, error),
) ([]Definition, error) {
	paths, err := definitionCandidatePaths(ctx, repoRoot, relativeDir)
	if err != nil {
		return nil, err
	}

	definitions := make([]Definition, 0, len(paths))
	for _, relativePath := range paths {
		data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, relativePath)
		if err != nil {
			continue
		}

		spec, err := parser(data, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			continue
		}

		definition, err := compatibilityDefinitionFromOrbitSpecWithValidation(spec, false)
		if err != nil {
			continue
		}
		if err := ids.ValidateOrbitID(definition.ID); err != nil {
			continue
		}

		definitions = append(definitions, definition)
	}

	SortDefinitions(definitions)

	return definitions, nil
}

func listDefinitionsAtHost(
	ctx context.Context,
	repoRoot string,
	relativeDir string,
	parser func([]byte, string) (OrbitSpec, error),
) ([]Definition, error) {
	paths, err := definitionCandidatePaths(ctx, repoRoot, relativeDir)
	if err != nil {
		return nil, err
	}

	definitions := make([]Definition, 0, len(paths))
	for _, relativePath := range paths {
		data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, relativePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
		}

		spec, err := parser(data, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
		}

		definition, err := compatibilityDefinitionFromOrbitSpecWithValidation(spec, false)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
		}

		definitions = append(definitions, definition)
	}

	return definitions, nil
}

func loadOrbitSpecAtHost(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	pathBuilder func(string) (string, error),
	parser func([]byte, string) (OrbitSpec, error),
) (OrbitSpec, error) {
	relativePath, err := pathBuilder(orbitID)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("build definition path: %w", err)
	}

	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, relativePath)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("read %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
	}

	spec, err := parser(data, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("parse %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
	}

	return spec, nil
}

func listOrbitSpecsAtHost(
	ctx context.Context,
	repoRoot string,
	relativeDir string,
	parser func([]byte, string) (OrbitSpec, error),
) ([]OrbitSpec, error) {
	paths, err := definitionCandidatePaths(ctx, repoRoot, relativeDir)
	if err != nil {
		return nil, err
	}

	specs := make([]OrbitSpec, 0, len(paths))
	for _, relativePath := range paths {
		data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, relativePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
		}

		spec, err := parser(data, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
		}

		specs = append(specs, spec)
	}

	return specs, nil
}

func loadDefinitionAtHost(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	pathBuilder func(string) (string, error),
	parser func([]byte, string) (OrbitSpec, error),
) (Definition, error) {
	spec, err := loadOrbitSpecAtHost(ctx, repoRoot, orbitID, pathBuilder, parser)
	if err != nil {
		return Definition{}, err
	}

	definition, err := compatibilityDefinitionFromOrbitSpecWithValidation(spec, false)
	if err != nil {
		relativePath, pathErr := pathBuilder(orbitID)
		if pathErr != nil {
			return Definition{}, fmt.Errorf("parse orbit spec: %w", err)
		}
		return Definition{}, fmt.Errorf("parse %s: %w", filepath.Join(repoRoot, filepath.FromSlash(relativePath)), err)
	}

	return definition, nil
}

func definitionCandidatePaths(ctx context.Context, repoRoot string, relativeDir string) ([]string, error) {
	worktreePaths, err := worktreeDefinitionPaths(repoRoot, relativeDir)
	if err != nil {
		return nil, err
	}

	headPaths, err := gitpkg.ListFilesAtRev(ctx, repoRoot, "HEAD", relativeDir)
	if err != nil {
		return nil, fmt.Errorf("list orbit definitions at HEAD: %w", err)
	}

	seen := make(map[string]struct{}, len(worktreePaths)+len(headPaths))
	for _, relativePath := range worktreePaths {
		seen[relativePath] = struct{}{}
	}
	for _, relativePath := range headPaths {
		if strings.ToLower(filepath.Ext(relativePath)) != ".yaml" {
			continue
		}
		seen[relativePath] = struct{}{}
	}

	paths := make([]string, 0, len(seen))
	for relativePath := range seen {
		paths = append(paths, relativePath)
	}

	sort.Strings(paths)

	return paths, nil
}

func worktreeDefinitionPaths(repoRoot string, relativeDir string) ([]string, error) {
	orbitsDir := filepath.Join(repoRoot, filepath.FromSlash(relativeDir))

	entries, err := os.ReadDir(orbitsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", orbitsDir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}

		relativePath, err := ids.NormalizeRepoRelativePath(filepath.ToSlash(filepath.Join(relativeDir, entry.Name())))
		if err != nil {
			return nil, fmt.Errorf("normalize definition path %q: %w", entry.Name(), err)
		}

		paths = append(paths, relativePath)
	}

	sort.Strings(paths)

	return paths, nil
}
