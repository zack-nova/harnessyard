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
	require.Equal(t, registry.PackageStatusActive, resolution.PackageStatus)
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

func TestResolvePackageHandleCoordinateResolvesNamespacedLatestDistTag(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("Acme/Docs")
	require.NoError(t, err)

	resolution, err := registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      t.TempDir(),
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
			return []byte("" +
				"schema_version: 1\n" +
				"namespace: acme\n" +
				"packages:\n" +
				"  docs:\n" +
				"    status: active\n" +
				"    dist_tags:\n" +
				"      latest: 0.2.0\n" +
				"    versions:\n" +
				"      0.1.0:\n" +
				"        package_type: orbit\n" +
				"        package_identity: docs\n" +
				"        source:\n" +
				"          remote: https://example.com/acme/packages.git\n" +
				"          ref: orbit-template/docs-old\n" +
				"          commit: 0123456789abcdef0123456789abcdef01234567\n" +
				"      0.2.0:\n" +
				"        package_type: orbit\n" +
				"        package_identity: docs\n" +
				"        source:\n" +
				"          remote: https://example.com/acme/packages.git\n" +
				"          ref: orbit-template/docs\n" +
				"          commit: abcdef0123456789abcdef0123456789abcdef01\n"), nil
		}),
	})
	require.NoError(t, err)
	require.Equal(t, "acme/docs@latest", resolution.Coordinate.String())
	require.Equal(t, "acme/docs@0.2.0", resolution.ResolvedCoordinate.String())
	require.Equal(t, "abcdef0123456789abcdef0123456789abcdef01", resolution.SourceCommit)
	require.False(t, resolution.FromCache)
	require.Empty(t, resolution.Warnings)
}

func TestResolvePackageHandleCoordinateResolvesCuratedLatestAlias(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("Docs@Latest")
	require.NoError(t, err)

	resolution, err := registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      t.TempDir(),
		Loader: testRegistryLoader{
			curatedIndex: []byte("" +
				"schema_version: 1\n" +
				"curated:\n" +
				"  docs:\n" +
				"    target: acme/docs\n"),
			namespaceIndexes: map[string][]byte{
				"acme": []byte("" +
					"schema_version: 1\n" +
					"namespace: acme\n" +
					"packages:\n" +
					"  docs:\n" +
					"    status: active\n" +
					"    dist_tags:\n" +
					"      latest: 0.2.0\n" +
					"    versions:\n" +
					"      0.2.0:\n" +
					"        package_type: orbit\n" +
					"        package_identity: docs\n" +
					"        source:\n" +
					"          remote: https://example.com/acme/packages.git\n" +
					"          ref: orbit-template/docs\n" +
					"          commit: abcdef0123456789abcdef0123456789abcdef01\n"),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "docs@latest", resolution.Coordinate.String())
	require.Equal(t, "acme/docs@0.2.0", resolution.ResolvedCoordinate.String())
	require.Equal(t, "abcdef0123456789abcdef0123456789abcdef01", resolution.SourceCommit)
}

func TestResolveCuratedHandleFromIndexRequiresNamespacedTarget(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("docs@latest")
	require.NoError(t, err)

	_, err = registry.ResolveCuratedHandleFromIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"curated:\n"+
		"  docs:\n"+
		"    target: docs\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "target must be namespaced")

	_, err = registry.ResolveCuratedHandleFromIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"curated:\n"+
		"  docs:\n"+
		"    target: acme/docs@0.1.0\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "without version or dist-tag")
}

func TestResolvePackageHandleCoordinateDoesNotInferLatestFromHighestVersion(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@latest")
	require.NoError(t, err)

	_, err = registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      t.TempDir(),
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
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
				"          commit: 0123456789abcdef0123456789abcdef01234567\n" +
				"      9.9.9:\n" +
				"        package_type: orbit\n" +
				"        package_identity: docs\n" +
				"        source:\n" +
				"          remote: https://example.com/acme/packages.git\n" +
				"          ref: orbit-template/docs-new\n" +
				"          commit: abcdef0123456789abcdef0123456789abcdef01\n"), nil
		}),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `package handle "acme/docs" has no registry dist-tag "latest"`)
}

