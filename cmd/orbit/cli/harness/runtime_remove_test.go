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

func TestUninstallRuntimeOrbitPackageDeletesInstallOwnedRuntimeFilesAndGuidance(t *testing.T) {
	t.Parallel()

	repo := seedRuntimePackageUninstallRepo(t)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	result, err := UninstallRuntimeOrbitPackageWithOptions(
		context.Background(),
		discovered,
		"docs",
		time.Date(2026, time.May, 7, 10, 30, 0, 0, time.UTC),
		RemoveRuntimeMemberOptions{ConfirmLocalChanges: true},
	)
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/installs/docs.yaml",
		".harness/orbits/docs.yaml",
		"AGENTS.md",
		"docs/guide.md",
		"docs/process",
		"docs/process/flow.md",
		"docs/rules",
		"docs/rules/review.md",
	}, result.RemovedPaths)
	require.True(t, result.RemovedAgentsBlock)
	require.False(t, result.DetachedInstallRecord)

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = LoadInstallRecord(repo.Root, "docs")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, "AGENTS.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "rules", "review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "process", "flow.md"))
	require.NoDirExists(t, filepath.Join(repo.Root, "docs", "rules"))
	require.NoDirExists(t, filepath.Join(repo.Root, "docs", "process"))
	require.DirExists(t, filepath.Join(repo.Root, "docs"))
	require.DirExists(t, filepath.Join(repo.Root, ".harness", "orbits"))
	require.DirExists(t, filepath.Join(repo.Root, ".harness", "installs"))
	require.FileExists(t, filepath.Join(repo.Root, "docs", "local-note.md"))
}

func TestRemoveRuntimeHarnessPackageUninstallsIncludedOrbitsGuidanceAndRecords(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeHarnessPackageUninstallRepo(t)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	plan, err := BuildRemoveRuntimeHarnessPackagePlanWithOptions(
		context.Background(),
		discovered,
		"workspace",
		RemoveRuntimeHarnessPackageOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "docs"}, plan.OrbitIDs)
	require.Contains(t, plan.RemovedPaths, ".harness/bundles/workspace.yaml")
	require.Contains(t, plan.RemovedPaths, ".harness/installs/docs.yaml")
	require.Contains(t, plan.RemovedPaths, ".harness/orbits/api.yaml")
	require.Contains(t, plan.RemovedPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, plan.RemovedPaths, "api/guide.md")
	require.Contains(t, plan.RemovedPaths, "docs/guide.md")
	require.Contains(t, plan.RemovedPaths, "AGENTS.md")
	require.Contains(t, plan.RemovedPaths, "HUMANS.md")
	require.Contains(t, plan.RemovedPaths, "BOOTSTRAP.md")
	require.True(t, plan.RemoveRootAgents)
	require.True(t, plan.DeleteBundleRecord)
	require.True(t, plan.ConfirmationRequired)

	result, err := ApplyRemoveRuntimeHarnessPackagePlanWithOptions(
		context.Background(),
		discovered,
		plan,
		time.Date(2026, time.May, 11, 12, 0, 0, 0, time.UTC),
		RemoveRuntimeHarnessPackageOptions{},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"api", "docs"}, result.OrbitIDs)
	require.True(t, result.RemovedAgentsBlock)
	require.True(t, result.DeletedBundleRecord)
	require.Contains(t, result.RemovedPaths, ".harness/bundles/workspace.yaml")
	require.Contains(t, result.RemovedPaths, ".harness/installs/docs.yaml")
	require.Contains(t, result.RemovedPaths, "api/guide.md")
	require.Contains(t, result.RemovedPaths, "docs/guide.md")

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = LoadBundleRecord(repo.Root, "workspace")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = LoadInstallRecord(repo.Root, "docs")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "api.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, "api", "guide.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoDirExists(t, filepath.Join(repo.Root, "api"))
	require.NoDirExists(t, filepath.Join(repo.Root, "docs"))
	require.NoFileExists(t, filepath.Join(repo.Root, "HUMANS.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"Markerless run view guidance.\n\n"+
		"<!-- orbit:begin workflow=\"external\" -->\n"+
		"External orbit guidance\n"+
		"<!-- orbit:end workflow=\"external\" -->\n", string(agentsData))
}

