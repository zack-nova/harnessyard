package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestOrbitBriefBackfillCheckReportsStructuredOnlyWithoutWritingTruth(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "runtime", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		"",
	)

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "state: structured_only\n")
	require.Contains(t, stdout, "revision_kind: runtime\n")
	require.Contains(t, stdout, "materialize.allowed: true\n")
	require.Contains(t, stdout, "backfill.allowed: false\n")

	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore.Meta.AgentsTemplate, specAfter.Meta.AgentsTemplate)
}

func TestOrbitBriefBackfillCheckReportsMissingTruthJSONWithoutWritingTruth(t *testing.T) {
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

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs", "--check", "--json")
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
	require.Equal(t, "missing_truth", raw["state"])
	require.Equal(t, "runtime", raw["revision_kind"])
	require.Equal(t, false, raw["has_authored_truth"])
	require.Equal(t, true, raw["has_orbit_block"])
	require.Equal(t, true, raw["backfill_allowed"])
	require.Equal(t, false, raw["materialize_allowed"])

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Empty(t, specAfter.Meta.AgentsTemplate)
}

func TestOrbitBriefBackfillCheckReportsInvalidContainer(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "source", ""+
		"You are the $project_name docs orbit.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
			"broken docs block\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs", "--check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "state: invalid_container\n")
	require.Contains(t, stdout, "materialize.allowed: false\n")
	require.Contains(t, stdout, "backfill.allowed: false\n")
}

func TestOrbitBriefBackfillCheckRejectsPlainRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "", "You are the $project_name docs orbit.\n", "")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs", "--check")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief backfill supports only runtime, source, or orbit_template revisions; current revision kind is "plain"`)
}
