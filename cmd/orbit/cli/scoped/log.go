package scoped

import (
	"context"
	"fmt"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// LogResult is the rendered orbit log payload.
type LogResult struct {
	CurrentOrbit string   `json:"current_orbit"`
	Paths        []string `json:"paths"`
	GitArgs      []string `json:"git_args"`
	Log          string   `json:"log"`
}

// Log returns a current-orbit path-limited git log.
func Log(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
	definition orbitpkg.Definition,
	gitArgs []string,
) (LogResult, error) {
	scope, err := resolveCurrentScopedOperationScope(ctx, repo, store, config, definition)
	if err != nil {
		return LogResult{}, err
	}

	output, err := gitpkg.LogPathspec(ctx, repo.Root, scope, gitArgs)
	if err != nil {
		return LogResult{}, fmt.Errorf("load git log: %w", err)
	}

	return LogResult{
		CurrentOrbit: current.Orbit,
		Paths:        scope,
		GitArgs:      append([]string(nil), gitArgs...),
		Log:          string(output),
	}, nil
}
