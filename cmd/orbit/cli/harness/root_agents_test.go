package harness

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildRootAgentsTemplateFileAppliesWholeFileReplacementAndStripsRuntimeMarkers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  service_url:\n"+
		"    value: https://example.test\n"+
		"    description: Service URL\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Rules for Orbit\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"Use Orbit docs at https://example.test\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n"+
		"<!-- keep this comment -->\n")
	repo.AddAndCommit(t, "seed root AGENTS whole-file content")

	result, err := BuildRootAgentsTemplateFile(context.Background(), repo.Root)
	require.NoError(t, err)
	require.True(t, result.IncludesRootAgents)
	require.Equal(t, &orbittemplate.CandidateFile{
		Path: "AGENTS.md",
		Content: []byte("" +
			"# Rules for $project_name\n" +
			"Use $project_name docs at $service_url\n" +
			"<!-- keep this comment -->\n"),
		Mode: gitpkg.FileModeRegular,
	}, result.File)
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
	require.Equal(t, &orbittemplate.FileReplacementSummary{
		Path: "AGENTS.md",
		Replacements: []orbittemplate.ReplacementSummary{
			{
				Variable: "service_url",
				Literal:  "https://example.test",
				Count:    1,
			},
			{
				Variable: "project_name",
				Literal:  "Orbit",
				Count:    2,
			},
		},
	}, result.ReplacementSummary)
	require.Nil(t, result.Ambiguity)
}

func TestBuildRootAgentsTemplateFileReturnsEmptyWhenRootAgentsMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "seed runtime repo without root agents")

	result, err := BuildRootAgentsTemplateFile(context.Background(), repo.Root)
	require.NoError(t, err)
	require.False(t, result.IncludesRootAgents)
	require.Nil(t, result.File)
	require.Nil(t, result.ReplacementSummary)
	require.Nil(t, result.Ambiguity)
	require.Empty(t, result.Variables)
}
