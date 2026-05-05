package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

type hyardAdoptCheckPayload struct {
	SchemaVersion          string                       `json:"schema_version"`
	RepoRoot               string                       `json:"repo_root"`
	Mode                   string                       `json:"mode"`
	Adoptable              bool                         `json:"adoptable"`
	ExistingHarnessRuntime bool                         `json:"existing_harness_runtime"`
	DirtyWorktree          hyardAdoptCheckDirtyWorktree `json:"dirty_worktree"`
	AdoptedOrbit           hyardAdoptCheckAdoptedOrbit  `json:"adopted_orbit"`
	Frameworks             hyardAdoptCheckFrameworks    `json:"frameworks"`
	Candidates             []hyardAdoptCheckCandidate   `json:"candidates"`
	Diagnostics            []hyardAdoptCheckDiagnostic  `json:"diagnostics"`
	NextActions            []hyardAdoptCheckNextAction  `json:"next_actions"`
}

type hyardAdoptCheckDirtyWorktree struct {
	Dirty bool     `json:"dirty"`
	Paths []string `json:"paths"`
}

type hyardAdoptCheckAdoptedOrbit struct {
	ID          string `json:"id"`
	DerivedFrom string `json:"derived_from"`
}

type hyardAdoptCheckFrameworks struct {
	Recommended string                     `json:"recommended,omitempty"`
	Detected    []hyardAdoptCheckFramework `json:"detected"`
	Unsupported []hyardAdoptCheckFramework `json:"unsupported,omitempty"`
}

type hyardAdoptCheckFramework struct {
	ID       string                    `json:"id"`
	Status   string                    `json:"status"`
	Evidence []hyardAdoptCheckEvidence `json:"evidence"`
}

type hyardAdoptCheckEvidence struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

type hyardAdoptCheckCandidate struct {
	Path                  string                          `json:"path"`
	Kind                  string                          `json:"kind"`
	Shape                 string                          `json:"shape"`
	RecommendedMemberRole string                          `json:"recommended_member_role,omitempty"`
	RoleConfirmation      hyardAdoptCheckRoleConfirmation `json:"role_confirmation,omitempty"`
	Evidence              []hyardAdoptCheckEvidence       `json:"evidence"`
}

type hyardAdoptCheckRoleConfirmation struct {
	Required               bool     `json:"required"`
	BatchAcceptRecommended bool     `json:"batch_accept_recommended"`
	EditableRoles          []string `json:"editable_roles,omitempty"`
}

type hyardAdoptCheckDiagnostic struct {
	Code     string                    `json:"code"`
	Severity string                    `json:"severity"`
	Message  string                    `json:"message"`
	Evidence []hyardAdoptCheckEvidence `json:"evidence,omitempty"`
}

type hyardAdoptCheckNextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type hyardAdoptWritePayload struct {
	SchemaVersion string                              `json:"schema_version"`
	RepoRoot      string                              `json:"repo_root"`
	Mode          string                              `json:"mode"`
	AdoptedOrbit  hyardAdoptCheckAdoptedOrbit         `json:"adopted_orbit"`
	WrittenPaths  []string                            `json:"written_paths"`
	AgentConfig   *harnesspkg.AgentConfigImportResult `json:"agent_config_import,omitempty"`
	Validations   []hyardAdoptWriteValidation         `json:"validations"`
	Check         harnesspkg.CheckResult              `json:"check"`
	Readiness     harnesspkg.ReadinessReport          `json:"readiness"`
	NextActions   []hyardAdoptCheckNextAction         `json:"next_actions"`
}

type hyardAdoptWriteValidation struct {
	Target string `json:"target"`
	OK     bool   `json:"ok"`
}

func TestHyardAdoptInteractiveOverridesDefaultOrbitIDBeforeWriting(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "My Runtime Repo!")
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, "AGENTS.md"),
		[]byte("# Agent guidance\n\nAdopt this repository.\n"),
		0o600,
	))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed root guidance")

	stdout, stderr, err := executeHyardCLIWithInput(t, repoRoot, "docs\ny\n", "adopt")
	require.NoError(t, err)
	require.Contains(t, stderr, "Adopted Orbit id [my-runtime-repo]:")
	require.Contains(t, stderr, "Write Adoption changes? [y/N]")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repoRoot, "docs")
	require.NoError(t, err)
	require.Equal(t, "docs", spec.ID)
	require.NoFileExists(t, filepath.Join(repoRoot, ".harness", "orbits", "my-runtime-repo.yaml"))
}

func TestHyardAdoptInteractiveAcceptsRecommendedGuidanceRoleAndDeclinesMove(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Follow [the runbook](docs/runbook.md) during Adoption.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.AddAndCommit(t, "seed referenced guidance")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\ny\nn\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Accept all recommended candidate roles? [Y/n]")
	require.Contains(t, stderr, "Apply layout move docs/runbook.md -> guidance/docs/runbook.md? [y/N]")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name: "runbook",
		Role: orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/runbook.md"},
		},
	})
	require.FileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))
}

func TestHyardAdoptInteractiveAcceptsLayoutMoveAndUpdatesTruthAndGuidanceLinks(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Follow [the runbook](docs/runbook.md) during Adoption.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.AddAndCommit(t, "seed movable referenced guidance")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\ny\ny\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply layout move docs/runbook.md -> guidance/docs/runbook.md? [y/N]")
	require.Contains(t, stdout, "written: guidance/docs/runbook.md\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name: "runbook",
		Role: orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"guidance/docs/runbook.md"},
		},
	})
	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "runbook.md"))
	require.FileExists(t, filepath.Join(repo.Root, "guidance", "docs", "runbook.md"))

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "[the runbook](guidance/docs/runbook.md)")
}

