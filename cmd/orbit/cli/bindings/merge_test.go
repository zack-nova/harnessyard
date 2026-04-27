package bindings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergePrefersBindingsFileThenRepoVarsThenInteractive(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"owner_name": {
				Description: "Owner name",
				Required:    true,
			},
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
			"service_url": {
				Description: "Service URL",
				Required:    true,
			},
			"support_email": {
				Description: "Support email",
				Required:    false,
			},
		},
		BindingsFile: map[string]VariableBinding{
			"project_name": {
				Value:       "From File",
				Description: "file description should not override declaration",
			},
		},
		RepoVars: map[string]VariableBinding{
			"project_name": {
				Value: "From Repo",
			},
			"service_url": {
				Value: "https://repo.example.com",
			},
			"support_email": {
				Value: "support@example.com",
			},
		},
		FillIn: map[string]VariableBinding{
			"owner_name": {
				Value: "Yixin",
			},
			"service_url": {
				Value: "https://interactive.example.com",
			},
		},
		FillSource: SourceInteractive,
	})
	require.NoError(t, err)

	require.Equal(t, map[string]ResolvedBinding{
		"owner_name": {
			Value:       "Yixin",
			Description: "Owner name",
			Required:    true,
			Source:      SourceInteractive,
		},
		"project_name": {
			Value:       "From File",
			Description: "Project name",
			Required:    true,
			Source:      SourceBindingsFile,
		},
		"service_url": {
			Value:       "https://repo.example.com",
			Description: "Service URL",
			Required:    true,
			Source:      SourceRepoVars,
		},
		"support_email": {
			Value:       "support@example.com",
			Description: "Support email",
			Required:    false,
			Source:      SourceRepoVars,
		},
	}, result.Resolved)
	require.Empty(t, result.Unresolved)
}

func TestMergeReturnsStableUnresolvedForMissingRequiredVariablesOnly(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"api_key": {
				Description: "API key",
				Required:    true,
			},
			"optional_note": {
				Description: "Additional notes",
				Required:    false,
			},
			"zone": {
				Description: "Availability zone",
				Required:    true,
			},
		},
	})
	require.NoError(t, err)

	require.Empty(t, result.Resolved)
	require.Equal(t, []UnresolvedBinding{
		{
			Name:        "api_key",
			Description: "API key",
			Required:    true,
		},
		{
			Name:        "zone",
			Description: "Availability zone",
			Required:    true,
		},
	}, result.Unresolved)
}

func TestMergeTreatsBlankValuesAsMissing(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
			"optional_note": {
				Description: "Additional notes",
				Required:    false,
			},
		},
		BindingsFile: map[string]VariableBinding{
			"project_name": {
				Value:       "  \t\n",
				Description: "blank file value",
			},
			"optional_note": {
				Value:       "",
				Description: "blank optional value",
			},
		},
		RepoVars: map[string]VariableBinding{
			"project_name": {
				Value:       "From Repo",
				Description: "repo title",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, map[string]ResolvedBinding{
		"project_name": {
			Value:       "From Repo",
			Description: "Project name",
			Required:    true,
			Source:      SourceRepoVars,
		},
	}, result.Resolved)
	require.Empty(t, result.Unresolved)
}

func TestMergePrefersScopedBindingsAndRecordsNamespaces(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"project_name": {
				Description: "Project title",
				Required:    true,
			},
			"service_url": {
				Description: "Service URL",
				Required:    true,
			},
			"team_name": {
				Description: "Team name",
				Required:    true,
			},
		},
		BindingsFile: map[string]VariableBinding{
			"project_name": {
				Value:       "Global File",
				Description: "global file title",
			},
		},
		BindingsFileScoped: map[string]VariableBinding{
			"project_name": {
				Value:       "Docs File",
				Description: "scoped file title",
			},
		},
		RepoVars: map[string]VariableBinding{
			"service_url": {
				Value:       "https://global.example.com",
				Description: "global service",
			},
			"team_name": {
				Value: "Platform",
			},
		},
		RepoVarsScoped: map[string]VariableBinding{
			"service_url": {
				Value:       "https://docs.example.com",
				Description: "scoped service",
			},
		},
		Namespace: "docs",
		NamespaceByVariable: map[string]string{
			"team_name": "docs",
		},
	})
	require.NoError(t, err)

	require.Equal(t, map[string]ResolvedBinding{
		"project_name": {
			Value:       "Docs File",
			Description: "Project title",
			Required:    true,
			Source:      SourceBindingsFileScoped,
			Namespace:   "docs",
		},
		"service_url": {
			Value:       "https://docs.example.com",
			Description: "Service URL",
			Required:    true,
			Source:      SourceRepoVarsScoped,
			Namespace:   "docs",
		},
		"team_name": {
			Value:       "Platform",
			Description: "Team name",
			Required:    true,
			Source:      SourceRepoVars,
			Namespace:   "docs",
		},
	}, result.Resolved)
	require.Empty(t, result.Unresolved)
}

func TestMergePreservesNamespaceForUnresolvedRequiredVariables(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"project_name": {
				Description: "Project title",
				Required:    true,
			},
		},
		NamespaceByVariable: map[string]string{
			"project_name": "docs",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []UnresolvedBinding{
		{
			Name:        "project_name",
			Description: "Project title",
			Required:    true,
			Namespace:   "docs",
		},
	}, result.Unresolved)
}

func TestMergeIgnoresUndeclaredInputs(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
		},
		BindingsFile: map[string]VariableBinding{
			"project_name": {Value: "Orbit"},
			"ignored_file": {Value: "ignored"},
		},
		RepoVars: map[string]VariableBinding{
			"ignored_repo": {Value: "ignored"},
		},
		FillIn: map[string]VariableBinding{
			"ignored_fill": {Value: "ignored"},
		},
		FillSource: SourceEditor,
	})
	require.NoError(t, err)

	require.Equal(t, map[string]ResolvedBinding{
		"project_name": {
			Value:       "Orbit",
			Description: "Project name",
			Required:    true,
			Source:      SourceBindingsFile,
		},
	}, result.Resolved)
	require.Empty(t, result.Unresolved)
}

func TestMergePreservesDescriptionFromDeclarationOrFallbackSource(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"owner_name": {
				Required: true,
			},
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
		},
		BindingsFile: map[string]VariableBinding{
			"project_name": {
				Value:       "Orbit",
				Description: "should not replace declared description",
			},
		},
		RepoVars: map[string]VariableBinding{
			"owner_name": {
				Value:       "Miles",
				Description: "Owner name",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, "Project name", result.Resolved["project_name"].Description)
	require.Equal(t, "Owner name", result.Resolved["owner_name"].Description)
}

func TestMergeRejectsUnknownFillSource(t *testing.T) {
	t.Parallel()

	_, err := Merge(MergeInput{
		Declared: map[string]VariableDeclaration{
			"project_name": {
				Required: true,
			},
		},
		FillIn: map[string]VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
		FillSource: MergeSource("clipboard"),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "fill source")
}

func TestMergeAllowsEmptyDeclaredVariables(t *testing.T) {
	t.Parallel()

	result, err := Merge(MergeInput{})
	require.NoError(t, err)
	require.Empty(t, result.Resolved)
	require.Empty(t, result.Unresolved)
}
