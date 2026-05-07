package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildTemplateInstallPreviewAllowsUnresolvedRequiredBindings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  root_guidance:\n"+
		"    agents: false\n"+
		"    humans: false\n"+
		"    bootstrap: false\n"+
		"members:\n"+
		"  - orbit_id: workspace\n"+
		"variables:\n"+
		"  command_name:\n"+
		"    description: CLI command name\n"+
		"    required: true\n"+
		"  project_name:\n"+
		"    description: Project title\n"+
		"    required: true\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "$project_name command $command_name\n")
	sourceRepo.AddAndCommit(t, "add two variable harness template")
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	bootstrap, err := BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(runtimeRepo.Root), bootstrap.ManifestPath)
	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: false,
		Now:                     now,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"install kept harness template variables unresolved: command_name"}, preview.Warnings)
	require.Equal(t, "Orbit command $command_name\n", string(requireTemplateInstallFile(t, preview.RenderedFiles, "docs/guide.md").Content))
	require.Equal(t, orbittemplate.InstallVariablesSnapshot{
		Declarations: map[string]bindings.VariableDeclaration{
			"command_name": {Description: "CLI command name", Required: true},
			"project_name": {Description: "Project title", Required: true},
		},
		ResolvedAtApply: map[string]bindings.VariableBinding{
			"project_name": {Value: "Orbit", Description: "Project title"},
		},
		UnresolvedAtApply:         []string{"command_name"},
		ObservedRuntimeUnresolved: []string{"command_name"},
	}, *preview.BundleRecord.Variables)

	result, err := ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, preview, false)
	require.NoError(t, err)
	require.Contains(t, result.WrittenPaths, ".harness/bundles/workspace.yaml")
	renderedData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Orbit command $command_name\n", string(renderedData))

	bundleRecord, err := LoadBundleRecord(runtimeRepo.Root, "workspace")
	require.NoError(t, err)
	require.Equal(t, preview.BundleRecord.Variables, bundleRecord.Variables)

	checkResult, err := CheckRuntime(ctx, runtimeRepo.Root)
	require.NoError(t, err)
	require.True(t, checkResult.OK)
	require.NotNil(t, checkResult.BindingsSummary)
	require.Equal(t, 1, checkResult.BindingsSummary.UnresolvedInstallCount)
	require.Equal(t, 1, checkResult.BindingsSummary.UnresolvedVariableCount)
	require.Empty(t, checkResult.BindingsSummary.OrbitIDs)
	require.Equal(t, []string{"workspace"}, checkResult.BindingsSummary.BundleIDs)
}

func TestBuildTemplateInstallPreviewStrictRequiresBindings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	bootstrap, err := BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(runtimeRepo.Root), bootstrap.ManifestPath)

	_, err = BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		RequireResolvedBindings: true,
		Now:                     now,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "missing required bindings: project_name")
}

func TestBuildTemplateInstallPreviewSnapshotsPackageFrameworkRecommendation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.WriteFile(t, FrameworksRepoPath(), ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	sourceRepo.AddAndCommit(t, "add package framework recommendation")

	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 30, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		Now: now,
	})
	require.NoError(t, err)
	require.Equal(t, "claude", preview.BundleRecord.RecommendedFramework)
}

func TestBuildTemplateInstallPreviewSnapshotsEmptyVariablesContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  root_guidance:\n"+
		"    agents: false\n"+
		"    humans: false\n"+
		"    bootstrap: false\n"+
		"members:\n"+
		"  - orbit_id: workspace\n"+
		"variables: {}\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Static workspace guide\n")
	sourceRepo.AddAndCommit(t, "remove harness template variables")
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 30, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		Now: now,
	})
	require.NoError(t, err)
	require.NotNil(t, preview.BundleRecord.Variables)
	require.Equal(t, &orbittemplate.InstallVariablesSnapshot{
		Declarations:    map[string]bindings.VariableDeclaration{},
		ResolvedAtApply: map[string]bindings.VariableBinding{},
	}, preview.BundleRecord.Variables)

	filename, err := WriteBundleRecord(runtimeRepo.Root, preview.BundleRecord)
	require.NoError(t, err)
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Contains(t, string(data), ""+
		"variables:\n"+
		"    declarations: {}\n"+
		"    resolved_at_apply: {}\n"+
		"    unresolved_at_apply: []\n"+
		"    observed_runtime_unresolved: []\n")
}

