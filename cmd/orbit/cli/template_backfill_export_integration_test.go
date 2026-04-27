package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplateSaveFailsClosedOnDriftedBriefWithoutBackfillFlag(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRepoWithDriftedBrief(t)

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
	require.ErrorContains(t, err, "--backfill-brief")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplateSaveBackfillsDriftedBriefBeforeSaving(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRepoWithDriftedBrief(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--backfill-brief", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		TargetBranch string   `json:"target_branch"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.TargetBranch)
	require.Contains(t, payload.Warnings, "auto-backfilled orbit brief docs into "+filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "Drifted docs orbit for $project_name")

	publishedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.Contains(t, string(publishedData), "Drifted docs orbit for $project_name")
}

func TestTemplateSaveDryRunRejectsBackfillBriefMutation(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRepoWithDriftedBrief(t)

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run", "--backfill-brief")
	require.Error(t, err)
	require.ErrorContains(t, err, "--dry-run cannot be combined with --backfill-brief")

	definitionData, readErr := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, readErr)
	require.Contains(t, string(definitionData), "Docs orbit for $project_name")
	require.NotContains(t, string(definitionData), "Drifted docs orbit for $project_name")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplatePublishFailsClosedOnSourceBriefDriftWithoutBackfillFlag(t *testing.T) {
	t.Parallel()

	repo := seedSourceRepoWithDriftedBrief(t)

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
	require.ErrorContains(t, err, "--backfill-brief")
}

func TestTemplatePublishBackfillsDriftedSourceBriefBeforePublishing(t *testing.T) {
	t.Parallel()

	repo := seedSourceRepoWithDriftedBrief(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--backfill-brief", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		Branch       string   `json:"branch"`
		SourceBranch string   `json:"source_branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.Contains(t, payload.Warnings, "auto-backfilled orbit brief docs into "+filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "Drifted docs orbit guidance")

	publishedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.Contains(t, string(publishedData), "Drifted docs orbit guidance")
}

func seedRuntimeRepoWithDriftedBrief(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	writeHostedDocsOrbitWithStructuredBrief(t, repo.Root, "Docs orbit for $project_name\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-17T10:00:00Z\n"+
		"  updated_at: 2026-04-17T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	agentsData, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Drifted docs orbit for Acme\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo with drifted brief")

	return repo
}

func seedSourceRepoWithDriftedBrief(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	writeHostedDocsOrbitWithStructuredBrief(t, repo.Root, "Docs orbit guidance\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	agentsData, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Drifted docs orbit guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed source repo with drifted brief")

	return repo
}

func writeHostedDocsOrbitWithStructuredBrief(t *testing.T, repoRoot string, agentsTemplate string) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = agentsTemplate
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
		orbitpkg.OrbitMemberRule,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repoRoot, spec)
	require.NoError(t, err)
}
