package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
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

func TestHyardViewRunCheckJSONReportsCleanupCandidatesWithoutMutation(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunNoDriftRuntimeRepo(t)
	agentsBefore, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	hintBefore, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Check             bool `json:"check"`
		Ready             bool `json:"ready"`
		Changed           bool `json:"changed"`
		CleanupCandidates []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Target  string `json:"target,omitempty"`
			OrbitID string `json:"orbit_id,omitempty"`
			Action  string `json:"action"`
		} `json:"cleanup_candidates"`
		Blockers         []string `json:"blockers"`
		DriftDiagnostics []struct {
			Kind            string `json:"kind"`
			Path            string `json:"path"`
			OrbitID         string `json:"orbit_id,omitempty"`
			RecoveryCommand string `json:"recovery_command,omitempty"`
		} `json:"drift_diagnostics"`
		NextActions []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.True(t, payload.Check)
	require.True(t, payload.Ready)
	require.False(t, payload.Changed)
	require.Empty(t, payload.Blockers)
	require.Empty(t, payload.DriftDiagnostics)
	require.Contains(t, payload.CleanupCandidates, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Target  string `json:"target,omitempty"`
		OrbitID string `json:"orbit_id,omitempty"`
		Action  string `json:"action"`
	}{
		Kind:    "root_guidance_marker_lines",
		Path:    "AGENTS.md",
		Target:  "agents",
		OrbitID: "docs",
		Action:  "strip_marker_lines_preserve_content",
	})
	require.Contains(t, payload.CleanupCandidates, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Target  string `json:"target,omitempty"`
		OrbitID string `json:"orbit_id,omitempty"`
		Action  string `json:"action"`
	}{
		Kind:    "member_hint",
		Path:    "docs/process/.orbit-member.yaml",
		OrbitID: "docs",
		Action:  "remove_consumed_hint",
	})
	require.Contains(t, payload.NextActions, "run `hyard view run` to apply Run View cleanup")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	hintAfter, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.NoError(t, err)
	require.Equal(t, agentsBefore, agentsAfter)
	require.Equal(t, hintBefore, hintAfter)
}

func TestHyardViewRunJSONStripsRootGuidanceMarkersAndReportsChangedFiles(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunRootGuidanceRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Check        bool `json:"check"`
		Ready        bool `json:"ready"`
		Changed      bool `json:"changed"`
		ChangedFiles []struct {
			Path       string `json:"path"`
			Target     string `json:"target"`
			Action     string `json:"action"`
			BlockCount int    `json:"block_count"`
		} `json:"changed_files"`
		SkippedTargets []struct {
			Path   string `json:"path"`
			Target string `json:"target"`
			Reason string `json:"reason"`
		} `json:"skipped_targets"`
		Notes []string `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.False(t, payload.Check)
	require.True(t, payload.Ready)
	require.True(t, payload.Changed)
	require.Contains(t, payload.ChangedFiles, struct {
		Path       string `json:"path"`
		Target     string `json:"target"`
		Action     string `json:"action"`
		BlockCount int    `json:"block_count"`
	}{
		Path:       "AGENTS.md",
		Target:     "agents",
		Action:     "strip_marker_lines_preserve_content",
		BlockCount: 2,
	})
	require.Contains(t, payload.ChangedFiles, struct {
		Path       string `json:"path"`
		Target     string `json:"target"`
		Action     string `json:"action"`
		BlockCount int    `json:"block_count"`
	}{
		Path:       "BOOTSTRAP.md",
		Target:     "bootstrap",
		Action:     "strip_marker_lines_preserve_content",
		BlockCount: 1,
	})
	require.Contains(t, payload.ChangedFiles, struct {
		Path       string `json:"path"`
		Target     string `json:"target"`
		Action     string `json:"action"`
		BlockCount int    `json:"block_count"`
	}{
		Path:       "HUMANS.md",
		Target:     "humans",
		Action:     "strip_marker_lines_preserve_content",
		BlockCount: 1,
	})
	require.Contains(t, payload.ChangedFiles, struct {
		Path       string `json:"path"`
		Target     string `json:"target"`
		Action     string `json:"action"`
		BlockCount int    `json:"block_count"`
	}{
		Path:   "docs/process/.orbit-member.yaml",
		Action: "remove_consumed_hint",
	})
	require.Empty(t, payload.SkippedTargets)
	require.Contains(t, payload.Notes, "marker removal is presentation cleanup only; later authoring requires explicit `hyard guide render` or reconciliation")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"Global agents guidance\n"+
		"Docs orbit guidance\n"+
		"Workspace harness guidance\n"+
		"Tail agents guidance\n", string(agentsAfter))

	humansAfter, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Equal(t, "Human docs guidance\n", string(humansAfter))

	bootstrapAfter, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Equal(t, "Bootstrap docs guidance\n", string(bootstrapAfter))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
}

func TestHyardViewRunJSONRemovesNestedMarkdownMemberHintAndPreservesMetadata(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunNestedMarkdownMemberHintRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready        bool `json:"ready"`
		Changed      bool `json:"changed"`
		ChangedFiles []struct {
			Path              string `json:"path"`
			Action            string `json:"action"`
			PreservedMetadata bool   `json:"preserved_metadata,omitempty"`
		} `json:"changed_files"`
		Blockers []string `json:"blockers"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Ready)
	require.True(t, payload.Changed)
	require.Empty(t, payload.Blockers)
	require.Contains(t, payload.ChangedFiles, struct {
		Path              string `json:"path"`
		Action            string `json:"action"`
		PreservedMetadata bool   `json:"preserved_metadata,omitempty"`
	}{
		Path:              "docs/process/review.md",
		Action:            "remove_consumed_hint",
		PreservedMetadata: true,
	})

	reviewAfter, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"---\n"+
		"title: Review Flow\n"+
		"tags:\n"+
		"    - process\n"+
		"---\n"+
		"\n"+
		"# Review\n", string(reviewAfter))
}

