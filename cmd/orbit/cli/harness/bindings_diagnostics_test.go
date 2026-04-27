package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestInspectMissingBindingsMarksObservedOptionalPlaceholder(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.April, 11, 12, 0, 0, 0, time.UTC)
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
	_, err = WriteVarsFile(repoRoot, bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
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
			Declarations: map[string]bindings.VariableDeclaration{
				"optional_note": {Description: "Optional footer", Required: false},
			},
			ResolvedAtApply:           map[string]bindings.VariableBinding{},
			ObservedRuntimeUnresolved: []string{"optional_note"},
		},
	})
	require.NoError(t, err)

	result, err := InspectMissingBindings(context.Background(), MissingBindingsInput{
		RepoRoot: repoRoot,
		OrbitID:  "docs",
	})
	require.NoError(t, err)
	require.Len(t, result.Orbits, 1)
	require.Equal(t, 1, result.Orbits[0].MissingCount)
	require.Equal(t, []MissingBindingsVariableResult{
		{
			Name:                      "optional_note",
			Description:               "Optional footer",
			Required:                  false,
			HasValue:                  false,
			ObservedRuntimeUnresolved: true,
			Missing:                   true,
		},
	}, result.Orbits[0].Variables)
}

func TestInspectMissingBindingsReportsSnapshotlessInstallRecord(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repoRoot := repo.Root
	now := time.Date(2026, time.April, 11, 12, 30, 0, 0, time.UTC)
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
	_, err = WriteVarsFile(repoRoot, bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
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
	})
	require.NoError(t, err)

	result, err := InspectMissingBindings(context.Background(), MissingBindingsInput{
		RepoRoot: repoRoot,
		OrbitID:  "docs",
	})
	require.NoError(t, err)
	require.Len(t, result.Orbits, 1)
	require.Equal(t, MissingBindingsOrbitResult{
		OrbitID:         "docs",
		SnapshotMissing: true,
		Variables:       []MissingBindingsVariableResult{},
	}, result.Orbits[0])
}

func TestInspectMissingBindingsRejectsDetachedInstallRecord(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.April, 16, 20, 0, 0, 0, time.UTC)
	runtimeFile, err := DefaultRuntimeFile(repoRoot, now)
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repoRoot, runtimeFile)
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
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	_, err = InspectMissingBindings(context.Background(), MissingBindingsInput{
		RepoRoot: repoRoot,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit "docs" is detached`)
}

func TestScanRuntimeBindingsRejectsDetachedInstallRecord(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.April, 16, 20, 5, 0, 0, time.UTC)
	runtimeFile, err := DefaultRuntimeFile(repoRoot, now)
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repoRoot, runtimeFile)
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
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	_, err = ScanRuntimeBindings(context.Background(), ScanRuntimeBindingsInput{
		RepoRoot: repoRoot,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit "docs" is detached`)
}
