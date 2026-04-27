package view

import (
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

func TestHiddenDirtyPaths(t *testing.T) {
	t.Parallel()

	hidden := hiddenDirtyPaths(
		[]string{"README.md", "docs/guide.md"},
		[]gitpkg.StatusEntry{
			{Path: "docs/guide.md", Code: "M", Tracked: true},
			{Path: "src/main.go", Code: "M", Tracked: true},
			{Path: "cmd/orbit/main.go", Code: "D", Tracked: true},
			{Path: "internal/new.go", Code: "A", Tracked: true},
			{Path: "scratch/outside.txt", Code: "??", Tracked: false},
		},
	)

	require.Equal(t, []string{
		"cmd/orbit/main.go",
		"internal/new.go",
		"src/main.go",
	}, hidden)
}
