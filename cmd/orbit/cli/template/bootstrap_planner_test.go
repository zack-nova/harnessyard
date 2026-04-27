package orbittemplate

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestListBootstrapEnabledOrbitsAndInspectCompletionState(t *testing.T) {
	t.Parallel()

	repo := seedBootstrapPlannerRepo(t)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "completed",
		UpdatedAt: time.Date(2026, time.April, 19, 8, 0, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 7, 45, 0, 0, time.UTC),
		},
	}))

	statuses, err := ListBootstrapEnabledOrbits(context.Background(), repo.Root, repo.GitDir(t), nil)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	require.Equal(t, "completed", statuses[0].OrbitID)
	require.Equal(t, BootstrapCompletionStateCompleted, statuses[0].CompletionState)
	require.Equal(t, "pending", statuses[1].OrbitID)
	require.Equal(t, BootstrapCompletionStatePending, statuses[1].CompletionState)

	notApplicable, err := InspectBootstrapOrbit(context.Background(), repo.Root, repo.GitDir(t), "plain")
	require.NoError(t, err)
	require.False(t, notApplicable.Enabled)
	require.Equal(t, BootstrapCompletionStateNotApplicable, notApplicable.CompletionState)

	completed, err := InspectBootstrapOrbit(context.Background(), repo.Root, repo.GitDir(t), "completed")
	require.NoError(t, err)
	require.True(t, completed.Enabled)
	require.True(t, completed.HasBootstrapTemplate)
	require.Equal(t, BootstrapCompletionStateCompleted, completed.CompletionState)

	pending, err := InspectBootstrapOrbit(context.Background(), repo.Root, repo.GitDir(t), "pending")
	require.NoError(t, err)
	require.True(t, pending.Enabled)
	require.True(t, pending.HasBootstrapMembers)
	require.Equal(t, BootstrapCompletionStatePending, pending.CompletionState)
}

func TestPlanBootstrapActionsUseStableDecisionMatrix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		status               BootstrapOrbitStatus
		hasOrbitBlock        bool
		materializeAction    BootstrapAction
		backfillAction       BootstrapAction
		composeAction        BootstrapAction
		frameworkApplyAction BootstrapAction
		runtimeExportAction  BootstrapAction
		runtimeExportInclude BootstrapAction
		completionAction     BootstrapAction
		reopenAction         BootstrapAction
		restoreSurfaceAction BootstrapAction
	}{
		{
			name: "not applicable",
			status: BootstrapOrbitStatus{
				OrbitID:         "plain",
				CompletionState: BootstrapCompletionStateNotApplicable,
			},
			materializeAction:    BootstrapActionReject,
			backfillAction:       BootstrapActionReject,
			composeAction:        BootstrapActionSkip,
			frameworkApplyAction: BootstrapActionSkip,
			runtimeExportAction:  BootstrapActionAllow,
			runtimeExportInclude: BootstrapActionAllow,
			completionAction:     BootstrapActionReject,
			reopenAction:         BootstrapActionReject,
			restoreSurfaceAction: BootstrapActionReject,
		},
		{
			name: "pending",
			status: BootstrapOrbitStatus{
				OrbitID:              "pending",
				Enabled:              true,
				HasBootstrapTemplate: true,
				CompletionState:      BootstrapCompletionStatePending,
			},
			hasOrbitBlock:        true,
			materializeAction:    BootstrapActionAllow,
			backfillAction:       BootstrapActionAllow,
			composeAction:        BootstrapActionAllow,
			frameworkApplyAction: BootstrapActionAllow,
			runtimeExportAction:  BootstrapActionAllow,
			runtimeExportInclude: BootstrapActionAllow,
			completionAction:     BootstrapActionAllow,
			reopenAction:         BootstrapActionWarningNoOp,
			restoreSurfaceAction: BootstrapActionWarningNoOp,
		},
		{
			name: "completed",
			status: BootstrapOrbitStatus{
				OrbitID:              "completed",
				Enabled:              true,
				HasBootstrapTemplate: true,
				HasBootstrapMembers:  true,
				CompletionState:      BootstrapCompletionStateCompleted,
			},
			materializeAction:    BootstrapActionReject,
			backfillAction:       BootstrapActionReject,
			composeAction:        BootstrapActionSkip,
			frameworkApplyAction: BootstrapActionSkip,
			runtimeExportAction:  BootstrapActionSkip,
			runtimeExportInclude: BootstrapActionAllow,
			completionAction:     BootstrapActionWarningNoOp,
			reopenAction:         BootstrapActionAllow,
			restoreSurfaceAction: BootstrapActionAllow,
		},
		{
			name: "members only pending",
			status: BootstrapOrbitStatus{
				OrbitID:             "member-only",
				Enabled:             true,
				HasBootstrapMembers: true,
				CompletionState:     BootstrapCompletionStatePending,
			},
			materializeAction:    BootstrapActionReject,
			backfillAction:       BootstrapActionReject,
			composeAction:        BootstrapActionSkip,
			frameworkApplyAction: BootstrapActionSkip,
			runtimeExportAction:  BootstrapActionAllow,
			runtimeExportInclude: BootstrapActionAllow,
			completionAction:     BootstrapActionAllow,
			reopenAction:         BootstrapActionWarningNoOp,
			restoreSurfaceAction: BootstrapActionWarningNoOp,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, testCase.materializeAction, PlanBootstrapGuidanceMaterialize(testCase.status).Action)
			require.Equal(t, testCase.backfillAction, PlanBootstrapGuidanceBackfill(testCase.status, testCase.hasOrbitBlock).Action)
			require.Equal(t, testCase.composeAction, PlanBootstrapCompose(testCase.status).Action)
			require.Equal(t, testCase.frameworkApplyAction, PlanBootstrapFrameworkApply(testCase.status).Action)
			require.Equal(t, testCase.runtimeExportAction, PlanBootstrapRuntimeExport(testCase.status, false).Action)
			require.Equal(t, testCase.runtimeExportInclude, PlanBootstrapRuntimeExport(testCase.status, true).Action)
			require.Equal(t, testCase.completionAction, PlanBootstrapCompletion(testCase.status).Action)
			require.Equal(t, testCase.reopenAction, PlanBootstrapReopen(testCase.status).Action)
			require.Equal(t, testCase.restoreSurfaceAction, PlanBootstrapSurfaceRestore(testCase.status).Action)
		})
	}
}

