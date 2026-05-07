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
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// RemoveRuntimeMemberResult captures one successful runtime cleanup remove.
type RemoveRuntimeMemberResult struct {
	ManifestPath          string
	Runtime               RuntimeFile
	RemovedPaths          []string
	LocallyChangedPaths   []RuntimeUninstallLocalChange
	ConfirmationRequired  bool
	RemovedAgentsBlock    bool
	AutoLeftCurrentOrbit  bool
	DetachedInstallRecord bool
	AgentCleanup          AgentCleanupResult
}

// RemoveRuntimeMemberOptions controls package-remove side effects.
type RemoveRuntimeMemberOptions struct {
	AllowGlobalAgentCleanup bool
	ConfirmLocalChanges     bool
}

// RuntimeUninstallLocalChange captures one locally changed path that uninstall plans to delete.
type RuntimeUninstallLocalChange struct {
	Path      string `json:"path"`
	GitStatus string `json:"git_status"`
	Tracked   bool   `json:"tracked"`
	Staged    bool   `json:"staged"`
}

// RuntimeOrbitPackageUninstallPlan captures one planned install-backed Orbit Package uninstall.
type RuntimeOrbitPackageUninstallPlan struct {
	OrbitID               string
	Runtime               RuntimeFile
	RemovedPaths          []string
	LocallyChangedPaths   []RuntimeUninstallLocalChange
	ConfirmationRequired  bool
	RemovedAgentsBlock    bool
	CurrentOrbitRemoved   bool
	DetachedInstallRecord bool
	OwnershipPlan         orbittemplate.InstallOwnedCleanupPlan
	GuidanceMutations     []runtimeRootGuidanceMutation
	DefinitionRepoPath    string
	InstallRecordRepoPath string
	AgentCleanup          AgentCleanupResult
}

type runtimeRemovePlan struct {
	Member RuntimeMember
	Spec   orbitpkg.OrbitSpec
	Plan   orbitpkg.ProjectionPlan
}

type runtimeAgentsMutation struct {
	WillTouch    bool
	RemovedBlock bool
	DeleteFile   bool
	UpdatedData  []byte
}

type runtimeRootGuidanceMutation struct {
	Path         string
	Container    string
	RemovedBlock bool
	DeleteFile   bool
	UpdatedData  []byte
}

// RemoveRuntimeMember removes one active runtime member and cleans its default runtime influence surface.
func RemoveRuntimeMember(
	ctx context.Context,
	repo gitpkg.Repo,
	orbitID string,
	now time.Time,
) (RemoveRuntimeMemberResult, error) {
	return RemoveRuntimeMemberWithOptions(ctx, repo, orbitID, now, RemoveRuntimeMemberOptions{})
}

// UninstallRuntimeOrbitPackage fully removes one install-backed Orbit Package from the runtime.
func UninstallRuntimeOrbitPackage(
	ctx context.Context,
	repo gitpkg.Repo,
	orbitID string,
	now time.Time,
) (RemoveRuntimeMemberResult, error) {
	return UninstallRuntimeOrbitPackageWithOptions(ctx, repo, orbitID, now, RemoveRuntimeMemberOptions{})
}

// UninstallRuntimeOrbitPackageWithOptions fully removes one install-backed Orbit Package from the runtime.
func UninstallRuntimeOrbitPackageWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	orbitID string,
	now time.Time,
	options RemoveRuntimeMemberOptions,
) (RemoveRuntimeMemberResult, error) {
	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	targetMember, found := findRuntimeMember(runtimeFile, orbitID)
	if !found {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("member %q not found", orbitID)
	}
	if targetMember.Source != MemberSourceInstallOrbit {
		return RemoveRuntimeMemberWithOptions(ctx, repo, orbitID, now, options)
	}

	plan, err := BuildUninstallRuntimeOrbitPackagePlanWithOptions(ctx, repo, orbitID, options)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	return ApplyUninstallRuntimeOrbitPackagePlanWithOptions(ctx, repo, plan, now, options)
}

