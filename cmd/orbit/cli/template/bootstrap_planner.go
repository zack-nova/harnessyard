package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// BootstrapCompletionState is the stable lifecycle state for one orbit's bootstrap lane in the current runtime.
type BootstrapCompletionState string

const (
	BootstrapCompletionStateNotApplicable BootstrapCompletionState = "not_applicable"
	BootstrapCompletionStatePending       BootstrapCompletionState = "pending"
	BootstrapCompletionStateCompleted     BootstrapCompletionState = "completed"
)

// BootstrapAction is the shared cross-command gating decision for one bootstrap-sensitive operation.
type BootstrapAction string

const (
	BootstrapActionAllow       BootstrapAction = "allow"
	BootstrapActionReject      BootstrapAction = "reject"
	BootstrapActionSkip        BootstrapAction = "skip"
	BootstrapActionWarningNoOp BootstrapAction = "warning_no_op"
)

// BootstrapOrbitStatus captures the authored/runtime bootstrap state needed by shared planner helpers.
type BootstrapOrbitStatus struct {
	OrbitID              string
	Enabled              bool
	HasBootstrapTemplate bool
	HasBootstrapMembers  bool
	CompletionState      BootstrapCompletionState
	CompletedAt          time.Time
}

// BootstrapPlan is one stable shared gating decision.
type BootstrapPlan struct {
	OrbitID         string
	CompletionState BootstrapCompletionState
	Action          BootstrapAction
	ReasonCode      string
}

// ListBootstrapEnabledOrbits returns all bootstrap-enabled orbit statuses in stable order.
func ListBootstrapEnabledOrbits(ctx context.Context, repoRoot string, gitDir string, orbitIDs []string) ([]BootstrapOrbitStatus, error) {
	resolvedOrbitIDs, err := resolveBootstrapOrbitIDs(ctx, repoRoot, orbitIDs)
	if err != nil {
		return nil, err
	}

	statuses := make([]BootstrapOrbitStatus, 0, len(resolvedOrbitIDs))
	for _, orbitID := range resolvedOrbitIDs {
		status, err := InspectBootstrapOrbit(ctx, repoRoot, gitDir, orbitID)
		if err != nil {
			return nil, err
		}
		if status.Enabled {
			statuses = append(statuses, status)
		}
	}

	return statuses, nil
}

// InspectBootstrapOrbit resolves one orbit's bootstrap-enabled signals and completion state.
func InspectBootstrapOrbit(ctx context.Context, repoRoot string, gitDir string, orbitID string) (BootstrapOrbitStatus, error) {
	if repoRoot == "" {
		return BootstrapOrbitStatus{}, fmt.Errorf("repo root must not be empty")
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		return BootstrapOrbitStatus{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	spec, err = orbitpkg.EnsureHostedMemberSchema(spec)
	if err != nil {
		return BootstrapOrbitStatus{}, fmt.Errorf("upgrade hosted orbit spec: %w", err)
	}

	status := BootstrapOrbitStatus{
		OrbitID:              orbitID,
		HasBootstrapTemplate: hasBootstrapTemplate(spec),
		HasBootstrapMembers:  hasBootstrapMembers(spec),
	}
	status.Enabled = status.HasBootstrapTemplate || status.HasBootstrapMembers
	if !status.Enabled {
		status.CompletionState = BootstrapCompletionStateNotApplicable
		return status, nil
	}

	completionState, completedAt, err := readBootstrapCompletionState(ctx, repoRoot, gitDir, orbitID)
	if err != nil {
		return BootstrapOrbitStatus{}, err
	}
	status.CompletionState = completionState
	status.CompletedAt = completedAt

	return status, nil
}

// InspectBootstrapOrbitForRevision resolves one orbit's bootstrap status while honoring
// the rule that runtime completion state only gates runtime revisions.
func InspectBootstrapOrbitForRevision(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	orbitID string,
	revisionKind string,
) (BootstrapOrbitStatus, error) {
	status, err := InspectBootstrapOrbit(ctx, repoRoot, gitDir, orbitID)
	if err != nil {
		return BootstrapOrbitStatus{}, err
	}
	if strings.TrimSpace(revisionKind) != "runtime" && status.CompletionState == BootstrapCompletionStateCompleted {
		status.CompletionState = BootstrapCompletionStatePending
		status.CompletedAt = time.Time{}
	}

	return status, nil
}

// PlanBootstrapGuidanceMaterialize returns the shared decision for one bootstrap materialize action.
func PlanBootstrapGuidanceMaterialize(status BootstrapOrbitStatus) BootstrapPlan {
	switch {
	case status.CompletionState == BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_completed")
	case !status.HasBootstrapTemplate:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_no_template")
	case status.CompletionState == BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_not_applicable")
	}
}

// PlanBootstrapGuidanceBackfill returns the shared decision for one bootstrap backfill action.
func PlanBootstrapGuidanceBackfill(status BootstrapOrbitStatus, hasOrbitBlock bool) BootstrapPlan {
	switch {
	case status.CompletionState == BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_completed")
	case hasOrbitBlock:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_missing_orbit_block")
	}
}

// PlanBootstrapCompose returns the shared decision for compose-time BOOTSTRAP materialization.
func PlanBootstrapCompose(status BootstrapOrbitStatus) BootstrapPlan {
	switch {
	case status.CompletionState == BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_completed")
	case !status.HasBootstrapTemplate:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_no_template")
	case status.CompletionState == BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_not_applicable")
	}
}

// PlanBootstrapFrameworkApply returns the shared decision for framework-apply BOOTSTRAP artifact materialization.
func PlanBootstrapFrameworkApply(status BootstrapOrbitStatus) BootstrapPlan {
	switch {
	case status.CompletionState == BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_completed")
	case !status.HasBootstrapTemplate:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_no_template")
	case status.CompletionState == BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_not_applicable")
	}
}

// PlanBootstrapRuntimeExport returns the shared decision for runtime save/export handling.
func PlanBootstrapRuntimeExport(status BootstrapOrbitStatus, includeCompleted bool) BootstrapPlan {
	switch {
	case status.CompletionState == BootstrapCompletionStateCompleted && status.HasBootstrapMembers && !includeCompleted:
		return newBootstrapPlan(status, BootstrapActionSkip, "bootstrap_completed")
	default:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending_or_irrelevant")
	}
}

// PlanBootstrapCompletion returns the shared decision for one completion command target.
func PlanBootstrapCompletion(status BootstrapOrbitStatus) BootstrapPlan {
	switch status.CompletionState {
	case BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionWarningNoOp, "bootstrap_already_completed")
	case BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_not_applicable")
	}
}

// PlanBootstrapReopen returns the shared decision for one bootstrap reopen command target.
func PlanBootstrapReopen(status BootstrapOrbitStatus) BootstrapPlan {
	switch status.CompletionState {
	case BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_completed")
	case BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionWarningNoOp, "bootstrap_already_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_not_applicable")
	}
}

