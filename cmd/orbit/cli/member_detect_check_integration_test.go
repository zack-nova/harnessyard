package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestMemberDetectReportsMatchExistingAndCreateNewJSON(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
		{
			Name: "docs-rules",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/style.md"},
			},
		},
	}, map[string]string{
		"docs/rules/style.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-rules\n" +
			"  role: rule\n" +
			"---\n" +
			"\n" +
			"# Style\n",
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot     string `json:"repo_root"`
		OrbitID      string `json:"orbit_id"`
		RevisionKind string `json:"revision_kind"`
		HintCount    int    `json:"hint_count"`
		Hints        []struct {
			Kind         string   `json:"kind"`
			HintPath     string   `json:"hint_path"`
			RootPath     string   `json:"root_path"`
			ResolvedName string   `json:"resolved_name"`
			ResolvedRole string   `json:"resolved_role"`
			Action       string   `json:"action"`
			TargetName   string   `json:"target_name"`
			Diagnostics  []string `json:"diagnostics"`
		} `json:"hints"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.Equal(t, 2, payload.HintCount)
	require.Len(t, payload.Hints, 2)

	require.Equal(t, "create_new", payload.Hints[0].Action)
	require.Equal(t, "docs/process/review.md", payload.Hints[0].HintPath)
	require.Equal(t, "review", payload.Hints[0].ResolvedName)
	require.Equal(t, "rule", payload.Hints[0].ResolvedRole)
	require.Empty(t, payload.Hints[0].TargetName)
	require.Empty(t, payload.Hints[0].Diagnostics)

	require.Equal(t, "match_existing", payload.Hints[1].Action)
	require.Equal(t, "docs/rules/style.md", payload.Hints[1].HintPath)
	require.Equal(t, "docs-rules", payload.Hints[1].ResolvedName)
	require.Equal(t, "docs-rules", payload.Hints[1].TargetName)
	require.Empty(t, payload.Hints[1].Diagnostics)
}

func TestMemberDetectDefaultsToSourceBranchOrbit(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		RevisionKind string `json:"revision_kind"`
		HintCount    int    `json:"hint_count"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.Equal(t, 1, payload.HintCount)
}

func TestMemberBackfillCheckDefaultsToOrbitTemplateBranchOrbit(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "orbit_template", nil, map[string]string{
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID         string `json:"orbit_id"`
		RevisionKind    string `json:"revision_kind"`
		DriftDetected   bool   `json:"drift_detected"`
		BackfillAllowed bool   `json:"backfill_allowed"`
		HintCount       int    `json:"hint_count"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit_template", payload.RevisionKind)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Equal(t, 1, payload.HintCount)
}

func TestMemberDetectReportsInvalidHintAndConflictJSON(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "orbit_template", []orbitpkg.OrbitMember{
		{
			Name: "docs-process",
			Role: orbitpkg.OrbitMemberProcess,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/process/**", "README.md"},
			},
		},
	}, map[string]string{
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  name: docs-process\n",
		"docs/broken.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  role: outside\n" +
			"---\n" +
			"\n" +
			"# Broken\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RevisionKind string `json:"revision_kind"`
		HintCount    int    `json:"hint_count"`
		Hints        []struct {
			Kind         string   `json:"kind"`
			HintPath     string   `json:"hint_path"`
			ResolvedName string   `json:"resolved_name"`
			Action       string   `json:"action"`
			TargetName   string   `json:"target_name"`
			Diagnostics  []string `json:"diagnostics"`
		} `json:"hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, "orbit_template", payload.RevisionKind)
	require.Equal(t, 2, payload.HintCount)
	require.Len(t, payload.Hints, 2)

	first := payload.Hints[0]
	require.Equal(t, "invalid_hint", first.Action)
	require.Equal(t, "docs/broken.md", first.HintPath)
	require.Equal(t, "file_frontmatter", first.Kind)
	require.Contains(t, first.Diagnostics, `orbit_member.role: invalid orbit member role "outside"`)

	second := payload.Hints[1]
	require.Equal(t, "merge_existing", second.Action)
	require.Equal(t, "docs/process/.orbit-member.yaml", second.HintPath)
	require.Equal(t, "docs-process", second.ResolvedName)
	require.Equal(t, "docs-process", second.TargetName)
	require.Empty(t, second.Diagnostics)
}

func TestMemberBackfillCheckReportsDriftWithoutWritingTruth(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	})

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot        string `json:"repo_root"`
		OrbitID         string `json:"orbit_id"`
		RevisionKind    string `json:"revision_kind"`
		DriftDetected   bool   `json:"drift_detected"`
		BackfillAllowed bool   `json:"backfill_allowed"`
		HintCount       int    `json:"hint_count"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Equal(t, 1, payload.HintCount)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "process", "review.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestMemberBackfillCheckReportsDuplicateNameConflictJSON(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: shared-docs\n" +
			"---\n" +
			"\n" +
			"# Review\n",
		"docs/rules/style.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: shared-docs\n" +
			"---\n" +
			"\n" +
			"# Style\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DriftDetected   bool `json:"drift_detected"`
		BackfillAllowed bool `json:"backfill_allowed"`
		HintCount       int  `json:"hint_count"`
		Hints           []struct {
			Action       string   `json:"action"`
			ResolvedName string   `json:"resolved_name"`
			Diagnostics  []string `json:"diagnostics"`
		} `json:"hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.True(t, payload.DriftDetected)
	require.False(t, payload.BackfillAllowed)
	require.Equal(t, 2, payload.HintCount)
	require.Len(t, payload.Hints, 2)

	for _, hint := range payload.Hints {
		require.Equal(t, "conflict", hint.Action)
		require.Equal(t, "shared-docs", hint.ResolvedName)
		require.Contains(
			t,
			hint.Diagnostics,
			`resolved member name "shared-docs" is declared by multiple hints: docs/process/review.md, docs/rules/style.md`,
		)
	}
}

