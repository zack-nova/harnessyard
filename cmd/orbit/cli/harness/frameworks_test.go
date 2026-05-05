package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestWriteAndLoadFrameworksFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := FrameworksFile{
		SchemaVersion:        1,
		RecommendedFramework: "claude",
	}

	filename, err := WriteFrameworksFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, FrameworksPath(repoRoot), filename)

	loaded, err := LoadFrameworksFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadAgentConfigFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := AgentConfigFile{
		SchemaVersion: 1,
	}

	filename, err := WriteAgentConfigFile(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, AgentConfigPath(repoRoot), filename)

	loaded, err := LoadAgentConfigFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadAgentOverlayFileRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := AgentOverlayFile{
		SchemaVersion: 1,
		Mode:          AgentOverlayModeRawPassthrough,
		Content: []byte("" +
			"schema_version: 1\n" +
			"mode: raw_passthrough\n" +
			"raw:\n" +
			"  profile: strict\n"),
	}

	filename, err := WriteAgentOverlayFile(repoRoot, "claude", input)
	require.NoError(t, err)
	require.Equal(t, AgentOverlayPath(repoRoot, "claude"), filename)

	loaded, err := LoadAgentOverlayFile(repoRoot, "claude")
	require.NoError(t, err)
	require.Equal(t, input.SchemaVersion, loaded.SchemaVersion)
	require.Equal(t, input.Mode, loaded.Mode)
	require.Equal(t, string(input.Content), string(loaded.Content))
}

func TestWriteAndLoadFrameworkSelectionRoundTrip(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	input := FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 16, 13, 0, 0, 0, time.UTC),
	}

	filename, err := WriteFrameworkSelection(filepath.Join(repo.Root, ".git"), input)
	require.NoError(t, err)
	require.Equal(t, FrameworkSelectionPath(filepath.Join(repo.Root, ".git")), filename)

	loaded, err := LoadFrameworkSelection(filepath.Join(repo.Root, ".git"))
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestWriteAndLoadFrameworkActivationRoundTrip(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	input := FrameworkActivation{
		Framework:             "claude",
		ResolutionSource:      FrameworkSelectionSourceExplicitLocal,
		RepoRoot:              repo.Root,
		AppliedAt:             time.Date(2026, time.April, 16, 15, 0, 0, 0, time.UTC),
		GuidanceHash:          "guidance-hash",
		CapabilitiesHash:      "capabilities-hash",
		SelectionHash:         "selection-hash",
		RuntimeAgentTruthHash: "agent-truth-hash",
		ProjectOutputs: []FrameworkActivationOutput{
			{
				Path:         "CLAUDE.md",
				AbsolutePath: filepath.Join(repo.Root, "CLAUDE.md"),
				Kind:         "framework_alias",
				Action:       "symlink",
				Target:       filepath.Join(repo.Root, "AGENTS.md"),
			},
		},
		GlobalOutputs: []FrameworkActivationOutput{
			{
				Path:         "~/.claude/commands/demo__docs__review.md",
				AbsolutePath: filepath.Join(repo.Root, ".tmp", "demo__docs__review.md"),
				Kind:         "command",
				Action:       "symlink",
				Target:       filepath.Join(repo.Root, "orbit", "commands", "review.md"),
			},
		},
	}

	filename, err := WriteFrameworkActivation(filepath.Join(repo.Root, ".git"), input)
	require.NoError(t, err)
	require.Equal(t, FrameworkActivationPath(filepath.Join(repo.Root, ".git"), "claude"), filename)

	loaded, err := LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "claude")
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestResolveFrameworkPrefersExplicitLocalSelectionOverHintProjectAndRecommended(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.WriteFile(t, "CLAUDE.md", "# Claude alias\n")

	_, err := WriteFrameworkSelection(filepath.Join(repo.Root, ".git"), FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 16, 13, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "codex", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceExplicitLocal, resolution.Source)
}

func TestResolveFrameworkUsesProjectDetectionBeforeRecommendedDefault(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")
	repo.WriteFile(t, "CLAUDE.md", "# Claude alias\n")

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "claudecode", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceProjectDetection, resolution.Source)
}

func TestResolveFrameworkReturnsRecommendedDefaultWhenNoHigherPrioritySignalExists(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "claudecode", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceRecommendedDefault, resolution.Source)
}

func TestResolveFrameworkReturnsUnresolvedWhenNoSignalExists(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Empty(t, resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceUnresolved, resolution.Source)
}

func TestResolveFrameworkUsesUniqueBundleRecommendationWhenRuntimeRootRecommendationIsAbsent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "docs_stack",
				AddedAt:        time.Date(2026, time.April, 23, 10, 1, 0, 0, time.UTC),
			},
			{
				OrbitID:        "cmd",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "cmd_stack",
				AddedAt:        time.Date(2026, time.April, 23, 10, 2, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "docs_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/docs-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"docs"},
		AppliedAt:            time.Date(2026, time.April, 23, 10, 1, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"docs/guide.md"},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "cmd_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/cmd-stack", TemplateCommit: "bbb222"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"cmd"},
		AppliedAt:            time.Date(2026, time.April, 23, 10, 2, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"cmd/main.go"},
	})
	require.NoError(t, err)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "claudecode", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourcePackageRecommendation, resolution.Source)
	require.Len(t, resolution.PackageRecommendations, 2)
	require.Equal(t, []FrameworkPackageRecommendation{
		{HarnessID: "cmd_stack", RecommendedFramework: "claudecode"},
		{HarnessID: "docs_stack", RecommendedFramework: "claudecode"},
	}, resolution.PackageRecommendations)
}

