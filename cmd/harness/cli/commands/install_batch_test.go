package commands

import (
	"testing"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestBatchVariableNamespacesConflictingCandidateDeclarations(t *testing.T) {
	t.Parallel()

	namespaces := batchVariableNamespaces([]orbitInstallBatchCandidate{
		{
			Preview: orbittemplate.TemplateApplyPreview{
				Source: orbittemplate.LocalTemplateSource{
					Manifest: orbittemplate.Manifest{
						Template: orbittemplate.Metadata{OrbitID: "docs"},
						Variables: map[string]orbittemplate.VariableSpec{
							"project_name": {Description: "Docs title", Required: true},
						},
					},
				},
			},
		},
		{
			Preview: orbittemplate.TemplateApplyPreview{
				Source: orbittemplate.LocalTemplateSource{
					Manifest: orbittemplate.Manifest{
						Template: orbittemplate.Metadata{OrbitID: "cmd"},
						Variables: map[string]orbittemplate.VariableSpec{
							"project_name": {Description: "CLI title", Required: true},
						},
					},
				},
			},
		},
	})

	require.Equal(t, map[string]map[string]string{
		"cmd":  {"project_name": "cmd"},
		"docs": {"project_name": "docs"},
	}, namespaces)
}
