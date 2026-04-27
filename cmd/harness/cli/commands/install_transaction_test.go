package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestInstallOverwriteRollsBackWhenCleanupFails(t *testing.T) {
	repo := seedInstallCommandRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeInstallCommand(t, repo.Root, "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before overwrite")

	originalRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	repo.Run(t, "checkout", runtimeBranch)

	beforeInstallOwnedCleanupHook = func(repoRoot string, orbitID string, plan orbittemplate.InstallOwnedCleanupPlan) {
		require.Equal(t, "docs", orbitID)
		require.Contains(t, plan.DeletePaths, "docs/guide.md")
		replaceOwnedPathWithNonEmptyDirectory(t, filepath.Join(repoRoot, "docs", "guide.md"))
	}
	t.Cleanup(func() {
		beforeInstallOwnedCleanupHook = nil
	})

	stdout, stderr, err := executeInstallCommand(
		t,
		repo.Root,
		"orbit-template/docs",
		"--bindings",
		bindingsPath,
		"--overwrite-existing",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "remove stale install-owned paths")
	require.ErrorContains(t, err, "docs/guide.md")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	restoredRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, originalRecord.Template.TemplateCommit, restoredRecord.Template.TemplateCommit)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)

	transactionsDir := filepath.Join(repo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestInstallBatchOverwriteRollsBackWhenCleanupFails(t *testing.T) {
	repo := seedInstallCommandRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeInstallBatchCommand(t, repo.Root, "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before batch overwrite")

	originalRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	originalVarsData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoError(t, err)

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents for batch overwrite")
	repo.Run(t, "checkout", runtimeBranch)

	beforeInstallOwnedCleanupHook = func(repoRoot string, orbitID string, plan orbittemplate.InstallOwnedCleanupPlan) {
		require.Equal(t, "docs", orbitID)
		require.Contains(t, plan.DeletePaths, "docs/guide.md")
		replaceOwnedPathWithNonEmptyDirectory(t, filepath.Join(repoRoot, "docs", "guide.md"))
	}
	t.Cleanup(func() {
		beforeInstallOwnedCleanupHook = nil
	})

	stdout, stderr, err := executeInstallBatchCommand(
		t,
		repo.Root,
		"orbit-template/docs",
		"--bindings",
		bindingsPath,
		"--overwrite-existing",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "remove stale install-owned paths")
	require.ErrorContains(t, err, "docs/guide.md")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	restoredVarsData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(originalVarsData), string(restoredVarsData))

	restoredRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, originalRecord.Template.TemplateCommit, restoredRecord.Template.TemplateCommit)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)

	transactionsDir := filepath.Join(repo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestInspectInstallTargetStateReadsDetachedInstallRecordHiddenByProjection(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 21, 0, 0, 0, time.UTC)
	runtimeFile, err := harnesspkg.DefaultRuntimeFile(repo.Root, now)
	require.NoError(t, err)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "runtime root\n")
	repo.AddAndCommit(t, "seed detached install target state")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	runtimeFile, err = harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)

	state, err := inspectInstallTargetState(repo.Root, runtimeFile, "docs")
	require.NoError(t, err)
	require.True(t, state.HasInstallRecord)
	require.Equal(t, orbittemplate.InstallRecordStatusDetached, orbittemplate.EffectiveInstallRecordStatus(state.ExistingRecord))
}

func executeInstallCommand(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	cmd := NewInstallCommand()
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.ExecuteContext(WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func executeInstallBatchCommand(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	cmd := NewInstallBatchCommand()
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.ExecuteContext(WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func seedInstallCommandRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 14, 12, 0, 0, 0, time.UTC)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

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
	repo.AddAndCommit(t, "seed runtime repo")

	_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          now,
		},
	})
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", ".harness/vars.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	return repo
}

func replaceOwnedPathWithNonEmptyDirectory(t *testing.T, absolutePath string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(absolutePath))
	require.NoError(t, os.MkdirAll(absolutePath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(absolutePath, "blocked.txt"), []byte("keep"), 0o600))
}
