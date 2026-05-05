package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	_, err := harnesspkg.WriteFrameworksFile(repo.Root, harnesspkg.FrameworksFile{
		SchemaVersion:        1,
		RecommendedFramework: "codex",
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed recommended framework", ".harness/agents/manifest.yaml")
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--dry-run", "--json")
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
