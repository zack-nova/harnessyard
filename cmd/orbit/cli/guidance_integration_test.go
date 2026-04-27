package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestOrbitGuidanceMaterializeAllWritesAllRootArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\nKeep release notes current.\n",
		"Run the $project_name docs workflow.\n",
		"Bootstrap the $project_name docs orbit.\n",
		nil,
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		OrbitID       string `json:"orbit_id"`
		Target        string `json:"target"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target  string `json:"target"`
			Path    string `json:"path"`
			Changed bool   `json:"changed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 3, payload.ArtifactCount)

	targets := make([]string, 0, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		targets = append(targets, artifact.Target)
		require.True(t, artifact.Changed)
	}
	require.ElementsMatch(t, []string{"agents", "humans", "bootstrap"}, targets)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the Acme docs orbit.\n")

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Acme docs workflow.\n")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "Bootstrap the Acme docs orbit.\n")
}

func TestOrbitGuidanceMaterializeAllSkipsMissingAuthoredTruthByDefault(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\nKeep release notes current.\n",
		"",
		"",
		nil,
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target        string `json:"target"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target           string `json:"target"`
			Status           string `json:"status"`
			Reason           string `json:"reason"`
			Path             string `json:"path"`
			Changed          bool   `json:"changed"`
			SeedEmptyAllowed bool   `json:"seed_empty_allowed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 3, payload.ArtifactCount)

	artifactsByTarget := make(map[string]struct {
		Status           string
		Reason           string
		Path             string
		Changed          bool
		SeedEmptyAllowed bool
	}, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		artifactsByTarget[artifact.Target] = struct {
			Status           string
			Reason           string
			Path             string
			Changed          bool
			SeedEmptyAllowed bool
		}{
			Status:           artifact.Status,
			Reason:           artifact.Reason,
			Path:             artifact.Path,
			Changed:          artifact.Changed,
			SeedEmptyAllowed: artifact.SeedEmptyAllowed,
		}
	}

	require.Equal(t, "rendered", artifactsByTarget["agents"].Status)
	require.Equal(t, "authored_truth", artifactsByTarget["agents"].Reason)
	require.Equal(t, filepath.Join(repo.Root, "AGENTS.md"), artifactsByTarget["agents"].Path)
	require.True(t, artifactsByTarget["agents"].Changed)
	require.False(t, artifactsByTarget["agents"].SeedEmptyAllowed)

	require.Equal(t, "skipped_no_authored_truth", artifactsByTarget["humans"].Status)
	require.Equal(t, "no_authored_truth", artifactsByTarget["humans"].Reason)
	require.Equal(t, filepath.Join(repo.Root, "HUMANS.md"), artifactsByTarget["humans"].Path)
	require.False(t, artifactsByTarget["humans"].Changed)
	require.True(t, artifactsByTarget["humans"].SeedEmptyAllowed)

	require.Equal(t, "skipped_no_authored_truth", artifactsByTarget["bootstrap"].Status)
	require.Equal(t, "no_authored_truth", artifactsByTarget["bootstrap"].Reason)
	require.Equal(t, filepath.Join(repo.Root, "BOOTSTRAP.md"), artifactsByTarget["bootstrap"].Path)
	require.False(t, artifactsByTarget["bootstrap"].Changed)
	require.True(t, artifactsByTarget["bootstrap"].SeedEmptyAllowed)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the Acme docs orbit.\n")
	_, err = os.Stat(filepath.Join(repo.Root, "HUMANS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOrbitGuidanceMaterializeTextOutputUsesNeutralProcessedHeadline(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\n",
		"",
		"",
		nil,
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "processed orbit guidance docs for target all\n")
	require.Contains(t, stdout, "artifact: agents status=rendered reason=authored_truth")
	require.Contains(t, stdout, "artifact: humans status=skipped_no_authored_truth reason=no_authored_truth")
	require.NotContains(t, stdout, "materialized orbit guidance docs for target all")
}

func TestOrbitGuidanceMaterializeDefaultsToSourceBranchOrbitBeforeStaleCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\n",
		"",
		"",
		nil,
	)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "switch guidance repo to source")
	writeCurrentOrbitState(t, repo, "api")

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--target", "agents", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID string `json:"orbit_id"`
		Target  string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "agents", payload.Target)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the $project_name docs orbit.\n")
}

