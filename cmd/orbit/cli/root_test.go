package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCommandUsesOrbitBinaryName(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	require.Equal(t, "orbit", rootCmd.Use)
	require.True(t, rootCmd.SilenceUsage)
	require.True(t, rootCmd.SilenceErrors)
}

func TestRootCommandHelpMentionsCompatibilityAndHyard(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	rootCmd.SetArgs([]string{"--help"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "orbit")
	require.Contains(t, stdout.String(), "hyard")
	require.Contains(t, stdout.String(), "compatibility")
}
