package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

type hyardStartDryRunPayload struct {
	DryRun              bool   `json:"dry_run"`
	HarnessRoot         string `json:"harness_root"`
	HarnessID           string `json:"harness_id"`
	FrameworkResolution struct {
		Status            string   `json:"status"`
		SelectedFramework string   `json:"selected_framework"`
		SelectionSource   string   `json:"selection_source"`
		Candidates        []string `json:"candidates"`
	} `json:"framework_resolution"`
	Activation struct {
		Status string `json:"status"`
		Route  string `json:"route"`
	} `json:"activation"`
	BootstrapAgentSkill struct {
		Framework string `json:"framework"`
		Action    string `json:"action"`
		Changed   bool   `json:"changed"`
		SkillPath string `json:"skill_path"`
	} `json:"bootstrap_agent_skill"`
	Launcher struct {
		Framework                  string   `json:"framework"`
		Status                     string   `json:"status"`
		Launchable                 bool     `json:"launchable"`
		DetectionStatus            string   `json:"detection_status"`
		TerminalCLIDetected        bool     `json:"terminal_cli_detected"`
		ManualFallbackInstructions []string `json:"manual_fallback_instructions"`
	} `json:"launcher"`
	StartPrompt string `json:"start_prompt"`
}

func decodeHyardStartDryRunPayload(t *testing.T, stdout string) hyardStartDryRunPayload {
	t.Helper()

	var payload hyardStartDryRunPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	return payload
}

type recordingStartLauncher struct {
	requests      []harnesspkg.StartLaunchRequest
	sawActivation bool
	sawBootstrap  bool
}

func (launcher *recordingStartLauncher) Plan(_ context.Context, _ harnesspkg.StartPlanInput, frameworkID string) harnesspkg.StartLauncherPlan {
	return harnesspkg.StartLauncherPlan{
		Framework:  frameworkID,
		Status:     "test",
		Launchable: true,
	}
}

func (launcher *recordingStartLauncher) Launch(_ context.Context, request harnesspkg.StartLaunchRequest) (harnesspkg.StartLaunchResult, error) {
	launcher.requests = append(launcher.requests, request)
	if _, err := harnesspkg.LoadFrameworkActivation(request.GitDir, request.Framework); err == nil {
		launcher.sawActivation = true
	}
	if _, err := os.Stat(filepath.Join(request.RepoRoot, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md")); err == nil {
		launcher.sawBootstrap = true
	}

	return harnesspkg.StartLaunchResult{
		Framework:  request.Framework,
		Status:     "launched",
		Launchable: true,
	}, nil
}

func addHyardStartAgentCapability(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the docs orbit.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/review.md\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work.\n")
	repo.AddAndCommit(t, "seed start command capability")
}

func frameworkActivationOutputPaths(outputs []harnesspkg.FrameworkActivationOutput) []string {
	paths := make([]string, 0, len(outputs))
	for _, output := range outputs {
		paths = append(paths, output.Path)
	}

	return paths
}

func TestHyardStartWithExplicitFrameworkExecutesProjectOnlyThroughInjectedLauncher(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)
	addHyardStartAgentCapability(t, repo)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	beforeHead := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	launcher := &recordingStartLauncher{}

	stdout, stderr, err := executeHyardCLIWithStartLauncherUnlocked(t, repo.Root, launcher, "start", "--with", "codex")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "harness start handed off to codex")

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "codex", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceExplicitLocal, selection.SelectionSource)

	activation, err := harnesspkg.LoadFrameworkActivation(repo.GitDir(t), "codex")
	require.NoError(t, err)
	require.Equal(t, "codex", activation.Framework)
	require.Empty(t, activation.GlobalOutputs)
	require.Contains(t, frameworkActivationOutputPaths(activation.ProjectOutputs), ".codex/skills/review")

	require.FileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "skills", "review"))
	require.NoFileExists(t, filepath.Join(homeDir, ".codex", "prompts", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review.md"))
	require.Len(t, launcher.requests, 1)
	require.True(t, launcher.sawActivation)
	require.True(t, launcher.sawBootstrap)
	require.Equal(t, repo.Root, launcher.requests[0].RepoRoot)
	require.Equal(t, repo.GitDir(t), launcher.requests[0].GitDir)
	require.Equal(t, "codex", launcher.requests[0].Framework)
	require.Contains(t, launcher.requests[0].StartPrompt, "Harness Start")
	require.Equal(t, beforeHead, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
}

func TestHyardStartSelectsOnlyReadyAgentThroughInjectedLauncher(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	t.Setenv("HOME", t.TempDir())
	stubCodexExecutableOnPath(t)
	beforeHead := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	launcher := &recordingStartLauncher{}

	stdout, stderr, err := executeHyardCLIWithStartLauncherUnlocked(t, repo.Root, launcher, "start")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "harness start handed off to codex")

	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	activation, err := harnesspkg.LoadFrameworkActivation(repo.GitDir(t), "codex")
	require.NoError(t, err)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceProjectDetection, activation.ResolutionSource)
	require.Len(t, launcher.requests, 1)
	require.Equal(t, "codex", launcher.requests[0].Framework)
	require.True(t, launcher.sawActivation)
	require.True(t, launcher.sawBootstrap)
	require.Equal(t, beforeHead, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
}

func TestHyardStartPrintPromptPrintsStartPromptInRuntime(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Harness Start")
	require.Contains(t, stdout, "Start Prompt")
	require.Contains(t, stdout, "First handle any pending Harness Runtime bootstrap work.")
	require.Contains(t, stdout, "Then introduce this Harness Runtime in the same session.")
}

func TestHyardStartPrintPromptFailsClearlyOutsideHarnessRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "Ordinary repository\n")
	repo.AddAndCommit(t, "seed ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "outside a Harness Runtime")
}

