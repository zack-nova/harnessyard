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

func TestTemplateApplyRemoteGitWritesRuntimeFilesAndDoesNotEnter(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"+
		"    description: Bound title\n"), 0o600))

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Applied Remote Orbit guide\n", string(guideData))

	installData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "installs", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(installData), "source_kind: external_git")
	require.Contains(t, string(installData), "source_repo: "+remoteURL)
	require.Contains(t, string(installData), "source_ref: orbit-template/docs")
	requireRemoteInstallStateStaysVersioned(t, runtimeRepo, "docs")
	requireNoRemoteTempRefs(t, runtimeRepo)

	currentStdout, currentStderr, currentErr := executeCLI(t, runtimeRepo.Root, "current")
	require.NoError(t, currentErr)
	require.Empty(t, currentStderr)
	require.Equal(t, "no current orbit\n", currentStdout)
}

func TestTemplateApplyRemoteGitReplacesSharedAgentsBlockInPlace(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepoWithSharedAgents(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)
	runtimeRepo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"old docs block\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n"+
		"tail guidance\n")
	runtimeRepo.AddAndCommit(t, "seed runtime agents")

	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "warnings:")
	require.Contains(t, stdout, `runtime AGENTS.md already contains orbit block "docs"; apply will replace it in place`)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	agentsData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs orbit for Applied Remote Orbit\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n"+
		"tail guidance\n", string(agentsData))

	requireNoRemoteTempRefs(t, runtimeRepo)
}

func TestTemplateApplyRemoteGitUsesEditorForMissingBindings(t *testing.T) {
	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)

	editorScript := filepath.Join(runtimeRepo.Root, "fill-bindings.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"cat > \"$1\" <<'EOF'\n"+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Editor Orbit\n"+
		"    description: Edited title\n"+
		"EOF\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--ref", "orbit-template/docs", "--editor")
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "docs", "orbit-template/docs")

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Remote Editor Orbit guide\n", string(guideData))

	varsData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "vars.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(varsData), "Remote Editor Orbit")
	requireNoRemoteTempRefs(t, runtimeRepo)
}

func TestTemplateApplyRemoteDryRunSupportsJSONAndDoesNotWriteRuntimeFiles(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Dry Remote Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--json")
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
		OrbitID string   `json:"orbit_id"`
		Files   []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "orbit-template/docs", payload.Source.Ref)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.Files, "docs/guide.md")

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	requireNoRemoteTempRefs(t, runtimeRepo)
}

func TestTemplateApplyRemoteDryRunOverwriteExistingKeepsConflictSummaryInJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)
	runtimeRepo.WriteFile(t, "docs/guide.md", "conflicting runtime content\n")

	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Dry Remote Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun            bool `json:"dry_run"`
		OverwriteExisting bool `json:"overwrite_existing"`
		Manifest          struct {
			OrbitID           string `json:"orbit_id"`
			DefaultTemplate   bool   `json:"default_template"`
			CreatedFromBranch string `json:"created_from_branch"`
			CreatedFromCommit string `json:"created_from_commit"`
			VariableCount     int    `json:"variable_count"`
		} `json:"manifest"`
		Conflicts []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.True(t, payload.OverwriteExisting)
	require.Equal(t, "docs", payload.Manifest.OrbitID)
	require.False(t, payload.Manifest.DefaultTemplate)
	require.NotEmpty(t, payload.Manifest.CreatedFromBranch)
	require.NotEmpty(t, payload.Manifest.CreatedFromCommit)
	require.Equal(t, 1, payload.Manifest.VariableCount)
	require.Contains(t, payload.Conflicts, struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Path:    "docs/guide.md",
		Message: "target path already exists with different content",
	})
	requireNoRemoteTempRefs(t, runtimeRepo)
}

