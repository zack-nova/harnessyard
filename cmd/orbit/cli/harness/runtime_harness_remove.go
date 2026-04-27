package harness

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// RemoveRuntimeHarnessPackagePlan captures one planned harness-package removal from a runtime.
type RemoveRuntimeHarnessPackagePlan struct {
	HarnessID           string
	Runtime             RuntimeFile
	OrbitIDs            []string
	RemovedPaths        []string
	RemoveRootAgents    bool
	DeleteBundleRecord  bool
	CurrentOrbitRemoved bool
	ShrinkPlan          BundleMemberShrinkPlan
	AgentCleanup        AgentCleanupResult
}

// RemoveRuntimeHarnessPackageResult captures one applied harness-package removal from a runtime.
type RemoveRuntimeHarnessPackageResult struct {
	HarnessID            string
	OrbitIDs             []string
	ManifestPath         string
	Runtime              RuntimeFile
	RemovedPaths         []string
	RemovedAgentsBlock   bool
	DeletedBundleRecord  bool
	AutoLeftCurrentOrbit bool
	AgentCleanup         AgentCleanupResult
}

// RemoveRuntimeHarnessPackageOptions controls harness package remove side effects.
type RemoveRuntimeHarnessPackageOptions struct {
	AllowGlobalAgentCleanup bool
}

// BuildRemoveRuntimeHarnessPackagePlan validates and previews removing all active runtime
// members owned by one installed harness package.
func BuildRemoveRuntimeHarnessPackagePlan(
	ctx context.Context,
	repo gitpkg.Repo,
	harnessID string,
) (RemoveRuntimeHarnessPackagePlan, error) {
	return BuildRemoveRuntimeHarnessPackagePlanWithOptions(ctx, repo, harnessID, RemoveRuntimeHarnessPackageOptions{})
}

// BuildRemoveRuntimeHarnessPackagePlanWithOptions validates and previews removing
// all active runtime members owned by one installed harness package.
func BuildRemoveRuntimeHarnessPackagePlanWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	harnessID string,
	options RemoveRuntimeHarnessPackageOptions,
) (RemoveRuntimeHarnessPackagePlan, error) {
	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("load harness runtime: %w", err)
	}

	orbitIDs := runtimeOrbitIDsOwnedByHarness(runtimeFile, harnessID)
	if len(orbitIDs) == 0 {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("harness package %q has no active orbit packages in the current runtime", harnessID)
	}

	record, err := LoadBundleRecord(repo.Root, harnessID)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("load bundle record for harness package %q: %w", harnessID, err)
	}
	bundleRecordPath, err := BundleRecordRepoPath(record.HarnessID)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	earlyCleanCheckPaths := append([]string{ManifestRepoPath(), bundleRecordPath}, record.OwnedPaths...)
	if record.IncludesRootAgents {
		earlyCleanCheckPaths = append(earlyCleanCheckPaths, rootAgentsPath)
	}
	if err := ensureRuntimeHarnessRemovePathsClean(ctx, repo.Root, harnessID, earlyCleanCheckPaths); err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}

	shrinkPlan, err := BuildBundleMemberShrinkPlan(ctx, repo.Root, record, orbitIDs)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("build bundle remove plan for harness package %q: %w", harnessID, err)
	}

	removedPaths := append([]string(nil), shrinkPlan.DeletePaths...)
	removedPaths = append(removedPaths, bundleRecordPath)
	if shrinkPlan.RemoveRootAgentsBlock {
		removedPaths = append(removedPaths, rootAgentsPath)
	}
	removedPaths = sortedUniqueStrings(removedPaths)

	touchedPaths := append([]string{ManifestRepoPath(), bundleRecordPath}, shrinkPlan.DeletePaths...)
	cleanCheckPaths := append([]string{ManifestRepoPath(), bundleRecordPath}, shrinkPlan.DeletePaths...)
	if shrinkPlan.RemoveRootAgentsBlock {
		touchedPaths = append(touchedPaths, rootAgentsPath)
		cleanCheckPaths = append(cleanCheckPaths, rootAgentsPath)
	}
	if err := ensureRuntimeHarnessRemovePathsClean(ctx, repo.Root, harnessID, cleanCheckPaths); err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("create state store: %w", err)
	}
	currentRemoved, err := currentOrbitInSet(store, orbitIDs)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	if len(hiddenPaths) > 0 && !currentRemoved {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf(
			"cannot remove harness package %q while the current orbit projection hides touched paths: %s; leave the current orbit first",
			harnessID,
			strings.Join(hiddenPaths, ", "),
		)
	}

	agentCleanup, err := PlanAgentCleanupForPackageRemove(ctx, repo.Root, repo.GitDir, orbitIDs, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("plan agent cleanup for package remove: %w", err)
	}

	return RemoveRuntimeHarnessPackagePlan{
		HarnessID:           harnessID,
		Runtime:             runtimeFile,
		OrbitIDs:            orbitIDs,
		RemovedPaths:        removedPaths,
		RemoveRootAgents:    shrinkPlan.RemoveRootAgentsBlock,
		DeleteBundleRecord:  shrinkPlan.DeleteBundleRecord,
		CurrentOrbitRemoved: currentRemoved,
		ShrinkPlan:          shrinkPlan,
		AgentCleanup:        agentCleanup,
	}, nil
}

