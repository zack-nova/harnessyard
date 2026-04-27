package view

import (
	"context"
	"errors"
	"fmt"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// LeaveResult describes the outcome of restoring the full tracked workspace view.
type LeaveResult struct {
	Orbit              string   `json:"orbit,omitempty"`
	Left               bool     `json:"left"`
	ProjectionRestored bool     `json:"projection_restored"`
	StateCleared       bool     `json:"state_cleared"`
	WarningMessages    []string `json:"warnings,omitempty"`
}

const (
	leaveLedgerRefreshWarning = "failed to refresh runtime and git ledger for orbit leave: %v"
	leaveWarningSnapshotError = "failed to refresh warning snapshot for orbit leave: %v"
)

// Leave restores the full tracked working tree view and clears current orbit state.
func Leave(ctx context.Context, repo gitpkg.Repo, store statepkg.FSStore) (result LeaveResult, err error) {
	lock, err := store.AcquireLock()
	if err != nil {
		return LeaveResult{}, fmt.Errorf("acquire state lock: %w", err)
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

	sparseEnabled, err := gitpkg.SparseCheckoutEnabled(ctx, repo.Root)
	if err != nil {
		return LeaveResult{}, fmt.Errorf("detect sparse-checkout state: %w", err)
	}

	current, readErr := store.ReadCurrentOrbit()
	statePresent := false
	switch {
	case readErr == nil:
		statePresent = true
		result.Orbit = current.Orbit
	case errors.Is(readErr, statepkg.ErrCurrentOrbitNotFound):
		if sparseEnabled {
			result.WarningMessages = append(
				result.WarningMessages,
				"current orbit state is missing; restored full workspace view without orbit metadata",
			)
		}
	default:
		statePresent = true
		result.WarningMessages = append(
			result.WarningMessages,
			fmt.Sprintf("current orbit state is unreadable; clearing it during leave: %v", readErr),
		)
	}

	if err := gitpkg.DisableSparseCheckout(ctx, repo.Root); err != nil {
		return LeaveResult{}, fmt.Errorf("disable sparse-checkout: %w", err)
	}
	result.ProjectionRestored = sparseEnabled
	if result.Orbit != "" {
		enteredAt, planHash := previousRuntimeLedgerState(store, result.Orbit)
		if enteredAt.IsZero() {
			enteredAt = current.EnteredAt
		}

		ledgerRefreshErr := error(nil)
		statusEntries, statusErr := gitpkg.WorktreeStatus(ctx, repo.Root)
		if statusErr != nil {
			ledgerRefreshErr = fmt.Errorf("load worktree status after leave: %w", statusErr)
		} else if err := WriteLeftRuntimeAndGitLedger(
			store,
			result.Orbit,
			enteredAt,
			planHash,
			time.Now().UTC(),
			statusEntries,
		); err != nil {
			ledgerRefreshErr = err
		}
		result.WarningMessages = appendLeaveLedgerRefreshWarning(result.WarningMessages, ledgerRefreshErr)
	}
	if err := store.ClearCurrentOrbit(); err != nil {
		return LeaveResult{}, fmt.Errorf("clear current orbit state: %w", err)
	}
	result.StateCleared = statePresent
	result.Left = result.ProjectionRestored || result.StateCleared
	if result.Orbit != "" || len(result.WarningMessages) > 0 {
		result.WarningMessages = persistLeaveWarningsBestEffort(store, result.Orbit, result.WarningMessages)
	}

	return result, nil
}

func appendLeaveLedgerRefreshWarning(warnings []string, err error) []string {
	if err == nil {
		return warnings
	}

	return append(warnings, fmt.Sprintf(leaveLedgerRefreshWarning, err))
}

func persistLeaveWarningsBestEffort(store statepkg.FSStore, orbitID string, warnings []string) []string {
	if err := WriteWarnings(store, orbitID, warnings); err != nil {
		return append(warnings, fmt.Sprintf(leaveWarningSnapshotError, err))
	}

	return warnings
}
