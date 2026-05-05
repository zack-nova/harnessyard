package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestOrbitCommandWorkflow(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.AddAndCommit(t, "initial commit")

	stdout, stderr, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, fmt.Sprintf(
		"initialized orbit in %s\nmigration_hint: orbit init is deprecated; use harness init\n",
		repo.Root,
	), stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, fmt.Sprintf(
		"initialized orbit in %s\nmigration_hint: orbit init is deprecated; use harness init\n",
		repo.Root,
	), stdout)

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "outside_changes_mode: warn")
	require.Contains(t, string(configData), "shared_scope: []")

	stdout, stderr, err = executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "no orbits configured\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "create", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, fmt.Sprintf("created orbit docs at %s\n", filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")), stdout)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err = executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\tdocs orbit\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "show", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "id: docs\n")
	require.Contains(t, stdout, "description: docs orbit\n")
	require.Contains(t, stdout, "schema: members\n")
	require.Contains(t, stdout, "name: docs-content\n")
	require.Contains(t, stdout, "- docs/**\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid: 1 orbit(s)\nok docs 3 file(s)\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "files", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ".harness/orbits/docs.yaml\ndocs/archive/old.md\ndocs/guide.md\n", stdout)

	scopeData, err := os.ReadFile(filepath.Join(repo.GitDir(t), "orbit", "state", "resolved_scope", "docs.txt"))
	require.NoError(t, err)
	require.Equal(t, stdout, string(scopeData))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "no current orbit\n", stdout)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\n", stdout)
}

func TestInitCreatesStateDirWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stateDir := filepath.Join(repo.GitDir(t), "orbit", "state")
	stateInfo, err := os.Stat(stateDir)
	require.NoError(t, err)
	require.True(t, stateInfo.IsDir())

	_, err = os.Stat(filepath.Join(stateDir, "current_orbit.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestInitJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "init", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot      string `json:"repo_root"`
		ConfigPath    string `json:"config_path"`
		OrbitsDir     string `json:"orbits_dir"`
		StateDir      string `json:"state_dir"`
		ConfigCreated bool   `json:"config_created"`
		OrbitsCreated bool   `json:"orbits_created"`
		Deprecated    bool   `json:"deprecated"`
		MigrationHint string `json:"migration_hint"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Equal(t, filepath.Join(repo.Root, ".orbit", "config.yaml"), output.ConfigPath)
	require.Equal(t, filepath.Join(repo.Root, ".orbit", "orbits"), output.OrbitsDir)
	require.Equal(t, filepath.Join(repo.GitDir(t), "orbit", "state"), output.StateDir)
	require.True(t, output.ConfigCreated)
	require.True(t, output.OrbitsCreated)
	require.True(t, output.Deprecated)
	require.Equal(t, "orbit init is deprecated; use harness init", output.MigrationHint)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "config_created", "config_path", "deprecated", "migration_hint", "orbits_created", "orbits_dir", "repo_root", "state_dir")
}

func TestOrbitInitReportsHarnessCreateGuidanceOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	_, stderr, err := executeCLI(t, workingDir, "init")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "current directory is not a Git repository")
	require.ErrorContains(t, err, "to start a new harness runtime repo here, run:")
	require.ErrorContains(t, err, "harness create .")
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
	require.NotContains(t, err.Error(), "discover git repository")
}

func TestValidateJSONOutput(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "validate", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string `json:"repo_root"`
		Valid    bool   `json:"valid"`
		Orbits   []struct {
			ID         string   `json:"id"`
			ScopeCount int      `json:"scope_count"`
			Warnings   []string `json:"warnings"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.True(t, output.Valid)
	require.Len(t, output.Orbits, 1)
	require.Equal(t, "docs", output.Orbits[0].ID)
	require.Equal(t, 2, output.Orbits[0].ScopeCount)
	require.Empty(t, output.Orbits[0].Warnings)
}

func TestValidateWarnsOnEmptyScope(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "cmd/orbit/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "validate", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		Valid  bool `json:"valid"`
		Orbits []struct {
			ID         string   `json:"id"`
			ScopeCount int      `json:"scope_count"`
			Warnings   []string `json:"warnings"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.True(t, output.Valid)
	require.Len(t, output.Orbits, 1)
	require.Equal(t, "docs", output.Orbits[0].ID)
	require.Equal(t, 1, output.Orbits[0].ScopeCount)
	require.Equal(t, []string{"projection scope is empty"}, output.Orbits[0].Warnings)
}

func TestValidateDoesNotWarnWhenProjectionOnlyContainsProcessSurface(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")
	repo.AddAndCommit(t, "switch docs orbit to process-only member schema")

	stdout, stderr, err := executeCLI(t, repo.Root, "validate", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		Valid  bool `json:"valid"`
		Orbits []struct {
			ID         string   `json:"id"`
			ScopeCount int      `json:"scope_count"`
			Warnings   []string `json:"warnings"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.True(t, output.Valid)
	require.Len(t, output.Orbits, 1)
	require.Equal(t, "docs", output.Orbits[0].ID)
	require.Equal(t, 2, output.Orbits[0].ScopeCount)
	require.Empty(t, output.Orbits[0].Warnings)
}

func TestValidateCountsMemberSchemaProcessPathsInProjectionScope(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.WriteFile(t, ".markdownlint.yaml", "default: true\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	repo.AddAndCommit(t, "add member schema orbit")

	stdout, stderr, err := executeCLI(t, repo.Root, "validate", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		Valid  bool `json:"valid"`
		Orbits []struct {
			ID         string   `json:"id"`
			ScopeCount int      `json:"scope_count"`
			Warnings   []string `json:"warnings"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.True(t, output.Valid)
	require.Len(t, output.Orbits, 1)
	require.Equal(t, "docs", output.Orbits[0].ID)
	require.Equal(t, 4, output.Orbits[0].ScopeCount)
	require.Empty(t, output.Orbits[0].Warnings)
}

func TestValidateFailsOnHostedCapabilitySkillRootMissingSkillMD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/*\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use Orbit style guide.\n")
	repo.AddAndCommit(t, "seed repo with incomplete skill capability")

	stdout, stderr, err := executeCLI(t, repo.Root, "validate")
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.Error(t, err)
	require.ErrorContains(t, err, `local skill root "orbit/skills/docs-style": SKILL.md must exist and be tracked`)
}

func TestValidateFailsOnMalformedRuntimeAgentsMarkers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"broken docs block\n")
	repo.AddAndCommit(t, "initial commit with malformed agents")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "validate runtime AGENTS.md")
	require.ErrorContains(t, err, "unterminated orbit block")
}

func TestValidateIgnoresPlainAgentsWithoutOrbitMarkers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "AGENTS.md", "plain guidance only\n")
	repo.AddAndCommit(t, "initial commit with plain agents")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid: 1 orbit(s)\nok docs 2 file(s)\n", stdout)
}

func TestOrbitBriefBackfillWritesCurrentOrbitBlockToHostedSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)
	setDocsCurrentOrbit(t, repo)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-07T00:00:00Z\n"+
		"  updated_at: 2026-04-07T00:00:00Z\n"+
		"members: []\n")

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"You are the Acme docs orbit.\n"+
		"Keep release notes current.\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n"+
		"<!-- orbit:begin workflow=\"api\" -->\n"+
		"Ignore this.\n"+
		"<!-- orbit:end workflow=\"api\" -->\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "backfilled orbit brief docs")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the Acme docs orbit.\n")
}

func TestOrbitBriefBackfillFailsClosedWhenCurrentOrbitBlockIsMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-07T00:00:00Z\n"+
		"  updated_at: 2026-04-07T00:00:00Z\n"+
		"members: []\n")

	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin workflow=\"api\" -->\n"+
		"API brief.\n"+
		"<!-- orbit:end workflow=\"api\" -->\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md does not contain orbit block "docs"`)

	spec, loadErr := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, loadErr)
	require.NotNil(t, spec.Meta)
	require.Empty(t, spec.Meta.AgentsTemplate)
}

func TestOrbitBriefBackfillWritesCurrentOrbitBlockOnTemplateRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefBackfillRevisionRepo(t, "orbit_template")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "backfilled orbit brief docs")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
}

func TestOrbitBriefBackfillWritesCurrentOrbitBlockOnSourceRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefBackfillRevisionRepo(t, "source")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "backfilled orbit brief docs")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
}

func TestOrbitBriefBackfillReportsSkippedStatusWhenHostedTruthAlreadyMatches(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "You are the $project_name docs orbit.\nKeep release notes current.\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-07T00:00:00Z\n"+
		"  updated_at: 2026-04-07T00:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"You are the Acme docs orbit.\n"+
		"Keep release notes current.\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed in-sync brief backfill repo")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string `json:"orbit_id"`
		Status       string `json:"status"`
		UpdatedField string `json:"updated_field"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "skipped", payload.Status)
	require.Equal(t, "meta.agents_template", payload.UpdatedField)
}

func TestOrbitBriefBackfillReportsRemovedStatusWhenClearingHostedTruth(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "You are the $project_name docs orbit.\nKeep release notes current.\n"
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-04-07T00:00:00Z\n"+
		"  updated_at: 2026-04-07T00:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed removable brief backfill repo")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "status: removed")
}

func TestOrbitBriefBackfillRejectsPlainRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefBackfillRevisionRepo(t, "")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief backfill supports only runtime, source, or orbit_template revisions; current revision kind is "plain"`)
}