// ApplyRemoveRuntimeHarnessPackagePlan applies a previously validated harness-package removal plan.
func ApplyRemoveRuntimeHarnessPackagePlan(
	ctx context.Context,
	repo gitpkg.Repo,
	plan RemoveRuntimeHarnessPackagePlan,
	now time.Time,
) (RemoveRuntimeHarnessPackageResult, error) {
	return ApplyRemoveRuntimeHarnessPackagePlanWithOptions(ctx, repo, plan, now, RemoveRuntimeHarnessPackageOptions{})
}

// ApplyRemoveRuntimeHarnessPackagePlanWithOptions applies a previously validated
// harness-package removal plan with explicit side-effect options.
func ApplyRemoveRuntimeHarnessPackagePlanWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	plan RemoveRuntimeHarnessPackagePlan,
	now time.Time,
	options RemoveRuntimeHarnessPackageOptions,
) (RemoveRuntimeHarnessPackageResult, error) {
	if agentCleanupBlocked(plan.AgentCleanup) || (agentCleanupRequiresConfirmation(plan.AgentCleanup) && !options.AllowGlobalAgentCleanup) {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("%s", agentCleanupErrorMessage(plan.AgentCleanup))
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("create state store: %w", err)
	}

	autoLeft := false
	if plan.CurrentOrbitRemoved {
		leaveResult, err := viewpkg.Leave(ctx, repo, store)
		if err != nil {
			return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("auto leave current orbit for harness package %q: %w", plan.HarnessID, err)
		}
		autoLeft = leaveResult.Left || leaveResult.StateCleared || leaveResult.ProjectionRestored
	}

	removedPaths, err := ApplyBundleMemberShrinkPlan(repo.Root, plan.ShrinkPlan)
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, err
	}

	removedSet := make(map[string]struct{}, len(plan.OrbitIDs))
	for _, orbitID := range plan.OrbitIDs {
		removedSet[orbitID] = struct{}{}
	}
	nextMembers := make([]RuntimeMember, 0, len(plan.Runtime.Members)-len(removedSet))
	for _, member := range plan.Runtime.Members {
		if _, removed := removedSet[member.OrbitID]; removed {
			continue
		}
		nextMembers = append(nextMembers, member)
	}

	runtimeFile := plan.Runtime
	runtimeFile.Members = nextMembers
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)
	manifestPath, err := WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	agentCleanup, err := ReconcileAgentCleanupAfterPackageRemove(ctx, repo.Root, repo.GitDir, plan.OrbitIDs, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("reconcile agent cleanup for package remove: %w", err)
	}
	if agentCleanupBlocked(agentCleanup) || (agentCleanupRequiresConfirmation(agentCleanup) && !options.AllowGlobalAgentCleanup) {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("%s", agentCleanupErrorMessage(agentCleanup))
	}

	return RemoveRuntimeHarnessPackageResult{
		HarnessID:            plan.HarnessID,
		OrbitIDs:             append([]string(nil), plan.OrbitIDs...),
		ManifestPath:         manifestPath,
		Runtime:              runtimeFile,
		RemovedPaths:         appendAgentCleanupRemovedPaths(removedPaths, agentCleanup),
		RemovedAgentsBlock:   plan.RemoveRootAgents,
		DeletedBundleRecord:  plan.DeleteBundleRecord,
		AutoLeftCurrentOrbit: autoLeft,
		AgentCleanup:         agentCleanup,
	}, nil
}

func runtimeOrbitIDsOwnedByHarness(runtimeFile RuntimeFile, harnessID string) []string {
	orbitIDs := make([]string, 0)
	for _, member := range runtimeFile.Members {
		if member.OwnerHarnessID != harnessID {
			continue
		}
		orbitIDs = append(orbitIDs, member.OrbitID)
	}
	sort.Strings(orbitIDs)

	return orbitIDs
}

func ensureRuntimeHarnessRemovePathsClean(ctx context.Context, repoRoot string, harnessID string, touchedPaths []string) error {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load harness package remove worktree status: %w", err)
	}

	statusByPath := make(map[string]string, len(statusEntries))
	for _, entry := range statusEntries {
		statusByPath[entry.Path] = entry.Code
	}

	dirtyPaths := make([]string, 0)
	for _, path := range sortedUniqueStrings(touchedPaths) {
		code, ok := statusByPath[path]
		if !ok {
			continue
		}
		dirtyPaths = append(dirtyPaths, fmt.Sprintf("%s (%s)", path, code))
	}
	if len(dirtyPaths) == 0 {
		return nil
	}

	return fmt.Errorf(
		"cannot remove harness package %q with uncommitted changes on touched paths: %s",
		harnessID,
		strings.Join(dirtyPaths, ", "),
	)
}

func currentOrbitInSet(store statepkg.FSStore, orbitIDs []string) (bool, error) {
	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("read current orbit state: %w", err)
	}

	for _, orbitID := range orbitIDs {
		if current.Orbit == orbitID {
			return true, nil
		}
	}

	return false, nil
}
