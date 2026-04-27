package view

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// EnterResult describes the outcome of projecting a target orbit into the worktree.
type EnterResult struct {
	Orbit           string   `json:"orbit"`
	ScopeCount      int      `json:"scope_count"`
	HiddenDirty     []string `json:"hidden_dirty"`
	WarningMessages []string `json:"warnings"`
}

const (
	enterLedgerRefreshWarning = "failed to refresh runtime and git ledger for orbit enter: %v"
	enterWarningSnapshotError = "failed to refresh warning snapshot for orbit enter: %v"
)

// Enter projects a target orbit into the current workspace.
func Enter(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	definition orbitpkg.Definition,
) (result EnterResult, err error) {
	lock, err := store.AcquireLock()
	if err != nil {
		return EnterResult{}, fmt.Errorf("acquire state lock: %w", err)
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

	trackedFiles, err := gitpkg.TrackedFiles(ctx, repo.Root)
	if err != nil {
		return EnterResult{}, fmt.Errorf("load tracked files: %w", err)
	}

	spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(ctx, repo.Root, config, definition.ID, trackedFiles)
	if err != nil {
		return EnterResult{}, fmt.Errorf("load target orbit runtime plan: %w", err)
	}
	if err := store.WriteProjectionCache(definition.ID, plan.ProjectionPaths); err != nil {
		return EnterResult{}, fmt.Errorf("write projection cache: %w", err)
	}
	inventorySnapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Now().UTC())
	if err != nil {
		return EnterResult{}, fmt.Errorf("build file inventory snapshot: %w", err)
	}
	if err := store.WriteFileInventorySnapshot(inventorySnapshot); err != nil {
		return EnterResult{}, fmt.Errorf("write file inventory snapshot: %w", err)
	}

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
	if err != nil {
		return EnterResult{}, fmt.Errorf("load worktree status: %w", err)
	}

	hiddenDirty := hiddenDirtyPaths(plan.ProjectionPaths, statusEntries)
	warningMessages, err := enterWarnings(config, spec, plan, definition.ID, hiddenDirty, statusEntries)
	if err != nil {
		return EnterResult{}, fmt.Errorf("classify enter warnings: %w", err)
	}
	result = EnterResult{
		Orbit:           definition.ID,
		ScopeCount:      len(plan.ProjectionPaths),
		HiddenDirty:     hiddenDirty,
		WarningMessages: warningMessages,
	}

	if len(hiddenDirty) > 0 && config.Global.Behavior.BlockSwitchIfHiddenDirty {
		blockErr := fmt.Errorf(
			"cannot enter orbit %q: dirty tracked paths would be hidden: %s",
			definition.ID,
			strings.Join(hiddenDirty, ", "),
		)
		if writeErr := WriteWarnings(store, definition.ID, append([]string{blockErr.Error()}, warningMessages...)); writeErr != nil {
			return result, errors.Join(blockErr, writeErr)
		}
		return result, blockErr
	}

	if err := gitpkg.InitNoConeSparseCheckout(ctx, repo.Root); err != nil {
		return result, fmt.Errorf("initialize sparse-checkout: %w", err)
	}
	if err := gitpkg.SetSparseCheckoutPaths(ctx, repo.Root, plan.ProjectionPaths); err != nil {
		return result, fmt.Errorf("apply sparse-checkout paths: %w", err)
	}
	enteredAt := time.Now().UTC()
	if err := store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         definition.ID,
		EnteredAt:     enteredAt,
		SparseEnabled: true,
	}); err != nil {
		return result, fmt.Errorf("write current orbit state: %w", err)
	}
	if err := WriteActiveRuntimeAndGitLedger(
		store,
		config,
		spec,
		plan,
		definition.ID,
		"entered",
		enteredAt,
		enteredAt,
		statusEntries,
	); err != nil {
		result.WarningMessages = appendEnterLedgerRefreshWarning(result.WarningMessages, err)
	}
	result.WarningMessages = persistEnterWarningsBestEffort(store, definition.ID, result.WarningMessages)

	return result, nil
}

func appendEnterLedgerRefreshWarning(warnings []string, err error) []string {
	if err == nil {
		return warnings
	}

	return append(warnings, fmt.Sprintf(enterLedgerRefreshWarning, err))
}

func persistEnterWarningsBestEffort(store statepkg.FSStore, orbitID string, warnings []string) []string {
	if err := WriteWarnings(store, orbitID, warnings); err != nil {
		return append(warnings, fmt.Sprintf(enterWarningSnapshotError, err))
	}

	return warnings
}

func hiddenDirtyPaths(scope []string, entries []gitpkg.StatusEntry) []string {
	scopeSet := makeScopeSet(scope)
	hidden := make([]string, 0)

	for _, entry := range entries {
		if !entry.Tracked {
			continue
		}
		if _, inScope := scopeSet[entry.Path]; inScope {
			continue
		}
		hidden = append(hidden, entry.Path)
	}

	sort.Strings(hidden)

	return hidden
}

func enterWarnings(
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	plan orbitpkg.ProjectionPlan,
	targetOrbit string,
	hiddenDirty []string,
	entries []gitpkg.StatusEntry,
) ([]string, error) {
	warnings := make([]string, 0)

	if len(plan.SubjectPaths)+len(plan.RulePaths)+len(plan.ProcessPaths) == 0 {
		warnings = append(warnings, fmt.Sprintf("orbit %q resolved to an empty projection scope", targetOrbit))
	}
	if len(hiddenDirty) > 0 {
		warnings = append(warnings, hiddenDirtyWarning(targetOrbit, hiddenDirty))
	}

	outsideUntracked := make([]string, 0)
	for _, entry := range entries {
		if entry.Tracked {
			continue
		}

		classification, err := orbitpkg.ClassifyOrbitPath(config, spec, plan, entry.Path, false)
		if err != nil {
			return nil, fmt.Errorf("classify untracked path %q: %w", entry.Path, err)
		}
		if classification.Projection {
			continue
		}

		outsideUntracked = append(outsideUntracked, entry.Path)
	}
	if len(outsideUntracked) > 0 {
		sort.Strings(outsideUntracked)
		warnings = append(
			warnings,
			fmt.Sprintf("outside untracked files remain in the working tree: %s", strings.Join(outsideUntracked, ", ")),
		)
	}

	return warnings, nil
}

func hiddenDirtyWarning(targetOrbit string, paths []string) string {
	return fmt.Sprintf("entering orbit %q would hide dirty tracked paths: %s", targetOrbit, strings.Join(paths, ", "))
}
