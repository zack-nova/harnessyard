package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

type hyardLayoutOptimizePayload struct {
	SchemaVersion  string                  `json:"schema_version"`
	RepoRoot       string                  `json:"repo_root"`
	Mode           string                  `json:"mode"`
	RepositoryMode string                  `json:"repository_mode"`
	MovePlan       hyardLayoutOptimizePlan `json:"move_plan"`
}

type hyardLayoutOptimizePlan struct {
	OrbitID   string                       `json:"orbit_id"`
	Moves     []hyardLayoutOptimizeMove    `json:"moves"`
	Conflicts []hyardLayoutOptimizeProblem `json:"conflicts"`
	Warnings  []hyardLayoutOptimizeProblem `json:"warnings"`
}

type hyardLayoutOptimizeMove struct {
	From                 string                       `json:"from"`
	To                   string                       `json:"to"`
	Reason               string                       `json:"reason"`
	AffectedTruthUpdates []hyardLayoutTruthUpdate     `json:"affected_truth_updates"`
	Conflicts            []hyardLayoutOptimizeProblem `json:"conflicts"`
	Warnings             []hyardLayoutOptimizeProblem `json:"warnings"`
}

type hyardLayoutTruthUpdate struct {
	Path  string `json:"path"`
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

type hyardLayoutOptimizeProblem struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

func TestHyardLayoutOptimizeCheckJSONPreviewsOrdinaryRepositoryMovePlan(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Follow docs/runbook.md before changing release flow.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed ordinary repository assets")

	beforeStatus := repo.Run(t, "status", "--short")
	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))
	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "check", payload.Mode)
	require.Equal(t, "ordinary_repository", payload.RepositoryMode)
	require.Equal(t, "docs", payload.MovePlan.OrbitID)
	require.Empty(t, payload.MovePlan.Conflicts)

	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   ".codex/skills/frontend-test-lab",
		To:     "skills/docs/frontend-test-lab",
		Reason: "local_skill_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "capabilities.skills.local.paths.include",
				From:  ".codex/skills/frontend-test-lab",
				To:    "skills/docs/frontend-test-lab",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   ".codex/prompts/review.md",
		To:     "commands/docs/review.md",
		Reason: "prompt_command_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "capabilities.commands.paths.include",
				From:  ".codex/prompts/review.md",
				To:    "commands/docs/review.md",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   "docs/runbook.md",
		To:     "guidance/docs/runbook.md",
		Reason: "referenced_guidance_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "members[].paths.include",
				From:  "docs/runbook.md",
				To:    "guidance/docs/runbook.md",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
}

func TestHyardLayoutOptimizeCheckJSONPreviewsExistingRuntimeTruth(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "docs/runbook.md", "# Runtime runbook\n")
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Capabilities = &orbitpkg.OrbitCapabilities{
		Commands: &orbitpkg.OrbitCommandCapabilityPaths{
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{".codex/prompts/*.md"},
			},
		},
		Skills: &orbitpkg.OrbitSkillCapabilities{
			Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{".codex/skills/*"},
				},
			},
		},
	}
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "runbook",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/runbook.md"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed runtime truth outside recommended positions")

	beforeStatus := repo.Run(t, "status", "--short")
	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, beforeStatus, repo.Run(t, "status", "--short"))

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness_runtime", payload.RepositoryMode)
	require.Equal(t, "docs", payload.MovePlan.OrbitID)
	require.Empty(t, payload.MovePlan.Conflicts)
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   ".codex/skills/frontend-test-lab",
		To:     "skills/docs/frontend-test-lab",
		Reason: "local_skill_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "capabilities.skills.local.paths.include",
				From:  ".codex/skills/frontend-test-lab",
				To:    "skills/docs/frontend-test-lab",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   ".codex/prompts/review.md",
		To:     "commands/docs/review.md",
		Reason: "prompt_command_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "capabilities.commands.paths.include",
				From:  ".codex/prompts/review.md",
				To:    "commands/docs/review.md",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   "docs/runbook.md",
		To:     "guidance/docs/runbook.md",
		Reason: "member_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "members[].paths.include",
				From:  "docs/runbook.md",
				To:    "guidance/docs/runbook.md",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
}

func TestHyardLayoutOptimizeCheckJSONReportsConflictsAndWarnings(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Review docs/draft.md before adopting.\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	writeHyardAdoptSkill(t, repo, "skills/docs/frontend-test-lab", "frontend-test-lab", "Existing recommended destination")
	repo.AddAndCommit(t, "seed conflicting skill destination", "AGENTS.md", ".codex/skills", "skills/docs")
	repo.WriteFile(t, "docs/draft.md", "# Draft guidance\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.MovePlan.Conflicts, hyardLayoutOptimizeProblem{
		Code:    "destination_exists",
		Path:    "skills/docs/frontend-test-lab",
		Message: "recommended destination already exists",
	})
	require.Contains(t, payload.MovePlan.Warnings, hyardLayoutOptimizeProblem{
		Code:    "referenced_guidance_untracked",
		Path:    "docs/draft.md",
		Message: "referenced guidance path is untracked and will not be adopted",
	})

	skillMove := requireHyardLayoutMoveFrom(t, payload.MovePlan.Moves, ".codex/skills/frontend-test-lab")
	require.Contains(t, skillMove.Conflicts, hyardLayoutOptimizeProblem{
		Code:    "destination_exists",
		Path:    "skills/docs/frontend-test-lab",
		Message: "recommended destination already exists",
	})
}

func requireHyardLayoutMoveFrom(t *testing.T, moves []hyardLayoutOptimizeMove, from string) hyardLayoutOptimizeMove {
	t.Helper()

	for _, move := range moves {
		if move.From == from {
			return move
		}
	}
	require.Failf(t, "missing layout move", "from path %q not found in %#v", from, moves)

	return hyardLayoutOptimizeMove{}
}