func TestOrbitBriefBackfillRejectsHarnessTemplateRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefBackfillRevisionRepo(t, "harness_template")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "backfill", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief backfill supports only runtime, source, or orbit_template revisions; current revision kind is "harness_template"`)
}

func TestOrbitBriefMaterializeWritesRenderedCurrentOrbitBlockToRootAgents(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "runtime", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		"",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "materialized orbit brief docs")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"You are the Acme docs orbit.\n"+
		"Keep release notes current.\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n", string(agentsData))

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, spec.Meta)
	require.Equal(t, "You are the $project_name docs orbit.\nKeep release notes current.\n", spec.Meta.AgentsTemplate)
}

func TestOrbitBriefMaterializePreservesOtherBlocksAndProseOnSourceRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "source", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin workflow=\"api\" -->\n"+
			"API brief.\n"+
			"<!-- orbit:end workflow=\"api\" -->\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "materialized orbit brief docs")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	agents := string(agentsData)
	require.Contains(t, agents, "Workspace overview.\n")
	require.Contains(t, agents, "<!-- orbit:begin workflow=\"api\" -->\nAPI brief.\n<!-- orbit:end workflow=\"api\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin workflow=\"docs\" -->\nYou are the $project_name docs orbit.\nKeep release notes current.\n<!-- orbit:end workflow=\"docs\" -->\n")
	require.Less(t, strings.Index(agents, "Workspace overview.\n"), strings.Index(agents, "<!-- orbit:begin workflow=\"api\" -->"))
	require.Less(t, strings.Index(agents, "<!-- orbit:end workflow=\"api\" -->"), strings.Index(agents, "<!-- orbit:begin workflow=\"docs\" -->"))
}

func TestOrbitBriefMaterializeFailsClosedWhenCurrentOrbitBlockIsDrifted(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "orbit_template", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin workflow=\"docs\" -->\n"+
			"You are the Drifted docs orbit.\n"+
			"Keep release notes current.\n"+
			"<!-- orbit:end workflow=\"docs\" -->\n",
	)

	originalAgents, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md already contains drifted orbit block "docs"`)
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
	require.ErrorContains(t, err, "--force")

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, string(originalAgents), string(agentsData))
}

func TestOrbitBriefMaterializeForceOverwritesDriftedCurrentOrbitBlock(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "orbit_template", ""+
		"You are the $project_name docs orbit.\n"+
		"Keep release notes current.\n",
		""+
			"Workspace overview.\n"+
			"<!-- orbit:begin workflow=\"api\" -->\n"+
			"API brief.\n"+
			"<!-- orbit:end workflow=\"api\" -->\n"+
			"<!-- orbit:begin workflow=\"docs\" -->\n"+
			"You are the Drifted docs orbit.\n"+
			"Keep release notes current.\n"+
			"<!-- orbit:end workflow=\"docs\" -->\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs", "--force")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "materialized orbit brief docs")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	agents := string(agentsData)
	require.Contains(t, agents, "<!-- orbit:begin workflow=\"api\" -->\nAPI brief.\n<!-- orbit:end workflow=\"api\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin workflow=\"docs\" -->\nYou are the $project_name docs orbit.\nKeep release notes current.\n<!-- orbit:end workflow=\"docs\" -->\n")
	require.NotContains(t, agents, "You are the Drifted docs orbit.\n")
}

func TestOrbitBriefMaterializeRejectsPlainRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "", "You are the $project_name docs orbit.\n", "")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief materialize supports only runtime, source, or orbit_template revisions; current revision kind is "plain"`)
}

func TestOrbitBriefMaterializeRejectsHarnessTemplateRevision(t *testing.T) {
	t.Parallel()

	repo := seedBriefMaterializeRevisionRepo(t, "harness_template", "You are the $project_name docs orbit.\n", "")

	stdout, stderr, err := executeCLI(t, repo.Root, "brief", "materialize", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `brief materialize supports only runtime, source, or orbit_template revisions; current revision kind is "harness_template"`)
}

func TestAuthoringCommandsUseHarnessHostedDefinitionsWithoutOrbitInit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	stdout, stderr, err := executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, fmt.Sprintf("created orbit docs at %s\n", filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")), stdout)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err = executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\tdocs orbit\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "show", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "id: docs\n")
	require.Contains(t, stdout, "description: docs orbit\n")
	require.Contains(t, stdout, "schema: members\n")
	require.Contains(t, stdout, "name: docs-content\n")
	require.Contains(t, stdout, "- docs/**\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid: 1 orbit(s)\nok docs 2 file(s)\n", stdout)
}

func TestFilesJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "files", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string   `json:"repo_root"`
		Orbit    string   `json:"orbit"`
		Files    []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Equal(t, "docs", output.Orbit)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, output.Files)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "files", "orbit", "repo_root")
}

func TestAddJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "add", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string `json:"repo_root"`
		File     string `json:"file"`
		Schema   string `json:"schema"`
		Orbit    struct {
			ID          string `json:"ID"`
			Description string `json:"Description"`
			Members     []any  `json:"Members"`
			SourcePath  string `json:"SourcePath"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), output.File)
	require.Equal(t, "members", output.Schema)
	require.Equal(t, "docs", output.Orbit.ID)
	require.Equal(t, "docs orbit", output.Orbit.Description)
	require.Empty(t, output.Orbit.Members)
	require.Empty(t, output.Orbit.SourcePath)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "orbit", "repo_root", "schema")
	orbitRaw := requireNestedJSONMap(t, raw, "orbit")
	requireJSONKeys(t, orbitRaw, "AgentAddons", "Behavior", "Capabilities", "Description", "Exclude", "ID", "Include", "Members", "Meta", "Name", "SourcePath")
}

func TestAddSupportsMemberSchemaFlag(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, fmt.Sprintf("created orbit docs at %s\n", filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")), stdout)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "meta:\n")
	require.NotContains(t, string(definitionData), "members:\n")
	require.Contains(t, string(definitionData), "behavior:\n")
	require.NotContains(t, string(definitionData), "rules:\n")
}

func TestAddMemberSchemaJSONOutputIncludesSchema(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "add", "docs", "--member-schema", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "file", "orbit", "repo_root", "schema")
	require.Equal(t, "members", raw["schema"])
}

func TestAddRejectsInvalidOrbitID(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	_, _, err = executeCLI(t, repo.Root, "add", "Docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "validate orbit id")
}

func TestAddFailsWhenDefinitionFileExists(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: wrong\ninclude:\n  - README.md\n")

	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "orbit definition file")
	require.ErrorContains(t, err, "already exists")
}

func TestValidateFailsOnDuplicateOrbitID(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	repo.WriteFile(t, ".harness/orbits/duplicate.yaml", "id: docs\ninclude:\n  - README.md\n")

	_, _, err = executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate orbit id")
}

func TestValidateFailsOnFilenameMismatch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: guide\ninclude:\n  - README.md\n")

	_, _, err = executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.ErrorContains(t, err, "definition filename must match orbit id")
}

func TestValidateFailsOnInvalidIncludeGlob(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - \"[\"\n")

	_, _, err = executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.ErrorContains(t, err, "include[0]")
}

func TestValidateFailsOnEmptyInclude(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude: []\n")

	_, _, err = executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.ErrorContains(t, err, "include must not be empty")
}

func TestValidateFailsOnCapabilityOwnedMemberOverlap(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "create", "execute")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/execute.yaml", ""+
		"id: execute\n"+
		"meta:\n"+
		"  file: .harness/orbits/execute.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/execute/**/*.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - skills/execute/*\n"+
		"members:\n"+
		"  - name: execute-assets\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/execute/**\n")

	_, _, err = executeCLI(t, repo.Root, "validate")
	require.Error(t, err)
	require.ErrorContains(t, err, `members[0].paths.include[0] overlaps capability-owned commands path "commands/execute/**/*.md"`)
}

func TestValidateIgnoresLegacySharedScopeConfigForAuthoringValidation(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - .harness/orbits/cmd.yaml\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid with warnings: 1 orbit(s)\nwarn docs projection scope is empty\n", stdout)
}

func TestValidateIgnoresLegacyBehaviorConfigForAuthoringValidation(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: ignore\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid with warnings: 1 orbit(s)\nwarn docs projection scope is empty\n", stdout)
}

func TestListSkipsInvalidDefinitions(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/bad.yaml", "id: bad\ninclude: [\n")
	repo.WriteFile(t, ".harness/orbits/extra.yml", "id: extra\ninclude:\n  - extra/**\n")
	repo.WriteFile(t, ".harness/orbits/invalid-id.yaml", "id: Docs\ninclude:\n  - docs/**\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\tdocs orbit\n", stdout)
}

func TestListJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string `json:"repo_root"`
		Orbits   []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			SourcePath  string `json:"source_path"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Equal(t, []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		SourcePath  string `json:"source_path"`
	}{
		{
			ID:          "cmd",
			Description: "cmd orbit",
			SourcePath:  filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"),
		},
		{
			ID:          "docs",
			Description: "docs orbit",
			SourcePath:  filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"),
		},
	}, output.Orbits)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "orbits", "repo_root")
}

func TestFilesRecomputesWhenCacheIsCorrupted(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "files", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ".harness/orbits/docs.yaml\ndocs/guide.md\n", stdout)

	cachePath := filepath.Join(repo.GitDir(t), "orbit", "state", "resolved_scope", "docs.txt")
	require.NoError(t, os.WriteFile(cachePath, []byte("../broken\n"), 0o600))

	stdout, stderr, err = executeCLI(t, repo.Root, "files", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ".harness/orbits/docs.yaml\ndocs/guide.md\n", stdout)

	cacheData, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	require.Equal(t, stdout, string(cacheData))
}

func TestCurrentWarnsOnStaleState(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))

	require.NoError(t, os.Remove(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")))

	stdout, stderr, err := executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\nwarning: stale current orbit state: orbit \"docs\" is missing\n", stdout)
}

func TestCurrentJSONOutputStableWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "current", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string                      `json:"repo_root"`
		Current  *statepkg.CurrentOrbitState `json:"current"`
		Stale    bool                        `json:"stale"`
		Warning  string                      `json:"warning"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Nil(t, output.Current)
	require.False(t, output.Stale)
	require.Empty(t, output.Warning)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "current", "repo_root", "stale")
	require.Contains(t, raw, "current")
	require.Nil(t, raw["current"])
}

