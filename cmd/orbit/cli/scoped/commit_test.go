package scoped

import (
	"testing"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestCommitPathspecIncludesTrackedScopeAndInScopeUntracked(t *testing.T) {
	t.Parallel()

	paths := commitPathspec(
		[]string{"README.md", "docs/guide.md"},
		[]statepkg.PathChange{
			{Path: "docs/new.md", InScope: true, Tracked: false},
			{Path: "docs/guide.md", InScope: true, Tracked: true},
		},
	)

	require.Equal(t, []string{"README.md", "docs/guide.md", "docs/new.md"}, paths)
}
