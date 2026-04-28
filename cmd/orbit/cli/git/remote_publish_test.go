package git_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestCompareBranchToRemoteBranchReportsMissingRemoteBranch(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.Run(t, "branch", "-m", "main")
	source.WriteFile(t, "README.md", "source\n")
	source.AddAndCommit(t, "seed source")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	deleteBareRemoteRef(t, remoteURL, "refs/heads/main")

	relation, err := gitpkg.CompareBranchToRemoteBranch(context.Background(), source.Root, remoteURL, "main")
	require.NoError(t, err)
	require.Equal(t, gitpkg.BranchRelationMissing, relation)
}

func TestCompareBranchToRemoteBranchFullHistoryReportsBehind(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.Run(t, "branch", "-m", "main")
	source.WriteFile(t, "README.md", "source\n")
	source.AddAndCommit(t, "seed source")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	clone := testutil.NewRepo(t)
	clone.Run(t, "remote", "add", "origin", remoteURL)
	clone.Run(t, "fetch", "origin", "main")
	clone.Run(t, "checkout", "-B", "main", "FETCH_HEAD")
	clone.WriteFile(t, "README.md", "remote source\n")
	clone.AddAndCommit(t, "advance remote source")
	clone.Run(t, "push", "origin", "main")

	relation, err := gitpkg.CompareBranchToRemoteBranchFullHistory(context.Background(), source.Root, remoteURL, "main")
	require.NoError(t, err)
	require.Equal(t, gitpkg.BranchRelationBehind, relation)
}

func deleteBareRemoteRef(t *testing.T, remoteURL string, ref string) {
	t.Helper()

	command := exec.Command("git", "--git-dir", remoteURL, "update-ref", "-d", ref)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "delete bare remote ref failed:\n%s", string(output))
}
