package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// BootstrapReopenInput describes one explicit runtime bootstrap reopen request.
type BootstrapReopenInput struct {
	OrbitID        string
	All            bool
	RestoreSurface bool
	Now            time.Time
}

// BootstrapReopenResult reports one successful runtime bootstrap reopen pass.
type BootstrapReopenResult struct {
	ReopenedOrbits          []string `json:"reopened_orbits,omitempty"`
	AlreadyPendingOrbits    []string `json:"already_pending_orbits,omitempty"`
	RestoredPaths           []string `json:"restored_paths,omitempty"`
	RestoredBootstrapBlocks []string `json:"restored_bootstrap_blocks,omitempty"`
	CreatedBootstrapFile    bool     `json:"created_bootstrap_file"`
}

type runtimeBootstrapReopenPlan struct {
	OrbitID      string
	RestorePaths []string
	RestoreBlock bool
}

type runtimeBootstrapArtifactBackup struct {
	Exists bool
	Data   []byte
}

// ReopenRuntimeBootstrap reopens one or more runtime orbit bootstraps back to pending,
// optionally restoring BOOTSTRAP.md blocks and bootstrap runtime files.
func ReopenRuntimeBootstrap(
	ctx context.Context,
	repo gitpkg.Repo,
	input BootstrapReopenInput,
) (BootstrapReopenResult, error) {
	if input.All && strings.TrimSpace(input.OrbitID) != "" {
		return BootstrapReopenResult{}, fmt.Errorf("--orbit and --all cannot be used together")
	}
	if !input.All && strings.TrimSpace(input.OrbitID) == "" {
		return BootstrapReopenResult{}, fmt.Errorf("either --orbit or --all must be provided")
	}

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return BootstrapReopenResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return BootstrapReopenResult{}, fmt.Errorf("create state store: %w", err)
	}

	statuses, err := resolveBootstrapCompletionStatuses(ctx, repo.Root, repo.GitDir, runtimeFile, BootstrapCompleteInput{
		OrbitID: input.OrbitID,
		All:     input.All,
	})
	if err != nil {
		return BootstrapReopenResult{}, err
	}
	if len(statuses) == 0 {
		return BootstrapReopenResult{}, fmt.Errorf("current runtime does not contain bootstrap-enabled orbits")
	}

	reopenPlans, alreadyPending, err := buildRuntimeBootstrapReopenPlans(ctx, repo.Root, runtimeFile, statuses, input.RestoreSurface)
	if err != nil {
		return BootstrapReopenResult{}, err
	}

	result := BootstrapReopenResult{
		ReopenedOrbits:       orbitIDsFromReopenPlans(reopenPlans),
		AlreadyPendingOrbits: alreadyPending,
	}
	if len(reopenPlans) == 0 {
		return result, nil
	}

	statusByOrbitID := make(map[string]orbittemplate.BootstrapOrbitStatus, len(statuses))
	for _, status := range statuses {
		statusByOrbitID[status.OrbitID] = status
	}

	restoreResult := runtimeBootstrapSurfaceRestoreResult{}
	if input.RestoreSurface {
		restoreResult, err = restoreRuntimeBootstrapSurface(ctx, repo.Root, reopenPlans)
		if err != nil {
			return BootstrapReopenResult{}, err
		}
		result.RestoredPaths = restoreResult.RestoredPaths
		result.RestoredBootstrapBlocks = restoreResult.RestoredBootstrapBlocks
		result.CreatedBootstrapFile = restoreResult.CreatedBootstrapFile
	}

	reopened := make([]string, 0, len(reopenPlans))
	for _, plan := range reopenPlans {
		if err := writeBootstrapPendingState(store, plan.OrbitID, resolveMutationTime(input.Now)); err != nil {
			if input.RestoreSurface {
				rollbackErr := rollbackRuntimeBootstrapSurfaceRestore(repo.Root, restoreResult)
				if rollbackErr != nil {
					err = errors.Join(err, rollbackErr)
				}
			}
			for _, orbitID := range reopened {
				rollbackErr := restoreBootstrapCompletedState(
					store,
					orbitID,
					statusByOrbitID[orbitID].CompletedAt,
					resolveMutationTime(input.Now),
				)
				if rollbackErr != nil {
					err = errors.Join(err, rollbackErr)
				}
			}
			return BootstrapReopenResult{}, err
		}
		reopened = append(reopened, plan.OrbitID)
	}

	return result, nil
}

