package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestBuildBundleMemberShrinkPlanAllowsSharedPathsWhenAllContributorsAreRemoved(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateRemoveRepo(t, false)
	templateCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	record := BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "workspace",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: templateCommit,
		},
		MemberIDs:          []string{"docs", "shared"},
		AppliedAt:          time.Date(2026, time.April, 25, 10, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths: []string{
			".harness/orbits/docs.yaml",
			".harness/orbits/shared.yaml",
			"docs/guide.md",
			"shared/checklist.md",
		},
		OwnedPathDigests: bundleTestDigests(t, repo.Root, []string{
			".harness/orbits/docs.yaml",
			".harness/orbits/shared.yaml",
			"docs/guide.md",
			"shared/checklist.md",
		}),
	}
	_, err := WriteBundleRecord(repo.Root, record)
	require.NoError(t, err)

	plan, err := BuildBundleMemberShrinkPlan(context.Background(), repo.Root, record, []string{"docs", "shared"})
	require.NoError(t, err)
	require.True(t, plan.DeleteBundleRecord)
	require.Nil(t, plan.NextRecord)
	require.Equal(t, []string{"docs", "shared"}, plan.RemovedMemberIDs)
	require.Contains(t, plan.DeletePaths, ".harness/orbits/docs.yaml")
	require.Contains(t, plan.DeletePaths, ".harness/orbits/shared.yaml")
	require.Contains(t, plan.DeletePaths, "docs/guide.md")
	require.Contains(t, plan.DeletePaths, "shared/checklist.md")

	removedPaths, err := ApplyBundleMemberShrinkPlan(repo.Root, plan)
	require.NoError(t, err)
	require.Contains(t, removedPaths, ".harness/bundles/workspace.yaml")
	require.Contains(t, removedPaths, "shared/checklist.md")

	_, err = os.Stat(filepath.Join(repo.Root, "shared", "checklist.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "bundles", "workspace.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func bundleTestDigests(t *testing.T, repoRoot string, paths []string) map[string]string {
	t.Helper()

	digests := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
		require.NoError(t, err)
		digests[path] = contentDigest(data)
	}

	return digests
}
