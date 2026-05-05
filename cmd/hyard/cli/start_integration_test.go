package cli_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardStartPrintPromptPrintsStartPromptInRuntime(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Harness Start")
	require.Contains(t, stdout, "Start Prompt")
	require.Contains(t, stdout, "First handle any pending Harness Runtime bootstrap work.")
	require.Contains(t, stdout, "Then introduce this Harness Runtime in the same session.")
}

func TestHyardStartPrintPromptFailsClearlyOutsideHarnessRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "Ordinary repository\n")
	repo.AddAndCommit(t, "seed ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "outside a Harness Runtime")
}

func TestHyardStartPrintPromptDoesNotMutateRuntimeOrAgentState(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	beforeStatus := repo.Run(t, "status", "--short")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "start", "--print-prompt")
	require.NoError(t, err)
	require.NotEmpty(t, stdout)
	require.Empty(t, stderr)

	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	require.NoFileExists(t, harnesspkg.FrameworkSelectionPath(repo.GitDir(t)))
	require.NoDirExists(t, filepath.Join(repo.GitDir(t), "orbit", "state", "agents", "activations"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".claude", "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "skills", harnesspkg.BootstrapAgentSkillName, "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "BOOTSTRAP.md"))
}
