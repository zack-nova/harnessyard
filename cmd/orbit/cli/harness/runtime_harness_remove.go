package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// RemoveRuntimeHarnessPackagePlan captures one planned harness-package removal from a runtime.
type RemoveRuntimeHarnessPackagePlan struct {
	HarnessID              string
	Runtime                RuntimeFile
	OrbitIDs               []string
	RemovedPaths           []string
	LocallyChangedPaths    []RuntimeUninstallLocalChange
	ConfirmationRequired   bool
	RemoveRootAgents       bool
	DeleteBundleRecord     bool
	CurrentOrbitRemoved    bool
	ShrinkPlan             BundleMemberShrinkPlan
	DeletePaths            []string
	GuidanceMutations      []runtimeRootGuidanceMutation
	BundleRecordRepoPath   string
	InstallRecordRepoPaths []string
	AgentCleanup           AgentCleanupResult
}

// RemoveRuntimeHarnessPackageResult captures one applied harness-package removal from a runtime.
type RemoveRuntimeHarnessPackageResult struct {
	HarnessID            string
	OrbitIDs             []string
	ManifestPath         string
	Runtime              RuntimeFile
	RemovedPaths         []string
	LocallyChangedPaths  []RuntimeUninstallLocalChange
	ConfirmationRequired bool
	RemovedAgentsBlock   bool
	DeletedBundleRecord  bool
	AutoLeftCurrentOrbit bool
	AgentCleanup         AgentCleanupResult
}

// RemoveRuntimeHarnessPackageOptions controls harness package remove side effects.
type RemoveRuntimeHarnessPackageOptions struct {
	AllowGlobalAgentCleanup bool
	ConfirmLocalChanges     bool
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

	record, err := LoadBundleRecord(repo.Root, harnessID)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("load bundle record for harness package %q: %w", harnessID, err)
	}
	bundleRecordPath, err := BundleRecordRepoPath(record.HarnessID)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}

	orbitIDs := runtimeHarnessUninstallOrbitIDs(runtimeFile, record, harnessID)
	if len(orbitIDs) == 0 {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("harness package %q has no included orbit packages in the current runtime", harnessID)
	}

	deletePaths := runtimeHarnessUninstallDeletePaths(record)
	if err := ensureRuntimeHarnessUninstallOwnershipExclusive(ctx, repo.Root, harnessID, orbitIDs, deletePaths, runtimeFile); err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	installRecordRepoPaths, err := existingRuntimeHarnessUninstallInstallRecordPaths(ctx, repo.Root, orbitIDs)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	guidanceMutations, err := analyzeRuntimeHarnessUninstallRootGuidance(ctx, repo.Root, harnessID)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}

	removedPaths := plannedRuntimeHarnessUninstallRemovedPaths(deletePaths, bundleRecordPath, installRecordRepoPaths, guidanceMutations)

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("create state store: %w", err)
	}
	currentRemoved, err := currentOrbitInSet(store, orbitIDs)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}
	touchedPaths := append([]string{ManifestRepoPath(), bundleRecordPath}, deletePaths...)
	touchedPaths = append(touchedPaths, installRecordRepoPaths...)
	touchedPaths = append(touchedPaths, runtimeHarnessUninstallGuidanceMutationPaths(guidanceMutations)...)
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

	locallyChangedPaths, err := locallyChangedRuntimeUninstallPaths(ctx, repo.Root, removedPaths)
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, err
	}

	agentCleanup, err := PlanAgentCleanupForPackageRemove(ctx, repo.Root, repo.GitDir, orbitIDs, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeHarnessPackagePlan{}, fmt.Errorf("plan agent cleanup for package remove: %w", err)
	}

	return RemoveRuntimeHarnessPackagePlan{
		HarnessID:              harnessID,
		Runtime:                runtimeFile,
		OrbitIDs:               orbitIDs,
		RemovedPaths:           removedPaths,
		LocallyChangedPaths:    append([]RuntimeUninstallLocalChange(nil), locallyChangedPaths...),
		ConfirmationRequired:   true,
		RemoveRootAgents:       runtimeUninstallRemovesGuidance(guidanceMutations),
		DeleteBundleRecord:     true,
		CurrentOrbitRemoved:    currentRemoved,
		DeletePaths:            deletePaths,
		GuidanceMutations:      guidanceMutations,
		BundleRecordRepoPath:   bundleRecordPath,
		InstallRecordRepoPaths: installRecordRepoPaths,
		AgentCleanup:           agentCleanup,
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
	if len(plan.LocallyChangedPaths) > 0 && !options.ConfirmLocalChanges {
		return RemoveRuntimeHarnessPackageResult{}, fmt.Errorf("%s", runtimeHarnessUninstallLocalChangesError(plan.HarnessID, plan.LocallyChangedPaths))
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

	removedPaths := append([]string(nil), plan.DeletePaths...)
	if err := removeRuntimeInfluencePaths(repo.Root, plan.DeletePaths); err != nil {
		return RemoveRuntimeHarnessPackageResult{}, err
	}
	removedDirs, err := cleanupEmptyRuntimeFileParentDirs(repo.Root, plan.DeletePaths)
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, err
	}
	removedPaths = append(removedPaths, removedDirs...)
	guidanceRemovedPaths, err := applyRuntimeUninstallRootGuidance(repo.Root, plan.GuidanceMutations)
	if err != nil {
		return RemoveRuntimeHarnessPackageResult{}, err
	}
	removedPaths = append(removedPaths, guidanceRemovedPaths...)
	for _, installRecordRepoPath := range plan.InstallRecordRepoPaths {
		if err := removeRepoPath(repo.Root, installRecordRepoPath, "install record"); err != nil {
			return RemoveRuntimeHarnessPackageResult{}, err
		}
		removedPaths = append(removedPaths, installRecordRepoPath)
	}
	if err := removeRepoPath(repo.Root, plan.BundleRecordRepoPath, "bundle record"); err != nil {
		return RemoveRuntimeHarnessPackageResult{}, err
	}
	removedPaths = append(removedPaths, plan.BundleRecordRepoPath)
	removedPaths = sortedUniqueStrings(removedPaths)

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
		LocallyChangedPaths:  append([]RuntimeUninstallLocalChange(nil), plan.LocallyChangedPaths...),
		ConfirmationRequired: plan.ConfirmationRequired,
		RemovedAgentsBlock:   len(guidanceRemovedPaths) > 0,
		DeletedBundleRecord:  true,
		AutoLeftCurrentOrbit: autoLeft,
		AgentCleanup:         agentCleanup,
	}, nil
}

