package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestRemoveRuntimeMemberDeletesInfluencePathsAndDetachesInstallRecord(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRemoveRepo(t, runtimeRemoveSeedOptions{
		memberSource:    MemberSourceInstallOrbit,
		withAgentsBlock: true,
	})
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	result, err := RemoveRuntimeMember(
		context.Background(),
		discovered,
		"docs",
		time.Date(2026, time.April, 16, 11, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	require.Equal(t, []string{
		"AGENTS.md",
		"docs/process/flow.md",
		"docs/rules/review.md",
	}, result.RemovedPaths)
	require.True(t, result.RemovedAgentsBlock)
	require.True(t, result.DetachedInstallRecord)
	require.False(t, result.AutoLeftCurrentOrbit)

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Docs guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "rules", "review.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", "flow.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)

	record, err := LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallRecordStatusDetached, record.Status)
}

func TestRemoveRuntimeMemberRejectsBundleBackedMember(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRemoveRepo(t, runtimeRemoveSeedOptions{
		memberSource: MemberSourceInstallBundle,
	})
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	_, err = RemoveRuntimeMember(context.Background(), discovered, "docs", time.Time{})
	require.Error(t, err)
	require.ErrorContains(t, err, `bundle-backed member "docs" has no bundle record`)
}

func TestRemoveRuntimeMemberRejectsSharedInfluencePath(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRemoveSharedPathRepo(t)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	_, err = RemoveRuntimeMember(context.Background(), discovered, "docs", time.Time{})
	require.Error(t, err)
	require.ErrorContains(t, err, `shared/runtime.md`)
	require.ErrorContains(t, err, `shared`)
}

func TestRemoveRuntimeMemberRejectsDirtyDeletePath(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRemoveRepo(t, runtimeRemoveSeedOptions{
		memberSource: MemberSourceManual,
	})
	repo.WriteFile(t, "docs/rules/review.md", "Locally edited review\n")

	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	_, err = RemoveRuntimeMember(context.Background(), discovered, "docs", time.Time{})
	require.Error(t, err)
	require.ErrorContains(t, err, `cannot remove runtime member "docs" with uncommitted changes`)
	require.ErrorContains(t, err, "docs/rules/review.md")
}

type runtimeRemoveSeedOptions struct {
	memberSource    string
	withAgentsBlock bool
}

func seedRuntimeRemoveRepo(t *testing.T, options runtimeRemoveSeedOptions) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC)

	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/rules/review.md", "Review checklist\n")
	repo.WriteFile(t, "docs/process/flow.md", "Process flow\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-subject\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/guide.md\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/rules/**\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")

	_, err := WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []ManifestMember{{
			OrbitID:        "docs",
			Source:         options.memberSource,
			OwnerHarnessID: bundleOwnerForSource(options.memberSource),
			AddedAt:        now,
		}},
	})
	require.NoError(t, err)

	if options.memberSource == MemberSourceInstallOrbit {
		_, err = WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
			SchemaVersion: 1,
			OrbitID:       "docs",
			Template: orbittemplate.Source{
				SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
				SourceRepo:     "",
				SourceRef:      "orbit-template/docs",
				TemplateCommit: "abc123",
			},
			AppliedAt: now,
		})
		require.NoError(t, err)
	}

	if options.withAgentsBlock {
		block, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs runtime guidance\n"))
		require.NoError(t, err)
		repo.WriteFile(t, "AGENTS.md", string(block))
	}

	repo.AddAndCommit(t, "seed runtime remove repo")

	return repo
}

func seedRuntimeRemoveSharedPathRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC)

	repo.WriteFile(t, "shared/runtime.md", "Shared runtime guidance\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - shared/runtime.md\n")
	repo.WriteFile(t, ".harness/orbits/shared.yaml", ""+
		"id: shared\n"+
		"description: Shared orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/shared.yaml\n"+
		"members:\n"+
		"  - key: shared-subject\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - shared/runtime.md\n")

	_, err := WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []ManifestMember{
			{
				OrbitID: "docs",
				Source:  ManifestMemberSourceManual,
				AddedAt: now,
			},
			{
				OrbitID: "shared",
				Source:  ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed runtime remove shared path repo")

	return repo
}

func TestRuntimeRemoveSeedHostedSpecParses(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRemoveRepo(t, runtimeRemoveSeedOptions{memberSource: MemberSourceManual})

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "docs", spec.ID)
}

func bundleOwnerForSource(source string) string {
	if source == MemberSourceInstallBundle {
		return "workspace"
	}

	return ""
}