// BuildUninstallRuntimeOrbitPackagePlanWithOptions validates and previews fully uninstalling
// one install-backed Orbit Package from the runtime.
func BuildUninstallRuntimeOrbitPackagePlanWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	orbitID string,
	options RemoveRuntimeMemberOptions,
) (RuntimeOrbitPackageUninstallPlan, error) {
	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("load harness runtime: %w", err)
	}

	targetMember, found := findRuntimeMember(runtimeFile, orbitID)
	if !found {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("member %q not found", orbitID)
	}
	if targetMember.Source != MemberSourceInstallOrbit {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("member %q is not an install-backed orbit package", orbitID)
	}

	installRecord, err := LoadInstallRecord(repo.Root, orbitID)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("load install record for %q: %w", orbitID, err)
	}
	if orbittemplate.EffectiveInstallRecordStatus(installRecord) != orbittemplate.InstallRecordStatusActive {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("install record for orbit %q is not active", orbitID)
	}

	ownershipPlan, err := orbittemplate.BuildInstallOwnedUninstallPlan(ctx, repo.Root, installRecord)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("build package ownership scope for %q: %w", orbitID, err)
	}
	definitionRepoPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("build hosted orbit definition path: %w", err)
	}
	installRecordRepoPath, err := InstallRecordRepoPath(orbitID)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, err
	}
	guidanceMutations, err := analyzeRuntimeUninstallRootGuidance(ctx, repo.Root, orbitID)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, err
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("create state store: %w", err)
	}
	currentIsTarget, err := currentOrbitMatches(store, orbitID)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, err
	}

	touchedPaths := append([]string{
		ManifestRepoPath(),
		definitionRepoPath,
		installRecordRepoPath,
	}, ownershipPlan.DeletePaths...)
	for _, mutation := range guidanceMutations {
		if mutation.RemovedBlock {
			touchedPaths = append(touchedPaths, mutation.Path)
		}
	}

	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, err
	}
	if len(hiddenPaths) > 0 && !currentIsTarget {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf(
			"cannot uninstall orbit package %q while the current orbit projection hides touched paths: %s; leave the current orbit first",
			orbitID,
			strings.Join(hiddenPaths, ", "),
		)
	}

	removedPaths := plannedRuntimeUninstallRemovedPaths(ownershipPlan.DeletePaths, definitionRepoPath, installRecordRepoPath, guidanceMutations)
	locallyChangedPaths, err := locallyChangedRuntimeUninstallPaths(ctx, repo.Root, removedPaths)
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, err
	}

	agentCleanupPlan, err := PlanAgentCleanupForPackageRemove(ctx, repo.Root, repo.GitDir, []string{orbitID}, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RuntimeOrbitPackageUninstallPlan{}, fmt.Errorf("plan agent cleanup for package uninstall: %w", err)
	}

	return RuntimeOrbitPackageUninstallPlan{
		OrbitID:               orbitID,
		Runtime:               runtimeFile,
		RemovedPaths:          removedPaths,
		LocallyChangedPaths:   locallyChangedPaths,
		ConfirmationRequired:  len(locallyChangedPaths) > 0,
		RemovedAgentsBlock:    runtimeUninstallRemovesGuidance(guidanceMutations),
		CurrentOrbitRemoved:   currentIsTarget,
		DetachedInstallRecord: false,
		OwnershipPlan:         ownershipPlan,
		GuidanceMutations:     guidanceMutations,
		DefinitionRepoPath:    definitionRepoPath,
		InstallRecordRepoPath: installRecordRepoPath,
		AgentCleanup:          agentCleanupPlan,
	}, nil
}

