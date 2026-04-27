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

func TestDiscoverRepoAndTrackedFiles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide with space.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	subdir := filepath.Join(repo.Root, "docs")
	require.NoError(t, os.MkdirAll(subdir, 0o750))

	ctx := context.Background()

	discoveredRepo, err := gitpkg.DiscoverRepo(ctx, subdir)
	require.NoError(t, err)
	require.Equal(t, repo.Root, discoveredRepo.Root)
	require.Equal(t, repo.GitDir(t), discoveredRepo.GitDir)

	trackedFiles, err := gitpkg.TrackedFiles(ctx, repo.Root)
	require.NoError(t, err)
	require.Equal(t, []string{
		"README.md",
		"docs/guide with space.md",
	}, trackedFiles)
}

func TestWorktreeFilesIncludesTrackedAndUntrackedButExcludesIgnoredAndMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".gitignore", "docs/ignored.md\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	require.NoError(t, os.Remove(filepath.Join(repo.Root, "README.md")))
	repo.WriteFile(t, "docs/draft.md", "draft\n")
	repo.WriteFile(t, "docs/ignored.md", "ignored\n")

	files, err := gitpkg.WorktreeFiles(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, []string{
		".gitignore",
		"docs/draft.md",
	}, files)
}
