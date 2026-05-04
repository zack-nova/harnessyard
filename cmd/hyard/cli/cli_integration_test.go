package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesscli "github.com/zack-nova/harnessyard/cmd/harness/cli"
	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	hyardcli "github.com/zack-nova/harnessyard/cmd/hyard/cli"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

var hyardCLITestEnvMu sync.RWMutex

func lockHyardProcessEnv(t *testing.T) {
	t.Helper()

	hyardCLITestEnvMu.Lock()
	t.Cleanup(func() {
		hyardCLITestEnvMu.Unlock()
	})
}

func executeHyardCLI(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	hyardCLITestEnvMu.RLock()
	defer hyardCLITestEnvMu.RUnlock()

	return executeHyardCLIUnlocked(t, workingDir, args...)
}

func executeHyardCLIUnlocked(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := hyardcli.NewRootCommand()
	rootCmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	ctx := harnesscommands.WithWorkingDir(context.Background(), workingDir)
	ctx = orbitcommands.WithWorkingDir(ctx, workingDir)
	ctx = hyardcli.WithWorkingDir(ctx, workingDir)
	err := rootCmd.ExecuteContext(ctx)

	return stdout.String(), stderr.String(), err
}

func executeHyardCLIWithInput(t *testing.T, workingDir string, stdin string, args ...string) (string, string, error) {
	t.Helper()

	hyardCLITestEnvMu.RLock()
	defer hyardCLITestEnvMu.RUnlock()

	return executeHyardCLIWithInputUnlocked(t, workingDir, stdin, args...)
}

func executeHyardCLIWithInputUnlocked(t *testing.T, workingDir string, stdin string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := hyardcli.NewRootCommand()
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(stdin))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	ctx := harnesscommands.WithWorkingDir(context.Background(), workingDir)
	ctx = orbitcommands.WithWorkingDir(ctx, workingDir)
	ctx = hyardcli.WithWorkingDir(ctx, workingDir)
	err := rootCmd.ExecuteContext(ctx)

	return stdout.String(), stderr.String(), err
}

func executeHarnessCLIForHyardTest(t *testing.T, workingDir string, args ...string) error {
	t.Helper()

	hyardCLITestEnvMu.RLock()
	defer hyardCLITestEnvMu.RUnlock()

	rootCmd := harnesscli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	ctx := harnesscommands.WithWorkingDir(context.Background(), workingDir)
	ctx = orbitcommands.WithWorkingDir(ctx, workingDir)
	err := rootCmd.ExecuteContext(ctx)

	return err
}

func TestHyardHooksRunUsesUnifiedHookProtocol(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".harness", "agents"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "hooks", "allow-shell"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".harness", "agents", "config.yaml"), []byte(""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  entries:\n"+
		"    - id: allow-shell\n"+
		"      event:\n"+
		"        kind: tool.before\n"+
		"      match:\n"+
		"        tools: [shell]\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/allow-shell/run.sh\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "hooks", "allow-shell", "run.sh"), []byte("#!/bin/sh\nset -eu\ncat > hooks/allow-shell/captured.json\nprintf '{\"decision\":\"allow\"}\\n'\n"), 0o755))

	stdout, stderr, err := executeHyardCLIWithInput(t, repoRoot, `{"tool_name":"shell","command":"echo ok"}`, "hooks", "run", "--root", repoRoot, "--target", "codex", "--hook", "allow-shell")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `"decision":"allow"`)

	captured, err := os.ReadFile(filepath.Join(repoRoot, "hooks", "allow-shell", "captured.json"))
	require.NoError(t, err)
	require.Contains(t, string(captured), `"target": "codex"`)
	require.Contains(t, string(captured), `"hook": "allow-shell"`)
	require.Contains(t, string(captured), `"native_input"`)
}

func seedHyardCloneHarnessTemplateSourceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)

	err := executeHarnessCLIForHyardTest(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")

	err = executeHarnessCLIForHyardTest(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed hyard clone harness template source")

	err = executeHarnessCLIForHyardTest(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)

	return repo
}

func seedHyardSourceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")

	return repo
}

func seedCommittedHyardSourceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHyardSourceRepo(t)
	repo.AddAndCommit(t, "seed hyard source repo")

	return repo
}

func seedHyardSourceRepoWithDriftedBrief(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	agentsData, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Drifted docs orbit guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))
	repo.AddAndCommit(t, "seed hyard source repo with drifted brief")

	return repo
}