// ApplyUninstallRuntimeOrbitPackagePlanWithOptions applies a previously validated
// install-backed Orbit Package uninstall plan.
func ApplyUninstallRuntimeOrbitPackagePlanWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	plan RuntimeOrbitPackageUninstallPlan,
	now time.Time,
	options RemoveRuntimeMemberOptions,
) (RemoveRuntimeMemberResult, error) {
	if agentCleanupBlocked(plan.AgentCleanup) || (agentCleanupRequiresConfirmation(plan.AgentCleanup) && !options.AllowGlobalAgentCleanup) {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("%s", agentCleanupErrorMessage(plan.AgentCleanup))
	}
	if plan.ConfirmationRequired && !options.ConfirmLocalChanges {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("%s", runtimeUninstallLocalChangesError(plan.OrbitID, plan.LocallyChangedPaths))
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("create state store: %w", err)
	}

	autoLeft := false
	if plan.CurrentOrbitRemoved {
		leaveResult, err := viewpkg.Leave(ctx, repo, store)
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("auto leave current orbit %q: %w", plan.OrbitID, err)
		}
		autoLeft = leaveResult.Left || leaveResult.StateCleared || leaveResult.ProjectionRestored
	}

	removedPaths := append([]string(nil), plan.OwnershipPlan.DeletePaths...)
	if err := removeRuntimeInfluencePaths(repo.Root, plan.OwnershipPlan.DeletePaths); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	guidanceRemovedPaths, err := applyRuntimeUninstallRootGuidance(repo.Root, plan.GuidanceMutations)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	removedPaths = append(removedPaths, guidanceRemovedPaths...)
	if err := removeRepoPath(repo.Root, plan.DefinitionRepoPath, "hosted orbit definition"); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	removedPaths = append(removedPaths, plan.DefinitionRepoPath)
	if err := removeRepoPath(repo.Root, plan.InstallRecordRepoPath, "install record"); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	removedPaths = append(removedPaths, plan.InstallRecordRepoPath)
	removedPaths = sortedUniqueStrings(removedPaths)

	runtimeFile := plan.Runtime
	nextMembers := make([]RuntimeMember, 0, len(runtimeFile.Members)-1)
	for _, member := range runtimeFile.Members {
		if member.OrbitID == plan.OrbitID {
			continue
		}
		nextMembers = append(nextMembers, member)
	}
	runtimeFile.Members = nextMembers
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)

	manifestPath, err := WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	agentCleanup, err := ReconcileAgentCleanupAfterPackageRemove(ctx, repo.Root, repo.GitDir, []string{plan.OrbitID}, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("reconcile agent cleanup for package uninstall: %w", err)
	}

	return RemoveRuntimeMemberResult{
		ManifestPath:          manifestPath,
		Runtime:               runtimeFile,
		RemovedPaths:          appendAgentCleanupRemovedPaths(removedPaths, agentCleanup),
		LocallyChangedPaths:   append([]RuntimeUninstallLocalChange(nil), plan.LocallyChangedPaths...),
		ConfirmationRequired:  plan.ConfirmationRequired,
		RemovedAgentsBlock:    len(guidanceRemovedPaths) > 0,
		AutoLeftCurrentOrbit:  autoLeft,
		DetachedInstallRecord: false,
		AgentCleanup:          agentCleanup,
	}, nil
}