func TestApplyTemplateInstallPreviewRollsBackWhenBundleRecordWriteFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		Now:                     now,
	})
	require.NoError(t, err)

	bundleRecordPath, err := BundleRecordPath(runtimeRepo.Root, "workspace")
	require.NoError(t, err)
	replaceInstallPathWithDirectory(t, bundleRecordPath)

	_, err = ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, preview, false)
	require.Error(t, err)
	require.ErrorContains(t, err, "write bundle record")

	runtimeFile, err := LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	require.NoFileExists(t, filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoFileExists(t, filepath.Join(runtimeRepo.Root, "schema", "example.schema.json"))
	require.NoFileExists(t, filepath.Join(runtimeRepo.Root, ".harness", "orbits", "workspace.yaml"))
	require.NoFileExists(t, VarsPath(runtimeRepo.Root))

	info, err := os.Stat(bundleRecordPath)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	transactionsDir := filepath.Join(runtimeRepo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestApplyTemplateInstallPreviewRollsBackWhenBundleCleanupFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	initialPreview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		Now:                     now,
	})
	require.NoError(t, err)

	_, err = ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, initialPreview, false)
	require.NoError(t, err)

	originalRecord, err := LoadBundleRecord(runtimeRepo.Root, "workspace")
	require.NoError(t, err)
	originalVarsData, err := os.ReadFile(VarsPath(runtimeRepo.Root))
	require.NoError(t, err)

	sourceRepo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	sourceRepo.Run(t, "rm", "-f", "docs/guide.md")
	sourceRepo.AddAndCommit(t, "update harness template source contents")

	updatedSource, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)
	overwritePreview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   updatedSource,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: updatedSource.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		OverwriteExisting:       true,
		Now:                     now.Add(time.Minute),
	})
	require.NoError(t, err)

	beforeBundleOwnedCleanupHook = func(repoRoot string, harnessID string, plan bundleOwnedCleanupPlan) {
		require.Equal(t, "workspace", harnessID)
		require.Contains(t, plan.DeletePaths, "docs/guide.md")
		replaceInstallOwnedPathWithNonEmptyDirectory(t, filepath.Join(repoRoot, "docs", "guide.md"))
	}
	t.Cleanup(func() {
		beforeBundleOwnedCleanupHook = nil
	})

	_, err = ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, overwritePreview, true)
	require.Error(t, err)
	require.ErrorContains(t, err, "remove stale bundle-owned paths")
	require.ErrorContains(t, err, "docs/guide.md")

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	restoredVarsData, err := os.ReadFile(VarsPath(runtimeRepo.Root))
	require.NoError(t, err)
	require.Equal(t, string(originalVarsData), string(restoredVarsData))

	restoredRecord, err := LoadBundleRecord(runtimeRepo.Root, "workspace")
	require.NoError(t, err)
	require.Equal(t, originalRecord.Template.TemplateCommit, restoredRecord.Template.TemplateCommit)

	runtimeFile, err := LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "workspace", runtimeFile.Members[0].OrbitID)
	require.Equal(t, MemberSourceInstallBundle, runtimeFile.Members[0].Source)
	require.Equal(t, "workspace", runtimeFile.Members[0].OwnerHarnessID)

	transactionsDir := filepath.Join(runtimeRepo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestBuildTemplateInstallPreviewConflictsWhenStaleBundlePathDrifted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	initialPreview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		Now:                     now,
	})
	require.NoError(t, err)
	_, err = ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, initialPreview, false)
	require.NoError(t, err)

	runtimeRepo.WriteFile(t, "docs/guide.md", "Locally drifted guide\n")
	sourceRepo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	sourceRepo.Run(t, "rm", "-f", "docs/guide.md")
	sourceRepo.AddAndCommit(t, "replace bundle docs payload")

	updatedSource, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)
	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   updatedSource,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: updatedSource.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		OverwriteExisting:       true,
		Now:                     now.Add(time.Minute),
	})
	require.NoError(t, err)
	requireTemplateInstallConflictMessageContains(t, preview.Conflicts, "docs/guide.md", "stale bundle-owned path no longer matches recorded content")
}

