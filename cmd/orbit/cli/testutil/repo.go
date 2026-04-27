package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testDirectoryPerm = 0o750
	testFilePerm      = 0o600
)

// Repo is an isolated temporary Git repository for tests.
type Repo struct {
	Root          string
	homeDir       string
	xdgConfigHome string
}

// NewRepo creates a temp Git repository with local user configuration.
func NewRepo(t *testing.T) *Repo {
	t.Helper()

	rootDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	repo := &Repo{
		Root:          rootDir,
		homeDir:       filepath.Join(rootDir, ".home"),
		xdgConfigHome: filepath.Join(rootDir, ".xdg"),
	}

	require.NoError(t, os.MkdirAll(repo.homeDir, testDirectoryPerm))
	require.NoError(t, os.MkdirAll(repo.xdgConfigHome, testDirectoryPerm))

	repo.Run(t, "init")
	repo.Run(t, "config", "user.name", "Orbit Test")
	repo.Run(t, "config", "user.email", "orbit@example.com")

	return repo
}

// GitDir returns the absolute git dir path for the temp repository.
func (repo *Repo) GitDir(t *testing.T) string {
	t.Helper()

	return strings.TrimSpace(repo.Run(t, "rev-parse", "--absolute-git-dir"))
}

// Run executes a git command in the temp repository and returns stdout.
func (repo *Repo) Run(t *testing.T, args ...string) string {
	t.Helper()

	//nolint:gosec // Test helper invokes the fixed git binary with explicit argument lists.
	command := exec.Command("git", args...)
	command.Dir = repo.Root
	command.Env = repo.env()

	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(output))

	return string(output)
}

// WriteFile writes a repo-relative file and creates parent directories as needed.
func (repo *Repo) WriteFile(t *testing.T, relativePath string, contents string) {
	t.Helper()

	absolutePath := filepath.Join(repo.Root, filepath.FromSlash(relativePath))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolutePath), testDirectoryPerm))
	require.NoError(t, os.WriteFile(absolutePath, []byte(contents), testFilePerm))
}

// AddAndCommit stages the requested paths and creates a commit.
func (repo *Repo) AddAndCommit(t *testing.T, message string, paths ...string) {
	t.Helper()

	if len(paths) == 0 {
		repo.Run(t, "add", "-A")
	} else {
		args := append([]string{"add", "--"}, paths...)
		repo.Run(t, args...)
	}

	repo.Run(t, "commit", "-m", message)
}

func (repo *Repo) env() []string {
	return append(
		os.Environ(),
		"HOME="+repo.homeDir,
		"XDG_CONFIG_HOME="+repo.xdgConfigHome,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}
