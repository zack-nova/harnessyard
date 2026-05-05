package orbit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMarkdownMemberHintExtractsOrbitMemberAndDefaultsNameAndRole(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"title: Review Flow\n"+
		"orbit_member:\n"+
		"  description: Documentation review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, resolvedMemberHint{
		Kind:        memberHintKindFileFrontmatter,
		HintPath:    "docs/process/review.md",
		RootPath:    "docs/process/review.md",
		Name:        "review",
		Description: "Documentation review workflow",
		Role:        OrbitMemberRule,
	}, hint)
}

func TestParseMarkdownMemberHintReturnsFalseWhenOrbitMemberIsAbsent(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"title: Review Flow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, resolvedMemberHint{}, hint)
}

func TestParseMarkdownMemberHintAcceptsFlatNameAndDescription(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"name: docs-review\n"+
		"description: Documentation review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, resolvedMemberHint{
		Kind:        memberHintKindFileFrontmatter,
		HintPath:    "docs/process/review.md",
		RootPath:    "docs/process/review.md",
		Name:        "docs-review",
		Description: "Documentation review workflow",
		Role:        OrbitMemberRule,
	}, hint)
}

func TestParseMarkdownMemberHintIgnoresMixedDocumentMetadata(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"name: docs-review\n"+
		"title: Review Flow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, resolvedMemberHint{}, hint)
}

func TestParseMarkdownMemberHintRejectsInvalidOrbitMemberShape(t *testing.T) {
	t.Parallel()

	_, _, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"orbit_member: review\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit_member must be a mapping")
}

func TestParseMarkdownMemberHintRejectsReservedName(t *testing.T) {
	t.Parallel()

	_, _, err := parseMarkdownMemberHint("docs/spec.md", []byte(""+
		"---\n"+
		"orbit_member:\n"+
		"  name: spec\n"+
		"---\n"+
		"\n"+
		"# Spec\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit_member.name "spec" is reserved`)
}

func TestParseMarkdownMemberHintPreservesExplicitFields(t *testing.T) {
	t.Parallel()

	writeFalse := false
	orchestrationTrue := true

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-review\n"+
		"  role: process\n"+
		"  lane: bootstrap\n"+
		"  scopes:\n"+
		"    write: false\n"+
		"    orchestration: true\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, resolvedMemberHint{
		Kind:     memberHintKindFileFrontmatter,
		HintPath: "docs/process/review.md",
		RootPath: "docs/process/review.md",
		Name:     "docs-review",
		Role:     OrbitMemberProcess,
		Lane:     OrbitMemberLaneBootstrap,
		Scopes: &OrbitMemberScopePatch{
			Write:         &writeFalse,
			Orchestration: &orchestrationTrue,
		},
	}, hint)
}

func TestParseDirectoryMemberHintDefaultsNameAndRole(t *testing.T) {
	t.Parallel()

	hint, err := parseDirectoryMemberHint("docs/process/.orbit-member.yaml", []byte(""+
		"orbit_member:\n"+
		"  description: Documentation review workflow\n"))
	require.NoError(t, err)
	require.Equal(t, resolvedMemberHint{
		Kind:        memberHintKindDirectoryMarker,
		HintPath:    "docs/process/.orbit-member.yaml",
		RootPath:    "docs/process",
		Name:        "process",
		Description: "Documentation review workflow",
		Role:        OrbitMemberProcess,
	}, hint)
}