func TestHyardAdoptInteractiveEditsCandidateRolesAndIgnoresCandidates(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Read CONTEXT.md and docs/runbook.md before making changes.\n")
	repo.WriteFile(t, "CONTEXT.md", "# Project language\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.AddAndCommit(t, "seed role-edit guidance")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\nn\nsubject\nignore\nn\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Role for CONTEXT.md [rule, subject, process, ignore] (rule):")
	require.Contains(t, stderr, "Role for docs/runbook.md [rule, subject, process, ignore] (rule):")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name: "context",
		Role: orbitpkg.OrbitMemberSubject,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"CONTEXT.md"},
		},
	})
	for _, member := range spec.Members {
		require.NotContains(t, member.Paths.Include, "docs/runbook.md")
	}
}

func TestHyardAdoptInteractiveExcludesDeclinedLocalSkillCapability(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nUse selected local skills only.\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed local skill candidate")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\nn\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Adopt .codex/skills/frontend-test-lab as local skill capability? [Y/n]")
	require.NotContains(t, stderr, "Apply layout move .codex/skills/frontend-test-lab -> skills/docs/frontend-test-lab?")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	if spec.Capabilities != nil && spec.Capabilities.Skills != nil && spec.Capabilities.Skills.Local != nil {
		require.NotContains(t, spec.Capabilities.Skills.Local.Paths.Include, ".codex/skills/frontend-test-lab")
	}
}

func TestHyardAdoptInteractiveDeclinedPromptCommandMoveKeepsCapabilityAtCurrentPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nKeep command prompts reviewable.\n")
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	repo.AddAndCommit(t, "seed prompt command candidate")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\ny\nn\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Adopt .codex/prompts/review.md as prompt command capability? [Y/n]")
	require.Contains(t, stderr, "Apply layout move .codex/prompts/review.md -> commands/docs/review.md? [y/N]")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Capabilities)
	require.NotNil(t, spec.Capabilities.Commands)
	require.Equal(t, []string{".codex/prompts/review.md"}, spec.Capabilities.Commands.Paths.Include)
	require.FileExists(t, filepath.Join(repo.Root, ".codex", "prompts", "review.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "commands", "docs", "review.md"))
}

func TestHyardAdoptInteractiveIgnoresCodexHookHandlerCandidateWithoutWritingHookTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nReview hook candidates.\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.AddAndCommit(t, "seed ignorable hook candidate")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\nn\nignore\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Role for hooks/block-dangerous-shell/run.sh [process, subject, ignore] (process):")
	require.NotContains(t, stderr, "Apply layout move hooks/block-dangerous-shell/run.sh -> hooks/docs/block-dangerous-shell/run.sh?")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.False(t, hyardAdoptSpecHasMemberPath(
		spec,
		"hook-handler-block-dangerous-shell",
		orbitpkg.OrbitMemberProcess,
		"hooks/block-dangerous-shell/run.sh",
	))

	config, err := harnesspkg.LoadAgentUnifiedConfigFile(repo.Root)
	if err == nil {
		require.Empty(t, config.Hooks.Entries)
	} else {
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestHyardAdoptInteractiveEditsCodexHookHandlerRole(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nReview hook roles.\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.AddAndCommit(t, "seed hook role edit candidate")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\nn\nsubject\nn\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Role for hooks/block-dangerous-shell/run.sh [process, subject, ignore] (process):")
	require.Contains(t, stdout, "adopted_orbit: docs\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.True(t, hyardAdoptSpecHasMemberPath(
		spec,
		"hook-handler-block-dangerous-shell",
		orbitpkg.OrbitMemberSubject,
		"hooks/block-dangerous-shell/run.sh",
	))

	config, err := harnesspkg.LoadAgentUnifiedConfigFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, config.Hooks.Entries, 1)
	require.Equal(t, "hooks/block-dangerous-shell/run.sh", config.Hooks.Entries[0].Handler.Path)
}

func TestHyardAdoptInteractiveFullHappyPathWithCodexAssetsAndAcceptedMoves(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Follow [the runbook](docs/runbook.md) and use Codex assets.\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.WriteFile(t, ".codex/prompts/review.md", "# Review prompt\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"description\": \"Block dangerous shell commands.\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-dangerous-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "seed full interactive adoption assets")

	stdout, stderr, err := executeHyardCLIWithInput(t, repo.Root, "\ny\ny\ny\ny\ny\ny\ny\ny\n", "adopt", "--orbit", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply layout move .codex/prompts/review.md -> commands/docs/review.md? [y/N]")
	require.Contains(t, stderr, "Apply layout move .codex/skills/frontend-test-lab -> skills/docs/frontend-test-lab? [y/N]")
	require.Contains(t, stderr, "Apply layout move docs/runbook.md -> guidance/docs/runbook.md? [y/N]")
	require.Contains(t, stderr, "Apply layout move hooks/block-dangerous-shell/run.sh -> hooks/docs/block-dangerous-shell/run.sh? [y/N]")
	require.Contains(t, stdout, "check_ok: true\n")
	require.Contains(t, stdout, "validation: runtime_readiness ok=true\n")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Capabilities)
	require.NotNil(t, spec.Capabilities.Commands)
	require.Equal(t, []string{"commands/docs/review.md"}, spec.Capabilities.Commands.Paths.Include)
	require.NotNil(t, spec.Capabilities.Skills)
	require.NotNil(t, spec.Capabilities.Skills.Local)
	require.Equal(t, []string{"skills/docs/frontend-test-lab"}, spec.Capabilities.Skills.Local.Paths.Include)
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name: "runbook",
		Role: orbitpkg.OrbitMemberRule,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"guidance/docs/runbook.md"},
		},
	})
	require.True(t, hyardAdoptSpecHasMemberPath(
		spec,
		"hook-handler-block-dangerous-shell",
		orbitpkg.OrbitMemberProcess,
		"hooks/docs/block-dangerous-shell/run.sh",
	))

	config, err := harnesspkg.LoadAgentUnifiedConfigFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, config.Hooks.Entries, 1)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", config.Hooks.Entries[0].Handler.Path)
}

