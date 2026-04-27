package git_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestListRemoteHeadsReturnsStableSortedHeads(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, "README.md", "remote fixture\n")
	source.AddAndCommit(t, "seed remote repo")
	source.Run(t, "branch", "-M", "zeta")
	source.Run(t, "branch", "beta")
	source.Run(t, "branch", "alpha")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	heads, err := gitpkg.ListRemoteHeads(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, []gitpkg.RemoteHead{
		{
			Name:   "alpha",
			Ref:    "refs/heads/alpha",
			Commit: source.RevParse(t, "alpha"),
		},
		{
			Name:   "beta",
			Ref:    "refs/heads/beta",
			Commit: source.RevParse(t, "beta"),
		},
		{
			Name:   "zeta",
			Ref:    "refs/heads/zeta",
			Commit: source.RevParse(t, "zeta"),
		},
	}, heads)
}

func TestResolveRemoteDefaultBranchReturnsHeadSymref(t *testing.T) {
	t.Parallel()

	source := testutil.NewRepo(t)
	source.WriteFile(t, "README.md", "remote fixture\n")
	source.AddAndCommit(t, "seed remote repo")
	source.Run(t, "branch", "-M", "main")
	source.Run(t, "branch", "beta")

	remoteURL := testutil.NewBareRemoteFromRepo(t, source)
	runtimeRepo := testutil.NewRepo(t)

	head, err := gitpkg.ResolveRemoteDefaultBranch(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Equal(t, gitpkg.RemoteHead{
		Name:   "main",
		Ref:    "refs/heads/main",
		Commit: source.RevParse(t, "main"),
	}, head)
}