func buildRuntimeBootstrapReopenPlans(
	ctx context.Context,
	repoRoot string,
	runtimeFile RuntimeFile,
	statuses []orbittemplate.BootstrapOrbitStatus,
	restoreSurface bool,
) ([]runtimeBootstrapReopenPlan, []string, error) {
	reopenPlans := make([]runtimeBootstrapReopenPlan, 0, len(statuses))
	alreadyPending := make([]string, 0)

	var plansByOrbitID map[string]runtimeRemovePlan
	var candidatePaths []string
	if restoreSurface {
		var err error
		plansByOrbitID, err = loadRuntimeRemovePlans(ctx, repoRoot, runtimeFile)
		if err != nil {
			return nil, nil, fmt.Errorf("load bootstrap restore plans: %w", err)
		}
		trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
		if err != nil {
			return nil, nil, fmt.Errorf("load tracked files: %w", err)
		}
		statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
		if err != nil {
			return nil, nil, fmt.Errorf("load worktree status: %w", err)
		}
		candidatePaths, _ = bootstrapCandidatePaths(trackedFiles, statusEntries)
	}

	for _, status := range statuses {
		reopenPlan := orbittemplate.PlanBootstrapReopen(status)
		switch reopenPlan.Action {
		case orbittemplate.BootstrapActionWarningNoOp:
			alreadyPending = append(alreadyPending, status.OrbitID)
			continue
		case orbittemplate.BootstrapActionReject:
			return nil, nil, fmt.Errorf("runtime member %q is not bootstrap-enabled", status.OrbitID)
		}

		plan := runtimeBootstrapReopenPlan{
			OrbitID:      status.OrbitID,
			RestoreBlock: restoreSurface && status.HasBootstrapTemplate,
		}
		if restoreSurface && status.HasBootstrapMembers {
			runtimePlan, ok := plansByOrbitID[status.OrbitID]
			if !ok {
				return nil, nil, fmt.Errorf("runtime member %q is missing hosted definition", status.OrbitID)
			}

			restorePaths, err := resolveBootstrapDeletePaths(runtimePlan.Spec, candidatePaths)
			if err != nil {
				return nil, nil, fmt.Errorf("resolve bootstrap restore paths for %q: %w", status.OrbitID, err)
			}
			if len(restorePaths) == 0 {
				return nil, nil, fmt.Errorf(
					"cannot restore bootstrap runtime files for orbit %q because the current runtime does not expose a stable restore source",
					status.OrbitID,
				)
			}

			resolvedPaths, err := preflightBootstrapRestorePaths(ctx, repoRoot, status.OrbitID, restorePaths)
			if err != nil {
				return nil, nil, err
			}
			plan.RestorePaths = resolvedPaths
		}

		if restoreSurface {
			restorePlan := orbittemplate.PlanBootstrapSurfaceRestore(status)
			switch restorePlan.Action {
			case orbittemplate.BootstrapActionAllow:
			case orbittemplate.BootstrapActionWarningNoOp:
				alreadyPending = append(alreadyPending, status.OrbitID)
				continue
			default:
				return nil, nil, fmt.Errorf("runtime member %q is not bootstrap-enabled", status.OrbitID)
			}
		}

		reopenPlans = append(reopenPlans, plan)
	}

	sort.Strings(alreadyPending)

	return reopenPlans, alreadyPending, nil
}

