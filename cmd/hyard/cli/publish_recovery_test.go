package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardPublishInteractiveRecoveryPreparesCheckpointsThenAllowsPublish(t *testing.T) {
	t.Parallel()

	repo := seedHyardPublishRecoveryRepo(t)
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	cmd, stdout, stderr := newHyardPublishRecoveryCommand(t, repo.Root, "\ny\ny\n")
	decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, "docs")
	require.NoError(t, err)
	require.Equal(t, hyardPublishRecoveryContinue, decision)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), `Orbit package "docs" is not ready to publish.`)
	require.Contains(t, stderr.String(), "Required before publish:")
	require.Contains(t, stderr.String(), "docs/guide.md")
	require.Contains(t, stderr.String(), `Apply 1 content hint to package "docs"?`)
	require.Contains(t, stderr.String(), `Create checkpoint commit "Update docs" for package "docs"?`)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Update docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))

	worktreeGuide, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.NotContains(t, string(worktreeGuide), "orbit_member:")

	publishStdout, publishStderr, err := executeHyardPublishRecoveryCLI(t, repo.Root, "publish", "orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, publishStderr)
	var payload struct {
		LocalPublish struct {
			Success bool `json:"success"`
		} `json:"local_publish"`
	}
	require.NoError(t, json.Unmarshal([]byte(publishStdout), &payload))
	require.True(t, payload.LocalPublish.Success)

	publishedGuide, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Contains(t, string(publishedGuide), "# Edited Guide")
	require.NotContains(t, string(publishedGuide), "orbit_member:")
}

func TestHyardPublishInteractiveRecoverySavesGuideDriftBeforeCheckpoint(t *testing.T) {
	t.Parallel()

	repo := seedHyardPublishRecoveryRepo(t)
	writeHyardPublishRecoveryHostedDocsOrbitWithStructuredBrief(t, repo.Root)
	block, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Edited docs guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(block))
	repo.AddAndCommit(t, "seed drifted docs guidance")
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	cmd, stdout, stderr := newHyardPublishRecoveryCommand(t, repo.Root, "\ny\ny\ny\n")
	decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, "docs")
	require.NoError(t, err)
	require.Equal(t, hyardPublishRecoveryContinue, decision)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "Save guide changes from AGENTS.md")
	require.Contains(t, stderr.String(), `Save guide changes from AGENTS.md for package "docs"?`)
	require.Contains(t, stderr.String(), `Create checkpoint commit "Update docs" for package "docs"?`)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Update docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "Edited docs guidance\n", spec.Meta.AgentsTemplate)
}

func TestHyardPublishInteractiveRecoveryTracksSafeUntrackedExportBeforeCheckpoint(t *testing.T) {
	t.Parallel()

	repo := seedHyardPublishRecoveryRepo(t)
	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed docs export member")
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/new.md", "# New export file\n")

	cmd, stdout, stderr := newHyardPublishRecoveryCommand(t, repo.Root, "\ny\ny\n")
	decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, "docs")
	require.NoError(t, err)
	require.Equal(t, hyardPublishRecoveryContinue, decision)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "Track new package files")
	require.Contains(t, stderr.String(), `Track 1 new package file for package "docs"?`)
	require.Contains(t, stderr.String(), `Create checkpoint commit "Update docs" for package "docs"?`)

	sourceAfter := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	require.NotEqual(t, sourceBefore, sourceAfter)
	require.Equal(t, "Update docs", strings.TrimSpace(repo.Run(t, "log", "-1", "--pretty=%B")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "status", "--short")))
	require.Contains(t, repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD"), "docs/new.md")
}

func TestHyardPublishInteractiveRecoveryHidesCheckpointOnlyWhenPrepareIsRequired(t *testing.T) {
	t.Parallel()

	repo := seedHyardPublishRecoveryRepo(t)
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	cmd, stdout, stderr := newHyardPublishRecoveryCommand(t, repo.Root, "q\n")
	decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, "docs")
	require.Error(t, err)
	require.Equal(t, hyardPublishRecoveryStop, decision)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), "  [Enter] prepare, track, checkpoint, then publish")
	require.Contains(t, stderr.String(), "  p       prepare only")
	require.NotContains(t, stderr.String(), "  c       checkpoint only")
}

func TestHyardPublishInteractiveRecoveryAbortDoesNotMutate(t *testing.T) {
	t.Parallel()

	repo := seedHyardPublishRecoveryRepo(t)
	sourceBefore := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.WriteFile(t, "docs/guide.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-guide\n"+
		"---\n"+
		"\n"+
		"# Edited Guide\n")

	cmd, stdout, stderr := newHyardPublishRecoveryCommand(t, repo.Root, "q\n")
	decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, "docs")
	require.Error(t, err)
	require.Equal(t, hyardPublishRecoveryStop, decision)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), `Orbit package "docs" is not ready to publish.`)
	require.ErrorContains(t, err, `publish canceled for package "docs"`)
	require.Equal(t, sourceBefore, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
	require.Empty(t, strings.TrimSpace(repo.Run(t, "branch", "--list", "orbit-template/docs")))

	worktreeGuide, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Contains(t, string(worktreeGuide), "orbit_member:")
}

func newHyardPublishRecoveryCommand(t *testing.T, workingDir string, stdin string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader(stdin))
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetContext(hyardPublishRecoveryContext(workingDir))

	return cmd, &stdout, &stderr
}

func executeHyardPublishRecoveryCLI(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	cmd := NewRootCommand()
	cmd.SetArgs(args)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.ExecuteContext(hyardPublishRecoveryContext(workingDir))

	return stdout.String(), stderr.String(), err
}

func hyardPublishRecoveryContext(workingDir string) context.Context {
	ctx := harnesscommands.WithWorkingDir(context.Background(), workingDir)
	ctx = orbitcommands.WithWorkingDir(ctx, workingDir)
	ctx = WithWorkingDir(ctx, workingDir)
	return ctx
}

func seedHyardPublishRecoveryRepo(t *testing.T) *testutil.Repo {
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
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Original guide\n")
	repo.AddAndCommit(t, "seed hyard publish recovery repo")

	return repo
}

func writeHyardPublishRecoveryHostedDocsOrbitWithStructuredBrief(t *testing.T, repoRoot string) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Docs orbit guidance\n"
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberSubject,
		orbitpkg.OrbitMemberRule,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repoRoot, spec)
	require.NoError(t, err)
}