func TestResolveFrameworkUsesOwnerBasedAssignedRecommendationWithoutBundleBackedMembers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 23, 10, 30, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 23, 10, 30, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceManual,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 23, 10, 31, 0, 0, time.UTC),
			},
			{
				OrbitID:        "api",
				Source:         MemberSourceInstallOrbit,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 23, 10, 32, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "writing_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/writing-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"docs", "api"},
		AppliedAt:            time.Date(2026, time.April, 23, 10, 31, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "claudecode", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourcePackageRecommendation, resolution.Source)
	require.Equal(t, []FrameworkPackageRecommendation{
		{HarnessID: "writing_stack", RecommendedFramework: "claudecode"},
	}, resolution.PackageRecommendations)
}

func TestResolveFrameworkReturnsUnresolvedConflictForConflictingBundleRecommendations(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 23, 11, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 23, 11, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "docs_stack",
				AddedAt:        time.Date(2026, time.April, 23, 11, 1, 0, 0, time.UTC),
			},
			{
				OrbitID:        "cmd",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "cmd_stack",
				AddedAt:        time.Date(2026, time.April, 23, 11, 2, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "docs_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/docs-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"docs"},
		AppliedAt:            time.Date(2026, time.April, 23, 11, 1, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"docs/guide.md"},
	})
	require.NoError(t, err)
	_, err = WriteBundleRecord(repo.Root, BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "cmd_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/cmd-stack", TemplateCommit: "bbb222"},
		RecommendedFramework: "codex",
		MemberIDs:            []string{"cmd"},
		AppliedAt:            time.Date(2026, time.April, 23, 11, 2, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"cmd/main.go"},
	})
	require.NoError(t, err)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Empty(t, resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceUnresolvedConflict, resolution.Source)
	require.Equal(t, []FrameworkPackageRecommendation{
		{HarnessID: "cmd_stack", RecommendedFramework: "codex"},
		{HarnessID: "docs_stack", RecommendedFramework: "claudecode"},
	}, resolution.PackageRecommendations)
	require.Contains(t, resolution.Warnings, `conflicting package framework recommendations detected: cmd_stack=codex, docs_stack=claudecode`)
}

func TestResolveFrameworkSkipsUnsupportedExplicitSelectionAndWarns(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	_, err := WriteFrameworkSelection(filepath.Join(repo.Root, ".git"), FrameworkSelection{
		SelectedFramework: "unknown",
		SelectionSource:   FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 16, 14, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	resolution, err := ResolveFramework(context.Background(), FrameworkResolutionInput{
		RepoRoot: repo.Root,
		GitDir:   filepath.Join(repo.Root, ".git"),
	})
	require.NoError(t, err)
	require.Equal(t, "claudecode", resolution.Framework)
	require.Equal(t, FrameworkSelectionSourceRecommendedDefault, resolution.Source)
	require.Contains(t, resolution.Warnings, `ignore unsupported explicit local framework selection "unknown"`)
}

func TestLoadFrameworkSelectionReturnsNotExistWhenMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, err := LoadFrameworkSelection(filepath.Join(repo.Root, ".git"))
	require.Error(t, err)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLoadFrameworksFileFallsBackToLegacyPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")

	file, err := LoadFrameworksFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, FrameworksFile{
		SchemaVersion:        1,
		RecommendedFramework: "claude",
	}, file)
}

func TestLoadFrameworkSelectionFallsBackToLegacyPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".git/orbit/state/frameworks/selection.json", ""+
		"{\n"+
		"  \"selected_framework\": \"codex\",\n"+
		"  \"selection_source\": \"explicit_local\",\n"+
		"  \"updated_at\": \"2026-04-16T13:00:00Z\"\n"+
		"}\n")

	selection, err := LoadFrameworkSelection(filepath.Join(repo.Root, ".git"))
	require.NoError(t, err)
	require.Equal(t, FrameworkSelection{
		SelectedFramework: "codex",
		SelectionSource:   FrameworkSelectionSourceExplicitLocal,
		UpdatedAt:         time.Date(2026, time.April, 16, 13, 0, 0, 0, time.UTC),
	}, selection)
}

func TestLoadFrameworkActivationFallsBackToLegacyPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".git/orbit/state/frameworks/activations/claude.json", ""+
		"{\n"+
		"  \"framework\": \"claude\",\n"+
		"  \"resolution_source\": \"explicit_local\",\n"+
		"  \"repo_root\": "+jsonString(repo.Root)+",\n"+
		"  \"applied_at\": \"2026-04-16T15:00:00Z\",\n"+
		"  \"guidance_hash\": \"guidance-hash\",\n"+
		"  \"capabilities_hash\": \"capabilities-hash\",\n"+
		"  \"selection_hash\": \"selection-hash\",\n"+
		"  \"runtime_agent_truth_hash\": \"agent-truth-hash\"\n"+
		"}\n")

	activation, err := LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "claude")
	require.NoError(t, err)
	require.Equal(t, FrameworkActivation{
		Framework:             "claude",
		ResolutionSource:      FrameworkSelectionSourceExplicitLocal,
		RepoRoot:              repo.Root,
		AppliedAt:             time.Date(2026, time.April, 16, 15, 0, 0, 0, time.UTC),
		GuidanceHash:          "guidance-hash",
		CapabilitiesHash:      "capabilities-hash",
		SelectionHash:         "selection-hash",
		RuntimeAgentTruthHash: "agent-truth-hash",
	}, activation)
}

func TestParseFrameworksFileDataRejectsLegacyRecommendedFrameworksList(t *testing.T) {
	t.Parallel()

	_, err := ParseFrameworksFileData([]byte("" +
		"schema_version: 1\n" +
		"recommended_frameworks:\n" +
		"  - claude\n" +
		"  - codex\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "recommended_frameworks")
}

func jsonString(value string) string {
	return `"` + value + `"`
}
