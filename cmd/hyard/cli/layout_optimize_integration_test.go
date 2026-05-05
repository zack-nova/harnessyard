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
	Check          *struct {
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
	} `json:"check,omitempty"`
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

func TestHyardLayoutOptimizeYesAppliesExistingRuntimeMovePlan(t *testing.T) {
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

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "apply", payload.Mode)
	require.Equal(t, "harness_runtime", payload.RepositoryMode)
	require.Len(t, payload.MovePlan.Moves, 3)
	require.NotNil(t, payload.Check)
	require.True(t, payload.Check.OK)
	require.Zero(t, payload.Check.FindingCount)

	require.NoDirExists(t, filepath.Join(repo.Root, ".codex", "skills", "frontend-test-lab"))
	require.FileExists(t, filepath.Join(repo.Root, "skills", "docs", "frontend-test-lab", "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "prompts", "review.md"))
	require.FileExists(t, filepath.Join(repo.Root, "commands", "docs", "review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))

	updatedSpec, err := orbitpkg.LoadHostedOrbitSpec(t.Context(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"commands/docs/*.md"}, updatedSpec.Capabilities.Commands.Paths.Include)
	require.Equal(t, []string{"skills/docs/*"}, updatedSpec.Capabilities.Skills.Local.Paths.Include)
	require.Equal(t, []string{"guidance/docs/runbook.md"}, updatedSpec.Members[0].Paths.Include)
}

func TestHyardLayoutOptimizeYesAppliesOrdinaryRepositoryMovePlan(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nRead [the runbook](docs/runbook.md) first.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.WriteFile(t, ".codex/prompts/fix.md", "# Fix prompt\n")
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/backend-lab", "backend-lab", "Backend validation workflow")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed ordinary repository assets")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "apply", payload.Mode)
	require.Equal(t, "ordinary_repository", payload.RepositoryMode)
	require.NotNil(t, payload.Check)
	require.True(t, payload.Check.OK)

	require.FileExists(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoDirExists(t, filepath.Join(repo.Root, ".codex", "skills", "frontend-test-lab"))
	require.FileExists(t, filepath.Join(repo.Root, "skills", "docs", "frontend-test-lab", "SKILL.md"))
	require.NoDirExists(t, filepath.Join(repo.Root, ".codex", "skills", "backend-lab"))
	require.FileExists(t, filepath.Join(repo.Root, "skills", "docs", "backend-lab", "SKILL.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "prompts", "fix.md"))
	require.FileExists(t, filepath.Join(repo.Root, "commands", "docs", "fix.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "prompts", "review.md"))
	require.FileExists(t, filepath.Join(repo.Root, "commands", "docs", "review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))

	updatedSpec, err := orbitpkg.LoadHostedOrbitSpec(t.Context(), repo.Root, "docs")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"skills/docs/backend-lab", "skills/docs/frontend-test-lab"}, updatedSpec.Capabilities.Skills.Local.Paths.Include)
	require.ElementsMatch(t, []string{"commands/docs/fix.md", "commands/docs/review.md"}, updatedSpec.Capabilities.Commands.Paths.Include)
	require.Equal(t, []string{"guidance/docs/runbook.md"}, updatedSpec.Members[0].Paths.Include)

	guidance, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(guidance), "[the runbook](guidance/docs/runbook.md)")
}

func TestHyardLayoutOptimizeApplyBlocksConflictsWithoutMoving(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	writeHyardAdoptSkill(t, repo, "skills/docs/frontend-test-lab", "frontend-test-lab", "Existing recommended destination")
	repo.AddAndCommit(t, "seed conflicting skill destination", "AGENTS.md", ".codex/skills", "skills/docs")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "layout optimize blocked by 1 conflict")
	require.ErrorContains(t, err, "destination_exists")
	require.DirExists(t, filepath.Join(repo.Root, ".codex", "skills", "frontend-test-lab"))
	require.DirExists(t, filepath.Join(repo.Root, "skills", "docs", "frontend-test-lab"))
}

func TestHyardLayoutOptimizeApplyBlocksMissingFilesWithoutMoving(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "missing-runbook",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/missing.md"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed missing runtime member truth")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "source_missing")
	require.NoFileExists(t, filepath.Join(repo.Root, "guidance", "docs", "missing.md"))
}

func TestHyardLayoutOptimizeCheckReportsOverlappingMoves(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "docs/runbook.md", "# Runtime runbook\n")
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "docs-directory",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
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
	repo.AddAndCommit(t, "seed overlapping runtime member truth")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.MovePlan.Conflicts, hyardLayoutOptimizeProblem{
		Code:    "overlapping_moves",
		Path:    "docs/runbook.md",
		Message: "multiple recommendations move overlapping source paths",
	})
}