func preflightBootstrapRestorePaths(ctx context.Context, repoRoot string, orbitID string, paths []string) ([]string, error) {
	restorePaths := make([]string, 0, len(paths))
	for _, path := range sortedUniqueStrings(paths) {
		existsAtHead, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", path)
		if err != nil {
			return nil, fmt.Errorf("check restore source for bootstrap path %q: %w", path, err)
		}
		if !existsAtHead {
			return nil, fmt.Errorf(
				"cannot restore bootstrap runtime files for orbit %q because the current runtime does not expose a stable restore source",
				orbitID,
			)
		}

		headData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, "HEAD", path)
		if err != nil {
			return nil, fmt.Errorf("read restore source for bootstrap path %q: %w", path, err)
		}

		filename := filepath.Join(repoRoot, filepath.FromSlash(path))
		//nolint:gosec // filename is derived from the repo root plus a validated repo-relative path.
		currentData, err := os.ReadFile(filename)
		if err == nil {
			if !bytes.Equal(currentData, headData) {
				return nil, fmt.Errorf(
					"cannot restore bootstrap runtime files for orbit %q because %s already contains conflicting local content",
					orbitID,
					path,
				)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read current bootstrap path %q: %w", path, err)
		}

		restorePaths = append(restorePaths, path)
	}

	return restorePaths, nil
}

type runtimeBootstrapSurfaceRestoreResult struct {
	RestoredPaths           []string
	RestoredBootstrapBlocks []string
	CreatedBootstrapFile    bool
	restoredFilePaths       []string
	bootstrapBackup         runtimeBootstrapArtifactBackup
}

func restoreRuntimeBootstrapSurface(
	ctx context.Context,
	repoRoot string,
	reopenPlans []runtimeBootstrapReopenPlan,
) (runtimeBootstrapSurfaceRestoreResult, error) {
	backup, err := readRuntimeBootstrapArtifactBackup(repoRoot)
	if err != nil {
		return runtimeBootstrapSurfaceRestoreResult{}, err
	}

	restorePaths := make([]string, 0)
	touchedPaths := make([]string, 0)
	for _, plan := range reopenPlans {
		restorePaths = append(restorePaths, plan.RestorePaths...)
		touchedPaths = append(touchedPaths, plan.RestorePaths...)
		if plan.RestoreBlock {
			touchedPaths = append(touchedPaths, rootBootstrapPath)
		}
	}
	touchedPaths = sortedUniqueStrings(touchedPaths)
	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repoRoot, touchedPaths)
	if err != nil {
		return runtimeBootstrapSurfaceRestoreResult{}, err
	}
	if len(hiddenPaths) > 0 {
		return runtimeBootstrapSurfaceRestoreResult{}, fmt.Errorf(
			"cannot reopen bootstrap with --restore-surface while the current orbit projection hides touched paths: %s; leave the current orbit first",
			strings.Join(hiddenPaths, ", "),
		)
	}

	restorePaths = sortedUniqueStrings(restorePaths)
	if len(restorePaths) > 0 {
		if err := gitpkg.RestorePathspec(ctx, repoRoot, "HEAD", restorePaths); err != nil {
			return runtimeBootstrapSurfaceRestoreResult{}, fmt.Errorf("restore bootstrap runtime files: %w", err)
		}
	}

	result := runtimeBootstrapSurfaceRestoreResult{
		bootstrapBackup:   backup,
		restoredFilePaths: append([]string(nil), restorePaths...),
	}

	for _, plan := range reopenPlans {
		if !plan.RestoreBlock {
			continue
		}
		restoreResult, err := orbittemplate.RestoreOrbitBootstrapArtifact(ctx, orbittemplate.BootstrapArtifactRestoreInput{
			RepoRoot: repoRoot,
			OrbitID:  plan.OrbitID,
		})
		if err != nil {
			rollbackErr := rollbackRuntimeBootstrapSurfaceRestore(repoRoot, result)
			if rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
			return runtimeBootstrapSurfaceRestoreResult{}, err
		}
		if !restoreResult.Changed {
			continue
		}
		result.RestoredBootstrapBlocks = append(result.RestoredBootstrapBlocks, plan.OrbitID)
		result.RestoredPaths = append(result.RestoredPaths, rootBootstrapPath)
		if restoreResult.Created {
			result.CreatedBootstrapFile = true
		}
	}

	result.RestoredPaths = sortedUniqueStrings(append(result.RestoredPaths, result.restoredFilePaths...))
	result.RestoredBootstrapBlocks = sortedUniqueStrings(result.RestoredBootstrapBlocks)

	return result, nil
}

