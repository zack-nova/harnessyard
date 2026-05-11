package cli_test

import (
	"encoding/json"
	"fmt"
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
			Repo   string `json:"repo"`
			Ref    string `json:"ref"`
			Commit string `json:"commit"`
		} `json:"source"`
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.False(t, result.DryRun)
	require.Equal(t, "docs", result.OrbitID)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(result.Source.Repo))
	require.Equal(t, sourceCommit, result.Source.Ref)
	require.Equal(t, sourceCommit, result.Source.Commit)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallSourceKindExternalGit, record.Template.SourceKind)
	require.Equal(t, gitpkg.ComparablePath(sourceRemote), gitpkg.ComparablePath(record.Template.SourceRepo))
	require.Equal(t, sourceCommit, record.Template.SourceRef)
	require.Equal(t, sourceCommit, record.Template.TemplateCommit)
}
