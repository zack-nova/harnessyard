package git

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePorcelainV1Z(t *testing.T) {
	t.Parallel()

	output := []byte("" +
		" M docs/guide.md\x00" +
		"D  docs/old.md\x00" +
		"?? scratch/outside.txt\x00" +
		"R  docs/old name.md\x00docs/new name.md\x00")

	entries, err := parsePorcelainV1Z(output)
	require.NoError(t, err)
	require.Equal(t, []StatusEntry{
		{Path: "docs/guide.md", Code: "M", Tracked: true, Staged: false},
		{Path: "docs/old.md", Code: "D", Tracked: true, Staged: true},
		{Path: "scratch/outside.txt", Code: "??", Tracked: false, Staged: false},
		{Path: "docs/new name.md", Code: "R", Tracked: true, Staged: true},
	}, entries)
}

func TestParsePorcelainV1ZRejectsMissingRenameDestination(t *testing.T) {
	t.Parallel()

	_, err := parsePorcelainV1Z([]byte("R  docs/old.md\x00"))
	require.Error(t, err)
	require.ErrorContains(t, err, "missing rename destination")
}