func TestCurrentJSONOutputStableWhenStateIsStale(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	enteredAt := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)
	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     enteredAt,
		SparseEnabled: true,
	}))

	require.NoError(t, os.Remove(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")))

	stdout, stderr, err := executeCLI(t, repo.Root, "current", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot    string                      `json:"repo_root"`
		Current     *statepkg.CurrentOrbitState `json:"current"`
		Stale       bool                        `json:"stale"`
		StaleReason string                      `json:"stale_reason"`
		Warning     string                      `json:"warning"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.NotNil(t, output.Current)
	require.Equal(t, "docs", output.Current.Orbit)
	require.Equal(t, enteredAt, output.Current.EnteredAt)
	require.True(t, output.Current.SparseEnabled)
	require.True(t, output.Stale)
	require.Equal(t, "missing_orbit", output.StaleReason)
	require.Equal(t, `stale current orbit state: orbit "docs" is missing`, output.Warning)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "current", "repo_root", "stale", "stale_reason", "warning")
	currentRaw, ok := raw["current"].(map[string]any)
	require.True(t, ok)
	requireJSONKeys(t, currentRaw, "entered_at", "orbit", "sparse_enabled")
}

func TestCurrentWarnsWhenRuntimeLedgerPlanIsStale(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      write: true\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\nwarning: stale current orbit state: active projection no longer matches the current revision; rerun `orbit enter docs` to refresh it\n", stdout)
}

func TestRuntimeOrbitCommandsOnlySeeActiveHarnessMembers(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRepoWithDormantHostedDefinition(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\tdocs orbit\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "valid: 1 orbit(s)\nok docs 2 file(s)\n", stdout)

	_, stderr, err = executeCLI(t, repo.Root, "show", "tools")
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "tools" not found`)

	_, stderr, err = executeCLI(t, repo.Root, "enter", "tools")
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "tools" not found`)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "tools",
		EnteredAt:     time.Date(2026, time.April, 16, 11, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "tools\nwarning: stale current orbit state: orbit \"tools\" is missing\n", stdout)
}

func TestRuntimeOrbitCommandsFailClosedWhenActiveMemberDefinitionIsMissing(t *testing.T) {
	t.Parallel()

	repo := seedRuntimeRepoWithMissingActiveDefinition(t)

	_, stderr, err := executeCLI(t, repo.Root, "list")
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `runtime member "docs" is missing hosted definition`)
}

func TestCurrentJSONOutputIncludesPlanHashStaleReason(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      write: true\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "current", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot    string                      `json:"repo_root"`
		Current     *statepkg.CurrentOrbitState `json:"current"`
		Stale       bool                        `json:"stale"`
		StaleReason string                      `json:"stale_reason"`
		Warning     string                      `json:"warning"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.NotNil(t, output.Current)
	require.Equal(t, "docs", output.Current.Orbit)
	require.True(t, output.Stale)
	require.Equal(t, "runtime_plan_mismatch", output.StaleReason)
	require.Equal(t, "stale current orbit state: active projection no longer matches the current revision; rerun `orbit enter docs` to refresh it", output.Warning)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "current", "repo_root", "stale", "stale_reason", "warning")
}

func TestCurrentFailsOnDamagedStateFile(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	currentPath := filepath.Join(repo.GitDir(t), "orbit", "state", "current_orbit.json")
	require.NoError(t, os.WriteFile(currentPath, []byte("{"), 0o600))

	_, _, err = executeCLI(t, repo.Root, "current")
	require.Error(t, err)
	require.ErrorContains(t, err, "read current orbit")
	require.ErrorContains(t, err, "unmarshal current orbit state")
}

func TestShowJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "show", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		RepoRoot string `json:"repo_root"`
		Orbit    struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Schema      string `json:"schema"`
			Members     []any  `json:"members"`
			SourcePath  string `json:"source_path"`
		} `json:"orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, repo.Root, output.RepoRoot)
	require.Equal(t, "docs", output.Orbit.ID)
	require.Equal(t, "docs orbit", output.Orbit.Description)
	require.Equal(t, "members", output.Orbit.Schema)
	require.Empty(t, output.Orbit.Members)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), output.Orbit.SourcePath)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "orbit", "repo_root")
	orbitRaw := requireNestedJSONMap(t, raw, "orbit")
	requireJSONKeys(t, orbitRaw, "behavior_defaults", "capabilities", "description", "id", "meta", "resolved_capabilities", "role_scopes", "schema", "source_path")
	capabilitiesRaw := requireNestedJSONMap(t, orbitRaw, "capabilities")
	commandsRaw := requireNestedJSONMap(t, capabilitiesRaw, "commands")
	commandPathsRaw := requireNestedJSONMap(t, commandsRaw, "paths")
	require.Equal(t, []any{"commands/docs/**/*.md"}, requireJSONArray(t, commandPathsRaw, "Include"))
	skillsRaw := requireNestedJSONMap(t, capabilitiesRaw, "skills")
	localSkillsRaw := requireNestedJSONMap(t, skillsRaw, "local")
	localSkillPathsRaw := requireNestedJSONMap(t, localSkillsRaw, "paths")
	require.Equal(t, []any{"skills/docs/*"}, requireJSONArray(t, localSkillPathsRaw, "Include"))
	resolvedRaw := requireNestedJSONMap(t, orbitRaw, "resolved_capabilities")
	require.Empty(t, resolvedRaw)
}

func TestShowDisplaysMemberSchemaView(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"name: Documentation\n"+
		"description: User-facing docs orbit.\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      write: true\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "show", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "schema: members\n")
	require.Contains(t, stdout, "members:\n")
	require.Contains(t, stdout, "name: docs-process\n")
	require.Contains(t, stdout, "behavior_defaults:\n")
	require.NotContains(t, stdout, "role_scopes:\n")
	require.Contains(t, stdout, "process:\n")
	require.Contains(t, stdout, "effective_scopes:\n")
}

func TestShowJSONOutputIncludesMemberSchemaView(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: User-facing docs orbit.\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "show", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "orbit", "repo_root")
	orbitRaw := requireNestedJSONMap(t, raw, "orbit")
	requireJSONKeys(t, orbitRaw, "behavior_defaults", "description", "id", "members", "meta", "role_scopes", "schema", "source_path")
	require.Equal(t, "members", orbitRaw["schema"])
}

func TestEnterAppliesSparseCheckoutAndLeaveRestoresView(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (3 file(s))\n", stdout)

	require.NoFileExists(t, filepath.Join(repo.Root, "README.md"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	stdout, stderr, err = executeCLI(t, repo.Root, "leave")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "left orbit docs\n", stdout)

	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "no current orbit\n", stdout)
}

func TestEnterJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		Orbit       string   `json:"orbit"`
		ScopeCount  int      `json:"scope_count"`
		HiddenDirty []string `json:"hidden_dirty"`
		Warnings    []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.Orbit)
	require.Equal(t, 3, output.ScopeCount)
	require.Empty(t, output.HiddenDirty)
	require.Empty(t, output.Warnings)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "hidden_dirty", "orbit", "scope_count", "warnings")
}

func TestControlPlaneCommandsWorkWhenOrbitFilesAreHiddenBySparseCheckout(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	repo.WriteFile(t, ".orbit/config.yaml",
		"version: 1\n"+
			"shared_scope:\n"+
			"  - README.md\n"+
			"behavior:\n"+
			"  outside_changes_mode: warn\n"+
			"  block_switch_if_hidden_dirty: true\n"+
			"  commit_append_trailer: true\n"+
			"  sparse_checkout_mode: no-cone\n",
	)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: docs orbit\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", "id: cmd\ndescription: cmd orbit\ninclude:\n  - cmd/**\n")
	repo.AddAndCommit(t, "add orbit control plane", ".orbit/config.yaml", ".harness/orbits/docs.yaml", ".harness/orbits/cmd.yaml")

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (3 file(s))\n", stdout)

	require.NoFileExists(t, filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "list")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "cmd\tcmd orbit\ndocs\tdocs orbit\n", stdout)

	stdout, stderr, err = executeCLI(t, repo.Root, "show", "cmd")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "id: cmd\n")
	require.Contains(t, stdout, "description: cmd orbit\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "enter", "cmd")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit cmd (3 file(s))\n", stdout)
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
}

func TestEnterBlocksWhenDirtyTrackedPathWouldBeHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, "src/main.go", "package main\n// dirty\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `warning: entering orbit "docs" would hide dirty tracked paths: src/main.go`)
	require.ErrorContains(t, err, `cannot enter orbit "docs": dirty tracked paths would be hidden: src/main.go`)
	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	store, storeErr := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, storeErr)

	_, readErr := store.ReadCurrentOrbit()
	require.ErrorIs(t, readErr, statepkg.ErrCurrentOrbitNotFound)

	warnings, readErr := store.ReadWarnings()
	require.NoError(t, readErr)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Contains(t, warnings.Messages, `cannot enter orbit "docs": dirty tracked paths would be hidden: src/main.go`)
}

func TestEnterBlocksWhenDirtyControlConfigWouldBeHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n"+
		"# dirty change\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `warning: entering orbit "docs" would hide dirty tracked paths: .orbit/config.yaml`)
	require.ErrorContains(t, err, `cannot enter orbit "docs": dirty tracked paths would be hidden: .orbit/config.yaml`)

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)
}

func TestEnterBlocksOrbitSwitchWhenDirtyCurrentOrbitDefinitionWouldBeHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	addDocsSubjectMember(t, repo)
	_, _, err = executeCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "member", "add", "--orbit", "cmd", "--key", "cmd-content", "--role", "subject", "--include", "cmd/**")
	require.NoError(t, err)
	repo.AddAndCommit(t, "add orbit config", ".orbit/config.yaml", ".harness/orbits/docs.yaml", ".harness/orbits/cmd.yaml")

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: dirty docs orbit\ninclude:\n  - docs/**\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "enter", "cmd")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Contains(t, stderr, `warning: entering orbit "cmd" would hide dirty tracked paths: .harness/orbits/docs.yaml`)
	require.ErrorContains(t, err, `cannot enter orbit "cmd": dirty tracked paths would be hidden: .harness/orbits/docs.yaml`)

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Contains(t, currentStdout+currentStderr, "warning: stale current orbit state")
	require.Contains(t, currentStdout, "docs\n")
}

func TestEnterWarnsOnlyForOutsideUntrackedFiles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, "docs/draft.md", "draft\n")
	repo.WriteFile(t, "scratch/outside.txt", "outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.Contains(t, stderr, "warning: outside untracked files remain in the working tree: scratch/outside.txt")
	require.NotContains(t, stderr, "docs/draft.md")
	require.FileExists(t, filepath.Join(repo.Root, "docs", "draft.md"))
	require.FileExists(t, filepath.Join(repo.Root, "scratch", "outside.txt"))
}

func TestEnterWarnsWhenLedgerRefreshFailsAfterCurrentOrbitIsWritten(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	conflictPath := filepath.Join(repo.GitDir(t), "orbit", "state", "orbits", "docs", "git_state.json")
	replacePathWithDirectory(t, conflictPath)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Equal(t, "entered orbit docs (3 file(s))\n", stdout)
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit enter:")

	require.NoFileExists(t, filepath.Join(repo.Root, "README.md"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "docs\n", currentStdout)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "entered", runtimeSnapshot.Phase)

	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh runtime and git ledger for orbit enter:")
}

func TestLeaveWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "leave")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "no current orbit\n", stdout)
}

func TestLeaveJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "leave", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		Orbit              string   `json:"orbit"`
		Left               bool     `json:"left"`
		ProjectionRestored bool     `json:"projection_restored"`
		StateCleared       bool     `json:"state_cleared"`
		Warnings           []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.Orbit)
	require.True(t, output.Left)
	require.True(t, output.ProjectionRestored)
	require.True(t, output.StateCleared)
	require.Empty(t, output.Warnings)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "left", "orbit", "projection_restored", "state_cleared")
}

func TestLeaveRestoresViewWhenCurrentStateIsMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	currentPath := filepath.Join(repo.GitDir(t), "orbit", "state", "current_orbit.json")
	require.NoError(t, os.Remove(currentPath))
	require.NoFileExists(t, currentPath)
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	stdout, stderr, err := executeCLI(t, repo.Root, "leave")
	require.NoError(t, err)
	require.Equal(t, "restored full workspace view\n", stdout)
	require.Contains(t, stderr, "warning: current orbit state is missing; restored full workspace view without orbit metadata")
	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	stdout, stderr, err = executeCLI(t, repo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "no current orbit\n", stdout)
}

func TestLeaveWarnsWhenLedgerRefreshFailsAfterProjectionIsRestored(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/archive/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	conflictPath := filepath.Join(repo.GitDir(t), "orbit", "state", "orbits", "docs", "git_state.json")
	replacePathWithDirectory(t, conflictPath)

	stdout, stderr, err := executeCLI(t, repo.Root, "leave")
	require.NoError(t, err)
	require.Equal(t, "left orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit leave:")

	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)

	sparseSetting := strings.TrimSpace(repo.Run(t, "config", "--bool", "--default", "false", "core.sparseCheckout"))
	require.Equal(t, "false", sparseSetting)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh runtime and git ledger for orbit leave:")
}

func TestEnterStatusAndLeaveUpdateRuntimeAndGitLedger(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	enteredRuntime, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", enteredRuntime.Orbit)
	require.True(t, enteredRuntime.Running)
	require.Equal(t, "entered", enteredRuntime.Phase)
	require.False(t, enteredRuntime.EnteredAt.IsZero())
	require.False(t, enteredRuntime.UpdatedAt.IsZero())
	require.NotEmpty(t, enteredRuntime.PlanHash)

	enteredGit, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, enteredGit.UpdatedAt.IsZero())
	requireGitScopeState(t, enteredGit.OrbitProjectionState, 0)
	requireGitScopeState(t, enteredGit.OrbitStageState, 0)
	requireGitScopeState(t, enteredGit.OrbitCommitState, 0)
	requireGitScopeState(t, enteredGit.OrbitExportState, 0)
	requireGitScopeState(t, enteredGit.OrbitOrchestrationState, 0)
	requireGitScopeState(t, enteredGit.GlobalStageState, 0)
	requireGitScopeState(t, enteredGit.GlobalCommitState, 0)

	repo.WriteFile(t, "docs/guide.md", "updated guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n// outside change\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Contains(t, stdout, "M docs/guide.md\n")
	require.Contains(t, stdout, "M src/main.go\n")
	require.Empty(t, stderr)

	statusRuntime, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", statusRuntime.Orbit)
	require.True(t, statusRuntime.Running)
	require.Equal(t, "status", statusRuntime.Phase)
	require.Equal(t, enteredRuntime.EnteredAt, statusRuntime.EnteredAt)
	require.Equal(t, enteredRuntime.PlanHash, statusRuntime.PlanHash)
	require.False(t, statusRuntime.UpdatedAt.IsZero())

	statusGit, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, statusGit.UpdatedAt.IsZero())
	requireGitScopeState(t, statusGit.OrbitProjectionState, 1, "docs/guide.md")
	requireGitScopeState(t, statusGit.OrbitStageState, 0)
	requireGitScopeState(t, statusGit.OrbitCommitState, 1, "docs/guide.md")
	requireGitScopeState(t, statusGit.OrbitExportState, 1, "docs/guide.md")
	requireGitScopeState(t, statusGit.OrbitOrchestrationState, 0)
	requireGitScopeState(t, statusGit.GlobalStageState, 0)
	requireGitScopeState(t, statusGit.GlobalCommitState, 2, "docs/guide.md", "src/main.go")

	stdout, stderr, err = executeCLI(t, repo.Root, "leave")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "left orbit docs\n", stdout)

	leftRuntime, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", leftRuntime.Orbit)
	require.False(t, leftRuntime.Running)
	require.Equal(t, "left", leftRuntime.Phase)
	require.Equal(t, enteredRuntime.EnteredAt, leftRuntime.EnteredAt)
	require.Equal(t, enteredRuntime.PlanHash, leftRuntime.PlanHash)
	require.False(t, leftRuntime.UpdatedAt.IsZero())

	leftGit, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, leftGit.UpdatedAt.IsZero())
	requireGitScopeState(t, leftGit.OrbitProjectionState, 0)
	requireGitScopeState(t, leftGit.OrbitStageState, 0)
	requireGitScopeState(t, leftGit.OrbitCommitState, 0)
	requireGitScopeState(t, leftGit.OrbitExportState, 0)
	requireGitScopeState(t, leftGit.OrbitOrchestrationState, 0)
	requireGitScopeState(t, leftGit.GlobalStageState, 0)
	requireGitScopeState(t, leftGit.GlobalCommitState, 2, "docs/guide.md", "src/main.go")
}

func TestStatusWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "current: none\n", stdout)
}

func TestStatusJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated\n")
	require.NoError(t, os.Remove(filepath.Join(repo.Root, "docs", "old.md")))
	repo.WriteFile(t, "docs/draft.md", "draft\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: updated docs orbit\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "src/main.go", "package main\n// changed\n")
	repo.WriteFile(t, "scratch/outside.txt", "outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string                   `json:"current_orbit"`
		Snapshot     *statepkg.StatusSnapshot `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.NotNil(t, output.Snapshot)
	require.Len(t, output.Snapshot.InScope, 4)
	require.Len(t, output.Snapshot.OutOfScope, 2)
	require.Equal(t, []string{"src/main.go"}, output.Snapshot.HiddenDirtyRisk)
	require.False(t, output.Snapshot.SafeToSwitch)
	require.Equal(t, []string{"outside changes are present; orbit commit will only include the current orbit scope"}, output.Snapshot.CommitWarnings)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "current_orbit", "snapshot")
	snapshotRaw := requireNestedJSONMap(t, raw, "snapshot")
	requireJSONKeys(t, snapshotRaw, "commit_warnings", "created_at", "current_orbit", "hidden_dirty_risk", "in_scope", "out_of_scope", "safe_to_switch")
}

func TestStatusWarnsWhenLedgerRefreshFails(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated\n")
	conflictPath := filepath.Join(repo.GitDir(t), "orbit", "state", "orbits", "docs", "git_state.json")
	replacePathWithDirectory(t, conflictPath)

	stdout, stderr, err := executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Contains(t, stdout, "current: docs\n")
	require.Contains(t, stdout, "M docs/guide.md\n")
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit status:")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "status", runtimeSnapshot.Phase)

	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh runtime and git ledger for orbit status:")
}

