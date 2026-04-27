package branchinfo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestInspectRevisionPrefersHostedDefinitionsForAuthoringBranches(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"include:\n"+
		"  - api/**\n")
	repo.AddAndCommit(t, "seed mixed source hosts")

	inspection, err := InspectRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, KindSource, inspection.Classification.Kind)
	require.Zero(t, inspection.ManifestMemberCount)
	require.Empty(t, inspection.ManifestMemberIDs)
	require.Equal(t, MemberCountScopeManifest, inspection.MemberCountScope)
	require.Zero(t, inspection.MemberCount)
	require.Empty(t, inspection.MemberIDs)
	require.Equal(t, 1, inspection.DefinitionCount)
	require.Equal(t, []string{"docs"}, inspection.DefinitionIDs)
	require.Equal(t, 1, inspection.DefinitionMemberCount)
	require.Equal(t, []DefinitionMemberSummary{
		{
			ID:          "docs",
			MemberCount: 1,
			MemberIDs:   []string{"docs-content"},
		},
	}, inspection.DefinitionMembers)
}

func TestInspectRevisionSkipsLegacyDefinitionsWhenHostedDefinitionsAreAbsent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.AddAndCommit(t, "seed legacy-only source host")

	inspection, err := InspectRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, KindSource, inspection.Classification.Kind)
	require.Zero(t, inspection.DefinitionCount)
	require.Empty(t, inspection.DefinitionIDs)
}

func TestInspectRevisionSeparatesDetachedAndInvalidInstallRecords(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	manifest, err := harnesspkg.DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.April, 16, 22, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, manifest)
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 22, 5, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "api",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/api",
			TemplateCommit: "def456",
		},
		AppliedAt: time.Date(2026, time.April, 16, 22, 10, 0, 0, time.UTC),
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	repo.WriteFile(t, filepath.Join(".harness", "installs", "broken.yaml"), "schema_version: [\n")
	repo.AddAndCommit(t, "seed runtime install provenance summary")

	inspection, err := InspectRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, KindRuntime, inspection.Classification.Kind)
	require.Equal(t, 1, inspection.InstallCount)
	require.Equal(t, []string{"docs"}, inspection.InstallIDs)
	require.Equal(t, 1, inspection.DetachedInstallCount)
	require.Equal(t, []string{"api"}, inspection.DetachedInstallIDs)
	require.Equal(t, 1, inspection.InvalidInstallCount)
	require.Equal(t, []string{"broken"}, inspection.InvalidInstallIDs)
}
