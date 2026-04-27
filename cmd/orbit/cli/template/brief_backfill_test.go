package orbittemplate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestReverseVariableizeBriefReplacesUniqueRuntimeValues(t *testing.T) {
	t.Parallel()

	result, err := ReverseVariableizeBrief([]byte("Welcome to Acme.\nOpen https://docs.acme.test.\n"), map[string]bindings.VariableBinding{
		"project_name": {
			Value: "Acme",
		},
		"docs_url": {
			Value: "https://docs.acme.test",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Welcome to $project_name.\nOpen $docs_url.\n", string(result.Content))
	require.ElementsMatch(t, []ReplacementSummary{
		{
			Variable: "docs_url",
			Literal:  "https://docs.acme.test",
			Count:    1,
		},
		{
			Variable: "project_name",
			Literal:  "Acme",
			Count:    1,
		},
	}, result.Replacements)
}

func TestReverseVariableizeBriefRejectsDuplicateLiteralBindings(t *testing.T) {
	t.Parallel()

	_, err := ReverseVariableizeBrief([]byte("Welcome to Acme.\n"), map[string]bindings.VariableBinding{
		"project_name": {
			Value: "Acme",
		},
		"product_name": {
			Value: "Acme",
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "reverse replacement is ambiguous")
	require.ErrorContains(t, err, "Acme")
	require.ErrorContains(t, err, "product_name")
	require.ErrorContains(t, err, "project_name")
}

func TestReverseVariableizeBriefRejectsOverlappingLiteralMatches(t *testing.T) {
	t.Parallel()

	_, err := ReverseVariableizeBrief([]byte("Follow the Acme Docs workflow.\n"), map[string]bindings.VariableBinding{
		"project_name": {
			Value: "Acme",
		},
		"workflow_name": {
			Value: "Acme Docs",
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "reverse replacement is ambiguous")
	require.ErrorContains(t, err, "Acme")
	require.ErrorContains(t, err, "Acme Docs")
}

func TestBackfillOrbitBriefPreservesHostedDefinitionComments(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"# keep hosted header\n"+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  # keep meta comment\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"# keep member comment\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	agentsData, err := WrapRuntimeAgentsBlock("docs", []byte("Docs orbit for Acme\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))

	result, err := BackfillOrbitBrief(context.Background(), BriefBackfillInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), result.DefinitionPath)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "# keep hosted header")
	require.Contains(t, string(data), "# keep meta comment")
	require.Contains(t, string(data), "# keep member comment")
	require.Contains(t, string(data), "agents_template: |")
	require.Contains(t, string(data), "Docs orbit for $project_name\n")
}

func TestBackfillOrbitBriefReturnsSkippedWhenHostedTemplateAlreadyMatches(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
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
	agentsData, err := WrapRuntimeAgentsBlock("docs", []byte("Docs orbit for Acme\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))

	result, err := BackfillOrbitBrief(context.Background(), BriefBackfillInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.NoError(t, err)
	require.Equal(t, GuidanceBackfillStatusSkipped, result.Status)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "agents_template: |")
	require.Contains(t, string(data), "Docs orbit for $project_name\n")
}

func TestBackfillOrbitBriefFailsClosedWhenHostedAgentsTemplateUsesAlias(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: &brief Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: *brief\n"+
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
	agentsData, err := WrapRuntimeAgentsBlock("docs", []byte("Docs orbit guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsData))

	_, err = BackfillOrbitBrief(context.Background(), BriefBackfillInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot safely preserve")
	require.ErrorContains(t, err, "agents_template")

	data, readErr := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, readErr)
	require.Contains(t, string(data), "agents_template: *brief")
}
