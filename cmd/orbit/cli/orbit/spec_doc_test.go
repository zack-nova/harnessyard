package orbit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestDefaultSpecMemberIncludesSpecDocAndRuleDirectory(t *testing.T) {
	t.Parallel()

	member, err := orbitpkg.DefaultSpecMember("docs")
	require.NoError(t, err)

	require.Equal(t, "spec", member.Name)
	require.Equal(t, orbitpkg.OrbitMemberRule, member.Role)
	require.Equal(t, []string{"docs/docs.md", "docs/docs/**"}, member.Paths.Include)
}

func TestWriteSpecScaffoldWritesSpecDocAndRuleDirectoryReadme(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()

	filename, err := orbitpkg.WriteSpecScaffold(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, "docs", "docs.md"), filename)

	specDocData, err := os.ReadFile(filepath.Join(repoRoot, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))

	readmeData, err := os.ReadFile(filepath.Join(repoRoot, "docs", "docs", "README.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs\n", string(readmeData))
	require.NotContains(t, string(readmeData), "orbit_member")
}

func TestPreflightSpecScaffoldFailsWhenRuleDirectoryExists(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "docs"), 0o755))

	err := orbitpkg.PreflightSpecScaffold(repoRoot, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "spec doc directory")
	require.ErrorContains(t, err, "already exists")
}
