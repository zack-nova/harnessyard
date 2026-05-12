package bindings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVarsDataAcceptsSchema2Bindings(t *testing.T) {
	t.Parallel()

	file, err := ParseVarsData([]byte("" +
		"schema_version: 2\n" +
		"variables:\n" +
		"  project_name:\n" +
		"    value: Harness Yard\n" +
		"  github_token:\n" +
		"    value_from:\n" +
		"      env: GITHUB_TOKEN\n" +
		"  issue_payload:\n" +
		"    value_from:\n" +
		"      file: .harness/context/issue.json\n" +
		"scoped_variables:\n" +
		"  docs:\n" +
		"    variables:\n" +
		"      project_name:\n" +
		"        value: Harness Yard Docs\n"))
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 2,
		Variables: map[string]VariableBinding{
			"github_token": {
				ValueFrom: &ValueSource{Env: "GITHUB_TOKEN"},
			},
			"issue_payload": {
				ValueFrom: &ValueSource{File: ".harness/context/issue.json"},
			},
			"project_name": {
				Value: "Harness Yard",
			},
		},
		ScopedVariables: map[string]ScopedVariableBindings{
			"docs": {
				Variables: map[string]VariableBinding{
					"project_name": {
						Value: "Harness Yard Docs",
					},
				},
			},
		},
	}, file)

	data, err := MarshalVarsFile(file)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"    github_token:\n"+
		"        value_from:\n"+
		"            env: GITHUB_TOKEN\n"+
		"    issue_payload:\n"+
		"        value_from:\n"+
		"            file: .harness/context/issue.json\n"+
		"    project_name:\n"+
		"        value: Harness Yard\n"+
		"scoped_variables:\n"+
		"    docs:\n"+
		"        variables:\n"+
		"            project_name:\n"+
		"                value: Harness Yard Docs\n", string(data))
}

func TestParseVarsDataSuggestsHowToFixInlineGitHubActionsExpressions(t *testing.T) {
	t.Parallel()

	_, err := ParseVarsData([]byte("" +
		"schema_version: 2\n" +
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
		SchemaVersion: 2,
		Variables: map[string]VariableBinding{
			"service_url": {
				Value:       "http://localhost:3000",
				Description: "Service URL",
			},
			"github_token": {
				ValueFrom:   &ValueSource{Env: "GITHUB_TOKEN"},
				Description: "GitHub token",
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
					"config_path": {
						ValueFrom: &ValueSource{File: ".harness/docs/config.json"},
					},
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
	require.Equal(t, filepath.Join(repoRoot, ".harness", "vars.yaml"), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"    empty_description:\n"+
		"        value: \"\"\n"+
		"    github_token:\n"+
		"        value_from:\n"+
		"            env: GITHUB_TOKEN\n"+
		"        description: GitHub token\n"+
		"    project_name:\n"+
		"        value: Orbit\n"+
		"        description: Project name\n"+
		"    service_url:\n"+
		"        value: http://localhost:3000\n"+
		"        description: Service URL\n"+
		"scoped_variables:\n"+
		"    docs:\n"+
		"        variables:\n"+
		"            config_path:\n"+
		"                value_from:\n"+
		"                    file: .harness/docs/config.json\n"+
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
	filename := filepath.Join(repoRoot, ".harness", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    description: project title\n"), 0o600))

	_, err := LoadVarsFile(repoRoot)
	require.Error(t, err)
	require.ErrorContains(t, err, "variables.project_name must set exactly one of value or value_from")
}

func TestLoadVarsFileAllowsEmptyValueString(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".harness", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: \"\"\n"), 0o600))

	loaded, err := LoadVarsFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 2,
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
	filename := filepath.Join(repoRoot, ".harness", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 2\n"+
		"variables: {}\n"), 0o600))

	loaded, err := LoadVarsFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, VarsFile{
		SchemaVersion: 2,
		Variables:     map[string]VariableBinding{},
	}, loaded)
}

