package scoped

import (
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

func TestOutsideDiffPaths(t *testing.T) {
	t.Parallel()

	paths := outsideDiffPaths(
		[]string{"README.md", "docs/guide.md"},
		[]gitpkg.StatusEntry{
			{Path: "docs/guide.md", Code: "M", Tracked: true},
			{Path: "src/main.go", Code: "M", Tracked: true},
			{Path: "cmd/orbit/main.go", Code: "D", Tracked: true},
			{Path: "cmd/orbit/main.go", Code: "M", Tracked: true},
			{Path: "scratch/outside.txt", Code: "??", Tracked: false},
		},
	)

	require.Equal(t, []string{
		"cmd/orbit/main.go",
		"src/main.go",
	}, paths)
}
