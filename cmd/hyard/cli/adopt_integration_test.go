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
	Candidates             []hyardAdoptCheckCandidate   `json:"candidates"`
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
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

type hyardAdoptCheckCandidate struct {
	Path                  string                          `json:"path"`
	Kind                  string                          `json:"kind"`
	Shape                 string                          `json:"shape"`
	RecommendedMemberRole string                          `json:"recommended_member_role,omitempty"`
	RoleConfirmation      hyardAdoptCheckRoleConfirmation `json:"role_confirmation,omitempty"`
	Evidence              []hyardAdoptCheckEvidence       `json:"evidence"`
}

type hyardAdoptCheckRoleConfirmation struct {
	Required               bool     `json:"required"`
	BatchAcceptRecommended bool     `json:"batch_accept_recommended"`
	EditableRoles          []string `json:"editable_roles,omitempty"`
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

func TestHyardAdoptCheckJSONReportsRootAndReferencedGuidanceCandidates(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [domain docs](docs/agents/domain.md).\n")
	repo.WriteFile(t, "docs/agents/domain.md", "# Domain language\n")
	repo.AddAndCommit(t, "seed agent guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:  "AGENTS.md",
		Kind:  "root_agent_guidance",
		Shape: "file",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "root_agent_guidance", Path: "AGENTS.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/agents/domain.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/agents/domain.md"},
		},
	})
}

func TestHyardAdoptCheckJSONDiscoversMarkdownLinksWithFragmentsAndTitles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [domain docs](docs/agents/domain.md#language \"Domain language\").\n")
	repo.WriteFile(t, "docs/agents/domain.md", "# Domain language\n")
	repo.AddAndCommit(t, "seed titled agent guidance link")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/agents/domain.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/agents/domain.md"},
		},
	})
	require.Empty(t, payload.Diagnostics)
}

func TestHyardAdoptCheckJSONDiscoversPathMentionsAndKeepsDirectoryCandidates(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Read CONTEXT.md, docs/runbook.md, and docs/ops/ before adopting this repository.\n")
	repo.WriteFile(t, "CONTEXT.md", "# Project language\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.WriteFile(t, "docs/ops/incident.md", "# Incident process\n")
	repo.AddAndCommit(t, "seed mentioned guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "CONTEXT.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "CONTEXT.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/runbook.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/runbook.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/ops",
		Kind:                  "referenced_guidance_document",
		Shape:                 "directory",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/ops"},
		},
	})
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), "docs/ops/incident.md")
}

func TestHyardAdoptCheckJSONWarnsAboutMissingAndUntrackedGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [missing guidance](docs/missing.md) and docs/draft.md.\n")
	repo.WriteFile(t, "docs/draft.md", "# Draft guidance\n")
	repo.AddAndCommit(t, "seed root guidance", "AGENTS.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	candidatePaths := hyardAdoptCheckCandidatePaths(payload.Candidates)
	require.NotContains(t, candidatePaths, "docs/missing.md")
	require.NotContains(t, candidatePaths, "docs/draft.md")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_missing",
		Severity: "warning",
		Message:  "referenced guidance path is missing",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/missing.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_untracked",
		Severity: "warning",
		Message:  "referenced guidance path is untracked and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/draft.md"},
		},
	})
}

func TestHyardAdoptCheckJSONFiltersExternalAnchorsAndWarnsAboutUnsafeGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [external](https://example.com/guide.md), [anchor](#local), "+
		"[parent](../outside.md), and [absolute](/tmp/secret.md).\n")
	repo.AddAndCommit(t, "seed unsafe guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []hyardAdoptCheckCandidate{
		{
			Path:  "AGENTS.md",
			Kind:  "root_agent_guidance",
			Shape: "file",
			Evidence: []hyardAdoptCheckEvidence{
				{Kind: "root_agent_guidance", Path: "AGENTS.md"},
			},
		},
	}, payload.Candidates)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_unsafe",
		Severity: "warning",
		Message:  "referenced guidance path is unsafe and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "../outside.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_unsafe",
		Severity: "warning",
		Message:  "referenced guidance path is unsafe and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "/tmp/secret.md"},
		},
	})
}

func TestHyardAdoptCheckJSONWarnsAboutIgnoredDependencyAndCacheGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".gitignore", "node_modules/\n.cache/\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Do not adopt node_modules/tool/README.md or .cache/agent.md.\n")
	repo.WriteFile(t, "node_modules/tool/README.md", "# Tool docs\n")
	repo.WriteFile(t, ".cache/agent.md", "# Cached agent output\n")
	repo.AddAndCommit(t, "seed ignored guidance references", ".gitignore", "AGENTS.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_ignored",
		Severity: "warning",
		Message:  "referenced guidance path is ignored dependency or cache content and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "node_modules/tool/README.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_ignored",
		Severity: "warning",
		Message:  "referenced guidance path is ignored dependency or cache content and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: ".cache/agent.md"},
		},
	})
}

func TestHyardAdoptCheckJSONKeepsGuidanceDiscoveryOneHopByDefault(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Start with [first hop](docs/first.md).\n")
	repo.WriteFile(t, "docs/first.md", ""+
		"# First hop\n\n"+
		"Do not recursively adopt [second hop](docs/second.md).\n")
	repo.WriteFile(t, "docs/second.md", "# Second hop\n")
	repo.AddAndCommit(t, "seed one-hop guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/first.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/first.md"},
		},
	})
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), "docs/second.md")
}

func hyardAdoptCheckCandidatePaths(candidates []hyardAdoptCheckCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}

	return paths
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