func readRuntimeBootstrapArtifactBackup(repoRoot string) (runtimeBootstrapArtifactBackup, error) {
	filename := filepath.Join(repoRoot, rootBootstrapPath)
	//nolint:gosec // filename is the repo-local BOOTSTRAP artifact path under the resolved repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runtimeBootstrapArtifactBackup{}, nil
		}
		return runtimeBootstrapArtifactBackup{}, fmt.Errorf("read runtime BOOTSTRAP.md before restore: %w", err)
	}

	return runtimeBootstrapArtifactBackup{
		Exists: true,
		Data:   append([]byte(nil), data...),
	}, nil
}

func rollbackRuntimeBootstrapSurfaceRestore(repoRoot string, restore runtimeBootstrapSurfaceRestoreResult) error {
	var rollbackErr error

	for _, path := range restore.restoredFilePaths {
		filename := filepath.Join(repoRoot, filepath.FromSlash(path))
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove restored bootstrap file %s: %w", path, err))
		}
	}

	filename := filepath.Join(repoRoot, rootBootstrapPath)
	if restore.bootstrapBackup.Exists {
		if err := contractutil.AtomicWriteFileMode(filename, restore.bootstrapBackup.Data, 0o644); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore runtime BOOTSTRAP.md after reopen failure: %w", err))
		}
	} else if restore.CreatedBootstrapFile || len(restore.RestoredBootstrapBlocks) > 0 {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove restored runtime BOOTSTRAP.md after reopen failure: %w", err))
		}
	}

	return rollbackErr
}

func writeBootstrapPendingState(store statepkg.FSStore, orbitID string, now time.Time) error {
	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		if !errors.Is(err, statepkg.ErrRuntimeStateSnapshotNotFound) {
			return fmt.Errorf("read runtime state snapshot: %w", err)
		}
		snapshot = statepkg.RuntimeStateSnapshot{Orbit: orbitID}
	}
	snapshot.Orbit = orbitID
	snapshot.UpdatedAt = now
	snapshot.Bootstrap = nil
	if err := store.WriteRuntimeStateSnapshot(snapshot); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	return nil
}

func restoreBootstrapCompletedState(store statepkg.FSStore, orbitID string, completedAt time.Time, now time.Time) error {
	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		if !errors.Is(err, statepkg.ErrRuntimeStateSnapshotNotFound) {
			return fmt.Errorf("read runtime state snapshot: %w", err)
		}
		snapshot = statepkg.RuntimeStateSnapshot{Orbit: orbitID}
	}
	snapshot.Orbit = orbitID
	snapshot.UpdatedAt = now
	snapshot.Bootstrap = &statepkg.RuntimeBootstrapState{
		Completed:   true,
		CompletedAt: completedAt,
	}
	if err := store.WriteRuntimeStateSnapshot(snapshot); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	return nil
}

func orbitIDsFromReopenPlans(plans []runtimeBootstrapReopenPlan) []string {
	orbitIDs := make([]string, 0, len(plans))
	for _, plan := range plans {
		orbitIDs = append(orbitIDs, plan.OrbitID)
	}

	return sortedUniqueStrings(orbitIDs)
}
