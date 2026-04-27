package orbittemplate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveLocalTemplateSourceRejectsPureAgentAddonHandlerOutsideExportSurface(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", sourceRef)
	writeTemplateBranchAgentAddonSpec(t, repo.Root, false)
	repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	repo.AddAndCommit(t, "add agent addon outside export surface")
	repo.Run(t, "checkout", runtimeBranch)

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, sourceRef)
	require.ErrorContains(t, err, `handler.path "hooks/docs/block-dangerous-shell/run.sh" must resolve inside the export surface`)
}

func TestApplyLocalTemplateSnapshotsAgentAddonsWithoutNativeActivation(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", sourceRef)
	writeTemplateBranchAgentAddonSpec(t, repo.Root, true)
	repo.WriteFile(t, "hooks/docs/block-dangerous-shell/run.sh", "#!/usr/bin/env bash\nexit 0\n")
	repo.AddAndCommit(t, "add agent addon")
	repo.Run(t, "checkout", runtimeBranch)

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	result, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.April, 26, 10, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Preview.Conflicts)

	record, err := loadRuntimeInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.AgentAddons)
	require.Len(t, record.AgentAddons.Hooks, 1)
	hook := record.AgentAddons.Hooks[0]
	require.Equal(t, "docs", hook.Package)
	require.Equal(t, "block-dangerous-shell", hook.ID)
	require.Equal(t, "docs:block-dangerous-shell", hook.DisplayID)
	require.True(t, hook.Required)
	require.Equal(t, "tool.before", hook.EventKind)
	require.Equal(t, []string{"shell"}, hook.Tools)
	require.Equal(t, "command", hook.HandlerType)
	require.Equal(t, "hooks/docs/block-dangerous-shell/run.sh", hook.HandlerPath)
	require.NotEmpty(t, hook.HandlerDigest)
	require.Equal(t, map[string]bool{"codex": true}, hook.Targets)

	_, err = os.Stat(filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func writeTemplateBranchAgentAddonSpec(t *testing.T, repoRoot string, includeHookExport bool) {
	t.Helper()

	content := "" +
		"  - key: docs-content\n" +
		"    role: rule\n" +
		"    paths:\n" +
		"      include:\n" +
		"        - docs/**\n"
	if includeHookExport {
		content += "" +
			"  - key: hook-assets\n" +
			"    role: rule\n" +
			"    paths:\n" +
			"      include:\n" +
			"        - hooks/**\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".harness", "orbits", "docs.yaml"), []byte(""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"content:\n"+
		content+
		"behavior:\n"+
		"  scope:\n"+
		"    projection_roles:\n"+
		"      - meta\n"+
		"      - subject\n"+
		"      - rule\n"+
		"      - process\n"+
		"    write_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    export_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    orchestration_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"      - process\n"+
		"  orchestration:\n"+
		"    include_orbit_description: true\n"+
		"    materialize_agents_from_meta: true\n"+
		"agent_addons:\n"+
		"  hooks:\n"+
		"    entries:\n"+
		"      - id: block-dangerous-shell\n"+
		"        required: true\n"+
		"        event:\n"+
		"          kind: tool.before\n"+
		"        match:\n"+
		"          tools:\n"+
		"            - shell\n"+
		"        handler:\n"+
		"          type: command\n"+
		"          path: hooks/docs/block-dangerous-shell/run.sh\n"+
		"        targets:\n"+
		"          codex: true\n"), 0o600))
}
