package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteAndLoadTemplateManifestRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	createdAt := time.Date(2026, time.March, 25, 13, 0, 0, 0, time.UTC)
	input := TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:          "project_a",
			DefaultTemplate:    false,
			CreatedFromBranch:  "main",
			CreatedFromCommit:  "abc123",
			CreatedAt:          createdAt,
			IncludesRootAgents: true,
		},
		Members: []TemplateMember{
			{OrbitID: "docs"},
			{OrbitID: "cli"},
		},
		Variables: map[string]TemplateVariableSpec{
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
			"service_url": {
				Required: false,
			},
		},
	}

	filename, err := WriteTemplateManifest(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, TemplatePath(repoRoot), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"    harness_id: project_a\n"+
		"    default_template: false\n"+
		"    created_from_branch: main\n"+
		"    created_from_commit: abc123\n"+
		"    created_at: 2026-03-25T13:00:00Z\n"+
		"    includes_root_agents: true\n"+
		"members:\n"+
		"    - orbit_id: cli\n"+
		"    - orbit_id: docs\n"+
		"variables:\n"+
		"    project_name:\n"+
		"        description: Project name\n"+
		"        required: true\n"+
		"    service_url:\n"+
		"        required: false\n", string(data))

	expected := input
	expected.Members = []TemplateMember{
		{OrbitID: "cli"},
		{OrbitID: "docs"},
	}

	loaded, err := LoadTemplateManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, expected, loaded)
}

func TestLoadTemplateManifestAllowsEmptyMembersAndVariables(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(TemplatePath(repoRoot)), 0o755))
	require.NoError(t, os.WriteFile(TemplatePath(repoRoot), []byte(""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: project_a\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-25T13:00:00Z\n"+
		"  includes_root_agents: false\n"+
		"members: []\n"+
		"variables: {}\n"), 0o600))

	loaded, err := LoadTemplateManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:          "project_a",
			DefaultTemplate:    false,
			CreatedFromBranch:  "main",
			CreatedFromCommit:  "abc123",
			CreatedAt:          time.Date(2026, time.March, 25, 13, 0, 0, 0, time.UTC),
			IncludesRootAgents: false,
		},
		Members:   []TemplateMember{},
		Variables: map[string]TemplateVariableSpec{},
	}, loaded)
}

func TestValidateTemplateManifestRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	input := TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:          "project_a",
			DefaultTemplate:    false,
			CreatedFromBranch:  "main",
			CreatedFromCommit:  "abc123",
			CreatedAt:          time.Date(2026, time.March, 25, 13, 0, 0, 0, time.UTC),
			IncludesRootAgents: false,
		},
		Members: []TemplateMember{
			{OrbitID: "docs"},
			{OrbitID: "docs"},
		},
		Variables: map[string]TemplateVariableSpec{},
	}

	err := ValidateTemplateManifest(input)
	require.Error(t, err)
	require.ErrorContains(t, err, "must be unique")
}
