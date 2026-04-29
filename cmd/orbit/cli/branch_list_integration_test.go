package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBranchListEnumeratesLocalBranchesWithStableClassification(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain branch\n")
	repo.AddAndCommit(t, "seed plain branch")
	repo.Run(t, "branch", "-M", "looks-runtime-but-plain")

	repo.Run(t, "checkout", "-b", "looks-plain-but-template")
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

	repo.Run(t, "checkout", "looks-runtime-but-plain")
	repo.Run(t, "checkout", "-b", "looks-template-but-runtime")
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

	repo.Run(t, "checkout", "looks-runtime-but-plain")
	repo.Run(t, "checkout", "-b", "looks-runtime-but-harness-template")
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
		"root_guidance:\n"+
		"  agents: false\n"+
		"  humans: false\n"+
		"  bootstrap: false\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "harness template content\n")
	repo.AddAndCommit(t, "seed harness template branch")

	repo.Run(t, "checkout", "looks-runtime-but-plain")
	repo.Run(t, "checkout", "-b", "looks-runtime-but-legacy-source")
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
	repo.WriteFile(t, "docs/guide.md", "legacy source content\n")
	repo.AddAndCommit(t, "seed legacy source branch")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"looks-plain-but-template\ttemplate\tvalid .harness/manifest.yaml present with kind=orbit_template\n"+
		"looks-runtime-but-harness-template\ttemplate\tvalid .harness/manifest.yaml present with kind=harness_template\n"+
		"looks-runtime-but-legacy-source\tplain\tno valid .harness/manifest.yaml found\n"+
		"looks-runtime-but-plain\tplain\tno valid .harness/manifest.yaml found\n"+
		"looks-template-but-runtime\truntime\tvalid .harness/manifest.yaml present with kind=runtime\n", stdout)
}

func TestBranchListFailsOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	stdout, stderr, err := executeCLI(t, workingDir, "branch", "list")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "discover git repository")
}

func TestBranchListSupportsJSONOutput(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain branch\n")
	repo.AddAndCommit(t, "seed plain branch")
	repo.Run(t, "branch", "-M", "plain-main")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branches []struct {
			Name           string `json:"name"`
			Classification struct {
				Kind   string `json:"kind"`
				Reason string `json:"reason"`
			} `json:"classification"`
		} `json:"branches"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Branches, 1)
	require.Equal(t, "plain-main", payload.Branches[0].Name)
	require.Equal(t, "plain", payload.Branches[0].Classification.Kind)
	require.Equal(t, "no valid .harness/manifest.yaml found", payload.Branches[0].Classification.Reason)
}

func TestBranchListSupportsTemplateKindInJSONOutput(t *testing.T) {
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
		"root_guidance:\n"+
		"  agents: false\n"+
		"  humans: false\n"+
		"  bootstrap: false\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "seed harness template branch")
	repo.Run(t, "branch", "-M", "harness-template")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branches []struct {
			Name           string `json:"name"`
			Classification struct {
				Kind         string `json:"kind"`
				TemplateKind string `json:"template_kind"`
				Reason       string `json:"reason"`
			} `json:"classification"`
		} `json:"branches"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Branches, 1)
	require.Equal(t, "harness-template", payload.Branches[0].Name)
	require.Equal(t, "template", payload.Branches[0].Classification.Kind)
	require.Equal(t, "harness", payload.Branches[0].Classification.TemplateKind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=harness_template", payload.Branches[0].Classification.Reason)
}

func TestBranchListTreatsLegacySourceMarkersAsPlainInJSONOutput(t *testing.T) {
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
	repo.AddAndCommit(t, "seed legacy source branch")
	repo.Run(t, "branch", "-M", "source-main")

	stdout, stderr, err := executeCLI(t, repo.Root, "branch", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branches []struct {
			Name           string `json:"name"`
			Classification struct {
				Kind   string `json:"kind"`
				Reason string `json:"reason"`
			} `json:"classification"`
		} `json:"branches"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Branches, 1)
	require.Equal(t, "source-main", payload.Branches[0].Name)
	require.Equal(t, "plain", payload.Branches[0].Classification.Kind)
	require.Equal(t, "no valid .harness/manifest.yaml found", payload.Branches[0].Classification.Reason)
}
