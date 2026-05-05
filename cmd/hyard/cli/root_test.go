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

func TestPublishHelpFramesRuntimePublicationAsDefaultPath(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	rootCmd.SetArgs([]string{"publish", "--help"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "Runtime users should publish the current runtime as a Harness Package with `hyard publish harness <package>`.")
	require.Contains(t, stdout.String(), "Use `hyard publish orbit <package>` from Author View or compatibility authoring flows.")
}

func TestPublishOrbitHelpRequiresAuthorViewForRuntimeRepos(t *testing.T) {
	t.Parallel()

	rootCmd := NewRootCommand()
	rootCmd.SetArgs([]string{"publish", "orbit", "--help"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	require.Empty(t, stderr.String())
	require.Contains(t, stdout.String(), "In a Harness Runtime repository, select Author View with `hyard view author` before publishing an Orbit Package.")
	require.Contains(t, stdout.String(), "Run View users should publish the current runtime with `hyard publish harness <package>`.")
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
