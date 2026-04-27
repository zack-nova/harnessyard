package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestComposeRuntimeGuidanceScopedIgnoresUnrelatedDriftedOrbitBlocks(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeGuidanceComposeRepo(t)

	cmdBlock, err := orbittemplate.WrapRuntimeAgentsBlock("cmd", []byte("Run the Drifted Acme cmd workflow.\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "HUMANS.md"), []byte("Workspace guidance.\n"+string(cmdBlock)), 0o600))

	result, err := ComposeRuntimeGuidance(context.Background(), ComposeRuntimeGuidanceInput{
		RepoRoot: repo.Root,
		Target:   GuidanceTargetHumans,
		OrbitIDs: []string{"docs"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.MemberCount)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, GuidanceTargetHumans, result.Artifacts[0].Target)
	require.Equal(t, filepath.Join(repo.Root, "HUMANS.md"), result.Artifacts[0].Path)
	require.Equal(t, []string{"docs"}, result.Artifacts[0].ComposedOrbitIDs)
	require.Empty(t, result.Artifacts[0].SkippedOrbitIDs)
	require.Equal(t, 1, result.Artifacts[0].ChangedCount)

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Drifted Acme cmd workflow.\n")
	require.Contains(t, string(humansData), "<!-- orbit:begin orbit_id=\"docs\" -->\nRun the Acme docs workflow.\n<!-- orbit:end orbit_id=\"docs\" -->\n")
}

func TestComposeRuntimeGuidanceScopedRejectsUnknownOrbitID(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeGuidanceComposeRepo(t)

	_, err := ComposeRuntimeGuidance(context.Background(), ComposeRuntimeGuidanceInput{
		RepoRoot: repo.Root,
		Target:   GuidanceTargetHumans,
		OrbitIDs: []string{"missing"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit_id "missing" is not a current runtime member`)
}

func seedRuntimeGuidanceComposeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	_, err := BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")

	docsSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	require.NotNil(t, docsSpec.Meta)
	docsSpec.Meta.HumansTemplate = "Run the $project_name docs workflow.\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, docsSpec)
	require.NoError(t, err)

	cmdSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("cmd")
	require.NoError(t, err)
	require.NotNil(t, cmdSpec.Meta)
	cmdSpec.Meta.HumansTemplate = "Run the $project_name cmd workflow.\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, cmdSpec)
	require.NoError(t, err)

	_, err = AddManualMember(context.Background(), repo.Root, "docs", now)
	require.NoError(t, err)
	_, err = AddManualMember(context.Background(), repo.Root, "cmd", now.Add(time.Minute))
	require.NoError(t, err)

	return repo
}
