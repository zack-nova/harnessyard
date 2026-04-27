package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessTemplateSavePlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"template",
		"save",
		"--to",
		"harness-template/workspace",
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: building template preview\n")
	require.Contains(t, stderr, "progress: writing template branch\n")
	require.Contains(t, stderr, "progress: template save complete\n")

	var payload struct {
		HarnessID string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotEmpty(t, payload.HarnessID)
}

func TestHarnessBindingsApplyPlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedInstalledHarnessProgressRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Filled Orbit\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: replaying install source\n")
	require.Contains(t, stderr, "progress: analyzing drift\n")
	require.Contains(t, stderr, "progress: writing install-owned files\n")
	require.Contains(t, stderr, "progress: bindings apply complete\n")

	var payload struct {
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
}

func TestHarnessCheckPlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedInstalledHarnessProgressRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--progress", "plain", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: scanning harness records\n")
	require.Contains(t, stderr, "progress: checking install-backed members\n")
	require.Contains(t, stderr, "progress: check complete\n")

	var payload struct {
		HarnessID string `json:"harness_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotEmpty(t, payload.HarnessID)
}

func TestHarnessBindingsPlanPlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBindingsPlanRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "Orbit guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run Orbit as `orbit`.\n",
			},
		},
	}, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	outputPath := filepath.Join(repo.Root, "bindings-plan.yaml")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"plan",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--out",
		outputPath,
		"--progress",
		"plain",
		"--json",
	)
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: preflighting source 1/2\n")
	require.Contains(t, stderr, "progress: preflighting source 2/2\n")
	require.Contains(t, stderr, "progress: merging bindings plan\n")
	require.Contains(t, stderr, "progress: writing bindings plan\n")
	require.Contains(t, stderr, "progress: bindings plan complete\n")

	var payload struct {
		SourceCount int    `json:"source_count"`
		OutputPath  string `json:"output_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 2, payload.SourceCount)
	require.Equal(t, outputPath, payload.OutputPath)

	_, err = os.Stat(outputPath)
	require.NoError(t, err)
}

func seedInstalledHarnessProgressRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	return repo
}
