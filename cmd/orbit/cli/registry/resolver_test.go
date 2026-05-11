package registry_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/registry"
)

func TestResolveExactPackageHandleCoordinateFromNamespaceIndex(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("Acme/Docs@v0.1.0")
	require.NoError(t, err)

	resolution, err := registry.ResolveExactVersionFromNamespaceIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"namespace: acme\n"+
		"packages:\n"+
		"  docs:\n"+
		"    status: active\n"+
		"    versions:\n"+
		"      0.1.0:\n"+
		"        package_type: orbit\n"+
		"        package_identity: docs\n"+
		"        source:\n"+
		"          remote: https://example.com/acme/packages.git\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: 0123456789abcdef0123456789abcdef01234567\n"))
	require.NoError(t, err)
	require.Equal(t, ids.PackageTypeOrbit, resolution.PackageType)
	require.Equal(t, "docs", resolution.PackageIdentity)
	require.Equal(t, "https://example.com/acme/packages.git", resolution.SourceRemote)
	require.Equal(t, "orbit-template/docs", resolution.SourceRef)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", resolution.SourceCommit)
	require.False(t, resolution.FromCache)
	require.Empty(t, resolution.Warnings)
}

func TestResolveExactPackageHandleCoordinateFailsClosedWhenVersionMissing(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.2.0")
	require.NoError(t, err)

	_, err = registry.ResolveExactVersionFromNamespaceIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"namespace: acme\n"+
		"packages:\n"+
		"  docs:\n"+
		"    status: active\n"+
		"    versions:\n"+
		"      0.1.0:\n"+
		"        package_type: orbit\n"+
		"        package_identity: docs\n"+
		"        source:\n"+
		"          remote: https://example.com/acme/packages.git\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: 0123456789abcdef0123456789abcdef01234567\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `package handle "acme/docs@0.2.0" is not registered`)
}

func TestResolveExactPackageHandleCoordinateUsesVerifiedCacheWhenRegistryUnavailable(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)

	cacheRoot := t.TempDir()
	loader := registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
		return []byte("" +
			"schema_version: 1\n" +
			"namespace: acme\n" +
			"packages:\n" +
			"  docs:\n" +
			"    status: active\n" +
			"    versions:\n" +
			"      0.1.0:\n" +
			"        package_type: orbit\n" +
			"        package_identity: docs\n" +
			"        source:\n" +
			"          remote: https://example.com/acme/packages.git\n" +
			"          ref: orbit-template/docs\n" +
			"          commit: 0123456789abcdef0123456789abcdef01234567\n"), nil
	})

	fresh, err := registry.ResolveExactPackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      cacheRoot,
		Loader:         loader,
	})
	require.NoError(t, err)
	require.False(t, fresh.FromCache)

	fallback, err := registry.ResolveExactPackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      cacheRoot,
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
			return nil, errors.New("registry unavailable")
		}),
	})
	require.NoError(t, err)
	require.True(t, fallback.FromCache)
	require.Equal(t, fresh.SourceCommit, fallback.SourceCommit)
	require.Len(t, fallback.Warnings, 1)
	require.Contains(t, fallback.Warnings[0], "using cached registry resolution")
}

func TestCacheRootUsesHYARDCacheDirOverride(t *testing.T) {
	t.Setenv("HYARD_CACHE_DIR", filepath.Join(t.TempDir(), "hyard-cache"))

	root, err := registry.DefaultCacheRoot()
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(osEnv(t, "HYARD_CACHE_DIR")), root)
}

func osEnv(t *testing.T, name string) string {
	t.Helper()

	value := os.Getenv(name)
	require.NotEmpty(t, value)
	return value
}