func TestHyardViewRunTextToleratesDirtyWorktreeAndReportsSkippedRootGuidanceTargets(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunNoDriftRuntimeRepo(t)
	repo.WriteFile(t, "HUMANS.md", "Human plain notes\n")
	repo.WriteFile(t, "README.md", "local runtime notes\n")
	require.NotEmpty(t, repo.Run(t, "status", "--short"))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "check: false\n")
	require.Contains(t, stdout, "ready: true\n")
	require.Contains(t, stdout, "changed: true\n")
	require.Contains(t, stdout, "changed_files:\n")
	require.Contains(t, stdout, "  agents AGENTS.md action=strip_marker_lines_preserve_content blocks=1\n")
	require.Contains(t, stdout, "  docs/process/.orbit-member.yaml action=remove_consumed_hint\n")
	require.Contains(t, stdout, "skipped_targets:\n")
	require.Contains(t, stdout, "  bootstrap BOOTSTRAP.md reason=missing\n")
	require.Contains(t, stdout, "  humans HUMANS.md reason=no_marker_lines\n")
	require.Contains(t, stdout, "notes:\n")
	require.Contains(t, stdout, "  marker removal is presentation cleanup only; later authoring requires explicit `hyard guide render` or reconciliation\n")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, "Docs orbit guidance\n", string(agentsAfter))

	humansAfter, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Equal(t, "Human plain notes\n", string(humansAfter))
	readmeAfter, err := os.ReadFile(filepath.Join(repo.Root, "README.md"))
	require.NoError(t, err)
	require.Equal(t, "local runtime notes\n", string(readmeAfter))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
}

func TestHyardViewRunJSONReportsSkippedRootGuidanceTargets(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunNoDriftRuntimeRepo(t)
	repo.WriteFile(t, "HUMANS.md", "Human plain notes\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SkippedTargets []struct {
			Path   string `json:"path"`
			Target string `json:"target"`
			Reason string `json:"reason"`
		} `json:"skipped_targets"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.SkippedTargets, struct {
		Path   string `json:"path"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}{
		Path:   "BOOTSTRAP.md",
		Target: "bootstrap",
		Reason: "missing",
	})
	require.Contains(t, payload.SkippedTargets, struct {
		Path   string `json:"path"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}{
		Path:   "HUMANS.md",
		Target: "humans",
		Reason: "no_marker_lines",
	})
}

func TestHyardViewRunJSONRefusesActualCleanupWhenPlannerReportsAuthoredTruthDrift(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunDriftRuntimeRepo(t)
	agentsBefore, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.ErrorContains(t, err, "Run View cleanup blocked by Authored Truth Drift")
	require.Empty(t, stderr)

	var payload struct {
		Ready        bool `json:"ready"`
		Changed      bool `json:"changed"`
		ChangedFiles []struct {
			Path string `json:"path"`
		} `json:"changed_files"`
		Blockers []string `json:"blockers"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.False(t, payload.Changed)
	require.Empty(t, payload.ChangedFiles)
	require.Contains(t, payload.Blockers, "AGENTS.md agents block \"docs\" has authored truth drift; run `hyard guide save --orbit docs --target agents`")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, agentsBefore, agentsAfter)
	require.FileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
}

