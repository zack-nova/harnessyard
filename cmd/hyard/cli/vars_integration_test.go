package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestHyardVarsHelpTeachesPublicRuntimeBindingsSurface(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHyardCLI(t, t.TempDir(), "vars", "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "Runtime Bindings")
	require.Contains(t, stdout, ".harness/vars.yaml")
	require.Contains(t, stdout, "schema_version: 2")
	require.Contains(t, stdout, "hyard vars init <package-source>")
	require.Contains(t, stdout, "{{ vars.<name> }}")
	require.NotContains(t, stdout, ".orbit/vars.yaml")
	require.NotContains(t, stdout, "$name")
	require.NotContains(t, stdout, "--strict-bindings")
	require.NotContains(t, stdout, "--allow-unresolved-bindings")
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

func TestHyardVarsInitWritesSchema2RuntimeBindingsSkeleton(t *testing.T) {
	t.Parallel()

	repo := seedHyardVarsInitTemplateRepo(t, true)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "init", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "wrote Runtime Bindings skeleton to .harness/vars.yaml\n", stdout)

	file, err := harnesspkg.LoadVarsFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, bindings.VarsSchemaVersion, file.SchemaVersion)
	require.Empty(t, file.Variables["project_name"].Value)
	require.Equal(t, "Product title", file.Variables["project_name"].Description)
	require.Empty(t, file.Variables["docs_url"].Value)
	require.Equal(t, "Documentation URL", file.Variables["docs_url"].Description)
	require.NotNil(t, file.Variables["github_token"].ValueFrom)
	require.Equal(t, "GITHUB_TOKEN", file.Variables["github_token"].ValueFrom.Env)
	require.Empty(t, file.Variables["github_token"].Value)
}

func TestHyardVarsInitDefaultsMaterializesDeclarationDefaults(t *testing.T) {
	t.Parallel()

	repo := seedHyardVarsInitTemplateRepo(t, true)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "init", "orbit-template/docs", "--defaults")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "wrote Runtime Bindings skeleton to .harness/vars.yaml\n", stdout)

	file, err := harnesspkg.LoadVarsFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, "https://docs.example.test", file.Variables["docs_url"].Value)
}

func TestHyardInstallInteractivePersistsMissingBindingsAndSkipsDefaults(t *testing.T) {
	t.Parallel()

	repo := seedHyardVarsInitTemplateRepo(t, false)

	stdout, stderr, err := executeHyardCLIWithInput(
		t,
		repo.Root,
		"Acme Docs\n",
		"install",
		"orbit-template/docs",
		"--interactive",
		"--json",
	)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
	require.Contains(t, stderr, "project_name (Product title): ")
	require.NotContains(t, stderr, "docs_url")

	var payload struct {
		DryRun  bool   `json:"dry_run"`
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, "docs", payload.OrbitID)

	rendered, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Acme Docs guide at https://docs.example.test\n", string(rendered))

	file, err := harnesspkg.LoadVarsFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, "Acme Docs", file.Variables["project_name"].Value)
	_, hasDefaultBinding := file.Variables["docs_url"]
	require.False(t, hasDefaultBinding)
	require.NoError(t, harnesspkg.ValidateVarsFile(repo.Root))
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

func TestHyardVarsDoctorRejectsSensitiveInlineBindingWithoutLeakingValue(t *testing.T) {
	t.Parallel()

	repo := seedHyardVarsInstallRuntime(t, map[string]bindings.VariableDeclaration{
		"github_token": {Description: "GitHub token", Required: true, Sensitive: true},
	})
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 2\n"+
		"variables:\n"+
		"  github_token:\n"+
		"    value: ghp_secret\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "vars", "doctor")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "status: error\n")
	require.Contains(t, stdout, "sensitive_value_source github_token")
	require.NotContains(t, stdout, "ghp_secret")
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

func seedHyardVarsInitTemplateRepo(t *testing.T, includeSensitive bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Date(2026, time.May, 12, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed empty runtime")

	repo.Run(t, "checkout", "-b", "orbit-template/docs")
	repo.Run(t, "rm", "--ignore-unmatch", ".harness/runtime.yaml")

	variables := "" +
		"variables:\n" +
		"  docs_url:\n" +
		"    description: Documentation URL\n" +
		"    required: true\n" +
		"    default: https://docs.example.test\n" +
		"  project_name:\n" +
		"    description: Product title\n" +
		"    required: true\n"
	if includeSensitive {
		variables += "" +
			"  github_token:\n" +
			"    description: GitHub token\n" +
			"    required: true\n" +
			"    sensitive: true\n"
	}

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-05-12T12:00:00Z\n"+
		variables)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "{{ vars.project_name }} guide at {{ vars.docs_url }}\n")
	repo.AddAndCommit(t, "seed vars init template")
	repo.Run(t, "checkout", "main")

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