func TestOrbitGuidanceBackfillDefaultsToOrbitTemplateBranchOrbit(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", map[string]string{
		"AGENTS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"You are the Acme docs orbit.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-07T00:00:00Z\n")
	repo.AddAndCommit(t, "switch guidance repo to orbit template")

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--target", "agents", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID string `json:"orbit_id"`
		Target  string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "agents", payload.Target)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\n", spec.Meta.AgentsTemplate)
}

func TestOrbitGuidanceMaterializeRejectsMissingBootstrapTruthWithoutSeedEmpty(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", nil)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "bootstrap")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" does not have authored bootstrap guidance; rerun with --seed-empty to create an editable empty block`)
	require.NotContains(t, err.Error(), "materializable")
}

func TestOrbitGuidanceMaterializeExplicitMissingTargetSuggestsSeedEmpty(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", nil)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "humans")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" does not have authored human guidance; rerun with --seed-empty to create an editable empty block`)
	require.NotContains(t, err.Error(), "materializable")
}

func TestOrbitGuidanceMaterializeStrictAllFailsBeforeWritingPartialArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\nKeep release notes current.\n",
		"",
		"",
		nil,
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--strict")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" does not have authored human guidance; rerun with --seed-empty to create an editable empty block`)

	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "HUMANS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOrbitGuidanceMaterializeAllSkipsCompletedBootstrapByDefault(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\n",
		"Run the $project_name docs workflow.\n",
		"Bootstrap the $project_name docs orbit.\n",
		nil,
	)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 19, 11, 0, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 30, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Artifacts []struct {
			Target string `json:"target"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	statusByTarget := make(map[string]string, len(payload.Artifacts))
	reasonByTarget := make(map[string]string, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		statusByTarget[artifact.Target] = artifact.Status
		reasonByTarget[artifact.Target] = artifact.Reason
	}
	require.Equal(t, "rendered", statusByTarget["agents"])
	require.Equal(t, "rendered", statusByTarget["humans"])
	require.Equal(t, "skipped_bootstrap_closed", statusByTarget["bootstrap"])
	require.Equal(t, "bootstrap_completed", reasonByTarget["bootstrap"])

	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOrbitGuidanceMaterializeCheckReportsSeedEmptyEligibility(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t,
		"You are the $project_name docs orbit.\n",
		"",
		"",
		nil,
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target                   string `json:"target"`
			State                    string `json:"state"`
			Reason                   string `json:"reason"`
			SeedEmptyAllowed         bool   `json:"seed_empty_allowed"`
			MaterializeAllowed       bool   `json:"materialize_allowed"`
			MaterializeRequiresForce bool   `json:"materialize_requires_force"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Len(t, payload.Artifacts, 3)

	artifactsByTarget := make(map[string]struct {
		State                    string
		Reason                   string
		SeedEmptyAllowed         bool
		MaterializeAllowed       bool
		MaterializeRequiresForce bool
	}, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		artifactsByTarget[artifact.Target] = struct {
			State                    string
			Reason                   string
			SeedEmptyAllowed         bool
			MaterializeAllowed       bool
			MaterializeRequiresForce bool
		}{
			State:                    artifact.State,
			Reason:                   artifact.Reason,
			SeedEmptyAllowed:         artifact.SeedEmptyAllowed,
			MaterializeAllowed:       artifact.MaterializeAllowed,
			MaterializeRequiresForce: artifact.MaterializeRequiresForce,
		}
	}

	require.Equal(t, "structured_only", artifactsByTarget["agents"].State)
	require.Equal(t, "authored_truth", artifactsByTarget["agents"].Reason)
	require.False(t, artifactsByTarget["agents"].SeedEmptyAllowed)
	require.True(t, artifactsByTarget["agents"].MaterializeAllowed)

	require.Equal(t, "missing_truth", artifactsByTarget["humans"].State)
	require.Equal(t, "no_authored_truth", artifactsByTarget["humans"].Reason)
	require.True(t, artifactsByTarget["humans"].SeedEmptyAllowed)
	require.False(t, artifactsByTarget["humans"].MaterializeAllowed)
	require.False(t, artifactsByTarget["humans"].MaterializeRequiresForce)

	require.Equal(t, "missing_truth", artifactsByTarget["bootstrap"].State)
	require.Equal(t, "no_authored_truth", artifactsByTarget["bootstrap"].Reason)
	require.True(t, artifactsByTarget["bootstrap"].SeedEmptyAllowed)
	require.False(t, artifactsByTarget["bootstrap"].MaterializeAllowed)
	require.False(t, artifactsByTarget["bootstrap"].MaterializeRequiresForce)
}

func TestOrbitGuidanceMaterializeSeedEmptyCreatesEditableBlocksThatBackfillAllTemplates(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", nil)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--seed-empty", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		OrbitID       string `json:"orbit_id"`
		Target        string `json:"target"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target  string `json:"target"`
			Path    string `json:"path"`
			Changed bool   `json:"changed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 3, payload.ArtifactCount)

	for _, path := range []string{"AGENTS.md", "HUMANS.md", "BOOTSTRAP.md"} {
		data, readErr := os.ReadFile(filepath.Join(repo.Root, path))
		require.NoError(t, readErr)
		require.Contains(t, string(data), `<!-- orbit:begin orbit_id="docs" -->`)
		require.Contains(t, string(data), `<!-- orbit:end orbit_id="docs" -->`)
	}

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("You are the Acme docs orbit.\nKeep release notes current.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", "Workspace overview.\n"+string(agentsBlock))

	humansBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Run the Acme docs workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", "Workspace overview.\n"+string(humansBlock))

	bootstrapBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap the Acme docs orbit.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "Workspace overview.\n"+string(bootstrapBlock))

	_, stderr, err = executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
	require.Equal(t, "Run the $project_name docs workflow.\n", spec.Meta.HumansTemplate)
	require.Equal(t, "Bootstrap the $project_name docs orbit.\n", spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceMaterializeSeedEmptyAppendsMissingBlockToExistingArtifact(t *testing.T) {
	t.Parallel()

	apiBlock, err := orbittemplate.WrapRuntimeAgentsBlock("api", []byte("API guidance.\n"))
	require.NoError(t, err)
	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", map[string]string{
		"AGENTS.md": "Workspace overview.\n" + string(apiBlock),
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "agents", "--seed-empty", "--json")
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
	require.Equal(t, "seeded_empty", payload.Artifacts[0].Status)
	require.True(t, payload.Artifacts[0].Changed)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	agents := string(agentsData)
	require.Contains(t, agents, "Workspace overview.\n")
	require.Contains(t, agents, `<!-- orbit:begin orbit_id="api" -->`)
	require.Contains(t, agents, `<!-- orbit:begin orbit_id="docs" -->`)
	require.Equal(t, 1, strings.Count(agents, `<!-- orbit:begin orbit_id="docs" -->`))
}

func TestOrbitGuidanceMaterializeSeedEmptySkipsExistingDriftedBlock(t *testing.T) {
	t.Parallel()

	existingBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance.\n"))
	require.NoError(t, err)
	repo := seedOrbitGuidanceRevisionRepo(t, "Hosted docs guidance.\n", "", "", map[string]string{
		"AGENTS.md": "Workspace overview.\n" + string(existingBlock),
	})
	originalAgents, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "agents", "--seed-empty", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Artifacts []struct {
			Target  string `json:"target"`
			Status  string `json:"status"`
			Reason  string `json:"reason"`
			Changed bool   `json:"changed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "agents", payload.Artifacts[0].Target)
	require.Equal(t, "skipped_existing_block", payload.Artifacts[0].Status)
	require.Equal(t, "existing_block", payload.Artifacts[0].Reason)
	require.False(t, payload.Artifacts[0].Changed)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, string(originalAgents), string(agentsData))
	require.Equal(t, 1, strings.Count(string(agentsData), `<!-- orbit:begin orbit_id="docs" -->`))
}

