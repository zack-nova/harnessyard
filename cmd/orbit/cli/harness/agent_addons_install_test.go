package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestResolveLocalTemplateInstallSourceRejectsPureAgentAddonHandlerOutsideExportSurface(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	writeHarnessTemplateAgentAddonSpec(t, repo.Root, false)
	repo.WriteFile(t, "hooks/workspace/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	repo.AddAndCommit(t, "add agent addon outside export surface")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.ErrorContains(t, err, `handler.path "hooks/workspace/block-dangerous-shell/run.sh" must resolve inside the export surface`)
}

func TestBuildTemplateInstallPreviewSnapshotsPackageAgentAddons(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	writeHarnessTemplateAgentAddonSpec(t, sourceRepo.Root, true)
	sourceRepo.WriteFile(t, "hooks/workspace/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	sourceRepo.AddAndCommit(t, "add agent addon")
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 26, 11, 0, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)
	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath: bindingsPath,
		Now:              now,
	})
	require.NoError(t, err)
	require.NotNil(t, preview.BundleRecord.AgentAddons)
	require.Len(t, preview.BundleRecord.AgentAddons.Hooks, 1)
	hook := preview.BundleRecord.AgentAddons.Hooks[0]
	require.Equal(t, "workspace:block-dangerous-shell", hook.DisplayID)
	require.Equal(t, "hooks/workspace/block-dangerous-shell/run.sh", hook.HandlerPath)
	require.NotEmpty(t, hook.HandlerDigest)

	result, err := ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, preview, false)
	require.NoError(t, err)
	require.Contains(t, result.WrittenPaths, ".harness/bundles/workspace.yaml")

	record, err := LoadBundleRecord(runtimeRepo.Root, "workspace")
	require.NoError(t, err)
	require.NotNil(t, record.AgentAddons)
	require.Equal(t, preview.BundleRecord.AgentAddons, record.AgentAddons)

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestEvaluateRuntimeReadinessReportsMissingAgentActivationForPackageAddons(t *testing.T) {
	t.Parallel()

	repo := seedAgentAddonReadinessRepo(t, true)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusUsable, report.Status)
	require.Equal(t, ReadinessStatusReady, report.Runtime.Status)
	require.Equal(t, ReadinessStatusUsable, report.Agent.Status)
	require.True(t, report.Agent.Required)
	require.Equal(t, "codex", report.Agent.ResolvedAgent)
	require.Equal(t, "missing", report.Agent.ActivationStatus)
	require.Contains(t, readinessReasonCodes(report.Agent.Reasons), string(ReadinessReasonAgentActivationMissing))
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonAgentActivationMissing))
	require.Contains(t, readinessStepCommands(report.NextSteps), "hyard agent plan --hooks")
}

func TestEvaluateRuntimeReadinessReportsHooksPendingAfterAgentApplyWithoutHooks(t *testing.T) {
	t.Parallel()

	repo := seedAgentAddonReadinessRepo(t, true)
	result, err := ApplyFramework(context.Background(), FrameworkApplyInput{
		RepoRoot:    repo.Root,
		GitDir:      repo.GitDir(t),
		HarnessID:   "workspace",
		RouteChoice: FrameworkApplyRouteProject,
		EnableHooks: false,
	})
	require.NoError(t, err)
	require.Equal(t, "ok", result.Status)

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusUsable, report.Status)
	require.Equal(t, ReadinessStatusReady, report.Runtime.Status)
	require.Equal(t, ReadinessStatusUsable, report.Agent.Status)
	require.Equal(t, "hooks_pending", report.Agent.ActivationStatus)
	require.Contains(t, readinessReasonCodes(report.Agent.Reasons), string(ReadinessReasonAgentHooksPending))
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonAgentHooksPending))
	require.Contains(t, readinessStepCommands(report.NextSteps), "hyard agent apply --hooks --yes")
}

