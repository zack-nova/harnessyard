package harness

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestApplyBundleAgentsPayloadAppendsAndReplacesBundleBlock(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"General runtime guidance\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs guidance\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed runtime agents")

	err := ApplyBundleAgentsPayload(repo.Root, "workspace", []byte("Bundle guidance v1\n"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), `<!-- orbit:begin orbit_id="docs" -->`)
	require.Contains(t, string(data), `<!-- orbit:begin orbit_id="workspace" -->`)
	require.Contains(t, string(data), "Bundle guidance v1\n")

	err = ApplyBundleAgentsPayload(repo.Root, "workspace", []byte("Bundle guidance v2\n"))
	require.NoError(t, err)

	data, err = os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "Bundle guidance v1\n")
	require.Contains(t, string(data), "Bundle guidance v2\n")
	require.Len(t, regexpMustCompile(`orbit_id="workspace"`).FindAll(data, -1), 2)
}

func TestRemoveBundleAgentsPayloadRemovesOnlyTargetBlock(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"General runtime guidance\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs guidance\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n"+
		"<!-- orbit:begin orbit_id=\"workspace\" -->\n"+
		"Bundle guidance\n"+
		"<!-- orbit:end orbit_id=\"workspace\" -->\n")
	repo.AddAndCommit(t, "seed runtime agents with bundle block")

	err := RemoveBundleAgentsPayload(repo.Root, "workspace")
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), `<!-- orbit:begin orbit_id="docs" -->`)
	require.NotContains(t, string(data), `<!-- orbit:begin orbit_id="workspace" -->`)
	require.NotContains(t, string(data), "Bundle guidance\n")
}

func regexpMustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
