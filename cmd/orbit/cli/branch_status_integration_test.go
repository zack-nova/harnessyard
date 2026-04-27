package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBranchStatusReportsTemplateForCurrentTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "seed template branch")
	repo.Run(t, "branch", "-M", "not-a-template-name")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "kind: template\nreason: valid .harness/manifest.yaml present with kind=orbit_template\n", stdout)
}

func TestBranchStatusReportsRuntimeForCurrentRuntimeBranch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "runtime content\n")
	repo.AddAndCommit(t, "seed runtime branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "kind: runtime\nreason: valid .harness/manifest.yaml present with kind=runtime\n", stdout)
}

func TestBranchStatusTreatsLegacySourceBranchAsPlain(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/source.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template_source\n"+
		"source_branch: main\n"+
		"publish:\n"+
		"  orbit_id: docs\n")
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "source content\n")
	repo.AddAndCommit(t, "seed source branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "kind: plain\nreason: no valid .harness/manifest.yaml found\n", stdout)
}

func TestBranchStatusReportsPlainForCurrentPlainBranch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain branch\n")
	repo.AddAndCommit(t, "seed plain branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "kind: plain\nreason: no valid .harness/manifest.yaml found\n", stdout)
}

func TestBranchStatusFailsOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	stdout, stderr, err := executeCLI(t, workingDir, "branch", "status")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "discover git repository")
}

func TestBranchStatusSupportsJSONOutput(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "runtime content\n")
	repo.AddAndCommit(t, "seed runtime branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "master", payload.Branch)
	require.False(t, payload.Detached)
	require.True(t, payload.HeadExists)
	require.Equal(t, "runtime", payload.Classification.Kind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=runtime", payload.Classification.Reason)
}

func TestBranchStatusSupportsTemplateKindInJSONOutput(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"includes_root_agents: false\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "seed harness template branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind         string `json:"kind"`
			TemplateKind string `json:"template_kind"`
			Reason       string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "master", payload.Branch)
	require.False(t, payload.Detached)
	require.True(t, payload.HeadExists)
	require.Equal(t, "template", payload.Classification.Kind)
	require.Equal(t, "harness", payload.Classification.TemplateKind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=harness_template", payload.Classification.Reason)
}

func TestBranchStatusTreatsLegacySourceMarkersAsPlainInJSONOutput(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/source.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template_source\n"+
		"source_branch: main\n"+
		"publish:\n"+
		"  orbit_id: docs\n")
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "source content\n")
	repo.AddAndCommit(t, "seed source branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "master", payload.Branch)
	require.False(t, payload.Detached)
	require.True(t, payload.HeadExists)
	require.Equal(t, "plain", payload.Classification.Kind)
	require.Equal(t, "no valid .harness/manifest.yaml found", payload.Classification.Reason)
}

func TestBranchStatusWorksWithoutCommittedHeadUsingCurrentWorktreeManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.Run(t, "add", "-A")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "main", payload.Branch)
	require.False(t, payload.Detached)
	require.False(t, payload.HeadExists)
	require.Equal(t, "source", payload.Classification.Kind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=source", payload.Classification.Reason)
}

func TestBranchStatusPrefersCurrentWorktreeManifestOverHeadHistory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "seed runtime branch")

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind         string `json:"kind"`
			TemplateKind string `json:"template_kind"`
			Reason       string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "master", payload.Branch)
	require.False(t, payload.Detached)
	require.True(t, payload.HeadExists)
	require.Equal(t, "template", payload.Classification.Kind)
	require.Equal(t, "orbit", payload.Classification.TemplateKind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=orbit_template", payload.Classification.Reason)
}

func TestBranchStatusReportsDetachedCurrentWorktreeStateInJSON(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "seed source branch")
	repo.Run(t, "checkout", "--detach", "HEAD")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch         string `json:"branch"`
		Detached       bool   `json:"detached"`
		HeadExists     bool   `json:"head_exists"`
		Classification struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		} `json:"classification"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Branch)
	require.True(t, payload.Detached)
	require.True(t, payload.HeadExists)
	require.Equal(t, "source", payload.Classification.Kind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=source", payload.Classification.Reason)
}