func TestEvaluateRuntimeReadinessReportsStaleAgentActivationWhenPackageAddonsChange(t *testing.T) {
	t.Parallel()

	repo := seedAgentAddonReadinessRepo(t, false)
	state, err := loadFrameworkDesiredState(context.Background(), repo.Root, repo.GitDir(t))
	require.NoError(t, err)
	guidanceHash, capabilitiesHash, selectionHash, runtimeAgentTruthHash, err := computeFrameworkDesiredHashes(repo.Root, state)
	require.NoError(t, err)
	_, err = WriteFrameworkActivation(repo.GitDir(t), FrameworkActivation{
		Framework:             "codex",
		ResolutionSource:      FrameworkSelectionSourceExplicitLocal,
		RepoRoot:              repo.Root,
		AppliedAt:             time.Date(2026, time.April, 26, 11, 30, 0, 0, time.UTC),
		GuidanceHash:          guidanceHash,
		CapabilitiesHash:      capabilitiesHash,
		SelectionHash:         selectionHash,
		RuntimeAgentTruthHash: runtimeAgentTruthHash,
	})
	require.NoError(t, err)

	writeReadinessHostedSpec(t, repo.Root, true)
	repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	repo.AddAndCommit(t, "add agent addon")

	report, err := EvaluateRuntimeReadiness(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ReadinessStatusUsable, report.Status)
	require.Equal(t, ReadinessStatusReady, report.Runtime.Status)
	require.Equal(t, ReadinessStatusUsable, report.Agent.Status)
	require.Equal(t, "codex", report.Agent.ResolvedAgent)
	require.Equal(t, "stale", report.Agent.ActivationStatus)
	require.Contains(t, readinessReasonCodes(report.Agent.Reasons), string(ReadinessReasonAgentActivationStale))
	require.Contains(t, readinessReasonCodes(report.RuntimeReasons), string(ReadinessReasonAgentActivationStale))
	require.Contains(t, readinessStepCommands(report.NextSteps), "hyard agent plan --hooks")
}

func writeHarnessTemplateAgentAddonSpec(t *testing.T, repoRoot string, includeHookExport bool) {
	t.Helper()

	content := "" +
		"  - key: workspace-content\n" +
		"    role: rule\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - docs/**\n" +
		"        - schema/**\n"
	if includeHookExport {
		content += "" +
			"  - key: hook-assets\n" +
			"    role: rule\n" +
			"    paths:\n" +
			"      include:\n" +
			"        - hooks/**\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".harness", "orbits", "workspace.yaml"), []byte(""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: workspace\n"+
		"description: Workspace orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/workspace.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"content:\n"+
		content+
		"behavior:\n"+
		"  scope:\n"+
		"    projection_roles:\n"+
		"      - meta\n"+
		"      - subject\n"+
		"      - rule\n"+
		"      - process\n"+
		"    write_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    export_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    orchestration_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"      - process\n"+
		"  orchestration:\n"+
		"    include_orbit_description: true\n"+
		"    materialize_agents_from_meta: true\n"+
		"agent_addons:\n"+
		"  hooks:\n"+
		"    entries:\n"+
		"      - id: block-dangerous-shell\n"+
		"        required: true\n"+
		"        event:\n"+
		"          kind: tool.before\n"+
		"        match:\n"+
		"          tools:\n"+
		"            - shell\n"+
		"        handler:\n"+
		"          type: command\n"+
		"          path: hooks/workspace/block-dangerous-shell/run.sh\n"+
		"        targets:\n"+
		"          codex: true\n"), 0o600))
}

func seedAgentAddonReadinessRepo(t *testing.T, withAddon bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 26, 11, 15, 0, 0, time.UTC)
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
			{OrbitID: "docs", Source: MemberSourceInstallBundle, OwnerHarnessID: "workspace", AddedAt: now},
		},
	})
	require.NoError(t, err)
	writeReadinessHostedSpec(t, repo.Root, withAddon)
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	if withAddon {
		repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	}
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "workspace",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: "abc123",
		},
		MemberIDs:          []string{"docs"},
		AppliedAt:          now,
		IncludesRootAgents: false,
		OwnedPaths:         []string{".harness/orbits/docs.yaml", "docs/guide.md"},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed readiness runtime")
	_, err = WriteFrameworkSelection(repo.GitDir(t), FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         now,
	})
	require.NoError(t, err)

	return repo
}

func writeReadinessHostedSpec(t *testing.T, repoRoot string, withAddon bool) {
	t.Helper()

	spec, err := orbit.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Members = []orbit.OrbitMember{
		{
			Key:  "docs-content",
			Role: orbit.OrbitMemberRule,
			Paths: orbit.OrbitMemberPaths{
				Include: []string{"docs/**", "hooks/**"},
			},
		},
	}
	if withAddon {
		spec.AgentAddons = &orbit.OrbitAgentAddons{
			Hooks: &orbit.OrbitAgentHookAddons{
				Entries: []orbit.OrbitAgentHookEntry{
					{
						ID:       "block-dangerous-shell",
						Required: true,
						Event:    orbit.AgentAddonHookEvent{Kind: "tool.before"},
						Match:    orbit.AgentAddonHookMatch{Tools: []string{"shell"}},
						Handler: orbit.AgentAddonHookHandler{
							Type: "command",
							Path: "hooks/docs/block-dangerous-shell/run.sh",
						},
						Targets: map[string]bool{"codex": true},
					},
				},
			},
		}
	}
	_, err = orbit.WriteHostedOrbitSpec(repoRoot, spec)
	require.NoError(t, err)
}
