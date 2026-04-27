package scoped

import (
	"context"
	"fmt"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

func resolveCurrentScopedOperationScope(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	definition orbitpkg.Definition,
) ([]string, error) {
	runtimePlan, err := resolveCurrentOrbitRuntimePlan(ctx, repo, store, config, definition)
	if err != nil {
		return nil, err
	}

	return runtimePlan.Plan.OrbitWritePaths, nil
}

type currentOrbitRuntimePlan struct {
	Spec orbitpkg.OrbitSpec
	Plan orbitpkg.ProjectionPlan
}

func resolveCurrentOrbitRuntimePlan(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	definition orbitpkg.Definition,
) (currentOrbitRuntimePlan, error) {
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repo.Root)
	if err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("load tracked files: %w", err)
	}

	spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(ctx, repo.Root, config, definition.ID, trackedFiles)
	if err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("load current orbit runtime plan: %w", err)
	}
	if err := viewpkg.ValidateCurrentRuntimeLedgerPlan(store, definition.ID, plan.PlanHash); err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("validate current runtime ledger plan: %w", err)
	}
	if err := store.WriteProjectionCache(definition.ID, plan.ProjectionPaths); err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("write projection cache: %w", err)
	}
	inventorySnapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Now().UTC())
	if err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("build file inventory snapshot: %w", err)
	}
	if err := store.WriteFileInventorySnapshot(inventorySnapshot); err != nil {
		return currentOrbitRuntimePlan{}, fmt.Errorf("write file inventory snapshot: %w", err)
	}

	return currentOrbitRuntimePlan{
		Spec: spec,
		Plan: plan,
	}, nil
}
