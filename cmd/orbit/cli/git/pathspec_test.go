package git_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

func TestCreatePathspecFileNormalizesSortsAndDedupes(t *testing.T) {
	t.Parallel()

	pathspecFile, err := gitpkg.CreatePathspecFile([]string{
		"docs/guide.md",
		`docs\guide.md`,
		"README.md",
		"./docs/extra.md",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pathspecFile.Cleanup())
	})

	data, err := os.ReadFile(pathspecFile.Path)
	require.NoError(t, err)
	require.Equal(t, []byte("README.md\x00docs/extra.md\x00docs/guide.md\x00"), data)
}
