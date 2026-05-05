package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplateApplyLocalBranchWritesRuntimeFilesAndDoesNotEnter(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"+
		"    description: Bound title\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")
	require.Contains(t, stdout, "files: 4")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Applied Orbit guide\n", string(guideData))

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)
}

func TestTemplateApplyLocalBranchCreatesSharedAgentsFileWhenAbsent(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepoWithSharedAgents(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"+
		"    description: Bound title\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "warnings: none")
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")
	require.Contains(t, stdout, "files: 5")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"Docs orbit for Applied Orbit\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n", string(agentsData))
}

func TestTemplateApplyLocalBranchFailsOnMalformedRuntimeAgents(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepoWithSharedAgents(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"broken docs block\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "parse runtime AGENTS.md")
}

func TestTemplateApplyLocalBranchPreservesExecutableFileMode(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	repo.Run(t, "checkout", "--orphan", "orbit-template/executable")
	repo.Run(t, "rm", "-rf", ".")

	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".harness"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".orbit", "orbits"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".orbit", "template.yaml"), []byte(""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"), []byte(""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".orbit", "orbits", "docs.yaml"), []byte(""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "docs", "build.sh"), []byte("#!/bin/sh\necho orbit\n"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "docs", "build.sh"), 0o755))
	repo.Run(t, "add", "-A")
	repo.Run(t, "commit", "-m", "seed executable template branch")
	repo.Run(t, "checkout", currentBranch)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/executable")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/executable")
	require.Contains(t, stdout, "files: 3")

	info, statErr := os.Stat(filepath.Join(repo.Root, "docs", "build.sh"))
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&0o111)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTemplateApplyLocalBranchFailsWithoutOrbitTemplateBranchManifest(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.Run(t, "rm", ".harness/manifest.yaml")
	repo.AddAndCommit(t, "remove template branch manifest")
	repo.Run(t, "checkout", currentBranch)

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
	require.ErrorContains(t, err, "valid orbit template branch")
}

func TestTemplateApplyDryRunDoesNotWriteRuntimeFiles(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplyDryRunOutput(t, stdout, "orbit-template/docs")
	require.Contains(t, stdout, "source_ref: orbit-template/docs")
	require.Contains(t, stdout, "source_kind: local_branch")
	require.Contains(t, stdout, "source_commit: ")
	require.Contains(t, stdout, "manifest:")
	require.Contains(t, stdout, "default_template: false")
	require.Contains(t, stdout, "created_from_branch: ")
	require.Contains(t, stdout, "created_from_commit: ")
	require.Contains(t, stdout, "project_name <- bindings_file")
	require.Contains(t, stdout, "conflicts: none")

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTemplateApplyDryRunOverwriteExistingStillReportsConflicts(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	repo.WriteFile(t, "docs/guide.md", "conflicting runtime content\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--overwrite-existing")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "overwrite_existing: true")
	require.Contains(t, stdout, "conflicts:")
	require.Contains(t, stdout, "docs/guide.md: target path already exists with different content")
}

func TestTemplateApplyDefaultsToUnresolvedBindings(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))
}

func TestTemplateApplyStrictBindingsFailsOnMissingBindings(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--strict-bindings")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "missing required bindings: project_name")
}

func TestTemplateApplyInteractivePromptsForMissingBindings(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)

	stdout, stderr, err := executeCLIWithInput(t, repo.Root, "Prompted Orbit\n", "template", "apply", "orbit-template/docs", "--interactive")
	require.NoError(t, err)
	require.Contains(t, stderr, "project_name")
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Prompted Orbit guide\n", string(guideData))

	varsData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(varsData), "Prompted Orbit")

	currentStdout, currentStderr, currentErr := executeCLI(t, repo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)
}