func TestParseDirectoryMemberHintRejectsUnknownOrbitMemberField(t *testing.T) {
	t.Parallel()

	_, err := parseDirectoryMemberHint("docs/process/.orbit-member.yaml", []byte(""+
		"orbit_member:\n"+
		"  unsupported: true\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "field unsupported not found")
}

func TestParseDirectoryMemberHintAcceptsFlatMarkerWithRuleDefault(t *testing.T) {
	t.Parallel()

	hint, err := parseDirectoryMemberHint("docs/process/.orbit-member.yaml", []byte(""+
		"name: docs-process\n"+
		"description: Documentation review workflow\n"))
	require.NoError(t, err)
	require.Equal(t, resolvedMemberHint{
		Kind:        memberHintKindDirectoryMarker,
		HintPath:    "docs/process/.orbit-member.yaml",
		RootPath:    "docs/process",
		Name:        "docs-process",
		Description: "Documentation review workflow",
		Role:        OrbitMemberRule,
	}, hint)
}

func TestBuildMemberHintCandidateForFileHint(t *testing.T) {
	t.Parallel()

	candidate := buildMemberHintCandidate(resolvedMemberHint{
		Kind:        memberHintKindFileFrontmatter,
		HintPath:    "docs/process/review.md",
		RootPath:    "docs/process/review.md",
		Name:        "review",
		Description: "Documentation review workflow",
		Role:        OrbitMemberRule,
	})

	require.Equal(t, memberHintCandidate{
		Hint: resolvedMemberHint{
			Kind:        memberHintKindFileFrontmatter,
			HintPath:    "docs/process/review.md",
			RootPath:    "docs/process/review.md",
			Name:        "review",
			Description: "Documentation review workflow",
			Role:        OrbitMemberRule,
		},
		Member: OrbitMember{
			Name:        "review",
			Description: "Documentation review workflow",
			Role:        OrbitMemberRule,
			Paths: OrbitMemberPaths{
				Include: []string{"docs/process/review.md"},
			},
		},
	}, candidate)
}

func TestBuildMemberHintCandidateForDirectoryHint(t *testing.T) {
	t.Parallel()

	candidate := buildMemberHintCandidate(resolvedMemberHint{
		Kind:     memberHintKindDirectoryMarker,
		HintPath: "docs/process/.orbit-member.yaml",
		RootPath: "docs/process",
		Name:     "process",
		Role:     OrbitMemberProcess,
	})

	require.Equal(t, OrbitMember{
		Name: "process",
		Role: OrbitMemberProcess,
		Paths: OrbitMemberPaths{
			Include: []string{"docs/process/**"},
		},
	}, candidate.Member)
}

func TestIsHintManageableMember(t *testing.T) {
	t.Parallel()

	require.True(t, isHintManageableMember(OrbitMember{
		Name: "review",
		Paths: OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
		},
	}))
	require.False(t, isHintManageableMember(OrbitMember{
		Name: "review",
		Paths: OrbitMemberPaths{
			Include: []string{"docs/process/review.md", "docs/process/checklist.md"},
		},
	}))
	require.False(t, isHintManageableMember(OrbitMember{
		Name: "review",
		Paths: OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
			Exclude: []string{"docs/process/archive/**"},
		},
	}))
}

func TestFilterMemberHintCandidateFilesExcludesControlAndCapabilityPaths(t *testing.T) {
	t.Parallel()

	spec := OrbitSpec{
		ID: "docs",
		Capabilities: &OrbitCapabilities{
			Commands: &OrbitCommandCapabilityPaths{
				Paths: OrbitMemberPaths{
					Include: []string{"tools/commands/**/*.md"},
				},
			},
			Skills: &OrbitSkillCapabilities{
				Local: &OrbitLocalSkillCapabilityPaths{
					Paths: OrbitMemberPaths{
						Include: []string{"tools/skills/*"},
					},
				},
			},
		},
	}

	filtered, err := filterMemberHintCandidateFiles(spec, []string{
		".harness/orbits/docs.yaml",
		"AGENTS.md",
		"BOOTSTRAP.md",
		"HUMANS.md",
		"commands/docs/review.md",
		"docs/rules/style.md",
		"extras/research-kit/SKILL.md",
		"skills/docs/frontend/SKILL.md",
		"skills/docs/frontend/notes.md",
		"tools/commands/check.md",
		"tools/skills/review/SKILL.md",
		"tools/skills/review/notes.md",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"docs/rules/style.md"}, filtered)
}

func TestConsumeMemberHintPathsRollsBackAppliedHintsWhenLaterMutationFails(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "process"), 0o755))

	reviewPath := filepath.Join(repoRoot, "docs", "process", "review.md")
	markerPath := filepath.Join(repoRoot, "docs", "process", ".orbit-member.yaml")
	reviewBefore := "" +
		"---\n" +
		"title: Review Flow\n" +
		"orbit_member:\n" +
		"  name: review\n" +
		"---\n" +
		"\n" +
		"# Review\n"
	require.NoError(t, os.WriteFile(reviewPath, []byte(reviewBefore), 0o644))
	require.NoError(t, os.WriteFile(markerPath, []byte("orbit_member:\n  description: Review workflow\n"), 0o644))

	previousHook := beforeMemberHintConsumeMutationHook
	beforeMemberHintConsumeMutationHook = func(filename string) {
		if filename != markerPath {
			return
		}
		require.NoError(t, os.Remove(markerPath))
		require.NoError(t, os.Mkdir(markerPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(markerPath, "child"), []byte("block remove"), 0o644))
	}
	t.Cleanup(func() {
		beforeMemberHintConsumeMutationHook = previousHook
	})

	_, err := ConsumeMemberHintPaths(repoRoot, []string{
		"docs/process/review.md",
		"docs/process/.orbit-member.yaml",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "member hint cleanup rollback after")

	reviewAfter, err := os.ReadFile(reviewPath)
	require.NoError(t, err)
	require.Equal(t, reviewBefore, string(reviewAfter))
}
