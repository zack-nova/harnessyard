package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestDeriveAgentTruthUsesOwnerBasedAssignedPackageTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceManual,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 23, 12, 1, 0, 0, time.UTC),
			},
			{
				OrbitID:        "api",
				Source:         MemberSourceInstallOrbit,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 23, 12, 2, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "writing_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/writing-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		AgentConfig: &AgentConfigFile{
			SchemaVersion: 1,
		},
		AgentOverlays: map[string]string{
			"claude": "" +
				"schema_version: 1\n" +
				"mode: raw_passthrough\n" +
				"raw:\n" +
				"  profile: strict\n",
		},
		MemberIDs:          []string{"docs", "api"},
		AppliedAt:          time.Date(2026, time.April, 23, 12, 1, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	result, err := DeriveAgentTruth(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, "claude", result.RecommendedFramework)
	require.Equal(t, 1, result.PackageCount)
	require.Empty(t, result.Warnings)
	require.Equal(t, []string{
		AgentConfigRepoPath(),
		FrameworksRepoPath(),
		AgentOverlayRepoPath("claude"),
	}, result.WrittenPaths)

	frameworksFile, err := LoadFrameworksFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, "claude", frameworksFile.RecommendedFramework)

	agentConfig, err := LoadAgentConfigFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, 1, agentConfig.SchemaVersion)

	overlay, err := LoadAgentOverlayFile(repo.Root, "claude")
	require.NoError(t, err)
	require.Equal(t, AgentOverlayModeRawPassthrough, overlay.Mode)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"mode: raw_passthrough\n"+
		"raw:\n"+
		"  profile: strict\n", string(overlay.Content))
}
