package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestPreviewBindingsApplyRejectsDetachedInstallRecord(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.April, 16, 20, 10, 0, 0, time.UTC)
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

	_, err = PreviewBindingsApply(context.Background(), BindingsApplyInput{
		RepoRoot: repoRoot,
		OrbitID:  "docs",
		Now:      now,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit "docs" is detached`)
}
