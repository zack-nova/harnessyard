package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestControlFileHelpersReadHiddenFilesFromHEAD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\nshared_scope:\n  - README.md\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/cmd.yaml", "id: cmd\ninclude:\n  - cmd/**\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "add orbit control plane")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	ctx := context.Background()

	filesAtHead, err := gitpkg.ListFilesAtRev(ctx, repo.Root, "HEAD", ".orbit/orbits")
	require.NoError(t, err)
	require.Equal(t, []string{
		".orbit/orbits/cmd.yaml",
		".orbit/orbits/docs.yaml",
	}, filesAtHead)

	exists, err := gitpkg.PathExistsAtRev(ctx, repo.Root, "HEAD", ".orbit/config.yaml")
	require.NoError(t, err)
	require.True(t, exists)

	headData, err := gitpkg.ReadFileAtRev(ctx, repo.Root, "HEAD", ".orbit/config.yaml")
	require.NoError(t, err)
	require.Contains(t, string(headData), "shared_scope:")

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	fallbackData, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repo.Root, ".orbit/config.yaml")
	require.NoError(t, err)
	require.Equal(t, headData, fallbackData)
}
