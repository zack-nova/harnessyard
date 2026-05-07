package cli_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessFrameworkPlanIncludesBootstrapOutputForPendingBootstrapOnly(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkBootstrapRepo(t, frameworkBootstrapSeedOptions{
		PendingOrbits:   []string{"docs"},
		CompletedOrbits: nil,
	})

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ProjectOutputs []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"project_outputs"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.ProjectOutputs, struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}{
		Path: "BOOTSTRAP.md",
		Kind: "guidance",
	})
}

func TestHarnessFrameworkApplyDoesNotMaterializePendingBootstrapGuidance(t *testing.T) {
	repo := seedHarnessFrameworkBootstrapRepo(t, frameworkBootstrapSeedOptions{
		PendingOrbits:   []string{"docs"},
		CompletedOrbits: []string{"ops"},
	})
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot        string `json:"harness_root"`
		Framework          string `json:"framework"`
		ActivationPath     string `json:"activation_path"`
		ProjectOutputCount int    `json:"project_output_count"`
		GlobalOutputCount  int    `json:"global_output_count"`
		ArtifactResults    []struct {
			Path string `json:"path"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "claudecode", payload.Framework)
	require.NotEmpty(t, payload.ActivationPath)
	require.Zero(t, payload.ProjectOutputCount)
	require.Zero(t, payload.GlobalOutputCount)
	require.Empty(t, payload.ArtifactResults)

	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))

	activation, err := harnesspkg.LoadFrameworkActivation(repo.GitDir(t), "claudecode")
	require.NoError(t, err)
	require.Equal(t, "claudecode", activation.Framework)
	require.Empty(t, activation.ProjectOutputs)
	require.Empty(t, activation.GlobalOutputs)
	require.NotEmpty(t, activation.GuidanceHash)
}

type frameworkBootstrapSeedOptions struct {
	PendingOrbits   []string
	CompletedOrbits []string
}

func seedHarnessFrameworkBootstrapRepo(t *testing.T, options frameworkBootstrapSeedOptions) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")

	seedOrbit := func(orbitID string) {
		t.Helper()
		repo.WriteFile(t, filepath.Join(".harness", "orbits", orbitID+".yaml"), ""+
			"id: "+orbitID+"\n"+
			"description: "+orbitID+" orbit\n"+
			"meta:\n"+
			"  file: .harness/orbits/"+orbitID+".yaml\n"+
			"  bootstrap_template: |\n"+
			"    Bootstrap the "+orbitID+" orbit.\n"+
			"  include_in_projection: true\n"+
			"  include_in_write: true\n"+
			"  include_in_export: true\n"+
			"  include_description_in_orchestration: true\n"+
			"members:\n"+
			"  - key: "+orbitID+"-content\n"+
			"    role: subject\n"+
			"    scopes:\n"+
			"      export: true\n"+
			"    paths:\n"+
			"      include:\n"+
			"        - "+orbitID+"/**\n")
		repo.WriteFile(t, filepath.Join(orbitID, "guide.md"), orbitID+" guide\n")
		_, _, addErr := executeHarnessCLI(t, repo.Root, "add", orbitID)
		require.NoError(t, addErr)
	}

	for _, orbitID := range options.PendingOrbits {
		seedOrbit(orbitID)
	}
	for _, orbitID := range options.CompletedOrbits {
		seedOrbit(orbitID)
	}

	repo.AddAndCommit(t, "seed harness framework bootstrap runtime")

	if len(options.CompletedOrbits) > 0 {
		store, err := statepkg.NewFSStore(repo.GitDir(t))
		require.NoError(t, err)
		for _, orbitID := range options.CompletedOrbits {
			require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
				Orbit: orbitID,
				Bootstrap: &statepkg.RuntimeBootstrapState{
					Completed:   true,
					CompletedAt: time.Date(2026, time.April, 18, 9, 0, 0, 0, time.UTC),
				},
			}))
		}
	}

	classification, err := harnesspkg.ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, repo.Root, classification.Repo.Root)

	return repo
}
