package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestAgentDetectionSupportedIDsAndAliases(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"claudecode", "codex", "openclaw"}, SupportedAgentIDs())

	for input, expected := range map[string]string{
		"claude":      "claudecode",
		"claude_code": "claudecode",
		"claude-code": "claudecode",
		"claudecode":  "claudecode",
		"codex":       "codex",
		"openclaw":    "openclaw",
	} {
		actual, ok := NormalizeAgentID(input)
		require.True(t, ok, input)
		require.Equal(t, expected, actual, input)
	}

	_, ok := NormalizeAgentID("gitagent")
	require.False(t, ok)
}

func TestDetectAgentsTreatsClaudeProjectFootprintAsFootprintOnly(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	repo.WriteFile(t, "CLAUDE.md", "# Claude project guidance\n")
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)

	claude := requireDetectedAgent(t, report, "claudecode")
	require.Equal(t, AgentDetectionStatusFootprintOnly, claude.Summary.Status)
	require.False(t, claude.Summary.Ready)
	require.Empty(t, report.SuggestedActions)

	footprint := requireDetectedComponent(t, claude, "project_footprint")
	require.Equal(t, AgentDetectionStatusFootprintOnly, footprint.Status)
	require.Len(t, footprint.Evidence, 1)
	require.Equal(t, "CLAUDE.md", footprint.Evidence[0].Path)
	require.False(t, footprint.Evidence[0].ContentRead)
}

func TestDetectAgentsDetectsFakeCodexCLIAndSuggestsUse(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "codex"), "#!/bin/sh\necho 'codex 0.125.0'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)

	codex := requireDetectedAgent(t, report, "codex")
	require.Equal(t, AgentDetectionStatusInstalledCLI, codex.Summary.Status)
	require.True(t, codex.Summary.Ready)

	cli := requireDetectedComponent(t, codex, "cli")
	require.Equal(t, AgentDetectionStatusInstalledCLI, cli.Status)
	require.Equal(t, "codex 0.125.0", cli.Version)
	require.NotEmpty(t, cli.Evidence)
	for _, evidence := range cli.Evidence {
		require.NotContains(t, evidence.Path, os.Getenv("HOME"))
	}

	require.Equal(t, []AgentSuggestedAction{
		{Command: "hyard agent use codex", Reason: "codex is the only ready detected agent"},
	}, report.SuggestedActions)
}

func TestDetectAgentsDeepDetectsNPMGlobalPackageWithoutCLI(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "npm"), "#!/bin/sh\nprintf '%s\\n' '{\"dependencies\":{\"@openai/codex\":{\"version\":\"0.125.0\"}}}'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	codex := requireDetectedAgent(t, report, "codex")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, codex.Summary.Status)
	require.False(t, codex.Summary.Ready)
	require.Empty(t, report.SuggestedActions)

	pkg := requireDetectedComponent(t, codex, "package")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, pkg.Status)
	require.Equal(t, "0.125.0", pkg.Version)
	require.Len(t, pkg.Evidence, 1)
	require.Equal(t, "package", pkg.Evidence[0].Kind)
	require.Equal(t, "npm_global", pkg.Evidence[0].Source)
	require.Equal(t, "@openai/codex", pkg.Evidence[0].Metadata["package"])
	require.False(t, pkg.Evidence[0].ContentRead)
}

func TestDetectAgentsDeepDetectsOpenClawPNPMGlobalPackageWithoutCLI(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "pnpm"), "#!/bin/sh\nprintf '%s\\n' '[{\"dependencies\":{\"openclaw\":{\"version\":\"0.9.1\"}}}]'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	openclaw := requireDetectedAgent(t, report, "openclaw")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, openclaw.Summary.Status)
	require.False(t, openclaw.Summary.Ready)

	pkg := requireDetectedComponent(t, openclaw, "package")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, pkg.Status)
	require.Equal(t, "0.9.1", pkg.Version)
	require.Len(t, pkg.Evidence, 1)
	require.Equal(t, "pnpm_global", pkg.Evidence[0].Source)
	require.Equal(t, "openclaw", pkg.Evidence[0].Metadata["package"])
}