// RemoveRuntimeMemberWithOptions removes one active runtime member and cleans its default runtime influence surface.
func RemoveRuntimeMemberWithOptions(
	ctx context.Context,
	repo gitpkg.Repo,
	orbitID string,
	now time.Time,
	options RemoveRuntimeMemberOptions,
) (RemoveRuntimeMemberResult, error) {
	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("load harness runtime: %w", err)
	}

	targetMember, found := findRuntimeMember(runtimeFile, orbitID)
	if !found {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("member %q not found", orbitID)
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("create state store: %w", err)
	}
	currentIsTarget, err := currentOrbitMatches(store, orbitID)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	agentCleanupPlan, err := PlanAgentCleanupForPackageRemove(ctx, repo.Root, repo.GitDir, []string{orbitID}, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("plan agent cleanup for package remove: %w", err)
	}
	if agentCleanupBlocked(agentCleanupPlan) || agentCleanupRequiresConfirmation(agentCleanupPlan) {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("%s", agentCleanupErrorMessage(agentCleanupPlan))
	}

	if targetMember.Source == MemberSourceInstallBundle {
		if strings.TrimSpace(targetMember.OwnerHarnessID) == "" {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("bundle-backed member %q is missing owner_harness_id", orbitID)
		}
		record, err := LoadBundleRecord(repo.Root, targetMember.OwnerHarnessID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return RemoveRuntimeMemberResult{}, fmt.Errorf(
					"bundle-backed member %q has no bundle record for owner_harness_id %q",
					orbitID,
					targetMember.OwnerHarnessID,
				)
			}
			return RemoveRuntimeMemberResult{}, fmt.Errorf("load bundle record for %q: %w", targetMember.OwnerHarnessID, err)
		}
		shrinkPlan, err := buildBundleMemberShrinkPlan(ctx, repo.Root, record, []string{orbitID})
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("build bundle shrink plan for %q: %w", orbitID, err)
		}

		bundleRecordPath, err := BundleRecordRepoPath(record.HarnessID)
		if err != nil {
			return RemoveRuntimeMemberResult{}, err
		}
		touchedPaths := append([]string{ManifestRepoPath(), bundleRecordPath}, shrinkPlan.DeletePaths...)
		cleanCheckPaths := append([]string(nil), shrinkPlan.DeletePaths...)
		if shrinkPlan.RemoveRootAgentsBlock {
			touchedPaths = append(touchedPaths, rootAgentsPath)
			cleanCheckPaths = append(cleanCheckPaths, rootAgentsPath)
		}
		if err := ensureRuntimeRemovePathsClean(ctx, repo.Root, orbitID, cleanCheckPaths); err != nil {
			return RemoveRuntimeMemberResult{}, err
		}

		hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
		if err != nil {
			return RemoveRuntimeMemberResult{}, err
		}
		if len(hiddenPaths) > 0 && !currentIsTarget {
			return RemoveRuntimeMemberResult{}, fmt.Errorf(
				"cannot remove runtime member %q while the current orbit projection hides touched paths: %s; leave the current orbit first",
				orbitID,
				strings.Join(hiddenPaths, ", "),
			)
		}

		autoLeft := false
		if currentIsTarget {
			leaveResult, err := viewpkg.Leave(ctx, repo, store)
			if err != nil {
				return RemoveRuntimeMemberResult{}, fmt.Errorf("auto leave current orbit %q: %w", orbitID, err)
			}
			autoLeft = leaveResult.Left || leaveResult.StateCleared || leaveResult.ProjectionRestored
		}

		removedPaths, err := applyBundleMemberShrinkPlan(repo.Root, shrinkPlan)
		if err != nil {
			return RemoveRuntimeMemberResult{}, err
		}

		nextMembers := make([]RuntimeMember, 0, len(runtimeFile.Members)-1)
		for _, member := range runtimeFile.Members {
			if member.OrbitID == orbitID {
				continue
			}
			nextMembers = append(nextMembers, member)
		}
		runtimeFile.Members = nextMembers
		runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)

		manifestPath, err := WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("write harness manifest: %w", err)
		}

		agentCleanup, err := ReconcileAgentCleanupAfterPackageRemove(ctx, repo.Root, repo.GitDir, []string{orbitID}, AgentCleanupOptions{
			AllowGlobal: options.AllowGlobalAgentCleanup,
		})
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("reconcile agent cleanup for package remove: %w", err)
		}

		return RemoveRuntimeMemberResult{
			ManifestPath:          manifestPath,
			Runtime:               runtimeFile,
			RemovedPaths:          appendAgentCleanupRemovedPaths(removedPaths, agentCleanup),
			RemovedAgentsBlock:    shrinkPlan.RemoveRootAgentsBlock,
			AutoLeftCurrentOrbit:  autoLeft,
			DetachedInstallRecord: false,
			AgentCleanup:          agentCleanup,
		}, nil
	}

	plansByOrbitID, err := loadRuntimeRemovePlans(ctx, repo.Root, runtimeFile)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	targetPlan, ok := plansByOrbitID[orbitID]
	if !ok {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("runtime member %q is missing hosted definition", orbitID)
	}

	deletePaths := sortedUniqueStrings(append(
		append([]string(nil), targetPlan.Plan.RulePaths...),
		targetPlan.Plan.ProcessPaths...,
	))
	if err := ensureRuntimeRemovePathsExclusive(orbitID, deletePaths, plansByOrbitID); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	agentsMutation, err := analyzeRuntimeRemoveAgents(repo.Root, orbitID)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	var installRecord orbittemplate.InstallRecord
	detachInstallRecord := targetMember.Source == MemberSourceInstallOrbit
	installRecordPath := ""
	if detachInstallRecord {
		installRecord, err = LoadInstallRecord(repo.Root, orbitID)
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("load install record for %q: %w", orbitID, err)
		}
		installRecordPath, err = InstallRecordRepoPath(orbitID)
		if err != nil {
			return RemoveRuntimeMemberResult{}, err
		}
	}

	touchedPaths := append([]string{ManifestRepoPath()}, deletePaths...)
	if agentsMutation.WillTouch {
		touchedPaths = append(touchedPaths, rootAgentsPath)
	}
	if installRecordPath != "" {
		touchedPaths = append(touchedPaths, installRecordPath)
	}
	cleanCheckPaths := append([]string(nil), deletePaths...)
	if agentsMutation.WillTouch {
		cleanCheckPaths = append(cleanCheckPaths, rootAgentsPath)
	}
	if err := ensureRuntimeRemovePathsClean(ctx, repo.Root, orbitID, cleanCheckPaths); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
	if err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	if len(hiddenPaths) > 0 && !currentIsTarget {
		return RemoveRuntimeMemberResult{}, fmt.Errorf(
			"cannot remove runtime member %q while the current orbit projection hides touched paths: %s; leave the current orbit first",
			orbitID,
			strings.Join(hiddenPaths, ", "),
		)
	}

	autoLeft := false
	if currentIsTarget {
		leaveResult, err := viewpkg.Leave(ctx, repo, store)
		if err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("auto leave current orbit %q: %w", orbitID, err)
		}
		autoLeft = leaveResult.Left || leaveResult.StateCleared || leaveResult.ProjectionRestored
	}

	removedPaths := append([]string(nil), deletePaths...)
	if agentsMutation.DeleteFile {
		removedPaths = append(removedPaths, rootAgentsPath)
	}
	sort.Strings(removedPaths)

	if err := removeRuntimeInfluencePaths(repo.Root, deletePaths); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}
	if err := applyRuntimeAgentsMutation(repo.Root, agentsMutation); err != nil {
		return RemoveRuntimeMemberResult{}, err
	}

	nextMembers := make([]RuntimeMember, 0, len(runtimeFile.Members)-1)
	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			continue
		}
		nextMembers = append(nextMembers, member)
	}
	runtimeFile.Members = nextMembers
	runtimeFile.Harness.UpdatedAt = resolveMutationTime(now)

	manifestPath, err := WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	detachedInstallRecord := false
	if detachInstallRecord {
		installRecord.Status = orbittemplate.InstallRecordStatusDetached
		if _, err := WriteInstallRecord(repo.Root, installRecord); err != nil {
			return RemoveRuntimeMemberResult{}, fmt.Errorf("write detached install record for %q: %w", orbitID, err)
		}
		detachedInstallRecord = true
	}

	agentCleanup, err := ReconcileAgentCleanupAfterPackageRemove(ctx, repo.Root, repo.GitDir, []string{orbitID}, AgentCleanupOptions{
		AllowGlobal: options.AllowGlobalAgentCleanup,
	})
	if err != nil {
		return RemoveRuntimeMemberResult{}, fmt.Errorf("reconcile agent cleanup for package remove: %w", err)
	}

	return RemoveRuntimeMemberResult{
		ManifestPath:          manifestPath,
		Runtime:               runtimeFile,
		RemovedPaths:          appendAgentCleanupRemovedPaths(removedPaths, agentCleanup),
		RemovedAgentsBlock:    agentsMutation.RemovedBlock,
		AutoLeftCurrentOrbit:  autoLeft,
		DetachedInstallRecord: detachedInstallRecord,
		AgentCleanup:          agentCleanup,
	}, nil
}