func TestInspectOrbitBootstrapLaneForOperationRejectsCompletedBootstrap(t *testing.T) {
	t.Parallel()

	repo := seedBootstrapPlannerRepo(t)
	repo.WriteFile(t, "BOOTSTRAP.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin orbit_id=\"completed\" -->\n"+
		"Bootstrap the completed orbit.\n"+
		"<!-- orbit:end orbit_id=\"completed\" -->\n")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "completed",
		UpdatedAt: time.Date(2026, time.April, 19, 8, 30, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 8, 15, 0, 0, time.UTC),
		},
	}))

	materializeStatus, err := InspectOrbitBootstrapLaneForOperation(context.Background(), repo.Root, "completed", "materialize")
	require.NoError(t, err)
	require.Equal(t, BootstrapCompletionStateCompleted, materializeStatus.CompletionState)
	require.False(t, materializeStatus.MaterializeAllowed)
	require.False(t, materializeStatus.MaterializeRequiresForce)

	backfillStatus, err := InspectOrbitBootstrapLaneForOperation(context.Background(), repo.Root, "completed", "backfill")
	require.NoError(t, err)
	require.Equal(t, BootstrapCompletionStateCompleted, backfillStatus.CompletionState)
	require.False(t, backfillStatus.BackfillAllowed)
}

func TestInspectOrbitBootstrapLaneIgnoresRuntimeCompletionStateOutsideRuntimeRevision(t *testing.T) {
	t.Parallel()

	repo := seedBootstrapPlannerRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: completed\n"+
		"  branch: source/completed\n")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "completed",
		UpdatedAt: time.Date(2026, time.April, 19, 9, 0, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 8, 45, 0, 0, time.UTC),
		},
	}))

	status, err := InspectOrbitBootstrapLaneForOperation(context.Background(), repo.Root, "completed", "materialize")
	require.NoError(t, err)
	require.Equal(t, "source", status.RevisionKind)
	require.Equal(t, BootstrapCompletionStatePending, status.CompletionState)
	require.True(t, status.MaterializeAllowed)
}

func seedBootstrapPlannerRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-19T00:00:00Z\n"+
		"  updated_at: 2026-04-19T00:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")

	completedSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("completed")
	require.NoError(t, err)
	require.NotNil(t, completedSpec.Meta)
	completedSpec.Meta.BootstrapTemplate = "Bootstrap the completed orbit.\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, completedSpec)
	require.NoError(t, err)

	pendingSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("pending")
	require.NoError(t, err)
	pendingSpec.Members = append(pendingSpec.Members, orbitpkg.OrbitMember{
		Key:  "pending-bootstrap",
		Role: orbitpkg.OrbitMemberRule,
		Lane: orbitpkg.OrbitMemberLaneBootstrap,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"bootstrap/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, pendingSpec)
	require.NoError(t, err)

	plainSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("plain")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, plainSpec)
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed bootstrap planner repo")

	return repo
}
