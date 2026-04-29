package commands

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestTemplateSaveDryRunConflictPayloadForPathConflict(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveDryRunConflictPayload(
		"/tmp/workspace",
		"harness-template/workspace",
		&harnesspkg.TemplatePathConflictError{
			Path:    "README.md",
			Members: []string{"cmd", "docs"},
		},
	)
	require.True(t, handled)
	require.True(t, payload.DryRun)
	require.Equal(t, "/tmp/workspace", payload.HarnessRoot)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.Empty(t, payload.Ambiguities)
	require.Len(t, payload.Conflicts, 1)
	require.Equal(t, templateSaveConflictJSON{
		Kind:         "path_conflict",
		Path:         "README.md",
		Contributors: []string{"cmd", "docs"},
		Message:      `path conflict for "README.md" (members: cmd, docs)`,
	}, payload.Conflicts[0])
}

func TestTemplateSaveDryRunConflictPayloadForVariableConflict(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveDryRunConflictPayload(
		"/tmp/workspace",
		"harness-template/workspace",
		&harnesspkg.TemplateVariableConflictError{
			Name:    "project_name",
			Members: []string{"cmd", "docs"},
		},
	)
	require.True(t, handled)
	require.Len(t, payload.Conflicts, 1)
	require.Equal(t, templateSaveConflictJSON{
		Kind:         "variable_conflict",
		Variable:     "project_name",
		Contributors: []string{"cmd", "docs"},
		Message:      `variable conflict for "project_name" (members: cmd, docs)`,
	}, payload.Conflicts[0])
}

func TestTemplateSaveDryRunConflictPayloadReturnsFalseForUnrelatedError(t *testing.T) {
	t.Parallel()

	_, handled := templateSaveDryRunConflictPayload("/tmp/workspace", "harness-template/workspace", &json.InvalidUnmarshalError{})
	require.False(t, handled)
}

func TestTemplateSaveFailurePayloadForPathConflict(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveFailurePayload(
		"/tmp/workspace",
		"harness-template/workspace",
		nil,
		&harnesspkg.TemplatePathConflictError{
			Path:    "README.md",
			Members: []string{"cmd", "docs"},
		},
	)
	require.True(t, handled)
	require.False(t, payload.DryRun)
	require.False(t, payload.Saved)
	require.Equal(t, "/tmp/workspace", payload.HarnessRoot)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.Len(t, payload.Conflicts, 1)
	require.Equal(t, templateSaveConflictJSON{
		Kind:         "path_conflict",
		Path:         "README.md",
		Contributors: []string{"cmd", "docs"},
		Message:      `path conflict for "README.md" (members: cmd, docs)`,
	}, payload.Conflicts[0])
}

func TestTemplateSaveFailurePayloadForVariableConflict(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveFailurePayload(
		"/tmp/workspace",
		"harness-template/workspace",
		nil,
		&harnesspkg.TemplateVariableConflictError{
			Name:    "project_name",
			Members: []string{"cmd", "docs"},
		},
	)
	require.True(t, handled)
	require.Len(t, payload.Conflicts, 1)
	require.Equal(t, templateSaveConflictJSON{
		Kind:         "variable_conflict",
		Variable:     "project_name",
		Contributors: []string{"cmd", "docs"},
		Message:      `variable conflict for "project_name" (members: cmd, docs)`,
	}, payload.Conflicts[0])
}

func TestTemplateSaveFailurePayloadForAmbiguityPreview(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveFailurePayload(
		"/tmp/workspace",
		"harness-template/workspace",
		&harnesspkg.TemplateSavePreview{
			HarnessID:    "workspace",
			TargetBranch: "harness-template/workspace",
			Ambiguities: []orbittemplate.FileReplacementAmbiguity{{
				Path: "docs/guide.md",
				Ambiguities: []orbittemplate.ReplacementAmbiguity{{
					Literal:   "Orbit",
					Variables: []string{"product_name", "project_name"},
				}},
			}},
			AmbiguitySources: map[string][]string{
				"AGENTS.md":     {"root_guidance"},
				"docs/guide.md": {"docs"},
			},
			Manifest: harnesspkg.TemplateManifest{
				Template: harnesspkg.TemplateMetadata{
					DefaultTemplate: true,
					RootGuidance: harnesspkg.RootGuidanceMetadata{
						Agents: true,
					},
				},
			},
		},
		nil,
	)
	require.True(t, handled)
	require.False(t, payload.DryRun)
	require.False(t, payload.Saved)
	require.Equal(t, "/tmp/workspace", payload.HarnessRoot)
	require.Equal(t, "workspace", payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.True(t, payload.DefaultTemplate)
	require.True(t, payload.RootGuidance.Agents)
	require.Contains(t, payload.Message, "replacement ambiguity detected")
	require.Contains(t, payload.Message, "AGENTS.md [root_guidance]")
	require.Contains(t, payload.Message, "docs/guide.md [docs]")
	require.Len(t, payload.Ambiguities, 1)
	require.Equal(t, "docs/guide.md", payload.Ambiguities[0].Path)
	require.Equal(t, []string{"docs"}, payload.Ambiguities[0].Contributors)
}

func TestTemplateSaveFailurePayloadForTargetBranchExists(t *testing.T) {
	t.Parallel()

	payload, handled := templateSaveFailurePayload(
		"/tmp/workspace",
		"harness-template/workspace",
		&harnesspkg.TemplateSavePreview{
			HarnessID:    "workspace",
			TargetBranch: "harness-template/workspace",
			Manifest: harnesspkg.TemplateManifest{
				Template: harnesspkg.TemplateMetadata{
					DefaultTemplate: false,
					RootGuidance: harnesspkg.RootGuidanceMetadata{
						Agents: true,
					},
				},
			},
		},
		&gitpkg.TemplateTargetBranchExistsError{Branch: "harness-template/workspace"},
	)
	require.True(t, handled)
	require.False(t, payload.DryRun)
	require.False(t, payload.Saved)
	require.Equal(t, "write", payload.Stage)
	require.Equal(t, "target_branch_exists", payload.Reason)
	require.Equal(t, "/tmp/workspace", payload.HarnessRoot)
	require.Equal(t, "workspace", payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.False(t, payload.DefaultTemplate)
	require.True(t, payload.RootGuidance.Agents)
	require.True(t, payload.OverwriteRequired)
	require.Equal(t, `target branch "harness-template/workspace" already exists; re-run with --overwrite to replace it`, payload.Message)
}
