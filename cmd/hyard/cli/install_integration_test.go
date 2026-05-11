package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardInstallOverwriteDryRunReplaysLegacyZeroVariableInstall(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	_, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	record.Variables = nil
	_, err = harnesspkg.WriteInstallRecord(repo.Root, record)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--overwrite-existing",
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun            bool   `json:"dry_run"`
		OrbitID           string `json:"orbit_id"`
		OverwriteExisting bool   `json:"overwrite_existing"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.True(t, payload.OverwriteExisting)
	require.Equal(t, "docs", payload.OrbitID)
}

func TestHyardInstallExactPackageHandleCoordinateFromGitRegistry(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	sourceRepo := seedCommittedHyardSourceRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, sourceRepo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	sourceRemote := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	sourceCommit := sourceRepo.RevParse(t, "orbit-template/docs")

	registryRepo := testutil.NewRepo(t)
	registryRepo.Run(t, "branch", "-m", "main")
	registryRepo.WriteFile(t, "packages/acme/index.yaml", fmt.Sprintf(""+
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
		"          remote: %q\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: %s\n", sourceRemote, sourceCommit))
	registryRepo.AddAndCommit(t, "seed package registry")
	registryRemote := testutil.NewBareRemoteFromRepo(t, registryRepo)

	runtimeRepo := testutil.NewRepo(t)
	runtimeRepo.Run(t, "branch", "-m", "main")
	_, err = harnesspkg.BootstrapRuntimeControlPlane(runtimeRepo.Root, time.Date(2026, time.May, 11, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "seed empty runtime")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		runtimeRepo.Root,
		"install",
		"Acme/Docs@v0.1.0",
		"--registry-source",
		registryRemote,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var preview struct {
		DryRun bool `json:"dry_run"`
		Source struct {
			Kind               string `json:"kind"`
			Repo               string `json:"repo"`
			Ref                string `json:"ref"`
			Commit             string `json:"commit"`
			PackageName        string `json:"package_name"`
			PackageCoordinate  string `json:"package_coordinate"`
			PackageLocatorKind string `json:"package_locator_kind"`
			PackageLocator     string `json:"package_locator"`
			RegistryProvenance struct {
				RequestedCoordinate string `json:"requested_coordinate"`
				ResolvedCoordinate  string `json:"resolved_coordinate"`
				ResolvedVersion     string `json:"resolved_version"`
				RegistryRemote      string `json:"registry_remote"`
				RegistryRef         string `json:"registry_ref"`
				PackageType         string `json:"package_type"`
				PackageIdentity     string `json:"package_identity"`
				PackageStatus       string `json:"package_status"`
				SourceRemote        string `json:"source_remote"`
				SourceRef           string `json:"source_ref"`
				SourceCommit        string `json:"source_commit"`
				CacheUsed           bool   `json:"cache_used"`
				CacheStale          bool   `json:"cache_stale"`
			} `json:"registry_provenance"`
		} `json:"source"`
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &preview))
	require.True(t, preview.DryRun)
	require.Equal(t, "docs", preview.OrbitID)
	require.Equal(t, orbittemplate.InstallSourceKindExternalGit, preview.Source.Kind)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(preview.Source.Repo))
	require.Equal(t, sourceCommit, preview.Source.Ref)
	require.Equal(t, sourceCommit, preview.Source.Commit)
	require.Equal(t, "docs", preview.Source.PackageName)
	require.Equal(t, "acme/docs@0.1.0", preview.Source.PackageCoordinate)
	require.Equal(t, "git", preview.Source.PackageLocatorKind)
	require.Contains(t, preview.Source.PackageLocator, sourceCommit)
	require.Equal(t, "acme/docs@0.1.0", preview.Source.RegistryProvenance.RequestedCoordinate)
	require.Equal(t, "acme/docs@0.1.0", preview.Source.RegistryProvenance.ResolvedCoordinate)
	require.Equal(t, "0.1.0", preview.Source.RegistryProvenance.ResolvedVersion)
	require.Equal(t, gitpkg.ComparablePath(registryRemote), gitpkg.ComparablePath(preview.Source.RegistryProvenance.RegistryRemote))
	require.Equal(t, "HEAD", preview.Source.RegistryProvenance.RegistryRef)
	require.Equal(t, "orbit", preview.Source.RegistryProvenance.PackageType)
	require.Equal(t, "docs", preview.Source.RegistryProvenance.PackageIdentity)
	require.Equal(t, "active", preview.Source.RegistryProvenance.PackageStatus)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(preview.Source.RegistryProvenance.SourceRemote))
	require.Equal(t, "orbit-template/docs", preview.Source.RegistryProvenance.SourceRef)
	require.Equal(t, sourceCommit, preview.Source.RegistryProvenance.SourceCommit)
	require.False(t, preview.Source.RegistryProvenance.CacheUsed)
	require.False(t, preview.Source.RegistryProvenance.CacheStale)

	stdout, stderr, err = executeHyardCLIUnlocked(
		t,
		runtimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		registryRemote,
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var result struct {
		DryRun bool `json:"dry_run"`
		Source struct {
			Repo               string `json:"repo"`
			Ref                string `json:"ref"`
			Commit             string `json:"commit"`
			RegistryProvenance struct {
				RequestedCoordinate string `json:"requested_coordinate"`
				ResolvedCoordinate  string `json:"resolved_coordinate"`
				SourceRef           string `json:"source_ref"`
				SourceCommit        string `json:"source_commit"`
			} `json:"registry_provenance"`
		} `json:"source"`
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.False(t, result.DryRun)
	require.Equal(t, "docs", result.OrbitID)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(result.Source.Repo))
	require.Equal(t, sourceCommit, result.Source.Ref)
	require.Equal(t, sourceCommit, result.Source.Commit)
	require.Equal(t, "acme/docs@0.1.0", result.Source.RegistryProvenance.RequestedCoordinate)
	require.Equal(t, "acme/docs@0.1.0", result.Source.RegistryProvenance.ResolvedCoordinate)
	require.Equal(t, "orbit-template/docs", result.Source.RegistryProvenance.SourceRef)
	require.Equal(t, sourceCommit, result.Source.RegistryProvenance.SourceCommit)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallSourceKindExternalGit, record.Template.SourceKind)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(record.Template.SourceRepo))
	require.Equal(t, sourceCommit, record.Template.SourceRef)
	require.Equal(t, sourceCommit, record.Template.TemplateCommit)
	require.NotNil(t, record.Registry)
	require.Equal(t, "acme/docs@0.1.0", record.Registry.RequestedCoordinate)
	require.Equal(t, "acme/docs@0.1.0", record.Registry.ResolvedCoordinate)
	require.Equal(t, "0.1.0", record.Registry.ResolvedVersion)
	require.Equal(t, gitpkg.ComparablePath(registryRemote), gitpkg.ComparablePath(record.Registry.RegistryRemote))
	require.Equal(t, "HEAD", record.Registry.RegistryRef)
	require.Equal(t, "orbit", record.Registry.PackageType)
	require.Equal(t, "docs", record.Registry.PackageIdentity)
	require.Equal(t, "active", record.Registry.PackageStatus)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(record.Registry.SourceRemote))
	require.Equal(t, "orbit-template/docs", record.Registry.SourceRef)
	require.Equal(t, sourceCommit, record.Registry.SourceCommit)
	require.False(t, record.Registry.CacheUsed)
	require.False(t, record.Registry.CacheStale)

	recordData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "installs", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(recordData), "requested_coordinate: acme/docs@0.1.0\n")
	require.Contains(t, string(recordData), "package_identity: docs\n")
	require.Contains(t, string(recordData), "source_commit: "+sourceCommit+"\n")
	require.Contains(t, string(recordData), "cache_used: false\n")
	require.Contains(t, string(recordData), "cache_stale: false\n")
}

func TestHyardInstallPackageHandleCoordinateTextReportsRegistryProvenance(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "active")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@latest",
		"--registry-source",
		fixture.RegistryRemote,
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "registry.requested_coordinate: acme/docs@latest\n")
	require.Contains(t, stdout, "registry.resolved_coordinate: acme/docs@0.1.0\n")
	require.Contains(t, stdout, "registry.resolved_version: 0.1.0\n")
	require.Contains(t, stdout, "registry.registry_remote: "+fixture.RegistryRemote+"\n")
	require.Contains(t, stdout, "registry.registry_ref: HEAD\n")
	require.Contains(t, stdout, "registry.package_type: orbit\n")
	require.Contains(t, stdout, "registry.package_identity: docs\n")
	require.Contains(t, stdout, "registry.source_remote: "+fixture.SourceRemote+"\n")
	require.Contains(t, stdout, "registry.source_ref: orbit-template/docs\n")
	require.Contains(t, stdout, "registry.source_commit: "+fixture.SourceCommit+"\n")
	require.Contains(t, stdout, "registry.cache_used: false\n")
	require.Contains(t, stdout, "registry.cache_stale: false\n")
}

func TestHyardInstallLatestPackageHandleCoordinateRecordsStaleRegistryCacheProvenance(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "active")

	_, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@latest",
		"--registry-source",
		fixture.RegistryRemote,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	offlineRegistryRemote := fixture.RegistryRemote + ".offline"
	require.NoError(t, os.Rename(fixture.RegistryRemote, offlineRegistryRemote))
	t.Cleanup(func() {
		require.NoError(t, os.Rename(offlineRegistryRemote, fixture.RegistryRemote))
	})

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@latest",
		"--registry-source",
		fixture.RegistryRemote,
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var result struct {
		Source struct {
			RegistryProvenance struct {
				RequestedCoordinate string `json:"requested_coordinate"`
				ResolvedCoordinate  string `json:"resolved_coordinate"`
				CacheUsed           bool   `json:"cache_used"`
				CacheStale          bool   `json:"cache_stale"`
			} `json:"registry_provenance"`
		} `json:"source"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "acme/docs@latest", result.Source.RegistryProvenance.RequestedCoordinate)
	require.Equal(t, "acme/docs@0.1.0", result.Source.RegistryProvenance.ResolvedCoordinate)
	require.True(t, result.Source.RegistryProvenance.CacheUsed)
	require.True(t, result.Source.RegistryProvenance.CacheStale)
	require.Len(t, result.Warnings, 1)
	require.Contains(t, result.Warnings[0], "stale cached registry resolution")

	record, err := harnesspkg.LoadInstallRecord(fixture.RuntimeRepo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Registry)
	require.Equal(t, "acme/docs@latest", record.Registry.RequestedCoordinate)
	require.Equal(t, "acme/docs@0.1.0", record.Registry.ResolvedCoordinate)
	require.True(t, record.Registry.CacheUsed)
	require.True(t, record.Registry.CacheStale)
}