func TestMemberBackfillCheckScansUntrackedWorktreeHints(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, nil)
	repo.WriteFile(t, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  description: Review workflow\n"+
		"---\n"+
		"\n"+
		"# Review\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DriftDetected   bool `json:"drift_detected"`
		BackfillAllowed bool `json:"backfill_allowed"`
		HintCount       int  `json:"hint_count"`
		Hints           []struct {
			Action       string `json:"action"`
			HintPath     string `json:"hint_path"`
			ResolvedName string `json:"resolved_name"`
		} `json:"hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Equal(t, 1, payload.HintCount)
	require.Len(t, payload.Hints, 1)
	require.Equal(t, "create_new", payload.Hints[0].Action)
	require.Equal(t, "docs/process/review.md", payload.Hints[0].HintPath)
	require.Equal(t, "review", payload.Hints[0].ResolvedName)
}

func TestMemberBackfillCheckExcludesCapabilityHintCandidates(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, nil)
	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	spec.Capabilities = &orbitpkg.OrbitCapabilities{
		Commands: &orbitpkg.OrbitCommandCapabilityPaths{
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"tools/commands/**/*.md"},
			},
		},
		Skills: &orbitpkg.OrbitSkillCapabilities{
			Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"tools/skills/*"},
				},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "commands/docs/review.md", ""+
		"---\n"+
		"name: command-review\n"+
		"description: This is a command, not a member\n"+
		"---\n"+
		"\n"+
		"# Command\n")
	repo.WriteFile(t, "skills/docs/frontend/SKILL.md", ""+
		"---\n"+
		"name: frontend\n"+
		"description: Frontend skill\n"+
		"---\n"+
		"\n"+
		"# Skill\n")
	repo.WriteFile(t, "tools/commands/check.md", ""+
		"---\n"+
		"name: custom-command\n"+
		"description: Custom command path\n"+
		"---\n"+
		"\n"+
		"# Custom Command\n")
	repo.WriteFile(t, "tools/skills/review/SKILL.md", ""+
		"---\n"+
		"name: review\n"+
		"description: Review skill\n"+
		"---\n"+
		"\n"+
		"# Review Skill\n")
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Style guide\n"+
		"---\n"+
		"\n"+
		"# Style\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HintCount int `json:"hint_count"`
		Hints     []struct {
			Action       string `json:"action"`
			HintPath     string `json:"hint_path"`
			ResolvedName string `json:"resolved_name"`
		} `json:"hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, 1, payload.HintCount)
	require.Len(t, payload.Hints, 1)
	require.Equal(t, "create_new", payload.Hints[0].Action)
	require.Equal(t, "docs/rules/style.md", payload.Hints[0].HintPath)
	require.Equal(t, "docs-style", payload.Hints[0].ResolvedName)
}

