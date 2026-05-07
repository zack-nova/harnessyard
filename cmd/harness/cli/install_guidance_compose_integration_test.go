package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessInstallDryRunPreviewIncludesScopedGuidanceArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{{
		OrbitID:           "docs",
		AgentsTemplate:    "You are the $project_name docs orbit.\n",
		HumansTemplate:    "Run the $project_name docs workflow.\n",
		BootstrapTemplate: "Bootstrap the $project_name docs orbit.\n",
		Files: map[string]string{
			"docs/guide.md": "$project_name guide\n",
		},
	}})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, "AGENTS.md")
	require.Contains(t, payload.Files, "HUMANS.md")
	require.Contains(t, payload.Files, "BOOTSTRAP.md")
}

func TestHarnessInstallDryRunPreviewIncludesDescriptionBackedAgentsArtifact(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{{
		OrbitID: "docs",
		Files: map[string]string{
			"docs/guide.md": "$project_name guide\n",
		},
	}})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, "AGENTS.md")
	require.NotContains(t, payload.Files, "HUMANS.md")
	require.NotContains(t, payload.Files, "BOOTSTRAP.md")
}

func TestHarnessInstallAutoComposesScopedGuidanceArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{{
		OrbitID:           "docs",
		AgentsTemplate:    "You are the $project_name docs orbit.\n",
		HumansTemplate:    "Run the $project_name docs workflow.\n",
		BootstrapTemplate: "Bootstrap the $project_name docs orbit.\n",
		Files: map[string]string{
			"docs/guide.md": "$project_name guide\n",
		},
	}})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")
	require.Contains(t, payload.WrittenPaths, "HUMANS.md")
	require.Contains(t, payload.WrittenPaths, "BOOTSTRAP.md")
	require.Empty(t, payload.Warnings)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the Installed Orbit docs orbit.\n")

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "Bootstrap the Installed Orbit docs orbit.\n")
}

func TestHarnessInstallAutoComposesDescriptionBackedAgentsAndKeepsHarnessCheckClean(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{{
		OrbitID: "docs",
		Files: map[string]string{
			"docs/guide.md": "$project_name guide\n",
		},
	}})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")
	require.Empty(t, payload.Warnings)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "docs orbit\n")

	checkStdout, checkStderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, checkStderr)
	require.Contains(t, checkStdout, `"ok": true`)
}

func TestHarnessInstallScopedGuidanceWarnsOnUnresolvedMarkedGuidanceDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{
		{
			OrbitID:        "cmd",
			HumansTemplate: "Run the $project_name cmd workflow.\n",
			Files: map[string]string{
				"cmd/README.md": "$project_name cmd guide\n",
			},
		},
		{
			OrbitID:        "docs",
			HumansTemplate: "Run the $project_name docs workflow.\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name docs guide\n",
			},
		},
	})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/cmd", "--bindings", bindingsPath)
	require.NoError(t, err)

	cmdBlock, err := orbittemplate.WrapRuntimeAgentsBlock("cmd", []byte("Run the Drifted cmd workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", "Workspace guide.\n"+string(cmdBlock))
	repo.AddAndCommit(t, "drift unrelated humans block")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], "Run View cleanup found unresolved drifted marked guidance; scoped guidance output was rolled back")
	require.NotContains(t, payload.Warnings[0], "scoped guidance compose was rolled back")
	require.Contains(t, payload.Warnings[0], "apply Run View presentation")
	require.Contains(t, payload.Warnings[0], "Run View cleanup blocked by Authored Truth Drift")
	require.Contains(t, payload.Warnings[0], "hyard guide sync --target all --output")

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Drifted cmd workflow.\n")
	require.NotContains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")
}

func TestHarnessInstallOverwriteExistingOverwritesTouchedGuidanceDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{{
		OrbitID:        "docs",
		HumansTemplate: "Run the $project_name docs workflow.\n",
		Files: map[string]string{
			"docs/guide.md": "$project_name docs guide\n",
		},
	}})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	driftedBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Run the Drifted docs workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(driftedBlock))
	repo.AddAndCommit(t, "drift touched humans block")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Warnings)

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.NotContains(t, string(humansData), "Run the Drifted docs workflow.\n")
	require.Contains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")
}

func TestHarnessInstallBatchAutoComposesScopedGuidanceArtifacts(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{
		{
			OrbitID:           "docs",
			AgentsTemplate:    "You are the $project_name docs orbit.\n",
			HumansTemplate:    "Run the $project_name docs workflow.\n",
			BootstrapTemplate: "Bootstrap the $project_name docs orbit.\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name docs guide\n",
			},
		},
		{
			OrbitID:        "cmd",
			AgentsTemplate: "Use $project_name cmd release flow.\n",
			HumansTemplate: "Run the $project_name cmd workflow.\n",
			Files: map[string]string{
				"cmd/README.md": "$project_name cmd guide\n",
			},
		},
	})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "batch", "orbit-template/docs", "orbit-template/cmd", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")
	require.Contains(t, payload.WrittenPaths, "HUMANS.md")
	require.Contains(t, payload.WrittenPaths, "BOOTSTRAP.md")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "You are the Installed Orbit docs orbit.\n")
	require.Contains(t, string(agentsData), "Use Installed Orbit cmd release flow.\n")

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")
	require.Contains(t, string(humansData), "Run the Installed Orbit cmd workflow.\n")

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.Contains(t, string(bootstrapData), "Bootstrap the Installed Orbit docs orbit.\n")
}

