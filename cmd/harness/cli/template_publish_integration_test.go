package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessTemplatePublishLocalJSONContract(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--default", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID       string `json:"harness_id"`
		PublishRef      string `json:"publish_ref"`
		Branch          string `json:"branch"`
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
	require.Equal(t, "refs/heads/harness-template/workspace", payload.PublishRef)
	require.Equal(t, "harness-template/workspace", payload.Branch)
	require.True(t, payload.DefaultTemplate)
	require.NotEmpty(t, payload.HarnessID)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)

	manifestData, readErr := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/manifest.yaml")
	require.NoError(t, readErr)
	require.Contains(t, string(manifestData), "kind: harness_template")
}

func TestHarnessTemplatePublishUsesZeroCommitProvenanceWithoutCommittedHeadOnRuntimeBranch(t *testing.T) {
	t.Parallel()

	repo := seedUncommittedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID    string `json:"harness_id"`
		Branch       string `json:"branch"`
		SourceBranch string `json:"source_branch"`
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotEmpty(t, payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.Branch)
	require.Equal(t, "main", payload.SourceBranch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "created_from_branch: main")
	require.Contains(t, string(manifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestHarnessTemplatePublishFailsClosedOnDetachedHeadRuntimeRevision(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "harness template publish requires a current branch; detached HEAD is not supported")
}

func TestHarnessTemplatePublishNoOpWhenPublishedTemplateIsUnchanged(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	firstStdout, firstStderr, firstErr := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--json")
	require.NoError(t, firstErr)
	require.Empty(t, firstStderr)

	var firstPayload struct {
		LocalPublish struct {
			Commit string `json:"commit"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstStdout), &firstPayload))
	require.NotEmpty(t, firstPayload.LocalPublish.Commit)

	secondStdout, secondStderr, secondErr := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--json")
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

	headCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "harness-template/workspace"))
	require.Equal(t, firstPayload.LocalPublish.Commit, headCommit)
}

func TestHarnessTemplatePublishRejectsRemoteWithoutPush(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--remote", "origin")
	require.Error(t, err)
	require.ErrorContains(t, err, "--remote")
	require.ErrorContains(t, err, "--push")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestHarnessTemplatePublishPushesWhenRuntimeBranchEqualsRemote(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
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

	remoteRef := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteRef, 2)
	require.Equal(t, payload.LocalPublish.Commit, remoteRef[0])
}

func TestHarnessTemplatePublishFailsBeforeLocalPublishWhenRuntimeBranchIsBehindRemote(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	advanceHarnessRemoteBranch(t, remoteURL, currentBranch, "advance remote runtime branch", "README.md", "remote runtime change\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
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

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestHarnessTemplatePublishBlocksNonInteractivePushWhenRuntimeBranchIsAheadOfRemote(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.WriteFile(t, "README.md", "local runtime change\n")
	repo.AddAndCommit(t, "advance local runtime branch")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		LocalPublish struct {
			Success bool   `json:"success"`
			Changed bool   `json:"changed"`
			Commit  string `json:"commit"`
		} `json:"local_publish"`
		RemotePush struct {
			Attempted          bool     `json:"attempted"`
			Success            bool     `json:"success"`
			Remote             string   `json:"remote"`
			Reason             string   `json:"reason"`
			SourceBranchStatus string   `json:"source_branch_status"`
			NextActions        []string `json:"next_actions"`
		} `json:"remote_push"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.False(t, payload.RemotePush.Attempted)
	require.False(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)
	require.Equal(t, "source_branch_push_required", payload.RemotePush.Reason)
	require.Equal(t, "ahead", payload.RemotePush.SourceBranchStatus)
	require.Equal(t, []string{
		"git push -u origin " + currentBranch,
		"rerun publish with --push",
	}, payload.RemotePush.NextActions)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestHarnessTemplatePublishPushesWhenRemoteTemplateBranchAdvancedBeyondLocal(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
	require.NoError(t, err)

	repo.WriteFile(t, "docs/guide.md", "Orbit guide v2\n")
	repo.AddAndCommit(t, "advance runtime branch")
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.Run(t, "push", "origin", currentBranch)

	advanceHarnessRemoteBranch(t, remoteURL, "harness-template/workspace", "advance remote harness template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
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
	require.Equal(t, "harness-template/workspace", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", payload.LocalPublish.Commit))
	require.Contains(t, parentLine, previousRemoteCommit)

	remoteRef := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteRef, 2)
	require.Equal(t, payload.LocalPublish.Commit, remoteRef[0])

	remoteGuide, readErr := gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/harness-template/workspace", "docs/guide.md")
	require.NoError(t, readErr)
	require.Equal(t, "$project_name guide v2\n", string(remoteGuide))

	_, readErr = gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/harness-template/workspace", "docs/remote-note.md")
	require.Error(t, readErr)
}

func TestHarnessTemplatePublishPushesWhenRemoteTemplateBranchDivergedFromMatchingLocalTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	firstStdout, firstStderr, firstErr := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--json")
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

	advanceHarnessRemoteBranch(t, remoteURL, "harness-template/workspace", "advance remote harness template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
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
	require.Equal(t, "harness-template/workspace", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.True(t, payload.LocalPublish.Changed)
	require.NotEmpty(t, payload.LocalPublish.Commit)
	require.NotEqual(t, firstPayload.LocalPublish.Commit, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", payload.LocalPublish.Commit))
	require.Contains(t, parentLine, previousRemoteCommit)

	remoteRef := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteRef, 2)
	require.Equal(t, payload.LocalPublish.Commit, remoteRef[0])

	remoteGuide, readErr := gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/harness-template/workspace", "docs/guide.md")
	require.NoError(t, readErr)
	require.Equal(t, "$project_name guide\n", string(remoteGuide))

	_, readErr = gitpkg.ReadFileAtRemoteRef(context.Background(), repo.Root, remoteURL, "refs/heads/harness-template/workspace", "docs/remote-note.md")
	require.Error(t, readErr)
}

func TestHarnessTemplatePublishSyncsRemoteNoOpBranchWithVisibleParentHistory(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, repo)
	repo.Run(t, "remote", "add", "origin", remoteURL)

	cloneRoot := cloneHarnessRemoteRepo(t, remoteURL)
	stdout, stderr, err := executeHarnessCLI(t, cloneRoot, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	advanceHarnessRemoteBranch(t, remoteURL, "harness-template/workspace", "advance remote harness template", "docs/remote-note.md", "remote template note\n")
	remoteHeadBefore := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, remoteHeadBefore, 2)
	previousRemoteCommit := remoteHeadBefore[0]

	removeHarnessRemoteBranchPath(t, remoteURL, "harness-template/workspace", "remove remote harness template note", "docs/remote-note.md")
	finalRemoteHead := strings.Fields(strings.TrimSpace(repo.Run(t, "ls-remote", remoteURL, "refs/heads/harness-template/workspace")))
	require.Len(t, finalRemoteHead, 2)
	finalRemoteCommit := finalRemoteHead[0]

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--push", "--json")
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
	require.Equal(t, "harness-template/workspace", payload.Branch)
	require.True(t, payload.LocalPublish.Success)
	require.False(t, payload.LocalPublish.Changed)
	require.Empty(t, payload.LocalPublish.Commit)
	require.True(t, payload.RemotePush.Attempted)
	require.True(t, payload.RemotePush.Success)
	require.Equal(t, "origin", payload.RemotePush.Remote)

	localHead := strings.TrimSpace(repo.Run(t, "rev-parse", "harness-template/workspace"))
	require.Equal(t, finalRemoteCommit, localHead)

	parentLine := strings.TrimSpace(repo.Run(t, "rev-list", "--parents", "-n", "1", "harness-template/workspace"))
	require.Contains(t, parentLine, previousRemoteCommit)
}

func advanceHarnessRemoteBranch(t *testing.T, remoteURL string, branch string, message string, path string, contents string) {
	t.Helper()

	cloneRoot := cloneHarnessRemoteRepo(t, remoteURL)
	run := func(args ...string) {
		t.Helper()
		runHarnessGitInDir(t, cloneRoot, args...)
	}
	run("checkout", branch)

	target := filepath.Join(cloneRoot, filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte(contents), 0o600))
	run("add", "--", path)
	run("commit", "-m", message)
	run("push", "origin", branch)
}

func removeHarnessRemoteBranchPath(t *testing.T, remoteURL string, branch string, message string, path string) {
	t.Helper()

	cloneRoot := cloneHarnessRemoteRepo(t, remoteURL)
	runHarnessGitInDir(t, cloneRoot, "checkout", branch)
	runHarnessGitInDir(t, cloneRoot, "rm", "--", path)
	runHarnessGitInDir(t, cloneRoot, "commit", "-m", message)
	runHarnessGitInDir(t, cloneRoot, "push", "origin", branch)
}

func cloneHarnessRemoteRepo(t *testing.T, remoteURL string) string {
	t.Helper()

	cloneRoot := filepath.Join(t.TempDir(), "clone")
	command := exec.Command("git", "clone", remoteURL, cloneRoot)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git clone failed:\n%s", string(output))

	runHarnessGitInDir(t, cloneRoot, "config", "user.name", "Orbit Test")
	runHarnessGitInDir(t, cloneRoot, "config", "user.email", "orbit@example.com")

	return cloneRoot
}

func runHarnessGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(output))
}

func seedUncommittedHarnessTemplateSaveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"description: Cmd orbit\n"+
		"include:\n"+
		"  - cmd/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n\nconst name = \"orbitctl\"\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace guide for Orbit\n"+
		"<!-- keep -->\n"+
		"Use orbitctl consistently.\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)

	repo.Run(t, "add", "-A")

	return repo
}