func TestHyardLayoutOptimizeApplyBlocksUnsafePathRewriteWithoutMoving(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Capabilities = &orbitpkg.OrbitCapabilities{
		Commands: &orbitpkg.OrbitCommandCapabilityPaths{
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{".codex/**/*.md"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed broad command capability path")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "unsafe_path_rewrite")
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "prompts", "review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "commands", "docs", "review.md"))
}

func TestHyardLayoutOptimizeInteractiveConfirmationAppliesMoves(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "docs/runbook.md", "# Runtime runbook\n")
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
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
	repo.AddAndCommit(t, "seed runtime guidance outside recommended position")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "y\n", "layout", "optimize")
	require.NoError(t, err)
	require.Contains(t, stdout, "layout optimize harness_runtime apply: 1 moves")
	require.Contains(t, stderr, "Apply layout optimization moves? [y/N]")
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))
}

func TestHyardLayoutOptimizeYesUpdatesHookHandlerPathTruth(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "scripts/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")

	exportScope := true
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.AgentAddons = &orbitpkg.OrbitAgentAddons{
		Hooks: &orbitpkg.OrbitAgentHookAddons{
			UnsupportedBehavior: "skip",
			Entries: []orbitpkg.OrbitAgentHookEntry{
				{
					ID:    "block-dangerous-shell",
					Event: orbitpkg.AgentAddonHookEvent{Kind: "tool.before"},
					Match: orbitpkg.AgentAddonHookMatch{Tools: []string{"shell"}},
					Handler: orbitpkg.AgentAddonHookHandler{
						Type: "command",
						Path: "scripts/block-dangerous-shell/run.sh",
					},
					Targets: map[string]bool{"codex": true},
				},
			},
		},
	}
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "hook-handler",
			Role: orbitpkg.OrbitMemberProcess,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"scripts/block-dangerous-shell/run.sh"},
			},
			Scopes: &orbitpkg.OrbitMemberScopePatch{
				Export: &exportScope,
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed hook handler outside recommended position")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardLayoutOptimizePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.MovePlan.Moves, hyardLayoutOptimizeMove{
		From:   "scripts/block-dangerous-shell/run.sh",
		To:     "hooks/docs/block-dangerous-shell/run.sh",
		Reason: "hook_handler_recommended_position",
		AffectedTruthUpdates: []hyardLayoutTruthUpdate{
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "agent_addons.hooks.entries[].handler.path",
				From:  "scripts/block-dangerous-shell/run.sh",
				To:    "hooks/docs/block-dangerous-shell/run.sh",
			},
			{
				Path:  ".harness/orbits/docs.yaml",
				Field: "members[].paths.include",
				From:  "scripts/block-dangerous-shell/run.sh",
				To:    "hooks/docs/block-dangerous-shell/run.sh",
			},
		},
		Conflicts: []hyardLayoutOptimizeProblem{},
		Warnings:  []hyardLayoutOptimizeProblem{},
	})
	require.NoFileExists(t, filepath.Join(repo.Root, "scripts", "block-dangerous-shell", "run.sh"))
	require.FileExists(t, filepath.Join(repo.Root, "hooks", "docs", "block-dangerous-shell", "run.sh"))

	updatedSpec, err := orbitpkg.LoadHostedOrbitSpec(t.Context(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", updatedSpec.AgentAddons.Hooks.Entries[0].Handler.Path)
	require.Equal(t, []string{"hooks/docs/block-dangerous-shell/run.sh"}, updatedSpec.Members[0].Paths.Include)
}

func TestHyardLayoutOptimizeYesUpdatesGuidanceLinks(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nRead [the runbook](docs/runbook.md) first.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runtime runbook\n")
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
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
	repo.AddAndCommit(t, "seed linked guidance outside recommended position")

	_, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	guidance, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(guidance), "[the runbook](guidance/docs/runbook.md)")
	require.NotContains(t, string(guidance), "(docs/runbook.md)")
}

func TestHyardLayoutOptimizeYesAppliesMultipleRuntimeMemberMoves(t *testing.T) {
	t.Parallel()

	repo := seedCommittedHyardRuntimeRepo(t)
	repo.WriteFile(t, "docs/runbook.md", "# Runtime runbook\n")
	repo.WriteFile(t, "docs/playbook.md", "# Runtime playbook\n")
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Members = []orbitpkg.OrbitMember{
		{
			Name: "runbook",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/runbook.md"},
			},
		},
		{
			Name: "playbook",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/playbook.md"},
			},
		},
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed multiple runtime guidance files outside recommended position")

	_, stderr, err := executeHyardCLI(t, repo.Root, "layout", "optimize", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "playbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "playbook.md"))

	updatedSpec, err := orbitpkg.LoadHostedOrbitSpec(t.Context(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"guidance/docs/runbook.md"}, updatedSpec.Members[0].Paths.Include)
	require.Equal(t, []string{"guidance/docs/playbook.md"}, updatedSpec.Members[1].Paths.Include)
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
