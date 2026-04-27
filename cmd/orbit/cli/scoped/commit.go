package scoped

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// CommitResult describes the outcome of orbit commit.
type CommitResult struct {
	CurrentOrbit    string   `json:"current_orbit"`
	ScopeCount      int      `json:"scope_count"`
	Committed       bool     `json:"committed"`
	CommitHash      string   `json:"commit_hash,omitempty"`
	WarningMessages []string `json:"warnings,omitempty"`
	RefUpdated      bool     `json:"ref_updated"`
}

const (
	commitLedgerRefreshWarning = "failed to refresh runtime and git ledger for orbit commit: %v"
	commitWarningSnapshotError = "failed to refresh warning snapshot for orbit commit: %v"
)

// Commit stages and commits only the current orbit scope.
func Commit(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
	definition orbitpkg.Definition,
	message string,
) (result CommitResult, err error) {
	lock, err := store.AcquireLock()
	if err != nil {
		return CommitResult{}, fmt.Errorf("acquire state lock: %w", err)
	}
	defer func() {
		releaseErr := lock.Release()
		if releaseErr == nil {
			return
		}

		wrappedReleaseErr := fmt.Errorf("release state lock: %w", releaseErr)
		if err == nil {
			err = wrappedReleaseErr
			return
		}

		err = errors.Join(err, wrappedReleaseErr)
	}()

	runtimePlan, err := resolveCurrentOrbitRuntimePlan(ctx, repo, store, config, definition)
	if err != nil {
		return CommitResult{}, err
	}
	scope := runtimePlan.Plan.OrbitWritePaths

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
	if err != nil {
		return CommitResult{}, fmt.Errorf("load worktree status: %w", err)
	}

	snapshot, err := viewpkg.ClassifyOrbitWriteStatusForSpec(config, runtimePlan.Spec, current.Orbit, runtimePlan.Plan, statusEntries)
	if err != nil {
		return CommitResult{}, fmt.Errorf("classify status: %w", err)
	}

	result = CommitResult{
		CurrentOrbit:    current.Orbit,
		ScopeCount:      len(scope),
		WarningMessages: append([]string(nil), snapshot.CommitWarnings...),
	}

	result.WarningMessages = persistCommitWarningsBestEffort(store, current.Orbit, result.WarningMessages)

	if len(snapshot.InScope) == 0 {
		result.WarningMessages = appendCommitLedgerRefreshWarning(
			result.WarningMessages,
			writeCommitLedgerSnapshot(
				store,
				config,
				runtimePlan.Spec,
				runtimePlan.Plan,
				current,
				statusEntries,
			),
		)
		result.WarningMessages = persistCommitWarningsBestEffort(store, current.Orbit, result.WarningMessages)
		return result, nil
	}

	commitPaths := commitPathspec(scope, snapshot.InScope)
	commitMessage := BuildCommitMessage(message, current.Orbit, config.Global.Behavior.CommitAppendTrailer)
	if err := gitpkg.StageAllPathspec(ctx, repo.Root, commitPaths); err != nil {
		return result, fmt.Errorf("stage scoped paths: %w", err)
	}
	if err := gitpkg.CommitPathspec(ctx, repo.Root, commitPaths, commitMessage); err != nil {
		return result, fmt.Errorf("create scoped commit: %w", err)
	}

	commitHash, err := gitpkg.HeadCommit(ctx, repo.Root)
	if err != nil {
		return result, fmt.Errorf("resolve head commit after orbit commit: %w", err)
	}

	result.Committed = true
	result.CommitHash = commitHash

	refName, err := gitpkg.LastScopedRef(current.Orbit)
	if err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to resolve last-scoped ref: %v", err))
	} else if err := gitpkg.UpdateRef(ctx, repo.Root, refName, commitHash); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to update %s: %v", refName, err))
	} else {
		result.RefUpdated = true
	}

	statusEntries, err = gitpkg.WorktreeStatus(ctx, repo.Root)
	if err != nil {
		result.WarningMessages = appendCommitLedgerRefreshWarning(
			result.WarningMessages,
			fmt.Errorf("load worktree status after orbit commit: %w", err),
		)
		result.WarningMessages = persistCommitWarningsBestEffort(store, current.Orbit, result.WarningMessages)
		return result, nil
	}
	result.WarningMessages = appendCommitLedgerRefreshWarning(
		result.WarningMessages,
		writeCommitLedgerSnapshot(
			store,
			config,
			runtimePlan.Spec,
			runtimePlan.Plan,
			current,
			statusEntries,
		),
	)
	result.WarningMessages = persistCommitWarningsBestEffort(store, current.Orbit, result.WarningMessages)

	return result, nil
}

func writeCommitLedgerSnapshot(
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	plan orbitpkg.ProjectionPlan,
	current statepkg.CurrentOrbitState,
	statusEntries []gitpkg.StatusEntry,
) error {
	if err := viewpkg.WriteActiveRuntimeAndGitLedger(
		store,
		config,
		spec,
		plan,
		current.Orbit,
		"commit",
		current.EnteredAt,
		time.Now().UTC(),
		statusEntries,
	); err != nil {
		return fmt.Errorf("write runtime and git ledger for orbit commit: %w", err)
	}

	return nil
}

func appendCommitLedgerRefreshWarning(warnings []string, err error) []string {
	if err == nil {
		return warnings
	}

	return append(warnings, fmt.Sprintf(commitLedgerRefreshWarning, err))
}

func persistCommitWarningsBestEffort(store statepkg.FSStore, orbitID string, warnings []string) []string {
	if err := viewpkg.WriteWarnings(store, orbitID, warnings); err != nil {
		return append(warnings, fmt.Sprintf(commitWarningSnapshotError, err))
	}

	return warnings
}

func commitPathspec(scope []string, changes []statepkg.PathChange) []string {
	pathSet := make(map[string]struct{}, len(scope)+len(changes))
	for _, pathValue := range scope {
		pathSet[pathValue] = struct{}{}
	}
	for _, change := range changes {
		pathSet[change.Path] = struct{}{}
	}

	paths := make([]string, 0, len(pathSet))
	for pathValue := range pathSet {
		paths = append(paths, pathValue)
	}

	sort.Strings(paths)

	return paths
}