func loadRuntimeRemovePlans(
	ctx context.Context,
	repoRoot string,
	runtimeFile RuntimeFile,
) (map[string]runtimeRemovePlan, error) {
	globalConfig, hasLegacyGlobalConfig, err := loadRuntimeRemoveGlobalConfig(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load tracked files: %w", err)
	}

	specsByOrbitID := make(map[string]orbitpkg.OrbitSpec, len(runtimeFile.Members))
	definitions := make([]orbitpkg.Definition, 0, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, member.OrbitID)
		if err != nil {
			return nil, fmt.Errorf("load runtime member %q definition: %w", member.OrbitID, err)
		}
		definition, err := orbitpkg.CompatibilityDefinitionFromOrbitSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("load runtime member %q definition: %w", member.OrbitID, err)
		}

		specsByOrbitID[member.OrbitID] = spec
		definitions = append(definitions, definition)
	}

	config := orbitpkg.RepositoryConfig{
		Global:                globalConfig,
		Orbits:                definitions,
		HasLegacyGlobalConfig: hasLegacyGlobalConfig,
	}
	if err := orbitpkg.ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return nil, fmt.Errorf("validate runtime repository config: %w", err)
	}

	plansByOrbitID := make(map[string]runtimeRemovePlan, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		spec := specsByOrbitID[member.OrbitID]
		plan, err := orbitpkg.ResolveProjectionPlan(config, spec, trackedFiles)
		if err != nil {
			return nil, fmt.Errorf("resolve runtime member %q projection plan: %w", member.OrbitID, err)
		}
		plansByOrbitID[member.OrbitID] = runtimeRemovePlan{
			Member: member,
			Spec:   spec,
			Plan:   plan,
		}
	}

	return plansByOrbitID, nil
}

