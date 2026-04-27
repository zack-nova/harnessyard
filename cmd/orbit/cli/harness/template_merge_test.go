package harness

import (
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestMergeTemplateMemberCandidatesMergesFilesAndVariables(t *testing.T) {
	t.Parallel()

	result, err := MergeTemplateMemberCandidates([]TemplateMemberCandidate{
		{
			OrbitID: "docs",
			Files: []orbittemplate.CandidateFile{
				{
					Path:    ".harness/orbits/docs.yaml",
					Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
					Mode:    gitpkg.FileModeRegular,
				},
				{
					Path:    "README.md",
					Content: []byte("Shared guide\n"),
					Mode:    gitpkg.FileModeRegular,
				},
				{
					Path:    "docs/guide.md",
					Content: []byte("$project_name guide\n"),
					Mode:    gitpkg.FileModeRegular,
				},
			},
			Variables: map[string]TemplateVariableSpec{
				"project_name": {
					Description: "",
					Required:    false,
				},
				"service_url": {
					Description: "Service URL",
					Required:    false,
				},
			},
		},
		{
			OrbitID: "cmd",
			Files: []orbittemplate.CandidateFile{
				{
					Path:    ".harness/orbits/cmd.yaml",
					Content: []byte("id: cmd\ninclude:\n  - cmd/**\n"),
					Mode:    gitpkg.FileModeRegular,
				},
				{
					Path:    "README.md",
					Content: []byte("Shared guide\n"),
					Mode:    gitpkg.FileModeRegular,
				},
				{
					Path:    "cmd/main.go",
					Content: []byte("package main\n"),
					Mode:    gitpkg.FileModeRegular,
				},
			},
			Variables: map[string]TemplateVariableSpec{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
				"service_url": {
					Description: "Service URL",
					Required:    true,
				},
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []TemplateMember{
		{OrbitID: "cmd"},
		{OrbitID: "docs"},
	}, result.Members)
	require.Equal(t, []string{
		".harness/orbits/cmd.yaml",
		".harness/orbits/docs.yaml",
		"README.md",
		"cmd/main.go",
		"docs/guide.md",
	}, result.FilePaths())
	require.Equal(t, map[string]TemplateVariableSpec{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
		"service_url": {
			Description: "Service URL",
			Required:    true,
		},
	}, result.Variables)
}

func TestMergeTemplateMemberCandidatesFailsOnPathCollisionWithDifferentContent(t *testing.T) {
	t.Parallel()

	_, err := MergeTemplateMemberCandidates([]TemplateMemberCandidate{
		{
			OrbitID: "docs",
			Files: []orbittemplate.CandidateFile{
				{
					Path:    "README.md",
					Content: []byte("Docs readme\n"),
					Mode:    gitpkg.FileModeRegular,
				},
			},
		},
		{
			OrbitID: "cmd",
			Files: []orbittemplate.CandidateFile{
				{
					Path:    "README.md",
					Content: []byte("Cmd readme\n"),
					Mode:    gitpkg.FileModeRegular,
				},
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `path conflict for "README.md"`)
	require.ErrorContains(t, err, `members: cmd, docs`)
}

func TestMergeTemplateMemberCandidatesFailsOnVariableDescriptionConflict(t *testing.T) {
	t.Parallel()

	_, err := MergeTemplateMemberCandidates([]TemplateMemberCandidate{
		{
			OrbitID: "docs",
			Variables: map[string]TemplateVariableSpec{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
			},
		},
		{
			OrbitID: "cmd",
			Variables: map[string]TemplateVariableSpec{
				"project_name": {
					Description: "CLI title",
					Required:    false,
				},
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `variable conflict for "project_name"`)
	require.ErrorContains(t, err, `members: cmd, docs`)
}