func hyardAdoptSpecHasMemberPath(
	spec orbitpkg.OrbitSpec,
	name string,
	role orbitpkg.OrbitMemberRole,
	includePath string,
) bool {
	for _, member := range spec.Members {
		if member.Name != name || member.Role != role {
			continue
		}
		for _, candidatePath := range member.Paths.Include {
			if candidatePath == includePath {
				return true
			}
		}
	}

	return false
}

func TestHyardAdoptWriteJSONConvertsCleanRootGuidanceSlice(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	originalGuidance := "# Agent guidance\n\nUse the project language from CONTEXT.md.\n"
	repo.WriteFile(t, "AGENTS.md", originalGuidance)
	repo.WriteFile(t, "CONTEXT.md", "# Project language\n")
	repo.AddAndCommit(t, "seed root guidance")
	initialHead := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "write", payload.Mode)
	require.Equal(t, "docs", payload.AdoptedOrbit.ID)
	require.ElementsMatch(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"AGENTS.md",
	}, payload.WrittenPaths)
	require.True(t, payload.Check.OK)
	require.Equal(t, 0, payload.Check.FindingCount)
	require.Equal(t, harnesspkg.ReadinessStatusReady, payload.Readiness.Runtime.Status)
	require.ElementsMatch(t, []hyardAdoptWriteValidation{
		{Target: "runtime_manifest", OK: true},
		{Target: "adopted_orbit_spec", OK: true},
		{Target: "projection_plan", OK: true},
		{Target: "runtime_check", OK: true},
		{Target: "runtime_readiness", OK: true},
	}, payload.Validations)
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard check",
		Reason:  "validate the generated Harness Runtime",
	})
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard agent apply --yes",
		Reason:  "optionally activate agent-facing runtime guidance",
	})
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard publish harness",
		Reason:  "optionally publish a Harness Template after review",
	})
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "git status && git add AGENTS.md .harness/manifest.yaml .harness/orbits/docs.yaml && git commit",
		Reason:  "review and commit Adoption changes when ready",
	})

	manifestFile, err := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, harnesspkg.ManifestKindRuntime, manifestFile.Kind)
	require.Len(t, manifestFile.Members, 1)
	require.Equal(t, "docs", manifestFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.ManifestMemberSourceManual, manifestFile.Members[0].Source)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, originalGuidance, spec.Meta.AgentsTemplate)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "<!-- orbit:begin workflow=\"docs\" -->\n")
	require.Contains(t, string(agentsData), "<!-- orbit:end workflow=\"docs\" -->\n")
	document, err := orbittemplate.ParseRuntimeAgentsDocument(agentsData)
	require.NoError(t, err)
	require.Equal(t, []orbittemplate.AgentsRuntimeSegment{{
		Kind:       orbittemplate.AgentsRuntimeSegmentBlock,
		OwnerKind:  orbittemplate.OwnerKindOrbit,
		WorkflowID: "docs",
		OrbitID:    "docs",
		Content:    []byte(originalGuidance),
	}}, document.Segments)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Stat(filepath.Join(repo.Root, ".harness", "agents"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Stat(filepath.Join(repo.Root, ".harness", "template.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
	require.Equal(t, initialHead, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
}

func TestHyardAdoptWriteJSONAuthorsCodexLocalSkillsAsCapabilityTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	originalGuidance := "# Agent guidance\n\nUse local Codex skills when they apply.\n"
	repo.WriteFile(t, "AGENTS.md", originalGuidance)
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.WriteFile(t, ".codex/skills/frontend-test-lab/checklist.md", "- run browser checks\n")
	repo.AddAndCommit(t, "seed root guidance and codex local skill")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Check.OK)
	require.Contains(t, payload.WrittenPaths, ".harness/agents/manifest.yaml")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Capabilities)
	require.NotNil(t, spec.Capabilities.Skills)
	require.NotNil(t, spec.Capabilities.Skills.Local)
	require.Equal(t, []string{".codex/skills/frontend-test-lab"}, spec.Capabilities.Skills.Local.Paths.Include)
	require.Empty(t, spec.Members)

	trackedFiles := strings.Fields(repo.Run(t, "ls-files"))
	resolved, err := orbitpkg.ResolveLocalSkillCapabilities(repo.Root, spec, trackedFiles, trackedFiles)
	require.NoError(t, err)
	require.Equal(t, []orbitpkg.ResolvedLocalSkillCapability{{
		Name:        "frontend-test-lab",
		RootPath:    ".codex/skills/frontend-test-lab",
		SkillMDPath: ".codex/skills/frontend-test-lab/SKILL.md",
	}}, resolved)

	frameworksData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(frameworksData), "recommended_framework: codex\n")
}

