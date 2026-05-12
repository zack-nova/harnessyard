package bindings

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRuntimeBindingPrecedence(t *testing.T) {
	t.Parallel()

	defaultValue := "Declared Default"
	varsFile := VarsFile{
		SchemaVersion: VarsSchemaVersion,
		Variables: map[string]VariableBinding{
			"project_name": {Value: "Global Value"},
		},
		ScopedVariables: map[string]ScopedVariableBindings{
			"docs": {
				Variables: map[string]VariableBinding{
					"project_name": {Value: "Scoped Value"},
				},
			},
		},
	}

	scoped, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name:          "project_name",
		SelectedScope: "docs",
		Declaration: VariableDeclaration{
			Description: "Project title",
			Required:    true,
			Default:     &defaultValue,
		},
		VarsFile: varsFile,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeBindingResolution{
		Name:          "project_name",
		Value:         "Scoped Value",
		ValueSource:   ".harness/vars.yaml",
		Source:        RuntimeBindingSourceScoped,
		SelectedScope: "docs",
		Required:      true,
		Resolved:      true,
	}, scoped)

	globalVars := varsFile
	delete(globalVars.ScopedVariables["docs"].Variables, "project_name")
	global, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name:          "project_name",
		SelectedScope: "docs",
		Declaration: VariableDeclaration{
			Required: true,
			Default:  &defaultValue,
		},
		VarsFile: globalVars,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeBindingSourceGlobal, global.Source)
	require.Equal(t, "global", global.SelectedScope)
	require.Equal(t, "Global Value", global.Value)

	defaultVars := globalVars
	delete(defaultVars.Variables, "project_name")
	fallback, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "project_name",
		Declaration: VariableDeclaration{
			Required: true,
			Default:  &defaultValue,
		},
		VarsFile: defaultVars,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeBindingSourceDefault, fallback.Source)
	require.Equal(t, "default", fallback.ValueSource)
	require.Equal(t, "Declared Default", fallback.Value)

	unresolved, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "project_name",
		Declaration: VariableDeclaration{
			Required: true,
		},
		VarsFile: defaultVars,
	})
	require.NoError(t, err)
	require.False(t, unresolved.Resolved)
	require.Equal(t, RuntimeBindingSourceUnresolved, unresolved.Source)
}

func TestResolveRuntimeBindingSelectsEmptyInlineValue(t *testing.T) {
	t.Parallel()

	defaultValue := "Declared Default"
	resolution, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "project_name",
		Declaration: VariableDeclaration{
			Required: true,
			Default:  &defaultValue,
		},
		VarsFile: VarsFile{
			SchemaVersion: VarsSchemaVersion,
			Variables: map[string]VariableBinding{
				"project_name": {Value: ""},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, resolution.Resolved)
	require.Equal(t, RuntimeBindingSourceGlobal, resolution.Source)
	require.Equal(t, ".harness/vars.yaml", resolution.ValueSource)
	require.Empty(t, resolution.Value)
}

func TestResolveRuntimeBindingValueSources(t *testing.T) {
	t.Parallel()

	envResolved, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "project_name",
		Declaration: VariableDeclaration{
			Required: true,
		},
		VarsFile: VarsFile{
			SchemaVersion: VarsSchemaVersion,
			Variables: map[string]VariableBinding{
				"project_name": {
					ValueFrom: &ValueSource{Env: "PROJECT_NAME"},
				},
			},
		},
		LookupEnv: func(name string) (string, bool) {
			require.Equal(t, "PROJECT_NAME", name)
			return "From Env", true
		},
	})
	require.NoError(t, err)
	require.True(t, envResolved.Resolved)
	require.Equal(t, "From Env", envResolved.Value)
	require.Equal(t, "env:PROJECT_NAME", envResolved.ValueSource)

	fileResolved, err := ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "issue_payload",
		VarsFile: VarsFile{
			SchemaVersion: VarsSchemaVersion,
			Variables: map[string]VariableBinding{
				"issue_payload": {
					ValueFrom: &ValueSource{File: ".harness/context/issue.json"},
				},
			},
		},
		ReadFile: func(path string) ([]byte, error) {
			require.Equal(t, ".harness/context/issue.json", path)
			return []byte(`{"number":138}`), nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, `{"number":138}`, fileResolved.Value)
	require.Equal(t, "file:.harness/context/issue.json", fileResolved.ValueSource)

	_, err = ResolveRuntimeBinding(RuntimeBindingInput{
		Name: "github_token",
		Declaration: VariableDeclaration{
			Required:  true,
			Sensitive: true,
		},
		VarsFile: VarsFile{
			SchemaVersion: VarsSchemaVersion,
			Variables: map[string]VariableBinding{
				"github_token": {
					ValueFrom: &ValueSource{File: ".secrets/token"},
				},
			},
		},
		ReadFile: func(string) ([]byte, error) {
			return nil, fmt.Errorf("must not read sensitive file")
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "sensitive")
	require.ErrorContains(t, err, "value_from.file")
}
