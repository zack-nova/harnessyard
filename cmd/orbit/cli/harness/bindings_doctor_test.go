package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestDoctorRuntimeBindingsReportsErrorsAndWarnings(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	now := time.Date(2026, time.May, 12, 11, 0, 0, 0, time.UTC)
	defaultRegion := "us-east-1"
	_, err := WriteRuntimeFile(repoRoot, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: now},
		},
	})
	require.NoError(t, err)
	_, err = WriteVarsFile(repoRoot, bindings.VarsFile{
		SchemaVersion: bindings.VarsSchemaVersion,
		Variables: map[string]bindings.VariableBinding{
			"github_token": {
				ValueFrom: &bindings.ValueSource{Env: "GITHUB_TOKEN"},
			},
			"issue_payload": {
				ValueFrom: &bindings.ValueSource{File: ".harness/context/issue.json"},
			},
			"orphan_binding": {
				Value: "unused",
			},
		},
	})
	require.NoError(t, err)
	_, err = WriteInstallRecord(repoRoot, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: now,
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"github_token": {
					Description: "GitHub token",
					Required:    true,
					Sensitive:   true,
				},
				"issue_payload": {
					Description: "Issue payload",
					Required:    false,
				},
				"project_name": {
					Description: "Project name",
					Required:    true,
				},
				"region": {
					Description: "Region",
					Required:    true,
					Default:     &defaultRegion,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{},
		},
	})
	require.NoError(t, err)

	result, err := DoctorRuntimeBindings(context.Background(), RuntimeBindingsDoctorInput{
		RepoRoot: repoRoot,
		LookupEnv: func(name string) (string, bool) {
			require.Equal(t, "GITHUB_TOKEN", name)
			return "", false
		},
	})
	require.NoError(t, err)

	require.Equal(t, RuntimeBindingsDoctorStatusError, result.Status)
	requireRuntimeBindingDiagnostic(t, result.Errors, "missing_required", "project_name", "docs")
	requireRuntimeBindingDiagnostic(t, result.Errors, "unset_env", "github_token", "global")
	require.NotContains(t, runtimeBindingDiagnosticKeys(result.Errors), "missing_required:region:docs")
	requireRuntimeBindingDiagnostic(t, result.Warnings, "missing_file", "issue_payload", "global")
	requireRuntimeBindingDiagnostic(t, result.Warnings, "undeclared_binding", "orphan_binding", "global")
}

func requireRuntimeBindingDiagnostic(t *testing.T, diagnostics []RuntimeBindingDiagnostic, code string, variable string, scope string) {
	t.Helper()

	require.Contains(t, runtimeBindingDiagnosticKeys(diagnostics), code+":"+variable+":"+scope)
}

func runtimeBindingDiagnosticKeys(diagnostics []RuntimeBindingDiagnostic) []string {
	keys := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		keys = append(keys, diagnostic.Code+":"+diagnostic.Variable+":"+diagnostic.Scope)
	}
	return keys
}
