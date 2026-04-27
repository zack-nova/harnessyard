package bindings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVarsDataAllowsScalarShorthandBindings(t *testing.T) {
	t.Parallel()

	file, err := ParseVarsData([]byte("" +
		"schema_version: 1\n" +
		"variables:\n" +
		"  github_token: '${{ secrets.GITHUB_TOKEN }}'\n" +
		"scoped_variables:\n" +
		"  docs:\n" +
		"    variables:\n" +
		"      project_name: Docs Orbit\n"))
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 1,
		Variables: map[string]VariableBinding{
			"github_token": {
				Value: "${{ secrets.GITHUB_TOKEN }}",
			},
		},
		ScopedVariables: map[string]ScopedVariableBindings{
			"docs": {
				Variables: map[string]VariableBinding{
					"project_name": {
						Value: "Docs Orbit",
					},
				},
			},
		},
	}, file)

	data, err := MarshalVarsFile(file)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"    github_token:\n"+
		"        value: ${{ secrets.GITHUB_TOKEN }}\n"+
		"scoped_variables:\n"+
		"    docs:\n"+
		"        variables:\n"+
		"            project_name:\n"+
		"                value: Docs Orbit\n", string(data))
}

func TestParseVarsDataSuggestsHowToFixInlineGitHubActionsExpressions(t *testing.T) {
	t.Parallel()

	_, err := ParseVarsData([]byte("" +
		"schema_version: 1\n" +
		"variables: {\n" +
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n" +
		"}\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.ErrorContains(t, err, "github_token:")
	require.ErrorContains(t, err, "value:")
}

func TestWriteAndLoadVarsFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := VarsFile{
		SchemaVersion: 1,
		Variables: map[string]VariableBinding{
			"service_url": {
				Value:       "http://localhost:3000",
				Description: "Service URL",
			},
			"project_name": {
				Value:       "Orbit",
				Description: "Project name",
			},
			"empty_description": {
				Value: "",
			},
		},
		ScopedVariables: map[string]ScopedVariableBindings{
			"docs": {
				Variables: map[string]VariableBinding{
					"project_name": {
						Value:       "Docs Orbit",
						Description: "Docs title",
					},
				},
			},
		},
	}

	filename, err := WriteVarsFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".orbit", "vars.yaml"), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"    empty_description:\n"+
		"        value: \"\"\n"+
		"    project_name:\n"+
		"        value: Orbit\n"+
		"        description: Project name\n"+
		"    service_url:\n"+
		"        value: http://localhost:3000\n"+
		"        description: Service URL\n"+
		"scoped_variables:\n"+
		"    docs:\n"+
		"        variables:\n"+
		"            project_name:\n"+
		"                value: Docs Orbit\n"+
		"                description: Docs title\n", string(data))

	loaded, err := LoadVarsFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadVarsFileRejectsMissingValueField(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    description: project title\n"), 0o600))

	_, err := LoadVarsFile(repoRoot)
	require.Error(t, err)
	require.ErrorContains(t, err, "variables.project_name.value")
}

func TestLoadVarsFileAllowsEmptyValueString(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: \"\"\n"), 0o600))

	loaded, err := LoadVarsFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 1,
		Variables: map[string]VariableBinding{
			"project_name": {
				Value: "",
			},
		},
	}, loaded)
}

func TestLoadVarsFileAllowsEmptyVariablesMap(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"variables: {}\n"), 0o600))

	loaded, err := LoadVarsFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]VariableBinding{},
	}, loaded)
}

func TestValidateVarsFileRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    VarsFile
		contains string
	}{
		{
			name: "schema version must be frozen",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"project_name": {Value: "Orbit"},
				},
			},
			contains: "schema_version must be 1",
		},
		{
			name: "variables field must be present",
			input: VarsFile{
				SchemaVersion: 1,
			},
			contains: "variables must be present",
		},
		{
			name: "variable names must stay template-safe",
			input: VarsFile{
				SchemaVersion: 1,
				Variables: map[string]VariableBinding{
					"bad-name": {Value: "Orbit"},
				},
			},
			contains: "variables.bad-name",
		},
		{
			name: "scoped namespace must be orbit id safe",
			input: VarsFile{
				SchemaVersion: 1,
				Variables:     map[string]VariableBinding{},
				ScopedVariables: map[string]ScopedVariableBindings{
					"Bad Docs": {
						Variables: map[string]VariableBinding{
							"project_name": {Value: "Orbit"},
						},
					},
				},
			},
			contains: "scoped_variables.Bad Docs",
		},
		{
			name: "scoped variables field must be present",
			input: VarsFile{
				SchemaVersion: 1,
				Variables:     map[string]VariableBinding{},
				ScopedVariables: map[string]ScopedVariableBindings{
					"docs": {},
				},
			},
			contains: "scoped_variables.docs.variables must be present",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateVarsFile(testCase.input)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}
