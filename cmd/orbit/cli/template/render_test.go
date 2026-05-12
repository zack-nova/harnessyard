package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

func TestRenderTemplateFilesRendersStrictVarsReferencesInTextFiles(t *testing.T) {
	t.Parallel()

	rendered, err := renderTemplateFilesWithOptions([]CandidateFile{
		{
			Path:    "config/app.txt",
			Content: []byte("Project={{ vars.project_name }}\n"),
		},
	}, map[string]string{
		"project_name": "Harness Yard",
	}, renderTemplateOptions{
		Declared: map[string]bindings.VariableDeclaration{
			"project_name": {
				Required: true,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []byte("Project=Harness Yard\n"), rendered[0].Content)
}

func TestRenderTemplateFilesRejectsUnsupportedNamespace(t *testing.T) {
	t.Parallel()

	_, err := renderTemplateFilesWithOptions([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Token {{ secrets.GITHUB_TOKEN }}\n"),
		},
	}, map[string]string{}, renderTemplateOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "docs/guide.md")
	require.ErrorContains(t, err, `unsupported Package Template Reference namespace "secrets"`)
}

func TestRenderTemplateFilesRejectsUnknownVarsReference(t *testing.T) {
	t.Parallel()

	_, err := renderTemplateFilesWithOptions([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Project {{ vars.missing }}\n"),
		},
	}, map[string]string{}, renderTemplateOptions{
		Declared: map[string]bindings.VariableDeclaration{
			"project_name": {
				Required: true,
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "docs/guide.md")
	require.ErrorContains(t, err, `unknown Package Variable "missing"`)
}

func TestRenderTemplateFilesRejectsUnresolvedDeclaredReference(t *testing.T) {
	t.Parallel()

	_, err := renderTemplateFilesWithOptions([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Project {{ vars.project_name }}\n"),
		},
	}, map[string]string{}, renderTemplateOptions{
		Declared: map[string]bindings.VariableDeclaration{
			"project_name": {
				Required: true,
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "docs/guide.md")
	require.ErrorContains(t, err, `unresolved Package Variable "project_name"`)
}

func TestRenderTemplateFilesRejectsMalformedReferenceWithPath(t *testing.T) {
	t.Parallel()

	_, err := RenderTemplateFiles([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Project {{ vars. }}\n"),
		},
	}, map[string]string{})
	require.Error(t, err)
	require.ErrorContains(t, err, "docs/guide.md")
	require.ErrorContains(t, err, "malformed Package Template Reference")
}

func TestRenderTemplateFilesPreservesGitHubActionsExpressions(t *testing.T) {
	t.Parallel()

	rendered, err := RenderTemplateFiles([]CandidateFile{
		{
			Path:    ".github/workflows/ci.yml",
			Content: []byte("token: ${{ secrets.GITHUB_TOKEN }}\n"),
		},
	}, map[string]string{})
	require.NoError(t, err)
	require.Equal(t, []byte("token: ${{ secrets.GITHUB_TOKEN }}\n"), rendered[0].Content)
}

func TestRenderTemplateFilesLeavesLegacyDollarReferencesUnchanged(t *testing.T) {
	t.Parallel()

	rendered, err := RenderTemplateFiles([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
		},
	}, map[string]string{
		"project_name": "Harness Yard",
	})
	require.NoError(t, err)
	require.Equal(t, []byte("$project_name guide\n"), rendered[0].Content)
}
