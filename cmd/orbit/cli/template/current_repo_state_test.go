package orbittemplate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestLoadCurrentRepoStateReturnsPlainForRepoWithoutManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	state, err := LoadCurrentRepoState(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, "plain", state.Kind)
	require.Empty(t, state.OrbitID)
	require.Empty(t, state.HarnessID)
	require.Equal(t, "main", state.CurrentBranch)
	require.False(t, state.Detached)
	require.False(t, state.HeadExists)
}

func TestLoadCurrentRepoStateReadsHiddenManifestFromHead(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: demo\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "seed runtime manifest")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	state, err := LoadCurrentRepoState(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, "runtime", state.Kind)
	require.Empty(t, state.OrbitID)
	require.Equal(t, "demo", state.HarnessID)
	require.Equal(t, "main", state.CurrentBranch)
	require.False(t, state.Detached)
	require.True(t, state.HeadExists)
}

func TestLoadCurrentRepoStateMarksDetachedHead(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n")
	repo.AddAndCommit(t, "seed source manifest")
	repo.Run(t, "checkout", "--detach", "HEAD")

	state, err := LoadCurrentRepoState(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, "source", state.Kind)
	require.Equal(t, "docs", state.OrbitID)
	require.Empty(t, state.HarnessID)
	require.Empty(t, state.CurrentBranch)
	require.True(t, state.Detached)
	require.True(t, state.HeadExists)
}

func TestRequireCurrentBranchAllowsUnbornBranch(t *testing.T) {
	t.Parallel()

	branch, err := RequireCurrentBranch(CurrentRepoState{CurrentBranch: "main"}, "template init")
	require.NoError(t, err)
	require.Equal(t, "main", branch)
}

func TestRequireCurrentBranchRejectsDetachedHead(t *testing.T) {
	t.Parallel()

	_, err := RequireCurrentBranch(CurrentRepoState{Detached: true}, "template init")
	require.ErrorContains(t, err, "template init requires a current branch; detached HEAD is not supported")
}

func TestCurrentCommitOrZeroReturnsZeroWhenHeadMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	commit, err := CurrentCommitOrZero(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, ZeroGitCommitID, commit)
}

func TestCurrentCommitOrZeroReturnsHeadCommitWhenPresent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "seed commit")

	commit, err := CurrentCommitOrZero(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, repo.Run(t, "rev-parse", "HEAD")[:40], commit)
}
