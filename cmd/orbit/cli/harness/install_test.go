package harness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestWriteAndLoadHarnessInstallRecordRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	appliedAt := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	input := orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
	}

	filename, err := WriteInstallRecord(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".harness", "installs", "docs.yaml"), filename)

	loaded, err := LoadInstallRecord(repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestLoadHarnessInstallRecordRejectsMismatchedOrbitID(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, ".harness", "installs", "docs.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"schema_version: 1\n"+
		"orbit_id: cmd\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: abc123\n"+
		"applied_at: 2026-03-25T12:00:00Z\n"), 0o600))

	_, err := LoadInstallRecord(repoRoot, "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit_id must match install path")
}

func TestLoadHarnessInstallRecordFallsBackToHEADWhenHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	appliedAt := time.Date(2026, time.April, 16, 16, 0, 0, 0, time.UTC)
	_, err := WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: appliedAt,
	})
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "runtime root\n")
	repo.AddAndCommit(t, "seed install record")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	installPath, err := InstallRecordPath(repo.Root, "docs")
	require.NoError(t, err)
	_, err = os.Stat(installPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	record, err := LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "docs", record.OrbitID)
	require.Equal(t, appliedAt, record.AppliedAt)
}

func TestListInstallRecordIDsFallsBackToHEADWhenHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 18, 45, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "runtime root\n")
	repo.AddAndCommit(t, "seed hidden install ids")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	ids, err := ListInstallRecordIDs(repo.Root)
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, ids)
}

func TestSummarizeInstallRecordsSeparatesDetachedRecords(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	_, err := WriteInstallRecord(repoRoot, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 19, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = WriteInstallRecord(repoRoot, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "api",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/api",
			TemplateCommit: "def456",
		},
		AppliedAt: time.Date(2026, time.April, 16, 19, 5, 0, 0, time.UTC),
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	summary, err := SummarizeInstallRecords(repoRoot)
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, summary.ActiveIDs)
	require.Equal(t, []string{"api"}, summary.DetachedIDs)
	require.Empty(t, summary.InvalidIDs)
}

func TestSummarizeInstallRecordsSeparatesInvalidRecords(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	_, err := WriteInstallRecord(repoRoot, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 19, 10, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	brokenPath, err := InstallRecordPath(repoRoot, "broken")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(brokenPath), 0o755))
	require.NoError(t, os.WriteFile(brokenPath, []byte("schema_version: [\n"), 0o600))

	summary, err := SummarizeInstallRecords(repoRoot)
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, summary.ActiveIDs)
	require.Empty(t, summary.DetachedIDs)
	require.Equal(t, []string{"broken"}, summary.InvalidIDs)
}
