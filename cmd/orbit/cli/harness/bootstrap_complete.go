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
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// BootstrapCompleteInput describes one explicit runtime bootstrap completion request.
type BootstrapCompleteInput struct {
	OrbitID                     string
	All                         bool
	Now                         time.Time
	AllowDirtyBootstrapArtifact bool
}

// BootstrapCompleteResult reports one successful runtime bootstrap completion pass.
type BootstrapCompleteResult struct {
	CompletedOrbits        []string `json:"completed_orbits,omitempty"`
	AlreadyCompletedOrbits []string `json:"already_completed_orbits,omitempty"`
	RemovedPaths           []string `json:"removed_paths,omitempty"`
	RemovedBootstrapBlocks []string `json:"removed_bootstrap_blocks,omitempty"`
	DeletedBootstrapFile   bool     `json:"deleted_bootstrap_file"`
	AutoLeftCurrentOrbit   bool     `json:"auto_left_current_orbit"`
}

type runtimeBootstrapCompletionPlan struct {
	OrbitID     string
	DeletePaths []string
}

type runtimeBootstrapMutation struct {
	WillTouch     bool
	RemovedBlocks []string
	DeleteFile    bool
	UpdatedData   []byte
}

// PlanRuntimeBootstrapCompletion previews runtime bootstrap completion without
// mutating runtime files or repo-local state.
func PlanRuntimeBootstrapCompletion(
	ctx context.Context,
	repo gitpkg.Repo,
	input BootstrapCompleteInput,
) (BootstrapCompleteResult, error) {
	if input.All && strings.TrimSpace(input.OrbitID) != "" {
		return BootstrapCompleteResult{}, fmt.Errorf("--orbit and --all cannot be used together")
	}
	if !input.All && strings.TrimSpace(input.OrbitID) == "" {
		return BootstrapCompleteResult{}, fmt.Errorf("either --orbit or --all must be provided")
	}

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return BootstrapCompleteResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return BootstrapCompleteResult{}, fmt.Errorf("create state store: %w", err)
	}

	statuses, err := resolveBootstrapCompletionStatuses(ctx, repo.Root, repo.GitDir, runtimeFile, input)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	if len(statuses) == 0 {
		return BootstrapCompleteResult{}, fmt.Errorf("current runtime does not contain bootstrap-enabled orbits")
	}

	plansByOrbitID, err := loadRuntimeRemovePlans(ctx, repo.Root, runtimeFile)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	pendingPlans, alreadyCompleted, err := buildRuntimeBootstrapCompletionPlans(ctx, repo.Root, statuses, plansByOrbitID)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}

	mutation, err := analyzeRuntimeBootstrapMutation(repo.Root, orbitIDsFromCompletionPlans(pendingPlans))
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	touchedPaths := touchedBootstrapCompletionPaths(pendingPlans, mutation)
	if err := ensureBootstrapCompletionPathsClean(ctx, repo.Root, statusesToOrbitIDs(statuses), bootstrapCompletionCleanCheckPaths(touchedPaths, input)); err != nil {
		return BootstrapCompleteResult{}, err
	}

	currentOrbitID, currentExists, err := currentRuntimeOrbitID(store)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	autoLeft := currentExists && containsString(orbitIDsFromCompletionPlans(pendingPlans), currentOrbitID)

	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	if len(hiddenPaths) > 0 {
		return BootstrapCompleteResult{}, fmt.Errorf(
			"cannot complete bootstrap for orbit(s) %s while the current orbit projection hides touched paths: %s; leave the current orbit first",
			strings.Join(statusesToOrbitIDs(statuses), ", "),
			strings.Join(hiddenPaths, ", "),
		)
	}

	deletePaths := bootstrapDeletePathsFromPlans(pendingPlans)
	removedPaths := sortedUniqueStrings(append(append([]string(nil), deletePaths...), touchedBootstrapArtifactPaths(mutation)...))

	return BootstrapCompleteResult{
		CompletedOrbits:        orbitIDsFromCompletionPlans(pendingPlans),
		AlreadyCompletedOrbits: alreadyCompleted,
		RemovedPaths:           removedPaths,
		RemovedBootstrapBlocks: append([]string(nil), mutation.RemovedBlocks...),
		DeletedBootstrapFile:   mutation.DeleteFile,
		AutoLeftCurrentOrbit:   autoLeft,
	}, nil
}

