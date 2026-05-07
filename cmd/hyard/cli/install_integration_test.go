package cli_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

func TestHyardInstallOverwriteDryRunReplaysLegacyZeroVariableInstall(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	_, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	record.Variables = nil
	_, err = harnesspkg.WriteInstallRecord(repo.Root, record)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--overwrite-existing",
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun            bool   `json:"dry_run"`
		OrbitID           string `json:"orbit_id"`
		OverwriteExisting bool   `json:"overwrite_existing"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.True(t, payload.OverwriteExisting)
	require.Equal(t, "docs", payload.OrbitID)
}