func TestUninstallRuntimeOrbitPackagePreservesUnrelatedMarkedAndMarkerlessRootGuidance(t *testing.T) {
	t.Parallel()

	repo := seedRuntimePackageUninstallRepo(t)
	docsBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindOrbit, "docs", []byte("Docs runtime guidance\n"))
	require.NoError(t, err)
	apiBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindOrbit, "api", []byte("API runtime guidance\n"))
	require.NoError(t, err)
	harnessBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "docs", []byte("Harness runtime guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", ""+
		"Markerless run view guidance.\n\n"+
		string(docsBlock)+
		string(apiBlock)+
		string(harnessBlock)+
		"Tail markerless guidance.\n")
	humansDocsBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindOrbit, "docs", []byte("Docs human guidance\n"))
	require.NoError(t, err)
	humansHarnessBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "docs", []byte("Harness human guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(humansDocsBlock)+string(humansHarnessBlock))
	bootstrapDocsBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindOrbit, "docs", []byte("Docs bootstrap guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", string(bootstrapDocsBlock))

	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	result, err := UninstallRuntimeOrbitPackageWithOptions(
		context.Background(),
		discovered,
		"docs",
		time.Date(2026, time.May, 7, 10, 30, 0, 0, time.UTC),
		RemoveRuntimeMemberOptions{ConfirmLocalChanges: true},
	)
	require.NoError(t, err)
	require.Contains(t, result.RemovedPaths, "AGENTS.md")
	require.Contains(t, result.RemovedPaths, "HUMANS.md")
	require.Contains(t, result.RemovedPaths, "BOOTSTRAP.md")
	require.True(t, result.RemovedAgentsBlock)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"Markerless run view guidance.\n\n"+
		"<!-- orbit:begin workflow=\"api\" -->\n"+
		"API runtime guidance\n"+
		"<!-- orbit:end workflow=\"api\" -->\n"+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"Harness runtime guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n"+
		"Tail markerless guidance.\n", string(agentsData))
	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"Harness human guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n", string(humansData))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
}

func TestUninstallRuntimeOrbitPackageFailsClosedOnAmbiguousRootGuidanceMarkers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		guidance      string
		errorContains string
	}{
		{
			name: "duplicate target block",
			guidance: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"first docs block\n" +
				"<!-- orbit:end workflow=\"docs\" -->\n" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"second docs block\n" +
				"<!-- orbit:end workflow=\"docs\" -->\n",
			errorContains: "duplicate orbit block",
		},
		{
			name: "mismatched owner end marker",
			guidance: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"docs block\n" +
				"<!-- harness:end workflow=\"docs\" -->\n",
			errorContains: "does not match begin orbit block",
		},
		{
			name:          "malformed marker attribute",
			guidance:      "<!-- orbit:begin workflow=\"docs\" extra=\"value\" -->\n",
			errorContains: "malformed orbit block marker",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := seedRuntimePackageUninstallRepo(t)
			repo.WriteFile(t, "AGENTS.md", testCase.guidance)
			discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
			require.NoError(t, err)

			_, err = UninstallRuntimeOrbitPackageWithOptions(
				context.Background(),
				discovered,
				"docs",
				time.Date(2026, time.May, 7, 10, 30, 0, 0, time.UTC),
				RemoveRuntimeMemberOptions{},
			)
			require.Error(t, err)
			require.ErrorContains(t, err, "remove root AGENTS.md block")
			require.ErrorContains(t, err, testCase.errorContains)
			require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
			require.FileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
			agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
			require.NoError(t, readErr)
			require.Equal(t, testCase.guidance, string(agentsData))
		})
	}
}

func TestBuildUninstallRuntimeOrbitPackagePlanReportsUntrackedInstallOwnedFiles(t *testing.T) {
	t.Parallel()

	repo := seedRuntimePackageUninstallRepo(t)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	plan, err := BuildUninstallRuntimeOrbitPackagePlanWithOptions(
		context.Background(),
		discovered,
		"docs",
		RemoveRuntimeMemberOptions{},
	)
	require.NoError(t, err)
	require.Contains(t, plan.RemovedPaths, "docs/guide.md")
	require.True(t, plan.ConfirmationRequired)
	require.Contains(t, plan.LocallyChangedPaths, RuntimeUninstallLocalChange{
		Path:      "docs/guide.md",
		GitStatus: "??",
		Tracked:   false,
		Staged:    false,
	})

	_, err = ApplyUninstallRuntimeOrbitPackagePlanWithOptions(
		context.Background(),
		discovered,
		plan,
		time.Date(2026, time.May, 7, 10, 30, 0, 0, time.UTC),
		RemoveRuntimeMemberOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `requires --yes to delete locally changed target-owned files`)
	require.FileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
}