// CompleteRuntimeBootstrap marks one or more runtime orbit bootstraps as completed and
// removes their runtime-only bootstrap surface.
func CompleteRuntimeBootstrap(
	ctx context.Context,
	repo gitpkg.Repo,
	input BootstrapCompleteInput,
) (BootstrapCompleteResult, error) {
	if input.All && strings.TrimSpace(input.OrbitID) != "" {
		return BootstrapCompleteResult{}, fmt.Errorf("--orbit and --all cannot be used together")
	}
	if !input.All && strings.TrimSpace(input.OrbitID) == "" {
		return BootstrapCompleteResult{}, fmt.Errorf("either --orbit or --all must be provided")
	}

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return BootstrapCompleteResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return BootstrapCompleteResult{}, fmt.Errorf("create state store: %w", err)
	}

	statuses, err := resolveBootstrapCompletionStatuses(ctx, repo.Root, repo.GitDir, runtimeFile, input)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	if len(statuses) == 0 {
		return BootstrapCompleteResult{}, fmt.Errorf("current runtime does not contain bootstrap-enabled orbits")
	}

	plansByOrbitID, err := loadRuntimeRemovePlans(ctx, repo.Root, runtimeFile)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	pendingPlans, alreadyCompleted, err := buildRuntimeBootstrapCompletionPlans(ctx, repo.Root, statuses, plansByOrbitID)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}

	mutation, err := analyzeRuntimeBootstrapMutation(repo.Root, orbitIDsFromCompletionPlans(pendingPlans))
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	touchedPaths := touchedBootstrapCompletionPaths(pendingPlans, mutation)
	if err := ensureBootstrapCompletionPathsClean(ctx, repo.Root, statusesToOrbitIDs(statuses), bootstrapCompletionCleanCheckPaths(touchedPaths, input)); err != nil {
		return BootstrapCompleteResult{}, err
	}

	currentOrbitID, currentExists, err := currentRuntimeOrbitID(store)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	autoLeft := false
	if currentExists && containsString(orbitIDsFromCompletionPlans(pendingPlans), currentOrbitID) {
		leaveResult, err := viewpkg.Leave(ctx, repo, store)
		if err != nil {
			return BootstrapCompleteResult{}, fmt.Errorf("auto leave current orbit %q: %w", currentOrbitID, err)
		}
		autoLeft = leaveResult.Left || leaveResult.StateCleared || leaveResult.ProjectionRestored
	}

	hiddenPaths, err := hiddenRuntimeRemovePaths(ctx, repo.Root, touchedPaths)
	if err != nil {
		return BootstrapCompleteResult{}, err
	}
	if len(hiddenPaths) > 0 {
		return BootstrapCompleteResult{}, fmt.Errorf(
			"cannot complete bootstrap for orbit(s) %s while the current orbit projection hides touched paths: %s; leave the current orbit first",
			strings.Join(statusesToOrbitIDs(statuses), ", "),
			strings.Join(hiddenPaths, ", "),
		)
	}

	deletePaths := bootstrapDeletePathsFromPlans(pendingPlans)
	if err := removeRuntimeInfluencePaths(repo.Root, deletePaths); err != nil {
		return BootstrapCompleteResult{}, err
	}
	if err := applyRuntimeBootstrapMutation(repo.Root, mutation); err != nil {
		return BootstrapCompleteResult{}, err
	}

	for _, plan := range pendingPlans {
		if err := writeBootstrapCompletedState(store, plan.OrbitID, resolveMutationTime(input.Now)); err != nil {
			return BootstrapCompleteResult{}, err
		}
	}

	removedPaths := sortedUniqueStrings(append(append([]string(nil), deletePaths...), touchedBootstrapArtifactPaths(mutation)...))

	return BootstrapCompleteResult{
		CompletedOrbits:        orbitIDsFromCompletionPlans(pendingPlans),
		AlreadyCompletedOrbits: alreadyCompleted,
		RemovedPaths:           removedPaths,
		RemovedBootstrapBlocks: append([]string(nil), mutation.RemovedBlocks...),
		DeletedBootstrapFile:   mutation.DeleteFile,
		AutoLeftCurrentOrbit:   autoLeft,
	}, nil
}