func loadRuntimeRemoveGlobalConfig(ctx context.Context, repoRoot string) (orbitpkg.GlobalConfig, bool, error) {
	globalConfig, err := orbitpkg.LoadGlobalConfig(ctx, repoRoot)
	if err == nil {
		return globalConfig, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return orbitpkg.DefaultGlobalConfig(), false, nil
	}

	return orbitpkg.GlobalConfig{}, false, fmt.Errorf("load global config: %w", err)
}

func ensureRuntimeRemovePathsExclusive(
	orbitID string,
	deletePaths []string,
	plansByOrbitID map[string]runtimeRemovePlan,
) error {
	for _, path := range deletePaths {
		for otherOrbitID, otherPlan := range plansByOrbitID {
			if otherOrbitID == orbitID {
				continue
			}
			if runtimeRemovePathShared(path, otherPlan.Plan.ProjectionPaths) {
				return fmt.Errorf(
					"cannot remove runtime member %q: delete candidate %q is still referenced by active member %q",
					orbitID,
					path,
					otherOrbitID,
				)
			}
		}
	}

	return nil
}

func runtimeRemovePathShared(path string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate == path {
			return true
		}
	}

	return false
}

func analyzeRuntimeRemoveAgents(repoRoot string, orbitID string) (runtimeAgentsMutation, error) {
	data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(context.Background(), repoRoot, rootAgentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runtimeAgentsMutation{}, nil
		}
		return runtimeAgentsMutation{}, fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	updated, removed, err := orbittemplate.RemoveRuntimeAgentsBlockData(data, orbitID)
	if err != nil {
		return runtimeAgentsMutation{}, fmt.Errorf("remove runtime AGENTS block: %w", err)
	}
	if !removed {
		return runtimeAgentsMutation{}, nil
	}
	if len(updated) == 0 {
		return runtimeAgentsMutation{
			WillTouch:    true,
			RemovedBlock: true,
			DeleteFile:   true,
		}, nil
	}

	return runtimeAgentsMutation{
		WillTouch:    true,
		RemovedBlock: true,
		UpdatedData:  updated,
	}, nil
}

func applyRuntimeAgentsMutation(repoRoot string, mutation runtimeAgentsMutation) error {
	if !mutation.WillTouch {
		return nil
	}

	filename := filepath.Join(repoRoot, rootAgentsPath)
	if mutation.DeleteFile {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete runtime AGENTS.md: %w", err)
		}
		return nil
	}

	if err := contractutil.AtomicWriteFileMode(filename, mutation.UpdatedData, 0o644); err != nil {
		return fmt.Errorf("write runtime AGENTS.md: %w", err)
	}

	return nil
}

func analyzeRuntimeUninstallRootGuidance(
	ctx context.Context,
	repoRoot string,
	orbitID string,
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
		updated, removed, err := orbittemplate.RemoveRuntimeGuidanceBlockData(data, orbitID, target.container)
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

func applyRuntimeUninstallRootGuidance(
	repoRoot string,
	mutations []runtimeRootGuidanceMutation,
) ([]string, error) {
	removedPaths := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		if !mutation.RemovedBlock {
			continue
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(mutation.Path))
		if mutation.DeleteFile {
			if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("delete %s: %w", mutation.Container, err)
			}
			removedPaths = append(removedPaths, mutation.Path)
			continue
		}
		if err := contractutil.AtomicWriteFileMode(filename, mutation.UpdatedData, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", mutation.Container, err)
		}
		removedPaths = append(removedPaths, mutation.Path)
	}

	return sortedUniqueStrings(removedPaths), nil
}