func TestHyardAdoptWriteJSONConvertsCodexNativeHookToUnifiedTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nUse Codex hook checks.\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"description\": \"Block dangerous shell commands.\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-dangerous-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "seed codex hook ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.WrittenPaths, ".harness/agents/config.yaml")

	config, err := harnesspkg.LoadAgentUnifiedConfigFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, map[string]harnesspkg.AgentUnifiedConfigTarget{
		"codex": {Enabled: true, Scope: "project"},
	}, config.Targets)
	require.True(t, config.Hooks.Enabled)
	require.Equal(t, "skip", config.Hooks.UnsupportedBehavior)
	require.Len(t, config.Hooks.Entries, 1)
	hook := config.Hooks.Entries[0]
	require.Equal(t, "block-dangerous-shell", hook.ID)
	require.Equal(t, "Block dangerous shell commands.", hook.Description)
	require.Equal(t, "tool.before", hook.Event.Kind)
	require.Equal(t, "command", hook.Handler.Type)
	require.Equal(t, "hooks/block-dangerous-shell/run.sh", hook.Handler.Path)
	require.Equal(t, map[string]bool{"codex": true}, hook.Targets)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Len(t, spec.Members, 1)
	require.Equal(t, "hook-handler-block-dangerous-shell", spec.Members[0].Name)
	require.Equal(t, orbitpkg.OrbitMemberProcess, spec.Members[0].Role)
	require.Equal(t, []string{"hooks/block-dangerous-shell/run.sh"}, spec.Members[0].Paths.Include)
}

func TestHyardAdoptWriteJSONMergesCodexNativeHooksWithImportedProjectConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nUse Codex config and hook checks.\n")
	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-dangerous-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "seed codex config and hook ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "model",
		Source: "project",
		Value:  "gpt-5.4",
	})
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "sandbox_mode",
		Source: "project",
		Value:  "workspace-write",
	})

	config, err := harnesspkg.LoadAgentUnifiedConfigFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", config.Config["model"])
	require.Equal(t, "workspace-write", config.Config["sandbox_mode"])
	require.Len(t, config.Hooks.Entries, 1)
	require.Equal(t, "block-dangerous-shell", config.Hooks.Entries[0].ID)
	require.Equal(t, "hooks/block-dangerous-shell/run.sh", config.Hooks.Entries[0].Handler.Path)
}

func TestHyardAdoptWriteJSONImportsSafeCodexProjectConfig(t *testing.T) {
	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nUse Codex project settings.\n")
	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")
	repo.AddAndCommit(t, "seed codex project config ordinary repository")

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".codex", "config.toml"), []byte("approval_policy = \"never\"\n"), 0o600))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Equal(t, "codex", payload.AgentConfig.Framework)
	require.False(t, payload.AgentConfig.DryRun)
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "model",
		Source: "project",
		Value:  "gpt-5.4",
	})
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "sandbox_mode",
		Source: "project",
		Value:  "workspace-write",
	})
	require.NotContains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "approval_policy",
		Source: "global",
		Value:  "never",
	})
	require.Contains(t, payload.WrittenPaths, ".harness/agents/config.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/agents/manifest.yaml")

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  codex:\n")
	require.Contains(t, string(configData), "    scope: project\n")
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
	require.Contains(t, string(configData), "  sandbox_mode: workspace-write\n")
	require.NotContains(t, string(configData), "approval_policy")

	frameworksData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(frameworksData), "recommended_framework: codex\n")
}

func TestHyardAdoptWriteJSONReportsSkippedUnsafeCodexProjectConfigKeys(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nUse safe Codex project settings.\n")
	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"api_key = \"secret-value\"\n"+
		"helper_path = \"~/bin/local-helper\"\n")
	repo.AddAndCommit(t, "seed unsafe codex project config ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "model",
		Source: "project",
		Value:  "gpt-5.4",
	})
	require.Contains(t, payload.AgentConfig.Skipped, harnesspkg.AgentConfigImportSkippedEntry{
		Key:    "api_key",
		Source: "project",
		Reason: "sensitive",
	})
	require.Contains(t, payload.AgentConfig.Skipped, harnesspkg.AgentConfigImportSkippedEntry{
		Key:    "helper_path",
		Source: "project",
		Reason: "local_path",
	})

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
	require.NotContains(t, string(configData), "secret-value")
	require.NotContains(t, string(configData), "local-helper")
}

func TestHyardAdoptWriteJSONPreservesRoundTripUnstableCodexProjectConfigSidecar(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	nativeConfig := "# keep native Codex comments\nmodel = \"gpt-5.4\"\n"
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nKeep Codex native config reviewable.\n")
	repo.WriteFile(t, ".codex/config.toml", nativeConfig)
	repo.AddAndCommit(t, "seed roundtrip unstable codex project config ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Contains(t, payload.AgentConfig.Sidecars, harnesspkg.AgentConfigImportSidecar{
		Path:   ".harness/agents/codex.config.toml",
		Source: "project",
		Reason: "roundtrip_unstable",
	})
	require.Contains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")

	sidecarData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.NoError(t, err)
	require.Equal(t, nativeConfig, string(sidecarData))

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
}

func TestHyardAdoptWriteJSONSkipsCodexProjectConfigSidecarWithUnsafeNativeContent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nKeep unsafe Codex config out of truth.\n")
	repo.WriteFile(t, ".codex/config.toml", ""+
		"# keep native Codex comments\n"+
		"model = \"gpt-5.4\"\n"+
		"api_key = \"secret-value\"\n")
	repo.AddAndCommit(t, "seed unsafe native codex project config ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "model",
		Source: "project",
		Value:  "gpt-5.4",
	})
	require.Contains(t, payload.AgentConfig.Skipped, harnesspkg.AgentConfigImportSkippedEntry{
		Key:    "api_key",
		Source: "project",
		Reason: "sensitive",
	})
	require.Empty(t, payload.AgentConfig.Sidecars)
	require.Contains(t, payload.AgentConfig.SkippedSidecars, harnesspkg.AgentConfigImportSidecar{
		Path:   ".harness/agents/codex.config.toml",
		Source: "project",
		Reason: "unsafe_native_content",
	})
	require.NotContains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
}

