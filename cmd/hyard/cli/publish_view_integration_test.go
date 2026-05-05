package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHyardPublishOrbitRunViewBlocksOrbitPackageJSON(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "Run View allows publishing only the current runtime as a Harness Package")
	require.ErrorContains(t, err, "hyard view author")
	require.ErrorContains(t, err, "hyard publish harness")

	var payload struct {
		Error                      string   `json:"error"`
		SelectedView               string   `json:"selected_view"`
		RequestedPublicationAction string   `json:"requested_publication_action"`
		AllowedPublicationActions  []string `json:"allowed_publication_actions"`
		NextActions                []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit_package_not_allowed_in_run_view", payload.Error)
	require.Equal(t, "run", payload.SelectedView)
	require.Equal(t, "orbit_package", payload.RequestedPublicationAction)
	require.Equal(t, []string{"current_runtime_harness_package"}, payload.AllowedPublicationActions)
	require.Contains(t, payload.NextActions, "switch to Author View with `hyard view author` before publishing an Orbit Package")
	require.Contains(t, payload.NextActions, "publish current runtime as a Harness Package with `hyard publish harness <package>`")
}

func TestHyardPublishOrbitRunViewBlocksOrbitPackageText(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "Run View allows publishing only the current runtime as a Harness Package")
	require.ErrorContains(t, err, "hyard view author")
	require.ErrorContains(t, err, "hyard publish harness")
}

func TestHyardPublishOrbitAuthorViewPublishesRuntimeOrbitPackageJSON(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	_, stderr, err := executeHyardCLI(t, repo.Root, "view", "author", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "orbit", "docs", "--json")
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

func TestHyardPublishHarnessAllowedFromRunAndAuthorViewsJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		selectView []string
		packageArg string
	}{
		{
			name:       "run view",
			packageArg: "run-share",
		},
		{
			name:       "author view",
			selectView: []string{"view", "author", "--json"},
			packageArg: "author-share",
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := seedCommittedHyardRuntimeRepo(t)
			if len(tt.selectView) > 0 {
				_, stderr, err := executeHyardCLI(t, repo.Root, tt.selectView...)
				require.NoError(t, err)
				require.Empty(t, stderr)
			}

			stdout, stderr, err := executeHyardCLI(t, repo.Root, "publish", "harness", tt.packageArg, "--json")
			require.NoError(t, err)
			require.Empty(t, stderr)

			var payload struct {
				PackageName  string `json:"package_name"`
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
			require.Equal(t, tt.packageArg, payload.PackageName)
			require.NotEmpty(t, payload.HarnessID)
			require.Equal(t, "harness-template/"+tt.packageArg, payload.Branch)
			require.Equal(t, "main", payload.SourceBranch)
			require.True(t, payload.LocalPublish.Success)
			require.True(t, payload.LocalPublish.Changed)
			require.NotEmpty(t, payload.LocalPublish.Commit)
		})
	}
}
