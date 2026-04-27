package orbittemplate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestInitSourceBranchLeavesLegacyFilesWhenSourceManifestWriteFails(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-31T00:00:00Z\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed legacy source authoring repo")

	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".harness", "manifest.yaml"), 0o755))

	_, err := InitSourceBranch(context.Background(), repo.Root)
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")

	_, statErr := os.Stat(filepath.Join(repo.Root, ".orbit", "orbits", "docs.yaml"))
	require.NoError(t, statErr)
	_, statErr = os.Stat(filepath.Join(repo.Root, ".orbit", "template.yaml"))
	require.NoError(t, statErr)
	_, statErr = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	info, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, statErr)
	require.True(t, info.IsDir())
}