func TestStatusJSONOutputStableWhenStatusSnapshotRefreshFails(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated\n")
	conflictPath := filepath.Join(repo.GitDir(t), "orbit", "state", "last_status.json")
	replacePathWithDirectory(t, conflictPath)

	stdout, stderr, err := executeCLI(t, repo.Root, "status", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: failed to refresh status snapshot for orbit status:")

	var output struct {
		CurrentOrbit string                   `json:"current_orbit"`
		Snapshot     *statepkg.StatusSnapshot `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.NotNil(t, output.Snapshot)
	require.Len(t, output.Snapshot.InScope, 1)
	require.Equal(t, "docs/guide.md", output.Snapshot.InScope[0].Path)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "current_orbit", "snapshot")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "status", runtimeSnapshot.Phase)

	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh status snapshot for orbit status:")
}

func TestStatusJSONOutputIncludesRoleAwareClassificationForMemberSchema(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)

	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"shared_scope:\n"+
		"  - README.md\n"+
		"projection_visible:\n"+
		"  - docs/process/**\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")
	repo.AddAndCommit(t, "add member schema orbit", ".orbit/config.yaml", ".harness/orbits/docs.yaml")
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, ".markdownlint.yaml", "heading-style = atx\n")
	repo.WriteFile(t, "docs/guide.md", "updated\n")
	repo.WriteFile(t, "docs/process/flow.md", "updated flow\n")
	repo.WriteFile(t, "README.md", "updated readme\n")
	repo.WriteFile(t, "docs/process/draft.md", "draft\n")
	repo.WriteFile(t, "scratch/outside.txt", "outside\n")
	repo.WriteFile(t, "src/main.go", "package main\n// changed\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string                   `json:"current_orbit"`
		Snapshot     *statepkg.StatusSnapshot `json:"snapshot"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.NotNil(t, output.Snapshot)

	require.Contains(t, output.Snapshot.InScope, statepkg.PathChange{
		Path:          ".markdownlint.yaml",
		Code:          "M",
		Tracked:       true,
		InScope:       true,
		Role:          orbitpkg.PathRoleRule,
		Projection:    true,
		OrbitWrite:    true,
		Export:        true,
		Orchestration: true,
	})
	require.Contains(t, output.Snapshot.InScope, statepkg.PathChange{
		Path:          "docs/guide.md",
		Code:          "M",
		Tracked:       true,
		InScope:       true,
		Role:          orbitpkg.PathRoleSubject,
		Projection:    true,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: false,
	})
	require.Contains(t, output.Snapshot.InScope, statepkg.PathChange{
		Path:          "docs/process/flow.md",
		Code:          "M",
		Tracked:       true,
		InScope:       true,
		Role:          orbitpkg.PathRoleProcess,
		Projection:    true,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: true,
	})
	require.Contains(t, output.Snapshot.OutOfScope, statepkg.PathChange{
		Path:          "src/main.go",
		Code:          "M",
		Tracked:       true,
		InScope:       false,
		Role:          orbitpkg.PathRoleOutside,
		Projection:    false,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: false,
	})

	raw := decodeJSONMap(t, stdout)
	snapshotRaw := requireNestedJSONMap(t, raw, "snapshot")
	inScopeRaw := requireJSONArray(t, snapshotRaw, "in_scope")
	firstInScope := requireJSONArrayObject(t, inScopeRaw, 0)
	requireJSONKeys(
		t,
		firstInScope,
		"code",
		"export",
		"in_scope",
		"orbit_write",
		"orchestration",
		"path",
		"projection",
		"role",
		"tracked",
	)
}

func TestStatusWritesFileInventoryLedgerForMemberSchema(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - README.md\n"+
		"projection_visible:\n"+
		"  - docs/process/**\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	commitDocsMemberSchemaOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	_, stderr, err := executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	inventory, err := store.ReadFileInventorySnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", inventory.Orbit)
	require.NotZero(t, inventory.GeneratedAt)
	require.Contains(t, inventory.Files, statepkg.FileInventoryEntry{
		Path:          ".markdownlint.yaml",
		MemberName:    "docs-rules",
		Role:          orbitpkg.PathRoleRule,
		Projection:    true,
		OrbitWrite:    true,
		Export:        true,
		Orchestration: true,
	})
	require.Contains(t, inventory.Files, statepkg.FileInventoryEntry{
		Path:          "docs/guide.md",
		MemberName:    "docs-content",
		Role:          orbitpkg.PathRoleSubject,
		Projection:    true,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: false,
	})
	require.Contains(t, inventory.Files, statepkg.FileInventoryEntry{
		Path:          "docs/process/flow.md",
		MemberName:    "docs-process",
		Role:          orbitpkg.PathRoleProcess,
		Projection:    true,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: true,
	})
	require.Contains(t, inventory.Files, statepkg.FileInventoryEntry{
		Path:          "README.md",
		Role:          orbitpkg.PathRoleSubject,
		Projection:    true,
		OrbitWrite:    false,
		Export:        false,
		Orchestration: false,
	})
}

func TestStatusShowsInScopeAndOutOfScopeChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/old.md", "old\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))

	repo.WriteFile(t, "docs/guide.md", "updated\n")
	require.NoError(t, os.Remove(filepath.Join(repo.Root, "docs", "old.md")))
	repo.WriteFile(t, "docs/draft.md", "draft\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: updated docs orbit\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "src/main.go", "package main\n// changed\n")
	repo.WriteFile(t, "scratch/outside.txt", "outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "current: docs\n")
	require.Contains(t, stdout, "in-scope:\n")
	require.Contains(t, stdout, "M docs/guide.md\n")
	require.Contains(t, stdout, "D docs/old.md\n")
	require.Contains(t, stdout, "?? docs/draft.md\n")
	require.Contains(t, stdout, "M .harness/orbits/docs.yaml\n")
	require.Contains(t, stdout, "out-of-scope:\n")
	require.Contains(t, stdout, "M src/main.go\n")
	require.Contains(t, stdout, "?? scratch/outside.txt\n")
	require.Contains(t, stdout, "hidden-dirty-risk:\nsrc/main.go\n")
	require.Contains(t, stdout, "safe-to-switch: false\n")
	require.Contains(t, stdout, "commit-warnings:\noutside changes are present; orbit commit will only include the current orbit scope\n")

	snapshot, err := store.ReadStatusSnapshot()
	require.NoError(t, err)
	require.Equal(t, "docs", snapshot.CurrentOrbit)
	require.Equal(t, []string{"src/main.go"}, snapshot.HiddenDirtyRisk)
}

func TestDiffShowsOnlyCurrentScopeChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: updated docs orbit\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "src/main.go", "package main\n// outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "diff")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "diff --git a/.harness/orbits/docs.yaml b/.harness/orbits/docs.yaml")
	require.Contains(t, stdout, "diff --git a/docs/guide.md b/docs/guide.md")
	require.NotContains(t, stdout, "src/main.go")
}

func TestDiffOutsideShowsOnlyScopeOutsideChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")
	repo.WriteFile(t, "src/main.go", "package main\n// outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "diff", "--outside")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "diff --git a/src/main.go b/src/main.go")
	require.NotContains(t, stdout, "docs/guide.md")
}

