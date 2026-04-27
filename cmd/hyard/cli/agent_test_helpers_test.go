package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func stubCodexExecutableOnPath(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	require.NoError(t, os.WriteFile(codexPath, []byte("#!/bin/sh\nprintf '%s\\n' 'codex test stub'\n"), 0o700))
	require.NoError(t, os.Chmod(codexPath, 0o700))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
