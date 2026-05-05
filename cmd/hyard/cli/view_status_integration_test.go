package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardViewStatusJSONReportsDefaultRunViewAndAuthoringPresentation(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewStatusRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SelectedView       string `json:"selected_view"`
		SelectionPersisted bool   `json:"selection_persisted"`
		ActualPresentation struct {
			Mode                       string `json:"mode"`
			AuthoringScaffoldsPresent  bool   `json:"authoring_scaffolds_present"`
			GuidanceMarkersPresent     bool   `json:"guidance_markers_present"`
			MemberHintsPresent         bool   `json:"member_hints_present"`
			CurrentOrbit               string `json:"current_orbit,omitempty"`
			CurrentOrbitSparseCheckout bool   `json:"current_orbit_sparse_checkout,omitempty"`
		} `json:"actual_presentation"`
		GuidanceMarkers []struct {
			Target            string `json:"target"`
			Path              string `json:"path"`
			Present           bool   `json:"present"`
			BlockCount        int    `json:"block_count"`
			OrbitBlockCount   int    `json:"orbit_block_count"`
			HarnessBlockCount int    `json:"harness_block_count"`
			ParseError        string `json:"parse_error,omitempty"`
		} `json:"guidance_markers"`
		MemberHints struct {
			HintCount       int      `json:"hint_count"`
			DriftDetected   bool     `json:"drift_detected"`
			BackfillAllowed bool     `json:"backfill_allowed"`
			BlockerCount    int      `json:"blocker_count"`
			Blockers        []string `json:"blockers"`
		} `json:"member_hints"`
		DriftBlockers             []string `json:"drift_blockers"`
		AllowedPublicationActions []string `json:"allowed_publication_actions"`
		NextActions               []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, "run", payload.SelectedView)
	require.False(t, payload.SelectionPersisted)
	require.Equal(t, "authoring_scaffolds", payload.ActualPresentation.Mode)
	require.True(t, payload.ActualPresentation.AuthoringScaffoldsPresent)
	require.True(t, payload.ActualPresentation.GuidanceMarkersPresent)
	require.True(t, payload.ActualPresentation.MemberHintsPresent)

	markersByTarget := make(map[string]struct {
		Path              string
		Present           bool
		BlockCount        int
		OrbitBlockCount   int
		HarnessBlockCount int
	})
	for _, marker := range payload.GuidanceMarkers {
		markersByTarget[marker.Target] = struct {
			Path              string
			Present           bool
			BlockCount        int
			OrbitBlockCount   int
			HarnessBlockCount int
		}{
			Path:              marker.Path,
			Present:           marker.Present,
			BlockCount:        marker.BlockCount,
			OrbitBlockCount:   marker.OrbitBlockCount,
			HarnessBlockCount: marker.HarnessBlockCount,
		}
	}
	require.Equal(t, map[string]struct {
		Path              string
		Present           bool
		BlockCount        int
		OrbitBlockCount   int
		HarnessBlockCount int
	}{
		"agents": {
			Path:            "AGENTS.md",
			Present:         true,
			BlockCount:      1,
			OrbitBlockCount: 1,
		},
		"humans": {
			Path: "HUMANS.md",
		},
		"bootstrap": {
			Path: "BOOTSTRAP.md",
		},
	}, markersByTarget)

	require.Equal(t, 1, payload.MemberHints.HintCount)
	require.True(t, payload.MemberHints.DriftDetected)
	require.True(t, payload.MemberHints.BackfillAllowed)
	require.Equal(t, 0, payload.MemberHints.BlockerCount)
	require.Empty(t, payload.DriftBlockers)
	require.Equal(t, []string{"current_runtime_harness_package"}, payload.AllowedPublicationActions)
	require.Contains(t, payload.NextActions, "publish current runtime as a Harness Package")
	require.NoFileExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "runtime_view_selection.json"))
}

func TestHyardViewStatusJSONReportsAuthorViewPublicationActions(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewStatusRuntimeRepo(t)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeViewSelection(statepkg.RuntimeViewSelection{
		View:       statepkg.RuntimeViewAuthor,
		SelectedAt: time.Date(2026, time.May, 5, 8, 30, 0, 0, time.UTC),
	}))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SelectedView              string   `json:"selected_view"`
		SelectionPersisted        bool     `json:"selection_persisted"`
		AllowedPublicationActions []string `json:"allowed_publication_actions"`
		NextActions               []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, "author", payload.SelectedView)
	require.True(t, payload.SelectionPersisted)
	require.Equal(t, []string{"orbit_package", "current_runtime_harness_package"}, payload.AllowedPublicationActions)
	require.Contains(t, payload.NextActions, "publish an Orbit Package")
	require.Contains(t, payload.NextActions, "publish current runtime as a Harness Package")
}

func TestHyardViewStatusTextReportsRepresentativeStatus(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewStatusRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "status")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "selected_view: run (default)\n")
	require.Contains(t, stdout, "actual_presentation: authoring_scaffolds\n")
	require.Contains(t, stdout, "guidance_markers:\n")
	require.Contains(t, stdout, "  agents AGENTS.md present blocks=1 orbit=1 harness=0\n")
	require.Contains(t, stdout, "  humans HUMANS.md absent blocks=0 orbit=0 harness=0\n")
	require.Contains(t, stdout, "  bootstrap BOOTSTRAP.md absent blocks=0 orbit=0 harness=0\n")
	require.Contains(t, stdout, "member_hints: 1 drift=true backfill_allowed=true blockers=0\n")
	require.Contains(t, stdout, "allowed_publication_actions:\n")
	require.Contains(t, stdout, "  current_runtime_harness_package\n")
	require.Contains(t, stdout, "next_actions:\n")
	require.Contains(t, stdout, "  publish current runtime as a Harness Package\n")
}

func TestHyardViewStatusMemberHintsAreScopedToRuntimeMembers(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewStatusRuntimeRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)
	repo.WriteFile(t, ".harness/orbits/ops.yaml", ""+
		"id: ops\n"+
		"description: Ops orbit\n"+
		"include:\n"+
		"  - ops/**\n")
	repo.WriteFile(t, "ops/runbook.md", "Ops runbook\n")
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.ManifestMember{
			{
				OrbitID: "docs",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
			{
				OrbitID: "ops",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "add ops runtime member")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		MemberHints struct {
			HintCount int `json:"hint_count"`
		} `json:"member_hints"`
		MemberHintOrbits []struct {
			OrbitID   string `json:"orbit_id"`
			HintCount int    `json:"hint_count"`
		} `json:"member_hint_orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, 1, payload.MemberHints.HintCount)
	require.Equal(t, []struct {
		OrbitID   string `json:"orbit_id"`
		HintCount int    `json:"hint_count"`
	}{
		{OrbitID: "docs", HintCount: 1},
		{OrbitID: "ops", HintCount: 0},
	}, payload.MemberHintOrbits)
}

func seedHyardViewStatusRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n"+
		"members: []\n")
	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: review\n"+
		"  role: process\n"+
		"---\n"+
		"# Review\n")
	repo.WriteFile(t, "HUMANS.md", "Runtime user notes\n")

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Use the docs workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsBlock))

	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.ManifestMember{
			{
				OrbitID: "docs",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed runtime view status repo")

	return repo
}