func TestDetectAgentsDeepDetectsOpenClawBunGlobalPackageWithoutCLI(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "bun"), "#!/bin/sh\nprintf '%s\\n' 'openclaw@0.9.2'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	openclaw := requireDetectedAgent(t, report, "openclaw")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, openclaw.Summary.Status)

	pkg := requireDetectedComponent(t, openclaw, "package")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, pkg.Status)
	require.Equal(t, "0.9.2", pkg.Version)
	require.Equal(t, "bun_global", pkg.Evidence[0].Source)
	require.Equal(t, "openclaw", pkg.Evidence[0].Metadata["package"])
}

func TestDetectAgentsDeepDetectsHomebrewPackageWithoutCLI(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "brew"), "#!/bin/sh\nprintf '%s\\n' 'codex 0.125.1'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	codex := requireDetectedAgent(t, report, "codex")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, codex.Summary.Status)
	require.False(t, codex.Summary.Ready)

	pkg := requireDetectedComponent(t, codex, "package")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, pkg.Status)
	require.Equal(t, "0.125.1", pkg.Version)
	require.Equal(t, "homebrew", pkg.Evidence[0].Source)
	require.Equal(t, "codex", pkg.Evidence[0].Metadata["package"])
}

func TestDetectAgentsDeepDetectsDebianPackageWithoutCLI(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "dpkg-query"), "#!/bin/sh\nprintf '%s\\n' 'claude-code\t1.2.3'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	claude := requireDetectedAgent(t, report, "claudecode")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, claude.Summary.Status)
	require.False(t, claude.Summary.Ready)

	pkg := requireDetectedComponent(t, claude, "package")
	require.Equal(t, AgentDetectionStatusInstalledUnverified, pkg.Status)
	require.Equal(t, "1.2.3", pkg.Version)
	require.Equal(t, "dpkg", pkg.Evidence[0].Source)
	require.Equal(t, "claude-code", pkg.Evidence[0].Metadata["package"])
}