func TestLogShowsOnlyCurrentScopeHistoryAndPassesGitArgs(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, "src/main.go", "package main\n// src change\n")
	repo.AddAndCommit(t, "src change")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: docs orbit v2\ninclude:\n  - docs/**\n")
	repo.AddAndCommit(t, "docs orbit config change")
	repo.WriteFile(t, "docs/guide.md", "docs change\n")
	repo.AddAndCommit(t, "docs change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "log", "--", "--oneline", "-1")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "docs change")
	require.Contains(t, repo.Run(t, "log", "--oneline", "--", ".harness/orbits/docs.yaml"), "docs orbit config change")
	require.NotContains(t, stdout, "src change")
	require.NotContains(t, stdout, "Author:")
}

func TestDiffJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")
	repo.WriteFile(t, "src/main.go", "package main\n// outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "diff", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string   `json:"current_orbit"`
		Outside      bool     `json:"outside"`
		Paths        []string `json:"paths"`
		Diff         string   `json:"diff"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.False(t, output.Outside)
	require.Contains(t, output.Paths, ".harness/orbits/docs.yaml")
	require.Contains(t, output.Paths, "docs/guide.md")
	require.Contains(t, output.Diff, "diff --git a/docs/guide.md b/docs/guide.md")
}

func TestLogJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, "docs/guide.md", "docs change\n")
	repo.AddAndCommit(t, "docs change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "log", "--json", "--", "--oneline", "-1")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string   `json:"current_orbit"`
		Paths        []string `json:"paths"`
		GitArgs      []string `json:"git_args"`
		Log          string   `json:"log"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.Contains(t, output.Paths, ".harness/orbits/docs.yaml")
	require.Contains(t, output.Paths, "docs/guide.md")
	require.Equal(t, []string{"--oneline", "-1"}, output.GitArgs)
	require.Contains(t, output.Log, "docs change")
}

func TestCommitJSONOutputStableWhenNothingToCommit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "src/main.go", "package main\n// outside only\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string   `json:"current_orbit"`
		ScopeCount   int      `json:"scope_count"`
		Committed    bool     `json:"committed"`
		CommitHash   string   `json:"commit_hash"`
		Warnings     []string `json:"warnings"`
		RefUpdated   bool     `json:"ref_updated"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.Equal(t, 2, output.ScopeCount)
	require.False(t, output.Committed)
	require.Empty(t, output.CommitHash)
	require.Equal(t, []string{"outside changes are present; orbit commit will only include the current orbit scope"}, output.Warnings)
	require.False(t, output.RefUpdated)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "committed", "current_orbit", "ref_updated", "scope_count", "warnings")
}

func TestCommitJSONOutputStableWhenCommitted(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit string   `json:"current_orbit"`
		ScopeCount   int      `json:"scope_count"`
		Committed    bool     `json:"committed"`
		CommitHash   string   `json:"commit_hash"`
		Warnings     []string `json:"warnings"`
		RefUpdated   bool     `json:"ref_updated"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.Equal(t, 2, output.ScopeCount)
	require.True(t, output.Committed)
	require.NotEmpty(t, output.CommitHash)
	require.Empty(t, output.Warnings)
	require.True(t, output.RefUpdated)
	require.Equal(t, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")), output.CommitHash)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "commit_hash", "committed", "current_orbit", "ref_updated", "scope_count")
}

func TestLogIncludesCurrentOrbitDefinitionHistory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: docs orbit v2\ninclude:\n  - docs/**\n")
	repo.AddAndCommit(t, "docs orbit config change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "log", "--", "--oneline", "-1")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "docs orbit config change")
}

func TestScopedReadCommandsFailWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	initializeDocsOrbit(t, repo)

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "diff",
			args: []string{"diff"},
		},
		{
			name: "log",
			args: []string{"log"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stdout, stderr, err := executeCLI(t, repo.Root, testCase.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "current orbit is not set")
		})
	}
}

func TestScopedReadCommandsFailOnStaleCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	require.NoError(t, os.Remove(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")))

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "diff",
			args: []string{"diff"},
		},
		{
			name: "log",
			args: []string{"log"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stdout, stderr, err := executeCLI(t, repo.Root, testCase.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "resolve current orbit definition")
			require.ErrorContains(t, err, "current orbit \"docs\" is stale")
		})
	}
}

func TestStatusFailsWhenCurrentOrbitLedgerPlanHashIsStale(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      write: true\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "status")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `current orbit "docs" ledger is stale`)
	require.ErrorContains(t, err, `orbit enter docs`)
}

func TestCommitFailsWhenCurrentOrbitLedgerPlanHashIsStale(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)

	_, _, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      write: true\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs update")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `current orbit "docs" ledger is stale`)
	require.ErrorContains(t, err, `orbit enter docs`)
}

func TestScopedReadCommandsFailOnDamagedCurrentOrbitState(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)

	currentPath := filepath.Join(repo.GitDir(t), "orbit", "state", "current_orbit.json")
	require.NoError(t, os.WriteFile(currentPath, []byte("{"), 0o600))

	testCases := []struct {
		name string
		args []string
	}{
		{
			name: "diff",
			args: []string{"diff"},
		},
		{
			name: "log",
			args: []string{"log"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stdout, stderr, err := executeCLI(t, repo.Root, testCase.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "read current orbit state")
			require.ErrorContains(t, err, "unmarshal current orbit state")
		})
	}
}

func TestScopedReadCommandsHandleSpecialPaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/-draft note.md", "draft\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	repo.WriteFile(t, "docs/-draft note.md", "tracked update\n")
	repo.AddAndCommit(t, "special docs change")
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/-draft note.md", "working tree update\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "diff")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "diff --git a/docs/-draft note.md b/docs/-draft note.md")

	stdout, stderr, err = executeCLI(t, repo.Root, "log", "--", "--name-only", "--format=format:%s", "-1")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "special docs change")
	require.Contains(t, stdout, "docs/-draft note.md")
}

func TestCommitFailsWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	initializeDocsOrbit(t, repo)

	_, _, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.Error(t, err)
	require.ErrorContains(t, err, "current orbit is not set")
}

func TestCommitReturnsNothingToCommitWhenScopeHasNoChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	beforeHead := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "src/main.go", "package main\n// outside only\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "nothing to commit\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")
	require.Equal(t, beforeHead, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")))
	require.Contains(t, repo.Run(t, "status", "--porcelain=v1"), " M src/main.go")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Equal(t, []string{"outside changes are present; orbit commit will only include the current orbit scope"}, warnings.Messages)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", runtimeSnapshot.Orbit)
	require.True(t, runtimeSnapshot.Running)
	require.Equal(t, "commit", runtimeSnapshot.Phase)
	require.Equal(t, time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), runtimeSnapshot.EnteredAt)
	require.False(t, runtimeSnapshot.UpdatedAt.IsZero())
	require.NotEmpty(t, runtimeSnapshot.PlanHash)

	gitSnapshot, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, gitSnapshot.UpdatedAt.IsZero())
	requireGitScopeState(t, gitSnapshot.OrbitProjectionState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitStageState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitCommitState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitExportState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitOrchestrationState, 0)
	requireGitScopeState(t, gitSnapshot.GlobalStageState, 0)
	requireGitScopeState(t, gitSnapshot.GlobalCommitState, 1, "src/main.go")
}

func TestCommitWarnsWhenLedgerRefreshFailsAfterPrimaryCommit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	installPostCommitHook(t, repo, ""+
		"target=\"$(git rev-parse --absolute-git-dir)/orbit/state/orbits/docs/runtime_state.json\"\n"+
		"rm -f \"$target\"\n"+
		"mkdir -p \"$target\"\n",
	)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit commit:")

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")

	head := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	lastScoped := strings.TrimSpace(repo.Run(t, "rev-parse", "refs/orbits/docs/last-scoped"))
	require.Equal(t, head, lastScoped)

	runtimeStatePath := filepath.Join(repo.GitDir(t), "orbit", "state", "orbits", "docs", "runtime_state.json")
	runtimeStateInfo, err := os.Stat(runtimeStatePath)
	require.NoError(t, err)
	require.True(t, runtimeStateInfo.IsDir())

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh runtime and git ledger for orbit commit:")
}

func TestCommitOnlyIncludesCurrentScopeAndAppendsTrailer(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.WriteFile(t, "src/extra.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ndescription: updated docs orbit\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/new.md", "new docs file\n")
	repo.WriteFile(t, "src/main.go", "package main\n// staged outside\n")
	repo.Run(t, "add", "--", "src/main.go")
	repo.WriteFile(t, "src/extra.go", "package main\n// unstaged outside\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")

	commitMessage := repo.Run(t, "log", "-1", "--pretty=%B")
	require.Contains(t, commitMessage, "docs commit")
	require.Contains(t, commitMessage, "Orbit: docs")

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, ".harness/orbits/docs.yaml")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.Contains(t, changedFiles, "docs/new.md")
	require.NotContains(t, changedFiles, "src/main.go")
	require.NotContains(t, changedFiles, "src/extra.go")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, "M  src/main.go")
	require.Contains(t, status, " M src/extra.go")

	head := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	lastScoped := strings.TrimSpace(repo.Run(t, "rev-parse", "refs/orbits/docs/last-scoped"))
	require.Equal(t, head, lastScoped)
}

func TestCommitIncludesInScopeUntrackedFromEnteredOrbitView(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	repo.WriteFile(t, "docs/guide.md", "updated docs\n")
	repo.WriteFile(t, "docs/new.md", "new docs file\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "committed orbit docs\n", stdout)

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.Contains(t, changedFiles, "docs/new.md")
	require.NotContains(t, changedFiles, "src/main.go")
}

func TestCommitDoesNotIncludeGlobalOrbitConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: false\n"+
		"  sparse_checkout_mode: no-cone\n",
	)
	repo.Run(t, "add", "--", ".orbit/config.yaml")
	repo.WriteFile(t, "docs/guide.md", "updated docs\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, ".orbit/config.yaml")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, ".orbit/config.yaml")
}

func TestProjectionVisibleFilesEnterOrbitAndStatusButStayOutOfScopedCommitAndDiff(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	commitDocsOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "files", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var filesOutput struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &filesOutput))
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
	}, filesOutput.Files)

	stdout, stderr, err = executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (3 file(s))\n", stdout)

	repo.WriteFile(t, "README.md", "updated readme\n")
	repo.WriteFile(t, "docs/guide.md", "updated docs\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "in-scope:\n")
	require.Contains(t, stdout, "M README.md\n")
	require.Contains(t, stdout, "M docs/guide.md\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "diff")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "diff --git a/docs/guide.md b/docs/guide.md")
	require.NotContains(t, stdout, "README.md")

	stdout, stderr, err = executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, "README.md")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, " M README.md")
}

func TestProjectionVisibleFilesStayOutOfScopedLogAndRestore(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "readme v1\n")
	repo.WriteFile(t, "docs/guide.md", "docs v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	commitDocsOrbitConfig(t, repo)

	baseRevision := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "README.md", "readme v2\n")
	repo.AddAndCommit(t, "readme change")
	repo.WriteFile(t, "docs/guide.md", "docs v2\n")
	repo.AddAndCommit(t, "docs change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "log", "--", "--oneline")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "docs change")
	require.NotContains(t, stdout, "readme change")

	stdout, stderr, err = executeCLI(t, repo.Root, "restore", "--to", baseRevision)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "restored orbit docs to "+baseRevision+"\n", stdout)

	readmeData, err := os.ReadFile(filepath.Join(repo.Root, "README.md"))
	require.NoError(t, err)
	require.Equal(t, "readme v2\n", string(readmeData))

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "docs v1\n", string(guideData))
}

func TestMemberSchemaFilesAndStatusUseProjectionButCommitUsesOrbitWritePaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "files", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var filesOutput struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &filesOutput))
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		".markdownlint.yaml",
		"docs/guide.md",
		"docs/process/flow.md",
	}, filesOutput.Files)

	stdout, stderr, err = executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (4 file(s))\n", stdout)

	repo.WriteFile(t, ".markdownlint.yaml", "heading-style = atx\n")
	repo.WriteFile(t, "docs/guide.md", "updated guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "updated flow\n")
	repo.WriteFile(t, "docs/new.md", "new doc\n")
	repo.WriteFile(t, "docs/process/draft.md", "draft\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "M .markdownlint.yaml\n")
	require.Contains(t, stdout, "M docs/guide.md\n")
	require.Contains(t, stdout, "M docs/process/flow.md\n")
	require.Contains(t, stdout, "?? docs/new.md\n")
	require.Contains(t, stdout, "?? docs/process/draft.md\n")

	stdout, stderr, err = executeCLI(t, repo.Root, "diff")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "diff --git a/.markdownlint.yaml b/.markdownlint.yaml")
	require.NotContains(t, stdout, "docs/guide.md")
	require.NotContains(t, stdout, "docs/process/flow.md")

	stdout, stderr, err = executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, ".markdownlint.yaml")
	require.NotContains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, "docs/process/flow.md")
	require.NotContains(t, changedFiles, "docs/new.md")
	require.NotContains(t, changedFiles, "docs/process/draft.md")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, " M docs/guide.md")
	require.Contains(t, status, " M docs/process/flow.md")
	require.Contains(t, status, "?? docs/new.md")
	require.Contains(t, status, "?? docs/process/draft.md")
}

func TestMemberSchemaCommitUpdatesRuntimeAndGitLedger(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, ".markdownlint.yaml", "heading-style = atx\n")
	repo.WriteFile(t, "docs/guide.md", "updated guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "updated flow\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "commit", "-m", "docs commit")
	require.NoError(t, err)
	require.Equal(t, "committed orbit docs\n", stdout)
	require.Contains(t, stderr, "warning: outside changes are present; orbit commit will only include the current orbit scope")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", runtimeSnapshot.Orbit)
	require.True(t, runtimeSnapshot.Running)
	require.Equal(t, "commit", runtimeSnapshot.Phase)
	require.Equal(t, time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), runtimeSnapshot.EnteredAt)
	require.False(t, runtimeSnapshot.UpdatedAt.IsZero())
	require.NotEmpty(t, runtimeSnapshot.PlanHash)

	gitSnapshot, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, gitSnapshot.UpdatedAt.IsZero())
	requireGitScopeState(t, gitSnapshot.OrbitProjectionState, 2, "docs/guide.md", "docs/process/flow.md")
	requireGitScopeState(t, gitSnapshot.OrbitStageState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitCommitState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitExportState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitOrchestrationState, 1, "docs/process/flow.md")
	requireGitScopeState(t, gitSnapshot.GlobalStageState, 0)
	requireGitScopeState(t, gitSnapshot.GlobalCommitState, 2, "docs/guide.md", "docs/process/flow.md")
}

func TestMemberSchemaLogAndRestoreUseOrbitWritePaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide v1\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)
	baseRevision := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "guide v2\n")
	repo.AddAndCommit(t, "subject change")
	repo.WriteFile(t, "docs/process/flow.md", "flow v2\n")
	repo.AddAndCommit(t, "process change")
	repo.WriteFile(t, ".markdownlint.yaml", "heading-style = atx\n")
	repo.AddAndCommit(t, "rule change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "log", "--", "--oneline")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "rule change")
	require.NotContains(t, stdout, "subject change")
	require.NotContains(t, stdout, "process change")

	stdout, stderr, err = executeCLI(t, repo.Root, "restore", "--to", baseRevision)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "restored orbit docs to "+baseRevision+"\n", stdout)

	require.Equal(t, "line-length = false\n", string(mustReadFile(t, filepath.Join(repo.Root, ".markdownlint.yaml"))))
	require.Equal(t, "guide v2\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))
	require.Equal(t, "flow v2\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "process", "flow.md"))))

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, ".markdownlint.yaml")
	require.NotContains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, "docs/process/flow.md")
}

func TestMemberSchemaRestoreUpdatesRuntimeAndGitLedger(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "guide v1\n")
	repo.WriteFile(t, "docs/process/flow.md", "flow v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsMemberSchemaOrbit(t, repo)
	commitDocsMemberSchemaOrbitConfig(t, repo)
	baseRevision := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "guide v2\n")
	repo.AddAndCommit(t, "subject change")
	repo.WriteFile(t, "docs/process/flow.md", "flow v2\n")
	repo.AddAndCommit(t, "process change")
	repo.WriteFile(t, ".markdownlint.yaml", "heading-style = atx\n")
	repo.AddAndCommit(t, "rule change")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "restore", "--to", baseRevision)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "restored orbit docs to "+baseRevision+"\n", stdout)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)

	runtimeSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, "docs", runtimeSnapshot.Orbit)
	require.True(t, runtimeSnapshot.Running)
	require.Equal(t, "restore", runtimeSnapshot.Phase)
	require.Equal(t, time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC), runtimeSnapshot.EnteredAt)
	require.False(t, runtimeSnapshot.UpdatedAt.IsZero())
	require.NotEmpty(t, runtimeSnapshot.PlanHash)

	gitSnapshot, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.False(t, gitSnapshot.UpdatedAt.IsZero())
	requireGitScopeState(t, gitSnapshot.OrbitProjectionState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitStageState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitCommitState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitExportState, 0)
	requireGitScopeState(t, gitSnapshot.OrbitOrchestrationState, 0)
	requireGitScopeState(t, gitSnapshot.GlobalStageState, 0)
	requireGitScopeState(t, gitSnapshot.GlobalCommitState, 0)
}

func TestRestoreFailsWithoutCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	initializeDocsOrbit(t, repo)

	_, _, err := executeCLI(t, repo.Root, "restore", "--to", "HEAD~1")
	require.Error(t, err)
	require.ErrorContains(t, err, "current orbit is not set")
}

func TestRestoreFailsWhenScopeHasDirtyChanges(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "docs/guide.md", "dirty docs\n")

	_, _, err := executeCLI(t, repo.Root, "restore", "--to", "HEAD~1")
	require.Error(t, err)
	require.ErrorContains(t, err, "scope contains uncommitted changes")
	require.ErrorContains(t, err, "docs/guide.md")
}

func TestRestoreCreatesNewCommitAndUpdatesRef(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	restoreTarget := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")
	setDocsCurrentOrbit(t, repo)

	repo.WriteFile(t, "src/main.go", "package main\n// outside local change\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "restore", "--to", restoreTarget)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("restored orbit docs to %s\n", restoreTarget), stdout)
	require.Empty(t, stderr)

	require.Equal(t, "v1\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))

	commitMessage := repo.Run(t, "log", "-1", "--pretty=%B")
	require.Contains(t, commitMessage, fmt.Sprintf("restore docs orbit to %s", restoreTarget))
	require.Contains(t, commitMessage, "Orbit: docs")
	require.Contains(t, commitMessage, fmt.Sprintf("Orbit-Restore-From: %s", restoreTarget))

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, "src/main.go")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, " M src/main.go")

	head := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	lastRestore := strings.TrimSpace(repo.Run(t, "rev-parse", "refs/orbits/docs/last-restore"))
	require.Equal(t, head, lastRestore)
	require.NotEqual(t, restoreTarget, head)
}

func TestRestoreWarnsWhenLedgerRefreshFailsAfterPrimaryRestoreCommit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	restoreTarget := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")
	setDocsCurrentOrbit(t, repo)

	installPostCommitHook(t, repo, ""+
		"target=\"$(git rev-parse --absolute-git-dir)/orbit/state/orbits/docs/runtime_state.json\"\n"+
		"rm -f \"$target\"\n"+
		"mkdir -p \"$target\"\n",
	)

	stdout, stderr, err := executeCLI(t, repo.Root, "restore", "--to", restoreTarget)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("restored orbit docs to %s\n", restoreTarget), stdout)
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit restore:")

	require.Equal(t, "v1\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))

	head := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	lastRestore := strings.TrimSpace(repo.Run(t, "rev-parse", "refs/orbits/docs/last-restore"))
	require.Equal(t, head, lastRestore)
	require.NotEqual(t, restoreTarget, head)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 1)
	require.Contains(t, warnings.Messages[0], "failed to refresh runtime and git ledger for orbit restore:")
}

func TestRestoreFailsWhenTargetPredatesCurrentOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	setDocsCurrentOrbit(t, repo)

	targetBeforeOrbitBirth := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD~1"))

	_, _, err := executeCLI(t, repo.Root, "restore", "--to", targetBeforeOrbitBirth)
	require.Error(t, err)
	require.ErrorContains(t, err, "does not exist at target revision")
	require.ErrorContains(t, err, "--allow-delete-current-orbit")
}

func TestRestoreAllowDeleteCurrentOrbitAutoLeaves(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	targetBeforeOrbitBirth := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD~1"))

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")

	stdout, stderr, err = executeCLI(t, repo.Root, "restore", "--to", targetBeforeOrbitBirth, "--allow-delete-current-orbit")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("restored orbit docs to %s\n", targetBeforeOrbitBirth), stdout)
	require.Contains(t, stderr, "warning: current orbit \"docs\" does not exist")
	require.Contains(t, stderr, "automatically left orbit view")

	require.Equal(t, "v1\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))
	require.FileExists(t, filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)

	sparseSetting := strings.TrimSpace(repo.Run(t, "config", "--bool", "--default", "false", "core.sparseCheckout"))
	require.Equal(t, "false", sparseSetting)

	commitMessage := repo.Run(t, "log", "-1", "--pretty=%B")
	require.Contains(t, commitMessage, "Orbit: docs")
	require.Contains(t, commitMessage, fmt.Sprintf("Orbit-Restore-From: %s", targetBeforeOrbitBirth))
}

func TestRestoreAllowDeleteCurrentOrbitWarnsWhenLedgerRefreshFailsAfterAutoLeave(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	targetBeforeOrbitBirth := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD~1"))

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.NoFileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")

	installPostCommitHook(t, repo, ""+
		"target=\"$(git rev-parse --absolute-git-dir)/orbit/state/orbits/docs/runtime_state.json\"\n"+
		"rm -f \"$target\"\n"+
		"mkdir -p \"$target\"\n",
	)

	stdout, stderr, err = executeCLI(t, repo.Root, "restore", "--to", targetBeforeOrbitBirth, "--allow-delete-current-orbit")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("restored orbit docs to %s\n", targetBeforeOrbitBirth), stdout)
	require.Contains(t, stderr, "warning: current orbit \"docs\" does not exist")
	require.Contains(t, stderr, "automatically left orbit view")
	require.Contains(t, stderr, "warning: failed to refresh runtime and git ledger for orbit restore:")

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)

	require.Equal(t, "v1\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))
	require.FileExists(t, filepath.Join(repo.Root, "src", "main.go"))

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	warnings, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, "docs", warnings.CurrentOrbit)
	require.Len(t, warnings.Messages, 2)
	require.Contains(t, warnings.Messages[0], `current orbit "docs" does not exist`)
	require.Contains(t, warnings.Messages[1], "failed to refresh runtime and git ledger for orbit restore:")
}

func TestRestoreDoesNotRestoreGlobalOrbitConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	restoreTarget := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")
	setDocsCurrentOrbit(t, repo)

	expectedConfig := "" +
		"version: 1\n" +
		"shared_scope:\n" +
		"  - README.md\n" +
		"behavior:\n" +
		"  outside_changes_mode: warn\n" +
		"  block_switch_if_hidden_dirty: false\n" +
		"  commit_append_trailer: true\n" +
		"  sparse_checkout_mode: no-cone\n"
	repo.WriteFile(t, ".orbit/config.yaml", expectedConfig)

	stdout, stderr, err := executeCLI(t, repo.Root, "restore", "--to", restoreTarget)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("restored orbit docs to %s\n", restoreTarget), stdout)
	require.Empty(t, stderr)

	require.Equal(t, "v1\n", string(mustReadFile(t, filepath.Join(repo.Root, "docs", "guide.md"))))
	require.Equal(t, expectedConfig, string(mustReadFile(t, filepath.Join(repo.Root, ".orbit", "config.yaml"))))

	changedFiles := repo.Run(t, "show", "--name-only", "--pretty=format:", "HEAD")
	require.Contains(t, changedFiles, "docs/guide.md")
	require.NotContains(t, changedFiles, ".orbit/config.yaml")

	status := repo.Run(t, "status", "--porcelain=v1")
	require.Contains(t, status, ".orbit/config.yaml")
}

func TestRestoreJSONOutputStable(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	restoreTarget := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")
	setDocsCurrentOrbit(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "restore", "--to", restoreTarget, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit   string   `json:"current_orbit"`
		TargetRevision string   `json:"target_revision"`
		ScopeCount     int      `json:"scope_count"`
		CommitHash     string   `json:"commit_hash"`
		Warnings       []string `json:"warnings"`
		RefUpdated     bool     `json:"ref_updated"`
		AutoLeft       bool     `json:"auto_left"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.Equal(t, restoreTarget, output.TargetRevision)
	require.Equal(t, 2, output.ScopeCount)
	require.NotEmpty(t, output.CommitHash)
	require.Empty(t, output.Warnings)
	require.True(t, output.RefUpdated)
	require.False(t, output.AutoLeft)
	require.Equal(t, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")), output.CommitHash)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "commit_hash", "current_orbit", "ref_updated", "scope_count", "target_revision")
}

func TestRestoreJSONOutputStableWhenAutoLeaving(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "hello\n")
	repo.WriteFile(t, "docs/guide.md", "v1\n")
	repo.WriteFile(t, "src/main.go", "package main\n")
	repo.AddAndCommit(t, "initial commit")

	initializeDocsOrbit(t, repo)
	commitDocsOrbitConfig(t, repo)
	targetBeforeOrbitBirth := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD~1"))

	stdout, stderr, err := executeCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)

	repo.WriteFile(t, "docs/guide.md", "v2\n")
	repo.AddAndCommit(t, "docs v2")

	stdout, stderr, err = executeCLI(t, repo.Root, "restore", "--to", targetBeforeOrbitBirth, "--allow-delete-current-orbit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var output struct {
		CurrentOrbit   string   `json:"current_orbit"`
		TargetRevision string   `json:"target_revision"`
		ScopeCount     int      `json:"scope_count"`
		CommitHash     string   `json:"commit_hash"`
		Warnings       []string `json:"warnings"`
		RefUpdated     bool     `json:"ref_updated"`
		AutoLeft       bool     `json:"auto_left"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &output))
	require.Equal(t, "docs", output.CurrentOrbit)
	require.Equal(t, targetBeforeOrbitBirth, output.TargetRevision)
	require.Equal(t, 2, output.ScopeCount)
	require.NotEmpty(t, output.CommitHash)
	require.Len(t, output.Warnings, 1)
	require.Contains(t, output.Warnings[0], `current orbit "docs" does not exist`)
	require.Contains(t, output.Warnings[0], "automatically left orbit view")
	require.True(t, output.RefUpdated)
	require.True(t, output.AutoLeft)
	require.Equal(t, strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")), output.CommitHash)

	raw := decodeJSONMap(t, stdout)
	requireJSONKeys(t, raw, "auto_left", "commit_hash", "current_orbit", "ref_updated", "scope_count", "target_revision", "warnings")
}

func initializeDocsOrbit(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
}

func addDocsSubjectMember(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	_, _, err := executeCLI(
		t,
		repo.Root,
		"member",
		"add",
		"--orbit", "docs",
		"--key", "docs-content",
		"--role", "subject",
		"--include", "docs/**",
	)
	require.NoError(t, err)
}

func initializeDocsMemberSchemaOrbit(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	_, _, err := executeCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeCLI(t, repo.Root, "add", "docs", "--member-schema")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .markdownlint.yaml\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")
}

func commitDocsOrbitConfig(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.AddAndCommit(t, "add orbit config", ".orbit/config.yaml", ".harness/orbits/docs.yaml")
}

func commitDocsMemberSchemaOrbitConfig(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.AddAndCommit(t, "add member schema orbit config", ".orbit/config.yaml", ".harness/orbits/docs.yaml")
}

func setDocsCurrentOrbit(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))
}

func seedBriefBackfillRevisionRepo(t *testing.T, revisionKind string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = ""
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
			"root_guidance:\n"+
			"  agents: false\n"+
			"  humans: false\n"+
			"  bootstrap: false\n")
	}

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"You are the Acme docs orbit.\n"+
		"Keep release notes current.\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed brief backfill revision repo")

	return repo
}

func seedBriefMaterializeRevisionRepo(t *testing.T, revisionKind string, agentsTemplate string, rootAgents string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = agentsTemplate
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
			"root_guidance:\n"+
			"  agents: false\n"+
			"  humans: false\n"+
			"  bootstrap: false\n")
	}

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n")
	if rootAgents != "" {
		repo.WriteFile(t, "AGENTS.md", rootAgents)
	}
	repo.AddAndCommit(t, "seed brief materialize revision repo")

	return repo
}

func requireGitScopeState(t *testing.T, actual statepkg.GitScopeState, count int, paths ...string) {
	t.Helper()

	require.Equal(t, count, actual.Count)
	if len(paths) == 0 {
		require.Empty(t, actual.Paths)
		return
	}

	require.Equal(t, paths, actual.Paths)
}

func seedRuntimeRepoWithDormantHostedDefinition(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC)

	repo.WriteFile(t, "docs/guide.md", "guide\n")
	repo.WriteFile(t, "tools/notes.md", "notes\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/tools.yaml", ""+
		"id: tools\n"+
		"description: tools orbit\n"+
		"include:\n"+
		"  - tools/**\n")
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.ManifestMember{
			{
				OrbitID: "docs",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed runtime repo with dormant definition")

	return repo
}

func seedRuntimeRepoWithMissingActiveDefinition(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC)

	repo.WriteFile(t, "tools/notes.md", "notes\n")
	repo.WriteFile(t, ".harness/orbits/tools.yaml", ""+
		"id: tools\n"+
		"description: tools orbit\n"+
		"include:\n"+
		"  - tools/**\n")
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.ManifestMember{
			{
				OrbitID: "docs",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed runtime repo with missing active definition")

	return repo
}

func executeCLI(t *testing.T, workingDir string, args ...string) (string, string, error) {
	return executeCLIWithInput(t, workingDir, "", args...)
}

func executeCLIWithInput(t *testing.T, workingDir string, stdin string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := cli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(stdin))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(commands.WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func decodeJSONMap(t *testing.T, stdout string) map[string]any {
	t.Helper()

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))

	return raw
}

func requireJSONKeys(t *testing.T, raw map[string]any, expectedKeys ...string) {
	t.Helper()

	actualKeys := make([]string, 0, len(raw))
	for key := range raw {
		actualKeys = append(actualKeys, key)
	}

	sort.Strings(actualKeys)
	sort.Strings(expectedKeys)
	require.Equal(t, expectedKeys, actualKeys)
}

func requireNestedJSONMap(t *testing.T, raw map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := raw[key]
	require.True(t, ok)

	nested, ok := value.(map[string]any)
	require.True(t, ok)

	return nested
}

func requireJSONArray(t *testing.T, raw map[string]any, key string) []any {
	t.Helper()

	value, ok := raw[key]
	require.True(t, ok)

	array, ok := value.([]any)
	require.True(t, ok)

	return array
}

func installPostCommitHook(t *testing.T, repo *testutil.Repo, scriptBody string) {
	t.Helper()

	hookPath := filepath.Join(repo.GitDir(t), "hooks", "post-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\nset -eu\n"+scriptBody), 0o755))
}

func replacePathWithDirectory(t *testing.T, absolutePath string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(absolutePath))
	require.NoError(t, os.MkdirAll(absolutePath, 0o755))
}

func requireJSONArrayObject(t *testing.T, values []any, index int) map[string]any {
	t.Helper()

	require.Greater(t, len(values), index)

	object, ok := values[index].(map[string]any)
	require.True(t, ok)

	return object
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
