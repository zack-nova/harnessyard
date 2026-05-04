package cli_test

import (
	"context"
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

type hyardAdoptCheckPayload struct {
	SchemaVersion          string                       `json:"schema_version"`
	RepoRoot               string                       `json:"repo_root"`
	Mode                   string                       `json:"mode"`
	Adoptable              bool                         `json:"adoptable"`
	ExistingHarnessRuntime bool                         `json:"existing_harness_runtime"`
	DirtyWorktree          hyardAdoptCheckDirtyWorktree `json:"dirty_worktree"`
	AdoptedOrbit           hyardAdoptCheckAdoptedOrbit  `json:"adopted_orbit"`
	Frameworks             hyardAdoptCheckFrameworks    `json:"frameworks"`
	Diagnostics            []hyardAdoptCheckDiagnostic  `json:"diagnostics"`
	NextActions            []hyardAdoptCheckNextAction  `json:"next_actions"`
}

type hyardAdoptCheckDirtyWorktree struct {
	Dirty bool     `json:"dirty"`
	Paths []string `json:"paths"`
}

type hyardAdoptCheckAdoptedOrbit struct {
	ID          string `json:"id"`
	DerivedFrom string `json:"derived_from"`
}

type hyardAdoptCheckFrameworks struct {
	Recommended string                     `json:"recommended,omitempty"`
	Detected    []hyardAdoptCheckFramework `json:"detected"`
	Unsupported []hyardAdoptCheckFramework `json:"unsupported,omitempty"`
}

type hyardAdoptCheckFramework struct {
	ID       string                    `json:"id"`
	Status   string                    `json:"status"`
	Evidence []hyardAdoptCheckEvidence `json:"evidence"`
}

type hyardAdoptCheckEvidence struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type hyardAdoptCheckDiagnostic struct {
	Code     string                    `json:"code"`
	Severity string                    `json:"severity"`
	Message  string                    `json:"message"`
	Evidence []hyardAdoptCheckEvidence `json:"evidence,omitempty"`
}

type hyardAdoptCheckNextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

func TestHyardAdoptCheckJSONReportsCleanOrdinaryRepositoryWithoutWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Clean ordinary repository\n")
	repo.AddAndCommit(t, "seed clean ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, `"source"`)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "check", payload.Mode)
	require.True(t, payload.Adoptable)
	require.False(t, payload.ExistingHarnessRuntime)
	require.False(t, payload.DirtyWorktree.Dirty)
	require.Empty(t, payload.DirtyWorktree.Paths)
	require.NotEmpty(t, payload.AdoptedOrbit.ID)
	require.Equal(t, "repository_name", payload.AdoptedOrbit.DerivedFrom)
	require.Empty(t, payload.Diagnostics)
	require.Empty(t, payload.NextActions)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptCheckJSONAllowsDirtyWorktreeAndReportsPreviewDiagnostic(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n")
	repo.AddAndCommit(t, "seed dirty ordinary repository")
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n\nUncommitted note.\n")
	repo.WriteFile(t, "notes/todo.md", "- adopt me later\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, `"source"`)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.True(t, payload.DirtyWorktree.Dirty)
	require.ElementsMatch(t, []string{"README.md", "notes/todo.md"}, payload.DirtyWorktree.Paths)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "dirty_worktree",
		Severity: "info",
		Message:  "dirty worktree is allowed in adoption check mode",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "worktree_status", Path: "README.md"},
			{Kind: "worktree_status", Path: "notes/todo.md"},
		},
	})

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptCheckJSONRefusesExistingHarnessRuntimeWithLayoutOptimizationAction(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.May, 4, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness runtime")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Adoptable)
	require.True(t, payload.ExistingHarnessRuntime)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "existing_harness_runtime",
		Severity: "error",
		Message:  "existing Harness Runtime cannot be adopted again",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "harness_manifest", Path: ".harness/manifest.yaml"},
		},
	})
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard layout optimize",
		Reason:  "existing Harness Runtimes should use Layout Optimization",
	})

	resolved, err := harnesspkg.ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, repo.Root, resolved.Repo.Root)
}

func TestHyardAdoptCheckJSONDerivesAdoptedOrbitIDFromRepositoryNameAndAllowsOverride(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "My Runtime Repo!")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# Named repo\n"), 0o600))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed named repo")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var defaultPayload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &defaultPayload))
	require.Equal(t, "my-runtime-repo", defaultPayload.AdoptedOrbit.ID)
	require.Equal(t, "repository_name", defaultPayload.AdoptedOrbit.DerivedFrom)

	stdout, stderr, err = executeHyardCLI(t, repoRoot, "adopt", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var overridePayload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &overridePayload))
	require.Equal(t, "docs", overridePayload.AdoptedOrbit.ID)
	require.Equal(t, "flag", overridePayload.AdoptedOrbit.DerivedFrom)
}

func TestHyardAdoptCheckJSONReportsCodexProjectFootprintAsRecommendedFramework(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex ordinary repository\n")
	repo.WriteFile(t, ".codex/config.toml", "model = \"gpt-5.4\"\n")
	repo.AddAndCommit(t, "seed codex ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Equal(t, "codex", payload.Frameworks.Recommended)
	require.Contains(t, payload.Frameworks.Detected, hyardAdoptCheckFramework{
		ID:     "codex",
		Status: "supported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: ".codex/config.toml"},
		},
	})
	require.Empty(t, payload.Frameworks.Unsupported)
}

func TestHyardAdoptCheckJSONReportsUnsupportedClaudeCodeAndOpenClawFootprints(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Unsupported footprint ordinary repository\n")
	repo.WriteFile(t, "CLAUDE.md", "# Claude Code project guidance\n")
	repo.WriteFile(t, ".openclaw/openclaw.json", "{}\n")
	repo.AddAndCommit(t, "seed unsupported agent footprints")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Empty(t, payload.Frameworks.Recommended)
	claude := hyardAdoptCheckFramework{
		ID:     "claudecode",
		Status: "unsupported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: "CLAUDE.md"},
		},
	}
	openclaw := hyardAdoptCheckFramework{
		ID:     "openclaw",
		Status: "unsupported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: ".openclaw/openclaw.json"},
		},
	}
	require.Contains(t, payload.Frameworks.Detected, claude)
	require.Contains(t, payload.Frameworks.Detected, openclaw)
	require.Contains(t, payload.Frameworks.Unsupported, claude)
	require.Contains(t, payload.Frameworks.Unsupported, openclaw)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "unsupported_agent_footprint",
		Severity: "warning",
		Message:  "Claude Code project footprint is detected but unsupported by first-version Adoption",
		Evidence: claude.Evidence,
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "unsupported_agent_footprint",
		Severity: "warning",
		Message:  "OpenClaw project footprint is detected but unsupported by first-version Adoption",
		Evidence: openclaw.Evidence,
	})
}

func newNamedGitRepoForHyardAdopt(t *testing.T, name string) string {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(repoRoot, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".home"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".xdg"), 0o750))
	runGitForHyardAdopt(t, repoRoot, "init")
	runGitForHyardAdopt(t, repoRoot, "config", "user.name", "Hyard Adopt Test")
	runGitForHyardAdopt(t, repoRoot, "config", "user.email", "hyard-adopt@example.com")

	return repoRoot
}

func runGitForHyardAdopt(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = repoRoot
	command.Env = append(
		os.Environ(),
		"HOME="+filepath.Join(repoRoot, ".home"),
		"XDG_CONFIG_HOME="+filepath.Join(repoRoot, ".xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, string(output))

	return string(output)
}
