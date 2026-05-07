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

func TestBuildBundleMemberShrinkPlanIgnoresSnapshotRootGuidancePaths(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveRepo(t)
	templateManifest := TemplateManifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC),
			RootGuidance: RootGuidanceMetadata{
				Humans:    true,
				Bootstrap: true,
			},
		},
		Members: []TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	}
	_, err := WriteTemplateManifest(repo.Root, templateManifest)
	require.NoError(t, err)
	_, err = WriteManifestFile(repo.Root, ManifestFile{
		SchemaVersion: 1,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC),
		},
		Members: []ManifestMember{
			{OrbitID: "docs"},
		},
		RootGuidance: RootGuidanceMetadata{
			Humans:    true,
			Bootstrap: true,
		},
	})
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", "Human guidance\n")
	repo.WriteFile(t, "BOOTSTRAP.md", "Bootstrap guidance\n")
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - BOOTSTRAP.md\n"+
		"    - HUMANS.md\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    BOOTSTRAP.md: "+contentDigest([]byte("Bootstrap guidance\n"))+"\n"+
		"    HUMANS.md: "+contentDigest([]byte("Human guidance\n"))+"\n"+
		"    docs/guide.md: "+contentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.AddAndCommit(t, "add root guidance payload")
	templateCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	record := BundleRecord{
		SchemaVersion: 1,
		HarnessID:     "workspace",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "harness-template/workspace",
			TemplateCommit: templateCommit,
		},
		MemberIDs:          []string{"docs"},
		AppliedAt:          time.Date(2026, time.April, 25, 10, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths: []string{
			".harness/orbits/docs.yaml",
			"BOOTSTRAP.md",
			"HUMANS.md",
			"docs/guide.md",
		},
		OwnedPathDigests: bundleTestDigests(t, repo.Root, []string{
			".harness/orbits/docs.yaml",
			"BOOTSTRAP.md",
			"HUMANS.md",
			"docs/guide.md",
		}),
	}

	plan, err := BuildBundleMemberShrinkPlan(context.Background(), repo.Root, record, []string{"docs"})
	require.NoError(t, err)
	require.True(t, plan.DeleteBundleRecord)
	require.Contains(t, plan.DeletePaths, ".harness/orbits/docs.yaml")
	require.Contains(t, plan.DeletePaths, "docs/guide.md")
	require.NotContains(t, plan.DeletePaths, "BOOTSTRAP.md")
	require.NotContains(t, plan.DeletePaths, "HUMANS.md")
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