func TestDetectAgentsDeepDetectsUserDesktopBundle(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	homeDir := t.TempDir()
	appInfoPath := filepath.Join(homeDir, "Applications", "Codex.app", "Contents", "Info.plist")
	require.NoError(t, os.MkdirAll(filepath.Dir(appInfoPath), 0o750))
	require.NoError(t, os.WriteFile(appInfoPath, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key>
  <string>com.openai.codex</string>
  <key>CFBundleShortVersionString</key>
  <string>0.130.0</string>
</dict>
</plist>
`), 0o600))
	t.Setenv("PATH", "")
	t.Setenv("HOME", homeDir)

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	codex := requireDetectedAgent(t, report, "codex")
	require.Equal(t, AgentDetectionStatusInstalledDesktop, codex.Summary.Status)
	require.True(t, codex.Summary.Ready)

	desktop := requireDetectedComponent(t, codex, "desktop")
	require.Equal(t, AgentDetectionStatusInstalledDesktop, desktop.Status)
	require.Equal(t, "0.130.0", desktop.Version)
	require.Equal(t, "~"+filepath.ToSlash(string(os.PathSeparator))+"Applications/Codex.app", desktop.Evidence[0].Path)
	require.Equal(t, "com.openai.codex", desktop.Evidence[0].Metadata["bundle_id"])
}

func TestDetectAgentsDeepDetectsOpenClawGatewayService(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "systemctl"), "#!/bin/sh\nif [ \"$1\" = \"--user\" ] && [ \"$2\" = \"is-active\" ] && [ \"$3\" = \"openclaw-gateway.service\" ]; then\n  printf '%s\\n' active\n  exit 0\nfi\nexit 1\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Deep:     true,
	})
	require.NoError(t, err)

	openclaw := requireDetectedAgent(t, report, "openclaw")
	require.Equal(t, AgentDetectionStatusRunning, openclaw.Summary.Status)
	require.True(t, openclaw.Summary.Ready)

	gateway := requireDetectedComponent(t, openclaw, "gateway")
	require.Equal(t, AgentDetectionStatusRunning, gateway.Status)
	require.Equal(t, "systemd", gateway.Evidence[0].Source)
	require.Equal(t, "openclaw-gateway.service", gateway.Evidence[0].Metadata["unit"])
}

func TestDetectAgentsDetectsOpenClawLocalPrefixCLIWithoutPATH(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	homeDir := t.TempDir()
	openClawPath := filepath.Join(homeDir, ".openclaw", "bin", "openclaw")
	require.NoError(t, os.MkdirAll(filepath.Dir(openClawPath), 0o750))
	writeExecutable(t, openClawPath, "#!/bin/sh\necho 'openclaw 0.9.0'\n")
	t.Setenv("PATH", "")
	t.Setenv("HOME", homeDir)

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)

	openclaw := requireDetectedAgent(t, report, "openclaw")
	require.Equal(t, AgentDetectionStatusInstalledCLI, openclaw.Summary.Status)
	require.True(t, openclaw.Summary.Ready)

	cli := requireDetectedComponent(t, openclaw, "cli")
	require.Equal(t, AgentDetectionStatusInstalledCLI, cli.Status)
	require.Equal(t, "openclaw 0.9.0", cli.Version)
	require.Equal(t, "~/.openclaw/bin/openclaw", cli.Evidence[0].Path)
	require.Equal(t, []AgentSuggestedAction{
		{Command: "hyard agent use openclaw", Reason: "openclaw is the only ready detected agent"},
	}, report.SuggestedActions)
}

func TestDetectAgentsDoesNotSuggestWhenMultipleAgentsAreReady(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "codex"), "#!/bin/sh\necho 'codex 0.125.0'\n")
	writeExecutable(t, filepath.Join(binDir, "claude"), "#!/bin/sh\necho 'claude 1.2.3'\n")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)

	require.True(t, requireDetectedAgent(t, report, "codex").Summary.Ready)
	require.True(t, requireDetectedAgent(t, report, "claudecode").Summary.Ready)
	require.Empty(t, report.SuggestedActions)
	require.Contains(t, report.Warnings, "multiple ready agents detected: claudecode, codex")
}

func TestDetectAgentsReportsLegacyGitagentSelection(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	repo.WriteFile(t, ".git/orbit/state/agents/selection.json", ""+
		"{\n"+
		"  \"selected_framework\": \"gitagent\",\n"+
		"  \"selection_source\": \"explicit_local\",\n"+
		"  \"updated_at\": \"2026-04-25T12:00:00Z\"\n"+
		"}\n")
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())

	report, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)

	require.Equal(t, "gitagent", report.LocalSelection)
	require.Contains(t, report.Warnings, `legacy or unsupported selected agent "gitagent" is ignored by detection`)
	require.Empty(t, report.SuggestedActions)
}

func TestDetectAgentsUsesCachedReportUntilRefresh(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitDir := repo.GitDir(t)
	binDir := t.TempDir()
	codexPath := filepath.Join(binDir, "codex")
	writeExecutable(t, codexPath, "#!/bin/sh\necho 'codex 0.125.0'\n")
	homeDir := t.TempDir()
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("CODEX_HOME", filepath.Join(homeDir, "missing-codex-home"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(homeDir, "missing-claude-home"))
	t.Setenv("OPENCLAW_STATE_DIR", filepath.Join(homeDir, "missing-openclaw-home"))

	first, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)
	require.True(t, requireDetectedAgent(t, first, "codex").Summary.Ready)
	require.FileExists(t, filepath.Join(gitDir, "orbit", "state", "agents", "detection-cache.json"))

	require.NoError(t, os.Remove(codexPath))

	cached, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
	})
	require.NoError(t, err)
	require.True(t, requireDetectedAgent(t, cached, "codex").Summary.Ready)
	require.Equal(t, first.SuggestedActions, cached.SuggestedActions)

	refreshed, err := DetectAgents(context.Background(), AgentDetectionInput{
		RepoRoot: repo.Root,
		GitDir:   gitDir,
		Refresh:  true,
	})
	require.NoError(t, err)
	require.Equal(t, AgentDetectionStatusNotFound, requireDetectedAgent(t, refreshed, "codex").Summary.Status)
	require.Empty(t, refreshed.SuggestedActions)
}

func requireDetectedAgent(t *testing.T, report AgentDetectionReport, agentID string) AgentToolDetection {
	t.Helper()

	for _, tool := range report.Tools {
		if tool.Agent == agentID {
			return tool
		}
	}
	require.Failf(t, "missing detected agent", "agent %q not found in %#v", agentID, report.Tools)
	return AgentToolDetection{}
}

func requireDetectedComponent(t *testing.T, tool AgentToolDetection, component string) AgentComponentDetection {
	t.Helper()

	for _, candidate := range tool.Components {
		if candidate.Component == component {
			return candidate
		}
	}
	require.Failf(t, "missing detected component", "component %q not found in %#v", component, tool.Components)
	return AgentComponentDetection{}
}

func writeExecutable(t *testing.T, path string, contents string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(contents), 0o700))
	require.NoError(t, os.Chmod(path, 0o700))
}