func TestHyardAdoptWriteJSONMergesCodexProjectConfigWithExistingUnifiedAgentTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Agent guidance\n\nPreserve team Codex defaults.\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: team-standard\n")
	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")
	repo.AddAndCommit(t, "seed existing unified agent truth ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptWritePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotNil(t, payload.AgentConfig)
	require.Contains(t, payload.AgentConfig.Imported, harnesspkg.AgentConfigImportEntry{
		Key:    "sandbox_mode",
		Source: "project",
		Value:  "workspace-write",
	})
	require.Contains(t, payload.AgentConfig.Skipped, harnesspkg.AgentConfigImportSkippedEntry{
		Key:    "model",
		Source: "project",
		Reason: "already_configured",
	})

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: team-standard\n")
	require.Contains(t, string(configData), "  sandbox_mode: workspace-write\n")
	require.NotContains(t, string(configData), "gpt-5.4")
}

func TestHyardAdoptWriteRefusesDirtyWorktreeBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	originalGuidance := "# Agent guidance\n\nStay clean before adoption.\n"
	repo.WriteFile(t, "AGENTS.md", originalGuidance)
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n")
	repo.AddAndCommit(t, "seed dirty refusal repo")
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n\nUncommitted note.\n")
	repo.WriteFile(t, "notes/todo.md", "- adopt later\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json")
	require.Error(t, err)
	require.ErrorContains(t, err, "adoption write mode requires a clean worktree")
	require.ErrorContains(t, err, "README.md")
	require.ErrorContains(t, err, "notes/todo.md")
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, originalGuidance, string(agentsData))
	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptWriteRefusesInvalidCodexLocalSkillBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	originalGuidance := "# Agent guidance\n\nDo not adopt invalid local skills.\n"
	repo.WriteFile(t, "AGENTS.md", originalGuidance)
	repo.WriteFile(t, ".codex/skills/frontend-test-lab/SKILL.md", ""+
		"---\n"+
		"name: frontend-test-lab\n"+
		"---\n"+
		"# Frontend Test Lab\n")
	repo.AddAndCommit(t, "seed invalid codex local skill")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "adoption has blocking diagnostics")
	require.ErrorContains(t, err, "Codex local skill frontmatter is invalid")
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, originalGuidance, string(agentsData))
	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptWriteRefusesExistingHarnessRuntimeBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", "# Runtime guidance\n")
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.May, 4, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed existing harness runtime")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "existing Harness Runtime cannot be adopted again")
	require.ErrorContains(t, err, "hyard layout optimize")
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptWriteRefusesMalformedExistingRootGuidanceMarkersBeforeWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	originalGuidance := "<!-- orbit:begin orbit_id=\"docs\" -->\nUnclosed adopted guidance.\n"
	repo.WriteFile(t, "AGENTS.md", originalGuidance)
	repo.AddAndCommit(t, "seed malformed root marker")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--json", "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "root AGENTS.md has malformed orbit markers")
	require.ErrorContains(t, err, "malformed workflow marker")
	require.Empty(t, stdout)
	require.Empty(t, stderr)

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, originalGuidance, string(agentsData))
	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptCheckJSONReportsCleanOrdinaryRepositoryWithoutWriting(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Clean ordinary repository\n")
	repo.AddAndCommit(t, "seed clean ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, `"source"`)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, "check", payload.Mode)
	require.True(t, payload.Adoptable)
	require.False(t, payload.ExistingHarnessRuntime)
	require.False(t, payload.DirtyWorktree.Dirty)
	require.Empty(t, payload.DirtyWorktree.Paths)
	require.NotEmpty(t, payload.AdoptedOrbit.ID)
	require.Equal(t, "repository_name", payload.AdoptedOrbit.DerivedFrom)
	require.Empty(t, payload.Diagnostics)
	require.Empty(t, payload.NextActions)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptCheckJSONAllowsDirtyWorktreeAndReportsPreviewDiagnostic(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n")
	repo.AddAndCommit(t, "seed dirty ordinary repository")
	repo.WriteFile(t, "README.md", "# Dirty ordinary repository\n\nUncommitted note.\n")
	repo.WriteFile(t, "notes/todo.md", "- adopt me later\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotContains(t, stdout, `"source"`)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.True(t, payload.DirtyWorktree.Dirty)
	require.ElementsMatch(t, []string{"README.md", "notes/todo.md"}, payload.DirtyWorktree.Paths)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "dirty_worktree",
		Severity: "info",
		Message:  "dirty worktree is allowed in adoption check mode",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "worktree_status", Path: "README.md"},
			{Kind: "worktree_status", Path: "notes/todo.md"},
		},
	})

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHyardAdoptCheckJSONRefusesExistingHarnessRuntimeWithLayoutOptimizationAction(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.May, 4, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness runtime")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Adoptable)
	require.True(t, payload.ExistingHarnessRuntime)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "existing_harness_runtime",
		Severity: "error",
		Message:  "existing Harness Runtime cannot be adopted again",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "harness_manifest", Path: ".harness/manifest.yaml"},
		},
	})
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard layout optimize",
		Reason:  "existing Harness Runtimes should use Layout Optimization",
	})

	resolved, err := harnesspkg.ResolveRoot(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, repo.Root, resolved.Repo.Root)
}

func TestHyardAdoptCheckJSONDerivesAdoptedOrbitIDFromRepositoryNameAndAllowsOverride(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "My Runtime Repo!")
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# Named repo\n"), 0o600))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed named repo")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var defaultPayload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &defaultPayload))
	require.Equal(t, "my-runtime-repo", defaultPayload.AdoptedOrbit.ID)
	require.Equal(t, "repository_name", defaultPayload.AdoptedOrbit.DerivedFrom)

	stdout, stderr, err = executeHyardCLI(t, repoRoot, "adopt", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var overridePayload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &overridePayload))
	require.Equal(t, "docs", overridePayload.AdoptedOrbit.ID)
	require.Equal(t, "flag", overridePayload.AdoptedOrbit.DerivedFrom)
}

