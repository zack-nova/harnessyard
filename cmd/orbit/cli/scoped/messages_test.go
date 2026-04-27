package scoped

import "testing"

import "github.com/stretchr/testify/require"

func TestBuildCommitMessageWithTrailer(t *testing.T) {
	t.Parallel()

	message := BuildCommitMessage("  docs commit  ", "docs", true)
	require.Equal(t, "docs commit\n\nOrbit: docs", message)
}

func TestBuildCommitMessageWithoutTrailer(t *testing.T) {
	t.Parallel()

	message := BuildCommitMessage("  docs commit  ", "docs", false)
	require.Equal(t, "docs commit", message)
}

func TestBuildRestoreCommitMessage(t *testing.T) {
	t.Parallel()

	message := BuildRestoreCommitMessage("docs", "abc123")
	require.Equal(t, "restore docs orbit to abc123\n\nOrbit: docs\nOrbit-Restore-From: abc123", message)
}
