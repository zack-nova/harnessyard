package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func seedHarnessTemplateInstallSourceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  includes_root_agents: false\n"+
		"members:\n"+
		"  - orbit_id: workspace\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: workspace\n")
	repo.WriteFile(t, ".harness/orbits/workspace.yaml", ""+
		"id: workspace\n"+
		"description: Workspace orbit\n"+
		"include:\n"+
		"  - docs/**\n"+
		"  - schema/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.WriteFile(t, "schema/example.schema.json", "{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"$id\": \"workspace/example.schema.json\",\n  \"title\": \"$project_name\"\n}\n")
	repo.AddAndCommit(t, "seed harness template source")

	return repo
}

func TestResolveLocalTemplateInstallSourceIgnoresNonMarkdownVariableSyntax(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)

	source, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, "HEAD", source.Ref)
	require.Equal(t, []string{"workspace"}, source.MemberIDs())
	require.Contains(t, source.FilePaths(), "schema/example.schema.json")
	require.Equal(t, time.Date(2026, time.April, 3, 0, 0, 0, 0, time.UTC), source.Manifest.Template.CreatedAt)
}

func TestResolveLocalTemplateInstallSourceIgnoresLegacyOrbitTemplateManifest(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: stray999\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "add stray legacy orbit template manifest")

	source, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, "HEAD", source.Ref)
	require.Equal(t, []string{"workspace"}, source.MemberIDs())
	require.Contains(t, source.FilePaths(), "docs/guide.md")
}

func TestResolveLocalTemplateInstallSourceLoadsMemberSnapshotsAndSkipsControlFiles(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	snapshotPath, err := TemplateMemberSnapshotRepoPath("workspace")
	require.NoError(t, err)
	repo.WriteFile(t, snapshotPath, ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: workspace\n"+
		"member_source: install_orbit\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"    - schema/example.schema.json\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+contentDigest([]byte("$project_name guide\n"))+"\n"+
		"    schema/example.schema.json: "+contentDigest([]byte("{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"$id\": \"workspace/example.schema.json\",\n  \"title\": \"$project_name\"\n}\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      required: true\n")
	repo.AddAndCommit(t, "add template member snapshot")

	source, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.NotContains(t, source.FilePaths(), snapshotPath)
	require.Len(t, source.MemberSnapshots, 1)
	require.Contains(t, source.MemberSnapshots, "workspace")
	require.Equal(t, []string{"docs/guide.md", "schema/example.schema.json"}, source.MemberSnapshots["workspace"].Snapshot.ExportedPaths)
}

func TestResolveLocalTemplateInstallSourceLoadsOptionalFrameworkRecommendation(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, FrameworksRepoPath(), ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.AddAndCommit(t, "add package framework recommendation")

	source, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, "claude", source.Frameworks.RecommendedFramework)
	require.NotContains(t, source.FilePaths(), FrameworksRepoPath())
}

func TestResolveLocalTemplateInstallSourceLoadsOptionalAgentPackageTruth(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
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
	repo.AddAndCommit(t, "add package agent truth")

	source, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, "claude", source.Frameworks.RecommendedFramework)
	require.NotNil(t, source.AgentConfig)
	require.Equal(t, 1, source.AgentConfig.SchemaVersion)
	require.Contains(t, source.AgentOverlays, "claude")
	require.Equal(t, AgentOverlayModeRawPassthrough, source.AgentOverlays["claude"].Mode)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"mode: raw_passthrough\n"+
		"raw:\n"+
		"  profile: strict\n", string(source.AgentOverlays["claude"].Content))
	require.NotContains(t, source.FilePaths(), FrameworksRepoPath())
	require.NotContains(t, source.FilePaths(), AgentConfigRepoPath())
	require.NotContains(t, source.FilePaths(), AgentOverlayRepoPath("claude"))
}

func TestResolveLocalTemplateInstallSourceRejectsHostedCapabilitySkillRootMissingSkillMD(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".harness/orbits/workspace.yaml", ""+
		"id: workspace\n"+
		"description: Workspace orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/workspace.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/*\n"+
		"members:\n"+
		"  - key: workspace-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - schema/**\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use $project_name style guide.\n")
	repo.AddAndCommit(t, "add incomplete skill capability to harness template source")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, `local skill root "orbit/skills/docs-style": SKILL.md must exist and be tracked`)
}

func TestResolveLocalTemplateInstallSourceRejectsPartialMemberSnapshotSet(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  includes_root_agents: false\n"+
		"members:\n"+
		"  - orbit_id: shared\n"+
		"  - orbit_id: workspace\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: shared\n"+
		"  - orbit_id: workspace\n")
	repo.WriteFile(t, ".harness/orbits/shared.yaml", ""+
		"id: shared\n"+
		"description: Shared orbit\n"+
		"include:\n"+
		"  - shared/**\n")
	repo.WriteFile(t, "shared/checklist.md", "Shared checklist\n")
	snapshotPath, err := TemplateMemberSnapshotRepoPath("workspace")
	require.NoError(t, err)
	repo.WriteFile(t, snapshotPath, ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: workspace\n"+
		"member_source: install_orbit\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+contentDigest([]byte("$project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      required: true\n")
	repo.AddAndCommit(t, "add partial member snapshots")

	_, err = ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, `template member snapshot for "shared" is required`)
}