func TestHyardAdoptCheckJSONReportsCodexProjectFootprintAsRecommendedFramework(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex ordinary repository\n")
	repo.WriteFile(t, ".codex/config.toml", "model = \"gpt-5.4\"\n")
	repo.AddAndCommit(t, "seed codex ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Equal(t, "codex", payload.Frameworks.Recommended)
	require.Contains(t, payload.Frameworks.Detected, hyardAdoptCheckFramework{
		ID:     "codex",
		Status: "supported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: ".codex/config.toml"},
		},
	})
	require.Empty(t, payload.Frameworks.Unsupported)
}

func TestHyardAdoptCheckJSONReportsValidCodexLocalSkillCandidate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex skill ordinary repository\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed codex local skill")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:  ".codex/skills/frontend-test-lab",
		Kind:  "local_skill_capability",
		Shape: "directory",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md", Detail: "frontend-test-lab"},
		},
	})
}

func TestHyardAdoptCheckJSONReportsConvertibleCodexHookHandlerCandidate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex hook ordinary repository\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"block-dangerous-shell\",\n"+
		"      \"description\": \"Block dangerous shell commands.\",\n"+
		"      \"command\": \"hooks/block-dangerous-shell/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.AddAndCommit(t, "seed codex hook ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Equal(t, "codex", payload.Frameworks.Recommended)
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "hooks/block-dangerous-shell/run.sh",
		Kind:                  "codex_hook_handler",
		Shape:                 "file",
		RecommendedMemberRole: "process",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"process", "subject", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_hook_definition", Path: ".codex/hooks.json", Detail: "PreToolUse:block-dangerous-shell"},
		},
	})
	require.Empty(t, payload.Diagnostics)
}

func TestHyardAdoptCheckJSONWarnsForUnsupportedCodexHookEvent(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex hook ordinary repository\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"Notification\": [\n"+
		"    {\n"+
		"      \"id\": \"notify-team\",\n"+
		"      \"command\": \"hooks/notify-team/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.WriteFile(t, "hooks/notify-team/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.AddAndCommit(t, "seed unsupported codex hook ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), "hooks/notify-team/run.sh")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_hook_unsupported_event",
		Severity: "warning",
		Message:  "Codex native hook event is unsupported and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_hook_event", Path: ".codex/hooks.json", Detail: "Notification"},
		},
	})
}

func TestHyardAdoptCheckJSONWarnsForUnparseableCodexHooksFile(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Broken Codex hook ordinary repository\n")
	repo.WriteFile(t, ".codex/hooks.json", "{ not-json\n")
	repo.AddAndCommit(t, "seed broken codex hooks ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Empty(t, payload.Candidates)
	require.Contains(t, hyardAdoptCheckDiagnosticCodes(payload.Diagnostics), "codex_hook_parse_error")
}

func TestHyardAdoptCheckJSONWarnsForUnsafeCodexHookHandlerPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Unsafe Codex hook ordinary repository\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"escaping-handler\",\n"+
		"      \"command\": \"../hooks/escaping-handler/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.AddAndCommit(t, "seed unsafe codex hooks ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Empty(t, payload.Candidates)
	require.Contains(t, hyardAdoptCheckDiagnosticCodes(payload.Diagnostics), "codex_hook_unsafe_command")
}

func TestHyardAdoptCheckJSONWarnsForMissingCodexHookHandler(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Missing Codex hook handler ordinary repository\n")
	repo.WriteFile(t, ".codex/hooks.json", ""+
		"{\n"+
		"  \"PreToolUse\": [\n"+
		"    {\n"+
		"      \"id\": \"missing-handler\",\n"+
		"      \"command\": \"hooks/missing-handler/run.sh\"\n"+
		"    }\n"+
		"  ]\n"+
		"}\n")
	repo.AddAndCommit(t, "seed missing codex hook handler ordinary repository")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Empty(t, payload.Candidates)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_hook_handler_missing",
		Severity: "warning",
		Message:  "Codex native hook handler path is missing",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_hook_definition", Path: ".codex/hooks.json", Detail: "PreToolUse:missing-handler"},
			{Kind: "codex_hook_handler", Path: "hooks/missing-handler/run.sh"},
		},
	})
}

func TestHyardAdoptCheckJSONWarnsWhenCodexLocalSkillUsesNonRecommendedPosition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Codex skill ordinary repository\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed codex local skill outside recommended position")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_non_recommended_path",
		Severity: "warning",
		Message:  "Codex local skill root is outside the recommended position; if recommended moves are declined, Adoption will keep it as a capability path",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md", Detail: "recommended: skills/docs/frontend-test-lab"},
		},
	})
}

func TestHyardAdoptCheckJSONDoesNotAddCodexLocalSkillRootAsOrdinaryMemberCandidate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Use .codex/skills/frontend-test-lab for frontend validation.\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed mentioned codex local skill")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:  ".codex/skills/frontend-test-lab",
		Kind:  "local_skill_capability",
		Shape: "directory",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md", Detail: "frontend-test-lab"},
		},
	})
	for _, candidate := range payload.Candidates {
		require.False(t,
			candidate.Path == ".codex/skills/frontend-test-lab" && candidate.Kind == "referenced_guidance_document",
			"skill roots must not be ordinary member-role candidates",
		)
	}
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_member_overlap_avoided",
		Severity: "warning",
		Message:  "referenced Codex local skill root is capability-owned and will not be adopted as ordinary member content",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: ".codex/skills/frontend-test-lab"},
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md"},
		},
	})
}

