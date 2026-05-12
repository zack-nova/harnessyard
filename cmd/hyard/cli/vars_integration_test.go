package cli_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	hyardcli "github.com/zack-nova/harnessyard/cmd/hyard/cli"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardVarsPathReportsCanonicalRuntimeBindingsPath(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "path")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ".harness/vars.yaml\n", stdout)
}

func TestHyardVarsValidateAcceptsSchema2RuntimeBindings(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Harness Yard\n"+
		"  github_token:\n"+
		"    value_from:\n"+
		"      env: GITHUB_TOKEN\n"+
		"scoped_variables:\n"+
		"  docs:\n"+
		"    variables:\n"+
		"      issue_payload:\n"+
		"        value_from:\n"+
		"          file: .harness/context/issue.json\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "validate")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "validated Runtime Bindings file: .harness/vars.yaml\n", stdout)
}

func TestHyardVarsValidateReportsActionableRuntimeBindingsErrors(t *testing.T) {
	t.Parallel()

	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Harness Yard\n"+
		"    value_from:\n"+
		"      env: PROJECT_NAME\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "validate")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, ".harness/vars.yaml")
	require.ErrorContains(t, err, "variables.project_name must set exactly one of value or value_from")
}

func TestHyardVarsDoctorReportsRuntimeBindingDiagnostics(t *testing.T) {
	lockHyardProcessEnv(t)

	repo := seedHyardVarsInstallRuntime(t, map[string]bindings.VariableDeclaration{
		"github_token": {Description: "GitHub token", Required: true, Sensitive: true},
		"project_name": {Description: "Project name", Required: true},
	})
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  github_token:\n"+
		"    value_from:\n"+
		"      env: HYARD_TEST_GITHUB_TOKEN_UNSET\n"+
		"  orphan_binding:\n"+
		"    value: unused\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "vars", "doctor", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)

	var payload struct {
		Status   string                  `json:"status"`
		Errors   []varsDiagnosticPayload `json:"errors"`
		Warnings []varsDiagnosticPayload `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "error", payload.Status)
	require.Contains(t, varsDiagnosticKeys(payload.Errors), "missing_required:project_name")
	require.Contains(t, varsDiagnosticKeys(payload.Errors), "unset_env:github_token")
	require.Contains(t, varsDiagnosticKeys(payload.Warnings), "undeclared_binding:orphan_binding")
}

func TestHyardVarsExplainReportsTextAndJSON(t *testing.T) {
	lockHyardProcessEnv(t)
	t.Setenv("GITHUB_TOKEN", "ghp_secret")

	repo := seedHyardVarsInstallRuntime(t, map[string]bindings.VariableDeclaration{
		"github_token": {Description: "GitHub token", Required: true, Sensitive: true},
	})
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  github_token:\n"+
		"    value_from:\n"+
		"      env: GITHUB_TOKEN\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "vars", "explain", "github_token")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "name: github_token\n")
	require.Contains(t, stdout, "status: resolved\n")
	require.Contains(t, stdout, "value_source: env:GITHUB_TOKEN\n")
	require.Contains(t, stdout, "value: <redacted>\n")
	require.Contains(t, stdout, "required: true\n")
	require.Contains(t, stdout, "sensitive: true\n")
	require.Contains(t, stdout, "declaring_orbits: docs\n")

	stdout, stderr, err = executeHyardCLIUnlocked(t, repo.Root, "vars", "explain", "github_token", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Name            string   `json:"name"`
		Status          string   `json:"status"`
		ValueSource     string   `json:"value_source"`
		Value           string   `json:"value"`
		Required        bool     `json:"required"`
		Sensitive       bool     `json:"sensitive"`
		SelectedScope   string   `json:"selected_scope"`
		DeclaringOrbits []string `json:"declaring_orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "github_token", payload.Name)
	require.Equal(t, "resolved", payload.Status)
	require.Equal(t, "env:GITHUB_TOKEN", payload.ValueSource)
	require.Equal(t, "<redacted>", payload.Value)
	require.True(t, payload.Required)
	require.True(t, payload.Sensitive)
	require.Equal(t, "global", payload.SelectedScope)
	require.Equal(t, []string{"docs"}, payload.DeclaringOrbits)
}

func seedHyardVarsInstallRuntime(t *testing.T, declarations map[string]bindings.VariableDeclaration) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	now := time.Date(2026, time.May, 12, 12, 0, 0, 0, time.UTC)
	_, err := harnesspkg.WriteRuntimeFile(repo.Root, harnesspkg.RuntimeFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.RuntimeKind,
		Harness: harnesspkg.RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.RuntimeMember{
			{OrbitID: "docs", Source: harnesspkg.MemberSourceInstallOrbit, AddedAt: now},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations:    declarations,
			ResolvedAtApply: map[string]bindings.VariableBinding{},
		},
	})
	require.NoError(t, err)

	return repo
}

type varsDiagnosticPayload struct {
	Code     string `json:"code"`
	Variable string `json:"variable"`
}

func varsDiagnosticKeys(diagnostics []varsDiagnosticPayload) []string {
	keys := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		keys = append(keys, diagnostic.Code+":"+diagnostic.Variable)
	}
	return keys
}
