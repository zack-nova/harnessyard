package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHyardAgentConfigImportCodexPreviewsProjectOverGlobalConfig(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".codex", "config.toml"), []byte(""+
		"model = \"gpt-5.3\"\n"+
		"approval_policy = \"on-request\"\n"+
		"[features]\n"+
		"web_search = true\n"), 0o600))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Framework string `json:"framework"`
		DryRun    bool   `json:"dry_run"`
		Imported  []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
		} `json:"imported"`
		WrittenPaths []string `json:"written_paths,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "codex", payload.Framework)
	require.True(t, payload.DryRun)
	require.Contains(t, payload.Imported, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}{Key: "model", Source: "project"})
	require.Contains(t, payload.Imported, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}{Key: "sandbox_mode", Source: "project"})
	require.Contains(t, payload.Imported, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}{Key: "approval_policy", Source: "global"})
	require.Contains(t, payload.Imported, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}{Key: "features.web_search", Source: "global"})
	require.Empty(t, payload.WrittenPaths)
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
}

func TestHyardAgentConfigImportCodexYesWritesHarnessAgentTruth(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".codex", "config.toml"), []byte(""+
		"approval_policy = \"on-request\"\n"+
		"[features]\n"+
		"web_search = true\n"), 0o600))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Framework    string   `json:"framework"`
		DryRun       bool     `json:"dry_run"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "codex", payload.Framework)
	require.False(t, payload.DryRun)
	require.Contains(t, payload.WrittenPaths, ".harness/agents/config.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/agents/manifest.yaml")

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "version: 1\n")
	require.Contains(t, string(configData), "targets:\n")
	require.Contains(t, string(configData), "  codex:\n")
	require.Contains(t, string(configData), "    enabled: true\n")
	require.Contains(t, string(configData), "    scope: project\n")
	require.Contains(t, string(configData), "config:\n")
	require.Contains(t, string(configData), "  approval_policy: on-request\n")
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
	require.Contains(t, string(configData), "  sandbox_mode: workspace-write\n")
	require.Contains(t, string(configData), "  features:\n")
	require.Contains(t, string(configData), "    web_search: true\n")

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "recommended_framework: codex\n")
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
}

func TestHyardAgentConfigImportCodexYesWritesRoundTripUnstableNativeConfigToSidecar(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	nativeConfig := "# keep exact codex native comments\nmodel = \"gpt-5.4\"\n"
	repo.WriteFile(t, ".codex/config.toml", nativeConfig)

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Sidecars []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"sidecars"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Sidecars, struct {
		Path   string `json:"path"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Path: ".harness/agents/codex.config.toml", Source: "project", Reason: "roundtrip_unstable"})
	require.Contains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")

	sidecarData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.NoError(t, err)
	require.Equal(t, nativeConfig, string(sidecarData))

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
}

func TestHyardAgentConfigImportCodexYesPreserveNativeWritesStableNativeConfigToSidecar(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	nativeConfig := "model = \"gpt-5.4\"\n"
	repo.WriteFile(t, ".codex/config.toml", nativeConfig)

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--preserve-native", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Sidecars []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"sidecars"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Sidecars, struct {
		Path   string `json:"path"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Path: ".harness/agents/codex.config.toml", Source: "project", Reason: "preserve_native"})
	require.Contains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")

	sidecarData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.NoError(t, err)
	require.Equal(t, nativeConfig, string(sidecarData))
}

