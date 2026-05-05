package harness

import (
	"context"
	"errors"
	"fmt"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// RunViewPresentationDefaultResult reports whether a runtime-facing flow applied
// Run View presentation cleanup after materializing authored guidance.
type RunViewPresentationDefaultResult struct {
	Applied bool
	Cleanup RuntimeViewCleanupPlanResult
}

// ApplyRunViewPresentationDefault applies Run View cleanup when the runtime's
// selected view is Run View. A missing selection resolves to Run View through
// the state store, so fresh runtimes use clean runtime presentation by default.
func ApplyRunViewPresentationDefault(ctx context.Context, repoRoot string) (RunViewPresentationDefaultResult, error) {
	if repoRoot == "" {
		return RunViewPresentationDefaultResult{}, fmt.Errorf("repo root must not be empty")
	}

	repo, err := gitpkg.DiscoverRepo(ctx, repoRoot)
	if err != nil {
		return RunViewPresentationDefaultResult{}, fmt.Errorf("discover repository git dir: %w", err)
	}
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RunViewPresentationDefaultResult{}, fmt.Errorf("create state store: %w", err)
	}
	selection, err := store.ReadRuntimeViewSelection()
	if err != nil {
		return RunViewPresentationDefaultResult{}, fmt.Errorf("read runtime view selection: %w", err)
	}
	if selection.View != statepkg.RuntimeViewRun {
		return RunViewPresentationDefaultResult{}, nil
	}

	plan, err := RuntimeViewCleanupPlan(ctx, repo, store, false)
	if err != nil {
		return RunViewPresentationDefaultResult{}, err
	}
	result := RunViewPresentationDefaultResult{
		Applied: true,
		Cleanup: plan,
	}
	if len(plan.Blockers) > 0 {
		return result, RuntimeViewCleanupBlockedError{Blockers: append([]string(nil), plan.Blockers...)}
	}

	transactionPaths := runtimeViewCleanupTransactionPaths(plan)
	if len(transactionPaths) == 0 {
		return result, nil
	}
	tx, err := BeginInstallTransaction(ctx, repo.Root, transactionPaths)
	if err != nil {
		return result, fmt.Errorf("begin Run View cleanup transaction: %w", err)
	}
	cleanup, err := RuntimeViewCleanup(ctx, repo, store, false)
	result.Cleanup = cleanup
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return result, errors.Join(
				fmt.Errorf("apply Run View cleanup: %w", err),
				fmt.Errorf("rollback Run View cleanup: %w", rollbackErr),
			)
		}
		return result, err
	}
	tx.Commit()

	return result, nil
}

func runtimeViewCleanupTransactionPaths(plan RuntimeViewCleanupPlanResult) []string {
	paths := make([]string, 0, len(plan.CleanupCandidates))
	for _, candidate := range plan.CleanupCandidates {
		switch candidate.Kind {
		case RuntimeViewCleanupCandidateRootGuidanceMarkerLines, RuntimeViewCleanupCandidateMemberHint:
			paths = append(paths, candidate.Path)
		}
	}

	return slicesCompactStrings(paths)
}
