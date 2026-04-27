package harness

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHarnessRepoPathsAreStable(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()

	require.Equal(t, ".harness/manifest.yaml", ManifestRepoPath())
	require.Equal(t, filepath.Join(repoRoot, ".harness", "manifest.yaml"), ManifestPath(repoRoot))
	require.Equal(t, ".harness/vars.yaml", VarsRepoPath())
	require.Equal(t, filepath.Join(repoRoot, ".harness", "vars.yaml"), VarsPath(repoRoot))
	require.Equal(t, ".harness/orbits", OrbitSpecsDirRepoPath())
	require.Equal(t, filepath.Join(repoRoot, ".harness", "orbits"), OrbitSpecsDirPath(repoRoot))
	require.Equal(t, ".harness/installs", InstallRecordsDirRepoPath())
	require.Equal(t, filepath.Join(repoRoot, ".harness", "installs"), InstallRecordsDirPath(repoRoot))
	require.Equal(t, ".harness/template.yaml", TemplateRepoPath())
	require.Equal(t, filepath.Join(repoRoot, ".harness", "template.yaml"), TemplatePath(repoRoot))
}

func TestInstallRecordPathsValidateOrbitID(t *testing.T) {
	t.Parallel()

	_, err := InstallRecordRepoPath("Docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "validate orbit id")
}

func TestInstallRecordPathBuildsHarnessInstallLocation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()

	repoPath, err := InstallRecordRepoPath("docs")
	require.NoError(t, err)
	require.Equal(t, ".harness/installs/docs.yaml", repoPath)

	filename, err := InstallRecordPath(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".harness", "installs", "docs.yaml"), filename)
}

func TestOrbitSpecPathsValidateOrbitIDAndBuildHarnessLocation(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()

	_, err := OrbitSpecRepoPath("Docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "validate orbit id")

	repoPath, err := OrbitSpecRepoPath("docs")
	require.NoError(t, err)
	require.Equal(t, ".harness/orbits/docs.yaml", repoPath)

	filename, err := OrbitSpecPath(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".harness", "orbits", "docs.yaml"), filename)
}
