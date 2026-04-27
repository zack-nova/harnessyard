package orbittemplate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteAndLoadManifestRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	createdAt := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)
	input := Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         createdAt,
		},
		Variables: map[string]VariableSpec{
			"project_name": {
				Description: "Project name",
				Required:    true,
			},
			"service_url": {
				Required: false,
			},
		},
	}

	filename, err := WriteManifest(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".orbit", "template.yaml"), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"    orbit_id: docs\n"+
		"    default_template: false\n"+
		"    created_from_branch: main\n"+
		"    created_from_commit: abc123\n"+
		"    created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"    project_name:\n"+
		"        description: Project name\n"+
		"        required: true\n"+
		"    service_url:\n"+
		"        required: false\n", string(data))

	loaded, err := LoadManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadManifestRejectsMissingRequiredFlag(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "template.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    description: title\n"), 0o600))

	_, err := LoadManifest(repoRoot)
	require.Error(t, err)
	require.ErrorContains(t, err, "variables.project_name.required")
}

func TestLoadManifestAllowsEmptyVariablesMap(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "template.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n"), 0o600))

	loaded, err := LoadManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
		Variables: map[string]VariableSpec{},
	}, loaded)
}

func TestValidateManifestRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)
	testCases := []struct {
		name     string
		input    Manifest
		contains string
	}{
		{
			name: "kind must stay frozen",
			input: Manifest{
				SchemaVersion: 1,
				Kind:          "runtime",
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "abc123",
					CreatedAt:         createdAt,
				},
				Variables: map[string]VariableSpec{
					"project_name": {Required: true},
				},
			},
			contains: "kind must be \"template\"",
		},
		{
			name: "orbit id must be valid",
			input: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "Docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "abc123",
					CreatedAt:         createdAt,
				},
				Variables: map[string]VariableSpec{
					"project_name": {Required: true},
				},
			},
			contains: "template.orbit_id",
		},
		{
			name: "variables field must be present",
			input: Manifest{
				SchemaVersion: 1,
				Kind:          TemplateKind,
				Template: Metadata{
					OrbitID:           "docs",
					DefaultTemplate:   false,
					CreatedFromBranch: "main",
					CreatedFromCommit: "abc123",
					CreatedAt:         createdAt,
				},
			},
			contains: "variables must be present",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateManifest(testCase.input)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}

func TestWriteAndLoadManifestRoundTripWithSharedAgentsEntry(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	createdAt := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)
	input := Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         createdAt,
		},
		Variables: map[string]VariableSpec{},
		SharedFiles: []SharedFileSpec{
			{
				Path:                   sharedFilePathAgents,
				Kind:                   SharedFileKindAgentsFragment,
				MergeMode:              SharedFileMergeModeReplaceBlock,
				IncludeUnmarkedContent: true,
			},
		},
	}

	filename, err := WriteManifest(repoRoot, input)
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"    orbit_id: docs\n"+
		"    default_template: false\n"+
		"    created_from_branch: main\n"+
		"    created_from_commit: abc123\n"+
		"    created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n"+
		"shared_files:\n"+
		"    - path: AGENTS.md\n"+
		"      kind: agents_fragment\n"+
		"      merge_mode: replace-block\n"+
		"      include_unmarked_content: true\n", string(data))

	loaded, err := LoadManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestValidateManifestRejectsInvalidSharedFileContracts(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)

	baseManifest := Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         createdAt,
		},
		Variables: map[string]VariableSpec{},
	}

	testCases := []struct {
		name     string
		input    Manifest
		contains string
	}{
		{
			name: "shared path must stay frozen",
			input: func() Manifest {
				manifest := baseManifest
				manifest.SharedFiles = []SharedFileSpec{
					{
						Path:                   "README.md",
						Kind:                   SharedFileKindAgentsFragment,
						MergeMode:              SharedFileMergeModeReplaceBlock,
						IncludeUnmarkedContent: true,
					},
				}
				return manifest
			}(),
			contains: "shared_files[0].path",
		},
		{
			name: "shared kind must stay frozen",
			input: func() Manifest {
				manifest := baseManifest
				manifest.SharedFiles = []SharedFileSpec{
					{
						Path:                   sharedFilePathAgents,
						Kind:                   "shared_file",
						MergeMode:              SharedFileMergeModeReplaceBlock,
						IncludeUnmarkedContent: true,
					},
				}
				return manifest
			}(),
			contains: "shared_files[0].kind",
		},
		{
			name: "shared merge mode must stay frozen",
			input: func() Manifest {
				manifest := baseManifest
				manifest.SharedFiles = []SharedFileSpec{
					{
						Path:                   sharedFilePathAgents,
						Kind:                   SharedFileKindAgentsFragment,
						MergeMode:              "append",
						IncludeUnmarkedContent: true,
					},
				}
				return manifest
			}(),
			contains: "shared_files[0].merge_mode",
		},
		{
			name: "shared entries must not be duplicated",
			input: func() Manifest {
				manifest := baseManifest
				manifest.SharedFiles = []SharedFileSpec{
					{
						Path:                   sharedFilePathAgents,
						Kind:                   SharedFileKindAgentsFragment,
						MergeMode:              SharedFileMergeModeReplaceBlock,
						IncludeUnmarkedContent: true,
					},
					{
						Path:                   sharedFilePathAgents,
						Kind:                   SharedFileKindAgentsFragment,
						MergeMode:              SharedFileMergeModeReplaceBlock,
						IncludeUnmarkedContent: false,
					},
				}
				return manifest
			}(),
			contains: "shared_files[1].path",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateManifest(testCase.input)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}
