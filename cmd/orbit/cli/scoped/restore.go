package scoped

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// RestoreResult describes the outcome of orbit restore.
type RestoreResult struct {
	CurrentOrbit    string   `json:"current_orbit"`
	TargetRevision  string   `json:"target_revision"`
	ScopeCount      int      `json:"scope_count"`
	CommitHash      string   `json:"commit_hash"`
	WarningMessages []string `json:"warnings,omitempty"`
	RefUpdated      bool     `json:"ref_updated"`
	AutoLeft        bool     `json:"auto_left,omitempty"`
}

const (
	restoreLedgerRefreshWarning = "failed to refresh runtime and git ledger for orbit restore: %v"
	restoreWarningSnapshotError = "failed to refresh warning snapshot for orbit restore: %v"
)

// RestoreOptions configures orbit restore behavior.
type RestoreOptions struct {
	AllowDeleteCurrentOrbit bool
}

// Restore applies a historical scope snapshot and records a normal Git commit.
func Restore(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
	definition orbitpkg.Definition,
	revision string,
	options RestoreOptions,
) (result RestoreResult, err error) {
	lock, err := store.AcquireLock()
	if err != nil {
		return RestoreResult{}, fmt.Errorf("acquire state lock: %w", err)
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
		return RestoreResult{}, err
	}
	scope := runtimePlan.Plan.OrbitWritePaths

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("load worktree status: %w", err)
	}

	snapshot, err := viewpkg.ClassifyOrbitWriteStatusForSpec(config, runtimePlan.Spec, current.Orbit, runtimePlan.Plan, statusEntries)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("classify status: %w", err)
	}
	if len(snapshot.InScope) > 0 {
		return RestoreResult{}, fmt.Errorf(
			"cannot restore orbit %q: scope contains uncommitted changes: %s",
			current.Orbit,
			formatChangedPaths(snapshot.InScope),
		)
	}

	definitionPath, err := orbitpkg.HostedDefinitionRelativePath(current.Orbit)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("resolve current orbit definition path: %w", err)
	}

	definitionExists, err := gitpkg.PathExistsAtRev(ctx, repo.Root, revision, definitionPath)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("check current orbit definition at target revision: %w", err)
	}
	if !definitionExists && !options.AllowDeleteCurrentOrbit {
		return RestoreResult{}, fmt.Errorf(
			"cannot restore orbit %q to %s: current orbit definition %q does not exist at target revision; rerun with --allow-delete-current-orbit to continue",
			current.Orbit,
			revision,
			definitionPath,
		)
	}

	result = RestoreResult{
		CurrentOrbit:   current.Orbit,
		TargetRevision: revision,
		ScopeCount:     len(scope),
	}

	if err := gitpkg.RestorePathspec(ctx, repo.Root, revision, scope); err != nil {
		return result, fmt.Errorf("restore scoped paths: %w", err)
	}

	restoreMessage := BuildRestoreCommitMessage(current.Orbit, revision)
	if err := gitpkg.CommitPathspec(ctx, repo.Root, scope, restoreMessage); err != nil {
		return result, fmt.Errorf("create restore commit: %w", err)
	}

	commitHash, err := gitpkg.HeadCommit(ctx, repo.Root)
	if err != nil {
		return result, fmt.Errorf("resolve head commit after orbit restore: %w", err)
	}
	result.CommitHash = commitHash

	refName, err := gitpkg.LastRestoreRef(current.Orbit)
	if err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to resolve last-restore ref: %v", err))
	} else if err := gitpkg.UpdateRef(ctx, repo.Root, refName, commitHash); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to update %s: %v", refName, err))
	} else {
		result.RefUpdated = true
	}

	updatedAt := time.Now().UTC()
	if !definitionExists && options.AllowDeleteCurrentOrbit {
		if err := gitpkg.DisableSparseCheckout(ctx, repo.Root); err != nil {
			return result, fmt.Errorf("disable sparse-checkout after deleting current orbit definition: %w", err)
		}
		ledgerRefreshErr := error(nil)
		statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
		if err != nil {
			ledgerRefreshErr = fmt.Errorf("load worktree status after auto-leave restore: %w", err)
		} else if err := viewpkg.WriteLeftRuntimeAndGitLedger(
			store,
			current.Orbit,
			current.EnteredAt,
			runtimePlan.Plan.PlanHash,
			updatedAt,
			statusEntries,
		); err != nil {
			ledgerRefreshErr = fmt.Errorf("write runtime and git ledger after auto-leave restore: %w", err)
		}
		if err := store.ClearCurrentOrbit(); err != nil {
			return result, fmt.Errorf("clear current orbit state after deleting current orbit definition: %w", err)
		}

		result.AutoLeft = true
		result.WarningMessages = append(
			result.WarningMessages,
			fmt.Sprintf(
				"current orbit %q does not exist at %s; restored projection and automatically left orbit view",
				current.Orbit,
				revision,
			),
		)
		result.WarningMessages = appendRestoreLedgerRefreshWarning(result.WarningMessages, ledgerRefreshErr)
	}

	if definitionExists || !options.AllowDeleteCurrentOrbit {
		statusEntries, err = gitpkg.WorktreeStatus(ctx, repo.Root)
		if err != nil {
			result.WarningMessages = appendRestoreLedgerRefreshWarning(
				result.WarningMessages,
				fmt.Errorf("load worktree status after orbit restore: %w", err),
			)
			result.WarningMessages = persistRestoreWarningsBestEffort(store, current.Orbit, result.WarningMessages)
			return result, nil
		}
		if err := viewpkg.WriteActiveRuntimeAndGitLedger(
			store,
			config,
			runtimePlan.Spec,
			runtimePlan.Plan,
			current.Orbit,
			"restore",
			current.EnteredAt,
			updatedAt,
			statusEntries,
		); err != nil {
			result.WarningMessages = appendRestoreLedgerRefreshWarning(
				result.WarningMessages,
				fmt.Errorf("write runtime and git ledger after orbit restore: %w", err),
			)
		}
	}

	result.WarningMessages = persistRestoreWarningsBestEffort(store, current.Orbit, result.WarningMessages)
	return result, nil
}

func appendRestoreLedgerRefreshWarning(warnings []string, err error) []string {
	if err == nil {
		return warnings
	}

	return append(warnings, fmt.Sprintf(restoreLedgerRefreshWarning, err))
}

func persistRestoreWarningsBestEffort(store statepkg.FSStore, orbitID string, warnings []string) []string {
	if err := viewpkg.WriteWarnings(store, orbitID, warnings); err != nil {
		return append(warnings, fmt.Sprintf(restoreWarningSnapshotError, err))
	}

	return warnings
}

func formatChangedPaths(changes []statepkg.PathChange) string {
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.Path)
	}

	return strings.Join(paths, ", ")
}