func TestHyardStartPrintPromptDoesNotMutateRuntimeOrAgentState(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.NoError(t, err)
	require.NotEmpty(t, stdout)
	require.Empty(t, stderr)

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".claude", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
}

func TestHyardStartDryRunJSONPlansHarnessStartWithoutMutatingRuntime(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)
	_, err := harnesspkg.WriteFrameworksFile(repo.Root, harnesspkg.FrameworksFile{
		SchemaVersion:        1,
		RecommendedFramework: "codex",
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed recommended framework", ".harness/agents/manifest.yaml")
	beforeStatus := repo.Run(t, "status", "--short")

	lockHyardProcessEnv(t)
	configureHyardStartDetectionPath(t, map[string]string{})

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHyardStartDryRunPayload(t, stdout)

	require.True(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.NotEmpty(t, payload.HarnessID)
	require.Equal(t, "resolved", payload.FrameworkResolution.Status)
	require.Equal(t, "codex", payload.FrameworkResolution.SelectedFramework)
	require.Equal(t, "recommended_default", payload.FrameworkResolution.SelectionSource)
	require.Empty(t, payload.FrameworkResolution.Candidates)
	require.Equal(t, "planned", payload.Activation.Status)
	require.Equal(t, "project", payload.Activation.Route)
	require.Equal(t, "codex", payload.BootstrapAgentSkill.Framework)
	require.Equal(t, "create", payload.BootstrapAgentSkill.Action)
	require.True(t, payload.BootstrapAgentSkill.Changed)
	require.Equal(t, ".codex/skills/harness-runtime-bootstrap/SKILL.md", payload.BootstrapAgentSkill.SkillPath)
	require.Equal(t, "codex", payload.Launcher.Framework)
	require.Equal(t, "unverified", payload.Launcher.Status)
	require.False(t, payload.Launcher.Launchable)
	require.NotEmpty(t, payload.Launcher.ManualFallbackInstructions)
	require.Contains(t, payload.StartPrompt, "Harness Start")
	require.Contains(t, payload.StartPrompt, "Start Prompt")

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
}

func TestHyardStartDryRunJSONSelectsOnlyReadyAgentWithoutMutatingDetectionCache(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\necho 'codex 0.125.0'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHyardStartDryRunPayload(t, stdout)
	require.Equal(t, "resolved", payload.FrameworkResolution.Status)
	require.Equal(t, "codex", payload.FrameworkResolution.SelectedFramework)
	require.Equal(t, "project_detection", payload.FrameworkResolution.SelectionSource)
	require.Equal(t, "planned", payload.Activation.Status)
	require.Equal(t, "codex", payload.BootstrapAgentSkill.Framework)
	require.Equal(t, "codex", payload.Launcher.Framework)
	require.Equal(t, "launchable", payload.Launcher.Status)
	require.True(t, payload.Launcher.Launchable)
	require.Equal(t, "installed_cli", payload.Launcher.DetectionStatus)
	require.True(t, payload.Launcher.TerminalCLIDetected)
	require.Empty(t, payload.Launcher.ManualFallbackInstructions)

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoFileExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "detection-cache.json"))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func TestHyardStartDryRunJSONExplicitFrameworkWinsOverSavedSelection(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	_, err := harnesspkg.WriteFrameworkSelection(repo.GitDir(t), harnesspkg.FrameworkSelection{
		SelectedFramework: "claudecode",
		SelectionSource:   harnesspkg.FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.May, 5, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--with", "codex", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHyardStartDryRunPayload(t, stdout)
	require.Equal(t, "resolved", payload.FrameworkResolution.Status)
	require.Equal(t, "codex", payload.FrameworkResolution.SelectedFramework)
	require.Equal(t, "explicit_local", payload.FrameworkResolution.SelectionSource)
	require.Equal(t, "codex", payload.BootstrapAgentSkill.Framework)
	require.Equal(t, ".codex/skills/harness-runtime-bootstrap/SKILL.md", payload.BootstrapAgentSkill.SkillPath)
	require.Equal(t, "codex", payload.Launcher.Framework)

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "claudecode", selection.SelectedFramework)
	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func TestHyardStartDryRunJSONReportsAmbiguousFrameworkCandidates(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	_, err := harnesspkg.WriteFrameworkSelection(repo.GitDir(t), harnesspkg.FrameworkSelection{
		SelectedFramework:  "codex",
		SelectedFrameworks: []string{"codex", "openclaw"},
		SelectionSource:    harnesspkg.FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:          time.Date(2026, time.May, 5, 12, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHyardStartDryRunPayload(t, stdout)
	require.Equal(t, "ambiguous", payload.FrameworkResolution.Status)
	require.Empty(t, payload.FrameworkResolution.SelectedFramework)
	require.Equal(t, "unresolved_conflict", payload.FrameworkResolution.SelectionSource)
	require.Equal(t, []string{"codex", "openclaw"}, payload.FrameworkResolution.Candidates)
	require.Equal(t, "skipped", payload.Activation.Status)
	require.Equal(t, "project", payload.Activation.Route)
	require.Equal(t, "skipped", payload.BootstrapAgentSkill.Action)
	require.Equal(t, "skipped", payload.Launcher.Status)
	require.False(t, payload.Launcher.Launchable)
	require.NotEmpty(t, payload.Launcher.ManualFallbackInstructions)
	require.Contains(t, payload.StartPrompt, "Harness Start")

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, []string{"codex", "openclaw"}, harnesspkg.FrameworkSelectionIDs(selection))
	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func TestHyardStartDryRunJSONShapesUnsupportedLauncherFallback(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--with", "openclaw", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHyardStartDryRunPayload(t, stdout)
	require.Equal(t, "resolved", payload.FrameworkResolution.Status)
	require.Equal(t, "openclaw", payload.FrameworkResolution.SelectedFramework)
	require.Equal(t, "explicit_local", payload.FrameworkResolution.SelectionSource)
	require.Equal(t, "planned", payload.Activation.Status)
	require.Equal(t, "openclaw", payload.BootstrapAgentSkill.Framework)
	require.Equal(t, "skills/harness-runtime-bootstrap/SKILL.md", payload.BootstrapAgentSkill.SkillPath)
	require.Equal(t, "openclaw", payload.Launcher.Framework)
	require.Equal(t, "unsupported", payload.Launcher.Status)
	require.False(t, payload.Launcher.Launchable)
	require.NotEmpty(t, payload.Launcher.ManualFallbackInstructions)

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func TestHyardStartDryRunJSONDistinguishesTerminalCLIFromNonLaunchableDetection(t *testing.T) {
	t.Run("terminal CLI", func(t *testing.T) {
		repo := seedCommittedHyardRuntimeRepo(t)

		lockHyardProcessEnv(t)
		configureHyardStartDetectionPath(t, map[string]string{
			"openclaw": "#!/bin/sh\nprintf '%s\\n' 'openclaw 0.9.0'\n",
		})

		stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--dry-run", "--json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		payload := decodeHyardStartDryRunPayload(t, stdout)
		require.Equal(t, "resolved", payload.FrameworkResolution.Status)
		require.Equal(t, "openclaw", payload.FrameworkResolution.SelectedFramework)
		require.Equal(t, "project_detection", payload.FrameworkResolution.SelectionSource)
		require.Equal(t, "openclaw", payload.Launcher.Framework)
		require.Equal(t, "unsupported", payload.Launcher.Status)
		require.Equal(t, "installed_cli", payload.Launcher.DetectionStatus)
		require.True(t, payload.Launcher.TerminalCLIDetected)
		require.Contains(t, payload.Launcher.ManualFallbackInstructions, "A terminal OpenClaw CLI was detected, but Harness Start does not yet implement an OpenClaw launcher.")
	})

	t.Run("gateway only", func(t *testing.T) {
		repo := seedCommittedHyardRuntimeRepo(t)

		lockHyardProcessEnv(t)
		configureHyardStartDetectionPath(t, map[string]string{
			"systemctl": "#!/bin/sh\nif [ \"$1\" = \"--user\" ] && [ \"$2\" = \"is-active\" ] && [ \"$3\" = \"openclaw-gateway.service\" ]; then\n  printf '%s\\n' active\n  exit 0\nfi\nexit 1\n",
		})

		stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--dry-run", "--json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		payload := decodeHyardStartDryRunPayload(t, stdout)
		require.Equal(t, "resolved", payload.FrameworkResolution.Status)
		require.Equal(t, "openclaw", payload.FrameworkResolution.SelectedFramework)
		require.Equal(t, "project_detection", payload.FrameworkResolution.SelectionSource)
		require.Equal(t, "openclaw", payload.Launcher.Framework)
		require.Equal(t, "unsupported", payload.Launcher.Status)
		require.Equal(t, "running", payload.Launcher.DetectionStatus)
		require.False(t, payload.Launcher.TerminalCLIDetected)
		require.Contains(t, payload.Launcher.ManualFallbackInstructions, "OpenClaw was detected as running, but not as a verified terminal CLI launcher.")
	})

	t.Run("package only", func(t *testing.T) {
		repo := seedCommittedHyardRuntimeRepo(t)

		lockHyardProcessEnv(t)
		configureHyardStartDetectionPath(t, map[string]string{
			"npm": "#!/bin/sh\nprintf '%s\\n' '{\"dependencies\":{\"openclaw\":{\"version\":\"0.9.1\"}}}'\n",
		})

		stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--with", "openclaw", "--dry-run", "--json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		payload := decodeHyardStartDryRunPayload(t, stdout)
		require.Equal(t, "resolved", payload.FrameworkResolution.Status)
		require.Equal(t, "openclaw", payload.FrameworkResolution.SelectedFramework)
		require.Equal(t, "openclaw", payload.Launcher.Framework)
		require.Equal(t, "unsupported", payload.Launcher.Status)
		require.Equal(t, "installed_unverified", payload.Launcher.DetectionStatus)
		require.False(t, payload.Launcher.TerminalCLIDetected)
		require.Contains(t, payload.Launcher.ManualFallbackInstructions, "OpenClaw was detected as installed_unverified, but not as a verified terminal CLI launcher.")
	})

	t.Run("desktop only", func(t *testing.T) {
		repo := seedCommittedHyardRuntimeRepo(t)

		lockHyardProcessEnv(t)
		homeDir := configureHyardStartDetectionPath(t, map[string]string{})
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, "Applications", "Codex.app"), 0o750))

		stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--dry-run", "--json")
		require.NoError(t, err)
		require.Empty(t, stderr)

		payload := decodeHyardStartDryRunPayload(t, stdout)
		require.Equal(t, "resolved", payload.FrameworkResolution.Status)
		require.Equal(t, "codex", payload.FrameworkResolution.SelectedFramework)
		require.Equal(t, "project_detection", payload.FrameworkResolution.SelectionSource)
		require.Equal(t, "codex", payload.Launcher.Framework)
		require.Equal(t, "unverified", payload.Launcher.Status)
		require.Equal(t, "installed_desktop", payload.Launcher.DetectionStatus)
		require.False(t, payload.Launcher.TerminalCLIDetected)
		require.Contains(t, payload.Launcher.ManualFallbackInstructions, "Codex was detected as installed_desktop, but not as a verified terminal CLI launcher.")
	})
}

func TestHyardStartFailsClosedWithPromptFallbackForUnsupportedLaunchers(t *testing.T) {
	cases := []struct {
		name          string
		argument      string
		framework     string
		bootstrapPath string
	}{
		{
			name:          "OpenClaw",
			argument:      "openclaw",
			framework:     "openclaw",
			bootstrapPath: "skills",
		},
		{
			name:          "Claude Code",
			argument:      "claude-code",
			framework:     "claudecode",
			bootstrapPath: ".claude/skills",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedCommittedHyardRuntimeRepo(t)
			beforeStatus := repo.Run(t, "status", "--short")

			lockHyardProcessEnv(t)
			configureHyardStartDetectionPath(t, map[string]string{})

			stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--with", tc.argument)
			require.Error(t, err)
			require.ErrorContains(t, err, "cannot launch "+tc.framework+" interactively")
			require.Empty(t, stdout)

			require.Contains(t, stderr, "Harness Start cannot launch "+tc.framework+" interactively.")
			require.Contains(t, stderr, "launcher_status: unsupported")
			require.Contains(t, stderr, "launcher_detection_status: not_found")
			require.Contains(t, stderr, "terminal_cli_detected: false")
			require.Contains(t, stderr, "manual_next_action:")
			require.Contains(t, stderr, "hyard start --print-prompt")
			require.Contains(t, stderr, "usage:")
			require.Contains(t, stderr, "hyard start --dry-run --json")
			require.Contains(t, stderr, "Start Prompt")
			require.Contains(t, stderr, "First handle any pending Harness Runtime bootstrap work.")
			require.NotContains(t, stderr, "interactive_start: success")

			require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
			require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
			require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
			require.NoFileExists(t, filepath.Join(repo.Root, tc.bootstrapPath, harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
		})
	}
}

func TestHyardStartFallsBackWithPromptWhenCodexInteractiveLaunchFails(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	configureHyardStartDetectionPath(t, map[string]string{
		"codex": "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  printf '%s\\n' 'codex-cli 0.128.0'\n  exit 0\nfi\nexit 42\n",
	})

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--with", "codex")
	require.Error(t, err)
	require.ErrorContains(t, err, "launch codex")
	require.Empty(t, stdout)

	require.Contains(t, stderr, "Harness Start cannot launch codex interactively.")
	require.Contains(t, stderr, "framework_resolution: resolved")
	require.Contains(t, stderr, "selection_source: explicit_local")
	require.Contains(t, stderr, "framework: codex")
	require.Contains(t, stderr, "launcher_status: failed")
	require.Contains(t, stderr, "launcher_detection_status: installed_cli")
	require.Contains(t, stderr, "terminal_cli_detected: true")
	require.Contains(t, stderr, "manual_next_action: A terminal Codex CLI was detected, but Harness Start could not complete the interactive launch.")
	require.Contains(t, stderr, "usage:")
	require.Contains(t, stderr, "hyard start --print-prompt")
	require.Contains(t, stderr, "hyard start --dry-run --json")
	require.Contains(t, stderr, "Start Prompt")
	require.Contains(t, stderr, "First handle any pending Harness Runtime bootstrap work.")
	require.NotContains(t, stderr, "interactive_start: success")
}

func TestHyardStartFailsClosedBeforeMutationWhenCodexVersionCheckFails(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)
	beforeStatus := repo.Run(t, "status", "--short")

	lockHyardProcessEnv(t)
	configureHyardStartDetectionPath(t, map[string]string{
		"codex": "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  exit 7\nfi\nexit 0\n",
	})

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--with", "codex")
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot launch codex interactively")
	require.Empty(t, stdout)

	require.Contains(t, stderr, "Harness Start cannot launch codex interactively.")
	require.Contains(t, stderr, "launcher_status: unverified")
	require.Contains(t, stderr, "launcher_detection_status: installed_unverified")
	require.Contains(t, stderr, "terminal_cli_detected: false")
	require.Contains(t, stderr, "manual_next_action: Codex was detected as installed_unverified, but not as a verified terminal CLI launcher.")
	require.Contains(t, stderr, "Start Prompt")

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func TestHyardStartFailsClosedBeforeMutationWhenCodexIsMissing(t *testing.T) {
	repo := seedCommittedHyardRuntimeRepo(t)
	beforeStatus := repo.Run(t, "status", "--short")

	lockHyardProcessEnv(t)
	configureHyardStartDetectionPath(t, map[string]string{})

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "start", "--with", "codex")
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot launch codex interactively")
	require.Empty(t, stdout)

	require.Contains(t, stderr, "Harness Start cannot launch codex interactively.")
	require.Contains(t, stderr, "framework_resolution: resolved")
	require.Contains(t, stderr, "selection_source: explicit_local")
	require.Contains(t, stderr, "framework: codex")
	require.Contains(t, stderr, "launcher_status: unverified")
	require.Contains(t, stderr, "launcher_detection_status: not_found")
	require.Contains(t, stderr, "terminal_cli_detected: false")
	require.Contains(t, stderr, "manual_next_action: From the runtime root, start Codex manually.")
	require.Contains(t, stderr, "manual_next_action: Run `hyard start --print-prompt` and paste the Start Prompt into Codex.")
	require.Contains(t, stderr, "Start Prompt")

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
}

func configureHyardStartDetectionPath(t *testing.T, executables map[string]string) string {
	t.Helper()

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)

	binDir := t.TempDir()
	homeDir := t.TempDir()
	writeHyardStartExecutable(t, filepath.Join(binDir, "git"), "#!/bin/sh\nexec "+strconv.Quote(gitExecutable)+" \"$@\"\n")
	for name, contents := range executables {
		writeHyardStartExecutable(t, filepath.Join(binDir, name), contents)
	}

	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	return homeDir
}

func writeHyardStartExecutable(t *testing.T, path string, contents string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(contents), 0o700))
	require.NoError(t, os.Chmod(path, 0o700))
}