// PlanBootstrapSurfaceRestore returns the shared decision for one explicit bootstrap surface restore.
func PlanBootstrapSurfaceRestore(status BootstrapOrbitStatus) BootstrapPlan {
	switch status.CompletionState {
	case BootstrapCompletionStateCompleted:
		return newBootstrapPlan(status, BootstrapActionAllow, "bootstrap_completed")
	case BootstrapCompletionStatePending:
		return newBootstrapPlan(status, BootstrapActionWarningNoOp, "bootstrap_already_pending")
	default:
		return newBootstrapPlan(status, BootstrapActionReject, "bootstrap_not_applicable")
	}
}

func newBootstrapPlan(status BootstrapOrbitStatus, action BootstrapAction, reasonCode string) BootstrapPlan {
	return BootstrapPlan{
		OrbitID:         status.OrbitID,
		CompletionState: status.CompletionState,
		Action:          action,
		ReasonCode:      reasonCode,
	}
}

func hasBootstrapTemplate(spec orbitpkg.OrbitSpec) bool {
	return spec.Meta != nil && strings.TrimSpace(spec.Meta.BootstrapTemplate) != ""
}

func hasBootstrapMembers(spec orbitpkg.OrbitSpec) bool {
	for _, member := range spec.Members {
		if member.Lane == orbitpkg.OrbitMemberLaneBootstrap {
			return true
		}
	}

	return false
}

func readBootstrapCompletionState(ctx context.Context, repoRoot string, gitDir string, orbitID string) (BootstrapCompletionState, time.Time, error) {
	resolvedGitDir, err := resolveBootstrapGitDir(ctx, repoRoot, gitDir)
	if err != nil {
		return "", time.Time{}, err
	}

	store, err := statepkg.NewFSStore(resolvedGitDir)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build runtime state store: %w", err)
	}

	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		if errors.Is(err, statepkg.ErrRuntimeStateSnapshotNotFound) {
			return BootstrapCompletionStatePending, time.Time{}, nil
		}
		return "", time.Time{}, fmt.Errorf("read runtime state snapshot: %w", err)
	}
	if snapshot.Bootstrap == nil || !snapshot.Bootstrap.Completed {
		return BootstrapCompletionStatePending, time.Time{}, nil
	}

	return BootstrapCompletionStateCompleted, snapshot.Bootstrap.CompletedAt, nil
}

func resolveBootstrapGitDir(ctx context.Context, repoRoot string, gitDir string) (string, error) {
	if strings.TrimSpace(gitDir) != "" {
		return gitDir, nil
	}

	repo, err := gitpkg.DiscoverRepo(ctx, repoRoot)
	if err != nil {
		return "", fmt.Errorf("discover repository git dir: %w", err)
	}

	return repo.GitDir, nil
}

func resolveBootstrapOrbitIDs(ctx context.Context, repoRoot string, orbitIDs []string) ([]string, error) {
	if len(orbitIDs) > 0 {
		return append([]string(nil), orbitIDs...), nil
	}

	config, err := orbitpkg.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load hosted repository config: %w", err)
	}

	resolved := make([]string, 0, len(config.Orbits))
	for _, definition := range config.Orbits {
		resolved = append(resolved, definition.ID)
	}

	return resolved, nil
}
