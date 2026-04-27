package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestOrbitBriefMaterializeCheckReportsStructuredOnlyWithoutWritingContainer(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "runtime", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		"",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "state: structured_only\n")
	require.Contains(t, stdout, "revision_kind: runtime\n")
	require.Contains(t, stdout, "materialize.allowed: true\n")
	require.Contains(t, stdout, "materialize.requires_force: false\n")
	require.Contains(t, stdout, "backfill.allowed: false\n")

	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestOrbitBriefMaterializeCheckReportsInSyncJSON(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "runtime", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
			"You are the Acme docs orbit.\n"+
			"Keep release notes current.\n"+
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw,
		"agents_path",
		"backfill_allowed",
		"has_authored_truth",
		"has_orbit_block",
		"has_root_agents",
		"materialize_allowed",
		"materialize_requires_force",
		"orbit_id",
		"repo_root",
		"revision_kind",
		"state",
	)
	require.Equal(t, repo.Root, raw["repo_root"])
	require.Equal(t, "docs", raw["orbit_id"])
	require.Equal(t, "runtime", raw["revision_kind"])
	require.Equal(t, "materialized_in_sync", raw["state"])
	require.Equal(t, filepath.Join(repo.Root, "AGENTS.md"), raw["agents_path"])
	require.Equal(t, true, raw["has_authored_truth"])
	require.Equal(t, true, raw["has_root_agents"])
	require.Equal(t, true, raw["has_orbit_block"])
	require.Equal(t, true, raw["materialize_allowed"])
	require.Equal(t, false, raw["materialize_requires_force"])
	require.Equal(t, true, raw["backfill_allowed"])
}

func TestOrbitBriefMaterializeCheckReportsDriftedWithoutWriting(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "orbit_template", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
			"You are the Drifted docs orbit.\n"+
			"Keep release notes current.\n"+
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	)

	originalAgents, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "state: materialized_drifted\n")
	require.Contains(t, stdout, "revision_kind: orbit_template\n")
	require.Contains(t, stdout, "materialize.allowed: false\n")
	require.Contains(t, stdout, "materialize.requires_force: true\n")
	require.Contains(t, stdout, "backfill.allowed: true\n")

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, string(originalAgents), string(agentsData))
}

func TestOrbitBriefMaterializeCheckReportsInvalidContainer(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "source", ""+
		"You are the $project_name docs orbit.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
			"broken docs block\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "state: invalid_container\n")
	require.Contains(t, stdout, "materialize.allowed: false\n")
	require.Contains(t, stdout, "backfill.allowed: false\n")
}

func TestOrbitBriefMaterializeCheckReportsMissingTruthAndRecoverableBackfillJSON(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "runtime", "",
		""+
			"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
			"You are the Acme docs orbit.\n"+
			"Keep release notes current.\n"+
			"<!-- orbit:end orbit_id=\"docs\" -->\n",
	)
	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	spec.Description = ""
	require.NotNil(t, spec.Meta)
	spec.Meta.IncludeDescriptionInOrchestration = false
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Orchestration.IncludeOrbitDescription = false
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	require.Equal(t, "missing_truth", raw["state"])
	require.Equal(t, false, raw["has_authored_truth"])
	require.Equal(t, true, raw["has_orbit_block"])
	require.Equal(t, false, raw["materialize_allowed"])
	require.Equal(t, false, raw["materialize_requires_force"])
	require.Equal(t, true, raw["backfill_allowed"])
}

func TestOrbitBriefMaterializeCheckRejectsPlainRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "", "You are the $project_name docs orbit.\n", "")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--check")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief materialize supports only runtime, source, or orbit_template revisions; current revision kind is "plain"`)
}
