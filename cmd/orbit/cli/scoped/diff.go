package scoped

import (
	"context"
	"fmt"
	"sort"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// DiffOptions configures orbit diff behavior.
type DiffOptions struct {
	Outside bool
}

// DiffResult is the rendered orbit diff payload.
type DiffResult struct {
	CurrentOrbit string   `json:"current_orbit"`
	Outside      bool     `json:"outside"`
	Paths        []string `json:"paths"`
	Diff         string   `json:"diff"`
}

// Diff returns a current-orbit path-limited diff.
func Diff(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
	definition orbitpkg.Definition,
	options DiffOptions,
) (DiffResult, error) {
	scope, err := resolveCurrentScopedOperationScope(ctx, repo, store, config, definition)
	if err != nil {
		return DiffResult{}, err
	}

	paths := append([]string(nil), scope...)
	if options.Outside {
		statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
		if err != nil {
			return DiffResult{}, fmt.Errorf("load worktree status: %w", err)
		}
		paths = outsideDiffPaths(scope, statusEntries)
	}

	output, err := gitpkg.DiffPathspec(ctx, repo.Root, paths)
	if err != nil {
		return DiffResult{}, fmt.Errorf("load git diff: %w", err)
	}

	return DiffResult{
		CurrentOrbit: current.Orbit,
		Outside:      options.Outside,
		Paths:        paths,
		Diff:         string(output),
	}, nil
}

func outsideDiffPaths(scope []string, entries []gitpkg.StatusEntry) []string {
	scopeSet := make(map[string]struct{}, len(scope))
	for _, pathValue := range scope {
		scopeSet[pathValue] = struct{}{}
	}

	outsideSet := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.Tracked {
			continue
		}
		if _, inScope := scopeSet[entry.Path]; inScope {
			continue
		}
		outsideSet[entry.Path] = struct{}{}
	}

	outsidePaths := make([]string, 0, len(outsideSet))
	for pathValue := range outsideSet {
		outsidePaths = append(outsidePaths, pathValue)
	}

	sort.Strings(outsidePaths)

	return outsidePaths
}
