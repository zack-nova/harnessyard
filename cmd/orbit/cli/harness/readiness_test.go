package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestEvaluateRuntimeReadinessZeroMemberRuntimeIsReady(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 13, 9, 0, 0, 0, time.UTC)
	runtimeFile, err := DefaultRuntimeFile(repo.Root, now)
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusReady, report.Status)
	require.Equal(t, runtimeFile.Harness.ID, report.HarnessID)
	require.Equal(t, ReadinessStatusReady, report.Runtime.Status)
	require.Equal(t, ReadinessStatusReady, report.Agent.Status)
	require.False(t, report.Agent.Required)
	require.Equal(t, "not_required", report.Agent.ActivationStatus)
	require.Equal(t, 0, report.Summary.OrbitCount)
	require.Empty(t, report.RuntimeReasons)
	require.Empty(t, report.OrbitReports)
	require.Empty(t, report.NextSteps)
}

func TestEvaluateRuntimeReadinessInstallBackedOrbitWithMissingRequiredBindingsIsUsable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 13, 10, 0, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			Name:      "Workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: now},
		},
	})
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Meta.AgentsTemplate = "Docs orbit for $project_name\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	_, err = WriteVarsFile(repo.Root, bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
	})
	require.NoError(t, err)

	_, err = WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {Description: "Project name", Required: true},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{},
		},
	})
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusUsable, report.Status)
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonUnresolvedRequiredBindings))

	orbitReport := requireReadinessOrbitReport(t, report, "docs")
	require.Equal(t, ReadinessStatusUsable, orbitReport.Status)
	require.Contains(t, readinessReasonCodes(orbitReport.Reasons), string(ReadinessReasonUnresolvedRequiredBindings))
}

func TestEvaluateRuntimeReadinessManualOrbitWithoutAgentsTemplateIsUsable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 13, 11, 0, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			Name:      "Workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceManual, AddedAt: now},
		},
	})
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusUsable, report.Status)

	orbitReport := requireReadinessOrbitReport(t, report, "docs")
	require.Equal(t, ReadinessStatusUsable, orbitReport.Status)
	require.Contains(t, readinessReasonCodes(orbitReport.Reasons), string(ReadinessReasonAgentsNotComposed))
	require.Contains(t, readinessStepCommands(report.NextSteps), "hyard guide sync --target agents --output")
}

func TestEvaluateRuntimeReadinessBundleOwnedOrbitsWithoutStandaloneAgentsBlocksAreReady(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 25, 9, 0, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "runtime-two",
			Name:      "Runtime Two",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "plan", Source: MemberSourceInstallBundle, OwnerHarnessID: "runtime-two", AddedAt: now},
			{OrbitID: "research", Source: MemberSourceInstallBundle, OwnerHarnessID: "runtime-two", AddedAt: now},
		},
	})
	require.NoError(t, err)

	for _, orbitID := range []string{"plan", "research"} {
		spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
		require.NoError(t, err)
		spec.Meta.AgentsTemplate = orbitID + " worker guidance\n"
		_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
		require.NoError(t, err)
	}

	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "runtime-two",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/runtime-two",
			TemplateCommit: "abc123",
		},
		MemberIDs:          []string{"plan", "research"},
		AppliedAt:          now,
		IncludesRootAgents: false,
		OwnedPaths: []string{
			".harness/orbits/plan.yaml",
			".harness/orbits/research.yaml",
		},
	})
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusReady, report.Status)
	require.Equal(t, 2, report.Summary.OrbitCount)
	require.Equal(t, 2, report.Summary.ReadyOrbitCount)
	require.Zero(t, report.Summary.UsableOrbitCount)
	require.Empty(t, report.RuntimeReasons)
	require.Empty(t, report.NextSteps)

	for _, orbitID := range []string{"plan", "research"} {
		orbitReport := requireReadinessOrbitReport(t, report, orbitID)
		require.Equal(t, MemberSourceInstallBundle, orbitReport.MemberSource)
		require.Equal(t, ReadinessStatusReady, orbitReport.Status)
		require.Empty(t, orbitReport.Reasons)
	}
}

