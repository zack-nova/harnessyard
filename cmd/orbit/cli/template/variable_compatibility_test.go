package orbittemplate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

func TestResolveRuntimeInstallVariableNamespacesUsesActiveInstallIDs(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	staleRecord := InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "stale",
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/stale",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 11, 9, 0, 0, 0, time.UTC),
		Variables: &InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {Description: "Legacy title", Required: true},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{},
		},
	}
	_, err := writeRuntimeInstallRecord(repoRoot, staleRecord)
	require.NoError(t, err)

	declared := map[string]bindings.VariableDeclaration{
		"project_name": {Description: "Product title", Required: true},
	}
	namespaces, err := resolveRuntimeInstallVariableNamespaces(repoRoot, "docs", declared, []string{})
	require.NoError(t, err)
	require.Empty(t, namespaces)

	namespaces, err = resolveRuntimeInstallVariableNamespaces(repoRoot, "docs", declared, []string{"stale"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"project_name": "docs"}, namespaces)
}
