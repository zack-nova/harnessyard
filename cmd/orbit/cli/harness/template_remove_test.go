package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestRemoveTemplateMemberDeletesExclusiveFilesAndUpdatesManifests(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateRemoveRepo(t, false)

	result, err := RemoveTemplateMember(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		".harness/template_members/docs.yaml",
		"docs/guide.md",
	}, result.RemovedPaths)
	require.False(t, result.RemovedAgentsBlock)
	require.False(t, result.ZeroMemberTemplate)

	manifest, err := LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, []ManifestMember{{Package: testOrbitPackage("shared"), OrbitID: "shared"}}, manifest.Members)

	templateManifest, err := LoadTemplateManifest(repo.Root)
	require.NoError(t, err)
	require.Equal(t, []TemplateMember{{OrbitID: "shared"}}, templateManifest.Members)
	require.Equal(t, map[string]TemplateVariableSpec{
		"shared_name": {Description: "Shared name", Required: true},
	}, templateManifest.Variables)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "template_members", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	sharedData, err := os.ReadFile(filepath.Join(repo.Root, "shared", "checklist.md"))
	require.NoError(t, err)
	require.Equal(t, "Shared $shared_name checklist\n", string(sharedData))
}

func TestRemoveTemplateMemberAllowsZeroMemberTemplate(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveRepo(t)

	result, err := RemoveTemplateMember(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.True(t, result.ZeroMemberTemplate)

	manifest, err := LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, manifest.Members)
	require.False(t, manifest.RootGuidance.Agents)

	templateManifest, err := LoadTemplateManifest(repo.Root)
	require.NoError(t, err)
	require.Empty(t, templateManifest.Members)
	require.Empty(t, templateManifest.Variables)
	require.False(t, templateManifest.Template.RootGuidance.Agents)
}

func TestRemoveTemplateMemberRemovesAgentsBlockWhenPresent(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateRemoveRepo(t, true)

	result, err := RemoveTemplateMember(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.True(t, result.RemovedAgentsBlock)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.NotContains(t, string(agentsData), `workflow="docs"`)
	require.Contains(t, string(agentsData), `workflow="shared"`)

	templateManifest, err := LoadTemplateManifest(repo.Root)
	require.NoError(t, err)
	require.True(t, templateManifest.Template.RootGuidance.Agents)

	manifest, err := LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.True(t, manifest.RootGuidance.Agents)
}

func TestRemoveTemplateMemberAllowsTemplateWithAgentPackageTruth(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateRemoveRepo(t, false)
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
	repo.AddAndCommit(t, "add agent package truth to harness template remove repo")

	result, err := RemoveTemplateMember(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, result.RemovedPaths, ".harness/orbits/docs.yaml")

	frameworksData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(frameworksData), "recommended_framework: claude\n")

	overlayData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "overlays", "claude.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(overlayData), "mode: raw_passthrough\n")
}

func TestRemoveTemplateMemberRejectsDirtyOwnedPath(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateRemoveRepo(t, false)
	repo.WriteFile(t, "docs/guide.md", "Locally edited guide\n")

	_, err := RemoveTemplateMember(context.Background(), repo.Root, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `cannot remove template member "docs" with uncommitted changes`)
	require.ErrorContains(t, err, "docs/guide.md")
}

func seedHarnessTemplateRemoveRepo(t *testing.T, withAgentsBlocks bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	templateManifest := TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC),
			RootGuidance: RootGuidanceMetadata{
				Agents: withAgentsBlocks,
			},
		},
		Members: []TemplateMember{
			{OrbitID: "docs"},
			{OrbitID: "shared"},
		},
		Variables: map[string]TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
			"shared_name":  {Description: "Shared name", Required: true},
		},
	}
	_, err := WriteTemplateManifest(repo.Root, templateManifest)
	require.NoError(t, err)

	_, err = WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{OrbitID: "docs"},
			{OrbitID: "shared"},
		},
		RootGuidance: RootGuidanceMetadata{
			Agents: withAgentsBlocks,
		},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/shared.yaml", ""+
		"id: shared\n"+
		"description: Shared orbit\n"+
		"include:\n"+
		"  - shared/**\n")
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"    - shared/checklist.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+contentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"    shared/checklist.md: "+contentDigest([]byte("Shared $shared_name checklist\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n"+
		"    shared_name:\n"+
		"      description: Shared name\n"+
		"      required: true\n")
	repo.WriteFile(t, ".harness/template_members/shared.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: shared\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - shared/checklist.md\n"+
		"  file_digests:\n"+
		"    shared/checklist.md: "+contentDigest([]byte("Shared $shared_name checklist\n"))+"\n"+
		"  variables:\n"+
		"    shared_name:\n"+
		"      description: Shared name\n"+
		"      required: true\n")
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.WriteFile(t, "shared/checklist.md", "Shared $shared_name checklist\n")

	if withAgentsBlocks {
		docsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs guidance\n"))
		require.NoError(t, err)
		sharedBlock, err := orbittemplate.WrapRuntimeAgentsBlock("shared", []byte("Shared guidance\n"))
		require.NoError(t, err)
		repo.WriteFile(t, "AGENTS.md", "Workspace guidance\n\n"+string(docsBlock)+string(sharedBlock))
	}

	repo.AddAndCommit(t, "seed harness template remove repo")
	return repo
}

func seedSingleMemberHarnessTemplateRemoveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	templateManifest := TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC),
			RootGuidance:      RootGuidanceMetadata{},
		},
		Members: []TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	}
	_, err := WriteTemplateManifest(repo.Root, templateManifest)
	require.NoError(t, err)

	_, err = WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{OrbitID: "docs"},
		},
		RootGuidance: RootGuidanceMetadata{},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+contentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.AddAndCommit(t, "seed single member harness template remove repo")

	return repo
}
