package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func stubCodexExecutableOnPath(t *testing.T) {
	t.Helper()

	gitExecutable, err := exec.LookPath("git")
	require.NoError(t, err)

	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\nprintf '%s\\n' 'codex test stub'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	gitPath := filepath.Join(binDir, "git")
	require.NoError(t, os.WriteFile(gitPath, []byte("#!/bin/sh\nexec "+strconv.Quote(gitExecutable)+" \"$@\"\n"), 0o700))
	require.NoError(t, os.Chmod(gitPath, 0o700))
	t.Setenv("PATH", binDir)
}
