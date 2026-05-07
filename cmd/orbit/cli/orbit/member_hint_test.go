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

func TestParseMarkdownMemberHintNormalizesCRLFBeforeStrictDelimiters(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\r\n"+
		"orbit_member:\r\n"+
		"  name: docs-review\r\n"+
		"---\r\n"+
		"\r\n"+
		"# Review\r\n"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "docs-review", hint.Name)
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

func TestParseMarkdownMemberHintIgnoresFlatMemberMetadata(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"name: docs-review\n"+
		"description: Documentation review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, resolvedMemberHint{}, hint)
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

func TestParseMarkdownMemberHintIgnoresNonMappingDocumentFrontmatter(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"- Review Flow\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, resolvedMemberHint{}, hint)
}

func TestParseMarkdownMemberHintIgnoresMalformedDocumentFrontmatterWithoutOrbitMember(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"title: [Review Flow\n"+
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

func TestParseMarkdownMemberHintRejectsMalformedFrontmatterWithOrbitMember(t *testing.T) {
	t.Parallel()

	_, _, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"title: [Review Flow\n"+
		"orbit_member:\n"+
		"  name: review\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "frontmatter is invalid YAML")
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

func TestParseMarkdownMemberHintRejectsMetaRole(t *testing.T) {
	t.Parallel()

	_, _, err := parseMarkdownMemberHint("docs/spec.md", []byte(""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-meta\n"+
		"  role: meta\n"+
		"---\n"+
		"\n"+
		"# Spec\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit_member.role: invalid member hint role "meta"`)
}

func TestParseMarkdownMemberHintRejectsDisallowedOrbitMemberFields(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"paths", "scopes", "capabilities", "behavior", "meta", "unsupported"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			_, _, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
				"---\n"+
				"orbit_member:\n"+
				"  name: docs-review\n"+
				"  "+field+": {}\n"+
				"---\n"+
				"\n"+
				"# Review\n"))
			require.Error(t, err)
			require.ErrorContains(t, err, "field "+field+" not found")
		})
	}
}

func TestParseMarkdownMemberHintRejectsNonBootstrapLane(t *testing.T) {
	t.Parallel()

	_, _, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-review\n"+
		"  lane: default\n"+
		"---\n"+
		"\n"+
		"# Review\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit_member.lane must be "bootstrap" when present`)
}

func TestParseMarkdownMemberHintPreservesExplicitFields(t *testing.T) {
	t.Parallel()

	hint, ok, err := parseMarkdownMemberHint("docs/process/review.md", []byte(""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-review\n"+
		"  description: Review workflow\n"+
		"  role: process\n"+
		"  lane: bootstrap\n"+
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
		Description: "Review workflow",
		Role:        OrbitMemberProcess,
		Lane:        OrbitMemberLaneBootstrap,
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

func TestParseDirectoryMemberHintRejectsTopLevelFieldsOutsideOrbitMember(t *testing.T) {
	t.Parallel()

	_, err := parseDirectoryMemberHint("docs/process/.orbit-member.yaml", []byte(""+
		"orbit_member:\n"+
		"  name: docs-process\n"+
		"metadata: docs\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `top-level field "metadata" is not supported`)
}

func TestParseDirectoryMemberHintRejectsFlatMarker(t *testing.T) {
	t.Parallel()

	_, err := parseDirectoryMemberHint("docs/process/.orbit-member.yaml", []byte(""+
		"name: docs-process\n"+
		"description: Documentation review workflow\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "must define nested orbit_member")
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

func TestBackfilledExistingMemberPreservesExplicitScopes(t *testing.T) {
	t.Parallel()

	writeFalse := false
	existing := OrbitMember{
		Name: "review",
		Role: OrbitMemberRule,
		Paths: OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
		},
		Scopes: &OrbitMemberScopePatch{
			Write: &writeFalse,
		},
	}
	hint := resolvedMemberHint{
		Kind:     memberHintKindFileFrontmatter,
		HintPath: "docs/process/review.md",
		RootPath: "docs/process/review.md",
		Name:     "review",
		Role:     OrbitMemberProcess,
	}
	candidate := buildMemberHintCandidate(hint).Member

	next, err := backfilledExistingMember(existing, hint, candidate)
	require.NoError(t, err)
	require.Equal(t, OrbitMemberProcess, next.Role)
	require.Equal(t, existing.Scopes, next.Scopes)
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

func TestConsumeMemberHintPathsRejectsFlatMarkdownMetadataWithoutMutation(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "docs", "rules"), 0o755))

	stylePath := filepath.Join(repoRoot, "docs", "rules", "style.md")
	styleBefore := "" +
		"---\n" +
		"name: docs-style\n" +
		"description: Ordinary document metadata\n" +
		"---\n" +
		"\n" +
		"# Style\n"
	require.NoError(t, os.WriteFile(stylePath, []byte(styleBefore), 0o644))

	_, err := ConsumeMemberHintPaths(repoRoot, []string{"docs/rules/style.md"})
	require.Error(t, err)
	require.ErrorContains(t, err, "frontmatter does not define orbit_member")

	styleAfter, err := os.ReadFile(stylePath)
	require.NoError(t, err)
	require.Equal(t, styleBefore, string(styleAfter))
}
