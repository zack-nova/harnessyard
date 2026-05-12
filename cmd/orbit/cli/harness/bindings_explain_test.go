package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestExplainRuntimeBindingReportsSelectedResolutionAndDeclarations(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.May, 12, 11, 20, 0, 0, time.UTC)
	seedRuntimeBindingExplainRepo(t, repoRoot, now, map[string]bindings.VariableDeclaration{
		"project_name": {
			Description: "Project name",
			Required:    true,
		},
	})
	_, err := WriteVarsFile(repoRoot, bindings.VarsFile{
		SchemaVersion: bindings.VarsSchemaVersion,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {Value: "Global Project"},
		},
		ScopedVariables: map[string]bindings.ScopedVariableBindings{
			"docs": {
				Variables: map[string]bindings.VariableBinding{
					"project_name": {Value: "Docs Project"},
				},
			},
		},
	})
	require.NoError(t, err)

	result, err := ExplainRuntimeBinding(context.Background(), RuntimeBindingExplainInput{
		RepoRoot: repoRoot,
		Name:     "project_name",
	})
	require.NoError(t, err)

	require.Equal(t, RuntimeBindingExplainStatusResolved, result.Status)
	require.Equal(t, "project_name", result.Name)
	require.Equal(t, ".harness/vars.yaml", result.ValueSource)
	require.Equal(t, "Docs Project", result.Value)
	require.True(t, result.Required)
	require.False(t, result.Sensitive)
	require.Equal(t, "docs", result.SelectedScope)
	require.Equal(t, []string{"docs"}, result.DeclaringOrbits)
}

func TestExplainRuntimeBindingRedactsSensitiveResolvedValues(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.May, 12, 11, 25, 0, 0, time.UTC)
	seedRuntimeBindingExplainRepo(t, repoRoot, now, map[string]bindings.VariableDeclaration{
		"github_token": {
			Description: "GitHub token",
			Required:    true,
			Sensitive:   true,
		},
	})
	_, err := WriteVarsFile(repoRoot, bindings.VarsFile{
		SchemaVersion: bindings.VarsSchemaVersion,
		Variables: map[string]bindings.VariableBinding{
			"github_token": {ValueFrom: &bindings.ValueSource{Env: "GITHUB_TOKEN"}},
		},
	})
	require.NoError(t, err)

	result, err := ExplainRuntimeBinding(context.Background(), RuntimeBindingExplainInput{
		RepoRoot: repoRoot,
		Name:     "github_token",
		LookupEnv: func(name string) (string, bool) {
			require.Equal(t, "GITHUB_TOKEN", name)
			return "ghp_secret", true
		},
	})
	require.NoError(t, err)

	require.Equal(t, RuntimeBindingExplainStatusResolved, result.Status)
	require.Equal(t, "env:GITHUB_TOKEN", result.ValueSource)
	require.Equal(t, "<redacted>", result.Value)
	require.True(t, result.Required)
	require.True(t, result.Sensitive)
	require.Equal(t, "global", result.SelectedScope)
}

func seedRuntimeBindingExplainRepo(
	t *testing.T,
	repoRoot string,
	now time.Time,
	declarations map[string]bindings.VariableDeclaration,
) {
	t.Helper()

	_, err := WriteRuntimeFile(repoRoot, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: now},
		},
	})
	require.NoError(t, err)
	_, err = WriteInstallRecord(repoRoot, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations:    declarations,
			ResolvedAtApply: map[string]bindings.VariableBinding{},
		},
	})
	require.NoError(t, err)
}
