package bindings

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeVariableDeclarationUsesSharedCompatibilityPolicy(t *testing.T) {
	t.Parallel()

	merged, err := MergeVariableDeclaration("project_name", VariableDeclaration{
		Description: "",
		Required:    false,
	}, VariableDeclaration{
		Description: "Product title",
		Required:    true,
	})
	require.NoError(t, err)
	require.Equal(t, VariableDeclaration{
		Description: "Product title",
		Required:    true,
	}, merged)
}

func TestMergeDeclarationsFailsOnConflictingDescriptions(t *testing.T) {
	t.Parallel()

	merged := map[string]VariableDeclaration{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
	}
	contributors := map[string][]string{
		"project_name": {"orbit-template/docs"},
	}

	err := MergeDeclarations(merged, contributors, map[string]VariableDeclaration{
		"project_name": {
			Description: "CLI title",
			Required:    true,
		},
	}, "orbit-template/cmd")
	require.Error(t, err)

	var conflict *DeclarationConflictError
	require.ErrorAs(t, err, &conflict)
	require.Equal(t, "project_name", conflict.Name)
	require.Equal(t, []string{"orbit-template/cmd", "orbit-template/docs"}, conflict.Sources)
}
