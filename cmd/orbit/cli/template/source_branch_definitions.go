package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func isAllowedSourceBranchHarnessPath(path string) bool {
	return path == sourceManifestRelativePath || strings.HasPrefix(path, ".harness/orbits/")
}

func loadHostedSourceBranchRepositoryConfig(ctx context.Context, repoRoot string) (orbitpkg.RepositoryConfig, error) {
	config, err := orbitpkg.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("load hosted repository config: %w", err)
	}

	return config, nil
}

func loadSingleHostedSourceOrbitDefinition(ctx context.Context, repoRoot string) (orbitpkg.Definition, error) {
	config, err := loadHostedSourceBranchRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return orbitpkg.Definition{}, fmt.Errorf("load hosted repository config: %w", err)
	}
	if err := orbitpkg.ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return orbitpkg.Definition{}, fmt.Errorf("validate repository config: %w", err)
	}
	orbitpkg.SortDefinitions(config.Orbits)

	switch len(config.Orbits) {
	case 1:
		legacyDefinitions, legacyErr := orbitpkg.DiscoverDefinitions(ctx, repoRoot)
		if legacyErr != nil {
			return orbitpkg.Definition{}, fmt.Errorf("discover legacy orbit definitions: %w", legacyErr)
		}
		if len(legacyDefinitions) > 0 {
			return orbitpkg.Definition{}, fmt.Errorf(
				"source publish requires hosted-only orbit definitions; remove legacy definitions from .orbit/orbits/ or run `orbit source init` to reconcile the source branch",
			)
		}
		return config.Orbits[0], nil
	case 0:
		legacyDefinitions, legacyErr := orbitpkg.DiscoverDefinitions(ctx, repoRoot)
		if legacyErr == nil && len(legacyDefinitions) > 0 {
			return orbitpkg.Definition{}, fmt.Errorf(
				"source publish requires hosted orbit definitions under .harness/orbits/; run `orbit source init` to migrate legacy definitions",
			)
		}
		return orbitpkg.Definition{}, fmt.Errorf("source branch must contain exactly one orbit definition")
	default:
		return orbitpkg.Definition{}, fmt.Errorf("source branch must contain exactly one orbit definition")
	}
}

func removeLegacySourceOrbitDefinition(repoRoot string, definition orbitpkg.Definition) error {
	legacyPath, err := orbitpkg.DefinitionPath(repoRoot, definition.ID)
	if err != nil {
		return fmt.Errorf("build legacy orbit definition path for %q: %w", definition.ID, err)
	}
	if err := os.Remove(legacyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove legacy orbit definition %s: %w", legacyPath, err)
	}

	legacyDir := filepath.Dir(legacyPath)
	if err := os.Remove(legacyDir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("remove legacy orbit definitions dir %s: %w", legacyDir, err)
	}

	return nil
}