func TestOrbitGuidanceMaterializeSeedEmptyTextReportsExistingBlockSkip(t *testing.T) {
	t.Parallel()

	existingBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance.\n"))
	require.NoError(t, err)
	repo := seedOrbitGuidanceRevisionRepo(t, "Hosted docs guidance.\n", "", "", map[string]string{
		"AGENTS.md": string(existingBlock),
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "agents", "--seed-empty")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "artifact: agents status=skipped_existing_block reason=existing_block")
	require.Contains(t, stdout, "changed=false")
}

func TestOrbitGuidanceBackfillAllDoesNotPersistEmptySeededTemplates(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", nil)

	_, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "all", "--seed-empty")
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target         string `json:"target"`
			Status         string `json:"status"`
			DefinitionPath string `json:"definition_path"`
			UpdatedField   string `json:"updated_field"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Len(t, payload.Artifacts, 3)
	for _, artifact := range payload.Artifacts {
		require.Equal(t, "skipped", artifact.Status)
		require.NotEmpty(t, artifact.UpdatedField)
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Empty(t, spec.Meta.AgentsTemplate)
	require.Empty(t, spec.Meta.HumansTemplate)
	require.Empty(t, spec.Meta.BootstrapTemplate)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "agents_template:")
	require.NotContains(t, string(data), "humans_template:")
	require.NotContains(t, string(data), "bootstrap_template:")
}

func TestOrbitGuidanceBackfillAgentsReportsSkippedWhenHostedTemplateAlreadyMatches(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", "", "", map[string]string{
		"AGENTS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"You are the Acme docs orbit.\n" +
			"Keep release notes current.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "agents", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target string `json:"target"`
			Status string `json:"status"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "agents", payload.Target)
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "agents", payload.Artifacts[0].Target)
	require.Equal(t, "skipped", payload.Artifacts[0].Status)
}