func TestResolveLocalTemplateInstallSourceRejectsSnapshotVariableSummaryDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	snapshotPath, err := TemplateMemberSnapshotRepoPath("workspace")
	require.NoError(t, err)
	repo.WriteFile(t, snapshotPath, ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: workspace\n"+
		"member_source: install_orbit\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+contentDigest([]byte("$project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    wrong_name:\n"+
		"      required: true\n")
	repo.AddAndCommit(t, "add drifted snapshot variable summary")

	_, err = ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, `template member snapshot for "workspace" has variable summary drift`)
}

func TestResolveLocalTemplateInstallSourceRejectsZeroMemberTemplate(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  includes_root_agents: false\n"+
		"members: []\n"+
		"variables: {}\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members: []\n")
	repo.Run(t, "rm", "-f", ".harness/orbits/workspace.yaml")
	repo.AddAndCommit(t, "make harness template empty")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, "zero-member harness template")
	require.ErrorContains(t, err, "cannot be installed")
}

func TestResolveLocalTemplateInstallSourceRejectsMissingBranchManifest(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.Run(t, "rm", ".harness/manifest.yaml")
	repo.AddAndCommit(t, "remove branch manifest")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
	require.ErrorContains(t, err, "valid harness template branch")

	var notFoundErr *LocalTemplateInstallSourceNotFoundError
	require.NotErrorAs(t, err, &notFoundErr)
}

func TestResolveLocalTemplateInstallSourceRejectsBranchManifestTemplateMismatch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: another\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: workspace\n")
	repo.AddAndCommit(t, "corrupt branch manifest harness id")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
	require.ErrorContains(t, err, ".harness/template.yaml")
	require.ErrorContains(t, err, "harness_id")
}

func TestResolveLocalTemplateInstallSourceRejectsBranchManifestDefaultTemplateMismatch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateInstallSourceRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: workspace\n")
	repo.AddAndCommit(t, "corrupt branch manifest default template")

	_, err := ResolveLocalTemplateInstallSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
	require.ErrorContains(t, err, ".harness/template.yaml")
	require.ErrorContains(t, err, "default_template")
}

func TestEnumerateRemoteTemplateInstallSourcesRejectsBranchesWithoutBranchManifest(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.Run(t, "branch", "harness-template/legacy-only")
	sourceRepo.Run(t, "checkout", "harness-template/legacy-only")
	sourceRepo.Run(t, "rm", ".harness/manifest.yaml")
	sourceRepo.AddAndCommit(t, "remove branch manifest from legacy-only branch")
	sourceRepo.Run(t, "checkout", "main")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateInstallSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "main", candidates[0].Branch)
}

func TestEnumerateRemoteTemplateInstallSourcesIgnoresLegacyOrbitTemplateManifest(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.Run(t, "branch", "harness-template/with-legacy-template")
	sourceRepo.Run(t, "checkout", "harness-template/with-legacy-template")
	sourceRepo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: stray999\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"variables: {}\n")
	sourceRepo.AddAndCommit(t, "add stray legacy orbit template manifest")
	sourceRepo.Run(t, "checkout", "main")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateInstallSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, "harness-template/with-legacy-template", candidates[0].Branch)
	require.Equal(t, "main", candidates[1].Branch)
}

func TestEnumerateRemoteTemplateInstallSourcesRejectsBranchesWithDefaultTemplateMismatch(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.Run(t, "branch", "harness-template/mismatch")
	sourceRepo.Run(t, "checkout", "harness-template/mismatch")
	sourceRepo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: workspace\n")
	sourceRepo.AddAndCommit(t, "corrupt branch manifest default template")
	sourceRepo.Run(t, "checkout", "main")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateInstallSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "main", candidates[0].Branch)
}

func TestEnumerateRemoteTemplateInstallSourcesIgnoresZeroMemberTemplates(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.Run(t, "branch", "harness-template/empty")
	sourceRepo.Run(t, "checkout", "harness-template/empty")
	sourceRepo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"  includes_root_agents: false\n"+
		"members: []\n"+
		"variables: {}\n")
	sourceRepo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-03T00:00:00Z\n"+
		"members: []\n")
	sourceRepo.Run(t, "rm", "-f", ".harness/orbits/workspace.yaml")
	sourceRepo.AddAndCommit(t, "make empty harness template branch")
	sourceRepo.Run(t, "checkout", "main")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	candidates, err := EnumerateRemoteTemplateInstallSources(context.Background(), runtimeRepo.Root, remoteURL)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "main", candidates[0].Branch)
}

func TestResolveRemoteTemplateInstallSourceExplicitRefRejectsMissingBranchManifest(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateInstallSourceRepo(t)
	sourceRepo.Run(t, "branch", "harness-template/legacy-only")
	sourceRepo.Run(t, "checkout", "harness-template/legacy-only")
	sourceRepo.Run(t, "rm", ".harness/manifest.yaml")
	sourceRepo.AddAndCommit(t, "remove branch manifest from legacy-only branch")
	sourceRepo.Run(t, "checkout", "main")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := testutil.NewRepo(t)

	_, _, err := ResolveRemoteTemplateInstallSource(context.Background(), runtimeRepo.Root, remoteURL, "harness-template/legacy-only")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")

	var notFoundErr *RemoteTemplateInstallNotFoundError
	require.NotErrorAs(t, err, &notFoundErr)
}