func TestHyardAdoptCheckJSONDoesNotAddCodexLocalSkillsDirectoryAsOrdinaryMemberCandidate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Use .codex/skills/ for local automation.\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	repo.AddAndCommit(t, "seed mentioned codex local skills directory")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), ".codex/skills")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_member_overlap_avoided",
		Severity: "warning",
		Message:  "referenced Codex local skill root is capability-owned and will not be adopted as ordinary member content",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: ".codex/skills"},
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md"},
		},
	})
}

func TestHyardAdoptCheckJSONBlocksCodexLocalSkillWithInvalidFrontmatter(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Invalid Codex skill ordinary repository\n")
	repo.WriteFile(t, ".codex/skills/frontend-test-lab/SKILL.md", ""+
		"---\n"+
		"description: Fast frontend validation workflow\n"+
		"---\n"+
		"# Frontend Test Lab\n")
	repo.AddAndCommit(t, "seed invalid codex local skill")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Adoptable)
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), ".codex/skills/frontend-test-lab")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_invalid_frontmatter",
		Severity: "error",
		Message:  "Codex local skill frontmatter is invalid: SKILL.md frontmatter must define non-empty name",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md"},
		},
	})
}

func TestHyardAdoptCheckJSONBlocksDuplicateCodexLocalSkillNames(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Duplicate Codex skill ordinary repository\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-lab", "frontend-test-lab", "Fast frontend validation workflow")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend-test-copy", "frontend-test-lab", "Duplicate frontend validation workflow")
	repo.AddAndCommit(t, "seed duplicate codex local skills")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Adoptable)
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), ".codex/skills/frontend-test-lab")
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), ".codex/skills/frontend-test-copy")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_duplicate_name",
		Severity: "error",
		Message:  `Codex local skill name "frontend-test-lab" is declared by multiple roots`,
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-copy/SKILL.md", Detail: "frontend-test-lab"},
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend-test-lab/SKILL.md", Detail: "frontend-test-lab"},
		},
	})
}

func TestHyardAdoptCheckJSONBlocksCodexLocalSkillWithInvalidIdentity(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Invalid Codex skill identity ordinary repository\n")
	writeHyardAdoptSkill(t, repo, ".codex/skills/frontend.test", "frontend.test", "Invalid skill identity")
	repo.AddAndCommit(t, "seed invalid codex local skill identity")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Adoptable)
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), ".codex/skills/frontend.test")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "codex_local_skill_invalid_identity",
		Severity: "error",
		Message:  "Codex local skill identity is invalid: invalid skill basename: orbit id must use lowercase letters, digits, hyphens, or underscores, and must start and end with an alphanumeric character",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "codex_skill_root", Path: ".codex/skills/frontend.test/SKILL.md"},
		},
	})
}

func TestHyardAdoptCheckJSONReportsUnsupportedClaudeCodeAndOpenClawFootprints(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "# Unsupported footprint ordinary repository\n")
	repo.WriteFile(t, "CLAUDE.md", "# Claude Code project guidance\n")
	repo.WriteFile(t, ".openclaw/openclaw.json", "{}\n")
	repo.AddAndCommit(t, "seed unsupported agent footprints")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Adoptable)
	require.Empty(t, payload.Frameworks.Recommended)
	claude := hyardAdoptCheckFramework{
		ID:     "claudecode",
		Status: "unsupported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: "CLAUDE.md"},
		},
	}
	openclaw := hyardAdoptCheckFramework{
		ID:     "openclaw",
		Status: "unsupported",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "project_footprint", Path: ".openclaw/openclaw.json"},
		},
	}
	require.Contains(t, payload.Frameworks.Detected, claude)
	require.Contains(t, payload.Frameworks.Detected, openclaw)
	require.Contains(t, payload.Frameworks.Unsupported, claude)
	require.Contains(t, payload.Frameworks.Unsupported, openclaw)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "unsupported_agent_footprint",
		Severity: "warning",
		Message:  "Claude Code project footprint is detected but unsupported by first-version Adoption",
		Evidence: claude.Evidence,
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "unsupported_agent_footprint",
		Severity: "warning",
		Message:  "OpenClaw project footprint is detected but unsupported by first-version Adoption",
		Evidence: openclaw.Evidence,
	})
}

func TestHyardAdoptCheckJSONReportsRootAndReferencedGuidanceCandidates(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [domain docs](docs/agents/domain.md).\n")
	repo.WriteFile(t, "docs/agents/domain.md", "# Domain language\n")
	repo.AddAndCommit(t, "seed agent guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:  "AGENTS.md",
		Kind:  "root_agent_guidance",
		Shape: "file",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "root_agent_guidance", Path: "AGENTS.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/agents/domain.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/agents/domain.md"},
		},
	})
}

func TestHyardAdoptCheckJSONDiscoversMarkdownLinksWithFragmentsAndTitles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [domain docs](docs/agents/domain.md#language \"Domain language\").\n")
	repo.WriteFile(t, "docs/agents/domain.md", "# Domain language\n")
	repo.AddAndCommit(t, "seed titled agent guidance link")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/agents/domain.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/agents/domain.md"},
		},
	})
	require.Empty(t, payload.Diagnostics)
}

func TestHyardAdoptCheckJSONDiscoversPathMentionsAndKeepsDirectoryCandidates(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Read CONTEXT.md, docs/runbook.md, and docs/ops/ before adopting this repository.\n")
	repo.WriteFile(t, "CONTEXT.md", "# Project language\n")
	repo.WriteFile(t, "docs/runbook.md", "# Runbook\n")
	repo.WriteFile(t, "docs/ops/incident.md", "# Incident process\n")
	repo.AddAndCommit(t, "seed mentioned guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "CONTEXT.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "CONTEXT.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/runbook.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/runbook.md"},
		},
	})
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/ops",
		Kind:                  "referenced_guidance_document",
		Shape:                 "directory",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/ops"},
		},
	})
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), "docs/ops/incident.md")
}

