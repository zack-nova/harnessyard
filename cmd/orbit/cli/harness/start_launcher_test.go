package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultStartLauncherLaunchesCodexInteractiveTUIWithVerifiedContract(t *testing.T) {
	repoRoot := t.TempDir()
	recordDir := t.TempDir()
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' 'codex-cli 0.128.0'
  exit 0
fi

printf '%s' "$PWD" > "$HARNESSYARD_CODEX_LAUNCH_RECORD_DIR/cwd"
printf '%s' "$HARNESSYARD_CODEX_LAUNCH_SENTINEL" > "$HARNESSYARD_CODEX_LAUNCH_RECORD_DIR/env"
printf '%s' "$#" > "$HARNESSYARD_CODEX_LAUNCH_RECORD_DIR/argc"
i=0
for arg do
  i=$((i + 1))
  printf '%s' "$arg" > "$HARNESSYARD_CODEX_LAUNCH_RECORD_DIR/arg_$i"
done
`)
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HARNESSYARD_CODEX_LAUNCH_RECORD_DIR", recordDir)
	t.Setenv("HARNESSYARD_CODEX_LAUNCH_SENTINEL", "inherited")

	startPrompt := "Harness Start\nPreserve this as one argv argument."
	result, err := DefaultStartLauncher().Launch(context.Background(), StartLaunchRequest{
		RepoRoot:    repoRoot,
		GitDir:      filepath.Join(repoRoot, ".git"),
		HarnessID:   "demo-runtime",
		Framework:   "codex",
		StartPrompt: startPrompt,
	})
	require.NoError(t, err)

	require.Equal(t, "codex", result.Framework)
	require.Equal(t, "launched", result.Status)
	require.True(t, result.Launchable)
	require.Empty(t, result.ManualFallbackInstructions)
	require.Equal(t, "installed_cli", string(result.DetectionStatus))
	require.True(t, result.TerminalCLIDetected)

	require.Equal(t, canonicalLaunchPath(t, repoRoot), canonicalLaunchPath(t, readLaunchRecord(t, recordDir, "cwd")))
	require.Equal(t, "inherited", readLaunchRecord(t, recordDir, "env"))
	require.Equal(t, "7", readLaunchRecord(t, recordDir, "argc"))
	require.Equal(t, "--cd", readLaunchRecord(t, recordDir, "arg_1"))
	require.Equal(t, repoRoot, readLaunchRecord(t, recordDir, "arg_2"))
	require.Equal(t, "--sandbox", readLaunchRecord(t, recordDir, "arg_3"))
	require.Equal(t, "workspace-write", readLaunchRecord(t, recordDir, "arg_4"))
	require.Equal(t, "--ask-for-approval", readLaunchRecord(t, recordDir, "arg_5"))
	require.Equal(t, "on-request", readLaunchRecord(t, recordDir, "arg_6"))
	require.Equal(t, startPrompt, readLaunchRecord(t, recordDir, "arg_7"))
}

func readLaunchRecord(t *testing.T, recordDir string, name string) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(recordDir, name))
	require.NoError(t, err)

	return string(contents)
}

func canonicalLaunchPath(t *testing.T, path string) string {
	t.Helper()

	canonical, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)

	return canonical
}