func TestHarnessInstallBatchScopedGuidanceWarnsOnUnresolvedMarkedGuidanceDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{
		{
			OrbitID:        "cmd",
			HumansTemplate: "Run the $project_name cmd workflow.\n",
			Files: map[string]string{
				"cmd/README.md": "$project_name cmd guide\n",
			},
		},
		{
			OrbitID:        "docs",
			HumansTemplate: "Run the $project_name docs workflow.\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name docs guide\n",
			},
		},
		{
			OrbitID:        "ops",
			HumansTemplate: "Run the $project_name ops workflow.\n",
			Files: map[string]string{
				"ops/runbook.md": "$project_name ops runbook\n",
			},
		},
	})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/cmd", "--bindings", bindingsPath)
	require.NoError(t, err)

	cmdBlock, err := orbittemplate.WrapRuntimeAgentsBlock("cmd", []byte("Run the Drifted cmd workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", "Workspace guide.\n"+string(cmdBlock))
	repo.AddAndCommit(t, "drift unrelated humans block for batch")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "batch", "orbit-template/docs", "orbit-template/ops", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], "Run View cleanup found unresolved drifted marked guidance; scoped guidance output was rolled back")
	require.NotContains(t, payload.Warnings[0], "scoped guidance compose was rolled back")
	require.Contains(t, payload.Warnings[0], "apply Run View presentation")
	require.Contains(t, payload.Warnings[0], "Run View cleanup blocked by Authored Truth Drift")
	require.Contains(t, payload.Warnings[0], "hyard guide sync --target all --output")

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansData), "Run the Drifted cmd workflow.\n")
	require.NotContains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")
	require.NotContains(t, string(humansData), "Run the Installed Orbit ops workflow.\n")
}

func TestHarnessInstallBatchOverwriteExistingOverwritesTouchedGuidanceDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallGuidanceRepo(t, []installGuidanceTemplateSpec{
		{
			OrbitID:        "docs",
			HumansTemplate: "Run the $project_name docs workflow.\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name docs guide\n",
			},
		},
		{
			OrbitID:        "ops",
			HumansTemplate: "Run the $project_name ops workflow.\n",
			Files: map[string]string{
				"ops/runbook.md": "$project_name ops runbook\n",
			},
		},
	})
	bindingsPath := writeHarnessInstallGuidanceBindings(t, repo.Root)

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "batch", "orbit-template/docs", "orbit-template/ops", "--bindings", bindingsPath)
	require.NoError(t, err)

	driftedBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Run the Drifted docs workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(driftedBlock))
	repo.AddAndCommit(t, "drift touched humans block for batch")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "batch", "orbit-template/docs", "orbit-template/ops", "--bindings", bindingsPath, "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Warnings)

	humansData, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.NotContains(t, string(humansData), "Run the Drifted docs workflow.\n")
	require.Contains(t, string(humansData), "Run the Installed Orbit docs workflow.\n")
	require.Contains(t, string(humansData), "Run the Installed Orbit ops workflow.\n")
}

type installGuidanceTemplateSpec struct {
	OrbitID           string
	AgentsTemplate    string
	HumansTemplate    string
	BootstrapTemplate string
	Files             map[string]string
}

func seedHarnessInstallGuidanceRepo(t *testing.T, templates []installGuidanceTemplateSpec) *testutil.Repo {
	t.Helper()

	repo := seedEmptyHarnessRuntimeRepo(t)

	for _, template := range templates {
		repo.WriteFile(t, ".harness/vars.yaml", ""+
			"schema_version: 1\n"+
			"variables:\n"+
			"  project_name:\n"+
			"    value: Orbit\n"+
			"    description: Product title\n")
		spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(template.OrbitID)
		require.NoError(t, err)
		require.NotNil(t, spec.Meta)
		spec.Meta.AgentsTemplate = template.AgentsTemplate
		spec.Meta.HumansTemplate = template.HumansTemplate
		spec.Meta.BootstrapTemplate = template.BootstrapTemplate
		spec.Members = []orbitpkg.OrbitMember{{
			Key:  template.OrbitID + "-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{template.OrbitID + "/**"},
			},
		}}
		require.NotNil(t, spec.Behavior)
		spec.Behavior.Scope.WriteRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberSubject}
		spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberSubject}
		spec.Behavior.Scope.OrchestrationRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberProcess, orbitpkg.OrbitMemberSubject}
		_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
		require.NoError(t, err)
		for path, content := range template.Files {
			repo.WriteFile(t, path, content)
		}
		repo.AddAndCommit(t, "seed "+template.OrbitID+" template source")

		_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
			Preview: orbittemplate.TemplateSavePreviewInput{
				RepoRoot:     repo.Root,
				OrbitID:      template.OrbitID,
				TargetBranch: "orbit-template/" + template.OrbitID,
				Now:          time.Date(2026, time.April, 21, 15, 0, 0, 0, time.UTC),
			},
		})
		require.NoError(t, err)

		rmArgs := []string{"rm", "-f", filepath.Join(".harness", "orbits", template.OrbitID+".yaml"), ".harness/vars.yaml"}
		for path := range template.Files {
			rmArgs = append(rmArgs, path)
		}
		repo.Run(t, rmArgs...)
		repo.AddAndCommit(t, "clear "+template.OrbitID+" runtime content")
	}

	return repo
}

func writeHarnessInstallGuidanceBindings(t *testing.T, repoRoot string) string {
	t.Helper()

	bindingsPath := filepath.Join(repoRoot, "install-guidance-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))
	return bindingsPath
}