func TestMemberBackfillCheckReportsComplexMemberIncludeAppend(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/**"},
				Exclude: []string{"docs/archive/**"},
			},
		},
	}, nil)
	repo.WriteFile(t, "docs/new.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-content\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# New\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DriftDetected   bool `json:"drift_detected"`
		BackfillAllowed bool `json:"backfill_allowed"`
		Hints           []struct {
			Action      string   `json:"action"`
			HintPath    string   `json:"hint_path"`
			Diagnostics []string `json:"diagnostics"`
		} `json:"hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.True(t, payload.DriftDetected)
	require.True(t, payload.BackfillAllowed)
	require.Len(t, payload.Hints, 1)
	require.Equal(t, "append_include", payload.Hints[0].Action)
	require.Equal(t, "docs/new.md", payload.Hints[0].HintPath)
	require.Contains(t, payload.Hints[0].Diagnostics, `will add include path "docs/new.md" to existing member "docs-content"`)
}

func TestMemberDetectWorksWithoutCommittedHeadForSourceRevision(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepoWithCommitState(t, "source", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	}, false)

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	require.Equal(t, "source", raw["revision_kind"])
	hintCount, ok := raw["hint_count"].(float64)
	require.True(t, ok)
	require.InDelta(t, 1, hintCount, 0)

	hints, ok := raw["hints"].([]any)
	require.True(t, ok)
	require.Len(t, hints, 1)

	hint, ok := hints[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "create_new", hint["action"])
	require.Equal(t, "docs/process/review.md", hint["hint_path"])
	require.Equal(t, "review", hint["resolved_name"])
}

func TestMemberBackfillCheckWorksWithoutCommittedHeadForTemplateRevision(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepoWithCommitState(t, "orbit_template", nil, map[string]string{
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
	}, false)

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	require.Equal(t, "orbit_template", raw["revision_kind"])
	require.Equal(t, true, raw["drift_detected"])
	require.Equal(t, true, raw["backfill_allowed"])
	hintCount, ok := raw["hint_count"].(float64)
	require.True(t, ok)
	require.InDelta(t, 1, hintCount, 0)
}

func TestMemberDetectRejectsOrbitMismatchedToCurrentSourceManifest(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	})

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: plan\n"+
		"  source_branch: main\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `current source manifest hosts orbit "plan"; requested --orbit "docs" does not match`)
}

func TestMemberBackfillCheckRejectsOrbitMismatchedToCurrentTemplateManifest(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "orbit_template", nil, map[string]string{
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
	})

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: plan\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-04-07T00:00:00Z\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `current orbit_template manifest hosts orbit "plan"; requested --orbit "docs" does not match`)
}

func TestMemberDetectRejectsUnsupportedRevisionKinds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		revisionKind string
	}{
		{name: "runtime", revisionKind: "runtime"},
		{name: "harness template", revisionKind: "harness_template"},
		{name: "plain", revisionKind: ""},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := seedMemberHintRevisionRepo(t, tc.revisionKind, nil, map[string]string{
				"docs/process/review.md": "" +
					"---\n" +
					"orbit_member:\n" +
					"  description: Review workflow\n" +
					"---\n" +
					"\n" +
					"# Review\n",
			})

			stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--orbit", "docs", "--json")
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, `member detect supports only source or orbit_template revisions; current revision kind is "`)
		})
	}
}

func TestMemberDetectWithoutOrbitStillRejectsRuntimeBeforeCurrentOrbitFallback(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "runtime", nil, map[string]string{
		"docs/process/review.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: Review workflow\n" +
			"---\n" +
			"\n" +
			"# Review\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "detect", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `member detect supports only source or orbit_template revisions; current revision kind is "runtime"`)
}

func TestMemberBackfillCheckRejectsUnsupportedRevisionKinds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		revisionKind string
	}{
		{name: "runtime", revisionKind: "runtime"},
		{name: "harness template", revisionKind: "harness_template"},
		{name: "plain", revisionKind: ""},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := seedMemberHintRevisionRepo(t, tc.revisionKind, nil, map[string]string{
				"docs/process/review.md": "" +
					"---\n" +
					"orbit_member:\n" +
					"  description: Review workflow\n" +
					"---\n" +
					"\n" +
					"# Review\n",
			})

			stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--check", "--json")
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, `member backfill supports only source or orbit_template revisions; current revision kind is "`)
		})
	}
}

func seedMemberHintRevisionRepo(t *testing.T, revisionKind string, members []orbitpkg.OrbitMember, files map[string]string) *testutil.Repo {
	t.Helper()

	return seedMemberHintRevisionRepoWithCommitState(t, revisionKind, members, files, true)
}

func seedMemberHintRevisionRepoWithCommitState(
	t *testing.T,
	revisionKind string,
	members []orbitpkg.OrbitMember,
	files map[string]string,
	commit bool,
) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	spec.Members = append([]orbitpkg.OrbitMember(nil), members...)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	switch revisionKind {
	case "runtime":
		repo.WriteFile(t, ".harness/manifest.yaml", ""+
			"schema_version: 1\n"+
			"kind: runtime\n"+
			"runtime:\n"+
			"  id: workspace\n"+
			"  created_at: 2026-04-07T00:00:00Z\n"+
			"  updated_at: 2026-04-07T00:00:00Z\n"+
			"members: []\n")
	case "source":
		repo.WriteFile(t, ".harness/manifest.yaml", ""+
			"schema_version: 1\n"+
			"kind: source\n"+
			"source:\n"+
			"  orbit_id: docs\n"+
			"  source_branch: main\n")
	case "orbit_template":
		repo.WriteFile(t, ".harness/manifest.yaml", ""+
			"schema_version: 1\n"+
			"kind: orbit_template\n"+
			"template:\n"+
			"  orbit_id: docs\n"+
			"  created_from_branch: main\n"+
			"  created_from_commit: abc123\n"+
			"  created_at: 2026-04-07T00:00:00Z\n")
	case "harness_template":
		repo.WriteFile(t, ".harness/manifest.yaml", ""+
			"schema_version: 1\n"+
			"kind: harness_template\n"+
			"template:\n"+
			"  harness_id: workspace\n"+
			"  created_from_branch: main\n"+
			"  created_from_commit: abc123\n"+
			"  created_at: 2026-04-07T00:00:00Z\n"+
			"members:\n"+
			"  - orbit_id: docs\n"+
			"includes_root_agents: false\n")
	}

	for path, content := range files {
		repo.WriteFile(t, path, content)
	}

	if commit {
		repo.AddAndCommit(t, "seed member hint revision repo")
	} else {
		repo.Run(t, "add", "-A")
	}

	return repo
}
