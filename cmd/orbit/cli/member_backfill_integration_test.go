package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestMemberBackfillWritesSpecAndConsumesHintsJSON(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
		{
			Name:        "docs-rules",
			Description: "Old rules",
			Role:        orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/style.md"},
			},
		},
	}, map[string]string{
		"docs/rules/style.md": "" +
			"---\n" +
			"title: Style Guide\n" +
			"orbit_member:\n" +
			"  name: docs-rules\n" +
			"  description: Style rules\n" +
			"---\n" +
			"\n" +
			"# Style\n",
		"docs/process/.orbit-member.yaml": "" +
			"orbit_member:\n" +
			"  description: Review workflow\n",
		"docs/note.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  description: One-off note\n" +
			"---\n" +
			"\n" +
			"# Note\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string   `json:"repo_root"`
		OrbitID            string   `json:"orbit_id"`
		RevisionKind       string   `json:"revision_kind"`
		DefinitionPath     string   `json:"definition_path"`
		UpdatedMemberCount int      `json:"updated_member_count"`
		UpdatedMembers     []string `json:"updated_members"`
		ConsumedHintCount  int      `json:"consumed_hint_count"`
		ConsumedHints      []string `json:"consumed_hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "source", payload.RevisionKind)
	requireSamePath(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), payload.DefinitionPath)
	require.Equal(t, 3, payload.UpdatedMemberCount)
	require.Equal(t, 3, payload.ConsumedHintCount)
	require.ElementsMatch(t, []string{"docs-rules", "note", "process"}, payload.UpdatedMembers)
	require.ElementsMatch(t, []string{"docs/note.md", "docs/process/.orbit-member.yaml", "docs/rules/style.md"}, payload.ConsumedHints)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Len(t, spec.Members, 3)

	rulesMember := memberByName(t, spec.Members, "docs-rules")
	require.Equal(t, "Style rules", rulesMember.Description)
	require.Equal(t, orbitpkg.OrbitMemberRule, rulesMember.Role)
	require.Equal(t, []string{"docs/rules/style.md"}, rulesMember.Paths.Include)

	processMember := memberByName(t, spec.Members, "process")
	require.Equal(t, "Review workflow", processMember.Description)
	require.Equal(t, orbitpkg.OrbitMemberProcess, processMember.Role)
	require.Equal(t, []string{"docs/process/**"}, processMember.Paths.Include)

	noteMember := memberByName(t, spec.Members, "note")
	require.Equal(t, "One-off note", noteMember.Description)
	require.Equal(t, orbitpkg.OrbitMemberRule, noteMember.Role)
	require.Equal(t, []string{"docs/note.md"}, noteMember.Paths.Include)

	styleData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "rules", "style.md"))
	require.NoError(t, err)
	require.NotContains(t, string(styleData), "orbit_member:")
	require.Contains(t, string(styleData), "title: Style Guide")
	require.Contains(t, string(styleData), "# Style\n")

	noteData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "note.md"))
	require.NoError(t, err)
	require.NotContains(t, string(noteData), "orbit_member:")
	require.Equal(t, "\n# Note\n", string(noteData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestMemberBackfillWritesSpecAndConsumesFlatHint(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", nil, map[string]string{
		"docs/rules/style.md": "" +
			"---\n" +
			"name: docs-style\n" +
			"description: Style guide\n" +
			"---\n" +
			"\n" +
			"# Style\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		UpdatedMembers []string `json:"updated_members"`
		ConsumedHints  []string `json:"consumed_hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, []string{"docs-style"}, payload.UpdatedMembers)
	require.Equal(t, []string{"docs/rules/style.md"}, payload.ConsumedHints)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	member := memberByName(t, spec.Members, "docs-style")
	require.Equal(t, "Style guide", member.Description)
	require.Equal(t, orbitpkg.OrbitMemberRule, member.Role)
	require.Equal(t, []string{"docs/rules/style.md"}, member.Paths.Include)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "rules", "style.md"))
	require.NoError(t, err)
	require.Equal(t, "\n# Style\n", string(data))
}

func decodeJSONInto(t *testing.T, stdout string, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(stdout), target))
}

func TestMemberBackfillMergesComplexMemberWithoutReplacingPaths(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
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
			"  name: docs-process\n" +
			"  description: Review workflow\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		UpdatedMembers    []string `json:"updated_members"`
		ConsumedHintCount int      `json:"consumed_hint_count"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, []string{"docs-process"}, payload.UpdatedMembers)
	require.Equal(t, 1, payload.ConsumedHintCount)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	member := memberByName(t, specAfter.Members, "docs-process")
	require.Equal(t, "Review workflow", member.Description)
	require.Equal(t, orbitpkg.OrbitMemberProcess, member.Role)
	require.Equal(t, []string{"docs/process/**", "README.md"}, member.Paths.Include)

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", ".orbit-member.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestMemberBackfillAppendsIncludeForComplexMemberWhenHintPathIsOutsidePaths(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
		{
			Name:        "docs-content",
			Description: "Old docs",
			Role:        orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/**"},
				Exclude: []string{"docs/archive/**"},
			},
		},
	}, map[string]string{
		"docs/new.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-content\n" +
			"  description: Current docs\n" +
			"  role: rule\n" +
			"---\n" +
			"\n" +
			"# New\n",
	})

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		UpdatedMembers []string `json:"updated_members"`
		ConsumedHints  []string `json:"consumed_hints"`
	}
	decodeJSONInto(t, stdout, &payload)
	require.Equal(t, []string{"docs-content"}, payload.UpdatedMembers)
	require.Equal(t, []string{"docs/new.md"}, payload.ConsumedHints)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	member := memberByName(t, spec.Members, "docs-content")
	require.Equal(t, "Current docs", member.Description)
	require.Equal(t, []string{"docs/rules/**", "docs/new.md"}, member.Paths.Include)
	require.Equal(t, []string{"docs/archive/**"}, member.Paths.Exclude)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "new.md"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "orbit_member:")
}

func TestMemberBackfillFailsClosedWhenComplexMemberExcludesHintPath(t *testing.T) {
	t.Parallel()

	repo := seedMemberHintRevisionRepo(t, "source", []orbitpkg.OrbitMember{
		{
			Name: "docs-content",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
				Exclude: []string{"docs/archive/**"},
			},
		},
	}, map[string]string{
		"docs/archive/old.md": "" +
			"---\n" +
			"orbit_member:\n" +
			"  name: docs-content\n" +
			"  role: rule\n" +
			"---\n" +
			"\n" +
			"# Old\n",
	})

	specBefore, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `member hint path "docs/archive/old.md" is excluded by existing member "docs-content"`)

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(repo.Root, "docs", "archive", "old.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func TestMemberBackfillRollsBackSpecWhenHintConsumeFails(t *testing.T) {
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

	processDir := filepath.Join(repo.Root, "docs", "process")
	info, err := os.Stat(processDir)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(processDir, 0o500))
	defer func() {
		require.NoError(t, os.Chmod(processDir, info.Mode().Perm()))
	}()

	stdout, stderr, err := executeCLI(t, repo.Root, "member", "backfill", "--orbit", "docs", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "rollback")

	specAfter, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, specBefore, specAfter)

	data, err := os.ReadFile(filepath.Join(processDir, "review.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "orbit_member:")
}

func memberByName(t *testing.T, members []orbitpkg.OrbitMember, name string) orbitpkg.OrbitMember {
	t.Helper()

	for _, member := range members {
		if member.Name == name {
			return member
		}
	}

	t.Fatalf("member %q not found", name)
	return orbitpkg.OrbitMember{}
}