func plannedRuntimeUninstallRemovedPaths(
	ownedRuntimePaths []string,
	definitionRepoPath string,
	installRecordRepoPath string,
	guidanceMutations []runtimeRootGuidanceMutation,
) []string {
	removedPaths := append([]string(nil), ownedRuntimePaths...)
	removedPaths = append(removedPaths, definitionRepoPath, installRecordRepoPath)
	for _, mutation := range guidanceMutations {
		if mutation.RemovedBlock {
			removedPaths = append(removedPaths, mutation.Path)
		}
	}

	return sortedUniqueStrings(removedPaths)
}

func runtimeUninstallRemovesGuidance(mutations []runtimeRootGuidanceMutation) bool {
	for _, mutation := range mutations {
		if mutation.RemovedBlock {
			return true
		}
	}

	return false
}

func locallyChangedRuntimeUninstallPaths(
	ctx context.Context,
	repoRoot string,
	removedPaths []string,
) ([]RuntimeUninstallLocalChange, error) {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load package uninstall worktree status: %w", err)
	}

	statusByPath := make(map[string]gitpkg.StatusEntry, len(statusEntries))
	for _, entry := range statusEntries {
		statusByPath[entry.Path] = entry
	}

	risks := make([]RuntimeUninstallLocalChange, 0)
	for _, path := range sortedUniqueStrings(removedPaths) {
		entry, ok := statusByPath[path]
		if !ok {
			continue
		}
		risks = append(risks, RuntimeUninstallLocalChange{
			Path:      path,
			GitStatus: entry.Code,
			Tracked:   entry.Tracked,
			Staged:    entry.Staged,
		})
	}

	return risks, nil
}

func runtimeUninstallLocalChangesError(orbitID string, risks []RuntimeUninstallLocalChange) string {
	return fmt.Sprintf(
		"uninstall orbit package %q requires --yes to delete locally changed target-owned files: %s; use --dry-run to inspect",
		orbitID,
		strings.Join(formatRuntimeUninstallLocalChanges(risks), ", "),
	)
}

func formatRuntimeUninstallLocalChanges(risks []RuntimeUninstallLocalChange) []string {
	formatted := make([]string, 0, len(risks))
	for _, risk := range risks {
		formatted = append(formatted, fmt.Sprintf("%s (%s)", risk.Path, risk.GitStatus))
	}

	return formatted
}

func ensureRuntimeRemovePathsClean(ctx context.Context, repoRoot string, orbitID string, touchedPaths []string) error {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load runtime remove worktree status: %w", err)
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
		"cannot remove runtime member %q with uncommitted changes on touched paths: %s",
		orbitID,
		strings.Join(dirtyPaths, ", "),
	)
}

func hiddenRuntimeRemovePaths(ctx context.Context, repoRoot string, touchedPaths []string) ([]string, error) {
	hiddenPaths := make([]string, 0)
	for _, path := range sortedUniqueStrings(touchedPaths) {
		skipped, err := gitpkg.PathIsSkipWorktree(ctx, repoRoot, path)
		if err != nil {
			return nil, fmt.Errorf("check skip-worktree for runtime remove path %q: %w", path, err)
		}
		if skipped {
			hiddenPaths = append(hiddenPaths, path)
		}
	}

	return hiddenPaths, nil
}

func removeRuntimeInfluencePaths(repoRoot string, deletePaths []string) error {
	for _, repoPath := range deletePaths {
		filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove runtime file %s: %w", repoPath, err)
		}
	}

	return nil
}

func removeRepoPath(repoRoot string, repoPath string, label string) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s %s: %w", label, repoPath, err)
	}

	return nil
}

func currentOrbitMatches(store statepkg.FSStore, orbitID string) (bool, error) {
	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("read current orbit state: %w", err)
	}

	return current.Orbit == orbitID, nil
}

func findRuntimeMember(runtimeFile RuntimeFile, orbitID string) (RuntimeMember, bool) {
	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			return member, true
		}
	}

	return RuntimeMember{}, false
}