func TestHyardViewRunJSONBlocksAmbiguousFlatMemberHintBeforeMutation(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunAmbiguousFlatMemberHintRuntimeRepo(t)
	agentsBefore, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	reviewBefore, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.ErrorContains(t, err, "Run View cleanup blocked by Authored Truth Drift")
	require.Empty(t, stderr)

	var payload struct {
		Ready        bool `json:"ready"`
		Changed      bool `json:"changed"`
		ChangedFiles []struct {
			Path string `json:"path"`
		} `json:"changed_files"`
		Blockers []string `json:"blockers"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.False(t, payload.Changed)
	require.Empty(t, payload.ChangedFiles)
	require.Contains(t, payload.Blockers, "docs docs/process/review.md: docs/process/review.md mixes flat member hint fields with ordinary frontmatter metadata; use nested orbit_member for Run View cleanup; run `hyard orbit content apply docs --check --json`")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	reviewAfter, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Equal(t, agentsBefore, agentsAfter)
	require.Equal(t, reviewBefore, reviewAfter)
}

func TestHyardViewRunJSONBlocksInvalidMemberHintBeforeMutation(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunAmbiguousFlatMemberHintRuntimeRepo(t)
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member: review\n"+
		"---\n"+
		"\n"+
		"# Review\n")
	agentsBefore, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	reviewBefore, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--json")
	require.ErrorContains(t, err, "Run View cleanup blocked by Authored Truth Drift")
	require.Empty(t, stderr)

	var payload struct {
		Ready        bool `json:"ready"`
		Changed      bool `json:"changed"`
		ChangedFiles []struct {
			Path string `json:"path"`
		} `json:"changed_files"`
		Blockers []string `json:"blockers"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Ready)
	require.False(t, payload.Changed)
	require.Empty(t, payload.ChangedFiles)
	require.Contains(t, payload.Blockers, "docs docs/process/review.md: docs/process/review.md orbit_member must be a mapping; run `hyard orbit content apply docs --check --json`")

	agentsAfter, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	reviewAfter, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Equal(t, agentsBefore, agentsAfter)
	require.Equal(t, reviewBefore, reviewAfter)
}

