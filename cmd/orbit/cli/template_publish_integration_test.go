package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplatePublishCreatesFixedTemplateBranchFromSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--default", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID         string `json:"orbit_id"`
		PublishRef      string `json:"publish_ref"`
		Branch          string `json:"branch"`
		SourceBranch    string `json:"source_branch"`
		DefaultTemplate bool   `json:"default_template"`
		LocalPublish    struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool `json:"attempted"`
			Success   bool `json:"success"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "refs/heads/orbit-template/docs", payload.PublishRef)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.DefaultTemplate)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)
	_, err = gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".orbit/source.yaml")
	require.Error(t, err)
}

func TestTemplatePublishCreatesFixedTemplateBranchFromHostedOnlySourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedHostedOnlyTemplatePublishRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
}

func TestTemplatePublishUsesZeroCommitProvenanceWithoutCommittedHeadOnSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedUncommittedTemplatePublishRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "created_from_branch: main")
	require.Contains(t, string(manifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestTemplatePublishFailsClosedOnDetachedHeadSourceRevision(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "publish requires a current branch; detached HEAD is not supported")
}

func TestTemplatePublishNoOpWhenCurrentOrbitTemplateBranchAlreadyMatchesGeneratedPayload(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID         string `json:"orbit_id"`
		PublishRef      string `json:"publish_ref"`
		Branch          string `json:"branch"`
		SourceBranch    string `json:"source_branch"`
		DefaultTemplate bool   `json:"default_template"`
		LocalPublish    struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool `json:"attempted"`
			Success   bool `json:"success"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "refs/heads/"+currentBranch, payload.PublishRef)
	require.Equal(t, currentBranch, payload.Branch)
	require.Equal(t, currentBranch, payload.SourceBranch)
	require.False(t, payload.DefaultTemplate)
	require.True(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)
}

func TestTemplatePublishCancelsWhenTrackedAuthorOnlyFilesAreRejectedOnCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	repo.WriteFile(t, "AGENTS.md", "temporary author entry\n")
	repo.WriteFile(t, "notes/scratch.md", "temporary author note\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide updated\n")
	repo.AddAndCommit(t, "add tracked author-only files")
	beforeHead := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	_, stderr, err := executeCLIWithInput(t, repo.Root, "n\n", "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "publish canceled")
	require.Equal(t, beforeHead, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
	require.Contains(t, repo.Run(t, "ls-files", "AGENTS.md"), "AGENTS.md")
	require.Contains(t, repo.Run(t, "ls-files", "notes/scratch.md"), "notes/scratch.md")
	require.Contains(t, stderr, "AGENTS.md")
	require.Contains(t, stderr, "notes/scratch.md")
	require.Contains(t, stderr, "continue? [y/N]")
}

func TestTemplatePublishInteractiveRemovesTrackedAuthorOnlyFilesFromCommitButKeepsThemInWorktree(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.WriteFile(t, "AGENTS.md", "temporary author entry\n")
	repo.WriteFile(t, "notes/scratch.md", "temporary author note\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide updated\n")
	repo.AddAndCommit(t, "add tracked author-only files")

	stdout, stderr, err := executeCLIWithInput(t, repo.Root, "y\n", "template", "publish", "--json")
	require.NoError(t, err)

	var payload struct {
		Branch       string   `json:"branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, currentBranch, payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.Contains(t, payload.Warnings, "restored tracked author-only files to the worktree as untracked files: AGENTS.md, notes/scratch.md")
	require.Contains(t, stderr, "AGENTS.md")
	require.Contains(t, stderr, "notes/scratch.md")

	exists, err := gitpkg.PathExistsAtRev(context.Background(), repo.Root, currentBranch, "AGENTS.md")
	require.NoError(t, err)
	require.False(t, exists)

	exists, err = gitpkg.PathExistsAtRev(context.Background(), repo.Root, currentBranch, "notes/scratch.md")
	require.NoError(t, err)
	require.False(t, exists)

	guideData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, currentBranch, "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, "Orbit guide updated\n", string(guideData))

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, "temporary author entry\n", string(agentsData))
	scratchData, err := os.ReadFile(filepath.Join(repo.Root, "notes", "scratch.md"))
	require.NoError(t, err)
	require.Equal(t, "temporary author note\n", string(scratchData))
	require.Contains(t, repo.Run(t, "status", "--short"), "?? AGENTS.md")
	require.Contains(t, repo.Run(t, "status", "--short"), "?? notes/")
}

func TestTemplatePublishFailsClosedOnDriftedBriefInCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepoWithStructuredBrief(t)
	agentsData, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Drifted guidance for $project_name\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))
	repo.AddAndCommit(t, "add drifted brief artifact")

	_, _, err = executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
	require.ErrorContains(t, err, "--backfill-brief")
}

func TestTemplatePublishFailsClosedOnDriftedMemberHintsFromSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	writeTemplatePublishMemberHintSpec(t, repo, nil)
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-rules\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Style\n")
	repo.AddAndCommit(t, "add drifted source member hint before publish")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "orbit member backfill --orbit docs")
	require.ErrorContains(t, err, "before publishing")
}

func TestTemplatePublishAllowsInSyncMemberHintsFromSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	writeTemplatePublishMemberHintSpec(t, repo, []orbitpkg.OrbitMember{
		{
			Name: "docs-rules",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/style.md"},
			},
		},
	})
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-rules\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Style\n")
	repo.AddAndCommit(t, "add in-sync source member hint before publish")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		LocalPublish struct {
			Success bool `json:"success"`
			Changed bool `json:"changed"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
}

func TestTemplatePublishFailsClosedOnDriftedMemberHintsInCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	writeTemplatePublishMemberHintSpec(t, repo, nil)
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-rules\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Style\n")
	repo.AddAndCommit(t, "add drifted template-branch member hint before publish")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "orbit member backfill --orbit docs")
	require.ErrorContains(t, err, "before publishing")
}

