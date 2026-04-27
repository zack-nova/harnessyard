package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplateSavePlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"template",
		"save",
		"docs",
		"--to",
		"orbit-template/docs",
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: building template preview\n")
	require.Contains(t, stderr, "progress: writing template branch\n")
	require.Contains(t, stderr, "progress: template save complete\n")

	var payload struct {
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
}

func TestTemplatePublishPlainProgressIncludesPushStages(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"template",
		"publish",
		"--push",
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: building publish preview\n")
	require.Contains(t, stderr, "progress: checking local publish state\n")
	require.Contains(t, stderr, "progress: writing published template\n")
	require.Contains(t, stderr, "progress: checking remote freshness\n")
	require.Contains(t, stderr, "progress: pushing published branch\n")
	require.Contains(t, stderr, "progress: publish complete\n")

	var payload struct {
		OrbitID string `json:"orbit_id"`
		Remote  struct {
			Attempted bool `json:"attempted"`
			Success   bool `json:"success"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.True(t, payload.Remote.Attempted)
	require.True(t, payload.Remote.Success)
}

func TestBindingsInitPlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain repo\n")
	repo.AddAndCommit(t, "seed plain repo")
	outputPath := filepath.Join(repo.Root, "generated-bindings.yaml")

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"bindings",
		"init",
		remoteURL,
		"--out",
		outputPath,
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: resolving external template source\n")
	require.Contains(t, stderr, "progress: building bindings skeleton\n")
	require.Contains(t, stderr, "progress: writing bindings skeleton\n")
	require.Contains(t, stderr, "progress: bindings init complete\n")

	var payload struct {
		Source struct {
			Kind string `json:"kind"`
		} `json:"source"`
		OutputPath string `json:"output_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, outputPath, payload.OutputPath)

	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}
