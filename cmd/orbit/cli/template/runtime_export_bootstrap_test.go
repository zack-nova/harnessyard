package orbittemplate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestFilterCompletedBootstrapExportPathsIncludesExistingPathsWhenExplicitlyRequested(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeExportBootstrapRepo(t)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC),
		},
	}))

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	result, err := FilterCompletedBootstrapExportPaths(context.Background(), RuntimeExportBootstrapFilterInput{
		RepoRoot:                  repo.Root,
		OrbitID:                   "docs",
		Spec:                      spec,
		ExportPaths:               []string{"docs/guide.md", "bootstrap/docs/setup.md"},
		IncludeCompletedBootstrap: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"docs/guide.md", "bootstrap/docs/setup.md"}, result.ExportPaths)
	require.Empty(t, result.SkippedBootstrapPaths)
	require.Empty(t, result.Warnings)
}

func TestFilterCompletedBootstrapExportPathsStillWarnsAndSkipsMissingPathsWhenIncluded(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeExportBootstrapRepo(t)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC),
		},
	}))
	require.NoError(t, os.Remove(repo.Root+"/bootstrap/docs/setup.md"))

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	result, err := FilterCompletedBootstrapExportPaths(context.Background(), RuntimeExportBootstrapFilterInput{
		RepoRoot:                  repo.Root,
		OrbitID:                   "docs",
		Spec:                      spec,
		ExportPaths:               []string{"docs/guide.md", "bootstrap/docs/setup.md"},
		IncludeCompletedBootstrap: true,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"docs/guide.md"}, result.ExportPaths)
	require.Equal(t, []string{"bootstrap/docs/setup.md"}, result.SkippedBootstrapPaths)
	require.Len(t, result.Warnings, 1)
	require.Contains(t, result.Warnings[0], `skip missing completed-bootstrap export paths for orbit "docs"`)
	require.Contains(t, result.Warnings[0], "bootstrap/docs/setup.md")
}

func seedRuntimeExportBootstrapRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-19T00:00:00Z\n"+
		"  updated_at: 2026-04-19T00:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "bootstrap/docs/setup.md", "setup\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	export := true
	spec.Members = append(spec.Members, orbitpkg.OrbitMember{
		Key:    "docs-bootstrap",
		Role:   orbitpkg.OrbitMemberRule,
		Lane:   orbitpkg.OrbitMemberLaneBootstrap,
		Scopes: &orbitpkg.OrbitMemberScopePatch{Export: &export},
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"bootstrap/docs/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed runtime export bootstrap repo")

	return repo
}
