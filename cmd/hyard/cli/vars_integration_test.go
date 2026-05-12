package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHyardVarsPathReportsCanonicalRuntimeBindingsPath(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "path")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ".harness/vars.yaml\n", stdout)
}

func TestHyardVarsValidateAcceptsSchema2RuntimeBindings(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Harness Yard\n"+
		"  github_token:\n"+
		"    value_from:\n"+
		"      env: GITHUB_TOKEN\n"+
		"scoped_variables:\n"+
		"  docs:\n"+
		"    variables:\n"+
		"      issue_payload:\n"+
		"        value_from:\n"+
		"          file: .harness/context/issue.json\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "validated Runtime Bindings file: .harness/vars.yaml\n", stdout)
}

func TestHyardVarsValidateReportsActionableRuntimeBindingsErrors(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Harness Yard\n"+
		"    value_from:\n"+
		"      env: PROJECT_NAME\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "validate")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, ".harness/vars.yaml")
	require.ErrorContains(t, err, "variables.project_name must set exactly one of value or value_from")
}