func TestTemplateApplyRemoteAutoSelectsUniqueDefaultTemplate(t *testing.T) {
	t.Parallel()

	sourceRepo := seedRemoteTemplateSourceRepo(t,
		remoteTemplateBranchSpec{OrbitID: "docs", Branch: "orbit-template/docs"},
		remoteTemplateBranchSpec{OrbitID: "api", Branch: "orbit-template/api", DefaultTemplate: true},
	)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Default Orbit\n"), 0o600))

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL, "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	requireLegacyTemplateApplySuccessOutput(t, stdout, "api", "orbit-template/api")

	apiData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "api", "openapi.yaml"))
	require.NoError(t, err)
	require.Equal(t, "Orbit api\n", string(apiData))

	installData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "installs", "api.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(installData), "source_ref: orbit-template/api")
	requireRemoteInstallStateStaysVersioned(t, runtimeRepo, "api")
	requireNoRemoteTempRefs(t, runtimeRepo)
}

func TestTemplateApplyRemoteFailsWhenNoTemplateBranchesExist(t *testing.T) {
	t.Parallel()

	sourceRepo := seedRemoteTemplateSourceRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "no valid external template branches found")
	requireNoRemoteTempRefs(t, runtimeRepo)

	_, statErr := os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "installs"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestTemplateApplyRemoteFailsWhenMultipleTemplatesRemainAmbiguous(t *testing.T) {
	t.Parallel()

	sourceRepo := seedRemoteTemplateSourceRepo(t,
		remoteTemplateBranchSpec{OrbitID: "docs", Branch: "orbit-template/docs"},
		remoteTemplateBranchSpec{OrbitID: "api", Branch: "orbit-template/api"},
	)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteTemplateApplyRuntimeRepo(t)

	stdout, stderr, err := executeCLI(t, runtimeRepo.Root, "template", "apply", remoteURL)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "ambiguous")
	require.ErrorContains(t, err, "orbit-template/api")
	require.ErrorContains(t, err, "orbit-template/docs")
	requireNoRemoteTempRefs(t, runtimeRepo)
}

func seedRemoteTemplateApplyRuntimeRepo(t *testing.T) *testutil.Repo {
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
	repo.WriteFile(t, "README.md", "runtime repo\n")
	repo.AddAndCommit(t, "seed runtime repo")

	return repo
}

type remoteTemplateBranchSpec struct {
	OrbitID         string
	Branch          string
	DefaultTemplate bool
}

func seedRemoteTemplateSourceRepo(t *testing.T, specs ...remoteTemplateBranchSpec) *testutil.Repo {
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
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	for _, spec := range specs {
		switch spec.OrbitID {
		case "docs":
			repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
				"id: docs\n"+
				"description: Docs orbit\n"+
				"include:\n"+
				"  - docs/**\n")
			repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
		case "api":
			repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
				"id: api\n"+
				"description: API orbit\n"+
				"include:\n"+
				"  - api/**\n")
			repo.WriteFile(t, "api/openapi.yaml", "Orbit api\n")
		default:
			t.Fatalf("unsupported remote template orbit fixture %q", spec.OrbitID)
		}
	}

	repo.WriteFile(t, "README.md", "remote source repo\n")
	orbitIDs := make([]string, 0, len(specs))
	for _, spec := range specs {
		orbitIDs = append(orbitIDs, spec.OrbitID)
	}
	if len(orbitIDs) > 0 {
		writeTestRuntimeManifest(t, repo, orbitIDs...)
	}
	repo.AddAndCommit(t, "seed remote source repo")

	for _, spec := range specs {
		args := []string{"template", "save", spec.OrbitID, "--to", spec.Branch}
		if spec.DefaultTemplate {
			args = append(args, "--default")
		}
		_, _, err := executeCLI(t, repo.Root, args...)
		require.NoError(t, err)
	}

	return repo
}

func requireRemoteInstallStateStaysVersioned(t *testing.T, repo *testutil.Repo, orbitID string) {
	t.Helper()

	installPath := filepath.Join(repo.Root, ".harness", "installs", orbitID+".yaml")
	_, err := os.Stat(installPath)
	require.NoError(t, err)

	gitStateInstallPath := filepath.Join(repo.GitDir(t), "orbit", "state", "installs", orbitID+".yaml")
	_, err = os.Stat(gitStateInstallPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func requireNoRemoteTempRefs(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	refs := strings.TrimSpace(repo.Run(t, "for-each-ref", "--format=%(refname)", "refs/orbits/tmp/remote-source"))
	require.Empty(t, refs)
}