func TestUninstallRuntimeOrbitPackageRejectsSharedInstallOwnedPath(t *testing.T) {
	t.Parallel()

	repo := seedRuntimePackageUninstallSharedOwnershipRepo(t)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	_, err = UninstallRuntimeOrbitPackageWithOptions(
		context.Background(),
		discovered,
		"docs",
		time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC),
		RemoveRuntimeMemberOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `docs`)
	require.ErrorContains(t, err, `shared`)
	require.ErrorContains(t, err, `docs/guide.md`)

	require.FileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 2)
}

func TestUninstallRuntimeOrbitPackageRejectsUnresolvableActiveOwnershipScope(t *testing.T) {
	t.Parallel()

	repo := seedRuntimePackageUninstallRepo(t)
	now := time.Date(2026, time.May, 7, 11, 15, 0, 0, time.UTC)
	addUnresolvableActiveInstallMember(t, repo, "broken", now)
	discovered, err := gitpkg.DiscoverRepo(context.Background(), repo.Root)
	require.NoError(t, err)

	_, err = UninstallRuntimeOrbitPackageWithOptions(
		context.Background(),
		discovered,
		"docs",
		now,
		RemoveRuntimeMemberOptions{},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, `resolve package ownership scope for active package "broken"`)

	require.FileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 2)
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

func seedRuntimePackageUninstallRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	_, err := BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs runtime guidance\n"
	spec.Members = []orbitpkg.OrbitMember{
		{
			Key:  "docs-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/*.md"},
			},
		},
		{
			Key:  "docs-rules",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/**"},
			},
		},
		{
			Key:  "docs-process",
			Role: orbitpkg.OrbitMemberProcess,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/process/**"},
			},
		},
	}
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberProcess,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/rules/review.md", "Review checklist\n")
	repo.WriteFile(t, "docs/process/flow.md", "Process flow\n")
	repo.AddAndCommit(t, "seed docs orbit template source")

	_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          now,
		},
	})
	require.NoError(t, err)

	repo.Run(t, "rm", "-f",
		filepath.Join(".harness", "orbits", "docs.yaml"),
		filepath.Join("docs", "guide.md"),
		filepath.Join("docs", "rules", "review.md"),
		filepath.Join("docs", "process", "flow.md"),
	)
	repo.AddAndCommit(t, "clear docs runtime content")

	_, err = orbittemplate.ApplyLocalTemplate(context.Background(), orbittemplate.TemplateApplyInput{
		Preview: orbittemplate.TemplateApplyPreviewInput{
			RepoRoot:  repo.Root,
			SourceRef: "orbit-template/docs",
			Now:       now.Add(15 * time.Minute),
		},
	})
	require.NoError(t, err)

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = []RuntimeMember{{
		OrbitID: "docs",
		Source:  MemberSourceInstallOrbit,
		AddedAt: now.Add(15 * time.Minute),
	}}
	runtimeFile.Harness.UpdatedAt = now.Add(15 * time.Minute)
	_, err = WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	require.NoError(t, err)

	repo.WriteFile(t, "docs/local-note.md", "Local note that matches the orbit scope but was not installed.\n")

	return repo
}

func seedRuntimePackageUninstallSharedOwnershipRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	_, err := BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	saveSingleFileOrbitTemplateBranch(t, repo, "docs", "docs/guide.md", "Shared guide\n", now)
	saveSingleFileOrbitTemplateBranch(t, repo, "shared", "docs/guide.md", "Shared guide\n", now.Add(time.Minute))

	_, err = orbittemplate.ApplyLocalTemplate(context.Background(), orbittemplate.TemplateApplyInput{
		Preview: orbittemplate.TemplateApplyPreviewInput{
			RepoRoot:  repo.Root,
			SourceRef: "orbit-template/docs",
			Now:       now.Add(15 * time.Minute),
		},
	})
	require.NoError(t, err)
	_, err = orbittemplate.ApplyLocalTemplate(context.Background(), orbittemplate.TemplateApplyInput{
		Preview: orbittemplate.TemplateApplyPreviewInput{
			RepoRoot:          repo.Root,
			SourceRef:         "orbit-template/shared",
			OverwriteExisting: true,
			Now:               now.Add(20 * time.Minute),
		},
	})
	require.NoError(t, err)

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = []RuntimeMember{
		{
			OrbitID: "docs",
			Source:  MemberSourceInstallOrbit,
			AddedAt: now.Add(15 * time.Minute),
		},
		{
			OrbitID: "shared",
			Source:  MemberSourceInstallOrbit,
			AddedAt: now.Add(20 * time.Minute),
		},
	}
	runtimeFile.Harness.UpdatedAt = now.Add(20 * time.Minute)
	_, err = WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	require.NoError(t, err)

	return repo
}

func saveSingleFileOrbitTemplateBranch(
	t *testing.T,
	repo *testutil.Repo,
	orbitID string,
	repoPath string,
	content string,
	now time.Time,
) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
	require.NoError(t, err)
	spec.Description = orbitID + " orbit"
	spec.Members = []orbitpkg.OrbitMember{
		{
			Key:  orbitID + "-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{repoPath},
			},
		},
	}
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, repoPath, content)
	repo.AddAndCommit(t, "seed "+orbitID+" template source")

	_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      orbitID,
			TargetBranch: "orbit-template/" + orbitID,
			Now:          now,
		},
	})
	require.NoError(t, err)

	definitionPath, err := orbitpkg.HostedDefinitionRelativePath(orbitID)
	require.NoError(t, err)
	repo.Run(t, "rm", "-f", definitionPath, repoPath)
	repo.AddAndCommit(t, "clear "+orbitID+" runtime source")
}

func addUnresolvableActiveInstallMember(t *testing.T, repo *testutil.Repo, orbitID string, now time.Time) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
	require.NoError(t, err)
	spec.Description = orbitID + " orbit"
	spec.Members = []orbitpkg.OrbitMember{
		{
			Key:  orbitID + "-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{orbitID + "/guide.md"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	_, err = WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       orbitID,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/missing-" + orbitID,
			TemplateCommit: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
		AppliedAt: now,
	})
	require.NoError(t, err)

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, RuntimeMember{
		OrbitID: orbitID,
		Source:  MemberSourceInstallOrbit,
		AddedAt: now,
	})
	runtimeFile.Harness.UpdatedAt = now
	_, err = WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	require.NoError(t, err)

	repo.AddAndCommit(t, "add unresolvable active install member")
}

func seedRuntimeHarnessPackageUninstallRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 11, 11, 0, 0, 0, time.UTC)
	_, err := BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	writeHarnessPackageUninstallOrbit(t, repo, "docs", "docs/guide.md", "Docs guide\n")
	writeHarnessPackageUninstallOrbit(t, repo, "api", "api/guide.md", "API guide\n")

	harnessAgentsBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "workspace", []byte("Workspace runtime guidance\n"))
	require.NoError(t, err)
	externalBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindOrbit, "external", []byte("External orbit guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", ""+
		"Markerless run view guidance.\n\n"+
		string(harnessAgentsBlock)+
		string(externalBlock))
	harnessHumansBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "workspace", []byte("Workspace human guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(harnessHumansBlock))
	harnessBootstrapBlock, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "workspace", []byte("Workspace bootstrap guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", string(harnessBootstrapBlock))

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = []RuntimeMember{
		{
			OrbitID:        "docs",
			Source:         MemberSourceInstallBundle,
			OwnerHarnessID: "workspace",
			AddedAt:        now,
		},
		{
			OrbitID:        "api",
			Source:         MemberSourceInstallBundle,
			OwnerHarnessID: "workspace",
			AddedAt:        now,
		},
	}
	runtimeFile.Harness.UpdatedAt = now
	_, err = WriteManifestFile(repo.Root, ManifestFileFromRuntimeFile(runtimeFile))
	require.NoError(t, err)

	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "workspace",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: "abc123",
		},
		MemberIDs:          []string{"docs", "api"},
		AppliedAt:          now,
		IncludesRootAgents: true,
		OwnedPaths: []string{
			".harness/orbits/api.yaml",
			".harness/orbits/docs.yaml",
			"AGENTS.md",
			"BOOTSTRAP.md",
			"HUMANS.md",
			"api/guide.md",
			"docs/guide.md",
		},
	})
	require.NoError(t, err)

	_, err = WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed harness package uninstall repo")

	return repo
}

func writeHarnessPackageUninstallOrbit(t *testing.T, repo *testutil.Repo, orbitID string, repoPath string, content string) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
	require.NoError(t, err)
	spec.Description = orbitID + " orbit"
	spec.Members = []orbitpkg.OrbitMember{
		{
			Key:  orbitID + "-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{repoPath},
			},
		},
	}
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, repoPath, content)
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
