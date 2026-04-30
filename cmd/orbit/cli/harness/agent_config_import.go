package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// AgentConfigImportInput captures one local native agent config import request.
type AgentConfigImportInput struct {
	RepoRoot       string
	HomeDir        string
	Framework      string
	Write          bool
	Replace        bool
	PreserveNative bool
}

// AgentConfigImportSource reports one native config source inspected by import.
type AgentConfigImportSource struct {
	Scope string `json:"scope"`
	Path  string `json:"path"`
	Found bool   `json:"found"`
}

// AgentConfigImportEntry reports one imported native config key.
type AgentConfigImportEntry struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Value  any    `json:"value,omitempty"`
}

// AgentConfigImportSkippedEntry reports one native config key skipped during import.
type AgentConfigImportSkippedEntry struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// AgentConfigImportSidecar reports one native sidecar that import will preserve or wrote.
type AgentConfigImportSidecar struct {
	Path   string `json:"path"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// AgentConfigImportResult reports one local native agent config import preview or write.
type AgentConfigImportResult struct {
	Framework       string                          `json:"framework"`
	DryRun          bool                            `json:"dry_run"`
	Sources         []AgentConfigImportSource       `json:"sources,omitempty"`
	Imported        []AgentConfigImportEntry        `json:"imported,omitempty"`
	Skipped         []AgentConfigImportSkippedEntry `json:"skipped,omitempty"`
	Sidecars        []AgentConfigImportSidecar      `json:"sidecars,omitempty"`
	SkippedSidecars []AgentConfigImportSidecar      `json:"skipped_sidecars,omitempty"`
	WrittenPaths    []string                        `json:"written_paths,omitempty"`
}

// ImportAgentConfig imports local native agent configuration into harness agent truth.
func ImportAgentConfig(ctx context.Context, input AgentConfigImportInput) (AgentConfigImportResult, error) {
	_ = ctx

	frameworkID := strings.TrimSpace(input.Framework)
	if _, ok := LookupFrameworkAdapter(frameworkID); !ok {
		return AgentConfigImportResult{}, fmt.Errorf("framework %q is not supported by this build", frameworkID)
	}
	if frameworkID != "codex" {
		return AgentConfigImportResult{}, fmt.Errorf("agent config import currently supports codex only")
	}

	homeDir := input.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return AgentConfigImportResult{}, fmt.Errorf("resolve home directory: %w", err)
		}
	}

	result := AgentConfigImportResult{
		Framework: frameworkID,
		DryRun:    !input.Write,
	}

	merged := map[string]any{}
	sourcesByKey := map[string]string{}
	var sidecarCandidate *agentConfigImportSidecarCandidate
	sidecarPath, hasSidecar := agentConfigSidecarRepoPath(frameworkID)
	blockUnifiedImport := false
	for _, source := range agentConfigImportSources(frameworkID) {
		result.Sources = append(result.Sources, AgentConfigImportSource{
			Scope: source.scope,
			Path:  source.displayPath,
		})
		data, err := os.ReadFile(frameworkRouteAbsolutePath(input.RepoRoot, homeDir, source.displayPath))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return AgentConfigImportResult{}, fmt.Errorf("read %s config %s: %w", source.scope, source.displayPath, err)
		}
		result.Sources[len(result.Sources)-1].Found = true
		config, err := parseNativeConfigMap(data, source.format)
		if err != nil {
			if hasSidecar {
				if agentConfigImportRawLooksUnsafe(data) {
					result.SkippedSidecars = append(result.SkippedSidecars, AgentConfigImportSidecar{
						Path:   sidecarPath,
						Source: source.scope,
						Reason: "unsafe_native_content",
					})
				} else {
					sidecarCandidate = &agentConfigImportSidecarCandidate{
						repoPath: sidecarPath,
						source:   source.scope,
						reason:   "parse_error",
						data:     append([]byte(nil), data...),
					}
				}
			}
			if source.scope == "project" {
				merged = map[string]any{}
				sourcesByKey = map[string]string{}
				blockUnifiedImport = true
			}
			continue
		}
		if source.scope == "project" && sidecarCandidate != nil && sidecarCandidate.reason == "parse_error" {
			sidecarCandidate = nil
		}
		sourceHasUnsafeNativeContent := false
		for _, keyPath := range nativeConfigKeyPaths(config) {
			value, _ := nativeConfigValueAtPath(config, keyPath)
			if reason, skipped := agentConfigImportSkipReason(keyPath, value); skipped {
				sourceHasUnsafeNativeContent = true
				result.Skipped = append(result.Skipped, AgentConfigImportSkippedEntry{
					Key:    keyPath,
					Source: source.scope,
					Reason: reason,
				})
				continue
			}
			nativeConfigSetPath(merged, keyPath, value)
			sourcesByKey[keyPath] = source.scope
		}
		if hasSidecar {
			if reason, needsSidecar := agentConfigImportSidecarReason(data, config, source.format, input.PreserveNative); needsSidecar {
				if sourceHasUnsafeNativeContent {
					result.SkippedSidecars = append(result.SkippedSidecars, AgentConfigImportSidecar{
						Path:   sidecarPath,
						Source: source.scope,
						Reason: "unsafe_native_content",
					})
				} else {
					sidecarCandidate = &agentConfigImportSidecarCandidate{
						repoPath: sidecarPath,
						source:   source.scope,
						reason:   reason,
						data:     append([]byte(nil), data...),
					}
				}
			}
		}
	}
	if sidecarCandidate != nil {
		result.Sidecars = append(result.Sidecars, AgentConfigImportSidecar{
			Path:   sidecarCandidate.repoPath,
			Source: sidecarCandidate.source,
			Reason: sidecarCandidate.reason,
		})
	}

	existingConfig, hasExistingConfig, err := LoadOptionalAgentUnifiedConfigFile(input.RepoRoot)
	if err != nil {
		return AgentConfigImportResult{}, fmt.Errorf("load existing unified agent config: %w", err)
	}
	importConfig := map[string]any{}
	if !blockUnifiedImport {
		for _, keyPath := range nativeConfigKeyPaths(merged) {
			value, _ := nativeConfigValueAtPath(merged, keyPath)
			if hasExistingConfig && !input.Replace {
				if _, exists := nativeConfigValueAtPath(existingConfig.Config, keyPath); exists {
					result.Skipped = append(result.Skipped, AgentConfigImportSkippedEntry{
						Key:    keyPath,
						Source: sourcesByKey[keyPath],
						Reason: "already_configured",
					})
					continue
				}
			}
			nativeConfigSetPath(importConfig, keyPath, value)
			result.Imported = append(result.Imported, AgentConfigImportEntry{
				Key:    keyPath,
				Source: sourcesByKey[keyPath],
				Value:  value,
			})
		}
	}
	sort.Slice(result.Imported, func(left, right int) bool {
		return result.Imported[left].Key < result.Imported[right].Key
	})
	sort.Slice(result.Skipped, func(left, right int) bool {
		if result.Skipped[left].Key == result.Skipped[right].Key {
			return result.Skipped[left].Source < result.Skipped[right].Source
		}
		return result.Skipped[left].Key < result.Skipped[right].Key
	})
	sort.Slice(result.SkippedSidecars, func(left, right int) bool {
		if result.SkippedSidecars[left].Path == result.SkippedSidecars[right].Path {
			return result.SkippedSidecars[left].Source < result.SkippedSidecars[right].Source
		}
		return result.SkippedSidecars[left].Path < result.SkippedSidecars[right].Path
	})

	if input.Write {
		frameworksFile, err := LoadOptionalFrameworksFile(input.RepoRoot)
		if err != nil {
			return AgentConfigImportResult{}, fmt.Errorf("load framework recommendation: %w", err)
		}
		if frameworksFile.RecommendedFramework != "" && frameworksFile.RecommendedFramework != frameworkID {
			return AgentConfigImportResult{}, fmt.Errorf(
				"recommended framework is %q; refusing to import %q config without changing the recommendation first",
				frameworksFile.RecommendedFramework,
				frameworkID,
			)
		}

		if sidecarCandidate != nil {
			sidecarFilename, err := writeAgentConfigImportSidecar(input.RepoRoot, sidecarCandidate.repoPath, sidecarCandidate.data)
			if err != nil {
				return AgentConfigImportResult{}, err
			}
			result.WrittenPaths = append(result.WrittenPaths, repoRelativeImportPath(input.RepoRoot, sidecarFilename))
		}

		if len(result.Imported) > 0 {
			nextConfig := AgentUnifiedConfigFile{
				Version: agentUnifiedConfigVersion,
				Targets: map[string]AgentUnifiedConfigTarget{
					frameworkID: {
						Enabled: true,
						Scope:   "project",
					},
				},
				Config: importConfig,
			}
			if hasExistingConfig {
				nextConfig = existingConfig
				if nextConfig.Targets == nil {
					nextConfig.Targets = map[string]AgentUnifiedConfigTarget{}
				}
				if _, ok := nextConfig.Targets[frameworkID]; !ok {
					nextConfig.Targets[frameworkID] = AgentUnifiedConfigTarget{
						Enabled: true,
						Scope:   "project",
					}
				}
				nextConfig.Config = cloneNativeConfigMap(existingConfig.Config)
				for _, keyPath := range nativeConfigKeyPaths(importConfig) {
					value, _ := nativeConfigValueAtPath(importConfig, keyPath)
					nativeConfigSetPath(nextConfig.Config, keyPath, value)
				}
			}

			configPath, err := WriteAgentUnifiedConfigFile(input.RepoRoot, nextConfig)
			if err != nil {
				return AgentConfigImportResult{}, fmt.Errorf("write unified agent config: %w", err)
			}
			result.WrittenPaths = append(result.WrittenPaths, repoRelativeImportPath(input.RepoRoot, configPath))
		}

		if frameworksFile.RecommendedFramework == "" {
			frameworksPath, err := WriteFrameworksFile(input.RepoRoot, FrameworksFile{
				SchemaVersion:        frameworksSchemaVersion,
				RecommendedFramework: frameworkID,
			})
			if err != nil {
				return AgentConfigImportResult{}, fmt.Errorf("write framework recommendation: %w", err)
			}
			result.WrittenPaths = append(result.WrittenPaths, repoRelativeImportPath(input.RepoRoot, frameworksPath))
		}
		sort.Strings(result.WrittenPaths)
	}

	return result, nil
}

type agentConfigImportSource struct {
	scope       string
	displayPath string
	format      nativeConfigFormat
}

type agentConfigImportSidecarCandidate struct {
	repoPath string
	source   string
	reason   string
	data     []byte
}

func agentConfigImportSources(frameworkID string) []agentConfigImportSource {
	sources := []agentConfigImportSource{}
	if path, format, _, ok := agentConfigTargetPaths(frameworkID, true); ok {
		sources = append(sources, agentConfigImportSource{
			scope:       "global",
			displayPath: path,
			format:      format,
		})
	}
	if path, format, _, ok := agentConfigTargetPaths(frameworkID, false); ok {
		sources = append(sources, agentConfigImportSource{
			scope:       "project",
			displayPath: path,
			format:      format,
		})
	}

	return sources
}

func agentConfigImportSidecarReason(data []byte, config map[string]any, format nativeConfigFormat, preserveNative bool) (string, bool) {
	if preserveNative {
		return "preserve_native", true
	}
	if nativeConfigContainsNativeOnlySyntax(data, format) {
		return "roundtrip_unstable", true
	}
	rendered, err := renderNativeConfigMap(config, format)
	if err != nil {
		return "roundtrip_unstable", true
	}
	reparsed, err := parseNativeConfigMap(rendered, format)
	if err != nil {
		return "roundtrip_unstable", true
	}
	if !reflect.DeepEqual(config, reparsed) {
		return "roundtrip_unstable", true
	}

	return "", false
}

func nativeConfigContainsNativeOnlySyntax(data []byte, format nativeConfigFormat) bool {
	switch format {
	case nativeConfigFormatTOML:
		for _, line := range strings.Split(string(data), "\n") {
			if stripTOMLComment(line) != line {
				return true
			}
		}
	case nativeConfigFormatJSON:
		return !bytes.Equal(bytes.TrimSpace(stripJSON5Syntax(data)), bytes.TrimSpace(data))
	}

	return false
}

func agentConfigImportRawLooksUnsafe(data []byte) bool {
	lowered := strings.ToLower(string(data))
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "credential"} {
		if strings.Contains(lowered, token) {
			return true
		}
	}

	return false
}

func agentConfigImportSkipReason(keyPath string, value any) (string, bool) {
	for _, segment := range strings.Split(strings.ToLower(keyPath), ".") {
		normalized := strings.NewReplacer("_", "", "-", "").Replace(segment)
		switch {
		case strings.Contains(normalized, "apikey"),
			strings.Contains(normalized, "token"),
			strings.Contains(normalized, "secret"),
			strings.Contains(normalized, "password"),
			strings.Contains(normalized, "credential"):
			return "sensitive", true
		}
	}
	if text, ok := value.(string); ok {
		trimmed := strings.TrimSpace(text)
		if filepath.IsAbs(trimmed) || strings.HasPrefix(trimmed, "~/") {
			return "local_path", true
		}
	}

	return "", false
}

func writeAgentConfigImportSidecar(repoRoot string, repoPath string, data []byte) (string, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	if err := contractutil.AtomicWriteFileMode(filename, data, 0o600); err != nil {
		return "", fmt.Errorf("write agent config sidecar %s: %w", repoPath, err)
	}

	return filename, nil
}

func repoRelativeImportPath(repoRoot string, filename string) string {
	relative, err := filepath.Rel(repoRoot, filename)
	if err != nil {
		return filepath.ToSlash(filename)
	}

	return filepath.ToSlash(relative)
}
