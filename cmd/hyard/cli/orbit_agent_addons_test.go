package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardOrbitAgentInspectReportsPackageHookAddons(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeWithAgentAddonHook(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "agent", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot string `json:"repo_root"`
		OrbitID  string `json:"orbit"`
		Hooks    []struct {
			Package     string          `json:"package"`
			ID          string          `json:"id"`
			DisplayID   string          `json:"display_id"`
			EventKind   string          `json:"event_kind"`
			HandlerPath string          `json:"handler_path"`
			Targets     map[string]bool `json:"targets"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Len(t, payload.Hooks, 1)
	require.Equal(t, "docs", payload.Hooks[0].Package)
	require.Equal(t, "block-dangerous-shell", payload.Hooks[0].ID)
	require.Equal(t, "docs:block-dangerous-shell", payload.Hooks[0].DisplayID)
	require.Equal(t, "tool.before", payload.Hooks[0].EventKind)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", payload.Hooks[0].HandlerPath)
	require.Equal(t, map[string]bool{"codex": true}, payload.Hooks[0].Targets)

	_, err = os.Stat(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardAgentInspectReportsPackageHookAddonsWithoutApplying(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeWithAgentAddonHook(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageAgentHookCount int `json:"package_agent_hook_count"`
		PackageAgentHooks     []struct {
			OrbitID     string `json:"orbit_id"`
			DisplayID   string `json:"display_id"`
			EventKind   string `json:"event_kind"`
			HandlerPath string `json:"handler_path"`
			Activation  string `json:"activation"`
		} `json:"package_agent_hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.PackageAgentHookCount)
	require.Len(t, payload.PackageAgentHooks, 1)
	require.Equal(t, "docs", payload.PackageAgentHooks[0].OrbitID)
	require.Equal(t, "docs:block-dangerous-shell", payload.PackageAgentHooks[0].DisplayID)
	require.Equal(t, "tool.before", payload.PackageAgentHooks[0].EventKind)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", payload.PackageAgentHooks[0].HandlerPath)
	require.Equal(t, "not_applied", payload.PackageAgentHooks[0].Activation)

	_, err = os.Stat(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardAgentPlanReportsPackageHookAddonsWithoutApplying(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeWithAgentAddonHook(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "plan", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageAgentHooks []struct {
			OrbitID     string `json:"orbit_id"`
			DisplayID   string `json:"display_id"`
			HandlerPath string `json:"handler_path"`
			Activation  string `json:"activation"`
		} `json:"package_agent_hooks"`
		DesiredTruth struct {
			PackageAgentHookCount int `json:"package_agent_hook_count"`
		} `json:"desired_truth"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.DesiredTruth.PackageAgentHookCount)
	require.Len(t, payload.PackageAgentHooks, 1)
	require.Equal(t, "docs", payload.PackageAgentHooks[0].OrbitID)
	require.Equal(t, "docs:block-dangerous-shell", payload.PackageAgentHooks[0].DisplayID)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", payload.PackageAgentHooks[0].HandlerPath)
	require.Equal(t, "not_applied", payload.PackageAgentHooks[0].Activation)

	_, err = os.Stat(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardAgentPlanHooksGroupsPackageHookNativePreview(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	selectHyardAgentAddonFramework(t, repo)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "plan", "--hooks", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HookPreview []struct {
			OrbitID        string `json:"orbit_id"`
			Package        string `json:"package"`
			AddonID        string `json:"addon_id"`
			Artifact       string `json:"artifact"`
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			Source         string `json:"source"`
			Path           string `json:"path"`
			Mode           string `json:"mode"`
			EffectiveScope string `json:"effective_scope"`
			HandlerDigest  string `json:"handler_digest"`
		} `json:"hook_preview"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	var found bool
	for _, output := range payload.HookPreview {
		if output.Artifact != "docs:block-dangerous-shell" || output.Route != "execute_later" {
			continue
		}
		found = true
		require.Equal(t, "docs", output.OrbitID)
		require.Equal(t, "docs", output.Package)
		require.Equal(t, "block-dangerous-shell", output.AddonID)
		require.Equal(t, "hook-implementation", output.ArtifactType)
		require.Equal(t, ".harness/orbits/docs.yaml", output.Source)
		require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", output.Path)
		require.Equal(t, "execute-later", output.Mode)
		require.Equal(t, "project", output.EffectiveScope)
		require.NotEmpty(t, output.HandlerDigest)
	}
	require.True(t, found, "expected package hook execute-later preview")
}

func TestHyardAgentApplyWithoutHooksLeavesPackageHookPending(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var applyPayload struct {
		Status    string `json:"status"`
		Readiness struct {
			Status string `json:"status"`
			Agent  struct {
				Status           string `json:"status"`
				ActivationStatus string `json:"activation_status"`
				Reasons          []struct {
					Code string `json:"code"`
				} `json:"reasons"`
			} `json:"agent"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, "ok", applyPayload.Status)
	require.Equal(t, "usable", applyPayload.Readiness.Status)
	require.Equal(t, "usable", applyPayload.Readiness.Agent.Status)
	require.Equal(t, "hooks_pending", applyPayload.Readiness.Agent.ActivationStatus)
	require.Contains(t, readinessReasonCodesFromApplyPayload(applyPayload.Readiness.Agent.Reasons), "agent_hooks_pending")
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "hooks.json"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var checkPayload struct {
		Configured   bool `json:"configured"`
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
		Findings     []struct {
			Kind     string `json:"kind"`
			OrbitID  string `json:"orbit_id"`
			Path     string `json:"path"`
			Message  string `json:"message"`
			Blocking bool   `json:"blocking"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &checkPayload))
	require.True(t, checkPayload.Configured)
	require.False(t, checkPayload.OK)
	require.GreaterOrEqual(t, checkPayload.FindingCount, 1)
	require.Contains(t, checkPayload.Findings, struct {
		Kind     string `json:"kind"`
		OrbitID  string `json:"orbit_id"`
		Path     string `json:"path"`
		Message  string `json:"message"`
		Blocking bool   `json:"blocking"`
	}{
		Kind:     "package_hook_pending",
		OrbitID:  "docs",
		Path:     "docs:block-dangerous-shell",
		Message:  "package hook add-on has not been applied with --hooks",
		Blocking: true,
	})
}

func readinessReasonCodesFromApplyPayload(reasons []struct {
	Code string `json:"code"`
}) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
	}

	return codes
}

func TestHyardAgentApplyHooksCodexAppliesPackageHookAndLedger(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "hooks/docs/block-dangerous-shell/run.sh")
	require.Contains(t, stderr, "Apply hook activation? accepted by --yes")

	var payload struct {
		Status    string `json:"status"`
		Readiness struct {
			Agent struct {
				Status           string `json:"status"`
				ActivationStatus string `json:"activation_status"`
			} `json:"agent"`
		} `json:"readiness"`
		ArtifactResults []struct {
			Artifact       string `json:"artifact"`
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			Mode           string `json:"mode"`
			Path           string `json:"path"`
			EffectiveScope string `json:"effective_scope"`
			Status         string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Equal(t, "ready", payload.Readiness.Agent.Status)
	require.Equal(t, "current", payload.Readiness.Agent.ActivationStatus)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string `json:"artifact"`
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Mode           string `json:"mode"`
		Path           string `json:"path"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		Artifact:       "codex-hooks",
		ArtifactType:   "hook-config",
		Route:          "project_hooks",
		Mode:           "generate",
		Path:           ".codex/hooks.json",
		EffectiveScope: "project",
		Status:         "project_applied",
	})

	codexHooksData, err := os.ReadFile(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.NoError(t, err)
	codexHooks := string(codexHooksData)
	require.Contains(t, codexHooks, `"PreToolUse"`)
	require.Contains(t, codexHooks, "--target codex")
	require.Contains(t, codexHooks, "--hook docs:block-dangerous-shell")

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.NoError(t, err)
	require.Len(t, activation.PackageHooks, 1)
	require.Equal(t, "docs", activation.PackageHooks[0].Package)
	require.Equal(t, "block-dangerous-shell", activation.PackageHooks[0].AddonID)
	require.Equal(t, "docs:block-dangerous-shell", activation.PackageHooks[0].DisplayID)
	require.Equal(t, "PreToolUse", activation.PackageHooks[0].NativeEvent)
	require.Equal(t, "project_hooks", activation.PackageHooks[0].HookApplyMode)
	require.NotEmpty(t, activation.PackageHooks[0].HandlerDigest)

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var checkPayload struct {
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &checkPayload))
	require.True(t, checkPayload.OK)
	require.Zero(t, checkPayload.FindingCount)
}

func TestHyardHooksRunExecutesPackageHookAddon(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHook(t)
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, `{"tool_name":"shell","command":"rm -rf build"}`, "hooks", "run", "--root", repo.Root, "--target", "codex", "--hook", "docs:block-dangerous-shell")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `"decision":"allow"`)

	captured, err := os.ReadFile(filepath.Join(repo.Root, "hooks", "docs", "block-dangerous-shell", "captured.json"))
	require.NoError(t, err)
	require.Contains(t, string(captured), `"hook": "docs:block-dangerous-shell"`)
	require.Contains(t, string(captured), `"kind": "tool.before"`)
	require.Contains(t, string(captured), `"native_input"`)
}

func TestHyardAgentCheckAndApplyBlockUnsupportedRequiredPackageHook(t *testing.T) {
	repo := seedHyardRuntimeWithAgentAddonHookOptions(t, hyardAgentAddonHookOptions{
		Required:  true,
		EventKind: "compact.before",
	})
	selectHyardAgentAddonFramework(t, repo)
	makeHyardAgentAddonHandlerExecutable(t, repo)
	t.Setenv("HOME", t.TempDir())

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Kind     string `json:"kind"`
			OrbitID  string `json:"orbit_id"`
			Path     string `json:"path"`
			Message  string `json:"message"`
			Blocking bool   `json:"blocking"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.OK)
	require.Contains(t, payload.Findings, struct {
		Kind     string `json:"kind"`
		OrbitID  string `json:"orbit_id"`
		Path     string `json:"path"`
		Message  string `json:"message"`
		Blocking bool   `json:"blocking"`
	}{
		Kind:     "package_hook_event_unsupported",
		OrbitID:  "docs",
		Path:     "docs:block-dangerous-shell",
		Message:  "required package hook event is not supported by the resolved framework",
		Blocking: true,
	})

	_, _, err = executeHyardCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.ErrorContains(t, err, `required package hook "docs:block-dangerous-shell" event is not supported by framework "codex"`)
}

func seedHyardRuntimeWithAgentAddonHook(t *testing.T) *testutil.Repo {
	t.Helper()

	return seedHyardRuntimeWithAgentAddonHookOptions(t, hyardAgentAddonHookOptions{
		EventKind: "tool.before",
	})
}

type hyardAgentAddonHookOptions struct {
	Required  bool
	EventKind string
}

func seedHyardRuntimeWithAgentAddonHookOptions(t *testing.T, options hyardAgentAddonHookOptions) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHyardCLI(t, repo.Root, "init", "runtime")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"agent_addons:\n"+
		"  hooks:\n"+
		"    unsupported_behavior: skip\n"+
		"    entries:\n"+
		"      - id: block-dangerous-shell\n"+
		"        required: "+boolYAML(options.Required)+"\n"+
		"        description: Block dangerous shell commands.\n"+
		"        event:\n"+
		"          kind: "+options.EventKind+"\n"+
		"        match:\n"+
		"          tools: [shell]\n"+
		"        handler:\n"+
		"          type: command\n"+
		"          path: hooks/docs/block-dangerous-shell/run.sh\n"+
		"        targets:\n"+
		"          codex: true\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - hooks/docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/bin/sh\nset -eu\ncat > hooks/docs/block-dangerous-shell/captured.json\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.Run(t, "add", "-A")
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "docs", time.Date(2026, time.April, 26, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	return repo
}

func selectHyardAgentAddonFramework(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	_, err := harnesspkg.WriteFrameworkSelection(filepath.Join(repo.Root, ".git"), harnesspkg.FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   harnesspkg.FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 26, 13, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
}

func makeHyardAgentAddonHandlerExecutable(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "docs", "block-dangerous-shell", "run.sh"), 0o755))
}

func boolYAML(value bool) string {
	if value {
		return "true"
	}

	return "false"
}