func TestHyardInstallNamespacedLatestPackageHandleCoordinateFromGitRegistry(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "active")

	for _, coordinate := range []string{"Acme/Docs", "acme/docs@latest"} {
		stdout, stderr, err := executeHyardCLIUnlocked(
			t,
			fixture.RuntimeRepo.Root,
			"install",
			coordinate,
			"--registry-source",
			fixture.RegistryRemote,
			"--dry-run",
			"--json",
		)
		require.NoError(t, err)
		require.Empty(t, stderr)

		var preview struct {
			DryRun bool `json:"dry_run"`
			Source struct {
				Kind              string `json:"kind"`
				Repo              string `json:"repo"`
				Ref               string `json:"ref"`
				Commit            string `json:"commit"`
				PackageCoordinate string `json:"package_coordinate"`
			} `json:"source"`
			OrbitID string `json:"orbit_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &preview))
		require.True(t, preview.DryRun)
		require.Equal(t, "docs", preview.OrbitID)
		require.Equal(t, orbittemplate.InstallSourceKindExternalGit, preview.Source.Kind)
		require.Equal(t, gitpkg.ComparablePath(fixture.SourceRemote), gitpkg.ComparablePath(preview.Source.Repo))
		require.Equal(t, fixture.SourceCommit, preview.Source.Ref)
		require.Equal(t, fixture.SourceCommit, preview.Source.Commit)
		require.Equal(t, "acme/docs@latest", preview.Source.PackageCoordinate)
	}
}

func TestHyardInstallCuratedLatestPackageHandleCoordinateFromGitRegistry(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "active")

	for _, coordinate := range []string{"Docs", "docs@latest"} {
		stdout, stderr, err := executeHyardCLIUnlocked(
			t,
			fixture.RuntimeRepo.Root,
			"install",
			coordinate,
			"--registry-source",
			fixture.RegistryRemote,
			"--dry-run",
			"--json",
		)
		require.NoError(t, err)
		require.Empty(t, stderr)

		var preview struct {
			DryRun bool `json:"dry_run"`
			Source struct {
				Kind              string `json:"kind"`
				Repo              string `json:"repo"`
				Ref               string `json:"ref"`
				Commit            string `json:"commit"`
				PackageCoordinate string `json:"package_coordinate"`
			} `json:"source"`
			OrbitID string `json:"orbit_id"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &preview))
		require.True(t, preview.DryRun)
		require.Equal(t, "docs", preview.OrbitID)
		require.Equal(t, orbittemplate.InstallSourceKindExternalGit, preview.Source.Kind)
		require.Equal(t, gitpkg.ComparablePath(fixture.SourceRemote), gitpkg.ComparablePath(preview.Source.Repo))
		require.Equal(t, fixture.SourceCommit, preview.Source.Ref)
		require.Equal(t, fixture.SourceCommit, preview.Source.Commit)
		require.Equal(t, "docs@latest", preview.Source.PackageCoordinate)
	}
}

