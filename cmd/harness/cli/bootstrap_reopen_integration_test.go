package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestHarnessBootstrapReopenOrbitClearsCompletionStateWithoutRestoringSurface(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})

	_, _, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "reopen", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot             string   `json:"harness_root"`
		ReopenedOrbits          []string `json:"reopened_orbits"`
		AlreadyPendingOrbits    []string `json:"already_pending_orbits"`
		RestoredPaths           []string `json:"restored_paths"`
		RestoredBootstrapBlocks []string `json:"restored_bootstrap_blocks"`
		CreatedBootstrapFile    bool     `json:"created_bootstrap_file"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, []string{"docs"}, payload.ReopenedOrbits)
	require.Empty(t, payload.AlreadyPendingOrbits)
	require.Empty(t, payload.RestoredPaths)
	require.Empty(t, payload.RestoredBootstrapBlocks)
	require.False(t, payload.CreatedBootstrapFile)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	snapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Nil(t, snapshot.Bootstrap)

	_, err = os.Stat(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessBootstrapReopenAllLeavesPendingOrbitsAsStableNoOp(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{
		includeOpsOrbit: true,
	})

	_, _, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "reopen", "--all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ReopenedOrbits       []string `json:"reopened_orbits"`
		AlreadyPendingOrbits []string `json:"already_pending_orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"docs"}, payload.ReopenedOrbits)
	require.Equal(t, []string{"ops"}, payload.AlreadyPendingOrbits)
}

func TestHarnessBootstrapReopenRestoreSurfaceRestoresBootstrapFilesAndBlock(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})

	_, _, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "reopen", "--orbit", "docs", "--restore-surface", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ReopenedOrbits          []string `json:"reopened_orbits"`
		RestoredPaths           []string `json:"restored_paths"`
		RestoredBootstrapBlocks []string `json:"restored_bootstrap_blocks"`
		CreatedBootstrapFile    bool     `json:"created_bootstrap_file"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"docs"}, payload.ReopenedOrbits)
	require.Contains(t, payload.RestoredPaths, "bootstrap/docs/setup.md")
	require.Contains(t, payload.RestoredPaths, "BOOTSTRAP.md")
	require.Equal(t, []string{"docs"}, payload.RestoredBootstrapBlocks)
	require.True(t, payload.CreatedBootstrapFile)

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), `orbit_id="docs"`)
	require.Contains(t, string(bootstrapData), "Bootstrap the docs orbit.\n")

	restoredData, err := os.ReadFile(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.NoError(t, err)
	require.Equal(t, "Docs bootstrap setup\n", string(restoredData))

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	snapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Nil(t, snapshot.Bootstrap)
}

func TestHarnessBootstrapReopenRestoreSurfaceFailsWhenRestoreSourceIsUnclear(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})

	_, _, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit bootstrap completion cleanup")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "reopen", "--orbit", "docs", "--restore-surface")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot restore bootstrap runtime files for orbit "docs" because the current runtime does not expose a stable restore source`)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	snapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.NotNil(t, snapshot.Bootstrap)
	require.True(t, snapshot.Bootstrap.Completed)
}

func TestHarnessBootstrapReopenAllowsBootstrapGuidanceMaterializeAgain(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})

	_, _, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "bootstrap", "reopen", "--orbit", "docs", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeOrbitCLI(t, repo.Root, "guidance", "materialize", "--orbit", "docs", "--target", "bootstrap", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Target    string `json:"target"`
		Artifacts []struct {
			Target string `json:"target"`
			Path   string `json:"path"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "bootstrap", payload.Target)
	require.Len(t, payload.Artifacts, 1)
	require.Equal(t, "bootstrap", payload.Artifacts[0].Target)
	require.Equal(t, filepath.Join(repo.Root, "BOOTSTRAP.md"), payload.Artifacts[0].Path)
}
