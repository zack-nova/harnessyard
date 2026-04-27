package orbit

import (
	"context"
	"fmt"
	"strings"
)

// LoadOrbitSpecAndProjectionPlan loads one orbit spec and resolves its
// role-aware projection plan for the current tracked file set.
func LoadOrbitSpecAndProjectionPlan(
	ctx context.Context,
	repoRoot string,
	config RepositoryConfig,
	orbitID string,
	trackedFiles []string,
) (OrbitSpec, ProjectionPlan, error) {
	definition, found := config.OrbitByID(orbitID)
	if !found {
		return OrbitSpec{}, ProjectionPlan{}, fmt.Errorf("orbit %q not found", orbitID)
	}

	spec, err := loadSpecForDefinition(ctx, repoRoot, definition)
	if err != nil {
		return OrbitSpec{}, ProjectionPlan{}, fmt.Errorf("load orbit spec: %w", err)
	}

	plan, err := ResolveProjectionPlan(config, spec, trackedFiles)
	if err != nil {
		return OrbitSpec{}, ProjectionPlan{}, fmt.Errorf("resolve projection plan: %w", err)
	}

	return spec, plan, nil
}

func loadSpecForDefinition(ctx context.Context, repoRoot string, definition Definition) (OrbitSpec, error) {
	if repoRelative, ok := sourcePathRepoRelativePath(definition.SourcePath); ok && strings.HasPrefix(repoRelative, hostedOrbitsRelativeDir+"/") {
		return LoadHostedOrbitSpec(ctx, repoRoot, definition.ID)
	}

	return LoadOrbitSpec(ctx, repoRoot, definition.ID)
}