func TestHyardAgentConfigImportCodexYesWritesUnparseableNativeConfigToSidecar(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	nativeConfig := "model = \"gpt-5.4\"\nexperimental = { enabled = true }\n"
	repo.WriteFile(t, ".codex/config.toml", nativeConfig)

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key string `json:"key"`
		} `json:"imported"`
		Sidecars []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"sidecars"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Imported)
	require.Contains(t, payload.Sidecars, struct {
		Path   string `json:"path"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Path: ".harness/agents/codex.config.toml", Source: "project", Reason: "parse_error"})
	require.Contains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")
	require.NotContains(t, payload.WrittenPaths, ".harness/agents/config.yaml")

	sidecarData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.NoError(t, err)
	require.Equal(t, nativeConfig, string(sidecarData))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
}

func TestHyardAgentConfigImportCodexSkipsSensitiveKeysByDefault(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".codex"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".codex", "config.toml"), []byte(""+
		"model = \"gpt-5.4\"\n"+
		"api_key = \"secret-value\"\n"), 0o600))

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key string `json:"key"`
		} `json:"imported"`
		Skipped []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "model"})
	require.NotContains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "api_key"})
	require.Contains(t, payload.Skipped, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Key: "api_key", Source: "global", Reason: "sensitive"})
}

func TestHyardAgentConfigImportCodexDoesNotWriteSidecarWithSensitiveNativeContent(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", ""+
		"# keep exact codex native comments\n"+
		"model = \"gpt-5.4\"\n"+
		"api_key = \"secret-value\"\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--preserve-native", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key string `json:"key"`
		} `json:"imported"`
		Skipped []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped"`
		Sidecars []struct {
			Path string `json:"path"`
		} `json:"sidecars"`
		SkippedSidecars []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped_sidecars"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "model"})
	require.Contains(t, payload.Skipped, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Key: "api_key", Source: "project", Reason: "sensitive"})
	require.Empty(t, payload.Sidecars)
	require.Contains(t, payload.SkippedSidecars, struct {
		Path   string `json:"path"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Path: ".harness/agents/codex.config.toml", Source: "project", Reason: "unsafe_native_content"})
	require.NotContains(t, payload.WrittenPaths, ".harness/agents/codex.config.toml")
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
}

func TestHyardAgentConfigImportCodexSkipsLocalPathValuesByDefault(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"helper_path = \"/Users/zack/bin/local-helper\"\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key string `json:"key"`
		} `json:"imported"`
		Skipped []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "model"})
	require.NotContains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "helper_path"})
	require.Contains(t, payload.Skipped, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Key: "helper_path", Source: "project", Reason: "local_path"})
}

func TestHyardAgentConfigImportCodexYesDoesNotOverwriteExistingHarnessConfigKeys(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: team-standard\n")

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", ""+
		"model = \"gpt-5.4\"\n"+
		"sandbox_mode = \"workspace-write\"\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key string `json:"key"`
		} `json:"imported"`
		Skipped []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "sandbox_mode"})
	require.NotContains(t, payload.Imported, struct {
		Key string `json:"key"`
	}{Key: "model"})
	require.Contains(t, payload.Skipped, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Key: "model", Source: "project", Reason: "already_configured"})

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: team-standard\n")
	require.Contains(t, string(configData), "  sandbox_mode: workspace-write\n")
	require.NotContains(t, string(configData), "gpt-5.4")
}

func TestHyardAgentConfigImportCodexYesReplaceOverwritesExistingHarnessConfigKeys(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: team-standard\n")

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", "model = \"gpt-5.4\"\n")

	stdout, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--replace", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Imported []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
		} `json:"imported"`
		Skipped []struct {
			Key    string `json:"key"`
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Imported, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
	}{Key: "model", Source: "project"})
	require.NotContains(t, payload.Skipped, struct {
		Key    string `json:"key"`
		Source string `json:"source"`
		Reason string `json:"reason"`
	}{Key: "model", Source: "project", Reason: "already_configured"})

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "  model: gpt-5.4\n")
	require.NotContains(t, string(configData), "team-standard")
}

func TestHyardAgentConfigImportCodexYesPreservesExistingHooks(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
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
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/block-dangerous-shell/run.sh\n")

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", "sandbox_mode = \"workspace-write\"\n")

	_, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	configData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(configData), "hooks:\n")
	require.Contains(t, string(configData), "    - id: block-dangerous-shell\n")
	require.Contains(t, string(configData), "        kind: tool.before\n")
	require.Contains(t, string(configData), "        path: hooks/block-dangerous-shell/run.sh\n")
	require.Contains(t, string(configData), "  sandbox_mode: workspace-write\n")
}

func TestHyardAgentConfigImportOnlySupportsCodexInitially(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)

	_, stderr, err := executeHyardCLI(t, repo.Root, "agent", "config", "import", "claude", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "agent config import currently supports codex only")
}

func TestHyardAgentConfigImportCodexYesFailsWhenRecommendationConflicts(t *testing.T) {
	repo := seedHyardRuntimeRepo(t)
	repo.WriteFile(t, ".harness/agents/manifest.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")

	lockHyardProcessEnv(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	repo.WriteFile(t, ".codex/config.toml", "model = \"gpt-5.4\"\n")

	_, stderr, err := executeHyardCLIUnlocked(t, repo.Root, "agent", "config", "import", "codex", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `recommended framework is "claude"`)
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "recommended_framework: claude\n")
}