func resolveBootstrapCompletionStatuses(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	runtimeFile RuntimeFile,
	input BootstrapCompleteInput,
) ([]orbittemplate.BootstrapOrbitStatus, error) {
	runtimeOrbitIDs := make([]string, 0, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		runtimeOrbitIDs = append(runtimeOrbitIDs, member.OrbitID)
	}

	if input.All {
		statuses, err := orbittemplate.ListBootstrapEnabledOrbits(ctx, repoRoot, gitDir, runtimeOrbitIDs)
		if err != nil {
			return nil, fmt.Errorf("list bootstrap-enabled orbits: %w", err)
		}

		return statuses, nil
	}

	if _, found := findRuntimeMember(runtimeFile, input.OrbitID); !found {
		return nil, fmt.Errorf("runtime member %q not found", input.OrbitID)
	}

	status, err := orbittemplate.InspectBootstrapOrbit(ctx, repoRoot, gitDir, input.OrbitID)
	if err != nil {
		return nil, fmt.Errorf("inspect bootstrap orbit %q: %w", input.OrbitID, err)
	}
	if !status.Enabled {
		return nil, fmt.Errorf("runtime member %q is not bootstrap-enabled", input.OrbitID)
	}

	return []orbittemplate.BootstrapOrbitStatus{status}, nil
}

func buildRuntimeBootstrapCompletionPlans(
	ctx context.Context,
	repoRoot string,
	statuses []orbittemplate.BootstrapOrbitStatus,
	plansByOrbitID map[string]runtimeRemovePlan,
) ([]runtimeBootstrapCompletionPlan, []string, error) {
	repoConfig, err := orbitpkg.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load hosted repository config: %w", err)
	}
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load tracked files: %w", err)
	}
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load worktree status: %w", err)
	}
	candidatePaths, trackedSet := bootstrapCandidatePaths(trackedFiles, statusEntries)

	targetSet := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		targetSet[status.OrbitID] = struct{}{}
	}

	pendingPlans := make([]runtimeBootstrapCompletionPlan, 0, len(statuses))
	alreadyCompleted := make([]string, 0)
	for _, status := range statuses {
		completionPlan := orbittemplate.PlanBootstrapCompletion(status)
		switch completionPlan.Action {
		case orbittemplate.BootstrapActionWarningNoOp:
			alreadyCompleted = append(alreadyCompleted, status.OrbitID)
			continue
		case orbittemplate.BootstrapActionReject:
			return nil, nil, fmt.Errorf("runtime member %q is not bootstrap-enabled", status.OrbitID)
		}

		runtimePlan, ok := plansByOrbitID[status.OrbitID]
		if !ok {
			return nil, nil, fmt.Errorf("runtime member %q is missing hosted definition", status.OrbitID)
		}

		deletePaths, err := resolveBootstrapDeletePaths(runtimePlan.Spec, candidatePaths)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve bootstrap delete paths for %q: %w", status.OrbitID, err)
		}
		if err := ensureBootstrapRemovePathsExclusive(targetSet, status.OrbitID, deletePaths, repoConfig, plansByOrbitID, trackedSet); err != nil {
			return nil, nil, err
		}

		pendingPlans = append(pendingPlans, runtimeBootstrapCompletionPlan{
			OrbitID:     status.OrbitID,
			DeletePaths: deletePaths,
		})
	}

	sort.Strings(alreadyCompleted)

	return pendingPlans, alreadyCompleted, nil
}