func TestParseVarsDataRejectsInvalidSchema2Shapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name: "schema one",
			input: "" +
				"schema_version: 1\n" +
				"variables: {}\n",
			contains: "schema_version must be 2",
		},
		{
			name: "scalar shorthand",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  project_name: Harness Yard\n",
			contains: "variables.project_name must be a mapping with value or value_from",
		},
		{
			name: "invalid variable name",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  bad-name:\n" +
				"    value: Harness Yard\n",
			contains: "variables.bad-name",
		},
		{
			name: "invalid scoped namespace",
			input: "" +
				"schema_version: 2\n" +
				"variables: {}\n" +
				"scoped_variables:\n" +
				"  Bad Docs:\n" +
				"    variables:\n" +
				"      project_name:\n" +
				"        value: Harness Yard\n",
			contains: "scoped_variables.Bad Docs",
		},
		{
			name: "value and value_from",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Harness Yard\n" +
				"    value_from:\n" +
				"      env: PROJECT_NAME\n",
			contains: "variables.project_name must set exactly one of value or value_from",
		},
		{
			name: "neither value nor value_from",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    description: Project title\n",
			contains: "variables.project_name must set exactly one of value or value_from",
		},
		{
			name: "blank env value source",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  github_token:\n" +
				"    value_from:\n" +
				"      env: \"  \"\n",
			contains: "variables.github_token.value_from.env must not be blank",
		},
		{
			name: "blank file value source",
			input: "" +
				"schema_version: 2\n" +
				"variables:\n" +
				"  issue_payload:\n" +
				"    value_from:\n" +
				"      file: \"  \"\n",
			contains: "variables.issue_payload.value_from.file must not be blank",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseVarsData([]byte(testCase.input))
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
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
				SchemaVersion: 1,
				Variables: map[string]VariableBinding{
					"project_name": {Value: "Orbit"},
				},
			},
			contains: "schema_version must be 2",
		},
		{
			name: "variables field must be present",
			input: VarsFile{
				SchemaVersion: 2,
			},
			contains: "variables must be present",
		},
		{
			name: "variable names must stay template-safe",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"bad-name": {Value: "Orbit"},
				},
			},
			contains: "variables.bad-name",
		},
		{
			name: "scoped namespace must be orbit id safe",
			input: VarsFile{
				SchemaVersion: 2,
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
				SchemaVersion: 2,
				Variables:     map[string]VariableBinding{},
				ScopedVariables: map[string]ScopedVariableBindings{
					"docs": {},
				},
			},
			contains: "scoped_variables.docs.variables must be present",
		},
		{
			name: "binding cannot use value and value_from",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"project_name": {
						Value:     "Orbit",
						ValueFrom: &ValueSource{Env: "PROJECT_NAME"},
					},
				},
			},
			contains: "variables.project_name must set exactly one of value or value_from",
		},
		{
			name: "value_from env must not be blank",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"github_token": {ValueFrom: &ValueSource{Env: "  "}},
				},
			},
			contains: "variables.github_token.value_from.env must not be blank",
		},
		{
			name: "value_from file must not be blank",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"issue_payload": {ValueFrom: &ValueSource{File: "  "}},
				},
			},
			contains: "variables.issue_payload.value_from.file must not be blank",
		},
		{
			name: "value_from must set exactly one source",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"project_name": {ValueFrom: &ValueSource{}},
				},
			},
			contains: "variables.project_name.value_from must set exactly one of env or file",
		},
		{
			name: "value_from cannot set env and file",
			input: VarsFile{
				SchemaVersion: 2,
				Variables: map[string]VariableBinding{
					"project_name": {ValueFrom: &ValueSource{Env: "PROJECT_NAME", File: ".harness/name.txt"}},
				},
			},
			contains: "variables.project_name.value_from must set exactly one of env or file",
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
