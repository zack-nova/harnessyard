package orbittemplate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestResolvePublishOrbitIDRejectsStrayLegacyDefinitionsAlongsideHostedSource(t *testing.T) {
	t.Parallel()

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
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "api/spec.md", "API spec\n")
	_, err := WriteSourceManifest(repo.Root, SourceManifest{
		SchemaVersion: sourceSchemaVersion,
		Kind:          SourceKind,
		SourceBranch:  "main",
		Publish: &SourcePublishConfig{
			OrbitID: "docs",
		},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed hosted source repo")

	_, err = resolvePublishOrbitID(context.Background(), repo.Root, "")
	require.Error(t, err)
	require.ErrorContains(t, err, ".orbit/orbits")
	require.ErrorContains(t, err, "source init")
}

func TestEnsureBriefExportSyncRejectsDriftedOrbitTemplateBriefUsingPlaceholderContract(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-12T10:00:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	agentsData, err := WrapRuntimeAgentsBlock("docs", []byte("Docs orbit for Acme\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))

	status, err := InspectOrbitBriefLane(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, BriefLaneStateMaterializedDrifted, status.State)

	_, err = EnsureBriefExportSync(context.Background(), repo.Root, "docs", "publishing", false)
	require.Error(t, err)
	require.ErrorContains(t, err, "drifted")
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
}

func TestEnsureBriefExportSyncTreatsFormatterMarkerPaddingAsInSync(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-12T10:00:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"\n"+
		"Docs orbit for $project_name\n"+
		"\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")

	status, err := InspectOrbitBriefLane(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, BriefLaneStateMaterializedInSync, status.State)

	_, err = EnsureBriefExportSync(context.Background(), repo.Root, "docs", "publishing", false)
	require.NoError(t, err)
}

func TestEnsureBriefExportSyncWarnsWhenBackfillRemovesHostedBrief(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-12T10:00:00Z\n"+
		"  updated_at: 2026-04-12T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")

	result, err := EnsureBriefExportSync(context.Background(), repo.Root, "docs", "publishing", true)
	require.NoError(t, err)
	require.True(t, result.Backfilled)
	require.Contains(t, result.Warning, "auto-removed orbit brief docs from ")

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "agents_template:")
}

func TestPublishTemplateRejectsOutOfRangeLocalSkillsWithoutFlags(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	_, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "extras/research-kit")
	require.ErrorContains(t, err, "--aggregate-detected-skills")
	require.ErrorContains(t, err, "--allow-out-of-range-skills")
}

func TestPublishTemplateRejectsCapabilityOwnedMemberOverlap(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - declared-skills/*\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - declared-skills/**\n"+
		"        - extras/**\n")
	repo.AddAndCommit(t, "seed invalid capability overlap")

	_, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `members[0].paths.include[1] overlaps capability-owned local skills path "declared-skills/*"`)
}

func TestPublishTemplateAggregatesDetectedSkillsBeforePublishing(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	result, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot:                repo.Root,
		OrbitID:                 "docs",
		AggregateDetectedSkills: true,
	})
	require.NoError(t, err)
	require.True(t, result.LocalSuccess)
	require.True(t, result.Changed)
	require.Empty(t, result.Preview.SavePreview.Warnings)

	require.NoFileExists(t, repo.Root+"/extras/research-kit/SKILL.md")
	require.FileExists(t, repo.Root+"/skills/docs/research-kit/SKILL.md")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, spec.Capabilities.Skills.Local.Paths.Include, "skills/docs/*")

	files := strings.Split(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")), "\n")
	require.Contains(t, files, "skills/docs/research-kit/SKILL.md")
	require.Contains(t, files, "skills/docs/research-kit/playbook.md")

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template")
}

func TestPublishTemplatePushPromptsAndPushesSourceBranchWhenRemoteMissing(t *testing.T) {
	t.Parallel()

	repo := seedPublishSourceRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)
	deletePublishBareRemoteRef(t, remoteURL, "refs/heads/main")

	var prompt SourceBranchPushPrompt
	result, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		Push:     true,
		Remote:   "origin",
		SourceBranchPushPrompter: sourceBranchPushPrompterFunc(func(_ context.Context, request SourceBranchPushPrompt) (SourceBranchPushDecision, error) {
			prompt = request
			return SourceBranchPushDecisionPush, nil
		}),
	})
	require.NoError(t, err)
	require.True(t, result.LocalSuccess)
	require.True(t, result.RemotePush.Attempted)
	require.True(t, result.RemotePush.Success)
	require.Equal(t, SourceBranchStatusMissing, prompt.Status)

	remoteMain := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/main")))
	require.Len(t, remoteMain, 2)
	require.Equal(t, repo.RevParse(t, "main"), remoteMain[0])

	remoteTemplate := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteTemplate, 2)
	require.Equal(t, result.Commit, remoteTemplate[0])
}

func TestPublishTemplatePushPromptsAndPushesSourceBranchWhenLocalAhead(t *testing.T) {
	t.Parallel()

	repo := seedPublishSourceRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)
	repo.WriteFile(t, "docs/guide.md", "Orbit guide v2\n")
	repo.AddAndCommit(t, "advance local source")

	var prompt SourceBranchPushPrompt
	result, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		Push:     true,
		Remote:   "origin",
		SourceBranchPushPrompter: sourceBranchPushPrompterFunc(func(_ context.Context, request SourceBranchPushPrompt) (SourceBranchPushDecision, error) {
			prompt = request
			return SourceBranchPushDecisionPush, nil
		}),
	})
	require.NoError(t, err)
	require.True(t, result.LocalSuccess)
	require.True(t, result.RemotePush.Attempted)
	require.True(t, result.RemotePush.Success)
	require.Equal(t, SourceBranchStatusAhead, prompt.Status)

	remoteMain := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/main")))
	require.Len(t, remoteMain, 2)
	require.Equal(t, repo.RevParse(t, "main"), remoteMain[0])
}

func TestPublishTemplatePushBlocksNonInteractiveWhenSourceBranchNeedsPush(t *testing.T) {
	t.Parallel()

	repo := seedPublishSourceRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)
	repo.WriteFile(t, "README.md", "local source update\n")
	repo.AddAndCommit(t, "advance local source")

	result, err := PublishTemplate(context.Background(), TemplatePublishInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		Push:     true,
		Remote:   "origin",
	})
	require.Error(t, err)

	var publishErr *PublishError
	require.ErrorAs(t, err, &publishErr)
	require.Equal(t, "source_branch_push_required", publishErr.Result.RemotePush.Reason)
	require.Equal(t, []string{
		"git push -u origin main",
		"rerun publish with --push",
	}, publishErr.Result.RemotePush.NextActions)
	require.False(t, result.LocalSuccess)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func seedPublishSourceRepo(t *testing.T) *testutil.Repo {
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
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed publish source repo")

	return repo
}

func deletePublishBareRemoteRef(t *testing.T, remoteURL string, ref string) {
	t.Helper()

	command := exec.Command("git", "--git-dir", remoteURL, "update-ref", "-d", ref)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "delete bare remote ref failed:\n%s", string(output))
}

type sourceBranchPushPrompterFunc func(ctx context.Context, prompt SourceBranchPushPrompt) (SourceBranchPushDecision, error)

func (fn sourceBranchPushPrompterFunc) PromptSourceBranchPush(ctx context.Context, prompt SourceBranchPushPrompt) (SourceBranchPushDecision, error) {
	return fn(ctx, prompt)
}