func TestTemplateApplyEditorFillsMissingBindingsWithoutMutatingRuntimeBeforeApply(t *testing.T) {
	repo := seedTemplateApplyRepo(t)

	editorScript := filepath.Join(repo.Root, "fill-bindings.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"cat > \"$1\" <<'EOF'\n"+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Editor Orbit\n"+
		"    description: Edited title\n"+
		"EOF\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--editor")
	require.NoError(t, err)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")
	require.Empty(t, stderr)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Editor Orbit guide\n", string(guideData))

	varsData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(varsData), "Editor Orbit")
}

func TestTemplateApplyEditorSupportsQuotedEditorCommandWithSpacedPath(t *testing.T) {
	repo := seedTemplateApplyRepo(t)

	editorDir := filepath.Join(repo.Root, "tools with spaces")
	require.NoError(t, os.MkdirAll(editorDir, 0o755))
	editorScript := filepath.Join(editorDir, "fill bindings.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"if [ \"$1\" != \"--label\" ] || [ \"$2\" != \"Product Title\" ]; then\n"+
		"  exit 19\n"+
		"fi\n"+
		"cat > \"$3\" <<'EOF'\n"+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Quoted Editor Orbit\n"+
		"    description: Edited title\n"+
		"EOF\n"), 0o755))
	t.Setenv("EDITOR", "\""+editorScript+"\" --label \"Product Title\"")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--editor")
	require.NoError(t, err)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")
	require.Empty(t, stderr)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Quoted Editor Orbit guide\n", string(guideData))
}

func TestTemplateApplyFailsOnNonTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "seed plain runtime repo")
	repo.Run(t, "branch", "plain-source")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "plain-source")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "not a valid orbit template branch")
}

func TestTemplateApplyRequiresOverwriteForExistingConflicts(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	repo.WriteFile(t, "docs/guide.md", "conflicting runtime content\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "conflicts detected")

	stdout, stderr, err = executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")
}

func TestTemplateApplyRequiresOverwriteForRepoVarsConflicts(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Old Orbit\n"+
		"    description: Local title\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, ".harness/vars.yaml: target path already exists with different content")
	require.Contains(t, stdout, ".harness/vars.yaml: target path has uncommitted worktree status ??")

	stdout, stderr, err = executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "conflicts detected")

	stdout, stderr, err = executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	varsData, readErr := os.ReadFile(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoError(t, readErr)
	require.Contains(t, string(varsData), "Applied Orbit")
}

func TestTemplateApplyLocalBranchSupportsJSONResultOutput(t *testing.T) {
	t.Parallel()

	repo := seedTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "apply", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun bool `json:"dry_run"`
		Source struct {
			Kind   string `json:"kind"`
			Repo   string `json:"repo"`
			Ref    string `json:"ref"`
			Commit string `json:"commit"`
		} `json:"source"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, "local_branch", payload.Source.Kind)
	require.Empty(t, payload.Source.Repo)
	require.Equal(t, "orbit-template/docs", payload.Source.Ref)
	require.NotEmpty(t, payload.Source.Commit)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/vars.yaml")
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")
}

func requireLegacyTemplateApplyDryRunOutput(t *testing.T, stdout string, sourceRef string) {
	t.Helper()

	require.Contains(t, stdout, "legacy orbit template apply dry-run from "+sourceRef)
	require.Contains(t, stdout, "preferred_command: harness install")
}

func requireLegacyTemplateApplySuccessOutput(t *testing.T, stdout string, orbitID string, sourceRef string) {
	t.Helper()

	require.Contains(t, stdout, "legacy orbit template apply installed orbit "+orbitID+" from "+sourceRef)
	require.Contains(t, stdout, "preferred_command: harness install")
}

func seedTemplateApplyRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	writeTestRuntimeManifest(t, repo, "docs")
	repo.AddAndCommit(t, "seed runtime repo")

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", ".harness/vars.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	return repo
}

func seedTemplateApplyRepoWithSharedAgents(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "runtime guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.Run(t, "checkout", "-b", "orbit-template/docs")
	repo.Run(t, "rm", "-rf", ".")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed template branch with structured brief")
	repo.Run(t, "checkout", currentBranch)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	return repo
}
