package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessGuidanceComposeSupportsBootstrapTargetInJSON(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapGuidanceRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "guidance", "compose", "--target", "bootstrap", "--output", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot   string `json:"harness_root"`
		Target        string `json:"target"`
		MemberCount   int    `json:"member_count"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target string `json:"target"`
			Path   string `json:"path"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "bootstrap", payload.Target)
	require.Equal(t, 1, payload.MemberCount)
	require.Equal(t, 1, payload.ArtifactCount)
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "bootstrap", payload.Artifacts[0].Target)
	require.Equal(t, filepath.Join(repo.Root, "BOOTSTRAP.md"), payload.Artifacts[0].Path)

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "Bootstrap the docs orbit.\n")
}

func TestHarnessGuidanceComposeAllIncludesBootstrapArtifact(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapGuidanceRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "guidance", "compose", "--target", "all", "--output", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target        string `json:"target"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target string `json:"target"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 3, payload.ArtifactCount)

	targets := make([]string, 0, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		targets = append(targets, artifact.Target)
	}
	require.ElementsMatch(t, []string{"agents", "humans", "bootstrap"}, targets)
}

func TestHarnessGuidanceComposeBootstrapSkipsCompletedOrbit(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapGuidanceRepo(t)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed: true,
		},
	}))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "guidance", "compose", "--target", "bootstrap", "--output", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target         string   `json:"target"`
			ComposedOrbits []string `json:"composed_orbits"`
			SkippedOrbits  []string `json:"skipped_orbits"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "bootstrap", payload.Target)
	require.Len(t, payload.Artifacts, 1)
	require.Empty(t, payload.Artifacts[0].ComposedOrbits)
	require.Equal(t, []string{"docs"}, payload.Artifacts[0].SkippedOrbits)

	_, err = os.Stat(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func seedHarnessBootstrapGuidanceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the docs orbit.\n"+
		"  humans_template: |\n"+
		"    Run the docs workflow.\n"+
		"  bootstrap_template: |\n"+
		"    Bootstrap the docs orbit.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	return repo
}