func resolveBootstrapDeletePaths(
	spec orbitpkg.OrbitSpec,
	candidatePaths []string,
) ([]string, error) {
	bootstrapMembers := make([]orbitpkg.OrbitMember, 0)
	for _, member := range spec.Members {
		if member.Lane == orbitpkg.OrbitMemberLaneBootstrap {
			bootstrapMembers = append(bootstrapMembers, member)
		}
	}
	if len(bootstrapMembers) == 0 {
		return nil, nil
	}

	deletePaths := make([]string, 0, len(candidatePaths))
	for _, candidatePath := range candidatePaths {
		for _, member := range bootstrapMembers {
			matches, err := orbitpkg.MemberMatchesPath(member, candidatePath)
			if err != nil {
				return nil, fmt.Errorf("match bootstrap member path %q: %w", candidatePath, err)
			}
			if !matches {
				continue
			}
			deletePaths = append(deletePaths, candidatePath)
			break
		}
	}

	return sortedUniqueStrings(deletePaths), nil
}

func ensureBootstrapRemovePathsExclusive(
	targetOrbitIDs map[string]struct{},
	orbitID string,
	deletePaths []string,
	config orbitpkg.RepositoryConfig,
	plansByOrbitID map[string]runtimeRemovePlan,
	trackedSet map[string]struct{},
) error {
	for _, path := range deletePaths {
		for otherOrbitID, otherPlan := range plansByOrbitID {
			if otherOrbitID == orbitID {
				continue
			}
			if _, targeted := targetOrbitIDs[otherOrbitID]; targeted {
				continue
			}
			_, tracked := trackedSet[path]
			classification, err := orbitpkg.ClassifyOrbitPath(config, otherPlan.Spec, otherPlan.Plan, path, tracked)
			if err != nil {
				return fmt.Errorf("classify bootstrap delete candidate %q for active member %q: %w", path, otherOrbitID, err)
			}
			if classification.Projection {
				return fmt.Errorf(
					"cannot complete bootstrap for orbit %q: delete candidate %q is still referenced by active member %q",
					orbitID,
					path,
					otherOrbitID,
				)
			}
		}
	}

	return nil
}

func bootstrapCandidatePaths(
	trackedFiles []string,
	statusEntries []gitpkg.StatusEntry,
) ([]string, map[string]struct{}) {
	candidateSet := make(map[string]struct{}, len(trackedFiles)+len(statusEntries))
	trackedSet := make(map[string]struct{}, len(trackedFiles))

	for _, path := range trackedFiles {
		candidateSet[path] = struct{}{}
		trackedSet[path] = struct{}{}
	}
	for _, entry := range statusEntries {
		candidateSet[entry.Path] = struct{}{}
		if entry.Tracked {
			trackedSet[entry.Path] = struct{}{}
		}
	}

	candidatePaths := make([]string, 0, len(candidateSet))
	for path := range candidateSet {
		candidatePaths = append(candidatePaths, path)
	}
	sort.Strings(candidatePaths)

	return candidatePaths, trackedSet
}

func analyzeRuntimeBootstrapMutation(repoRoot string, orbitIDs []string) (runtimeBootstrapMutation, error) {
	if len(orbitIDs) == 0 {
		return runtimeBootstrapMutation{}, nil
	}

	data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(context.Background(), repoRoot, rootBootstrapPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runtimeBootstrapMutation{}, nil
		}
		return runtimeBootstrapMutation{}, fmt.Errorf("read runtime BOOTSTRAP.md: %w", err)
	}

	updated := append([]byte(nil), data...)
	removedBlocks := make([]string, 0, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		next, removed, err := orbittemplate.RemoveRuntimeGuidanceBlockData(updated, orbitID, "root BOOTSTRAP.md")
		if err != nil {
			return runtimeBootstrapMutation{}, fmt.Errorf("remove runtime BOOTSTRAP block: %w", err)
		}
		if removed {
			removedBlocks = append(removedBlocks, orbitID)
			updated = next
		}
	}
	if len(removedBlocks) == 0 {
		return runtimeBootstrapMutation{}, nil
	}
	if len(bytes.TrimSpace(updated)) == 0 {
		return runtimeBootstrapMutation{
			WillTouch:     true,
			RemovedBlocks: removedBlocks,
			DeleteFile:    true,
		}, nil
	}

	return runtimeBootstrapMutation{
		WillTouch:     true,
		RemovedBlocks: removedBlocks,
		UpdatedData:   updated,
	}, nil
}

