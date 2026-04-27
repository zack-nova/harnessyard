package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestWriteAndLoadRuntimeFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	createdAt := time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC)
	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			Name:      "Project A",
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Members: []RuntimeMember{
			{
				OrbitID: "docs",
				Source:  MemberSourceInstallOrbit,
				AddedAt: time.Date(2026, time.March, 25, 10, 20, 0, 0, time.UTC),
			},
			{
				OrbitID: "cli",
				Source:  MemberSourceManual,
				AddedAt: time.Date(2026, time.March, 25, 10, 10, 0, 0, time.UTC),
			},
			{
				OrbitID:        "bundle",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "project_a",
				AddedAt:        time.Date(2026, time.March, 25, 10, 30, 0, 0, time.UTC),
			},
		},
	}

	filename, err := WriteRuntimeFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repoRoot), filename)
	_, err = os.Stat(filepath.Join(repoRoot, ".harness", "runtime.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	expected := input
	expected.Members = []RuntimeMember{
		{
			OrbitID:        "bundle",
			Source:         MemberSourceInstallBundle,
			OwnerHarnessID: "project_a",
			AddedAt:        time.Date(2026, time.March, 25, 10, 30, 0, 0, time.UTC),
		},
		{
			OrbitID: "cli",
			Source:  MemberSourceManual,
			AddedAt: time.Date(2026, time.March, 25, 10, 10, 0, 0, time.UTC),
		},
		{
			OrbitID: "docs",
			Source:  MemberSourceInstallOrbit,
			AddedAt: time.Date(2026, time.March, 25, 10, 20, 0, 0, time.UTC),
		},
	}

	loaded, err := LoadRuntimeFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, expected, loaded)

	manifest, err := LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, ManifestFileFromRuntimeFile(expected), manifest)
}

func TestWriteAndLoadRuntimeFileAllowsZeroMembers(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{},
	}

	_, err := WriteRuntimeFile(repoRoot, input)
	require.NoError(t, err)

	loaded, err := LoadRuntimeFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadRuntimeFileAtPathRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".harness", "runtime.yaml")
	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: time.Date(2026, time.March, 25, 10, 20, 0, 0, time.UTC)},
		},
	}

	writtenPath, err := WriteRuntimeFileAtPath(filename, input)
	require.NoError(t, err)
	require.Equal(t, filename, writtenPath)

	loaded, err := LoadRuntimeFileAtPath(filename)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadRuntimeFileAtPathRoundTripPreservesAffiliationMetadata(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".harness", "runtime.yaml")
	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 9, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 9, 30, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 9, 10, 0, 0, time.UTC),
				LastStandaloneOrigin: &orbittemplate.Source{
					SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
					SourceRepo:     "",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
			},
		},
	}

	writtenPath, err := WriteRuntimeFileAtPath(filename, input)
	require.NoError(t, err)
	require.Equal(t, filename, writtenPath)

	loaded, err := LoadRuntimeFileAtPath(filename)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadRuntimeFileIgnoresLegacyRuntimeFileWhenManifestIsValid(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceManual, AddedAt: time.Date(2026, time.March, 25, 10, 10, 0, 0, time.UTC)},
		},
	}

	_, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(input))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".harness", "runtime.yaml"), []byte("schema_version: nope\n"), 0o600))

	loaded, err := LoadRuntimeFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestValidateRuntimeFileRejectsDuplicateMembers(t *testing.T) {
	t.Parallel()

	input := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceManual, AddedAt: time.Date(2026, time.March, 25, 10, 10, 0, 0, time.UTC)},
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: time.Date(2026, time.March, 25, 10, 20, 0, 0, time.UTC)},
		},
	}

	err := ValidateRuntimeFile(input)
	require.Error(t, err)
	require.ErrorContains(t, err, "must be unique")
}

func TestValidateRuntimeFileRejectsInvalidAffiliationCombinations(t *testing.T) {
	t.Parallel()

	base := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 9, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 9, 30, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name     string
		member   RuntimeMember
		contains string
	}{
		{
			name: "bundle member requires owner harness id",
			member: RuntimeMember{
				OrbitID: "docs",
				Source:  MemberSourceInstallBundle,
				AddedAt: time.Date(2026, time.April, 22, 9, 10, 0, 0, time.UTC),
			},
			contains: `members[0].owner_harness_id must be present when source is "install_bundle"`,
		},
		{
			name: "last standalone origin validates source kind",
			member: RuntimeMember{
				OrbitID:        "docs",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 9, 10, 0, 0, time.UTC),
				LastStandaloneOrigin: &orbittemplate.Source{
					SourceKind:     "unknown",
					SourceRepo:     "",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
			},
			contains: `members[0].last_standalone_origin.source_kind must be one of "local_branch" or "external_git"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := base
			input.Members = []RuntimeMember{tt.member}

			err := ValidateRuntimeFile(input)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.contains)
		})
	}
}

func TestValidateRuntimeFileAllowsAssignedStandaloneSources(t *testing.T) {
	t.Parallel()

	base := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "project_a",
			CreatedAt: time.Date(2026, time.April, 22, 9, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 9, 30, 0, 0, time.UTC),
		},
	}

	tests := []struct {
		name   string
		member RuntimeMember
	}{
		{
			name: "manual member may declare owner harness id",
			member: RuntimeMember{
				OrbitID:        "docs",
				Source:         MemberSourceManual,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 9, 10, 0, 0, time.UTC),
			},
		},
		{
			name: "install orbit member may declare owner harness id",
			member: RuntimeMember{
				OrbitID:        "api",
				Source:         MemberSourceInstallOrbit,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 9, 20, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := base
			input.Members = []RuntimeMember{tt.member}

			err := ValidateRuntimeFile(input)
			require.NoError(t, err)
		})
	}
}