func TestTemplatePublishBackfillsDriftedBriefOnCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepoWithStructuredBrief(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	agentsData, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Drifted guidance for $project_name\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))
	repo.AddAndCommit(t, "add drifted brief artifact")

	stdout, stderr, err := executeCLIWithInput(t, repo.Root, "y\n", "template", "publish", "--backfill-brief", "--json")
	require.NoError(t, err)

	var payload struct {
		Branch       string   `json:"branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, currentBranch, payload.Branch)
	require.Contains(t, payload.Warnings, "auto-backfilled orbit brief docs into "+filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.Contains(t, payload.Warnings, "restored tracked author-only files to the worktree as untracked files: AGENTS.md")
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	exists, err := gitpkg.PathExistsAtRev(context.Background(), repo.Root, currentBranch, "AGENTS.md")
	require.NoError(t, err)
	require.False(t, exists)

	definitionData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, currentBranch, ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "Drifted guidance for $project_name")

	agentsDiskData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsDiskData), "Drifted guidance for $project_name")
	require.Contains(t, stderr, "AGENTS.md")
	require.Contains(t, repo.Run(t, "status", "--short"), "?? AGENTS.md")
}

func TestTemplatePublishPushesGeneratedCommitOnCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.WriteFile(t, "AGENTS.md", "temporary author entry\n")
	repo.WriteFile(t, "notes/scratch.md", "temporary author note\n")
	repo.AddAndCommit(t, "add temporary root agents")
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	stdout, stderr, err := executeCLIWithInput(t, repo.Root, "y\n", "template", "publish", "--push", "--json")
	require.NoError(t, err)

	var payload struct {
		Branch       string   `json:"branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, currentBranch, payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.Contains(t, payload.Warnings, "restored tracked author-only files to the worktree as untracked files: AGENTS.md, notes/scratch.md")
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	headCommit := strings.TrimSpace(repo.Run(t, "rev-parse", currentBranch))
	remoteRef := strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/"+currentBranch))
	require.Contains(t, remoteRef, headCommit)

	_, err = gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/"+currentBranch, "AGENTS.md")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Contains(t, stderr, "AGENTS.md")
	require.Contains(t, repo.Run(t, "status", "--short"), "?? AGENTS.md")
	require.Contains(t, repo.Run(t, "status", "--short"), "?? notes/")
}