func runtimeHarnessUninstallOrbitIDs(runtimeFile RuntimeFile, record BundleRecord, harnessID string) []string {
	orbitIDs := append([]string(nil), record.MemberIDs...)
	orbitIDs = append(orbitIDs, runtimeOrbitIDsOwnedByHarness(runtimeFile, harnessID)...)

	return sortedUniqueStrings(orbitIDs)
}

func runtimeHarnessUninstallDeletePaths(record BundleRecord) []string {
	deletePaths := make([]string, 0, len(record.OwnedPaths))
	for _, path := range record.OwnedPaths {
		if isRootGuidancePath(path) {
			continue
		}
		deletePaths = append(deletePaths, path)
	}

	return sortedUniqueStrings(deletePaths)
}

func existingRuntimeHarnessUninstallInstallRecordPaths(
	ctx context.Context,
	repoRoot string,
	orbitIDs []string,
) ([]string, error) {
	paths := make([]string, 0, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		repoPath, err := InstallRecordRepoPath(orbitID)
		if err != nil {
			return nil, fmt.Errorf("build install record path for included orbit package %q: %w", orbitID, err)
		}
		exists, err := repoPathExistsWorktreeOrHEAD(ctx, repoRoot, repoPath)
		if err != nil {
			return nil, fmt.Errorf("check install record for included orbit package %q: %w", orbitID, err)
		}
		if !exists {
			continue
		}
		paths = append(paths, repoPath)
	}

	return sortedUniqueStrings(paths), nil
}

func repoPathExistsWorktreeOrHEAD(ctx context.Context, repoRoot string, repoPath string) (bool, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	if _, err := os.Stat(filename); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat %s: %w", repoPath, err)
	}

	exists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", repoPath)
	if err != nil {
		if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "ambiguous argument 'HEAD'") {
			return false, nil
		}
		return false, fmt.Errorf("check %s at HEAD: %w", repoPath, err)
	}

	return exists, nil
}

