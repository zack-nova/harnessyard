package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestComposeInstallScopedGuidanceRollsBackArtifactsAndReturnsWarning(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "workspace guidance\n")

	outcome := composeInstallScopedGuidance(context.Background(), repo.Root, []string{"docs"}, false, func(_ context.Context, input harnesspkg.ComposeRuntimeGuidanceInput) (harnesspkg.ComposeRuntimeGuidanceResult, error) {
		require.Equal(t, repo.Root, input.RepoRoot)
		require.Equal(t, harnesspkg.GuidanceTargetAll, input.Target)
		require.Equal(t, []string{"docs"}, input.OrbitIDs)
		require.False(t, input.Force)

		require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "AGENTS.md"), []byte("mutated guidance\n"), 0o600))
		return harnesspkg.ComposeRuntimeGuidanceResult{}, fmt.Errorf("compose failed")
	})

	require.Empty(t, outcome.WrittenPaths)
	require.Len(t, outcome.Warnings, 1)
	require.Contains(t, outcome.Warnings[0], "compose failed")
	require.Contains(t, outcome.Warnings[0], "hyard guide sync --target all")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, "workspace guidance\n", string(agentsData))
}
