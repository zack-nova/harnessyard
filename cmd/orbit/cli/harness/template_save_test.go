package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildTemplateSavePreviewIncludesMemberSnapshotFiles(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 9, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, preview.MemberSnapshotFiles, 1)

	snapshotPath, err := TemplateMemberSnapshotRepoPath("docs")
	require.NoError(t, err)
	require.Contains(t, preview.FilePaths(), snapshotPath)
	require.Equal(t, snapshotPath, preview.MemberSnapshotFiles[0].Path)

	snapshot, err := ParseTemplateMemberSnapshotData(preview.MemberSnapshotFiles[0].Content)
	require.NoError(t, err)
	require.Equal(t, TemplateMemberSnapshot{
		SchemaVersion: 1,
		Kind:          TemplateMemberSnapshotKind,
		OrbitID:       "docs",
		MemberSource:  MemberSourceManual,
		Snapshot: TemplateMemberSnapshotData{
			ExportedPaths: []string{"docs/guide.md"},
			FileDigests: map[string]string{
				"docs/guide.md": contentDigest([]byte("$project_name guide\n")),
			},
			Variables: map[string]TemplateVariableSpec{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
			},
		},
	}, snapshot)
}

func TestBuildTemplateSavePreviewIncludesRootHumansGuidance(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	humansBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Help humans operate Orbit\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", "Workspace notes for Orbit\n\n"+string(humansBlock))
	repo.AddAndCommit(t, "add root humans guidance")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 9, 5, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Contains(t, preview.FilePaths(), "HUMANS.md")
	require.Equal(t, RootGuidanceMetadata{Humans: true}, preview.Manifest.Template.RootGuidance)
	humansFile := requireTemplateSaveFile(t, preview.Files, "HUMANS.md")
	require.Equal(t, "Workspace notes for $project_name\n\nHelp humans operate $project_name\n", string(humansFile.Content))
}

func TestBuildTemplateSavePreviewIncludesPendingRootBootstrapGuidance(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  bootstrap_template: |\n"+
		"    Bootstrap Orbit\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	bootstrapBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap Orbit for humans\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "Workspace bootstrap for Orbit\n\n"+string(bootstrapBlock))
	repo.AddAndCommit(t, "add pending root bootstrap guidance")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 9, 7, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Contains(t, preview.FilePaths(), "BOOTSTRAP.md")
	require.Equal(t, RootGuidanceMetadata{Bootstrap: true}, preview.Manifest.Template.RootGuidance)
	bootstrapFile := requireTemplateSaveFile(t, preview.Files, "BOOTSTRAP.md")
	require.Equal(t, "Workspace bootstrap for $project_name\n\nBootstrap $project_name for humans\n", string(bootstrapFile.Content))
}

func TestBuildTemplateSavePreviewSkipsCompletedRootBootstrapGuidance(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  bootstrap_template: |\n"+
		"    Bootstrap Orbit\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	bootstrapBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap Orbit for humans\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "Workspace bootstrap for Orbit\n\n"+string(bootstrapBlock))
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 16, 8, 30, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 16, 8, 20, 0, 0, time.UTC),
		},
	}))
	repo.AddAndCommit(t, "add completed root bootstrap guidance")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 9, 8, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotContains(t, preview.FilePaths(), "BOOTSTRAP.md")
	require.False(t, preview.Manifest.Template.RootGuidance.Bootstrap)
}

func TestBuildTemplateSavePreviewIncludesCompletedRootBootstrapWhenRequested(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  bootstrap_template: |\n"+
		"    Bootstrap Orbit\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	bootstrapBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap Orbit for humans\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "BOOTSTRAP.md", "Workspace bootstrap for Orbit\n\n"+string(bootstrapBlock))
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 16, 8, 30, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 16, 8, 20, 0, 0, time.UTC),
		},
	}))
	repo.AddAndCommit(t, "add completed root bootstrap guidance")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:         repo.Root,
		TargetBranch:     "harness-template/workspace",
		Now:              time.Date(2026, time.April, 16, 9, 8, 0, 0, time.UTC),
		IncludeBootstrap: true,
	})
	require.NoError(t, err)
	require.Contains(t, preview.FilePaths(), "BOOTSTRAP.md")
	require.True(t, preview.Manifest.Template.RootGuidance.Bootstrap)

	bootstrapFile := requireTemplateSaveFile(t, preview.Files, "BOOTSTRAP.md")
	require.Equal(t, "Workspace bootstrap for $project_name\n\nBootstrap $project_name for humans\n", string(bootstrapFile.Content))
}

func TestBuildTemplateSavePreviewSnapshotTracksEditedFiles(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:        repo.Root,
		TargetBranch:    "harness-template/workspace",
		EditTemplate:    true,
		Editor:          rewriteTemplateFileEditor{Path: "docs/guide.md", Content: "Edited guide\n"},
		Now:             time.Date(2026, time.April, 16, 9, 30, 0, 0, time.UTC),
		DefaultTemplate: false,
	})
	require.NoError(t, err)
	require.Len(t, preview.MemberSnapshotFiles, 1)

	snapshot, err := ParseTemplateMemberSnapshotData(preview.MemberSnapshotFiles[0].Content)
	require.NoError(t, err)
	require.Equal(t, []string{"docs/guide.md"}, snapshot.Snapshot.ExportedPaths)
	require.Equal(t, map[string]string{
		"docs/guide.md": contentDigest([]byte("Edited guide\n")),
	}, snapshot.Snapshot.FileDigests)
	require.Empty(t, snapshot.Snapshot.Variables)
}