func analyzeRuntimeHarnessUninstallRootGuidance(
	ctx context.Context,
	repoRoot string,
	harnessID string,
) ([]runtimeRootGuidanceMutation, error) {
	targets := []struct {
		path      string
		container string
	}{
		{path: rootAgentsPath, container: "root AGENTS.md"},
		{path: rootHumansPath, container: "root HUMANS.md"},
		{path: rootBootstrapPath, container: "root BOOTSTRAP.md"},
	}
	mutations := make([]runtimeRootGuidanceMutation, 0, len(targets))
	for _, target := range targets {
		data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(ctx, repoRoot, target.path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", target.container, err)
		}
		updated, removed, err := orbittemplate.RemoveRuntimeGuidanceOwnerBlockData(data, orbittemplate.OwnerKindHarness, harnessID, target.container)
		if err != nil {
			return nil, fmt.Errorf("remove %s block: %w", target.container, err)
		}
		if !removed {
			continue
		}
		mutations = append(mutations, runtimeRootGuidanceMutation{
			Path:         target.path,
			Container:    target.container,
			RemovedBlock: true,
			DeleteFile:   len(updated) == 0,
			UpdatedData:  updated,
		})
	}

	return mutations, nil
}

func plannedRuntimeHarnessUninstallRemovedPaths(
	deletePaths []string,
	bundleRecordRepoPath string,
	installRecordRepoPaths []string,
	guidanceMutations []runtimeRootGuidanceMutation,
) []string {
	removedPaths := append([]string(nil), deletePaths...)
	removedPaths = append(removedPaths, bundleRecordRepoPath)
	removedPaths = append(removedPaths, installRecordRepoPaths...)
	removedPaths = append(removedPaths, runtimeHarnessUninstallGuidanceMutationPaths(guidanceMutations)...)

	return sortedUniqueStrings(removedPaths)
}

func runtimeHarnessUninstallGuidanceMutationPaths(mutations []runtimeRootGuidanceMutation) []string {
	paths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.RemovedBlock {
			paths = append(paths, mutation.Path)
		}
	}

	return sortedUniqueStrings(paths)
}

func ensureRuntimeHarnessUninstallOwnershipExclusive(
	ctx context.Context,
	repoRoot string,
	harnessID string,
	orbitIDs []string,
	deletePaths []string,
	runtimeFile RuntimeFile,
) error {
	targetSet := stringSet(orbitIDs)
	plansByOrbitID, err := loadRuntimeRemovePlans(ctx, repoRoot, runtimeFile)
	if err != nil {
		return fmt.Errorf("resolve active runtime references for harness package uninstall: %w", err)
	}
	for _, path := range deletePaths {
		for otherOrbitID, otherPlan := range plansByOrbitID {
			if targetSet[otherOrbitID] {
				continue
			}
			if runtimeRemovePathShared(path, otherPlan.Plan.ProjectionPaths) {
				return fmt.Errorf(
					"cannot uninstall harness package %q: delete candidate %q is still referenced by active package %q",
					harnessID,
					path,
					otherOrbitID,
				)
			}
		}
	}

	deletePathSet := stringSet(deletePaths)
	for _, member := range runtimeFile.Members {
		if targetSet[member.OrbitID] {
			continue
		}
		ownedPaths, err := runtimeUninstallActivePackageOwnedPaths(ctx, repoRoot, member)
		if err != nil {
			return fmt.Errorf("resolve package ownership scope for active package %q: %w", member.OrbitID, err)
		}
		for _, path := range ownedPaths {
			if !deletePathSet[path] {
				continue
			}
			return fmt.Errorf(
				"cannot uninstall harness package %q: delete candidate %q is still owned by active package %q",
				harnessID,
				path,
				member.OrbitID,
			)
		}
	}

	return nil
}

func runtimeHarnessUninstallLocalChangesError(harnessID string, risks []RuntimeUninstallLocalChange) string {
	return fmt.Sprintf(
		"uninstall harness package %q requires --yes to delete locally changed target-owned files: %s; use --dry-run to inspect",
		harnessID,
		strings.Join(formatRuntimeUninstallLocalChanges(risks), ", "),
	)
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