func TestBuildTemplateInstallPreviewConflictsWhenStaleBundleAgentsBlockDrifted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.WriteFile(t, "AGENTS.md", "Workspace guide for $project_name\n")
	sourceRepo.AddAndCommit(t, "add bundle agents")
	source, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)

	runtimeRepo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	_, err = BootstrapRuntimeControlPlane(runtimeRepo.Root, now)
	require.NoError(t, err)

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	runtimeRepo.WriteFile(t, "bindings.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	initialPreview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: source.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		Now:                     now,
	})
	require.NoError(t, err)
	_, err = ApplyTemplateInstallPreview(ctx, runtimeRepo.Root, initialPreview, false)
	require.NoError(t, err)

	driftedAgents, err := orbittemplate.WrapRuntimeAgentsOwnerBlock(orbittemplate.OwnerKindHarness, "workspace", []byte("Locally drifted bundle guide\n"))
	require.NoError(t, err)
	runtimeRepo.WriteFile(t, "AGENTS.md", string(driftedAgents))
	sourceRepo.Run(t, "rm", "-f", "AGENTS.md")
	sourceRepo.AddAndCommit(t, "remove bundle agents")

	updatedSource, err := ResolveLocalTemplateInstallSource(ctx, sourceRepo.Root, "HEAD")
	require.NoError(t, err)
	preview, err := BuildTemplateInstallPreview(ctx, TemplateInstallPreviewInput{
		RepoRoot: runtimeRepo.Root,
		Source:   updatedSource,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: updatedSource.Commit,
		},
		BindingsFilePath:        bindingsPath,
		RequireResolvedBindings: true,
		OverwriteExisting:       true,
		Now:                     now.Add(time.Minute),
	})
	require.NoError(t, err)
	requireTemplateInstallConflictMessageContains(t, preview.Conflicts, "AGENTS.md", "stale bundle AGENTS block no longer matches recorded content")
}

func requireTemplateInstallFile(
	t *testing.T,
	files []orbittemplate.CandidateFile,
	path string,
) orbittemplate.CandidateFile {
	t.Helper()

	for _, file := range files {
		if file.Path == path {
			return file
		}
	}
	require.Failf(t, "missing rendered file", "path %s was not rendered", path)
	return orbittemplate.CandidateFile{}
}

func replaceInstallOwnedPathWithNonEmptyDirectory(t *testing.T, absolutePath string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(absolutePath))
	require.NoError(t, os.MkdirAll(absolutePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(absolutePath, "blocked.txt"), []byte("keep"), 0o600))
}

func replaceInstallPathWithDirectory(t *testing.T, absolutePath string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(absolutePath))
	require.NoError(t, os.MkdirAll(absolutePath, 0o755))
}

func requireTemplateInstallConflictMessageContains(
	t *testing.T,
	conflicts []orbittemplate.ApplyConflict,
	path string,
	substring string,
) {
	t.Helper()

	for _, conflict := range conflicts {
		if conflict.Path == path && strings.Contains(conflict.Message, substring) {
			return
		}
	}
	require.Failf(t, "missing template install conflict", "path %s did not contain conflict substring %q in %#v", path, substring, conflicts)
}
