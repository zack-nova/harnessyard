package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestHyardViewAuthorJSONRecordsAuthorViewWithoutMaterializingFiles(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewStatusRuntimeRepo(t)
	agentsBefore := readRepoFile(t, repo.Root, "AGENTS.md")
	humansBefore := readRepoFile(t, repo.Root, "HUMANS.md")
	guideBefore := readRepoFile(t, repo.Root, "docs/guide.md")
	memberHintBefore := readRepoFile(t, repo.Root, "docs/process/review.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "author", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SelectedView       string `json:"selected_view"`
		SelectionPersisted bool   `json:"selection_persisted"`
		Materialized       struct {
			GuidanceMarkers bool `json:"guidance_markers"`
			MarkdownContent bool `json:"markdown_content"`
			MemberHints     bool `json:"member_hints"`
		} `json:"materialized"`
		NextActions []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, "author", payload.SelectedView)
	require.True(t, payload.SelectionPersisted)
	require.False(t, payload.Materialized.GuidanceMarkers)
	require.False(t, payload.Materialized.MarkdownContent)
	require.False(t, payload.Materialized.MemberHints)
	require.Contains(t, payload.NextActions, "render editable guidance with `hyard guide render`")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	selection, err := store.ReadRuntimeViewSelection()
	require.NoError(t, err)
	require.Equal(t, statepkg.RuntimeViewAuthor, selection.View)
	require.True(t, selection.Persisted)

	require.Equal(t, agentsBefore, readRepoFile(t, repo.Root, "AGENTS.md"))
	require.Equal(t, humansBefore, readRepoFile(t, repo.Root, "HUMANS.md"))
	require.Equal(t, guideBefore, readRepoFile(t, repo.Root, "docs/guide.md"))
	require.Equal(t, memberHintBefore, readRepoFile(t, repo.Root, "docs/process/review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", ".orbit-member.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.Empty(t, repo.Run(t, "status", "--short"))
}

func TestHyardGuideRenderTransitionsRunViewToAuthorViewWhenGuidanceIsWritten(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	writeHyardHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	repo.AddAndCommit(t, "seed runtime with authored docs guidance")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeViewSelection(statepkg.RuntimeViewSelection{
		View:       statepkg.RuntimeViewRun,
		SelectedAt: time.Date(2026, time.May, 5, 8, 45, 0, 0, time.UTC),
	}))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "render", "--orbit", "docs", "--target", "agents", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Artifacts []struct {
			Target  string `json:"target"`
			Status  string `json:"status"`
			Changed bool   `json:"changed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "agents", payload.Artifacts[0].Target)
	require.Equal(t, "rendered", payload.Artifacts[0].Status)
	require.True(t, payload.Artifacts[0].Changed)
	require.Contains(t, readRepoFile(t, repo.Root, "AGENTS.md"), "Docs orbit guidance\n")

	selection, err := store.ReadRuntimeViewSelection()
	require.NoError(t, err)
	require.Equal(t, statepkg.RuntimeViewAuthor, selection.View)
	require.True(t, selection.Persisted)
}

func TestHyardViewStatusReportsAuthorViewWithoutAuthoringScaffolds(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	_, stderr, err := executeHyardCLI(t, repo.Root, "view", "author", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SelectedView       string `json:"selected_view"`
		SelectionPersisted bool   `json:"selection_persisted"`
		ActualPresentation struct {
			Mode                      string `json:"mode"`
			AuthoringScaffoldsPresent bool   `json:"authoring_scaffolds_present"`
			GuidanceMarkersPresent    bool   `json:"guidance_markers_present"`
			MemberHintsPresent        bool   `json:"member_hints_present"`
		} `json:"actual_presentation"`
		AllowedPublicationActions []string `json:"allowed_publication_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, "author", payload.SelectedView)
	require.True(t, payload.SelectionPersisted)
	require.Equal(t, "runtime_content", payload.ActualPresentation.Mode)
	require.False(t, payload.ActualPresentation.AuthoringScaffoldsPresent)
	require.False(t, payload.ActualPresentation.GuidanceMarkersPresent)
	require.False(t, payload.ActualPresentation.MemberHintsPresent)
	require.Equal(t, []string{"orbit_package", "current_runtime_harness_package"}, payload.AllowedPublicationActions)
}

func readRepoFile(t *testing.T, repoRoot string, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	require.NoError(t, err)

	return string(data)
}