func TestBuildTemplateSavePreviewRejectsEditedUnownedPayloadFile(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)

	_, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:        repo.Root,
		TargetBranch:    "harness-template/workspace",
		EditTemplate:    true,
		Editor:          addTemplateFileEditor{Path: "docs/extra.md", Content: "Extra guide\n"},
		Now:             time.Date(2026, time.April, 16, 9, 45, 0, 0, time.UTC),
		DefaultTemplate: false,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "unowned payload")
	require.ErrorContains(t, err, "docs/extra.md")
}

func TestBuildTemplateSavePreviewRejectsEditedRenamedPayloadFile(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)

	_, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:        repo.Root,
		TargetBranch:    "harness-template/workspace",
		EditTemplate:    true,
		Editor:          renameTemplateFileEditor{FromPath: "docs/guide.md", ToPath: "docs/renamed.md", Content: "Renamed guide\n"},
		Now:             time.Date(2026, time.April, 16, 9, 50, 0, 0, time.UTC),
		DefaultTemplate: false,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "unowned payload")
	require.ErrorContains(t, err, "docs/renamed.md")
}

func TestSaveTemplateBranchUsesZeroCommitProvenanceWithoutCommittedHead(t *testing.T) {
	t.Parallel()

	repo := seedUncommittedTemplateSaveRepo(t)

	result, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			TargetBranch: "harness-template/workspace",
			Now:          time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC),
		},
		Overwrite: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.WriteResult.Commit)

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ManifestRepoPath())
	require.NoError(t, err)
	require.Contains(t, string(branchManifestData), "created_from_branch: main")
	require.Contains(t, string(branchManifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")

	templateManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", TemplateRepoPath())
	require.NoError(t, err)
	require.Contains(t, string(templateManifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestBuildTemplateSavePreviewRejectsDetachedHead(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	_, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 10, 15, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "harness template save requires a current branch; detached HEAD is not supported")
}

func TestBuildTemplateSavePreviewIncludesFrameworkManifestWhenPresent(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	repo.WriteFile(t, FrameworksRepoPath(), ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.AddAndCommit(t, "add runtime framework recommendation")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	var frameworkFile *orbittemplate.CandidateFile
	for index := range preview.Files {
		if preview.Files[index].Path == FrameworksRepoPath() {
			frameworkFile = &preview.Files[index]
			break
		}
	}
	require.NotNil(t, frameworkFile)
	require.Contains(t, string(frameworkFile.Content), "recommended_framework: claude\n")
}

func TestBuildTemplateSavePreviewIncludesAgentPackageTruthWhenPresent(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t)
	repo.WriteFile(t, FrameworksRepoPath(), ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.WriteFile(t, AgentConfigRepoPath(), ""+
		"schema_version: 1\n")
	repo.WriteFile(t, AgentOverlayRepoPath("claude"), ""+
		"schema_version: 1\n"+
		"mode: raw_passthrough\n"+
		"raw:\n"+
		"  profile: strict\n")
	repo.AddAndCommit(t, "add runtime agent package truth")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		TargetBranch: "harness-template/workspace",
		Now:          time.Date(2026, time.April, 16, 10, 35, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	var frameworkFile *orbittemplate.CandidateFile
	var agentConfigFile *orbittemplate.CandidateFile
	var overlayFile *orbittemplate.CandidateFile
	for index := range preview.Files {
		switch preview.Files[index].Path {
		case FrameworksRepoPath():
			frameworkFile = &preview.Files[index]
		case AgentConfigRepoPath():
			agentConfigFile = &preview.Files[index]
		case AgentOverlayRepoPath("claude"):
			overlayFile = &preview.Files[index]
		}
	}

	require.NotNil(t, frameworkFile)
	require.NotNil(t, agentConfigFile)
	require.NotNil(t, overlayFile)
	require.Contains(t, string(frameworkFile.Content), "recommended_framework: claude\n")
	require.Equal(t, "schema_version: 1\n", string(agentConfigFile.Content))
	require.Contains(t, string(overlayFile.Content), "mode: raw_passthrough\n")
}

func seedTemplateSaveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 16, 8, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 16, 8, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID: "docs",
				Source:  MemberSourceManual,
				AddedAt: time.Date(2026, time.April, 16, 8, 10, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed runtime repo for harness template save")

	return repo
}

func seedUncommittedTemplateSaveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 16, 8, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 16, 8, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID: "docs",
				Source:  MemberSourceManual,
				AddedAt: time.Date(2026, time.April, 16, 8, 10, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	repo.Run(t, "add", "-A")

	return repo
}

func requireTemplateSaveFile(
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

	require.Failf(t, "missing template save file", "expected %s in preview files", path)
	return orbittemplate.CandidateFile{}
}

type rewriteTemplateFileEditor struct {
	Path    string
	Content string
}

func (editor rewriteTemplateFileEditor) Edit(_ context.Context, dir string) error {
	filename := filepath.Join(dir, filepath.FromSlash(editor.Path))
	return os.WriteFile(filename, []byte(editor.Content), 0o600)
}

type addTemplateFileEditor struct {
	Path    string
	Content string
}

func (editor addTemplateFileEditor) Edit(_ context.Context, dir string) error {
	filename := filepath.Join(dir, filepath.FromSlash(editor.Path))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, []byte(editor.Content), 0o600)
}

type renameTemplateFileEditor struct {
	FromPath string
	ToPath   string
	Content  string
}

func (editor renameTemplateFileEditor) Edit(_ context.Context, dir string) error {
	fromFilename := filepath.Join(dir, filepath.FromSlash(editor.FromPath))
	if err := os.Remove(fromFilename); err != nil {
		return err
	}

	toFilename := filepath.Join(dir, filepath.FromSlash(editor.ToPath))
	if err := os.MkdirAll(filepath.Dir(toFilename), 0o755); err != nil {
		return err
	}

	return os.WriteFile(toFilename, []byte(editor.Content), 0o600)
}
