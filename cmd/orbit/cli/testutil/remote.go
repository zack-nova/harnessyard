package testutil

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// NewBareRemoteFromRepo clones a source repo into a local bare remote fixture.
func NewBareRemoteFromRepo(t *testing.T, source *Repo) string {
	t.Helper()

	bareRoot := filepath.Join(t.TempDir(), "remote.git")

	//nolint:gosec // Test helper invokes the fixed git binary with explicit argument lists.
	command := exec.Command("git", "clone", "--bare", source.Root, bareRoot)
	command.Env = source.env()

	output, err := command.CombinedOutput()
	require.NoError(t, err, "git clone --bare failed:\n%s", string(output))

	return bareRoot
}

// RevParse resolves one revision in the test repo.
func (repo *Repo) RevParse(t *testing.T, revision string) string {
	t.Helper()

	return strings.TrimSpace(repo.Run(t, "rev-parse", revision))
}
