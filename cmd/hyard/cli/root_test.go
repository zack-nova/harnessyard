package cli

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCommandUsesHyardBinaryName(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	require.Equal(t, "hyard", rootCmd.Use)
	require.True(t, rootCmd.SilenceUsage)
	require.True(t, rootCmd.SilenceErrors)
}

func TestRootCommandHelpRendersHarnessYardHeadline(t *testing.T) {
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
	require.Contains(t, stdout.String(), "Harness Yard CLI (hyard)")
}

func TestRootCommandVersionRendersBuildMetadata(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	rootCmd.SetArgs([]string{"--version"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "hyard dev")
	require.Contains(t, stdout.String(), "commit: none")
	require.Contains(t, stdout.String(), "built by: unknown")
}

func TestErrorExitCodeUnwrapsHookRunExitCode(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("wrap: %w", hookRunExitError{code: 2})

	exitCode, ok := ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 2, exitCode)
}