func TestResolvePackageHandleCoordinateUsesStaleCacheWhenLatestRegistryUnavailable(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@latest")
	require.NoError(t, err)

	cacheRoot := t.TempDir()
	fresh, err := registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      cacheRoot,
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
			return []byte("" +
				"schema_version: 1\n" +
				"namespace: acme\n" +
				"packages:\n" +
				"  docs:\n" +
				"    status: active\n" +
				"    dist_tags:\n" +
				"      latest: 0.1.0\n" +
				"    versions:\n" +
				"      0.1.0:\n" +
				"        package_type: orbit\n" +
				"        package_identity: docs\n" +
				"        source:\n" +
				"          remote: https://example.com/acme/packages.git\n" +
				"          ref: orbit-template/docs\n" +
				"          commit: 0123456789abcdef0123456789abcdef01234567\n"), nil
		}),
	})
	require.NoError(t, err)
	require.False(t, fresh.FromCache)

	fallback, err := registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
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
	require.Equal(t, "acme/docs@latest", fallback.Coordinate.String())
	require.Equal(t, "acme/docs@0.1.0", fallback.ResolvedCoordinate.String())
	require.Equal(t, fresh.SourceCommit, fallback.SourceCommit)
	require.Len(t, fallback.Warnings, 1)
	require.Contains(t, fallback.Warnings[0], "stale cached registry resolution")
}

func TestResolvePackageHandleCoordinateFailsWhenLatestRegistryUnavailableWithoutCache(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@latest")
	require.NoError(t, err)

	_, err = registry.ResolvePackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      t.TempDir(),
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
			return nil, errors.New("registry unavailable")
		}),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "registry unavailable")
	require.ErrorContains(t, err, "no verified cache entry is available")
}

func TestResolutionCacheKeysIncludeRequestedCoordinate(t *testing.T) {
	t.Parallel()

	requested, err := registry.ParsePackageHandleCoordinate("acme/docs@latest")
	require.NoError(t, err)
	exact, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)
	cacheRoot := t.TempDir()

	err = registry.WriteVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", registry.Resolution{
		Coordinate:         requested,
		ResolvedCoordinate: exact,
		PackageType:        ids.PackageTypeOrbit,
		PackageIdentity:    "docs",
		PackageStatus:      registry.PackageStatusActive,
		SourceRemote:       "https://example.com/acme/packages.git",
		SourceRef:          "orbit-template/docs",
		SourceCommit:       "0123456789abcdef0123456789abcdef01234567",
	})
	require.NoError(t, err)

	cached, err := registry.ReadVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", requested)
	require.NoError(t, err)
	require.Equal(t, "acme/docs@latest", cached.Coordinate.String())
	require.Equal(t, "acme/docs@0.1.0", cached.ResolvedCoordinate.String())

	_, err = registry.ReadVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", exact)
	require.Error(t, err)
	_, err = registry.ReadVerifiedResolutionCache(cacheRoot, "https://example.com/other-registry.git", requested)
	require.Error(t, err)
}

func TestResolveExactPackageHandleCoordinateReturnsYankedStatus(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)

	resolution, err := registry.ResolveExactVersionFromNamespaceIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"namespace: acme\n"+
		"packages:\n"+
		"  docs:\n"+
		"    status: yanked\n"+
		"    versions:\n"+
		"      0.1.0:\n"+
		"        package_type: orbit\n"+
		"        package_identity: docs\n"+
		"        source:\n"+
		"          remote: https://example.com/acme/packages.git\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: 0123456789abcdef0123456789abcdef01234567\n"))
	require.NoError(t, err)
	require.Equal(t, registry.PackageStatusYanked, resolution.PackageStatus)
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", resolution.SourceCommit)
}

func TestResolveExactPackageHandleCoordinateWarnsForDeprecatedStatus(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)

	resolution, err := registry.ResolveExactVersionFromNamespaceIndex(coordinate, []byte(""+
		"schema_version: 1\n"+
		"namespace: acme\n"+
		"packages:\n"+
		"  docs:\n"+
		"    status: deprecated\n"+
		"    versions:\n"+
		"      0.1.0:\n"+
		"        package_type: orbit\n"+
		"        package_identity: docs\n"+
		"        source:\n"+
		"          remote: https://example.com/acme/packages.git\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: 0123456789abcdef0123456789abcdef01234567\n"))
	require.NoError(t, err)
	require.Equal(t, registry.PackageStatusDeprecated, resolution.PackageStatus)
	require.Len(t, resolution.Warnings, 1)
	require.Contains(t, resolution.Warnings[0], "deprecated")
}

func TestRequireInstallableResolutionRequiresYankedOverride(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)
	resolution := registry.Resolution{
		Coordinate:    coordinate,
		PackageStatus: registry.PackageStatusYanked,
	}

	err = registry.RequireInstallableResolution(resolution, registry.InstallGateOptions{})
	require.ErrorIs(t, err, registry.ErrYankedPackageRequiresOverride)
	require.ErrorContains(t, err, "--allow-yanked")

	err = registry.RequireInstallableResolution(resolution, registry.InstallGateOptions{AllowYanked: true})
	require.NoError(t, err)
}