func TestHyardInstallYankedPackageHandleCoordinateRequiresOverride(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "yanked")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		fixture.RegistryRemote,
		"--json",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "yanked")
	require.ErrorContains(t, err, "--allow-yanked")
	_, loadErr := harnesspkg.LoadInstallRecord(fixture.RuntimeRepo.Root, "docs")
	require.ErrorIs(t, loadErr, os.ErrNotExist)
	require.NoFileExists(t, filepath.Join(fixture.RuntimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(fixture.RuntimeRepo.Root, "docs", "guide.md"))
}

func TestHyardInstallYankedPackageHandleCoordinateWithOverride(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "yanked")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		fixture.RegistryRemote,
		"--allow-yanked",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var result struct {
		OrbitID string `json:"orbit_id"`
		Source  struct {
			Repo   string `json:"repo"`
			Ref    string `json:"ref"`
			Commit string `json:"commit"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "docs", result.OrbitID)
	require.Equal(t, gitpkg.ComparablePath(fixture.SourceRemote), gitpkg.ComparablePath(result.Source.Repo))
	require.Equal(t, fixture.SourceCommit, result.Source.Ref)
	require.Equal(t, fixture.SourceCommit, result.Source.Commit)

	record, err := harnesspkg.LoadInstallRecord(fixture.RuntimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, fixture.SourceCommit, record.Template.TemplateCommit)
}

func TestHyardInstallBlockedPackageHandleCoordinateHasNoOverride(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "blocked")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		fixture.RegistryRemote,
		"--allow-yanked",
		"--json",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "blocked")
	require.ErrorContains(t, err, "no override")
	_, loadErr := harnesspkg.LoadInstallRecord(fixture.RuntimeRepo.Root, "docs")
	require.ErrorIs(t, loadErr, os.ErrNotExist)
	require.NoFileExists(t, filepath.Join(fixture.RuntimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(fixture.RuntimeRepo.Root, "docs", "guide.md"))
}

func TestHyardInstallDeprecatedPackageHandleCoordinateWarnsInJSONAndText(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("HYARD_CACHE_DIR", t.TempDir())

	fixture := seedPackageHandleInstallFixture(t, "deprecated")

	stdout, stderr, err := executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		fixture.RegistryRemote,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	var preview struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &preview))
	require.Len(t, preview.Warnings, 1)
	require.Contains(t, preview.Warnings[0], "deprecated")

	stdout, stderr, err = executeHyardCLIUnlocked(
		t,
		fixture.RuntimeRepo.Root,
		"install",
		"acme/docs@0.1.0",
		"--registry-source",
		fixture.RegistryRemote,
		"--dry-run",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "warnings:")
	require.Contains(t, stdout, "deprecated")
}

func TestHyardInstallHelpShowsYankedOverride(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "install", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "--allow-yanked")
	require.Contains(t, stdout, "yanked")
}

type packageHandleInstallFixture struct {
	SourceRemote   string
	SourceCommit   string
	RegistryRemote string
	RuntimeRepo    *testutil.Repo
}

func seedPackageHandleInstallFixture(t *testing.T, packageStatus string) packageHandleInstallFixture {
	t.Helper()

	sourceRepo := seedCommittedHyardSourceRepo(t)
	_, stderr, err := executeHyardCLIUnlocked(t, sourceRepo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	sourceRemote := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	sourceCommit := sourceRepo.RevParse(t, "orbit-template/docs")

	registryRepo := testutil.NewRepo(t)
	registryRepo.Run(t, "branch", "-m", "main")
	registryRepo.WriteFile(t, "curated/index.yaml", ""+
		"schema_version: 1\n"+
		"curated:\n"+
		"  docs:\n"+
		"    target: acme/docs\n")
	registryRepo.WriteFile(t, "packages/acme/index.yaml", fmt.Sprintf(""+
		"schema_version: 1\n"+
		"namespace: acme\n"+
		"packages:\n"+
		"  docs:\n"+
		"    status: %s\n"+
		"    dist_tags:\n"+
		"      latest: 0.1.0\n"+
		"    versions:\n"+
		"      0.1.0:\n"+
		"        package_type: orbit\n"+
		"        package_identity: docs\n"+
		"        source:\n"+
		"          remote: %q\n"+
		"          ref: orbit-template/docs\n"+
		"          commit: %s\n", packageStatus, sourceRemote, sourceCommit))
	registryRepo.AddAndCommit(t, "seed package registry")
	registryRemote := testutil.NewBareRemoteFromRepo(t, registryRepo)

	runtimeRepo := testutil.NewRepo(t)
	runtimeRepo.Run(t, "branch", "-m", "main")
	_, err = harnesspkg.BootstrapRuntimeControlPlane(runtimeRepo.Root, time.Date(2026, time.May, 11, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "seed empty runtime")

	return packageHandleInstallFixture{
		SourceRemote:   sourceRemote,
		SourceCommit:   sourceCommit,
		RegistryRemote: registryRemote,
		RuntimeRepo:    runtimeRepo,
	}
}
