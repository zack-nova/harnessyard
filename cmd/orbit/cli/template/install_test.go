package orbittemplate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

func TestWriteAndLoadInstallRecordRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	appliedAt := time.Date(2026, time.March, 21, 10, 30, 0, 0, time.UTC)
	input := InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
	}

	filename, err := WriteInstallRecord(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".orbit", "installs", "docs.yaml"), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"    source_kind: local_branch\n"+
		"    source_repo: \"\"\n"+
		"    source_ref: orbit-template/docs\n"+
		"    template_commit: abc123\n"+
		"applied_at: 2026-03-21T10:30:00Z\n", string(data))

	loaded, err := LoadInstallRecord(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadInstallRecordRoundTripWithDetachedStatus(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	appliedAt := time.Date(2026, time.March, 21, 10, 30, 0, 0, time.UTC)
	input := InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Status:        InstallRecordStatusDetached,
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
	}

	filename, err := WriteInstallRecord(repoRoot, input)
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"status: detached\n"+
		"template:\n"+
		"    source_kind: local_branch\n"+
		"    source_repo: \"\"\n"+
		"    source_ref: orbit-template/docs\n"+
		"    template_commit: abc123\n"+
		"applied_at: 2026-03-21T10:30:00Z\n", string(data))

	loaded, err := LoadInstallRecord(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadInstallRecordRejectsMismatchedOrbitID(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".orbit", "installs", "docs.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"orbit_id: cmd\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: abc123\n"+
		"applied_at: 2026-03-21T10:30:00Z\n"), 0o600))

	_, err := LoadInstallRecord(repoRoot, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit_id must match install path")
}

func TestParseInstallRecordDataNormalizesLegacyRemoteGitKind(t *testing.T) {
	t.Parallel()

	record, err := ParseInstallRecordData([]byte("" +
		"schema_version: 1\n" +
		"orbit_id: docs\n" +
		"template:\n" +
		"  source_kind: remote_git\n" +
		"  source_repo: /tmp/source-repo\n" +
		"  source_ref: orbit-template/docs\n" +
		"  template_commit: abc123\n" +
		"applied_at: 2026-03-21T10:30:00Z\n"))
	require.NoError(t, err)
	require.Equal(t, InstallSourceKindExternalGit, record.Template.SourceKind)
	require.Equal(t, "/tmp/source-repo", record.Template.SourceRepo)
}

func TestWriteInstallRecordCanonicalizesLegacyRemoteGitKind(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename, err := WriteInstallRecord(repoRoot, InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: Source{
			SourceKind:     "remote_git",
			SourceRepo:     "/tmp/source-repo",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.March, 21, 10, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Contains(t, string(data), "source_kind: external_git")
	require.NotContains(t, string(data), "source_kind: remote_git")
}

func TestValidateInstallRecordRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	appliedAt := time.Date(2026, time.March, 21, 10, 30, 0, 0, time.UTC)
	testCases := []struct {
		name     string
		input    InstallRecord
		contains string
	}{
		{
			name: "schema version must be frozen",
			input: InstallRecord{
				SchemaVersion: 2,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
			},
			contains: "schema_version must be 1",
		},
		{
			name: "source kind enum is constrained",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     "manual_copy",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
			},
			contains: "template.source_kind",
		},
		{
			name: "status enum is constrained",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Status:        "paused",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
			},
			contains: "status must be one of",
		},
		{
			name: "remote source requires repo URL",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindExternalGit,
					SourceRef:      "refs/heads/template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
			},
			contains: "template.source_repo",
		},
		{
			name: "resolved snapshot must reference declared variable",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
				Variables: &InstallVariablesSnapshot{
					Declarations: map[string]bindings.VariableDeclaration{
						"project_name": {
							Required: true,
						},
					},
					ResolvedAtApply: map[string]bindings.VariableBinding{
						"command_name": {
							Value: "orbit",
						},
					},
				},
			},
			contains: "resolved_at_apply.command_name must reference a declared variable",
		},
		{
			name: "unresolved snapshot must reference declared variable",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
				Variables: &InstallVariablesSnapshot{
					Declarations: map[string]bindings.VariableDeclaration{
						"project_name": {
							Required: true,
						},
					},
					UnresolvedAtApply: []string{"command_name"},
				},
			},
			contains: "unresolved_at_apply.command_name must reference a declared variable",
		},
		{
			name: "namespace snapshot must reference declared variable",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
				Variables: &InstallVariablesSnapshot{
					Declarations: map[string]bindings.VariableDeclaration{
						"project_name": {
							Required: true,
						},
					},
					Namespaces: map[string]string{
						"command_name": "docs",
					},
				},
			},
			contains: "namespaces.command_name must reference a declared variable",
		},
		{
			name: "namespace snapshot must use valid orbit ids",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
				Variables: &InstallVariablesSnapshot{
					Declarations: map[string]bindings.VariableDeclaration{
						"project_name": {
							Required: true,
						},
					},
					Namespaces: map[string]string{
						"project_name": "Bad Docs",
					},
				},
			},
			contains: "namespaces.project_name",
		},
		{
			name: "observed runtime unresolved snapshot must use valid variable names",
			input: InstallRecord{
				SchemaVersion: 1,
				OrbitID:       "docs",
				Template: Source{
					SourceKind:     InstallSourceKindLocalBranch,
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
				AppliedAt: appliedAt,
				Variables: &InstallVariablesSnapshot{
					ObservedRuntimeUnresolved: []string{"123bad"},
				},
			},
			contains: "observed_runtime_unresolved",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateInstallRecord(testCase.input)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}

func TestWriteAndLoadInstallRecordRoundTripWithVariableSnapshots(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	appliedAt := time.Date(2026, time.March, 21, 10, 30, 0, 0, time.UTC)
	input := InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
		Variables: &InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
				"command_name": {
					Description: "CLI binary",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Orbit",
					Description: "Product title",
				},
			},
			Namespaces: map[string]string{
				"project_name": "docs",
			},
			UnresolvedAtApply: []string{"command_name"},
			ObservedRuntimeUnresolved: []string{
				"command_name",
				"manual_placeholder",
			},
		},
	}

	filename, err := WriteInstallRecord(repoRoot, input)
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"    source_kind: local_branch\n"+
		"    source_repo: \"\"\n"+
		"    source_ref: orbit-template/docs\n"+
		"    template_commit: abc123\n"+
		"applied_at: 2026-03-21T10:30:00Z\n"+
		"variables:\n"+
		"    declarations:\n"+
		"        command_name:\n"+
		"            description: CLI binary\n"+
		"            required: true\n"+
		"        project_name:\n"+
		"            description: Product title\n"+
		"            required: true\n"+
		"    namespaces:\n"+
		"        project_name: docs\n"+
		"    resolved_at_apply:\n"+
		"        project_name:\n"+
		"            value: Orbit\n"+
		"            description: Product title\n"+
		"    unresolved_at_apply:\n"+
		"        - command_name\n"+
		"    observed_runtime_unresolved:\n"+
		"        - command_name\n"+
		"        - manual_placeholder\n", string(data))

	loaded, err := LoadInstallRecord(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}