func TestOrbitGuidanceBackfillAllWritesAllHostedTemplates(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", map[string]string{
		"AGENTS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"You are the Acme docs orbit.\n" +
			"Keep release notes current.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
		"HUMANS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Run the Acme docs workflow.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
		"BOOTSTRAP.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Bootstrap the Acme docs orbit.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		OrbitID       string `json:"orbit_id"`
		Target        string `json:"target"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target       string `json:"target"`
			Status       string `json:"status"`
			UpdatedField string `json:"updated_field"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 3, payload.ArtifactCount)
	for _, artifact := range payload.Artifacts {
		require.Equal(t, "updated", artifact.Status)
		require.NotEmpty(t, artifact.UpdatedField)
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
	require.Equal(t, "Run the $project_name docs workflow.\n", spec.Meta.HumansTemplate)
	require.Equal(t, "Bootstrap the $project_name docs orbit.\n", spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceBackfillAllSkipsMissingBootstrapRootArtifact(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Keep existing bootstrap truth.\n", map[string]string{
		"AGENTS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"You are the Acme docs orbit.\n" +
			"Keep release notes current.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
		"HUMANS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Run the Acme docs workflow.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target         string `json:"target"`
			Status         string `json:"status"`
			DefinitionPath string `json:"definition_path"`
			UpdatedField   string `json:"updated_field"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Len(t, payload.Artifacts, 3)

	statusByTarget := make(map[string]string, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		statusByTarget[artifact.Target] = artifact.Status
		require.NotEmpty(t, artifact.DefinitionPath)
		require.NotEmpty(t, artifact.UpdatedField)
	}
	require.Equal(t, "updated", statusByTarget["agents"])
	require.Equal(t, "updated", statusByTarget["humans"])
	require.Equal(t, "skipped", statusByTarget["bootstrap"])

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
	require.Equal(t, "Run the $project_name docs workflow.\n", spec.Meta.HumansTemplate)
	require.Equal(t, "Keep existing bootstrap truth.\n", spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceBackfillAllSkipsMissingBlocksWithoutHostedTruth(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "", map[string]string{
		"HUMANS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Run the Acme docs workflow.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target       string `json:"target"`
			Status       string `json:"status"`
			UpdatedField string `json:"updated_field"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "all", payload.Target)
	require.Len(t, payload.Artifacts, 3)

	statusByTarget := make(map[string]string, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		statusByTarget[artifact.Target] = artifact.Status
		require.NotEmpty(t, artifact.UpdatedField)
	}
	require.Equal(t, "skipped", statusByTarget["agents"])
	require.Equal(t, "updated", statusByTarget["humans"])
	require.Equal(t, "skipped", statusByTarget["bootstrap"])

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Empty(t, spec.Meta.AgentsTemplate)
	require.Equal(t, "Run the $project_name docs workflow.\n", spec.Meta.HumansTemplate)
	require.Empty(t, spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceBackfillAllBlocksMissingBlockWithHostedTruth(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "Keep existing agents truth.\n", "", "", map[string]string{
		"AGENTS.md": "Workspace overview.\n",
		"HUMANS.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Run the Acme docs workflow.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "all", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md does not contain orbit block "docs"`)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "Keep existing agents truth.\n", spec.Meta.AgentsTemplate)
	require.Empty(t, spec.Meta.HumansTemplate)
	require.Empty(t, spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceBackfillBootstrapFailsWhenRootArtifactMissing(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Keep existing bootstrap truth.\n", map[string]string{})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "bootstrap")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "root BOOTSTRAP.md is missing")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "Keep existing bootstrap truth.\n", spec.Meta.BootstrapTemplate)
}

func TestOrbitGuidanceBackfillEmptyBootstrapBlockRemovesHostedTemplate(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Bootstrap the $project_name docs orbit.\n", map[string]string{
		"BOOTSTRAP.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "bootstrap")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "artifact: bootstrap status=removed updated_field=meta.bootstrap_template")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Empty(t, spec.Meta.BootstrapTemplate)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "bootstrap_template:")
}

func TestOrbitGuidanceBackfillCheckReportsBootstrapBackfillAllowedWhenInSync(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Bootstrap the $project_name docs orbit.\n", map[string]string{
		"BOOTSTRAP.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Bootstrap the Acme docs orbit.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "bootstrap", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target          string `json:"target"`
			State           string `json:"state"`
			BackfillAllowed bool   `json:"backfill_allowed"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "bootstrap", payload.Target)
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "bootstrap", payload.Artifacts[0].Target)
	require.Equal(t, "materialized_in_sync", payload.Artifacts[0].State)
	require.True(t, payload.Artifacts[0].BackfillAllowed)
}

func TestOrbitGuidanceMaterializeRejectsCompletedBootstrapInRuntime(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Bootstrap the $project_name docs orbit.\n", nil)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 19, 11, 0, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 30, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "bootstrap")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `bootstrap guidance for orbit "docs" is closed because bootstrap is already completed in this runtime`)
}

func TestOrbitGuidanceBackfillRejectsCompletedBootstrapInRuntime(t *testing.T) {
	t.Parallel()

	repo := seedOrbitGuidanceRevisionRepo(t, "", "", "Bootstrap the $project_name docs orbit.\n", map[string]string{
		"BOOTSTRAP.md": "" +
			"Workspace overview.\n" +
			"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
			"Bootstrap the Acme docs orbit.\n" +
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	})
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 19, 11, 0, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 30, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeCLI(t, repo.Root, "guidance", "backfill", "--orbit", "docs", "--target", "bootstrap")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `bootstrap guidance for orbit "docs" is closed because bootstrap is already completed in this runtime`)
}

func seedOrbitGuidanceRevisionRepo(t *testing.T, agentsTemplate string, humansTemplate string, bootstrapTemplate string, rootFiles map[string]string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = agentsTemplate
	spec.Meta.HumansTemplate = humansTemplate
	spec.Meta.BootstrapTemplate = bootstrapTemplate
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-18T00:00:00Z\n"+
		"  updated_at: 2026-04-18T00:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	for path, content := range rootFiles {
		repo.WriteFile(t, path, content)
	}
	repo.AddAndCommit(t, "seed orbit guidance revision repo")

	return repo
}

func writeCurrentOrbitState(t *testing.T, repo *testutil.Repo, orbitID string) {
	t.Helper()

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         orbitID,
		EnteredAt:     time.Date(2026, time.April, 24, 9, 30, 0, 0, time.UTC),
		SparseEnabled: true,
	}))
}