func TestRequireInstallableResolutionRefusesBlockedPackage(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)
	resolution := registry.Resolution{
		Coordinate:    coordinate,
		PackageStatus: registry.PackageStatusBlocked,
	}

	err = registry.RequireInstallableResolution(resolution, registry.InstallGateOptions{AllowYanked: true})
	require.ErrorIs(t, err, registry.ErrBlockedPackageInstallRefused)
	require.ErrorContains(t, err, "no override")
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

func TestResolutionCachePreservesBlockedPackageStatus(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)
	cacheRoot := t.TempDir()

	err = registry.WriteVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", registry.Resolution{
		Coordinate:      coordinate,
		PackageType:     ids.PackageTypeOrbit,
		PackageIdentity: "docs",
		PackageStatus:   registry.PackageStatusBlocked,
		SourceRemote:    "https://example.com/acme/packages.git",
		SourceRef:       "orbit-template/docs",
		SourceCommit:    "0123456789abcdef0123456789abcdef01234567",
	})
	require.NoError(t, err)

	cached, err := registry.ReadVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", coordinate)
	require.NoError(t, err)
	require.Equal(t, registry.PackageStatusBlocked, cached.PackageStatus)
	require.ErrorIs(t, registry.RequireInstallableResolution(cached, registry.InstallGateOptions{AllowYanked: true}), registry.ErrBlockedPackageInstallRefused)
}

func TestFreshBlockedStatusTakesPrecedenceOverActiveCache(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("acme/docs@0.1.0")
	require.NoError(t, err)
	cacheRoot := t.TempDir()
	err = registry.WriteVerifiedResolutionCache(cacheRoot, "https://example.com/registry.git", registry.Resolution{
		Coordinate:      coordinate,
		PackageType:     ids.PackageTypeOrbit,
		PackageIdentity: "docs",
		PackageStatus:   registry.PackageStatusActive,
		SourceRemote:    "https://example.com/acme/packages.git",
		SourceRef:       "orbit-template/docs",
		SourceCommit:    "0123456789abcdef0123456789abcdef01234567",
	})
	require.NoError(t, err)

	resolution, err := registry.ResolveExactPackageHandleCoordinate(context.Background(), registry.ResolveInput{
		RepoRoot:       "/repo",
		Coordinate:     coordinate,
		RegistrySource: registry.Source{Remote: "https://example.com/registry.git", Ref: "HEAD"},
		CacheRoot:      cacheRoot,
		Loader: registry.NamespaceIndexLoaderFunc(func(context.Context, string, registry.Source, string) ([]byte, error) {
			return []byte("" +
				"schema_version: 1\n" +
				"namespace: acme\n" +
				"packages:\n" +
				"  docs:\n" +
				"    status: blocked\n" +
				"    versions:\n" +
				"      0.1.0:\n" +
				"        package_type: orbit\n" +
				"        package_identity: docs\n" +
				"        source:\n" +
				"          remote: https://example.com/acme/packages.git\n" +
				"          ref: orbit-template/docs\n" +
				"          commit: abcdef0123456789abcdef0123456789abcdef01\n"), nil
		}),
	})
	require.NoError(t, err)
	require.False(t, resolution.FromCache)
	require.Equal(t, registry.PackageStatusBlocked, resolution.PackageStatus)
	require.Equal(t, "abcdef0123456789abcdef0123456789abcdef01", resolution.SourceCommit)
	require.ErrorIs(t, registry.RequireInstallableResolution(resolution, registry.InstallGateOptions{AllowYanked: true}), registry.ErrBlockedPackageInstallRefused)
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

type testRegistryLoader struct {
	curatedIndex     []byte
	namespaceIndexes map[string][]byte
}

func (loader testRegistryLoader) LoadNamespaceIndex(_ context.Context, _ string, _ registry.Source, namespace string) ([]byte, error) {
	data, ok := loader.namespaceIndexes[namespace]
	if !ok {
		return nil, errors.New("namespace unavailable")
	}

	return data, nil
}

func (loader testRegistryLoader) LoadCuratedIndex(context.Context, string, registry.Source) ([]byte, error) {
	if loader.curatedIndex == nil {
		return nil, errors.New("curated index unavailable")
	}

	return loader.curatedIndex, nil
}
