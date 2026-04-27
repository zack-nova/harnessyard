package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestResolveRootReturnsRepoRootFromSubdirectory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	manifestFile, err := DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.March, 25, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteManifestFile(repo.Root, manifestFile)
	require.NoError(t, err)

	subdir := filepath.Join(repo.Root, "docs")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	resolved, err := ResolveRoot(context.Background(), subdir)
	require.NoError(t, err)
	require.Equal(t, repo.Root, resolved.Repo.Root)
	require.Equal(t, manifestFile, resolved.Manifest)
}

func TestResolveRootFailsWhenManifestIsMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, err := ResolveRoot(context.Background(), repo.Root)
	require.Error(t, err)
	require.ErrorContains(t, err, "harness manifest is not initialized")
}

func TestResolveRootFailsWhenManifestIsInvalid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(ManifestPath(repo.Root)), 0o755))
	require.NoError(t, os.WriteFile(ManifestPath(repo.Root), []byte("schema_version: nope\n"), 0o600))

	_, err := ResolveRoot(context.Background(), repo.Root)
	require.Error(t, err)
	require.ErrorContains(t, err, "load harness manifest")
}

func TestResolveRootIgnoresLegacyRuntimeFileWhenManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	manifestFile, err := DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.March, 25, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteManifestFile(repo.Root, manifestFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "runtime.yaml"), []byte("schema_version: nope\n"), 0o600))

	resolved, err := ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)

	expectedRuntime, err := RuntimeFileFromManifestFile(manifestFile)
	require.NoError(t, err)
	require.Equal(t, expectedRuntime, resolved.Runtime)
}

func TestResolveManifestRootAllowsHarnessTemplateManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{},
	})
	require.NoError(t, err)

	resolved, err := ResolveManifestRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, repo.Root, resolved.Repo.Root)
	require.Equal(t, ManifestKindHarnessTemplate, resolved.Manifest.Kind)
}

func TestResolveRootLoadsHiddenManifestFromHEAD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	manifestFile, err := DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.April, 16, 15, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteManifestFile(repo.Root, manifestFile)
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "runtime root\n")
	repo.AddAndCommit(t, "seed runtime manifest")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	_, err = os.Stat(ManifestPath(repo.Root))
	require.ErrorIs(t, err, os.ErrNotExist)

	resolved, err := ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, manifestFile, resolved.Manifest)
}