func seedHyardSourceRepoWithOutOfRangeSkill(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - declared-skills/*\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - extras/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "declared-skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "declared-skills/docs-style/checklist.md", "Use docs style guide.\n")
	repo.WriteFile(t, "extras/research-kit/SKILL.md", ""+
		"---\n"+
		"name: research-kit\n"+
		"description: Research kit references.\n"+
		"---\n"+
		"# Research Kit\n")
	repo.WriteFile(t, "extras/research-kit/playbook.md", "Use research kit.\n")
	repo.AddAndCommit(t, "seed hyard source repo with out-of-range skill")

	return repo
}

func seedCommittedHyardMemberHintSourceRepo(
	t *testing.T,
	members []orbitpkg.OrbitMember,
	files map[string]string,
) *testutil.Repo {
	t.Helper()

	repo := seedHyardSourceRepo(t)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	spec.Members = append([]orbitpkg.OrbitMember(nil), members...)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	for path, content := range files {
		repo.WriteFile(t, path, content)
	}

	repo.AddAndCommit(t, "seed hyard member hint source repo")

	return repo
}

func replaceHostedDocsOrbitBehaviorKey(t *testing.T, repo *testutil.Repo, replacement string) {
	t.Helper()

	path := filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	next := strings.Replace(string(data), "behavior:\n", replacement+":\n", 1)
	require.NotEqual(t, string(data), next)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", next)
}

func seedHyardRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	err := executeHarnessCLIForHyardTest(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")

	err = executeHarnessCLIForHyardTest(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	return repo
}

func seedCommittedHyardRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHyardRuntimeRepo(t)
	repo.AddAndCommit(t, "seed hyard runtime repo")

	return repo
}

func addHyardHostedOrbitDefinition(t *testing.T, repo *testutil.Repo, orbitID string) {
	t.Helper()

	repo.WriteFile(t, filepath.ToSlash(filepath.Join(".harness", "orbits", orbitID+".yaml")), fmt.Sprintf(""+
		"id: %s\n"+
		"description: %s orbit\n"+
		"include:\n"+
		"  - %s/**\n", orbitID, orbitID, orbitID))
	repo.WriteFile(t, filepath.ToSlash(filepath.Join(orbitID, "README.md")), fmt.Sprintf("%s orbit\n", orbitID))
}

func writeHyardHostedDocsOrbitWithStructuredBrief(t *testing.T, repoRoot string) {
	t.Helper()

	writeHyardHostedOrbitWithStructuredBrief(t, repoRoot, "docs", "Docs orbit guidance\n")
}

func writeHyardHostedOrbitWithStructuredBrief(t *testing.T, repoRoot string, orbitID string, agentsTemplate string) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
	require.NoError(t, err)
	spec.Description = orbitID + " orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = agentsTemplate
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
		orbitpkg.OrbitMemberRule,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repoRoot, spec)
	require.NoError(t, err)
}

func addHyardInstallOrbitMember(t *testing.T, repo *testutil.Repo, orbitID string) {
	t.Helper()

	appliedAt := time.Date(2026, time.April, 23, 9, 15, 0, 0, time.UTC)
	_, err := harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       orbitID,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/" + orbitID,
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
	})
	require.NoError(t, err)

	_, err = harnesspkg.UpsertInstallMember(context.Background(), repo.Root, orbitID, appliedAt)
	require.NoError(t, err)
}

func writeHyardBundleRecord(t *testing.T, repoRoot string, harnessID string, memberIDs []string) {
	t.Helper()

	_, err := harnesspkg.WriteBundleRecord(repoRoot, harnesspkg.BundleRecord{
		SchemaVersion: 1,
		HarnessID:     harnessID,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/" + harnessID,
			TemplateCommit: "abc123",
		},
		MemberIDs:          append([]string(nil), memberIDs...),
		AppliedAt:          time.Date(2026, time.April, 23, 9, 30, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{},
	})
	require.NoError(t, err)
}

func TestHyardHelpShowsUserLayerHeadlineAndPlumbingEntry(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Harness Yard CLI (hyard)")
	require.Contains(t, stdout, "create")
	require.Contains(t, stdout, "clone")
	require.Contains(t, stdout, "init")
	require.Contains(t, stdout, "install")
	require.Contains(t, stdout, "orbit")
	require.Contains(t, stdout, "publish")
	require.Contains(t, stdout, "assign")
	require.Contains(t, stdout, "unassign")
	require.Contains(t, stdout, "remove")
	require.Contains(t, stdout, "current")
	require.Contains(t, stdout, "enter")
	require.Contains(t, stdout, "status")
	require.Contains(t, stdout, "agent")
	require.Contains(t, stdout, "guide")
	require.Contains(t, stdout, "plumbing")
	require.NotContains(t, stdout, "framework")
}

func TestHyardAgentDetectReportsFakeCodexCLI(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "init", "runtime")
	require.NoError(t, err)
	require.Empty(t, stderr)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\necho 'codex 0.125.0'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "detect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SchemaVersion  string `json:"schema_version"`
		LocalSelection string `json:"local_selection"`
		Tools          []struct {
			Agent   string `json:"agent"`
			Summary struct {
				Status string `json:"status"`
				Ready  bool   `json:"ready"`
			} `json:"summary"`
			Components []struct {
				Component string `json:"component"`
				Status    string `json:"status"`
				Version   string `json:"version"`
			} `json:"components"`
		} `json:"tools"`
		SuggestedActions []struct {
			Command string `json:"command"`
			Reason  string `json:"reason"`
		} `json:"suggested_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Empty(t, payload.LocalSelection)

	var codex struct {
		Agent   string `json:"agent"`
		Summary struct {
			Status string `json:"status"`
			Ready  bool   `json:"ready"`
		} `json:"summary"`
		Components []struct {
			Component string `json:"component"`
			Status    string `json:"status"`
			Version   string `json:"version"`
		} `json:"components"`
	}
	for _, tool := range payload.Tools {
		if tool.Agent == "codex" {
			codex = tool
		}
	}
	require.Equal(t, "codex", codex.Agent)
	require.True(t, codex.Summary.Ready)
	require.Equal(t, "installed_cli", codex.Summary.Status)
	require.Contains(t, payload.SuggestedActions, struct {
		Command string `json:"command"`
		Reason  string `json:"reason"`
	}{
		Command: "hyard agent use codex",
		Reason:  "codex is the only ready detected agent",
	})
}

func TestHyardAgentDetectDeepReportsNPMPackage(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "init", "runtime")
	require.NoError(t, err)
	require.Empty(t, stderr)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	require.NoError(t, os.Symlink(gitExecutable, filepath.Join(binDir, "git")))
	npmPath := filepath.Join(binDir, "npm")
	require.NoError(t, os.WriteFile(npmPath, []byte("#!/bin/sh\nprintf '%s\\n' '{\"dependencies\":{\"@openai/codex\":{\"version\":\"0.125.0\"}}}'\n"), 0o700))
	require.NoError(t, os.Chmod(npmPath, 0o700))
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "detect", "--deep", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Tools []struct {
			Agent   string `json:"agent"`
			Summary struct {
				Status string `json:"status"`
				Ready  bool   `json:"ready"`
			} `json:"summary"`
			Components []struct {
				Component string `json:"component"`
				Status    string `json:"status"`
				Version   string `json:"version"`
			} `json:"components"`
		} `json:"tools"`
		SuggestedActions []struct {
			Command string `json:"command"`
		} `json:"suggested_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	var codex struct {
		Agent   string `json:"agent"`
		Summary struct {
			Status string `json:"status"`
			Ready  bool   `json:"ready"`
		} `json:"summary"`
		Components []struct {
			Component string `json:"component"`
			Status    string `json:"status"`
			Version   string `json:"version"`
		} `json:"components"`
	}
	for _, tool := range payload.Tools {
		if tool.Agent == "codex" {
			codex = tool
		}
	}
	require.Equal(t, "codex", codex.Agent)
	require.False(t, codex.Summary.Ready)
	require.Equal(t, "installed_unverified", codex.Summary.Status)
	require.Empty(t, payload.SuggestedActions)
	require.Contains(t, codex.Components, struct {
		Component string `json:"component"`
		Status    string `json:"status"`
		Version   string `json:"version"`
	}{
		Component: "package",
		Status:    "installed_unverified",
		Version:   "0.125.0",
	})
}

func TestHyardPlumbingHarnessFrameworkDetectUsesAgentDetector(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "init", "runtime")
	require.NoError(t, err)
	require.Empty(t, stderr)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	claudePath := filepath.Join(binDir, "claude")
	require.NoError(t, os.WriteFile(claudePath, []byte("#!/bin/sh\necho 'claude 1.2.3'\n"), 0o700))
	require.NoError(t, os.Chmod(claudePath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "plumbing", "harness", "framework", "detect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Tools []struct {
			Agent   string `json:"agent"`
			Summary struct {
				Status string `json:"status"`
				Ready  bool   `json:"ready"`
			} `json:"summary"`
		} `json:"tools"`
		SuggestedActions []struct {
			Command string `json:"command"`
		} `json:"suggested_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	var foundClaude bool
	for _, tool := range payload.Tools {
		if tool.Agent != "claudecode" {
			continue
		}
		foundClaude = true
		require.True(t, tool.Summary.Ready)
		require.Equal(t, "installed_cli", tool.Summary.Status)
	}
	require.True(t, foundClaude)
	require.Contains(t, payload.SuggestedActions, struct {
		Command string `json:"command"`
	}{
		Command: "hyard agent use claudecode",
	})
}

func TestHyardAgentUseAcceptsDetectorFacingClaudeCodeID(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "agent", "use", "claudecode")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "selected framework claude")

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "claude", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceExplicitLocal, selection.SelectionSource)
}

func TestHyardOrbitHelpShowsCanonicalAuthoringSubcommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "orbit", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "create")
	require.Contains(t, stdout, "list")
	require.Contains(t, stdout, "show")
	require.Contains(t, stdout, "files")
	require.Contains(t, stdout, "set")
	require.Contains(t, stdout, "validate")
	require.Contains(t, stdout, "prepare")
	require.Contains(t, stdout, "checkpoint")
	require.Contains(t, stdout, "content")
	require.Contains(t, stdout, "member")
	require.Contains(t, stdout, "skill")
	require.NotContains(t, stdout, "template")
	require.NotContains(t, stdout, "bindings")
	require.NotContains(t, stdout, "branch")
}

func TestHyardOrbitMemberHelpShowsApplyWithoutRawDetectOrBackfill(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "orbit", "member", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "add")
	require.Contains(t, stdout, "apply")
	require.Contains(t, stdout, "remove")
	require.NotContains(t, stdout, "detect")
	require.NotContains(t, stdout, "backfill")
}

func TestHyardOrbitContentHelpShowsApplyWithoutMemberVocabulary(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "orbit", "content", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "apply")
	require.NotContains(t, stdout, "member")
	require.NotContains(t, stdout, "backfill")
}

func TestHyardOrbitSkillLinkInspectAndUnlink(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(
		t,
		repo.Root,
		"orbit", "skill", "link", "github://acme/review-skill",
		"--orbit", "docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	var linkPayload struct {
		OrbitID    string `json:"orbit"`
		URI        string `json:"uri"`
		Required   bool   `json:"required"`
		Dependency struct {
			URI      string `json:"uri"`
			Required bool   `json:"required"`
		} `json:"dependency"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &linkPayload))
	require.Equal(t, "docs", linkPayload.OrbitID)
	require.Equal(t, "github://acme/review-skill", linkPayload.URI)
	require.False(t, linkPayload.Required)
	require.Equal(t, "github://acme/review-skill", linkPayload.Dependency.URI)
	require.False(t, linkPayload.Dependency.Required)

	_, stderr, err = executeHyardCLI(
		t,
		repo.Root,
		"orbit", "skill", "link", "https://example.com/skills/release-gate",
		"--orbit", "docs",
		"--required",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	inspectStdout, inspectStderr, err := executeHyardCLI(t, repo.Root, "orbit", "skill", "inspect", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, inspectStderr)
	var inspectPayload struct {
		OrbitID            string `json:"orbit"`
		RemoteDependencies []struct {
			URI      string `json:"uri"`
			Required bool   `json:"required"`
			Strength string `json:"strength"`
		} `json:"remote_dependencies"`
	}
	require.NoError(t, json.Unmarshal([]byte(inspectStdout), &inspectPayload))
	require.Equal(t, "docs", inspectPayload.OrbitID)
	require.Contains(t, inspectPayload.RemoteDependencies, struct {
		URI      string `json:"uri"`
		Required bool   `json:"required"`
		Strength string `json:"strength"`
	}{URI: "github://acme/review-skill", Required: false, Strength: "recommended"})
	require.Contains(t, inspectPayload.RemoteDependencies, struct {
		URI      string `json:"uri"`
		Required bool   `json:"required"`
		Strength string `json:"strength"`
	}{URI: "https://example.com/skills/release-gate", Required: true, Strength: "required"})

	_, stderr, err = executeHyardCLI(
		t,
		repo.Root,
		"orbit", "skill", "unlink", "github://acme/review-skill",
		"--orbit", "docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "github://acme/review-skill")
	require.Contains(t, string(definitionData), "uri: https://example.com/skills/release-gate")
	require.Contains(t, string(definitionData), "required: true")
}

func TestHyardPublishHelpShowsCanonicalSubcommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "publish", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "orbit")
	require.Contains(t, stdout, "harness")
	require.NotContains(t, stdout, "save")
}

func TestHyardCreateHelpShowsRevisionKinds(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "create", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "runtime")
	require.Contains(t, stdout, "source")
	require.Contains(t, stdout, "orbit-template")
}

func TestHyardInitHelpShowsRevisionKinds(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "init", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "runtime")
	require.Contains(t, stdout, "source")
	require.Contains(t, stdout, "orbit-template")
}

func TestHyardCreateLegacyRuntimeShorthandFailsClosed(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "create", "demo-repo")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "hyard create <path> is no longer supported")
	require.ErrorContains(t, err, "hyard create runtime demo-repo")
}

func TestHyardInitLegacyRuntimeShorthandFailsClosed(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "init")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "hyard init no longer defaults to runtime")
	require.ErrorContains(t, err, "hyard init runtime")
}

func TestHyardAssignHelpShowsOrbitSubcommand(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "assign", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "orbit")
}

func TestHyardUnassignHelpShowsOrbitSubcommand(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "unassign", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "orbit")
}

func TestHyardRemoveHelpShowsPackageDisambiguationSubcommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "remove", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "orbit")
	require.Contains(t, stdout, "harness")
	require.Contains(t, stdout, "--yes")
	require.Contains(t, stdout, "--dry-run")
}

func TestHyardAssignOrbitAssignsStandaloneManualMemberToExistingHarness(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))

	repo := &testutil.Repo{Root: clonePayload.HarnessRoot}
	addHyardHostedOrbitDefinition(t, repo, "api")
	require.NoError(t, executeHarnessCLIForHyardTest(t, repo.Root, "add", "api"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "assign", "orbit", "api", "--harness", clonePayload.HarnessID, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot            string `json:"harness_root"`
		OrbitID                string `json:"orbit_id"`
		HarnessID              string `json:"harness_id"`
		Source                 string `json:"source"`
		PreviousOwnerHarnessID string `json:"previous_owner_harness_id"`
		OwnerHarnessID         string `json:"owner_harness_id"`
		ManifestPath           string `json:"manifest_path"`
		MemberCount            int    `json:"member_count"`
		Changed                bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "api", payload.OrbitID)
	require.Equal(t, clonePayload.HarnessID, payload.HarnessID)
	require.Equal(t, harnesspkg.MemberSourceManual, payload.Source)
	require.Empty(t, payload.PreviousOwnerHarnessID)
	require.Equal(t, clonePayload.HarnessID, payload.OwnerHarnessID)
	require.NotEmpty(t, payload.ManifestPath)
	require.Equal(t, 2, payload.MemberCount)
	require.True(t, payload.Changed)

	resolved, err := harnesspkg.ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Len(t, resolved.Runtime.Members, 2)
	found := false
	for _, member := range resolved.Runtime.Members {
		if member.OrbitID != "api" {
			continue
		}
		require.Equal(t, harnesspkg.MemberSourceManual, member.Source)
		require.Equal(t, clonePayload.HarnessID, member.OwnerHarnessID)
		found = true
	}
	require.True(t, found)
}

func TestHyardAssignOrbitInfersSingleInstalledHarnessPackage(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))

	repo := &testutil.Repo{Root: clonePayload.HarnessRoot}
	addHyardHostedOrbitDefinition(t, repo, "api")
	require.NoError(t, executeHarnessCLIForHyardTest(t, repo.Root, "add", "api"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "assign", "orbit", "api", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitPackage           string `json:"orbit_package"`
		HarnessPackage         string `json:"harness_package"`
		HarnessID              string `json:"harness_id"`
		OwnerHarnessID         string `json:"owner_harness_id"`
		PreviousOwnerHarnessID string `json:"previous_owner_harness_id"`
		Changed                bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "api", payload.OrbitPackage)
	require.Equal(t, clonePayload.HarnessID, payload.HarnessPackage)
	require.Equal(t, clonePayload.HarnessID, payload.HarnessID)
	require.Equal(t, clonePayload.HarnessID, payload.OwnerHarnessID)
	require.Empty(t, payload.PreviousOwnerHarnessID)
	require.True(t, payload.Changed)
}

func TestHyardAssignOrbitInfersCurrentRuntimeCompositionWithoutInstalledPackage(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "init", "runtime", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	addHyardHostedOrbitDefinition(t, repo, "execute")
	require.NoError(t, executeHarnessCLIForHyardTest(t, repo.Root, "add", "execute"))

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.NotEmpty(t, runtimeFile.Harness.ID)

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "assign", "orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitPackage   string `json:"orbit_package"`
		HarnessPackage string `json:"harness_package"`
		HarnessID      string `json:"harness_id"`
		OwnerHarnessID string `json:"owner_harness_id"`
		Changed        bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "execute", payload.OrbitPackage)
	require.Equal(t, runtimeFile.Harness.ID, payload.HarnessPackage)
	require.Equal(t, runtimeFile.Harness.ID, payload.HarnessID)
	require.Equal(t, runtimeFile.Harness.ID, payload.OwnerHarnessID)
	require.True(t, payload.Changed)
}

func TestHyardAssignOrbitRequiresHarnessPackageWhenMultipleAreInstalled(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))
	writeHyardBundleRecord(t, clonePayload.HarnessRoot, "archive", []string{"ghost"})

	stdout, stderr, err = executeHyardCLI(t, clonePayload.HarnessRoot, "assign", "orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "multiple harness packages")
	require.ErrorContains(t, err, "--harness <harness-package>")
}

func TestHyardAssignOrbitPreservesInstallOrbitProvenance(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))

	repo := &testutil.Repo{Root: clonePayload.HarnessRoot}
	addHyardHostedOrbitDefinition(t, repo, "api")
	addHyardInstallOrbitMember(t, repo, "api")

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "assign", "orbit", "api", "--harness", clonePayload.HarnessID, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var assignPayload struct {
		OrbitID        string `json:"orbit_id"`
		HarnessID      string `json:"harness_id"`
		Source         string `json:"source"`
		OwnerHarnessID string `json:"owner_harness_id"`
		Changed        bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &assignPayload))
	require.Equal(t, "api", assignPayload.OrbitID)
	require.Equal(t, clonePayload.HarnessID, assignPayload.HarnessID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, assignPayload.Source)
	require.Equal(t, clonePayload.HarnessID, assignPayload.OwnerHarnessID)
	require.True(t, assignPayload.Changed)

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "unassign", "orbit", "api", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var unassignPayload struct {
		OrbitID                string `json:"orbit_id"`
		SourceBefore           string `json:"source_before"`
		SourceAfter            string `json:"source_after"`
		PreviousOwnerHarnessID string `json:"previous_owner_harness_id"`
		OwnerHarnessID         string `json:"owner_harness_id"`
		Changed                bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &unassignPayload))
	require.Equal(t, "api", unassignPayload.OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, unassignPayload.SourceBefore)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, unassignPayload.SourceAfter)
	require.Equal(t, clonePayload.HarnessID, unassignPayload.PreviousOwnerHarnessID)
	require.Empty(t, unassignPayload.OwnerHarnessID)
	require.True(t, unassignPayload.Changed)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "api")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallRecordStatusActive, orbittemplate.EffectiveInstallRecordStatus(record))

	resolved, err := harnesspkg.ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	found := false
	for _, member := range resolved.Runtime.Members {
		if member.OrbitID != "api" {
			continue
		}
		require.Equal(t, harnesspkg.MemberSourceInstallOrbit, member.Source)
		require.Empty(t, member.OwnerHarnessID)
		found = true
	}
	require.True(t, found)
}

func TestHyardAssignOrbitAllowsNoOpWhenAlreadyAssignedToTargetHarness(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))

	stdout, stderr, err = executeHyardCLI(t, clonePayload.HarnessRoot, "assign", "orbit", "docs", "--harness", clonePayload.HarnessID, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID        string `json:"orbit_id"`
		HarnessID      string `json:"harness_id"`
		Source         string `json:"source"`
		OwnerHarnessID string `json:"owner_harness_id"`
		Changed        bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, clonePayload.HarnessID, payload.HarnessID)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, payload.Source)
	require.Equal(t, clonePayload.HarnessID, payload.OwnerHarnessID)
	require.False(t, payload.Changed)
}

func TestHyardAssignOrbitRejectsDifferentExistingHarnessOwner(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))
	writeHyardBundleRecord(t, clonePayload.HarnessRoot, "archive", []string{"ghost"})

	stdout, stderr, err = executeHyardCLI(t, clonePayload.HarnessRoot, "assign", "orbit", "docs", "--harness", "archive", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" is currently assigned to harness "`+clonePayload.HarnessID+`"`)
	require.ErrorContains(t, err, `unassign`)
}

func TestHyardUnassignOrbitDetachesBundleBackedMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var clonePayload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &clonePayload))

	stdout, stderr, err = executeHyardCLI(t, clonePayload.HarnessRoot, "unassign", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID                string   `json:"orbit_id"`
		SourceBefore           string   `json:"source_before"`
		SourceAfter            string   `json:"source_after"`
		PreviousOwnerHarnessID string   `json:"previous_owner_harness_id"`
		OwnerHarnessID         string   `json:"owner_harness_id"`
		Changed                bool     `json:"changed"`
		RemovedPaths           []string `json:"removed_paths"`
		RemovedAgentsBlock     bool     `json:"removed_agents_block"`
		DeletedBundleRecord    bool     `json:"deleted_bundle_record"`
		MemberCount            int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, payload.SourceBefore)
	require.Equal(t, harnesspkg.MemberSourceManual, payload.SourceAfter)
	require.Equal(t, clonePayload.HarnessID, payload.PreviousOwnerHarnessID)
	require.Empty(t, payload.OwnerHarnessID)
	require.True(t, payload.Changed)
	require.False(t, payload.RemovedAgentsBlock)
	require.True(t, payload.DeletedBundleRecord)
	require.Equal(t, 1, payload.MemberCount)

	resolved, err := harnesspkg.ResolveRoot(context.Background(), clonePayload.HarnessRoot)
	require.NoError(t, err)
	require.Len(t, resolved.Runtime.Members, 1)
	require.Equal(t, harnesspkg.MemberSourceManual, resolved.Runtime.Members[0].Source)
	require.Empty(t, resolved.Runtime.Members[0].OwnerHarnessID)

	_, err = harnesspkg.LoadBundleRecord(clonePayload.HarnessRoot, clonePayload.HarnessID)
	require.Error(t, err)
	require.ErrorContains(t, err, "read")
}

func TestHyardUnassignOrbitRejectsStandaloneMember(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "unassign", "orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" is already standalone`)
}

func TestHyardPlumbingHelpShowsCompatibilityTrees(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "plumbing", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "orbit")
	require.Contains(t, stdout, "harness")
}

func TestHyardCreateDelegatesToHarnessCreate(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	stdout, stderr, err := executeHyardCLI(t, parentDir, "create", "runtime", "demo", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string `json:"harness_root"`
		ManifestPath string `json:"manifest_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "demo")), gitpkg.ComparablePath(payload.HarnessRoot))
	_, err = os.Stat(payload.ManifestPath)
	require.NoError(t, err)
}

func TestHyardCreateSourceDelegatesToOrbitSourceCreate(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	targetPath := filepath.Join(parentDir, "source-authoring")

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"create",
		"source",
		targetPath,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs source branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string `json:"repo_root"`
		SourceManifestPath string `json:"source_manifest_path"`
		SourceBranch       string `json:"source_branch"`
		Package            struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		GitInitialized bool `json:"git_initialized"`
		Changed        bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, gitpkg.ComparablePath(targetPath), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(targetPath, ".harness", "manifest.yaml")), gitpkg.ComparablePath(payload.SourceManifestPath))
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.NotEmpty(t, payload.SourceBranch)
	require.True(t, payload.GitInitialized)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: source\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")

	specData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(specData), "package:\n")
	require.Contains(t, string(specData), "type: orbit\n")
	require.Contains(t, string(specData), "name: docs\n")
	require.NotContains(t, string(specData), "id: docs\n")

	textTargetPath := filepath.Join(parentDir, "source-authoring-text")
	textStdout, textStderr, err := executeHyardCLI(
		t,
		parentDir,
		"create",
		"source",
		textTargetPath,
		"--orbit",
		"notes",
	)
	require.NoError(t, err)
	require.Empty(t, textStderr)
	require.Contains(t, textStdout, "package: notes\n")
	require.Contains(t, textStdout, "package_type: orbit\n")
	require.NotContains(t, textStdout, "publish_orbit_id")
	require.NotContains(t, textStdout, "orbit_id")
}

func TestHyardCreateOrbitTemplateDelegatesToOrbitTemplateCreate(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	targetPath := filepath.Join(parentDir, "template-authoring")

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"create",
		"orbit-template",
		targetPath,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs template branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		ManifestPath  string `json:"manifest_path"`
		CurrentBranch string `json:"current_branch"`
		Package       struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		GitInitialized bool `json:"git_initialized"`
		Changed        bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	require.Equal(t, gitpkg.ComparablePath(targetPath), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(targetPath, ".harness", "manifest.yaml")), gitpkg.ComparablePath(payload.ManifestPath))
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.NotEmpty(t, payload.CurrentBranch)
	require.True(t, payload.GitInitialized)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
}

func TestHyardInitRuntimeDelegatesToHarnessInit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "init", "runtime", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot      string `json:"harness_root"`
		ManifestPath     string `json:"manifest_path"`
		ManifestCreated  bool   `json:"manifest_created"`
		OrbitsDirCreated bool   `json:"orbits_dir_created"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(payload.HarnessRoot))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(repo.Root, ".harness", "manifest.yaml")), gitpkg.ComparablePath(payload.ManifestPath))
	require.True(t, payload.ManifestCreated)
	require.True(t, payload.OrbitsDirCreated)
}

func TestHyardInitSourceDelegatesToOrbitSourceInit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeHyardCLI(
		t,
		repo.Root,
		"init",
		"source",
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs source branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string `json:"repo_root"`
		SourceManifestPath string `json:"source_manifest_path"`
		SourceBranch       string `json:"source_branch"`
		Package            struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(repo.Root, ".harness", "manifest.yaml")), gitpkg.ComparablePath(payload.SourceManifestPath))
	require.Equal(t, "main", payload.SourceBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
}

func TestHyardInitSourceDefaultsToOnlyHostedOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	_, _, err := executeHyardCLI(t, repo.Root, "orbit", "create", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "init", "source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SourceBranch string `json:"source_branch"`
		Package      struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, "main", payload.SourceBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
}

func TestHyardOrbitRenameUpdatesHostedPackageTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	_, _, err := executeHyardCLI(t, repo.Root, "init", "source", "--orbit", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "rename", "docs", "api", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot          string `json:"repo_root"`
		OldPackage        string `json:"old_package"`
		NewPackage        string `json:"new_package"`
		OldDefinitionPath string `json:"old_definition_path"`
		NewDefinitionPath string `json:"new_definition_path"`
		ManifestPath      string `json:"manifest_path"`
		ManifestChanged   bool   `json:"manifest_changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, "docs", payload.OldPackage)
	require.Equal(t, "api", payload.NewPackage)
	require.Equal(t, ".harness/orbits/docs.yaml", payload.OldDefinitionPath)
	require.Equal(t, ".harness/orbits/api.yaml", payload.NewDefinitionPath)
	require.Equal(t, ".harness/manifest.yaml", payload.ManifestPath)
	require.True(t, payload.ManifestChanged)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "api")
	require.NoError(t, err)
	require.NotNil(t, spec.Package)
	require.Equal(t, "api", spec.Package.Name)
	require.Equal(t, ".harness/orbits/api.yaml", spec.Meta.File)

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "name: api\n")
	require.NotContains(t, string(manifestData), "name: docs\n")

	listStdout, listStderr, err := executeHyardCLI(t, repo.Root, "orbit", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, listStderr)
	var listPayload struct {
		Orbits []struct {
			ID string `json:"id"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(listStdout), &listPayload))
	require.Equal(t, []struct {
		ID string `json:"id"`
	}{{ID: "api"}}, listPayload.Orbits)
}

func TestHyardInitOrbitTemplateDelegatesToOrbitTemplateInit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeHyardCLI(
		t,
		repo.Root,
		"init",
		"orbit-template",
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs template branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		ManifestPath  string `json:"manifest_path"`
		CurrentBranch string `json:"current_branch"`
		Package       struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(repo.Root, ".harness", "manifest.yaml")), gitpkg.ComparablePath(payload.ManifestPath))
	require.Equal(t, "main", payload.CurrentBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)
}

func TestHyardInitOrbitTemplateDefaultsToOnlyHostedOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	_, _, err := executeHyardCLI(t, repo.Root, "orbit", "create", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "init", "orbit-template", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		CurrentBranch string `json:"current_branch"`
		Package       struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	require.Equal(t, "main", payload.CurrentBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
}

func TestHyardCurrentDelegatesToOrbitCurrent(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	_, _, err := executeHyardCLI(t, parentDir, "create", "runtime", "demo", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, filepath.Join(parentDir, "demo"), "current", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot string          `json:"repo_root"`
		Current  json.RawMessage `json:"current"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "demo")), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, "null", string(payload.Current))
}

func TestHyardOrbitCreateDelegatesToOrbitCreate(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "create", "api", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot string `json:"repo_root"`
		File     string `json:"file"`
		Orbit    struct {
			ID string `json:"id"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(payload.RepoRoot))
	require.Equal(t, "api", payload.Orbit.ID)
	_, err = os.Stat(payload.File)
	require.NoError(t, err)
}

func TestHyardOrbitCreateActivatesRuntimeMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "init", "runtime", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "orbit", "create", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var createPayload struct {
		RepoRoot string `json:"repo_root"`
		File     string `json:"file"`
		Orbit    struct {
			ID string `json:"id"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &createPayload))
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(createPayload.RepoRoot))
	require.Equal(t, "execute", createPayload.Orbit.ID)
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml")), gitpkg.ComparablePath(createPayload.File))

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "execute", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceManual, runtimeFile.Members[0].Source)
	require.Empty(t, runtimeFile.Members[0].OwnerHarnessID)

	listStdout, listStderr, err := executeHyardCLI(t, repo.Root, "orbit", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, listStderr)
	var listPayload struct {
		Orbits []struct {
			ID string `json:"id"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(listStdout), &listPayload))
	require.Len(t, listPayload.Orbits, 1)
	require.Equal(t, "execute", listPayload.Orbits[0].ID)

	showStdout, showStderr, err := executeHyardCLI(t, repo.Root, "orbit", "show", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)
	var showPayload struct {
		Orbit struct {
			ID string `json:"id"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(showStdout), &showPayload))
	require.Equal(t, "execute", showPayload.Orbit.ID)

	assignStdout, assignStderr, err := executeHyardCLI(t, repo.Root, "assign", "orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, assignStderr)
	var assignPayload struct {
		OrbitPackage   string `json:"orbit_package"`
		HarnessPackage string `json:"harness_package"`
		OwnerHarnessID string `json:"owner_harness_id"`
		Changed        bool   `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(assignStdout), &assignPayload))
	require.Equal(t, "execute", assignPayload.OrbitPackage)
	require.Equal(t, runtimeFile.Harness.ID, assignPayload.HarnessPackage)
	require.Equal(t, runtimeFile.Harness.ID, assignPayload.OwnerHarnessID)
	require.True(t, assignPayload.Changed)
}

func TestHyardOrbitPublicPathCoversAuthoringHappyPath(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/service.md": "Docs service guide\n",
	})

	memberStdout, memberStderr, err := executeHyardCLI(
		t,
		repo.Root,
		"orbit",
		"member",
		"add",
		"--orbit",
		"docs",
		"--name",
		"docs-service",
		"--role",
		"rule",
		"--include",
		"docs/service.md",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, memberStderr)

	var memberPayload struct {
		RepoRoot string `json:"repo_root"`
		Member   struct {
			Name  string `json:"name"`
			Role  string `json:"role"`
			Paths struct {
				Include []string `json:"include"`
			} `json:"paths"`
		} `json:"member"`
	}
	require.NoError(t, json.Unmarshal([]byte(memberStdout), &memberPayload))
	require.Equal(t, gitpkg.ComparablePath(repo.Root), gitpkg.ComparablePath(memberPayload.RepoRoot))
	require.Equal(t, "docs-service", memberPayload.Member.Name)
	require.Equal(t, "rule", memberPayload.Member.Role)
	require.Equal(t, []string{"docs/service.md"}, memberPayload.Member.Paths.Include)

	setStdout, setStderr, err := executeHyardCLI(
		t,
		repo.Root,
		"orbit",
		"set",
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs scope",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, setStderr)

	var setPayload struct {
		Orbit struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(setStdout), &setPayload))
	require.Equal(t, "docs", setPayload.Orbit.ID)
	require.Equal(t, "Docs Orbit", setPayload.Orbit.Name)
	require.Equal(t, "Docs scope", setPayload.Orbit.Description)

	showStdout, showStderr, err := executeHyardCLI(t, repo.Root, "orbit", "show", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)

	var showPayload struct {
		Orbit struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Members     []struct {
				Name string `json:"name"`
			} `json:"members"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(showStdout), &showPayload))
	require.Equal(t, "docs", showPayload.Orbit.ID)
	require.Equal(t, "Docs Orbit", showPayload.Orbit.Name)
	require.Equal(t, "Docs scope", showPayload.Orbit.Description)
	showMemberNames := make([]string, 0, len(showPayload.Orbit.Members))
	for _, member := range showPayload.Orbit.Members {
		showMemberNames = append(showMemberNames, member.Name)
	}
	require.Contains(t, showMemberNames, "docs-service")

	listStdout, listStderr, err := executeHyardCLI(t, repo.Root, "orbit", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, listStderr)

	var listPayload struct {
		Orbits []struct {
			ID string `json:"id"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(listStdout), &listPayload))
	require.Len(t, listPayload.Orbits, 1)
	require.Equal(t, "docs", listPayload.Orbits[0].ID)

	filesStdout, filesStderr, err := executeHyardCLI(t, repo.Root, "orbit", "files", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, filesStderr)

	var filesPayload struct {
		Orbit string   `json:"orbit"`
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(filesStdout), &filesPayload))
	require.Equal(t, "docs", filesPayload.Orbit)
	require.Contains(t, filesPayload.Files, "docs/service.md")

	validateStdout, validateStderr, err := executeHyardCLI(t, repo.Root, "orbit", "validate", "--json")
	require.NoError(t, err)
	require.Empty(t, validateStderr)

	var validatePayload struct {
		Valid  bool `json:"valid"`
		Orbits []struct {
			ID string `json:"id"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(validateStdout), &validatePayload))
	require.True(t, validatePayload.Valid)
	require.Len(t, validatePayload.Orbits, 1)
	require.Equal(t, "docs", validatePayload.Orbits[0].ID)

	removeStdout, removeStderr, err := executeHyardCLI(
		t,
		repo.Root,
		"orbit",
		"member",
		"remove",
		"--orbit",
		"docs",
		"--name",
		"docs-service",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, removeStderr)

	var removePayload struct {
		OrbitID string `json:"orbit_id"`
		Name    string `json:"name"`
	}
	require.NoError(t, json.Unmarshal([]byte(removeStdout), &removePayload))
	require.Equal(t, "docs", removePayload.OrbitID)
	require.Equal(t, "docs-service", removePayload.Name)
}

func TestHyardGuideRenderDefaultsToApplicableGuidanceTargets(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "render", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target string `json:"target"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)

	statusByTarget := make(map[string]string, len(payload.Artifacts))
	reasonByTarget := make(map[string]string, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		statusByTarget[artifact.Target] = artifact.Status
		reasonByTarget[artifact.Target] = artifact.Reason
	}
	require.Equal(t, "rendered", statusByTarget["agents"])
	require.Equal(t, "authored_truth", reasonByTarget["agents"])
	require.Equal(t, "skipped_no_authored_truth", statusByTarget["humans"])
	require.Equal(t, "no_authored_truth", reasonByTarget["humans"])
	require.Equal(t, "skipped_no_authored_truth", statusByTarget["bootstrap"])
	require.Equal(t, "no_authored_truth", reasonByTarget["bootstrap"])
}

func TestHyardGuideRenderDriftSuggestsPublicSaveCommand(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepoWithDriftedBrief(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "render", "--orbit", "docs", "--target", "agents")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md already has local edits in orbit block "docs"`)
	require.ErrorContains(t, err, `hyard guide save --orbit docs --target agents`)
	require.ErrorContains(t, err, `hyard guide render --orbit docs --target agents --force`)
	require.NotContains(t, err.Error(), "orbit brief backfill")
}

func TestHyardGuideRenderDefaultsToAllApplicableOrbits(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	writeHyardHostedOrbitWithStructuredBrief(t, repo.Root, "api", "API orbit guidance\n")
	repo.WriteFile(t, "api/service.md", "API service\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "render", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target        string `json:"target"`
		OrbitCount    int    `json:"orbit_count"`
		ArtifactCount int    `json:"artifact_count"`
		Orbits        []struct {
			OrbitID   string `json:"orbit_id"`
			Artifacts []struct {
				Target string `json:"target"`
				Status string `json:"status"`
			} `json:"artifacts"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 2, payload.OrbitCount)
	require.Equal(t, 6, payload.ArtifactCount)

	renderedAgents := map[string]bool{}
	for _, orbit := range payload.Orbits {
		for _, artifact := range orbit.Artifacts {
			if artifact.Target == "agents" && artifact.Status == "rendered" {
				renderedAgents[orbit.OrbitID] = true
			}
		}
	}
	require.Equal(t, map[string]bool{"api": true, "docs": true}, renderedAgents)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Docs orbit guidance\n")
	require.Contains(t, string(agentsData), "API orbit guidance\n")
}

func TestHyardGuideWritebackDefaultsToAllRootOrbitBlocks(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	writeHyardHostedOrbitWithStructuredBrief(t, repo.Root, "api", "API orbit guidance\n")

	docsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance\n"))
	require.NoError(t, err)
	apiBlock, err := orbittemplate.WrapRuntimeAgentsBlock("api", []byte("Edited API guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(docsBlock)+string(apiBlock))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "writeback", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target        string `json:"target"`
		OrbitCount    int    `json:"orbit_count"`
		ArtifactCount int    `json:"artifact_count"`
		Orbits        []struct {
			OrbitID       string `json:"orbit_id"`
			ArtifactCount int    `json:"artifact_count"`
			Artifacts     []struct {
				Target string `json:"target"`
				Status string `json:"status"`
			} `json:"artifacts"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 2, payload.OrbitCount)
	require.Equal(t, 2, payload.ArtifactCount)

	statusByOrbit := make(map[string]string, len(payload.Orbits))
	for _, orbit := range payload.Orbits {
		require.Equal(t, 1, orbit.ArtifactCount)
		require.Len(t, orbit.Artifacts, 1)
		require.Equal(t, "agents", orbit.Artifacts[0].Target)
		statusByOrbit[orbit.OrbitID] = orbit.Artifacts[0].Status
	}
	require.Equal(t, map[string]string{"api": "updated", "docs": "updated"}, statusByOrbit)

	docsSpec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "Edited docs guidance\n", docsSpec.Meta.AgentsTemplate)
	apiSpec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "api")
	require.NoError(t, err)
	require.Equal(t, "Edited API guidance\n", apiSpec.Meta.AgentsTemplate)
}

func TestHyardGuideWritebackExplicitAllSkipsMissingBlocksWithoutHostedTruth(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	writeHyardHostedOrbitWithStructuredBrief(t, repo.Root, "execute", "")

	block, err := orbittemplate.WrapRuntimeAgentsBlock("execute", []byte("Edited execute guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(block))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "writeback", "--orbit", "execute", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target string `json:"target"`
			Status string `json:"status"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Len(t, payload.Artifacts, 3)

	statusByTarget := make(map[string]string, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		statusByTarget[artifact.Target] = artifact.Status
	}
	require.Equal(t, "skipped", statusByTarget["agents"])
	require.Equal(t, "updated", statusByTarget["humans"])
	require.Equal(t, "skipped", statusByTarget["bootstrap"])

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "execute")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Empty(t, spec.Meta.AgentsTemplate)
	require.Equal(t, "Edited execute guidance\n", spec.Meta.HumansTemplate)
	require.Empty(t, spec.Meta.BootstrapTemplate)
}

func TestHyardGuideWritebackRejectsUnknownOrbitBlockBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)

	docsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance\n"))
	require.NoError(t, err)
	unknownBlock, err := orbittemplate.WrapRuntimeAgentsBlock("api", []byte("Unknown API guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(docsBlock)+string(unknownBlock))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "writeback", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md contains orbit block "api", but that orbit is not in the current guidance scope`)

	docsSpec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "Docs orbit guidance\n", docsSpec.Meta.AgentsTemplate)
}

func TestHyardGuideSyncCanFilterToOneOrbit(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	writeHyardHostedOrbitWithStructuredBrief(t, repo.Root, "api", "API orbit guidance\n")
	addHyardInstallOrbitMember(t, repo, "api")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "sync", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		MemberCount int `json:"member_count"`
		Artifacts   []struct {
			Target         string   `json:"target"`
			ComposedOrbits []string `json:"composed_orbits"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.MemberCount)
	require.NotEmpty(t, payload.Artifacts)
	for _, artifact := range payload.Artifacts {
		if artifact.Target == "agents" {
			require.Equal(t, []string{"docs"}, artifact.ComposedOrbits)
		}
	}
}

func TestHyardOrbitMemberApplyCheckDelegatesToMemberHintPreflight(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, nil)
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  description: Review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n")

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "member", "apply", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot        string `json:"repo_root"`
		OrbitID         string `json:"orbit_id"`
		RevisionKind    string `json:"revision_kind"`
		DriftDetected   bool   `json:"drift_detected"`
		BackfillAllowed bool   `json:"backfill_allowed"`
		HintCount       int    `json:"hint_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Equal(t, 1, payload.HintCount)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestHyardOrbitContentApplyCheckDelegatesToMemberHintPreflight(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, nil)
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  description: Review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n")

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "content", "apply", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot        string `json:"repo_root"`
		OrbitID         string `json:"orbit_id"`
		RevisionKind    string `json:"revision_kind"`
		DriftDetected   bool   `json:"drift_detected"`
		BackfillAllowed bool   `json:"backfill_allowed"`
		HintCount       int    `json:"hint_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Equal(t, 1, payload.HintCount)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestHyardOrbitMemberApplyWritesTruthAndConsumesHints(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, []orbitpkg.OrbitMember{
		{
			Name:        "docs-rules",
			Description: "Old rules",
			Role:        orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/style.md"},
			},
		},
	}, map[string]string{
		"docs/rules/style.md": "" +
			"---\n" +
			"title: Style Guide\n" +
			"orbit_member:\n" +
			"  name: docs-rules\n" +
			"  description: Style rules\n" +
			"---\n" +
			"\n" +
			"# Style\n",
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
	})

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "member", "apply", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string   `json:"repo_root"`
		OrbitID            string   `json:"orbit_id"`
		RevisionKind       string   `json:"revision_kind"`
		DefinitionPath     string   `json:"definition_path"`
		UpdatedMemberCount int      `json:"updated_member_count"`
		UpdatedMembers     []string `json:"updated_members"`
		ConsumedHintCount  int      `json:"consumed_hint_count"`
		ConsumedHints      []string `json:"consumed_hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")), gitpkg.ComparablePath(payload.DefinitionPath))
	require.Equal(t, 2, payload.UpdatedMemberCount)
	require.ElementsMatch(t, []string{"docs-rules", "process"}, payload.UpdatedMembers)
	require.Equal(t, 2, payload.ConsumedHintCount)
	require.ElementsMatch(t, []string{"docs/process/.orbit-member.yaml", "docs/rules/style.md"}, payload.ConsumedHints)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Len(t, spec.Members, 2)

	var processMember orbitpkg.OrbitMember
	for _, member := range spec.Members {
		if member.Name == "process" {
			processMember = member
		}
	}
	require.Equal(t, "process", processMember.Name)
	require.Equal(t, orbitpkg.OrbitMemberProcess, processMember.Role)
	require.Equal(t, []string{"docs/process/**"}, processMember.Paths.Include)

	styleData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "rules", "style.md"))
	require.NoError(t, err)
	require.NotContains(t, string(styleData), "orbit_member:")
	require.Contains(t, string(styleData), "title: Style Guide")

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardOrbitContentApplyWritesTruthAndConsumesHints(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
	})

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "content", "apply", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID            string   `json:"orbit_id"`
		UpdatedMemberCount int      `json:"updated_member_count"`
		UpdatedMembers     []string `json:"updated_members"`
		ConsumedHintCount  int      `json:"consumed_hint_count"`
		ConsumedHints      []string `json:"consumed_hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.UpdatedMemberCount)
	require.Equal(t, []string{"process"}, payload.UpdatedMembers)
	require.Equal(t, 1, payload.ConsumedHintCount)
	require.Equal(t, []string{"docs/process/.orbit-member.yaml"}, payload.ConsumedHints)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Len(t, spec.Members, 1)
	require.Equal(t, "process", spec.Members[0].Name)

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardOrbitPrepareCheckReportsContentAndCheckpointReadiness(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, []orbitpkg.OrbitMember{
		{
			Name:        "docs-content",
			Description: "Docs content",
			Role:        orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}, nil)
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  description: Review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n")
	repo.WriteFile(t, "docs/guide.md", "Edited guide\n")

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready        bool     `json:"ready"`
		Blocked      bool     `json:"blocked"`
		PackageName  string   `json:"package_name"`
		OrbitID      string   `json:"orbit"`
		NextActions  []string `json:"next_actions"`
		ContentHints struct {
			DriftDetected   bool `json:"drift_detected"`
			BackfillAllowed bool `json:"backfill_allowed"`
			HintCount       int  `json:"hint_count"`
		} `json:"content_hints"`
		Checkpoint struct {
			Required       bool     `json:"required"`
			CandidatePaths []string `json:"candidate_paths"`
			BlockedPaths   []string `json:"blocked_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Equal(t, "docs", payload.PackageName)
	require.Equal(t, "docs", payload.OrbitID)
	require.True(t, payload.ContentHints.DriftDetected)
	require.True(t, payload.ContentHints.BackfillAllowed)
	require.Equal(t, 1, payload.ContentHints.HintCount)
	require.True(t, payload.Checkpoint.Required)
	require.Equal(t, []string{"docs/guide.md"}, payload.Checkpoint.CandidatePaths)
	require.Empty(t, payload.Checkpoint.BlockedPaths)
	require.Contains(t, payload.NextActions, "hyard orbit content apply docs")
	require.Contains(t, payload.NextActions, `hyard orbit checkpoint docs -m "Update docs"`)
	require.Contains(t, payload.NextActions, "hyard publish orbit docs")

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)
	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestHyardOrbitPrepareCheckReportsUntrackedExportSurfaceFiles(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "docs/new.md", "New export file\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready      bool `json:"ready"`
		Blocked    bool `json:"blocked"`
		Checkpoint struct {
			UntrackedExportPaths []string `json:"untracked_export_paths"`
		} `json:"checkpoint"`
		NextActions []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Equal(t, []string{"docs/new.md"}, payload.Checkpoint.UntrackedExportPaths)
	require.Contains(t, payload.NextActions, "git add docs/new.md")
}

func TestHyardOrbitPrepareCheckIgnoresSeededEmptyGuidanceBlocks(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "HUMANS.md", "<!-- orbit:begin orbit_id=\"docs\" -->\n<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.WriteFile(t, "BOOTSTRAP.md", "<!-- orbit:begin orbit_id=\"docs\" -->\n<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed empty guidance blocks")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready    bool `json:"ready"`
		Blocked  bool `json:"blocked"`
		Guidance struct {
			DriftDetected bool `json:"drift_detected"`
			Artifacts     []struct {
				Target    string `json:"target"`
				State     string `json:"state"`
				NeedsSave bool   `json:"needs_save"`
			} `json:"artifacts"`
		} `json:"guidance"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Ready)
	require.False(t, payload.Blocked)
	require.False(t, payload.Guidance.DriftDetected)
	for _, artifact := range payload.Guidance.Artifacts {
		require.False(t, artifact.NeedsSave, artifact.Target)
	}
}

func TestHyardOrbitPrepareAppliesContentHintsAfterConfirmation(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-guide\n" +
			"  role: rule\n" +
			"---\n" +
			"\n" +
			"# Guide\n",
	})

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "y\n", "orbit", "prepare", "docs", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, `Apply 1 content hint to package "docs"?`)

	var payload struct {
		Ready        bool `json:"ready"`
		Blocked      bool `json:"blocked"`
		ContentHints struct {
			Applied       bool `json:"applied"`
			DriftDetected bool `json:"drift_detected"`
		} `json:"content_hints"`
		Checkpoint struct {
			Required       bool     `json:"required"`
			CandidatePaths []string `json:"candidate_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.True(t, payload.ContentHints.Applied)
	require.False(t, payload.ContentHints.DriftDetected)
	require.True(t, payload.Checkpoint.Required)
	require.ElementsMatch(t, []string{".harness/orbits/docs.yaml", "docs/guide.md"}, payload.Checkpoint.CandidatePaths)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Len(t, spec.Members, 1)
	require.Equal(t, "docs-guide", spec.Members[0].Name)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "orbit_member:")
}

func TestHyardOrbitPrepareDeclineDoesNotMutate(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-guide\n" +
			"---\n" +
			"\n" +
			"# Guide\n",
	})
	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "n\n", "orbit", "prepare", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `Apply 1 content hint to package "docs"?`)
	require.ErrorContains(t, err, "prepare canceled")

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestHyardOrbitPrepareYesAppliesContentHintsWithoutPrompt(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-guide\n" +
			"---\n" +
			"\n" +
			"# Guide\n",
	})

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ContentHints struct {
			Applied bool `json:"applied"`
		} `json:"content_hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.ContentHints.Applied)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "orbit_member:")
}

func TestHyardOrbitPrepareCheckReportsLegacyRulesSchemaMigration(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, nil)
	replaceHostedDocsOrbitBehaviorKey(t, repo, "rules")
	repo.AddAndCommit(t, "seed legacy rules orbit spec")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready  bool `json:"ready"`
		Schema struct {
			LegacyRulesPresent bool     `json:"legacy_rules_present"`
			BehaviorPresent    bool     `json:"behavior_present"`
			MigrationRequired  bool     `json:"migration_required"`
			Blocked            bool     `json:"blocked"`
			Diagnostics        []string `json:"diagnostics"`
		} `json:"schema"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Ready)
	require.True(t, payload.Schema.LegacyRulesPresent)
	require.False(t, payload.Schema.BehaviorPresent)
	require.True(t, payload.Schema.MigrationRequired)
	require.False(t, payload.Schema.Blocked)
	require.Contains(t, strings.Join(payload.Schema.Diagnostics, "\n"), "legacy top-level rules")
	require.Contains(t, strings.Join(payload.Schema.Diagnostics, "\n"), "behavior")

	specData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(specData), "rules:\n")
}

func TestHyardOrbitPrepareYesNormalizesLegacyRulesSchema(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, nil)
	replaceHostedDocsOrbitBehaviorKey(t, repo, "rules")
	repo.AddAndCommit(t, "seed legacy rules orbit spec")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "prepare", "docs", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready  bool `json:"ready"`
		Schema struct {
			Applied            bool `json:"applied"`
			LegacyRulesPresent bool `json:"legacy_rules_present"`
			BehaviorPresent    bool `json:"behavior_present"`
			MigrationRequired  bool `json:"migration_required"`
			Blocked            bool `json:"blocked"`
		} `json:"schema"`
		Checkpoint struct {
			Required       bool     `json:"required"`
			CandidatePaths []string `json:"candidate_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Schema.Applied)
	require.False(t, payload.Schema.LegacyRulesPresent)
	require.True(t, payload.Schema.BehaviorPresent)
	require.False(t, payload.Schema.MigrationRequired)
	require.False(t, payload.Schema.Blocked)
	require.True(t, payload.Checkpoint.Required)
	require.Equal(t, []string{".harness/orbits/docs.yaml"}, payload.Checkpoint.CandidatePaths)

	specData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(specData), "rules:\n")
	require.Contains(t, string(specData), "behavior:\n")
}

func TestHyardOrbitCheckpointCheckRefusesUnrelatedTrackedChanges(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "docs/guide.md", "Edited guide\n")
	repo.WriteFile(t, "README.md", "Edited README\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "checkpoint", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready          bool     `json:"ready"`
		Blocked        bool     `json:"blocked"`
		CandidatePaths []string `json:"candidate_paths"`
		BlockedPaths   []string `json:"blocked_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Equal(t, []string{"docs/guide.md"}, payload.CandidatePaths)
	require.Equal(t, []string{"README.md"}, payload.BlockedPaths)
}

func TestHyardOrbitCheckpointCommitsOnlyPackageRelevantTrackedChanges(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	before := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", "Edited guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "orbit", "checkpoint", "docs", "-m", "Update docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Committed      bool     `json:"committed"`
		Commit         string   `json:"commit"`
		CandidatePaths []string `json:"candidate_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Committed)
	require.NotEmpty(t, payload.Commit)
	require.Equal(t, []string{"docs/guide.md"}, payload.CandidatePaths)
	require.NotEqual(t, before, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
	require.Equal(t, "Update docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))
}

func TestHyardPublishOrbitDelegatesToOrbitTemplatePublish(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
}

func TestHyardPublishOrbitBarePackagePublishesSnapshotJSON(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageName    string `json:"package_name"`
		PackageVersion string `json:"package_version"`
		PackageKind    string `json:"package_publish_kind"`
		PackageCoord   string `json:"package_coordinate"`
		Branch         string `json:"branch"`
		NextAction     string `json:"next_action"`
		NextRef        string `json:"next_ref"`
		LocalPublish   struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.Equal(t, "none", payload.PackageVersion)
	require.Equal(t, "snapshot", payload.PackageKind)
	require.Equal(t, "docs", payload.PackageCoord)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "install", payload.NextAction)
	require.Equal(t, "refs/heads/orbit-template/docs", payload.NextRef)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
}

func TestHyardPublishOrbitReleaseCoordinateReportsReleaseText(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs@0.1.0")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "package_name: docs\n")
	require.Contains(t, stdout, "package_version: 0.1.0\n")
	require.Contains(t, stdout, "package_publish_kind: release\n")
	require.Contains(t, stdout, "package_coordinate: docs@0.1.0\n")
	require.Contains(t, stdout, "publish_ref: refs/heads/orbit-template/docs\n")
	require.Contains(t, stdout, "next_action: install\n")
	require.Contains(t, stdout, "next_ref: refs/heads/orbit-template/docs\n")
	require.Contains(t, stdout, "local_publish.success: true\n")
}

func TestHyardPublishOrbitGitLocatorReportsLocatorText(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs@git:orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "package_name: docs\n")
	require.Contains(t, stdout, "package_version: none\n")
	require.Contains(t, stdout, "package_publish_kind: git_locator\n")
	require.Contains(t, stdout, "package_coordinate: docs@git:orbit-template/docs\n")
	require.Contains(t, stdout, "package_locator_kind: git\n")
	require.Contains(t, stdout, "package_locator: orbit-template/docs\n")
	require.Contains(t, stdout, "publish_ref: refs/heads/orbit-template/docs\n")
	require.Contains(t, stdout, "local_publish.success: true\n")
}

func TestHyardPublishRejectsBareLocatorPackageCoordinate(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs@main")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "package coordinate locator must be explicit")
	require.ErrorContains(t, err, "docs@git:main")
}

func TestHyardPublishHarnessDelegatesToHarnessTemplatePublish(t *testing.T) {
	t.Parallel()

	repo := seedHyardCloneHarnessTemplateSourceRepo(t)
	expectedSourceBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	targetBranch := "harness-template/published"

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", "--to", targetBranch, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageName    string `json:"package_name"`
		PackageVersion string `json:"package_version"`
		PackageKind    string `json:"package_publish_kind"`
		PackageCoord   string `json:"package_coordinate"`
		HarnessID      string `json:"harness_id"`
		Branch         string `json:"branch"`
		SourceBranch   string `json:"source_branch"`
		NextAction     string `json:"next_action"`
		NextRef        string `json:"next_ref"`
		LocalPublish   struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "published", payload.PackageName)
	require.Equal(t, "none", payload.PackageVersion)
	require.Equal(t, "snapshot", payload.PackageKind)
	require.Equal(t, "published", payload.PackageCoord)
	require.NotEmpty(t, payload.HarnessID)
	require.Equal(t, targetBranch, payload.Branch)
	require.Equal(t, expectedSourceBranch, payload.SourceBranch)
	require.Equal(t, "clone", payload.NextAction)
	require.Equal(t, "refs/heads/"+targetBranch, payload.NextRef)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
}

func TestHyardPublishHarnessBarePackagePublishesSnapshotJSON(t *testing.T) {
	t.Parallel()

	repo := seedHyardCloneHarnessTemplateSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", "frontend-lab", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageName    string `json:"package_name"`
		PackageVersion string `json:"package_version"`
		PackageKind    string `json:"package_publish_kind"`
		PackageCoord   string `json:"package_coordinate"`
		Branch         string `json:"branch"`
		NextAction     string `json:"next_action"`
		NextRef        string `json:"next_ref"`
		LocalPublish   struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "frontend-lab", payload.PackageName)
	require.Equal(t, "none", payload.PackageVersion)
	require.Equal(t, "snapshot", payload.PackageKind)
	require.Equal(t, "frontend-lab", payload.PackageCoord)
	require.Equal(t, "harness-template/frontend-lab", payload.Branch)
	require.Equal(t, "clone", payload.NextAction)
	require.Equal(t, "refs/heads/harness-template/frontend-lab", payload.NextRef)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
}

func TestHyardPublishHarnessReleaseCoordinateReportsReleaseText(t *testing.T) {
	t.Parallel()

	repo := seedHyardCloneHarnessTemplateSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", "frontend-lab@0.1.0")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "package_name: frontend-lab\n")
	require.Contains(t, stdout, "package_version: 0.1.0\n")
	require.Contains(t, stdout, "package_publish_kind: release\n")
	require.Contains(t, stdout, "package_coordinate: frontend-lab@0.1.0\n")
	require.Contains(t, stdout, "publish_ref: refs/heads/harness-template/frontend-lab\n")
	require.Contains(t, stdout, "next_action: clone\n")
	require.Contains(t, stdout, "next_ref: refs/heads/harness-template/frontend-lab\n")
	require.Contains(t, stdout, "local_publish.success: true\n")
}

func TestHyardInstallGitLocatorPackageCoordinateRecordsProvenanceJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedCommittedHyardSourceRepo(t)
	_, stderr, err := executeHyardCLI(t, sourceRepo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	runtimeRepo := testutil.NewRepo(t)
	runtimeRepo.Run(t, "branch", "-m", "main")
	err = executeHarnessCLIForHyardTest(t, runtimeRepo.Root, "init")
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "seed empty runtime")
	runtimeRepo.Run(t, "fetch", sourceRepo.Root, "orbit-template/docs:orbit-template/docs")

	stdout, stderr, err := executeHyardCLI(t, runtimeRepo.Root, "install", "docs@git:orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Source struct {
			Kind               string `json:"kind"`
			Ref                string `json:"ref"`
			Commit             string `json:"commit"`
			PackageName        string `json:"package_name"`
			PackageCoordinate  string `json:"package_coordinate"`
			PackageLocatorKind string `json:"package_locator_kind"`
			PackageLocator     string `json:"package_locator"`
		} `json:"source"`
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, orbittemplate.InstallSourceKindLocalBranch, payload.Source.Kind)
	require.Equal(t, "orbit-template/docs", payload.Source.Ref)
	require.NotEmpty(t, payload.Source.Commit)
	require.Equal(t, "docs", payload.Source.PackageName)
	require.Equal(t, "docs@git:orbit-template/docs", payload.Source.PackageCoordinate)
	require.Equal(t, "git", payload.Source.PackageLocatorKind)
	require.Equal(t, "orbit-template/docs", payload.Source.PackageLocator)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallSourceKindLocalBranch, record.Template.SourceKind)
	require.Equal(t, "orbit-template/docs", record.Template.SourceRef)
	require.Equal(t, payload.Source.Commit, record.Template.TemplateCommit)
}

func TestHyardInstallRejectsBareLocatorPackageCoordinate(t *testing.T) {
	t.Parallel()

	runtimeRepo := testutil.NewRepo(t)
	runtimeRepo.Run(t, "branch", "-m", "main")
	err := executeHarnessCLIForHyardTest(t, runtimeRepo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, runtimeRepo.Root, "install", "docs@main", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "package coordinate locator must be explicit")
	require.ErrorContains(t, err, "docs@git:main")
}

func TestHyardPublishOrbitHelpHidesAdvancedCompatibilityFlags(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "publish", "orbit", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Legacy automation and migration compatibility flags remain supported")
	require.Contains(t, stdout, "--default")
	require.Contains(t, stdout, "--push")
	require.Contains(t, stdout, "--remote")
	require.Contains(t, stdout, "--prepare")
	require.Contains(t, stdout, "--track-new")
	require.Contains(t, stdout, "--checkpoint")
	require.Contains(t, stdout, "--yes")
	require.Contains(t, stdout, "--message")
	require.Contains(t, stdout, "--json")
	require.NotContains(t, stdout, "--orbit")
	require.NotContains(t, stdout, "--backfill-brief")
	require.NotContains(t, stdout, "--aggregate-detected-skills")
	require.NotContains(t, stdout, "--allow-out-of-range-skills")
}

func TestHyardPublishOrbitHiddenOrbitFlagStillEnforcesSingleSourceOrbitContract(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--orbit", "api", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "single source orbit")
	require.ErrorContains(t, err, "docs")
}

func TestHyardPublishOrbitHiddenBackfillBriefBackfillsDriftedSourceBriefBeforePublishing(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepoWithDriftedBrief(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--backfill-brief", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		Branch       string   `json:"branch"`
		SourceBranch string   `json:"source_branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.Contains(t, payload.Warnings, "auto-backfilled orbit brief docs into "+filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "Drifted docs orbit guidance")

	publishedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.Contains(t, string(publishedData), "Drifted docs orbit guidance")
}

func TestHyardPublishOrbitHiddenAllowOutOfRangeSkillsWarnsAndKeepsDetectedSkillInTemplatePayload(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepoWithOutOfRangeSkill(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--allow-out-of-range-skills", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)

	warningsText := strings.Join(payload.Warnings, "\n")
	require.Contains(t, warningsText, "extras/research-kit")
	require.Contains(t, warningsText, "skills/docs/*")

	files := strings.Split(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")), "\n")
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"declared-skills/docs-style/SKILL.md",
		"declared-skills/docs-style/checklist.md",
		"docs/guide.md",
		"extras/research-kit/SKILL.md",
		"extras/research-kit/playbook.md",
	}, files)

	specData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.NotContains(t, string(specData), "skills/docs/*")
}

func TestHyardPublishOrbitHiddenAggregateDetectedSkillsMovesRootUpdatesSpecAndPublishes(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepoWithOutOfRangeSkill(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--aggregate-detected-skills", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)

	require.NoFileExists(t, filepath.Join(repo.Root, "extras", "research-kit", "SKILL.md"))
	require.FileExists(t, filepath.Join(repo.Root, "skills", "docs", "research-kit", "SKILL.md"))

	specData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(specData), "- declared-skills/*\n")
	require.Contains(t, string(specData), "- skills/docs/*\n")

	files := strings.Split(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")), "\n")
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"declared-skills/docs-style/SKILL.md",
		"declared-skills/docs-style/checklist.md",
		"docs/guide.md",
		"skills/docs/research-kit/SKILL.md",
		"skills/docs/research-kit/playbook.md",
	}, files)
}

func TestHyardPublishOrbitUsesZeroCommitProvenanceWithoutCommittedHeadOnSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepo(t)
	repo.Run(t, "add", ".")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "created_from_branch: main")
	require.Contains(t, string(manifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestHyardPublishOrbitSuggestsMemberApplyWhenHintsAreDrifted(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/rules/style.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-rules\n" +
			"  role: rule\n" +
			"---\n" +
			"\n" +
			"# Style\n",
	})

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit \"docs\" is not ready")
	require.ErrorContains(t, err, "hyard orbit content apply docs")
	require.NotContains(t, err.Error(), "orbit member backfill --orbit docs")

	var payload struct {
		PackageName  string   `json:"package_name"`
		NextActions  []string `json:"next_actions"`
		ContentHints struct {
			DriftDetected bool     `json:"drift_detected"`
			HintPaths     []string `json:"hint_paths"`
		} `json:"content_hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.Contains(t, payload.NextActions, "hyard orbit content apply docs")
	require.True(t, payload.ContentHints.DriftDetected)
	require.Equal(t, []string{"docs/rules/style.md"}, payload.ContentHints.HintPaths)
}

func TestHyardPublishOrbitSuggestsContentApplyCheckForHintDiagnostics(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: shared-docs\n" +
			"---\n" +
			"\n" +
			"# Review\n",
		"docs/rules/style.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: shared-docs\n" +
			"---\n" +
			"\n" +
			"# Style\n",
	})

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit \"docs\" is not ready")
	require.ErrorContains(t, err, "hyard orbit content apply docs --check --json")
	require.NotContains(t, err.Error(), "orbit member detect --orbit docs --json")

	var payload struct {
		PackageName  string   `json:"package_name"`
		NextActions  []string `json:"next_actions"`
		ContentHints struct {
			DriftDetected   bool     `json:"drift_detected"`
			BackfillAllowed bool     `json:"backfill_allowed"`
			HintPaths       []string `json:"hint_paths"`
		} `json:"content_hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.Contains(t, payload.NextActions, "hyard orbit content apply docs --check --json")
	require.True(t, payload.ContentHints.DriftDetected)
	require.False(t, payload.ContentHints.BackfillAllowed)
	require.Equal(t, []string{"docs/process/review.md", "docs/rules/style.md"}, payload.ContentHints.HintPaths)
}

func TestHyardPublishOrbitDirtyTrackedSuggestsCheckpoint(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "docs/guide.md", "Edited guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit \"docs\" is not ready")
	require.ErrorContains(t, err, `hyard orbit checkpoint docs -m "Update docs"`)
	require.ErrorContains(t, err, "hyard orbit prepare docs --check --json")

	var payload struct {
		Ready       bool     `json:"ready"`
		Blocked     bool     `json:"blocked"`
		NextActions []string `json:"next_actions"`
		Checkpoint  struct {
			CandidatePaths []string `json:"candidate_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Contains(t, payload.NextActions, `hyard orbit checkpoint docs -m "Update docs"`)
	require.Equal(t, []string{"docs/guide.md"}, payload.Checkpoint.CandidatePaths)
}

func TestHyardPublishOrbitBareSingleSourceDirtyTrackedSuggestsCheckpoint(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "docs/guide.md", "Edited guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit \"docs\" is not ready")
	require.ErrorContains(t, err, `hyard orbit checkpoint docs -m "Update docs"`)
	require.NotContains(t, err.Error(), "publish requires a clean tracked worktree")

	var payload struct {
		PackageName string   `json:"package_name"`
		Ready       bool     `json:"ready"`
		Blocked     bool     `json:"blocked"`
		NextActions []string `json:"next_actions"`
		Checkpoint  struct {
			CandidatePaths []string `json:"candidate_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Contains(t, payload.NextActions, `hyard orbit checkpoint docs -m "Update docs"`)
	require.Equal(t, []string{"docs/guide.md"}, payload.Checkpoint.CandidatePaths)
}

func TestHyardPublishOrbitBareSingleSourceUntrackedExportSuggestsTrackNew(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}, nil)
	repo.WriteFile(t, "docs/new.md", "# New export file\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit \"docs\" is not ready")
	require.ErrorContains(t, err, "git add docs/new.md")
	require.NotContains(t, err.Error(), "member export path")
	require.NotContains(t, err.Error(), "template payload")

	var payload struct {
		PackageName string   `json:"package_name"`
		Ready       bool     `json:"ready"`
		Blocked     bool     `json:"blocked"`
		NextActions []string `json:"next_actions"`
		Checkpoint  struct {
			UntrackedExportPaths []string `json:"untracked_export_paths"`
		} `json:"checkpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.False(t, payload.Ready)
	require.True(t, payload.Blocked)
	require.Contains(t, payload.NextActions, "git add docs/new.md")
	require.Equal(t, []string{"docs/new.md"}, payload.Checkpoint.UntrackedExportPaths)
}

func TestHyardPublishOrbitCheckpointCommitsUntrackedSourceManifestBeforePublishing(t *testing.T) {
	t.Parallel()

	repo := seedHyardSourceRepo(t)
	repo.AddAndCommit(
		t,
		"seed source repo without manifest",
		".orbit/config.yaml",
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
	)
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--checkpoint", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Contains(t, repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD"), ".harness/manifest.yaml")
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))
}

func TestHyardPublishOrbitYesRequiresExplicitMutationFlow(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "--yes can only be used with --prepare or --checkpoint")
	require.Empty(t, strings.TrimSpace(repo.Run(t, "branch", "--list", "orbit-template/docs")))
}

func TestHyardPublishOrbitPrepareCheckpointAppliesHintsCommitsAndPublishes(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "Original guide\n",
	})
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"  role: rule\n"+
		"  description: Edited guide\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--checkpoint", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))

	worktreeGuide, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.NotContains(t, string(worktreeGuide), "orbit_member:")
	require.Contains(t, string(worktreeGuide), "# Edited Guide")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	var found bool
	for _, member := range spec.Members {
		if member.Name == "docs-guide" {
			found = true
			require.Equal(t, orbitpkg.OrbitMemberRule, member.Role)
			require.Equal(t, []string{"docs/guide.md"}, member.Paths.Include)
		}
	}
	require.True(t, found)

	publishedGuide, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedGuide), "# Edited Guide")
	require.NotContains(t, string(publishedGuide), "orbit_member:")
}

func TestHyardPublishOrbitPrepareCheckpointSavesGuideDriftBeforePublishing(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	block, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(block))
	repo.AddAndCommit(t, "seed drifted docs guidance")
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--checkpoint", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "Edited docs guidance\n", spec.Meta.AgentsTemplate)
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))

	publishedGuide, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedGuide), "# Edited Guide")
	require.NotContains(t, string(publishedGuide), "orbit_member:")
}

func TestHyardPublishOrbitPrepareCheckpointAcceptsGuardedYes(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "Original guide\n",
	})
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--checkpoint", "--yes", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))

	worktreeGuide, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.NotContains(t, string(worktreeGuide), "orbit_member:")

	publishedGuide, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedGuide), "# Edited Guide")
	require.NotContains(t, string(publishedGuide), "orbit_member:")
}

func TestHyardPublishOrbitPrepareCheckpointRequiresMessageBeforeMutating(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, nil, map[string]string{
		"docs/guide.md": "Original guide\n",
	})
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--checkpoint", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish orbit --checkpoint requires -m/--message")

	worktreeGuide, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Contains(t, string(worktreeGuide), "orbit_member:")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	for _, member := range spec.Members {
		require.NotEqual(t, "docs-guide", member.Name)
	}
	require.Empty(t, strings.TrimSpace(repo.Run(t, "branch", "--list", "orbit-template/docs")))
}

func TestHyardPublishOrbitPrepareCheckpointRefusesUntrackedExportFiles(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, "docs/new.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: new-doc\n"+
		"---\n"+
		"\n"+
		"# New export file\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--checkpoint", "-m", "Prepare docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "untracked export files")
	require.ErrorContains(t, err, "docs/new.md")
	require.ErrorContains(t, err, "git add docs/new.md")
	require.Empty(t, strings.TrimSpace(repo.Run(t, "branch", "--list", "orbit-template/docs")))

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "new.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestHyardPublishOrbitPrepareTrackNewCheckpointPublishesUntrackedExportFiles(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}, nil)
	repo.WriteFile(t, "docs/new.md", "# New export file\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--prepare", "--track-new", "--checkpoint", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))

	worktreeData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "new.md"))
	require.NoError(t, err)
	require.Equal(t, "# New export file\n", string(worktreeData))

	publishedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/new.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedData), "# New export file")
}

func TestHyardPublishOrbitBareSingleSourcePrepareTrackNewCheckpointPublishesUntrackedExportFiles(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardMemberHintSourceRepo(t, []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}, nil)
	repo.WriteFile(t, "docs/new.md", "# New export file\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--prepare", "--track-new", "--checkpoint", "-m", "Prepare docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		PackageName  string `json:"package_name"`
		LocalPublish struct {
			Success bool `json:"success"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.PackageName)
	require.True(t, payload.LocalPublish.Success)
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))
	require.Equal(t, "Prepare docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))

	publishedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/new.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedData), "# New export file")
}

func TestHyardPublishOrbitTrackNewRequiresCheckpoint(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--track-new", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "--track-new can only be used with --checkpoint")
}

func TestHyardPublishOrbitFailsClosedOnDetachedHeadSourceRevision(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish requires a current branch; detached HEAD is not supported")
}

func TestHyardPublishOrbitFailsClosedOnMalformedSourceManifest(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ":\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
}

func TestHyardPublishHarnessUsesZeroCommitProvenanceWithoutCommittedHeadOnRuntimeBranch(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	targetBranch := "harness-template/workspace"

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", "--to", targetBranch, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID    string `json:"harness_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotEmpty(t, payload.HarnessID)
	require.Equal(t, targetBranch, payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, targetBranch, ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "created_from_branch: main")
	require.Contains(t, string(manifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestHyardPublishHarnessFailsClosedOnDetachedHeadRuntimeRevision(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", "--to", "harness-template/workspace", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "harness template publish requires a current branch; detached HEAD is not supported")
}

func TestHyardGuideHelpShowsCanonicalSubcommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "guide", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "render")
	require.Contains(t, stdout, "save")
	require.Contains(t, stdout, "writeback")
	require.Contains(t, stdout, "sync")
	require.NotContains(t, stdout, "materialize")
	require.NotContains(t, stdout, "backfill")
	require.NotContains(t, stdout, "compose")
}

func TestHyardGuideSaveDelegatesToGuidanceWriteback(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardSourceRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)

	block, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Saved docs guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(block))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "save", "--orbit", "docs", "--target", "agents", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID   string `json:"orbit_id"`
		Target    string `json:"target"`
		Artifacts []struct {
			Target string `json:"target"`
			Status string `json:"status"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "agents", payload.Target)
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "agents", payload.Artifacts[0].Target)
	require.Equal(t, "updated", payload.Artifacts[0].Status)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "Saved docs guidance\n", spec.Meta.AgentsTemplate)
}

func TestHyardGuideSyncDelegatesToHarnessGuidanceCompose(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	_, _, err := executeHyardCLI(t, parentDir, "create", "runtime", "demo", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, filepath.Join(parentDir, "demo"), "guide", "sync", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		Target      string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "demo")), gitpkg.ComparablePath(payload.HarnessRoot))
	require.Equal(t, "all", payload.Target)
}

func TestHyardReadyTreatsBundleOwnedOrbitsWithoutStandaloneGuidanceAsReady(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 25, 9, 30, 0, 0, time.UTC)
	_, err := harnesspkg.WriteRuntimeFile(repo.Root, harnesspkg.RuntimeFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.RuntimeKind,
		Harness: harnesspkg.RuntimeMetadata{
			ID:        "runtime-two",
			Name:      "Runtime Two",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.RuntimeMember{
			{OrbitID: "plan", Source: harnesspkg.MemberSourceInstallBundle, OwnerHarnessID: "runtime-two", AddedAt: now},
			{OrbitID: "research", Source: harnesspkg.MemberSourceInstallBundle, OwnerHarnessID: "runtime-two", AddedAt: now},
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

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
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

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "ready")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "status: ready\n")
	require.Contains(t, stdout, "ready_orbit_count: 2\n")
	require.Contains(t, stdout, "orbit: plan source=install_bundle status=ready\n")
	require.Contains(t, stdout, "orbit: research source=install_bundle status=ready\n")
	require.NotContains(t, stdout, "agents_not_composed")
	require.NotContains(t, stdout, "root AGENTS.md has not been composed")
	require.NotContains(t, stdout, "suggested_command: harness agents compose")
}

func TestHyardPrepareYesAddsRepoLevelBlocksWhenBootstrapExists(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "Existing bootstrap instructions.\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "prepared harness runtime")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "<!-- hyard:repo-bootstrap:begin -->")
	require.Contains(t, string(bootstrapData), "<!-- hyard:repo-bootstrap:end -->")
	require.Contains(t, string(bootstrapData), "Run `hyard bootstrap complete --check --json` to preview the closeout.")
	require.Contains(t, string(bootstrapData), "Run `hyard bootstrap complete --yes` after confirming the preview only removes bootstrap guidance and bootstrap-lane runtime files.")
	require.Contains(t, string(bootstrapData), "Existing bootstrap instructions.\n")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "<!-- hyard:repo-agents:begin -->")
	require.Contains(t, string(agentsData), "<!-- hyard:repo-agents:end -->")
	require.Contains(t, string(agentsData), "Before starting normal work, read `BOOTSTRAP.md` if it exists.")
	require.Contains(t, string(agentsData), "If it contains hyard bootstrap instructions, complete that initialization flow first.")
}

func TestHyardPrepareYesSkipsRepoLevelBlocksWhenBootstrapMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "prepared harness runtime")

	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
}

func TestHyardPrepareYesSkipsRepoLevelBlocksWhenBootstrapEmpty(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", " \n\t\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "prepared harness runtime")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Equal(t, " \n\t\n", string(bootstrapData))
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
}

func TestHyardPrepareYesPreservesEditedRepoLevelBlocks(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	bootstrap := "Existing bootstrap instructions.\n\n<!-- hyard:repo-bootstrap:begin -->\nCustom bootstrap prep.\n<!-- hyard:repo-bootstrap:end -->\n"
	agents := "Existing agent instructions.\n\n<!-- hyard:repo-agents:begin -->\nCustom agent prep.\n<!-- hyard:repo-agents:end -->\n"
	repo.WriteFile(t, "BOOTSTRAP.md", bootstrap)
	repo.WriteFile(t, "AGENTS.md", agents)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "prepared harness runtime")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Equal(t, bootstrap, string(bootstrapData))
	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, agents, string(agentsData))
}

func TestHyardPrepareYesRejectsMalformedRepoLevelBlockMarkers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	bootstrap := "Existing bootstrap instructions.\n<!-- hyard:repo-bootstrap:begin -->\nUnclosed custom prep.\n"
	repo.WriteFile(t, "BOOTSTRAP.md", bootstrap)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.Contains(t, err.Error(), "malformed hyard repo-level guidance block markers")
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Equal(t, bootstrap, string(bootstrapData))
}

func TestHyardPrepareCheckJSONPreviewsWithoutWritingRepoLevelBlocks(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	bootstrap := "Existing bootstrap instructions.\n"
	repo.WriteFile(t, "BOOTSTRAP.md", bootstrap)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot     string `json:"repo_root"`
		Check        bool   `json:"check"`
		RepoGuidance struct {
			BootstrapPresent bool `json:"bootstrap_present"`
			Files            []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"files"`
		} `json:"repo_guidance"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.True(t, payload.Check)
	require.True(t, payload.RepoGuidance.BootstrapPresent)
	require.Equal(t, []struct {
		Path   string `json:"path"`
		Action string `json:"action"`
	}{
		{Path: "BOOTSTRAP.md", Action: "update"},
		{Path: "AGENTS.md", Action: "create"},
	}, payload.RepoGuidance.Files)

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Equal(t, bootstrap, string(bootstrapData))
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
}

func TestHyardBootstrapCompleteCheckPreviewsRepositoryBootstrapCloseout(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot             string   `json:"repo_root"`
		Check                bool     `json:"check"`
		Completed            bool     `json:"completed"`
		CompletedOrbits      []string `json:"completed_orbits"`
		RemovedPaths         []string `json:"removed_paths"`
		RemovedRepoBlocks    []string `json:"removed_repo_blocks"`
		DeletedBootstrapFile bool     `json:"deleted_bootstrap_file"`
		DeletedAgentsFile    bool     `json:"deleted_agents_file"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.True(t, payload.Check)
	require.False(t, payload.Completed)
	require.Equal(t, []string{"docs", "ops"}, payload.CompletedOrbits)
	require.ElementsMatch(t, []string{"agents", "bootstrap"}, payload.RemovedRepoBlocks)
	require.Contains(t, payload.RemovedPaths, "AGENTS.md")
	require.Contains(t, payload.RemovedPaths, "BOOTSTRAP.md")
	require.Contains(t, payload.RemovedPaths, "bootstrap/docs/setup.md")
	require.Contains(t, payload.RemovedPaths, "bootstrap/ops/setup.md")
	require.True(t, payload.DeletedBootstrapFile)
	require.True(t, payload.DeletedAgentsFile)

	require.FileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
	require.FileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "ops", "setup.md"))
}

func TestHyardBootstrapCompleteYesClosesRepositoryBootstrap(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot             string   `json:"repo_root"`
		Check                bool     `json:"check"`
		Completed            bool     `json:"completed"`
		CompletedOrbits      []string `json:"completed_orbits"`
		RemovedPaths         []string `json:"removed_paths"`
		RemovedRepoBlocks    []string `json:"removed_repo_blocks"`
		DeletedBootstrapFile bool     `json:"deleted_bootstrap_file"`
		DeletedAgentsFile    bool     `json:"deleted_agents_file"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.False(t, payload.Check)
	require.True(t, payload.Completed)
	require.Equal(t, []string{"docs", "ops"}, payload.CompletedOrbits)
	require.ElementsMatch(t, []string{"agents", "bootstrap"}, payload.RemovedRepoBlocks)
	require.Contains(t, payload.RemovedPaths, "AGENTS.md")
	require.Contains(t, payload.RemovedPaths, "BOOTSTRAP.md")
	require.Contains(t, payload.RemovedPaths, "bootstrap/docs/setup.md")
	require.Contains(t, payload.RemovedPaths, "bootstrap/ops/setup.md")
	require.True(t, payload.DeletedBootstrapFile)
	require.True(t, payload.DeletedAgentsFile)

	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "ops", "setup.md"))

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	for _, orbitID := range []string{"docs", "ops"} {
		snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
		require.NoError(t, err)
		require.NotNil(t, snapshot.Bootstrap)
		require.True(t, snapshot.Bootstrap.Completed)
	}
}

func TestHyardBootstrapReopenRestoresRepositoryBootstrapBlocks(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)

	_, _, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "reopen", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string   `json:"repo_root"`
		ReopenedOrbits     []string `json:"reopened_orbits"`
		RestoredPaths      []string `json:"restored_paths"`
		RestoredRepoBlocks []string `json:"restored_repo_blocks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, []string{"docs", "ops"}, payload.ReopenedOrbits)
	require.ElementsMatch(t, []string{"agents", "bootstrap"}, payload.RestoredRepoBlocks)
	require.Contains(t, payload.RestoredPaths, "AGENTS.md")
	require.Contains(t, payload.RestoredPaths, "BOOTSTRAP.md")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "<!-- hyard:repo-bootstrap:begin -->")
	require.Contains(t, string(bootstrapData), "hyard bootstrap complete --check --json")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "<!-- hyard:repo-agents:begin -->")
	require.Contains(t, string(agentsData), "read `BOOTSTRAP.md`")

	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "ops", "setup.md"))

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	for _, orbitID := range []string{"docs", "ops"} {
		snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
		require.NoError(t, err)
		require.Nil(t, snapshot.Bootstrap)
	}
}

func TestHyardBootstrapReopenRejectsMalformedBootstrapBlockBeforeRuntimeMutation(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)

	_, _, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "<!-- hyard:repo-bootstrap:begin -->\nUnclosed bootstrap guidance.\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "reopen", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "malformed hyard repo-level guidance block markers")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	for _, orbitID := range []string{"docs", "ops"} {
		snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
		require.NoError(t, err)
		require.NotNil(t, snapshot.Bootstrap)
		require.True(t, snapshot.Bootstrap.Completed)
	}
}

func TestHyardBootstrapCompleteYesAcceptsUncommittedPrepareGuidance(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepoWithoutRepoGuidance(t)

	_, _, err := executeHyardCLI(t, repo.Root, "prepare", "--yes")
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		CompletedOrbits      []string `json:"completed_orbits"`
		DeletedBootstrapFile bool     `json:"deleted_bootstrap_file"`
		DeletedAgentsFile    bool     `json:"deleted_agents_file"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"docs", "ops"}, payload.CompletedOrbits)
	require.True(t, payload.DeletedBootstrapFile)
	require.True(t, payload.DeletedAgentsFile)
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
}

func TestHyardBootstrapCompleteYesAcceptsDirtyBootstrapLaneArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Locally edited docs bootstrap\n")
	repo.WriteFile(t, "bootstrap/docs/generated.md", "Generated docs bootstrap artifact\n")
	repo.Run(t, "add", "--", "bootstrap/docs/setup.md", "bootstrap/docs/generated.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var checkPayload struct {
		Completed    bool     `json:"completed"`
		RemovedPaths []string `json:"removed_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &checkPayload))
	require.False(t, checkPayload.Completed)
	require.Contains(t, checkPayload.RemovedPaths, "bootstrap/docs/setup.md")
	require.Contains(t, checkPayload.RemovedPaths, "bootstrap/docs/generated.md")
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "generated.md"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var completePayload struct {
		Completed    bool     `json:"completed"`
		RemovedPaths []string `json:"removed_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &completePayload))
	require.True(t, completePayload.Completed)
	require.Contains(t, completePayload.RemovedPaths, "bootstrap/docs/setup.md")
	require.Contains(t, completePayload.RemovedPaths, "bootstrap/docs/generated.md")
	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "generated.md"))

	status := repo.Run(t, "status", "--short")
	require.Contains(t, status, " D bootstrap/docs/setup.md")
	require.NotContains(t, status, "MD bootstrap/docs/setup.md")
	require.NotContains(t, status, "AD bootstrap/docs/generated.md")
	require.NotContains(t, status, "bootstrap/docs/generated.md")
}

func TestHyardBootstrapCompleteYesRejectsMalformedAgentsBlockBeforeRuntimeMutation(t *testing.T) {
	t.Parallel()

	repo := seedHyardBootstrapCompletionRepo(t)
	repo.WriteFile(t, "AGENTS.md", "<!-- hyard:repo-agents:begin -->\nUnclosed agent bootstrap guidance.\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "bootstrap", "complete", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "malformed hyard repo-level guidance block markers")

	require.FileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.FileExists(t, filepath.Join(repo.Root, "bootstrap", "ops", "setup.md"))

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	for _, orbitID := range []string{"docs", "ops"} {
		_, err := store.ReadRuntimeStateSnapshot(orbitID)
		require.ErrorIs(t, err, statepkg.ErrRuntimeStateSnapshotNotFound)
	}
}

func seedHyardBootstrapCompletionRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	return seedHyardBootstrapCompletionRepoWithRepoGuidance(t, true)
}

func seedHyardBootstrapCompletionRepoWithoutRepoGuidance(t *testing.T) *testutil.Repo {
	t.Helper()

	return seedHyardBootstrapCompletionRepoWithRepoGuidance(t, false)
}

func seedHyardBootstrapCompletionRepoWithRepoGuidance(t *testing.T, includeRepoGuidance bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 30, 10, 0, 0, 0, time.UTC)
	members := make([]harnesspkg.ManifestMember, 0, 2)
	var bootstrapData []byte

	for _, orbitID := range []string{"docs", "ops"} {
		displayName := map[string]string{"docs": "Docs", "ops": "Ops"}[orbitID]
		repo.WriteFile(t, filepath.Join(orbitID, "guide.md"), displayName+" guide\n")
		repo.WriteFile(t, filepath.Join("bootstrap", orbitID, "setup.md"), displayName+" bootstrap setup\n")

		spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
		require.NoError(t, err)
		require.NotNil(t, spec.Meta)
		spec.Description = displayName + " orbit"
		spec.Meta.BootstrapTemplate = fmt.Sprintf("Bootstrap the %s orbit.\n", orbitID)
		spec.Members = append(spec.Members, orbitpkg.OrbitMember{
			Key:  orbitID + "-bootstrap",
			Role: orbitpkg.OrbitMemberRule,
			Lane: orbitpkg.OrbitMemberLaneBootstrap,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{fmt.Sprintf("bootstrap/%s/**", orbitID)},
			},
		})
		_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
		require.NoError(t, err)

		members = append(members, harnesspkg.ManifestMember{
			OrbitID: orbitID,
			Source:  harnesspkg.MemberSourceManual,
			AddedAt: now,
		})

		block, err := orbittemplate.WrapRuntimeAgentsBlock(orbitID, []byte(fmt.Sprintf("Bootstrap the %s orbit.\n", orbitID)))
		require.NoError(t, err)
		if len(bootstrapData) > 0 {
			bootstrapData = append(bootstrapData, '\n')
		}
		bootstrapData = append(bootstrapData, block...)
	}

	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: members,
	})
	require.NoError(t, err)

	repo.WriteFile(t, "BOOTSTRAP.md", "Workspace bootstrap notes.\n\n"+string(bootstrapData)+"\n")
	if includeRepoGuidance {
		repo.WriteFile(t, "BOOTSTRAP.md", "Workspace bootstrap notes.\n\n"+string(bootstrapData)+"\n\n<!-- hyard:repo-bootstrap:begin -->\nComplete the repository bootstrap tasks before normal work starts.\nRun `hyard bootstrap complete --check --json` to preview the closeout.\nRun `hyard bootstrap complete --yes` after confirming the preview only removes bootstrap guidance and bootstrap-lane runtime files.\n<!-- hyard:repo-bootstrap:end -->\n")
		repo.WriteFile(t, "AGENTS.md", "<!-- hyard:repo-agents:begin -->\nBefore starting normal work, read `BOOTSTRAP.md` if it exists.\nIf it contains hyard bootstrap instructions, complete that initialization flow first.\n<!-- hyard:repo-agents:end -->\n")
	}
	repo.AddAndCommit(t, "seed hyard bootstrap completion repo")

	return repo
}

func TestHyardPrepareYesSelectsOnlyReadyAgent(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\necho 'codex 0.125.0'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "prepare", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentSelection struct {
			Action           string   `json:"action"`
			ReadyAgents      []string `json:"ready_agents"`
			SelectedAgent    string   `json:"selected_agent"`
			SelectionSource  string   `json:"selection_source"`
			SuggestedCommand string   `json:"suggested_command"`
		} `json:"agent_selection"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "selected", payload.AgentSelection.Action)
	require.Equal(t, []string{"codex"}, payload.AgentSelection.ReadyAgents)
	require.Equal(t, "codex", payload.AgentSelection.SelectedAgent)
	require.Equal(t, "project_detection", payload.AgentSelection.SelectionSource)
	require.Equal(t, "hyard agent use codex", payload.AgentSelection.SuggestedCommand)

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "codex", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceProjectDetection, selection.SelectionSource)
}

func TestHyardPrepareYesSelectsRecommendedAgentWhenMultipleReady(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.WriteFrameworksFile(repo.Root, harnesspkg.FrameworksFile{
		SchemaVersion:        1,
		RecommendedFramework: "codex",
	})
	require.NoError(t, err)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\necho 'codex 0.125.0'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	openClawPath := filepath.Join(binDir, "openclaw")
	require.NoError(t, os.WriteFile(openClawPath, []byte("#!/bin/sh\necho 'openclaw 0.9.0'\n"), 0o700))
	require.NoError(t, os.Chmod(openClawPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "prepare", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentSelection struct {
			Action           string   `json:"action"`
			RecommendedAgent string   `json:"recommended_agent"`
			ReadyAgents      []string `json:"ready_agents"`
			SelectedAgent    string   `json:"selected_agent"`
			SelectionSource  string   `json:"selection_source"`
			SuggestedCommand string   `json:"suggested_command"`
		} `json:"agent_selection"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "selected", payload.AgentSelection.Action)
	require.Equal(t, "codex", payload.AgentSelection.RecommendedAgent)
	require.Equal(t, []string{"codex", "openclaw"}, payload.AgentSelection.ReadyAgents)
	require.Equal(t, "codex", payload.AgentSelection.SelectedAgent)
	require.Equal(t, "recommended_default", payload.AgentSelection.SelectionSource)
	require.Equal(t, "hyard agent use codex", payload.AgentSelection.SuggestedCommand)

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "codex", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceRecommendedDefault, selection.SelectionSource)
}

func TestHyardPreparePromptsBeforeSelectingOnlyReadyAgent(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\necho 'codex 0.125.0'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(gitExecutable))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), ".claude"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(t.TempDir(), ".openclaw"))

	stdout, stderr, err := executeHyardCLIWithInputUnlocked(t, repo.Root, "n\n", "prepare")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Use detected agent codex now?")
	require.Contains(t, stdout, "suggested_command: hyard agent use codex")

	_, err = harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHyardPrepareYesKeepsExistingSelectedAgent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.WriteFrameworkSelection(repo.GitDir(t), harnesspkg.FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   harnesspkg.FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "prepare", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AgentSelection struct {
			Action            string   `json:"action"`
			LocalSelection    string   `json:"local_selection"`
			ReadyAgents       []string `json:"ready_agents"`
			SelectedAgent     string   `json:"selected_agent"`
			SelectedFramework string   `json:"selected_framework"`
			SelectionSource   string   `json:"selection_source"`
		} `json:"agent_selection"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "already_selected", payload.AgentSelection.Action)
	require.Equal(t, "codex", payload.AgentSelection.LocalSelection)
	require.Empty(t, payload.AgentSelection.ReadyAgents)
	require.Equal(t, "codex", payload.AgentSelection.SelectedAgent)
	require.Equal(t, "codex", payload.AgentSelection.SelectedFramework)
	require.Equal(t, "explicit_local", payload.AgentSelection.SelectionSource)

	selection, err := harnesspkg.LoadFrameworkSelection(repo.GitDir(t))
	require.NoError(t, err)
	require.Equal(t, "codex", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceExplicitLocal, selection.SelectionSource)
	require.Equal(t, time.Date(2026, time.April, 30, 9, 0, 0, 0, time.UTC), selection.UpdatedAt)
}

func TestHyardCloneFromLocalHarnessTemplateSourceCreatesRuntime(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		sourceRepo.Root,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
		MemberCount int    `json:"member_count"`
		BundleCount int    `json:"bundle_count"`
		Source      struct {
			Repo         string `json:"repo"`
			RequestedRef string `json:"requested_ref"`
			ResolvedRef  string `json:"resolved_ref"`
			Commit       string `json:"commit"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, filepath.Base(sourceRepo.Root))), gitpkg.ComparablePath(payload.HarnessRoot))
	require.Equal(t, gitpkg.ComparablePath(sourceRepo.Root), gitpkg.ComparablePath(payload.Source.Repo))
	require.Equal(t, "harness-template/workspace", payload.Source.RequestedRef)
	require.Equal(t, "harness-template/workspace", payload.Source.ResolvedRef)
	require.NotEmpty(t, payload.Source.Commit)
	require.Equal(t, 1, payload.MemberCount)
	require.Equal(t, 1, payload.BundleCount)

	resolved, err := harnesspkg.ResolveRoot(context.Background(), payload.HarnessRoot)
	require.NoError(t, err)
	require.Len(t, resolved.Runtime.Members, 1)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, resolved.Runtime.Members[0].Source)
	require.Equal(t, payload.HarnessID, resolved.Runtime.Members[0].OwnerHarnessID)
}

func TestHyardCloneUsesExplicitRepoNameAndParentPath(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	workingDir := t.TempDir()
	parentDir := filepath.Join(t.TempDir(), "clones")
	require.NoError(t, os.MkdirAll(parentDir, 0o750))

	stdout, stderr, err := executeHyardCLI(
		t,
		workingDir,
		"clone",
		sourceRepo.Root,
		"workspace-copy",
		"--path",
		parentDir,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "workspace-copy")), gitpkg.ComparablePath(payload.HarnessRoot))

	_, err = os.Stat(filepath.Join(parentDir, "workspace-copy", ".harness", "manifest.yaml"))
	require.NoError(t, err)
}

func TestHyardCloneRejectsExplicitRepoNameWithPathSeparator(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := filepath.Join(t.TempDir(), "clones")
	require.NoError(t, os.MkdirAll(parentDir, 0o750))
	targetRoot := filepath.Join(parentDir, "nested")

	_, _, err := executeHyardCLI(
		t,
		t.TempDir(),
		"clone",
		sourceRepo.Root,
		filepath.Join("nested", "workspace-copy"),
		"--path",
		parentDir,
		"--ref",
		"harness-template/workspace",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `clone [repo-name] must be one leaf directory name`)

	_, statErr := os.Stat(targetRoot)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardCloneRejectsExplicitRepoNameDotOrDotDot(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)

	for _, repoName := range []string{".", ".."} {
		repoName := repoName
		t.Run(repoName, func(t *testing.T) {
			t.Parallel()

			parentDir := filepath.Join(t.TempDir(), "clones")
			require.NoError(t, os.MkdirAll(parentDir, 0o750))

			_, _, err := executeHyardCLI(
				t,
				t.TempDir(),
				"clone",
				sourceRepo.Root,
				repoName,
				"--path",
				parentDir,
				"--ref",
				"harness-template/workspace",
			)
			require.Error(t, err)
			require.ErrorContains(t, err, `clone [repo-name] must be one leaf directory name`)
		})
	}
}

func TestHyardCloneRejectsExplicitWhitespaceRepoName(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	parentDir := filepath.Join(t.TempDir(), "clones")
	require.NoError(t, os.MkdirAll(parentDir, 0o750))

	_, _, err := executeHyardCLI(
		t,
		t.TempDir(),
		"clone",
		sourceRepo.Root,
		"   ",
		"--path",
		parentDir,
		"--ref",
		"harness-template/workspace",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `clone [repo-name] must not be empty`)
}

func TestHyardCloneFromRemoteHarnessTemplateSourceCreatesRuntime(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		remoteURL,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		Source      struct {
			Repo string `json:"repo"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(remoteURL), gitpkg.ComparablePath(payload.Source.Repo))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "remote")), gitpkg.ComparablePath(payload.HarnessRoot))

	resolved, err := harnesspkg.ResolveRoot(context.Background(), payload.HarnessRoot)
	require.NoError(t, err)
	require.Len(t, resolved.Runtime.Members, 1)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, resolved.Runtime.Members[0].Source)
}

func TestHyardCloneGitLocatorPackageCoordinateRecordsProvenanceJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardCloneHarnessTemplateSourceRepo(t)
	_, stderr, err := executeHyardCLI(t, sourceRepo.Root, "publish", "harness", "frontend-lab", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	parentDir := t.TempDir()

	stdout, stderr, err := executeHyardCLI(
		t,
		sourceRepo.Root,
		"clone",
		"frontend-lab@git:harness-template/frontend-lab",
		"runtime-two",
		"--path",
		parentDir,
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
		Source      struct {
			Repo               string `json:"repo"`
			RequestedRef       string `json:"requested_ref"`
			ResolvedRef        string `json:"resolved_ref"`
			Commit             string `json:"commit"`
			PackageName        string `json:"package_name"`
			PackageCoordinate  string `json:"package_coordinate"`
			PackageLocatorKind string `json:"package_locator_kind"`
			PackageLocator     string `json:"package_locator"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, gitpkg.ComparablePath(filepath.Join(parentDir, "runtime-two")), gitpkg.ComparablePath(payload.HarnessRoot))
	require.Equal(t, gitpkg.ComparablePath(sourceRepo.Root), gitpkg.ComparablePath(payload.Source.Repo))
	require.Equal(t, "harness-template/frontend-lab", payload.Source.RequestedRef)
	require.Equal(t, "harness-template/frontend-lab", payload.Source.ResolvedRef)
	require.NotEmpty(t, payload.Source.Commit)
	require.Equal(t, "frontend-lab", payload.Source.PackageName)
	require.Equal(t, "frontend-lab@git:harness-template/frontend-lab", payload.Source.PackageCoordinate)
	require.Equal(t, "git", payload.Source.PackageLocatorKind)
	require.Equal(t, "harness-template/frontend-lab", payload.Source.PackageLocator)

	record, err := harnesspkg.LoadBundleRecord(payload.HarnessRoot, payload.HarnessID)
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallSourceKindExternalGit, record.Template.SourceKind)
	require.Equal(t, gitpkg.ComparablePath(sourceRepo.Root), gitpkg.ComparablePath(record.Template.SourceRepo))
	require.Equal(t, "harness-template/frontend-lab", record.Template.SourceRef)
	require.Equal(t, payload.Source.Commit, record.Template.TemplateCommit)
}

func TestHyardCloneRejectsInvalidHarnessTemplateSourceWithoutCreatingRepo(t *testing.T) {
	t.Parallel()

	invalidSource := testutil.NewRepo(t)
	parentDir := t.TempDir()
	targetRoot := filepath.Join(parentDir, filepath.Base(invalidSource.Root))

	_, _, err := executeHyardCLI(
		t,
		parentDir,
		"clone",
		invalidSource.Root,
		"--ref",
		"harness-template/workspace",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `remote harness template ref "harness-template/workspace" from "`)
	require.ErrorContains(t, err, `" is not a valid harness template branch`)

	_, statErr := os.Stat(targetRoot)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