func applyRuntimeBootstrapMutation(repoRoot string, mutation runtimeBootstrapMutation) error {
	if !mutation.WillTouch {
		return nil
	}

	filename := filepath.Join(repoRoot, rootBootstrapPath)
	if mutation.DeleteFile {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete runtime BOOTSTRAP.md: %w", err)
		}
		return nil
	}

	if err := contractutil.AtomicWriteFileMode(filename, mutation.UpdatedData, 0o644); err != nil {
		return fmt.Errorf("write runtime BOOTSTRAP.md: %w", err)
	}

	return nil
}

func ensureBootstrapCompletionPathsClean(ctx context.Context, repoRoot string, orbitIDs []string, touchedPaths []string) error {
	if len(touchedPaths) == 0 {
		return nil
	}

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load bootstrap completion worktree status: %w", err)
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
		"cannot complete bootstrap for orbit(s) %s with uncommitted changes on touched paths: %s",
		strings.Join(sortedUniqueStrings(orbitIDs), ", "),
		strings.Join(dirtyPaths, ", "),
	)
}

func writeBootstrapCompletedState(store statepkg.FSStore, orbitID string, now time.Time) error {
	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		if !errors.Is(err, statepkg.ErrRuntimeStateSnapshotNotFound) {
			return fmt.Errorf("read runtime state snapshot: %w", err)
		}
		snapshot = statepkg.RuntimeStateSnapshot{Orbit: orbitID}
	}
	snapshot.Orbit = orbitID
	snapshot.UpdatedAt = now
	if snapshot.Bootstrap == nil {
		snapshot.Bootstrap = &statepkg.RuntimeBootstrapState{}
	}
	snapshot.Bootstrap.Completed = true
	snapshot.Bootstrap.CompletedAt = now
	if err := store.WriteRuntimeStateSnapshot(snapshot); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	return nil
}

func currentRuntimeOrbitID(store statepkg.FSStore) (string, bool, error) {
	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read current orbit state: %w", err)
	}

	return current.Orbit, true, nil
}

func bootstrapDeletePathsFromPlans(plans []runtimeBootstrapCompletionPlan) []string {
	deletePaths := make([]string, 0)
	for _, plan := range plans {
		deletePaths = append(deletePaths, plan.DeletePaths...)
	}

	return sortedUniqueStrings(deletePaths)
}

func touchedBootstrapArtifactPaths(mutation runtimeBootstrapMutation) []string {
	if !mutation.WillTouch {
		return nil
	}

	return []string{rootBootstrapPath}
}

func touchedBootstrapCompletionPaths(plans []runtimeBootstrapCompletionPlan, mutation runtimeBootstrapMutation) []string {
	touchedPaths := append([]string(nil), bootstrapDeletePathsFromPlans(plans)...)
	touchedPaths = append(touchedPaths, touchedBootstrapArtifactPaths(mutation)...)

	return sortedUniqueStrings(touchedPaths)
}

func bootstrapCompletionCleanCheckPaths(paths []string, input BootstrapCompleteInput) []string {
	if !input.AllowDirtyBootstrapArtifact {
		return paths
	}
	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == rootBootstrapPath {
			continue
		}
		filtered = append(filtered, path)
	}

	return filtered
}

func orbitIDsFromCompletionPlans(plans []runtimeBootstrapCompletionPlan) []string {
	orbitIDs := make([]string, 0, len(plans))
	for _, plan := range plans {
		orbitIDs = append(orbitIDs, plan.OrbitID)
	}

	return sortedUniqueStrings(orbitIDs)
}

func statusesToOrbitIDs(statuses []orbittemplate.BootstrapOrbitStatus) []string {
	orbitIDs := make([]string, 0, len(statuses))
	for _, status := range statuses {
		orbitIDs = append(orbitIDs, status.OrbitID)
	}

	return sortedUniqueStrings(orbitIDs)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}