func TestEvaluateRuntimeReadinessBundleOwnedOrbitWithInvalidAgentsContainerIsBroken(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 25, 9, 15, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "runtime-two",
			Name:      "Runtime Two",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "plan", Source: MemberSourceInstallBundle, OwnerHarnessID: "runtime-two", AddedAt: now},
		},
	})
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("plan")
	require.NoError(t, err)
	spec.Meta.AgentsTemplate = "plan worker guidance\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "runtime-two",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/runtime-two",
			TemplateCommit: "abc123",
		},
		MemberIDs:          []string{"plan"},
		AppliedAt:          now,
		IncludesRootAgents: true,
		OwnedPaths:         []string{".harness/orbits/plan.yaml", "AGENTS.md"},
		RootAgentsDigest:   contentDigest([]byte("bundle guidance\n")),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "AGENTS.md"), []byte("<!-- harness:begin workflow=\"runtime-two\" -->\nbundle guidance\n"), 0o600))

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusBroken, report.Status)
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonInvalidAgentsContainer))

	orbitReport := requireReadinessOrbitReport(t, report, "plan")
	require.Equal(t, ReadinessStatusReady, orbitReport.Status)
	require.Empty(t, orbitReport.Reasons)
}

func TestEvaluateRuntimeReadinessHarnessOwnedManualOrbitWithoutStandaloneAgentsBlockIsReady(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 25, 10, 0, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "runtime-two",
			Name:      "Runtime Two",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "plan", Source: MemberSourceManual, OwnerHarnessID: "runtime-two", AddedAt: now},
		},
	})
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("plan")
	require.NoError(t, err)
	spec.Meta.AgentsTemplate = "plan worker guidance\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusReady, report.Status)
	require.Empty(t, report.RuntimeReasons)
	require.Empty(t, report.NextSteps)

	orbitReport := requireReadinessOrbitReport(t, report, "plan")
	require.Equal(t, ReadinessStatusReady, orbitReport.Status)
	require.Empty(t, orbitReport.Reasons)
}

func TestEvaluateRuntimeReadinessIgnoresDetachedInstallRecordWithoutMatchingMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 18, 0, 0, 0, time.UTC)
	runtimeFile, err := DefaultRuntimeFile(repo.Root, now)
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusReady, report.Status)
	require.Equal(t, 0, report.Summary.OrbitCount)
	require.Empty(t, report.RuntimeReasons)
	require.Empty(t, report.OrbitReports)
}

func TestEvaluateRuntimeReadinessInvalidAgentsContainerIsBroken(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 13, 12, 0, 0, 0, time.UTC)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			Name:      "Workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceManual, AddedAt: now},
		},
	})
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Meta.AgentsTemplate = "Docs orbit for $project_name\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "AGENTS.md"), []byte("<<broken>>\n<!-- orbit:begin workflow='docs' -->\n"), 0o600))

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusBroken, report.Status)
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonInvalidAgentsContainer))

	orbitReport := requireReadinessOrbitReport(t, report, "docs")
	require.Equal(t, ReadinessStatusBroken, orbitReport.Status)
	require.Contains(t, readinessReasonCodes(orbitReport.Reasons), string(ReadinessReasonInvalidAgentsContainer))
}

func requireReadinessOrbitReport(t *testing.T, report ReadinessReport, orbitID string) ReadinessOrbitReport {
	t.Helper()

	for _, orbitReport := range report.OrbitReports {
		if orbitReport.OrbitID == orbitID {
			return orbitReport
		}
	}

	t.Fatalf("orbit report %q not found in %+v", orbitID, report.OrbitReports)
	return ReadinessOrbitReport{}
}

func readinessReasonCodes(reasons []ReadinessReason) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, string(reason.Code))
	}
	return codes
}

func readinessStepCommands(steps []ReadinessNextStep) []string {
	commands := make([]string, 0, len(steps))
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	return commands
}
