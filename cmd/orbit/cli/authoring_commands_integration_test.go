package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestCreateWithNameAndDescriptionWritesMemberSchema(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "create", "docs", "--name", "Docs Orbit", "--description", "Docs authoring contract", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot string `json:"repo_root"`
		File     string `json:"file"`
		Schema   string `json:"schema"`
		Orbit    struct {
			ID          string `json:"ID"`
			Name        string `json:"Name"`
			Description string `json:"Description"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), payload.File)
	require.Equal(t, "members", payload.Schema)
	require.Equal(t, "docs", payload.Orbit.ID)
	require.Equal(t, "Docs Orbit", payload.Orbit.Name)
	require.Equal(t, "Docs authoring contract", payload.Orbit.Description)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs authoring contract\n")
	require.Contains(t, string(definitionData), "meta:\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	require.NotContains(t, string(definitionData), "members:\n")
	require.NotContains(t, string(definitionData), "key:")
}

func TestSetUpgradesLegacyHostedDefinitionAndUpdatesTopLevelFields(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "set", "--orbit", "docs", "--name", "Docs Orbit", "--description", "Docs authoring contract", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "orbit", "repo_root")

	showStdout, showStderr, err := executeCLI(t, repo.Root, "show", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)

	showRaw := decodeJSONMap(t, showStdout)
	orbitRaw := requireNestedJSONMap(t, showRaw, "orbit")
	require.Equal(t, "members", orbitRaw["schema"])
	require.Equal(t, "Docs Orbit", orbitRaw["name"])
	require.Equal(t, "Docs authoring contract", orbitRaw["description"])

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs authoring contract\n")
	require.Contains(t, string(definitionData), "meta:\n")
	require.NotContains(t, string(definitionData), "members:\n")
}

func TestMemberAddAndRemoveMutateHostedOrbitSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "docs")
	require.NoError(t, err)

	_, _, err = executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--orbit", "docs",
		"--name", "docs-rules",
		"--description", "Reusable rules for docs work",
		"--role", "rule",
		"--lane", "bootstrap",
		"--include", "docs/rules/**",
		"--include", "docs/templates/**",
		"--exclude", "docs/rules/tmp/**",
		"--json",
	)
	require.NoError(t, err)

	showStdout, showStderr, err := executeCLI(t, repo.Root, "show", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)

	showRaw := decodeJSONMap(t, showStdout)
	orbitRaw := requireNestedJSONMap(t, showRaw, "orbit")
	require.Equal(t, "members", orbitRaw["schema"])
	membersRaw, ok := orbitRaw["members"].([]any)
	require.True(t, ok)
	require.Len(t, membersRaw, 1)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: docs-rules\n")
	require.Contains(t, string(definitionData), "description: Reusable rules for docs work\n")
	require.Contains(t, string(definitionData), "role: rule\n")
	require.Contains(t, string(definitionData), "lane: bootstrap\n")
	require.Contains(t, string(definitionData), "- docs/rules/**\n")
	require.Contains(t, string(definitionData), "- docs/templates/**\n")
	require.Contains(t, string(definitionData), "- docs/rules/tmp/**\n")

	_, _, err = executeCLI(t, repo.Root, "member", "remove", "--orbit", "docs", "--name", "docs-rules", "--json")
	require.NoError(t, err)

	definitionData, err = os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "name: docs-rules\n")
}

func TestMemberAddSkipsCapabilityOwnedIncludesAndWarns(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--orbit", "execute",
		"--name", "execute-assets",
		"--role", "rule",
		"--include", "commands/execute/**",
		"--include", "execute/rules/**",
		"--include", "skills/execute/**",
	)
	require.NoError(t, err)
	require.Contains(t, stdout, "added member execute-assets to orbit execute")
	require.Contains(t, stderr, `warning: skipped member include "commands/execute/**": path is managed by capabilities.commands.paths`)
	require.Contains(t, stderr, `warning: skipped member include "skills/execute/**": path is managed by capabilities.skills.local.paths`)
	require.Contains(t, stderr, "hyard orbit capability list --orbit execute --resolve")

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: execute-assets\n")
	require.Contains(t, string(definitionData), "- execute/rules/**\n")
	require.NotContains(t, string(definitionData), "- commands/execute/**\n")
	require.NotContains(t, string(definitionData), "- skills/execute/**\n")
}

func TestMemberAddJSONReportsSkippedCapabilityOwnedIncludes(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--orbit", "execute",
		"--name", "execute-assets",
		"--role", "rule",
		"--include", "commands/execute/**",
		"--include", "execute/rules/**",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "member", "repo_root", "warnings")
	warnings := requireJSONArray(t, raw, "warnings")
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], `skipped member include "commands/execute/**": path is managed by capabilities.commands.paths`)

	member := requireNestedJSONMap(t, raw, "member")
	paths := requireNestedJSONMap(t, member, "Paths")
	include, ok := paths["Include"].([]any)
	require.True(t, ok)
	require.Equal(t, []any{"execute/rules/**"}, include)
}

func TestMemberAddFailsWhenAllIncludesAreCapabilityOwned(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)
	definitionBefore, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--orbit", "execute",
		"--name", "execute-assets",
		"--role", "rule",
		"--include", "commands/execute/**",
		"--include", "skills/execute/**",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "no member paths left after removing capability-owned paths")
	require.ErrorContains(t, err, "hyard orbit capability list --orbit execute --resolve")

	definitionAfter, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(definitionBefore), string(definitionAfter))
}

func TestAuthoringTruthCommandsDefaultToSourceBranchOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "docs")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "seed source authoring repo")

	showStdout, showStderr, err := executeCLI(t, repo.Root, "show", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)
	showRaw := decodeJSONMap(t, showStdout)
	showOrbitRaw := requireNestedJSONMap(t, showRaw, "orbit")
	require.Equal(t, "docs", showOrbitRaw["id"])

	stdout, stderr, err := executeCLI(t, repo.Root, "set", "--name", "Docs Orbit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "orbit", "repo_root")

	stdout, stderr, err = executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--name", "docs-content",
		"--role", "subject",
		"--include", "docs/**",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw = decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "member", "repo_root")

	filesStdout, filesStderr, err := executeCLI(t, repo.Root, "files", "--json")
	require.NoError(t, err)
	require.Empty(t, filesStderr)
	var filesPayload struct {
		Orbit string   `json:"orbit"`
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(filesStdout), &filesPayload))
	require.Equal(t, "docs", filesPayload.Orbit)
	require.Contains(t, filesPayload.Files, "docs/guide.md")

	stdout, stderr, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"commands-paths",
		"--include", "commands/docs/**/*.md",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw = decodeJSONMap(t, stdout)
	require.Equal(t, "docs", raw["orbit"])

	stdout, stderr, err = executeCLI(t, repo.Root, "capability", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw = decodeJSONMap(t, stdout)
	require.Equal(t, "docs", raw["orbit"])

	_, stderr, err = executeCLI(t, repo.Root, "member", "remove", "--name", "docs-content", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "- commands/docs/**/*.md\n")
	require.NotContains(t, string(definitionData), "name: docs-content\n")
}

func TestCreateWithSpecAddsSpecMemberAndDocFile(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "create", "docs", "--name", "Docs Orbit", "--with-spec", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "orbit", "repo_root", "schema")

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: spec\n")
	require.Contains(t, string(definitionData), "role: rule\n")
	require.Contains(t, string(definitionData), "- docs/docs.md\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))

	specDocData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))
}

func TestCapabilityAddSetListAndRemoveMutateHostedOrbitSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"capability",
		"add",
		"command",
		"--orbit", "execute",
		"--id", "tdd-loop",
		"--path", "execute/commands/tdd-loop.md",
		"--description", "Repeatable frontend TDD loop",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "capability", "file", "kind", "orbit", "repo_root")

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"add",
		"skill",
		"--orbit", "execute",
		"--id", "frontend-test-lab",
		"--path", "execute/skills/frontend-test-lab/SKILL.md",
		"--description", "Local skill for fast frontend validation",
		"--json",
	)
	require.NoError(t, err)

	_, stderr, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skill",
		"--orbit", "execute",
		"--id", "frontend-test-lab",
		"--path", "execute/skills/frontend-test-lab/SKILL.md",
		"--description", "Updated local validation workflow",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err = executeCLI(t, repo.Root, "capability", "list", "--orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw = decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "capabilities", "file", "orbit", "repo_root")
	capabilitiesRaw := requireNestedJSONMap(t, raw, "capabilities")
	commandsRaw := requireNestedJSONMap(t, capabilitiesRaw, "commands")
	requireNestedJSONMap(t, commandsRaw, "paths")
	skillsRaw := requireNestedJSONMap(t, capabilitiesRaw, "skills")
	localSkillsRaw := requireNestedJSONMap(t, skillsRaw, "local")
	requireNestedJSONMap(t, localSkillsRaw, "paths")

	showStdout, showStderr, err := executeCLI(t, repo.Root, "show", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)
	showRaw := decodeJSONMap(t, showStdout)
	orbitRaw := requireNestedJSONMap(t, showRaw, "orbit")
	capsFromShow := requireNestedJSONMap(t, orbitRaw, "capabilities")
	showCommandsRaw := requireNestedJSONMap(t, capsFromShow, "commands")
	requireNestedJSONMap(t, showCommandsRaw, "paths")
	showSkillsRaw := requireNestedJSONMap(t, capsFromShow, "skills")
	showLocalSkillsRaw := requireNestedJSONMap(t, showSkillsRaw, "local")
	requireNestedJSONMap(t, showLocalSkillsRaw, "paths")

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "capabilities:\n")
	require.Contains(t, string(definitionData), "commands:\n")
	require.Contains(t, string(definitionData), "- execute/commands/tdd-loop.md\n")
	require.Contains(t, string(definitionData), "skills:\n")
	require.Contains(t, string(definitionData), "- execute/skills/frontend-test-lab\n")

	_, _, err = executeCLI(t, repo.Root, "capability", "remove", "skill", "--orbit", "execute", "--id", "frontend-test-lab", "--json")
	require.NoError(t, err)

	definitionData, err = os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "execute/skills/frontend-test-lab\n")
}

func requireContainsDefaultCapabilityTruth(t *testing.T, definitionYAML string) {
	t.Helper()

	require.Contains(t, definitionYAML, "capabilities:\n")
	require.Contains(t, definitionYAML, "commands:\n")
	require.Contains(t, definitionYAML, "paths:\n")
	require.Contains(t, definitionYAML, "- commands/docs/**/*.md\n")
	require.Contains(t, definitionYAML, "skills:\n")
	require.Contains(t, definitionYAML, "local:\n")
	require.Contains(t, definitionYAML, "- skills/docs/*\n")
}

func requireContainsSeedEmptyGuidanceArtifacts(t *testing.T, repoRoot string) {
	t.Helper()

	for _, path := range []string{"AGENTS.md", "HUMANS.md", "BOOTSTRAP.md"} {
		data, err := os.ReadFile(filepath.Join(repoRoot, path))
		require.NoError(t, err)
		require.Contains(t, string(data), `<!-- orbit:begin workflow="docs" -->`)
		require.Contains(t, string(data), `<!-- orbit:end workflow="docs" -->`)
	}
}

func TestCapabilityAddFailsClosedForRuntimeGuidancePath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	definitionBefore, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"capability",
		"add",
		"command",
		"--orbit", "execute",
		"--id", "bad-guidance-link",
		"--path", "AGENTS.md",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `capabilities.commands.paths.include[0]: path "AGENTS.md" must not target runtime guidance artifacts`)

	definitionAfter, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(definitionBefore), string(definitionAfter))
}

func TestCapabilitySetPathScopesAndRemoteURIsMutateHostedOrbitSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"commands-paths",
		"--orbit", "execute",
		"--include", "commands/execute/**/*.md",
		"--exclude", "commands/execute/_drafts/**",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "capabilities", "file", "orbit", "repo_root")

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skills-local-paths",
		"--orbit", "execute",
		"--include", "skills/execute/*",
		"--exclude", "skills/execute/_archive/*",
		"--json",
	)
	require.NoError(t, err)

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skills-remote-uris",
		"--orbit", "execute",
		"--uri", "github://acme/frontend-remote-skill",
		"--uri", "https://example.com/skills/research-playbook",
		"--json",
	)
	require.NoError(t, err)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "commands:\n")
	require.Contains(t, string(definitionData), "- commands/execute/**/*.md\n")
	require.Contains(t, string(definitionData), "- commands/execute/_drafts/**\n")
	require.Contains(t, string(definitionData), "local:\n")
	require.Contains(t, string(definitionData), "- skills/execute/*\n")
	require.Contains(t, string(definitionData), "- skills/execute/_archive/*\n")
	require.Contains(t, string(definitionData), "remote:\n")
	require.Contains(t, string(definitionData), "- github://acme/frontend-remote-skill\n")
	require.Contains(t, string(definitionData), "- https://example.com/skills/research-playbook\n")
}

func TestCapabilityMigrateV066MigratesLegacyEntriesIntoCanonicalTruth(t *testing.T) {
	t.Parallel()

	repo := seedLegacyCapabilityRepo(t, "execute", ""+
		"id: execute\n"+
		"description: Execute orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/execute.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    - id: review\n"+
		"      path: commands/execute/review.md\n"+
		"      description: Review work\n"+
		"  skills:\n"+
		"    - id: frontend-test-lab\n"+
		"      path: skills/execute/frontend-test-lab/SKILL.md\n"+
		"      description: Fast frontend validation\n"+
		"    - id: frontend-remote-skill\n"+
		"      uri: github://acme/frontend-remote-skill\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "capability", "migrate-v0-66", "--orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "capabilities", "file", "migrated", "orbit", "repo_root")
	require.Equal(t, true, raw["migrated"])

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "commands:\n")
	require.Contains(t, string(definitionData), "- commands/execute/**/*.md\n")
	require.Contains(t, string(definitionData), "local:\n")
	require.Contains(t, string(definitionData), "- skills/execute/*\n")
	require.Contains(t, string(definitionData), "remote:\n")
	require.Contains(t, string(definitionData), "dependencies:\n")
	require.Contains(t, string(definitionData), "uri: github://acme/frontend-remote-skill\n")
	require.Contains(t, string(definitionData), "required: false\n")
	require.NotContains(t, string(definitionData), "path: commands/execute/review.md\n")
	require.NotContains(t, string(definitionData), "path: skills/execute/frontend-test-lab/SKILL.md\n")
}

func TestCapabilityMigrateV066DefaultsToSourceBranchOrbit(t *testing.T) {
	t.Parallel()

	repo := seedLegacyCapabilityRepo(t, "execute", ""+
		"id: execute\n"+
		"description: Execute orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/execute.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    - id: review\n"+
		"      path: commands/execute/review.md\n")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: execute\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "add source authoring manifest")

	stdout, stderr, err := executeCLI(t, repo.Root, "capability", "migrate-v0-66", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	require.Equal(t, "execute", raw["orbit"])
	require.Equal(t, true, raw["migrated"])
}

func TestCapabilityMigrateV066FallsBackToExplicitIncludesForNonDefaultLegacyPaths(t *testing.T) {
	t.Parallel()

	repo := seedLegacyCapabilityRepo(t, "execute", ""+
		"id: execute\n"+
		"description: Execute orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/execute.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    - id: review\n"+
		"      path: execute/commands/review.md\n"+
		"  skills:\n"+
		"    - id: frontend-test-lab\n"+
		"      path: execute/skills/frontend-test-lab/SKILL.md\n")

	_, stderr, err := executeCLI(t, repo.Root, "capability", "migrate-v0-66", "--orbit", "execute")
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "- execute/commands/review.md\n")
	require.NotContains(t, string(definitionData), "commands/execute/**/*.md")
	require.Contains(t, string(definitionData), "- execute/skills/frontend-test-lab\n")
	require.NotContains(t, string(definitionData), "skills/execute/*")
}

func TestCapabilityMigrateV066IsNoOpForCanonicalTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "capability", "set", "commands-paths", "--orbit", "execute", "--include", "commands/execute/**/*.md")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "capability", "migrate-v0-66", "--orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	require.Equal(t, false, raw["migrated"])
}

func TestCapabilityMigrateV066MigratesRemoteURIsIntoStructuredDependencies(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)
	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skills-remote-uris",
		"--orbit", "execute",
		"--uri", "github://acme/frontend-remote-skill",
		"--uri", "https://example.com/skills/research-playbook",
	)
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "capability", "migrate-v0-66", "--orbit", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw := decodeJSONMap(t, stdout)
	require.Equal(t, true, raw["migrated"])

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "execute.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "remote:\n")
	require.Contains(t, string(definitionData), "dependencies:\n")
	require.Contains(t, string(definitionData), "uri: github://acme/frontend-remote-skill\n")
	require.Contains(t, string(definitionData), "uri: https://example.com/skills/research-playbook\n")
	require.Contains(t, string(definitionData), "required: false\n")
	require.NotContains(t, string(definitionData), "uris:\n")
}

func TestCapabilityListResolveAndShowJSONIncludeResolvedCapabilities(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"commands-paths",
		"--orbit", "execute",
		"--include", "commands/execute/**/*.md",
	)
	require.NoError(t, err)

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skills-local-paths",
		"--orbit", "execute",
		"--include", "skills/execute/*",
	)
	require.NoError(t, err)

	_, _, err = executeCLI(
		t,
		repo.Root,
		"capability",
		"set",
		"skills-remote-uris",
		"--orbit", "execute",
		"--uri", "github://acme/frontend-remote-skill",
	)
	require.NoError(t, err)

	repo.WriteFile(t, "commands/execute/review.md", "Review current work.\n")
	repo.WriteFile(t, "skills/execute/frontend-test-lab/SKILL.md", ""+
		"---\n"+
		"name: frontend-test-lab\n"+
		"description: Fast frontend validation workflow\n"+
		"---\n"+
		"# Frontend Test Lab\n")
	repo.WriteFile(t, "skills/execute/frontend-test-lab/checklist.md", "ship it\n")
	repo.AddAndCommit(t, "seed capability assets")

	stdout, stderr, err := executeCLI(t, repo.Root, "capability", "list", "--orbit", "execute", "--resolve", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "capabilities", "file", "orbit", "repo_root", "resolved_commands", "resolved_local_skills", "resolved_remote_skills")
	resolvedCommands := requireJSONArray(t, raw, "resolved_commands")
	require.Len(t, resolvedCommands, 1)
	resolvedCommand, ok := resolvedCommands[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "review", resolvedCommand["name"])
	require.Equal(t, "commands/execute/review.md", resolvedCommand["path"])
	resolvedLocalSkills := requireJSONArray(t, raw, "resolved_local_skills")
	require.Len(t, resolvedLocalSkills, 1)
	resolvedLocalSkill, ok := resolvedLocalSkills[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "frontend-test-lab", resolvedLocalSkill["name"])
	require.Equal(t, "skills/execute/frontend-test-lab", resolvedLocalSkill["root_path"])
	resolvedRemoteSkills := requireJSONArray(t, raw, "resolved_remote_skills")
	require.Len(t, resolvedRemoteSkills, 1)
	resolvedRemoteSkill, ok := resolvedRemoteSkills[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "github://acme/frontend-remote-skill", resolvedRemoteSkill["uri"])

	showStdout, showStderr, err := executeCLI(t, repo.Root, "show", "execute", "--json")
	require.NoError(t, err)
	require.Empty(t, showStderr)
	showRaw := decodeJSONMap(t, showStdout)
	orbitRaw := requireNestedJSONMap(t, showRaw, "orbit")
	resolvedRaw := requireNestedJSONMap(t, orbitRaw, "resolved_capabilities")
	showResolvedCommands := requireJSONArray(t, resolvedRaw, "commands")
	require.Len(t, showResolvedCommands, 1)
	showResolvedLocalSkills := requireJSONArray(t, resolvedRaw, "local_skills")
	require.Len(t, showResolvedLocalSkills, 1)
	showResolvedRemoteSkills := requireJSONArray(t, resolvedRaw, "remote_skills")
	require.Len(t, showResolvedRemoteSkills, 1)
}

func seedLegacyCapabilityRepo(t *testing.T, orbitID string, definitionYAML string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, filepath.Join(".harness", "orbits", orbitID+".yaml"), definitionYAML)
	repo.AddAndCommit(t, "seed legacy capability repo")

	return repo
}