func TestHyardViewRunCheckJSONBlocksOnAuthoredTruthDrift(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunDriftRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ready            bool     `json:"ready"`
		Blockers         []string `json:"blockers"`
		DriftDiagnostics []struct {
			Kind            string `json:"kind"`
			Path            string `json:"path"`
			Target          string `json:"target,omitempty"`
			OrbitID         string `json:"orbit_id,omitempty"`
			State           string `json:"state,omitempty"`
			RecoveryCommand string `json:"recovery_command,omitempty"`
		} `json:"drift_diagnostics"`
		NextActions []string `json:"next_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.False(t, payload.Ready)
	require.Contains(t, payload.Blockers, "AGENTS.md agents block \"docs\" has authored truth drift; run `hyard guide save --orbit docs --target agents`")
	require.Contains(t, payload.Blockers, "docs docs/process/.orbit-member.yaml has pending member hint drift; run `hyard orbit content apply docs --check --json`")
	require.Contains(t, payload.Blockers, "docs docs/rules/style.md has unapplied member hint action \"create_new\"; run `hyard orbit content apply docs --check --json`")
	require.Contains(t, payload.DriftDiagnostics, struct {
		Kind            string `json:"kind"`
		Path            string `json:"path"`
		Target          string `json:"target,omitempty"`
		OrbitID         string `json:"orbit_id,omitempty"`
		State           string `json:"state,omitempty"`
		RecoveryCommand string `json:"recovery_command,omitempty"`
	}{
		Kind:            "root_guidance_drift",
		Path:            "AGENTS.md",
		Target:          "agents",
		OrbitID:         "docs",
		State:           "materialized_drifted",
		RecoveryCommand: "hyard guide save --orbit docs --target agents",
	})
	require.Contains(t, payload.DriftDiagnostics, struct {
		Kind            string `json:"kind"`
		Path            string `json:"path"`
		Target          string `json:"target,omitempty"`
		OrbitID         string `json:"orbit_id,omitempty"`
		State           string `json:"state,omitempty"`
		RecoveryCommand string `json:"recovery_command,omitempty"`
	}{
		Kind:            "member_hint_drift",
		Path:            "docs/process/.orbit-member.yaml",
		OrbitID:         "docs",
		State:           "match_existing",
		RecoveryCommand: "hyard orbit content apply docs --check --json",
	})
	require.Contains(t, payload.DriftDiagnostics, struct {
		Kind            string `json:"kind"`
		Path            string `json:"path"`
		Target          string `json:"target,omitempty"`
		OrbitID         string `json:"orbit_id,omitempty"`
		State           string `json:"state,omitempty"`
		RecoveryCommand string `json:"recovery_command,omitempty"`
	}{
		Kind:            "member_hint_drift",
		Path:            "docs/rules/style.md",
		OrbitID:         "docs",
		State:           "create_new",
		RecoveryCommand: "hyard orbit content apply docs --check --json",
	})
	require.Contains(t, payload.NextActions, "hyard guide save --orbit docs --target agents")
	require.Contains(t, payload.NextActions, "hyard orbit content apply docs --check --json")
	require.FileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
}

func TestHyardViewRunCheckTextToleratesDirtyWorktreeAndRendersNextActions(t *testing.T) {
	t.Parallel()

	repo := seedHyardViewRunNoDriftRuntimeRepo(t)
	repo.WriteFile(t, "README.md", "local runtime notes\n")
	require.NotEmpty(t, repo.Run(t, "status", "--short"))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "view", "run", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.Contains(t, stdout, "check: true\n")
	require.Contains(t, stdout, "ready: true\n")
	require.Contains(t, stdout, "changed: false\n")
	require.Contains(t, stdout, "cleanup_candidates:\n")
	require.Contains(t, stdout, "  root_guidance_marker_lines AGENTS.md action=strip_marker_lines_preserve_content target=agents orbit=docs\n")
	require.Contains(t, stdout, "  member_hint docs/process/.orbit-member.yaml action=remove_consumed_hint orbit=docs\n")
	require.Contains(t, stdout, "blockers:\n")
	require.Contains(t, stdout, "  none\n")
	require.Contains(t, stdout, "next_actions:\n")
	require.Contains(t, stdout, "  run `hyard view run` to apply Run View cleanup\n")
	require.FileExists(t, filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
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

func seedHyardViewRunNoDriftRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs orbit guidance\n"
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Name:        "process",
		Description: "Review workflow",
		Role:        orbitpkg.OrbitMemberProcess,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/process/review.md", "# Review\n")
	repo.WriteFile(t, "docs/process/.orbit-member.yaml", ""+
		"orbit_member:\n"+
		"  description: Review workflow\n")

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs orbit guidance\n"))
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

	repo.AddAndCommit(t, "seed run view cleanup repo")

	return repo
}

func seedHyardViewRunNestedMarkdownMemberHintRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Name:        "review",
		Description: "Review workflow",
		Role:        orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"title: Review Flow\n"+
		"orbit_member:\n"+
		"  name: review\n"+
		"  description: Review workflow\n"+
		"tags:\n"+
		"  - process\n"+
		"---\n"+
		"\n"+
		"# Review\n")

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

	repo.AddAndCommit(t, "seed nested markdown member hint runtime repo")

	return repo
}

func seedHyardViewRunAmbiguousFlatMemberHintRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs orbit guidance\n"
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Name:        "review",
		Description: "Review workflow",
		Role:        orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"name: review\n"+
		"description: Review workflow\n"+
		"title: Review Flow\n"+
		"---\n"+
		"\n"+
		"# Review\n")
	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs orbit guidance\n"))
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

	repo.AddAndCommit(t, "seed ambiguous flat member hint runtime repo")

	return repo
}

func seedHyardViewRunRootGuidanceRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs orbit guidance\n"
	spec.Meta.HumansTemplate = "Human docs guidance\n"
	spec.Meta.BootstrapTemplate = "Bootstrap docs guidance\n"
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Name:        "process",
		Description: "Review workflow",
		Role:        orbitpkg.OrbitMemberProcess,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/process/review.md", "# Review\n")
	repo.WriteFile(t, "docs/process/.orbit-member.yaml", ""+
		"orbit_member:\n"+
		"  description: Review workflow\n")

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs orbit guidance\n"))
	require.NoError(t, err)
	harnessBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "workspace", []byte("Workspace harness guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", ""+
		"Global agents guidance\n"+
		string(agentsBlock)+
		string(harnessBlock)+
		"Tail agents guidance\n")

	humansBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Human docs guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(humansBlock))

	bootstrapBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap docs guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", string(bootstrapBlock))

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

	repo.AddAndCommit(t, "seed run view root guidance cleanup repo")

	return repo
}

func seedHyardViewRunDriftRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs orbit guidance\n"
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Name: "content",
		Role: orbitpkg.OrbitMemberSubject,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/**"},
		},
	}, orbitpkg.OrbitMember{
		Name:        "process",
		Description: "Old review workflow",
		Role:        orbitpkg.OrbitMemberProcess,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/process/review.md", "# Review\n")
	repo.WriteFile(t, "docs/process/.orbit-member.yaml", ""+
		"orbit_member:\n"+
		"  description: Review workflow\n")
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  description: Style rules\n"+
		"---\n"+
		"\n"+
		"# Style\n")

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance\n"))
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

	repo.AddAndCommit(t, "seed drifted run view cleanup repo")

	return repo
}
