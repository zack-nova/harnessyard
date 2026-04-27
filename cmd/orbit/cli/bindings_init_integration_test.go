package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBindingsInitLocalBranchWritesSkeletonToStdout(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "bindings", "init", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"    project_name:\n"+
		"        value: \"\"\n"+
		"        description: Product title\n", stdout)

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "vars.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(repo.GitDir(t), "orbit", "state", "current_orbit.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestBindingsInitRemoteGitWritesOutputFileAndJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "plain repo\n")
	repo.AddAndCommit(t, "seed plain repo")
	outputPath := filepath.Join(repo.Root, "generated-bindings.yaml")

	stdout, stderr, err := executeCLI(t, repo.Root, "bindings", "init", remoteURL, "--out", outputPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Source struct {
			Kind   string `json:"kind"`
			Repo   string `json:"repo"`
			Ref    string `json:"ref"`
			Commit string `json:"commit"`
		} `json:"source"`
		OutputPath string `json:"output_path"`
		Bindings   struct {
			SchemaVersion int `json:"schema_version"`
			Variables     map[string]struct {
				Value       string `json:"value"`
				Description string `json:"description"`
			} `json:"variables"`
		} `json:"bindings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "orbit-template/docs", payload.Source.Ref)
	require.NotEmpty(t, payload.Source.Commit)
	require.Equal(t, outputPath, payload.OutputPath)
	require.Equal(t, 1, payload.Bindings.SchemaVersion)
	require.Empty(t, payload.Bindings.Variables["project_name"].Value)
	require.Equal(t, "Product title", payload.Bindings.Variables["project_name"].Description)

	outputData, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	parsed, err := bindings.ParseVarsData(outputData)
	require.NoError(t, err)
	require.Equal(t, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "",
				Description: "Product title",
			},
		},
	}, parsed)

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit"))
	require.ErrorIs(t, err, os.ErrNotExist)
	requireNoRemoteTempRefs(t, repo)
}