func TestTemplatePublishAllowsDefaultOverrideOnCurrentOrbitTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedDirectTemplatePublishRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: "+strings.TrimSpace(repo.Run(t, "rev-parse", "main"))+"\n"+
		"  created_at: 2026-04-07T12:00:00Z\n")
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: "+strings.TrimSpace(repo.Run(t, "rev-parse", "main"))+"\n"+
		"  created_at: 2026-04-07T12:00:00Z\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "seed false default template manifest")

	stdout, stderr, err := executeCLIWithInput(t, repo.Root, "y\n", "template", "publish", "--default", "--json")
	require.NoError(t, err)

	var payload struct {
		Branch       string   `json:"branch"`
		Warnings     []string `json:"warnings"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, currentBranch, payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.Contains(t, payload.Warnings, "restored tracked author-only files to the worktree as untracked files: .orbit/template.yaml")
	require.Contains(t, stderr, ".orbit/template.yaml")

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, currentBranch, ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "default_template: true")
	require.Contains(t, repo.Run(t, "status", "--short"), "?? .orbit/")
}

func TestTemplatePublishFailsWithoutSourceManifest(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.Run(t, "rm", ".harness/manifest.yaml")
	repo.AddAndCommit(t, "remove source manifest")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
}

func TestTemplatePublishFailsWhenCurrentBranchDiffersFromSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.Run(t, "checkout", "-b", "feature")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "source branch")
	require.ErrorContains(t, err, "main")
}

func TestTemplatePublishFailsWhenCurrentTemplateBranchUsesLegacyDefinitionHost(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-12T10:00:00Z\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed legacy-only template branch")
	repo.Run(t, "checkout", "-b", "orbit-template/docs")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/orbits")
	require.ErrorContains(t, err, "hosted orbit definitions")
}

func TestTemplatePublishFailsWhenSourceBranchUsesLegacyDefinitionHost(t *testing.T) {
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
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed legacy-hosted source repo")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/orbits")
	require.ErrorContains(t, err, "source init")
}

func TestTemplatePublishFailsWhenSourceBranchStillContainsStrayLegacyDefinitions(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "API spec\n")
	repo.AddAndCommit(t, "add stray legacy orbit definition")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, ".orbit/orbits")
	require.ErrorContains(t, err, "source init")
}

func TestTemplatePublishNoOpWhenPublishedTemplateTreeIsUnchanged(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	firstStdout, firstStderr, firstErr := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, firstErr)
	require.Empty(t, firstStderr)

	var firstPayload struct {
		LocalPublish struct {
			Commit string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstStdout), &firstPayload))
	require.NotEmpty(t, firstPayload.LocalPublish.Commit)

	secondStdout, secondStderr, secondErr := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, secondErr)
	require.Empty(t, secondStderr)

	var secondPayload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(secondStdout), &secondPayload))
	require.True(t, secondPayload.LocalPublish.Success)
	require.False(t, secondPayload.LocalPublish.Changed)
	require.Empty(t, secondPayload.LocalPublish.Commit)

	headCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs"))
	require.Equal(t, firstPayload.LocalPublish.Commit, headCommit)
}

func TestTemplatePublishNoOpWhenPublishedBranchOnlyHasLegacyTemplateManifestDrift(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	firstStdout, firstStderr, firstErr := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, firstErr)
	require.Empty(t, firstStderr)

	var firstPayload struct {
		LocalPublish struct {
			Commit string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstStdout), &firstPayload))
	require.NotEmpty(t, firstPayload.LocalPublish.Commit)

	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: true\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-31T00:00:00Z\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "drift legacy template manifest only")
	repo.Run(t, "checkout", "main")

	secondStdout, secondStderr, secondErr := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, secondErr)
	require.Empty(t, secondStderr)

	var secondPayload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(secondStdout), &secondPayload))
	require.True(t, secondPayload.LocalPublish.Success)
	require.False(t, secondPayload.LocalPublish.Changed)
	require.Empty(t, secondPayload.LocalPublish.Commit)

	headCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs"))
	require.NotEqual(t, firstPayload.LocalPublish.Commit, headCommit)
}

func TestTemplatePublishRepairsStaleBranchManifest(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	_, _, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)

	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-06T00:00:00Z\n"+
		"  updated_at: 2026-04-06T00:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "corrupt branch manifest")
	repo.Run(t, "checkout", "main")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template")
	require.Contains(t, string(manifestData), "orbit_id: docs")
}

func TestTemplatePublishFailsWhenSourceBranchContainsMultipleDefinitions(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".harness/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "API spec\n")
	repo.AddAndCommit(t, "add second orbit definition")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "exactly one")
	require.ErrorContains(t, err, "orbit definition")

	_, _, err = executeCLI(t, repo.Root, "template", "publish", "--orbit", "api")
	require.Error(t, err)
	require.ErrorContains(t, err, "exactly one")
}

func TestTemplatePublishFailsWhenExplicitOrbitDiffersFromSingleSourceOrbit(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	_, _, err := executeCLI(t, repo.Root, "template", "publish", "--orbit", "api")
	require.Error(t, err)
	require.ErrorContains(t, err, "single source orbit")
	require.ErrorContains(t, err, "docs")
}

func TestTemplatePublishFailsWhenSourceOrbitIDDoesNotMatchSingleSourceOrbit(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: api\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "mismatch source orbit id")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "source.orbit_id")
	require.ErrorContains(t, err, "docs")
}

func TestTemplatePublishFailsWhenSourceOrbitIDIsMissing(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "remove source orbit id")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, "source.orbit_id")
}

func TestTemplatePublishFailsWhenSourceBranchContainsHarnessMetadata(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.AddAndCommit(t, "add forbidden harness metadata")

	_, _, err := executeCLI(t, repo.Root, "template", "publish")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/")
}

func TestTemplatePublishIgnoresLegacyTemplateManifestOnSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-31T00:00:00Z\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "introduce conflicting template marker")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	_, err = gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".orbit/template.yaml")
	require.Error(t, err)
}

func TestTemplatePublishRejectsRemoteWithoutPush(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	_, _, err := executeCLI(t, repo.Root, "template", "publish", "--remote", "origin")
	require.Error(t, err)
	require.ErrorContains(t, err, "--remote")
	require.ErrorContains(t, err, "--push")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplatePublishPushesToDefaultOriginWhenSourceBranchEqualsRemote(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	remoteRef := strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs"))
	require.Contains(t, remoteRef, payload.LocalPublish.Commit)
}

func TestTemplatePublishPushesToExplicitRemoteWhenRequested(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "upstream", remoteURL)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--remote", "upstream", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "upstream", payload.RemotePush.Remote)

	remoteRef := strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs"))
	require.Contains(t, remoteRef, payload.LocalPublish.Commit)
}

func TestTemplatePublishPushesWhenLocalSourceBranchIsAheadOfRemote(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	repo.WriteFile(t, "README.md", "local ahead change\n")
	repo.AddAndCommit(t, "advance local main only")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)
}

func TestTemplatePublishNoOpStillPushesWhenRequested(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	firstStdout, firstStderr, firstErr := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, firstErr)
	require.Empty(t, firstStderr)

	var firstPayload struct {
		LocalPublish struct {
			Commit string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstStdout), &firstPayload))
	require.NotEmpty(t, firstPayload.LocalPublish.Commit)

	secondStdout, secondStderr, secondErr := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, secondErr)
	require.Empty(t, secondStderr)

	var secondPayload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(secondStdout), &secondPayload))
	require.True(t, secondPayload.LocalPublish.Success)
	require.False(t, secondPayload.LocalPublish.Changed)
	require.Empty(t, secondPayload.LocalPublish.Commit)
	require.True(t, secondPayload.RemotePush.Attempted)
	require.True(t, secondPayload.RemotePush.Success)
	require.Equal(t, "origin", secondPayload.RemotePush.Remote)

	remoteRef := strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs"))
	require.Contains(t, remoteRef, firstPayload.LocalPublish.Commit)
}

func TestTemplatePublishPushesWhenRemoteTemplateBranchAdvancedBeyondLocal(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	_, _, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)

	repo.WriteFile(t, "docs/guide.md", "Orbit guide v2\n")
	repo.AddAndCommit(t, "advance local source branch")

	advanceRemoteBranch(t, remoteURL, "orbit-template/docs", "advance remote published template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", payload.LocalPublish.Commit))
	require.Contains(t, parentLine, previousRemoteCommit)

	remoteRef := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteRef, 2)
	require.Equal(t, payload.LocalPublish.Commit, remoteRef[0])

	remoteGuide, readErr := gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/orbit-template/docs", "docs/guide.md")
	require.NoError(t, readErr)
	require.Equal(t, "Orbit guide v2\n", string(remoteGuide))

	_, readErr = gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/orbit-template/docs", "docs/remote-note.md")
	require.Error(t, readErr)
}

func TestTemplatePublishPushesWhenRemoteTemplateBranchDivergedFromMatchingLocalTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	firstStdout, firstStderr, firstErr := executeCLI(t, repo.Root, "template", "publish", "--json")
	require.NoError(t, firstErr)
	require.Empty(t, firstStderr)

	var firstPayload struct {
		LocalPublish struct {
			Commit string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstStdout), &firstPayload))
	require.NotEmpty(t, firstPayload.LocalPublish.Commit)

	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	advanceRemoteBranch(t, remoteURL, "orbit-template/docs", "advance remote published template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.NotEqual(t, firstPayload.LocalPublish.Commit, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", payload.LocalPublish.Commit))
	require.Contains(t, parentLine, previousRemoteCommit)

	remoteRef := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteRef, 2)
	require.Equal(t, payload.LocalPublish.Commit, remoteRef[0])

	remoteGuide, readErr := gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/orbit-template/docs", "docs/guide.md")
	require.NoError(t, readErr)
	require.Equal(t, "Orbit guide\n", string(remoteGuide))

	_, readErr = gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/orbit-template/docs", "docs/remote-note.md")
	require.Error(t, readErr)
}

func TestTemplatePublishSyncsRemoteNoOpBranchWithVisibleParentHistory(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	cloneRoot := cloneRemoteRepo(t, remoteURL)
	stdout, stderr, err := executeCLI(t, cloneRoot, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	advanceRemoteBranch(t, remoteURL, "orbit-template/docs", "advance remote published template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	removeRemoteBranchPath(t, remoteURL, "orbit-template/docs", "remove remote published template note", "docs/remote-note.md")
	finalRemoteHead := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/orbit-template/docs")))
	require.Len(t, finalRemoteHead, 2)
	finalRemoteCommit := finalRemoteHead[0]

	stdout, stderr, err = executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	localHead := strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs"))
	require.Equal(t, finalRemoteCommit, localHead)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", "orbit-template/docs"))
	require.Contains(t, parentLine, previousRemoteCommit)
}

func TestTemplatePublishBlocksPushWhenLocalSourceBranchIsBehindRemote(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)
	advanceRemoteBranch(t, remoteURL, "main", "advance remote main", "README.md", "remote change\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Branch       string `json:"branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
			Reason    string `json:"reason"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "orbit-template/docs", payload.Branch)
	require.False(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)
	require.Equal(t, "source_branch_not_up_to_date", payload.RemotePush.Reason)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplatePublishBlocksPushWhenLocalSourceBranchDivergesFromRemote(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	repo.WriteFile(t, "README.md", "local change\n")
	repo.AddAndCommit(t, "local main diverges")
	advanceRemoteBranch(t, remoteURL, "main", "remote main diverges", "docs/guide.md", "remote guide\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
			Reason    string `json:"reason"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)
	require.Equal(t, "source_branch_not_up_to_date", payload.RemotePush.Reason)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplatePublishFailsBeforeLocalPublishWhenRemoteIsMissing(t *testing.T) {
	t.Parallel()

	repo := seedTemplatePublishRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "publish", "--push", "--remote", "missing", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted bool   `json:"attempted"`
			Success   bool   `json:"success"`
			Remote    string `json:"remote"`
			Reason    string `json:"reason"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Success)
	require.Equal(t, "missing", payload.RemotePush.Remote)
	require.NotEmpty(t, payload.RemotePush.Reason)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func seedTemplatePublishRepo(t *testing.T) *testutil.Repo {
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
	repo.AddAndCommit(t, "seed template source repo")

	return repo
}

func seedUncommittedTemplatePublishRepo(t *testing.T) *testutil.Repo {
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
	repo.Run(t, "add", "-A")

	return repo
}

func writeTemplatePublishMemberHintSpec(t *testing.T, repo *testutil.Repo, members []orbitpkg.OrbitMember) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	spec.Members = append([]orbitpkg.OrbitMember(nil), members...)

	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
}

func seedHostedOnlyTemplatePublishRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
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
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed hosted-only template source repo")

	return repo
}

func seedDirectTemplatePublishRepo(t *testing.T) *testutil.Repo {
	return seedDirectTemplatePublishRepoOnBranch(t, "author/docs")
}

func seedDirectTemplatePublishRepoOnBranch(t *testing.T, branch string) *testutil.Repo {
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
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo for direct template publish")

	_, err := orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: branch,
			Now:          time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		},
		Overwrite: true,
	})
	require.NoError(t, err)

	repo.Run(t, "checkout", branch)

	return repo
}

func seedDirectTemplatePublishRepoWithStructuredBrief(t *testing.T) *testutil.Repo {
	return seedDirectTemplatePublishRepoWithStructuredBriefOnBranch(t, "author/docs")
}

func seedDirectTemplatePublishRepoWithStructuredBriefOnBranch(t *testing.T, branch string) *testutil.Repo {
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
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo for structured direct template publish")

	_, err := orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: branch,
			Now:          time.Date(2026, time.April, 7, 12, 0, 0, 0, time.UTC),
		},
		Overwrite: true,
	})
	require.NoError(t, err)

	repo.Run(t, "checkout", branch)

	return repo
}

func advanceRemoteBranch(t *testing.T, remoteURL string, branch string, message string, path string, contents string) {
	t.Helper()

	cloneRoot := cloneRemoteRepo(t, remoteURL)
	run := func(args ...string) {
		t.Helper()
		runGitInDir(t, cloneRoot, args...)
	}
	run("checkout", branch)

	target := filepath.Join(cloneRoot, filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte(contents), 0o600))
	run("add", "--", path)
	run("commit", "-m", message)
	run("push", "origin", branch)
}

func removeRemoteBranchPath(t *testing.T, remoteURL string, branch string, message string, path string) {
	t.Helper()

	cloneRoot := cloneRemoteRepo(t, remoteURL)
	runGitInDir(t, cloneRoot, "checkout", branch)
	runGitInDir(t, cloneRoot, "rm", "--", path)
	runGitInDir(t, cloneRoot, "commit", "-m", message)
	runGitInDir(t, cloneRoot, "push", "origin", branch)
}

func cloneRemoteRepo(t *testing.T, remoteURL string) string {
	t.Helper()

	cloneRoot := filepath.Join(t.TempDir(), "clone")
	command := exec.Command("git", "clone", remoteURL, cloneRoot)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git clone failed:\n%s", string(output))

	runGitInDir(t, cloneRoot, "config", "user.name", "Orbit Test")
	runGitInDir(t, cloneRoot, "config", "user.email", "orbit@example.com")

	return cloneRoot
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(output))
}