func TestHyardAdoptCheckJSONWarnsAboutMissingAndUntrackedGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [missing guidance](docs/missing.md) and docs/draft.md.\n")
	repo.WriteFile(t, "docs/draft.md", "# Draft guidance\n")
	repo.AddAndCommit(t, "seed root guidance", "AGENTS.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	candidatePaths := hyardAdoptCheckCandidatePaths(payload.Candidates)
	require.NotContains(t, candidatePaths, "docs/missing.md")
	require.NotContains(t, candidatePaths, "docs/draft.md")
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_missing",
		Severity: "warning",
		Message:  "referenced guidance path is missing",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/missing.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_untracked",
		Severity: "warning",
		Message:  "referenced guidance path is untracked and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "docs/draft.md"},
		},
	})
}

func TestHyardAdoptCheckJSONFiltersExternalAnchorsAndWarnsAboutUnsafeGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"See [external](https://example.com/guide.md), [anchor](#local), "+
		"[parent](../outside.md), and [absolute](/tmp/secret.md).\n")
	repo.AddAndCommit(t, "seed unsafe guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []hyardAdoptCheckCandidate{
		{
			Path:  "AGENTS.md",
			Kind:  "root_agent_guidance",
			Shape: "file",
			Evidence: []hyardAdoptCheckEvidence{
				{Kind: "root_agent_guidance", Path: "AGENTS.md"},
			},
		},
	}, payload.Candidates)
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_unsafe",
		Severity: "warning",
		Message:  "referenced guidance path is unsafe and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "../outside.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_unsafe",
		Severity: "warning",
		Message:  "referenced guidance path is unsafe and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "/tmp/secret.md"},
		},
	})
}

func TestHyardAdoptCheckJSONWarnsAboutIgnoredDependencyAndCacheGuidanceReferences(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".gitignore", "node_modules/\n.cache/\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Do not adopt node_modules/tool/README.md or .cache/agent.md.\n")
	repo.WriteFile(t, "node_modules/tool/README.md", "# Tool docs\n")
	repo.WriteFile(t, ".cache/agent.md", "# Cached agent output\n")
	repo.AddAndCommit(t, "seed ignored guidance references", ".gitignore", "AGENTS.md")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_ignored",
		Severity: "warning",
		Message:  "referenced guidance path is ignored dependency or cache content and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: "node_modules/tool/README.md"},
		},
	})
	require.Contains(t, payload.Diagnostics, hyardAdoptCheckDiagnostic{
		Code:     "referenced_guidance_ignored",
		Severity: "warning",
		Message:  "referenced guidance path is ignored dependency or cache content and will not be adopted",
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "path_mention", Path: "AGENTS.md", Detail: ".cache/agent.md"},
		},
	})
}

func TestHyardAdoptCheckJSONKeepsGuidanceDiscoveryOneHopByDefault(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"# Agent guidance\n\n"+
		"Start with [first hop](docs/first.md).\n")
	repo.WriteFile(t, "docs/first.md", ""+
		"# First hop\n\n"+
		"Do not recursively adopt [second hop](docs/second.md).\n")
	repo.WriteFile(t, "docs/second.md", "# Second hop\n")
	repo.AddAndCommit(t, "seed one-hop guidance")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "adopt", "--check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Candidates, hyardAdoptCheckCandidate{
		Path:                  "docs/first.md",
		Kind:                  "referenced_guidance_document",
		Shape:                 "file",
		RecommendedMemberRole: "rule",
		RoleConfirmation: hyardAdoptCheckRoleConfirmation{
			Required:               true,
			BatchAcceptRecommended: true,
			EditableRoles:          []string{"rule", "subject", "process", "ignore"},
		},
		Evidence: []hyardAdoptCheckEvidence{
			{Kind: "markdown_link", Path: "AGENTS.md", Detail: "docs/first.md"},
		},
	})
	require.NotContains(t, hyardAdoptCheckCandidatePaths(payload.Candidates), "docs/second.md")
}

func hyardAdoptCheckCandidatePaths(candidates []hyardAdoptCheckCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.Path)
	}

	return paths
}

func hyardAdoptCheckDiagnosticCodes(diagnostics []hyardAdoptCheckDiagnostic) []string {
	codes := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code)
	}

	return codes
}

func newNamedGitRepoForHyardAdopt(t *testing.T, name string) string {
	t.Helper()

	repoRoot := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(repoRoot, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".home"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".xdg"), 0o750))
	runGitForHyardAdopt(t, repoRoot, "init")
	runGitForHyardAdopt(t, repoRoot, "config", "user.name", "Hyard Adopt Test")
	runGitForHyardAdopt(t, repoRoot, "config", "user.email", "hyard-adopt@example.com")

	return repoRoot
}

func runGitForHyardAdopt(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = repoRoot
	command.Env = append(
		os.Environ(),
		"HOME="+filepath.Join(repoRoot, ".home"),
		"XDG_CONFIG_HOME="+filepath.Join(repoRoot, ".xdg"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, string(output))

	return string(output)
}

func writeHyardAdoptSkill(t *testing.T, repo *testutil.Repo, rootPath string, name string, description string) {
	t.Helper()

	repo.WriteFile(t, rootPath+"/SKILL.md", ""+
		"---\n"+
		"name: "+name+"\n"+
		"description: "+description+"\n"+
		"---\n"+
		"# "+name+"\n")
}
