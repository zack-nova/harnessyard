package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAgentUnifiedConfigFileDataValidatesTargetsAndConfig(t *testing.T) {
	t.Parallel()

	file, err := ParseAgentUnifiedConfigFileData([]byte("" +
		"version: 1\n" +
		"targets:\n" +
		"  codex:\n" +
		"    enabled: true\n" +
		"    scope: project\n" +
		"  claudeCode:\n" +
		"    enabled: true\n" +
		"config:\n" +
		"  model: gpt-5.4\n" +
		"  features:\n" +
		"    web_search: true\n"))
	require.NoError(t, err)
	require.Equal(t, 1, file.Version)
	require.Equal(t, AgentUnifiedConfigTarget{Enabled: true, Scope: "project"}, file.Targets["codex"])
	require.Equal(t, AgentUnifiedConfigTarget{Enabled: true}, file.Targets["claude"])
	require.Equal(t, "gpt-5.4", file.Config["model"])
	require.Equal(t, map[string]any{"web_search": true}, file.Config["features"])
}

func TestParseAgentUnifiedConfigFileDataRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := ParseAgentUnifiedConfigFileData([]byte("" +
		"version: 1\n" +
		"targets:\n" +
		"  codex:\n" +
		"    enabled: true\n" +
		"unexpected: true\n"))
	require.ErrorContains(t, err, `unknown top-level field "unexpected"`)
}

func TestParseAgentUnifiedConfigFileDataParsesHooksTruth(t *testing.T) {
	t.Parallel()

	file, err := ParseAgentUnifiedConfigFileData([]byte("" +
		"version: 1\n" +
		"targets:\n" +
		"  codex:\n" +
		"    enabled: true\n" +
		"hooks:\n" +
		"  enabled: true\n" +
		"  unsupported_behavior: skip\n" +
		"  defaults:\n" +
		"    timeout_seconds: 45\n" +
		"    runner: hyard\n" +
		"  entries:\n" +
		"    - id: block-dangerous-shell\n" +
		"      enabled: true\n" +
		"      description: Block dangerous shell commands.\n" +
		"      event:\n" +
		"        kind: tool.before\n" +
		"      match:\n" +
		"        tools: [shell]\n" +
		"        command_patterns:\n" +
		"          - \"rm -rf *\"\n" +
		"      handler:\n" +
		"        type: command\n" +
		"        path: hooks/block-dangerous-shell/run.sh\n" +
		"        timeout_seconds: 10\n" +
		"        status_message: Checking shell command\n" +
		"      targets:\n" +
		"        codex: true\n"))
	require.NoError(t, err)
	require.True(t, file.Hooks.Enabled)
	require.Equal(t, "skip", file.Hooks.UnsupportedBehavior)
	require.Equal(t, 45, file.Hooks.Defaults.TimeoutSeconds)
	require.Equal(t, "hyard", file.Hooks.Defaults.Runner)
	require.Len(t, file.Hooks.Entries, 1)
	entry := file.Hooks.Entries[0]
	require.Equal(t, "block-dangerous-shell", entry.ID)
	require.Equal(t, "tool.before", entry.Event.Kind)
	require.Equal(t, []string{"shell"}, entry.Match.Tools)
	require.Equal(t, []string{"rm -rf *"}, entry.Match.CommandPatterns)
	require.Equal(t, "command", entry.Handler.Type)
	require.Equal(t, "hooks/block-dangerous-shell/run.sh", entry.Handler.Path)
	require.Equal(t, 10, entry.Handler.TimeoutSeconds)
	require.Equal(t, map[string]bool{"codex": true}, entry.Targets)
}

func TestRunAgentHookTranslatesNativeInputToUnifiedProtocol(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	hookDir := filepath.Join(repoRoot, "hooks", "block-dangerous-shell")
	require.NoError(t, os.MkdirAll(hookDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, ".harness", "agents"), 0o755))
	handlerPath := filepath.Join(hookDir, "run.sh")
	require.NoError(t, os.WriteFile(handlerPath, []byte("#!/bin/sh\nset -eu\ncat > hooks/block-dangerous-shell/captured.json\nprintf '{\"decision\":\"block\",\"message\":\"blocked dangerous command\"}\\n'\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".harness", "agents", "config.yaml"), []byte(""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  entries:\n"+
		"    - id: block-dangerous-shell\n"+
		"      event:\n"+
		"        kind: tool.before\n"+
		"      match:\n"+
		"        tools: [shell]\n"+
		"        command_patterns: [\"rm -rf *\"]\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/block-dangerous-shell/run.sh\n"), 0o600))

	result, err := RunAgentHook(context.Background(), AgentHookRunInput{
		RepoRoot:    repoRoot,
		Target:      "codex",
		HookID:      "block-dangerous-shell",
		NativeStdin: []byte(`{"tool_name":"shell","command":"rm -rf build"}`),
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.ExitCode)
	require.Contains(t, string(result.Stderr), "blocked dangerous command")

	captured, err := os.ReadFile(filepath.Join(hookDir, "captured.json"))
	require.NoError(t, err)
	require.Contains(t, string(captured), `"target": "codex"`)
	require.Contains(t, string(captured), `"hook": "block-dangerous-shell"`)
	require.Contains(t, string(captured), `"kind": "tool.before"`)
	require.Contains(t, string(captured), `"native_input"`)
}

func TestMergeAgentNativeConfigMapsRejectsSidecarOverride(t *testing.T) {
	t.Parallel()

	_, err := mergeAgentNativeConfigMaps(
		map[string]any{"model": "gpt-4.1"},
		map[string]any{"model": "gpt-5.4"},
		".harness/agents/codex.config.toml",
	)
	require.ErrorContains(t, err, `sidecar ".harness/agents/codex.config.toml" cannot override unified config key "model"`)
}

func TestMergeAgentNativeConfigMapsPreservesSidecarSupplements(t *testing.T) {
	t.Parallel()

	merged, err := mergeAgentNativeConfigMaps(
		map[string]any{"approval_policy": "on-request", "features": map[string]any{"web_search": true}},
		map[string]any{"model": "gpt-5.4"},
		".harness/agents/codex.config.toml",
	)
	require.NoError(t, err)
	require.Equal(t, []string{"approval_policy", "features.web_search", "model"}, nativeConfigKeyPaths(merged))
	require.Equal(t, "on-request", merged["approval_policy"])
	require.Equal(t, "gpt-5.4", merged["model"])
}

func TestParseNativeConfigMapAcceptsJSON5SidecarSubset(t *testing.T) {
	t.Parallel()

	parsed, err := parseNativeConfigMap([]byte("{\n  // openclaw sidecar\n  \"workspaceMode\": \"trusted\",\n}\n"), nativeConfigFormatJSON)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"workspaceMode": "trusted"}, parsed)
}
