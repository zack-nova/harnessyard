package packageops_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/packageops"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestRenameHostedOrbitPackageUpdatesSpecAndSourceManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  package:\n"+
		"    type: orbit\n"+
		"    name: docs\n"+
		"  source_branch: main\n")
	spec, err := orbit.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Name = "Docs Workflow"
	spec.Description = "Docs workflow package"
	spec.Members = []orbit.OrbitMember{
		{
			Name: "docs-content",
			Role: orbit.OrbitMemberRule,
			Paths: orbit.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	result, err := packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, "docs", "api")
	require.NoError(t, err)
	require.Equal(t, "docs", result.OldPackage)
	require.Equal(t, "api", result.NewPackage)
	require.Equal(t, ".harness/orbits/docs.yaml", result.OldDefinitionPath)
	require.Equal(t, ".harness/orbits/api.yaml", result.NewDefinitionPath)
	require.Equal(t, ".harness/manifest.yaml", result.ManifestPath)
	require.True(t, result.ManifestChanged)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	renamedSpec, err := orbit.LoadHostedOrbitSpec(context.Background(), repo.Root, "api")
	require.NoError(t, err)
	require.NotNil(t, renamedSpec.Package)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "api"}, *renamedSpec.Package)
	require.Equal(t, "api", renamedSpec.ID)
	require.Equal(t, "Docs Workflow", renamedSpec.Name)
	require.Equal(t, "Docs workflow package", renamedSpec.Description)
	require.NotNil(t, renamedSpec.Meta)
	require.Equal(t, ".harness/orbits/api.yaml", renamedSpec.Meta.File)
	require.Len(t, renamedSpec.Members, 1)
	require.Equal(t, "docs-content", renamedSpec.Members[0].Name)

	manifest, err := harness.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.NotNil(t, manifest.Source)
	require.Equal(t, "api", manifest.Source.OrbitID)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "api"}, manifest.Source.Package)
}

func TestRenameHostedOrbitPackageRejectsDestinationCollision(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	docsSpec, err := orbit.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, docsSpec)
	require.NoError(t, err)
	apiSpec, err := orbit.DefaultHostedMemberSchemaSpec("api")
	require.NoError(t, err)
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, apiSpec)
	require.NoError(t, err)

	_, err = packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, "docs", "api")
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit package "api" already exists`)
}
