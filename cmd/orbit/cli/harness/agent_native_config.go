package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	agentUnifiedConfigVersion = 1
	agentUnifiedConfigSource  = ".harness/agents/config.yaml"
)

// AgentUnifiedConfigFile is the v0.78 first-truth host for runtime-level agent config.
type AgentUnifiedConfigFile struct {
	Version int
	Targets map[string]AgentUnifiedConfigTarget
	Config  map[string]any
	Hooks   AgentUnifiedHooks
}

// AgentUnifiedConfigTarget controls whether and how one framework consumes unified config.
type AgentUnifiedConfigTarget struct {
	Enabled bool
	Scope   string
}

type nativeConfigFormat string

const (
	nativeConfigFormatJSON nativeConfigFormat = "json"
	nativeConfigFormatTOML nativeConfigFormat = "toml"
)

// AgentUnifiedConfigRepoPath returns the repo-relative v0.78 unified config truth path.
func AgentUnifiedConfigRepoPath() string {
	return agentUnifiedConfigSource
}

func AgentUnifiedConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".harness", "agents", "config.yaml")
}

func LoadOptionalAgentUnifiedConfigFile(repoRoot string) (AgentUnifiedConfigFile, bool, error) {
	file, err := LoadAgentUnifiedConfigFile(repoRoot)
	if err == nil {
		return file, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return AgentUnifiedConfigFile{}, false, nil
	}

	return AgentUnifiedConfigFile{}, false, err
}

func LoadAgentUnifiedConfigFile(repoRoot string) (AgentUnifiedConfigFile, error) {
	filename := AgentUnifiedConfigPath(repoRoot)
	data, err := os.ReadFile(filename) //nolint:gosec // Path is repo-local and built from the fixed config truth path.
	if err != nil {
		return AgentUnifiedConfigFile{}, fmt.Errorf("read %s: %w", filename, err)
	}
	file, err := ParseAgentUnifiedConfigFileData(data)
	if err != nil {
		return AgentUnifiedConfigFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// WriteAgentUnifiedConfigFile validates and writes .harness/agents/config.yaml.
func WriteAgentUnifiedConfigFile(repoRoot string, file AgentUnifiedConfigFile) (string, error) {
	filename := AgentUnifiedConfigPath(repoRoot)
	data, err := MarshalAgentUnifiedConfigFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}
	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

// MarshalAgentUnifiedConfigFile validates and encodes one unified agent config file.
func MarshalAgentUnifiedConfigFile(file AgentUnifiedConfigFile) ([]byte, error) {
	if file.Version != agentUnifiedConfigVersion {
		return nil, fmt.Errorf("version must be %d", agentUnifiedConfigVersion)
	}

	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "version", contractutil.IntNode(file.Version))
	if len(file.Targets) > 0 {
		targetsNode := contractutil.MappingNode()
		targetIDs := make([]string, 0, len(file.Targets))
		for targetID, target := range file.Targets {
			if _, ok := LookupFrameworkAdapter(targetID); !ok {
				return nil, fmt.Errorf("targets.%s is not supported by this build", targetID)
			}
			if target.Scope != "" && target.Scope != "project" && target.Scope != "global" && target.Scope != "hybrid" {
				return nil, fmt.Errorf("targets.%s.scope must be project, global, or hybrid", targetID)
			}
			targetIDs = append(targetIDs, targetID)
		}
		sort.Strings(targetIDs)
		for _, targetID := range targetIDs {
			target := file.Targets[targetID]
			targetNode := contractutil.MappingNode()
			contractutil.AppendMapping(targetNode, "enabled", contractutil.BoolNode(target.Enabled))
			if target.Scope != "" {
				contractutil.AppendMapping(targetNode, "scope", contractutil.StringNode(target.Scope))
			}
			contractutil.AppendMapping(targetsNode, targetID, targetNode)
		}
		contractutil.AppendMapping(root, "targets", targetsNode)
	}
	if len(file.Config) > 0 {
		configNode, err := nativeConfigYAMLNode(file.Config, "config")
		if err != nil {
			return nil, err
		}
		contractutil.AppendMapping(root, "config", configNode)
	}
	if agentUnifiedHooksPresent(file.Hooks) {
		contractutil.AppendMapping(root, "hooks", agentUnifiedHooksNode(file.Hooks))
	}

	data, err := contractutil.EncodeYAMLDocument(root)
	if err != nil {
		return nil, fmt.Errorf("encode agent unified config: %w", err)
	}

	return data, nil
}

func ParseAgentUnifiedConfigFileData(data []byte) (AgentUnifiedConfigFile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return AgentUnifiedConfigFile{}, fmt.Errorf("decode agent config file: %w", err)
	}
	if len(document.Content) != 1 {
		return AgentUnifiedConfigFile{}, fmt.Errorf("agent config document must contain exactly one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return AgentUnifiedConfigFile{}, fmt.Errorf("agent config file must be a YAML mapping")
	}

	file := AgentUnifiedConfigFile{
		Targets: map[string]AgentUnifiedConfigTarget{},
		Config:  map[string]any{},
	}
	for index := 0; index < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		switch keyNode.Value {
		case "version", "schema_version":
			version, err := yamlNodeInt(valueNode, keyNode.Value)
			if err != nil {
				return AgentUnifiedConfigFile{}, err
			}
			file.Version = version
		case "targets":
			targets, err := parseAgentUnifiedConfigTargets(valueNode)
			if err != nil {
				return AgentUnifiedConfigFile{}, err
			}
			file.Targets = targets
		case "config":
			config, err := yamlNodeMappingToNativeMap(valueNode, "config")
			if err != nil {
				return AgentUnifiedConfigFile{}, err
			}
			file.Config = config
		case "hooks":
			hooks, err := parseAgentUnifiedHooks(valueNode)
			if err != nil {
				return AgentUnifiedConfigFile{}, err
			}
			file.Hooks = hooks
		default:
			return AgentUnifiedConfigFile{}, fmt.Errorf("unknown top-level field %q", keyNode.Value)
		}
	}
	if file.Version != agentUnifiedConfigVersion {
		return AgentUnifiedConfigFile{}, fmt.Errorf("version must be %d", agentUnifiedConfigVersion)
	}

	return file, nil
}

func nativeConfigYAMLNode(value any, path string) (*yaml.Node, error) {
	switch typed := value.(type) {
	case map[string]any:
		node := contractutil.MappingNode()
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := nativeConfigYAMLNode(typed[key], joinNativeConfigKey(path, key))
			if err != nil {
				return nil, err
			}
			contractutil.AppendMapping(node, key, child)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for index, item := range typed {
			child, err := nativeConfigYAMLNode(item, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case string:
		return contractutil.StringNode(typed), nil
	case bool:
		return contractutil.BoolNode(typed), nil
	case int:
		return contractutil.IntNode(typed), nil
	case float64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(typed, 'f', -1, 64)}, nil
	default:
		return nil, fmt.Errorf("%s uses unsupported config value type %T", path, value)
	}
}

func agentUnifiedHooksPresent(hooks AgentUnifiedHooks) bool {
	return hooks.Enabled ||
		hooks.UnsupportedBehavior != "" ||
		hooks.Defaults.TimeoutSeconds != 0 ||
		hooks.Defaults.Runner != "" ||
		len(hooks.Entries) > 0
}

func agentUnifiedHooksNode(hooks AgentUnifiedHooks) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "enabled", contractutil.BoolNode(hooks.Enabled))
	if hooks.UnsupportedBehavior != "" {
		contractutil.AppendMapping(node, "unsupported_behavior", contractutil.StringNode(hooks.UnsupportedBehavior))
	}
	if hooks.Defaults.TimeoutSeconds != 0 || hooks.Defaults.Runner != "" {
		defaultsNode := contractutil.MappingNode()
		if hooks.Defaults.TimeoutSeconds != 0 {
			contractutil.AppendMapping(defaultsNode, "timeout_seconds", contractutil.IntNode(hooks.Defaults.TimeoutSeconds))
		}
		if hooks.Defaults.Runner != "" {
			contractutil.AppendMapping(defaultsNode, "runner", contractutil.StringNode(hooks.Defaults.Runner))
		}
		contractutil.AppendMapping(node, "defaults", defaultsNode)
	}
	if len(hooks.Entries) > 0 {
		entriesNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, entry := range hooks.Entries {
			entriesNode.Content = append(entriesNode.Content, agentUnifiedHookEntryNode(entry))
		}
		contractutil.AppendMapping(node, "entries", entriesNode)
	}

	return node
}

func agentUnifiedHookEntryNode(entry AgentHookEntry) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "id", contractutil.StringNode(entry.ID))
	contractutil.AppendMapping(node, "enabled", contractutil.BoolNode(entry.Enabled))
	if entry.Description != "" {
		contractutil.AppendMapping(node, "description", contractutil.StringNode(entry.Description))
	}
	eventNode := contractutil.MappingNode()
	contractutil.AppendMapping(eventNode, "kind", contractutil.StringNode(entry.Event.Kind))
	contractutil.AppendMapping(node, "event", eventNode)
	if len(entry.Match.Tools) > 0 || len(entry.Match.CommandPatterns) > 0 {
		matchNode := contractutil.MappingNode()
		if len(entry.Match.Tools) > 0 {
			contractutil.AppendMapping(matchNode, "tools", stringSequenceNode(entry.Match.Tools))
		}
		if len(entry.Match.CommandPatterns) > 0 {
			contractutil.AppendMapping(matchNode, "command_patterns", stringSequenceNode(entry.Match.CommandPatterns))
		}
		contractutil.AppendMapping(node, "match", matchNode)
	}

	handlerNode := contractutil.MappingNode()
	contractutil.AppendMapping(handlerNode, "type", contractutil.StringNode(entry.Handler.Type))
	contractutil.AppendMapping(handlerNode, "path", contractutil.StringNode(entry.Handler.Path))
	if entry.Handler.TimeoutSeconds != 0 {
		contractutil.AppendMapping(handlerNode, "timeout_seconds", contractutil.IntNode(entry.Handler.TimeoutSeconds))
	}
	if entry.Handler.StatusMessage != "" {
		contractutil.AppendMapping(handlerNode, "status_message", contractutil.StringNode(entry.Handler.StatusMessage))
	}
	contractutil.AppendMapping(node, "handler", handlerNode)

	if len(entry.Targets) > 0 {
		targetsNode := contractutil.MappingNode()
		targetIDs := make([]string, 0, len(entry.Targets))
		for targetID := range entry.Targets {
			targetIDs = append(targetIDs, targetID)
		}
		sort.Strings(targetIDs)
		for _, targetID := range targetIDs {
			contractutil.AppendMapping(targetsNode, targetID, contractutil.BoolNode(entry.Targets[targetID]))
		}
		contractutil.AppendMapping(node, "targets", targetsNode)
	}

	return node
}

func stringSequenceNode(values []string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		node.Content = append(node.Content, contractutil.StringNode(value))
	}

	return node
}

func parseAgentUnifiedConfigTargets(node *yaml.Node) (map[string]AgentUnifiedConfigTarget, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("targets must be a YAML mapping")
	}
	targets := map[string]AgentUnifiedConfigTarget{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		frameworkID, ok := normalizeAgentConfigTargetID(keyNode.Value)
		if !ok {
			return nil, fmt.Errorf("targets.%s is not supported by this build", keyNode.Value)
		}
		if valueNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("targets.%s must be a YAML mapping", keyNode.Value)
		}
		target := AgentUnifiedConfigTarget{Enabled: true}
		for child := 0; child < len(valueNode.Content); child += 2 {
			childKey := valueNode.Content[child]
			childValue := valueNode.Content[child+1]
			switch childKey.Value {
			case "enabled":
				enabled, err := yamlNodeBool(childValue, "targets."+keyNode.Value+".enabled")
				if err != nil {
					return nil, err
				}
				target.Enabled = enabled
			case "scope":
				scope, err := yamlNodeString(childValue, "targets."+keyNode.Value+".scope")
				if err != nil {
					return nil, err
				}
				target.Scope = strings.TrimSpace(scope)
				if target.Scope != "" && target.Scope != "project" && target.Scope != "global" && target.Scope != "hybrid" {
					return nil, fmt.Errorf("targets.%s.scope must be project, global, or hybrid", keyNode.Value)
				}
			case "workspace":
				// OpenClaw hook/workspace support is handled by the hooks lane.
			default:
				return nil, fmt.Errorf("unknown field targets.%s.%s", keyNode.Value, childKey.Value)
			}
		}
		targets[frameworkID] = target
	}

	return targets, nil
}

func normalizeAgentConfigTargetID(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "codex":
		return "codex", true
	case "claude", "claudeCode", "claude-code", "claudecode":
		return "claudecode", true
	case "openclaw":
		return "openclaw", true
	default:
		return "", false
	}
}

func yamlNodeMappingToNativeMap(node *yaml.Node, path string) (map[string]any, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a YAML mapping", path)
	}
	result := map[string]any{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		key, err := yamlNodeString(keyNode, path+" key")
		if err != nil {
			return nil, err
		}
		value, err := yamlNodeToNative(valueNode, joinNativeConfigKey(path, key))
		if err != nil {
			return nil, err
		}
		result[key] = value
	}

	return result, nil
}

func yamlNodeToNative(node *yaml.Node, path string) (any, error) {
	switch node.Kind {
	case yaml.MappingNode:
		return yamlNodeMappingToNativeMap(node, path)
	case yaml.SequenceNode:
		values := make([]any, 0, len(node.Content))
		for index, child := range node.Content {
			value, err := yamlNodeToNative(child, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!bool":
			return yamlNodeBool(node, path)
		case "!!int":
			return yamlNodeInt(node, path)
		case "!!float":
			value, err := strconv.ParseFloat(strings.TrimSpace(node.Value), 64)
			if err != nil {
				return nil, fmt.Errorf("%s must be a number", path)
			}
			return value, nil
		case "!!null":
			return nil, fmt.Errorf("%s must not be null", path)
		default:
			return node.Value, nil
		}
	default:
		return nil, fmt.Errorf("%s uses unsupported YAML node kind", path)
	}
}

func yamlNodeString(node *yaml.Node, path string) (string, error) {
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("%s must be a scalar string", path)
	}

	return node.Value, nil
}

func yamlNodeBool(node *yaml.Node, path string) (bool, error) {
	if node.Kind != yaml.ScalarNode {
		return false, fmt.Errorf("%s must be a scalar bool", path)
	}
	value, err := strconv.ParseBool(strings.TrimSpace(node.Value))
	if err != nil {
		return false, fmt.Errorf("%s must be a bool", path)
	}

	return value, nil
}

func yamlNodeInt(node *yaml.Node, path string) (int, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("%s must be a scalar integer", path)
	}
	value, err := strconv.Atoi(strings.TrimSpace(node.Value))
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", path)
	}

	return value, nil
}

func agentConfigSidecarRepoPath(frameworkID string) (string, bool) {
	switch frameworkID {
	case "codex":
		return ".harness/agents/codex.config.toml", true
	case "claudecode":
		return ".harness/agents/claude-code.settings.json", true
	case "openclaw":
		return ".harness/agents/openclaw.openclaw.json5", true
	default:
		return "", false
	}
}

func agentConfigTargetPaths(frameworkID string, global bool) (string, nativeConfigFormat, string, bool) {
	switch frameworkID {
	case "codex":
		if global {
			return "~/.codex/config.toml", nativeConfigFormatTOML, "merge-config", true
		}
		return ".codex/config.toml", nativeConfigFormatTOML, "merge-config", true
	case "claudecode":
		if global {
			return "~/.claude/settings.json", nativeConfigFormatJSON, "merge-config", true
		}
		return ".claude/settings.json", nativeConfigFormatJSON, "merge-config", true
	case "openclaw":
		if global {
			return "~/.openclaw/openclaw.json", nativeConfigFormatJSON, "patch-global-config", true
		}
		return "", "", "", false
	default:
		return "", "", "", false
	}
}

func frameworkConfigArtifactName(frameworkID string) string {
	return frameworkID + "-config"
}

func frameworkAgentConfigEnabled(summary FrameworkInspectSummary, frameworkID string) (AgentUnifiedConfigTarget, bool) {
	if summary.AgentConfig == nil {
		return AgentUnifiedConfigTarget{}, false
	}
	target, ok := summary.AgentConfig.Targets[frameworkID]
	if !ok || !target.Enabled {
		return AgentUnifiedConfigTarget{}, false
	}

	return target, true
}

func frameworkAgentConfigSidecars(repoRoot string) (map[string]string, error) {
	sidecars := map[string]string{}
	for _, adapter := range RegisteredFrameworkAdapters() {
		repoPath, ok := agentConfigSidecarRepoPath(adapter.ID)
		if !ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(repoPath))); err == nil {
			sidecars[adapter.ID] = repoPath
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat agent config sidecar %s: %w", repoPath, err)
		}
	}

	return sidecars, nil
}

func cloneAgentUnifiedConfigTargets(targets map[string]AgentUnifiedConfigTarget) map[string]AgentUnifiedConfigTarget {
	clone := make(map[string]AgentUnifiedConfigTarget, len(targets))
	for key, value := range targets {
		clone[key] = value
	}

	return clone
}

func frameworkAgentConfigSourceFiles(summary FrameworkInspectSummary, frameworkID string) (string, []string) {
	if summary.AgentConfig == nil {
		return "", nil
	}
	sidecar := ""
	if summary.AgentConfig.Sidecars != nil {
		sidecar = summary.AgentConfig.Sidecars[frameworkID]
	}
	sourceFiles := []string{summary.AgentConfig.Source}
	if sidecar != "" {
		sourceFiles = append(sourceFiles, sidecar)
	}

	return sidecar, sourceFiles
}

func buildProjectAgentConfigRouteOutput(frameworkID string, summary FrameworkInspectSummary) (FrameworkRoutePlanOutput, bool) {
	target, ok := frameworkAgentConfigEnabled(summary, frameworkID)
	if !ok || target.Scope == "global" {
		return FrameworkRoutePlanOutput{}, false
	}
	path, _, mode, ok := agentConfigTargetPaths(frameworkID, false)
	if !ok {
		return FrameworkRoutePlanOutput{}, false
	}
	sidecar, sourceFiles := frameworkAgentConfigSourceFiles(summary, frameworkID)

	return FrameworkRoutePlanOutput{
		Artifact:       frameworkConfigArtifactName(frameworkID),
		ArtifactType:   "agent-config",
		Route:          "project_config",
		Recommended:    true,
		Source:         AgentUnifiedConfigRepoPath(),
		Sidecar:        sidecar,
		SourceFiles:    sourceFiles,
		Path:           path,
		Action:         mode,
		Kind:           "config",
		Mode:           mode,
		EffectiveScope: "project",
	}, true
}

func buildGlobalAgentConfigRouteOutput(frameworkID string, summary FrameworkInspectSummary) (FrameworkRoutePlanOutput, bool) {
	if _, ok := frameworkAgentConfigEnabled(summary, frameworkID); !ok {
		return FrameworkRoutePlanOutput{}, false
	}
	path, _, mode, ok := agentConfigTargetPaths(frameworkID, true)
	if !ok {
		return FrameworkRoutePlanOutput{}, false
	}
	sidecar, sourceFiles := frameworkAgentConfigSourceFiles(summary, frameworkID)

	return FrameworkRoutePlanOutput{
		Artifact:       frameworkConfigArtifactName(frameworkID),
		ArtifactType:   "agent-config",
		Route:          "global_config",
		Recommended:    false,
		Source:         AgentUnifiedConfigRepoPath(),
		Sidecar:        sidecar,
		SourceFiles:    sourceFiles,
		Path:           path,
		Action:         mode,
		Kind:           "config",
		Mode:           mode,
		EffectiveScope: "global",
	}, true
}

func buildCompatibilityAgentConfigRouteOutput(frameworkID string, summary FrameworkInspectSummary) (FrameworkRoutePlanOutput, bool) {
	if _, ok := frameworkAgentConfigEnabled(summary, frameworkID); !ok {
		return FrameworkRoutePlanOutput{}, false
	}
	if _, _, _, ok := agentConfigTargetPaths(frameworkID, false); ok {
		return FrameworkRoutePlanOutput{}, false
	}
	globalPath, _, _, ok := agentConfigTargetPaths(frameworkID, true)
	if !ok {
		return FrameworkRoutePlanOutput{}, false
	}

	return FrameworkRoutePlanOutput{
		Artifact:       frameworkConfigArtifactName(frameworkID),
		ArtifactType:   "agent-config",
		Route:          "project_compatibility",
		Recommended:    false,
		Source:         AgentUnifiedConfigRepoPath(),
		Path:           globalPath,
		Kind:           "config",
		Mode:           "not_implemented",
		EffectiveScope: "global",
	}, true
}

func activationOutputFromAgentConfigRoute(repoRoot string, gitDir string, homeDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput) (FrameworkActivationOutput, error) {
	compiledPath, generatedKeys, err := compileAgentNativeConfig(repoRoot, gitDir, frameworkID, routeOutput)
	if err != nil {
		return FrameworkActivationOutput{}, err
	}
	backupPath := ""
	if routeOutput.EffectiveScope == "global" {
		backupPath = frameworkConfigBackupPath(gitDir, frameworkID, routeOutput)
	}

	return FrameworkActivationOutput{
		Path:           routeOutput.Path,
		AbsolutePath:   frameworkRouteAbsolutePath(repoRoot, homeDir, routeOutput.Path),
		Kind:           routeOutput.Kind,
		Action:         routeOutput.Action,
		Target:         compiledPath,
		Artifact:       routeOutput.Artifact,
		ArtifactType:   routeOutput.ArtifactType,
		Source:         routeOutput.Source,
		Sidecar:        routeOutput.Sidecar,
		SourceFiles:    append([]string(nil), routeOutput.SourceFiles...),
		Route:          routeOutput.Route,
		Mode:           routeOutput.Mode,
		EffectiveScope: routeOutput.EffectiveScope,
		GeneratedKeys:  append([]string(nil), generatedKeys...),
		PatchOwnedKeys: append([]string(nil), generatedKeys...),
		BackupPath:     backupPath,
	}, nil
}

func compileAgentNativeConfig(repoRoot string, gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput) (string, []string, error) {
	configFile, err := LoadAgentUnifiedConfigFile(repoRoot)
	if err != nil {
		return "", nil, err
	}
	target, ok := configFile.Targets[frameworkID]
	if !ok || !target.Enabled {
		return "", nil, fmt.Errorf("agent config target %q is not enabled", frameworkID)
	}
	_, format, _, ok := agentConfigTargetPaths(frameworkID, routeOutput.EffectiveScope == "global")
	if !ok {
		return "", nil, fmt.Errorf("framework %q does not support %s agent config", frameworkID, routeOutput.EffectiveScope)
	}

	sidecarConfig := map[string]any{}
	if routeOutput.Sidecar != "" {
		parsed, err := loadAgentConfigSidecar(repoRoot, routeOutput.Sidecar, format)
		if err != nil {
			return "", nil, err
		}
		sidecarConfig = parsed
	}
	merged, err := mergeAgentNativeConfigMaps(sidecarConfig, configFile.Config, routeOutput.Sidecar)
	if err != nil {
		return "", nil, err
	}
	generatedKeys := nativeConfigKeyPaths(merged)
	data, err := renderNativeConfigMap(merged, format)
	if err != nil {
		return "", nil, err
	}

	compiledPath := frameworkCompiledConfigPath(gitDir, frameworkID, routeOutput, format)
	if err := contractutil.AtomicWriteFileMode(compiledPath, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("write compiled native config: %w", err)
	}

	return compiledPath, generatedKeys, nil
}

func loadAgentConfigSidecar(repoRoot string, repoPath string, format nativeConfigFormat) (map[string]any, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(repoPath))) //nolint:gosec // Sidecar path is resolved from the fixed adapter matrix.
	if err != nil {
		return nil, fmt.Errorf("read agent config sidecar %s: %w", repoPath, err)
	}
	parsed, err := parseNativeConfigMap(data, format)
	if err != nil {
		return nil, fmt.Errorf("parse agent config sidecar %q: %w", repoPath, err)
	}

	return parsed, nil
}

func mergeAgentNativeConfigMaps(sidecar map[string]any, unified map[string]any, sidecarPath string) (map[string]any, error) {
	merged := cloneNativeConfigMap(sidecar)
	for _, keyPath := range nativeConfigKeyPaths(unified) {
		unifiedValue, _ := nativeConfigValueAtPath(unified, keyPath)
		if sidecarValue, ok := nativeConfigValueAtPath(sidecar, keyPath); ok && !reflect.DeepEqual(sidecarValue, unifiedValue) {
			return nil, fmt.Errorf("sidecar %q cannot override unified config key %q", sidecarPath, keyPath)
		}
		nativeConfigSetPath(merged, keyPath, unifiedValue)
	}

	return merged, nil
}

func ensureFrameworkConfigOutput(output FrameworkActivationOutput) (bool, error) {
	format, err := nativeConfigFormatForOutput(output)
	if err != nil {
		return false, err
	}
	compiledData, err := os.ReadFile(output.Target)
	if err != nil {
		return false, fmt.Errorf("read compiled native config for %s: %w", output.Path, err)
	}
	generatedConfig, err := parseNativeConfigMap(compiledData, format)
	if err != nil {
		return false, fmt.Errorf("parse compiled native config for %s: %w", output.Path, err)
	}

	existingData, existing, err := readExistingNativeConfig(output.AbsolutePath, format)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			existingData = nil
			existing = map[string]any{}
		} else {
			return false, err
		}
	}
	for _, keyPath := range output.GeneratedKeys {
		generatedValue, ok := nativeConfigValueAtPath(generatedConfig, keyPath)
		if !ok {
			continue
		}
		if existingValue, ok := nativeConfigValueAtPath(existing, keyPath); ok && !reflect.DeepEqual(existingValue, generatedValue) {
			return false, fmt.Errorf("native config target %s has conflicting unmanaged key %q", output.Path, keyPath)
		}
	}

	merged := cloneNativeConfigMap(existing)
	for _, keyPath := range output.GeneratedKeys {
		generatedValue, ok := nativeConfigValueAtPath(generatedConfig, keyPath)
		if ok {
			nativeConfigSetPath(merged, keyPath, generatedValue)
		}
	}
	updatedData, err := renderNativeConfigMap(merged, format)
	if err != nil {
		return false, err
	}
	if bytes.Equal(bytes.TrimSpace(existingData), bytes.TrimSpace(updatedData)) {
		return false, nil
	}

	if len(existingData) > 0 && output.BackupPath != "" {
		if _, err := os.Stat(output.BackupPath); errors.Is(err, os.ErrNotExist) {
			if err := contractutil.AtomicWriteFileMode(output.BackupPath, existingData, 0o600); err != nil {
				return false, fmt.Errorf("write native config backup for %s: %w", output.Path, err)
			}
		} else if err != nil {
			return false, fmt.Errorf("stat native config backup for %s: %w", output.Path, err)
		}
	}
	if err := contractutil.AtomicWriteFileMode(output.AbsolutePath, updatedData, 0o600); err != nil {
		return false, fmt.Errorf("write native config %s: %w", output.Path, err)
	}

	return true, nil
}

func frameworkConfigOutputOwned(output FrameworkActivationOutput) (bool, error) {
	_, found, err := frameworkConfigOutputFinding(output)
	if err != nil {
		return false, err
	}

	return !found, nil
}

func frameworkConfigOutputFinding(output FrameworkActivationOutput) (FrameworkCheckFinding, bool, error) {
	format, err := nativeConfigFormatForOutput(output)
	if err != nil {
		return FrameworkCheckFinding{}, false, err
	}
	compiledData, err := os.ReadFile(output.Target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{
				Kind:    configOutputFindingKind(output),
				Path:    output.Path,
				Message: "compiled native config cache is missing",
			}, true, nil
		}
		return FrameworkCheckFinding{}, false, fmt.Errorf("read compiled native config for %s: %w", output.Path, err)
	}
	generatedConfig, err := parseNativeConfigMap(compiledData, format)
	if err != nil {
		return FrameworkCheckFinding{}, false, fmt.Errorf("parse compiled native config for %s: %w", output.Path, err)
	}
	_, existing, err := readExistingNativeConfig(output.AbsolutePath, format)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{
				Kind:    configOutputFindingKind(output),
				Path:    output.Path,
				Message: configOutputFindingMessage(output),
			}, true, nil
		}
		return FrameworkCheckFinding{}, false, err
	}
	for _, keyPath := range output.GeneratedKeys {
		generatedValue, ok := nativeConfigValueAtPath(generatedConfig, keyPath)
		if !ok {
			return FrameworkCheckFinding{
				Kind:    configOutputFindingKind(output),
				Path:    output.Path,
				Message: "compiled native config cache is stale",
			}, true, nil
		}
		existingValue, ok := nativeConfigValueAtPath(existing, keyPath)
		if !ok || !reflect.DeepEqual(existingValue, generatedValue) {
			return FrameworkCheckFinding{
				Kind:    configOutputFindingKind(output),
				Path:    output.Path,
				Message: configOutputFindingMessage(output),
			}, true, nil
		}
	}

	return FrameworkCheckFinding{}, false, nil
}

func configOutputFindingKind(output FrameworkActivationOutput) string {
	if strings.HasPrefix(output.Path, "~/") || output.EffectiveScope == "global" || output.EffectiveScope == "hybrid" {
		return "config_patch_missing"
	}

	return "config_output_stale"
}

func configOutputFindingMessage(output FrameworkActivationOutput) string {
	if configOutputFindingKind(output) == "config_patch_missing" {
		return "framework-managed native config patch keys are missing or stale"
	}

	return "framework-managed native config keys differ from the compiled activation output"
}

func rollbackFrameworkConfigOutput(output FrameworkActivationOutput) error {
	if output.BackupPath != "" {
		backupData, err := os.ReadFile(output.BackupPath)
		if err == nil {
			if err := contractutil.AtomicWriteFileMode(output.AbsolutePath, backupData, 0o600); err != nil {
				return fmt.Errorf("restore native config backup for %s: %w", output.Path, err)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read native config backup for %s: %w", output.Path, err)
		}
	}
	if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove native config %s: %w", output.Path, err)
	}

	return nil
}

func removeFrameworkConfigOutput(output FrameworkActivationOutput) (bool, error) {
	format, err := nativeConfigFormatForOutput(output)
	if err != nil {
		return false, err
	}
	existingData, existing, err := readExistingNativeConfig(output.AbsolutePath, format)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	updated := cloneNativeConfigMap(existing)
	for _, keyPath := range output.PatchOwnedKeys {
		nativeConfigDeletePath(updated, keyPath)
	}
	updatedData, err := renderNativeConfigMap(updated, format)
	if err != nil {
		return false, err
	}
	if bytes.Equal(bytes.TrimSpace(existingData), bytes.TrimSpace(updatedData)) {
		return false, nil
	}
	if len(updated) == 0 {
		if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("remove native config %s: %w", output.Path, err)
		}
		return true, nil
	}
	if err := contractutil.AtomicWriteFileMode(output.AbsolutePath, updatedData, 0o600); err != nil {
		return false, fmt.Errorf("write native config %s: %w", output.Path, err)
	}

	return true, nil
}

func readExistingNativeConfig(filename string, format nativeConfigFormat) ([]byte, map[string]any, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, map[string]any{}, os.ErrNotExist
		}
		return nil, nil, fmt.Errorf("stat native config %s: %w", filename, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("native config target %s already exists as a symlink", filename)
	}
	data, err := os.ReadFile(filename) //nolint:gosec // Path is an adapter target path resolved by the route model.
	if err != nil {
		return nil, nil, fmt.Errorf("read native config %s: %w", filename, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return data, map[string]any{}, nil
	}
	config, err := parseNativeConfigMap(data, format)
	if err != nil {
		return nil, nil, fmt.Errorf("parse native config %s: %w", filename, err)
	}

	return data, config, nil
}

func frameworkCompiledConfigPath(gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, format nativeConfigFormat) string {
	extension := "json"
	if format == nativeConfigFormatTOML {
		extension = "toml"
	}
	scope := routeOutput.EffectiveScope
	if scope == "" {
		scope = "project"
	}

	return filepath.Join(gitDir, "orbit", "state", "agents", "compiled", frameworkID, "config", scope+"."+extension)
}

func frameworkConfigBackupPath(gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput) string {
	name := strings.NewReplacer("~", "home", "/", "_", "\\", "_").Replace(routeOutput.Path)
	return filepath.Join(gitDir, "orbit", "state", "agents", "backups", frameworkID, "config", name+".bak")
}

func nativeConfigFormatForOutput(output FrameworkActivationOutput) (nativeConfigFormat, error) {
	switch {
	case strings.HasSuffix(output.Path, ".toml"):
		return nativeConfigFormatTOML, nil
	case strings.HasSuffix(output.Path, ".json"), strings.HasSuffix(output.Path, ".json5"):
		return nativeConfigFormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported native config target %s", output.Path)
	}
}

func parseNativeConfigMap(data []byte, format nativeConfigFormat) (map[string]any, error) {
	switch format {
	case nativeConfigFormatJSON:
		var value map[string]any
		if err := json.Unmarshal(stripJSON5Syntax(data), &value); err != nil {
			return nil, fmt.Errorf("decode JSON config: %w", err)
		}
		if value == nil {
			value = map[string]any{}
		}
		return normalizeJSONNumbers(value), nil
	case nativeConfigFormatTOML:
		return parseSimpleTOMLMap(data)
	default:
		return nil, fmt.Errorf("unsupported native config format %q", format)
	}
}

func renderNativeConfigMap(config map[string]any, format nativeConfigFormat) ([]byte, error) {
	switch format {
	case nativeConfigFormatJSON:
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode JSON config: %w", err)
		}
		return append(data, '\n'), nil
	case nativeConfigFormatTOML:
		return renderSimpleTOMLMap(config)
	default:
		return nil, fmt.Errorf("unsupported native config format %q", format)
	}
}

func parseSimpleTOMLMap(data []byte) (map[string]any, error) {
	result := map[string]any{}
	currentPath := ""
	for lineNumber, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentPath = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if currentPath == "" {
				return nil, fmt.Errorf("line %d: table name must not be empty", lineNumber+1)
			}
			continue
		}
		key, valueText, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key = value", lineNumber+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: key must not be empty", lineNumber+1)
		}
		value, err := parseSimpleTOMLValue(strings.TrimSpace(valueText))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		path := key
		if currentPath != "" {
			path = currentPath + "." + key
		}
		nativeConfigSetPath(result, path, value)
	}

	return result, nil
}

func stripTOMLComment(line string) string {
	inString := false
	escaped := false
	for index, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == '#' && !inString {
			return line[:index]
		}
	}

	return line
}

func parseSimpleTOMLValue(value string) (any, error) {
	if strings.HasPrefix(value, "\"") {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return nil, fmt.Errorf("decode quoted string: %w", err)
		}
		return parsed, nil
	}
	if value == "true" || value == "false" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("decode bool: %w", err)
		}
		return parsed, nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		inside := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
		if inside == "" {
			return []any{}, nil
		}
		parts := splitSimpleTOMLArray(inside)
		values := make([]any, 0, len(parts))
		for _, part := range parts {
			parsed, err := parseSimpleTOMLValue(strings.TrimSpace(part))
			if err != nil {
				return nil, err
			}
			values = append(values, parsed)
		}
		return values, nil
	}
	if strings.Contains(value, ".") {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed, nil
		}
	}
	parsed, err := strconv.Atoi(value)
	if err == nil {
		return parsed, nil
	}

	return nil, fmt.Errorf("unsupported TOML value %q", value)
}

func splitSimpleTOMLArray(value string) []string {
	parts := []string{}
	start := 0
	inString := false
	escaped := false
	for index, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' && inString {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == ',' && !inString {
			parts = append(parts, value[start:index])
			start = index + 1
		}
	}
	parts = append(parts, value[start:])

	return parts
}

func renderSimpleTOMLMap(config map[string]any) ([]byte, error) {
	rootKeys := []string{}
	tableKeys := []string{}
	for key, value := range config {
		if _, ok := value.(map[string]any); ok {
			tableKeys = append(tableKeys, key)
			continue
		}
		rootKeys = append(rootKeys, key)
	}
	sort.Strings(rootKeys)
	sort.Strings(tableKeys)

	var builder strings.Builder
	for _, key := range rootKeys {
		value, err := renderSimpleTOMLValue(config[key])
		if err != nil {
			return nil, fmt.Errorf("render TOML key %q: %w", key, err)
		}
		builder.WriteString(key)
		builder.WriteString(" = ")
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
	for _, key := range tableKeys {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		table, ok := config[key].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("render TOML table %q: expected mapping", key)
		}
		if err := renderSimpleTOMLTable(&builder, key, table); err != nil {
			return nil, err
		}
	}

	return []byte(builder.String()), nil
}

func renderSimpleTOMLTable(builder *strings.Builder, tablePath string, table map[string]any) error {
	builder.WriteString("[")
	builder.WriteString(tablePath)
	builder.WriteString("]\n")

	keys := make([]string, 0, len(table))
	childTables := []string{}
	for key, value := range table {
		if _, ok := value.(map[string]any); ok {
			childTables = append(childTables, key)
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(childTables)
	for _, key := range keys {
		value, err := renderSimpleTOMLValue(table[key])
		if err != nil {
			return fmt.Errorf("render TOML key %q: %w", joinNativeConfigKey(tablePath, key), err)
		}
		builder.WriteString(key)
		builder.WriteString(" = ")
		builder.WriteString(value)
		builder.WriteByte('\n')
	}
	for _, key := range childTables {
		builder.WriteByte('\n')
		childTable, ok := table[key].(map[string]any)
		if !ok {
			return fmt.Errorf("render TOML table %q: expected mapping", joinNativeConfigKey(tablePath, key))
		}
		if err := renderSimpleTOMLTable(builder, joinNativeConfigKey(tablePath, key), childTable); err != nil {
			return err
		}
	}

	return nil
}

func renderSimpleTOMLValue(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed), nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			rendered, err := renderSimpleTOMLValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, rendered)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	default:
		return "", fmt.Errorf("unsupported value type %T", value)
	}
}

func stripJSON5Syntax(data []byte) []byte {
	withoutComments := stripJSON5Comments(string(data))
	return []byte(stripJSON5TrailingCommas(withoutComments))
}

func stripJSON5Comments(value string) string {
	var builder strings.Builder
	inString := false
	escaped := false
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if escaped {
			builder.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			builder.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			builder.WriteByte(ch)
			inString = !inString
			continue
		}
		if !inString && ch == '/' && index+1 < len(value) && value[index+1] == '/' {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			if index < len(value) {
				builder.WriteByte('\n')
			}
			continue
		}
		if !inString && ch == '/' && index+1 < len(value) && value[index+1] == '*' {
			index += 2
			for index+1 < len(value) && (value[index] != '*' || value[index+1] != '/') {
				index++
			}
			index++
			continue
		}
		builder.WriteByte(ch)
	}

	return builder.String()
}

func stripJSON5TrailingCommas(value string) string {
	var builder strings.Builder
	inString := false
	escaped := false
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if escaped {
			builder.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			builder.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			builder.WriteByte(ch)
			inString = !inString
			continue
		}
		if !inString && ch == ',' {
			next := index + 1
			for next < len(value) && (value[next] == ' ' || value[next] == '\n' || value[next] == '\r' || value[next] == '\t') {
				next++
			}
			if next < len(value) && (value[next] == '}' || value[next] == ']') {
				continue
			}
		}
		builder.WriteByte(ch)
	}

	return builder.String()
}

func normalizeJSONNumbers(config map[string]any) map[string]any {
	normalized := map[string]any{}
	for key, value := range config {
		normalized[key] = normalizeNativeJSONValue(value)
	}

	return normalized
}

func normalizeNativeJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeJSONNumbers(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, normalizeNativeJSONValue(item))
		}
		return values
	case float64:
		if typed == float64(int64(typed)) {
			return int(typed)
		}
		return typed
	default:
		return value
	}
}

func nativeConfigKeyPaths(config map[string]any) []string {
	paths := []string{}
	collectNativeConfigKeyPaths("", config, &paths)
	sort.Strings(paths)

	return paths
}

func collectNativeConfigKeyPaths(prefix string, config map[string]any, paths *[]string) {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		path := key
		if prefix != "" {
			path = joinNativeConfigKey(prefix, key)
		}
		if nested, ok := config[key].(map[string]any); ok {
			collectNativeConfigKeyPaths(path, nested, paths)
			continue
		}
		*paths = append(*paths, path)
	}
}

func nativeConfigValueAtPath(config map[string]any, keyPath string) (any, bool) {
	parts := strings.Split(keyPath, ".")
	var current any = config
	for _, part := range parts {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := mapping[part]
		if !ok {
			return nil, false
		}
		current = value
	}

	return current, true
}

func nativeConfigSetPath(config map[string]any, keyPath string, value any) {
	parts := strings.Split(keyPath, ".")
	current := config
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = cloneNativeConfigValue(value)
}

func nativeConfigDeletePath(config map[string]any, keyPath string) {
	parts := strings.Split(keyPath, ".")
	current := config
	parents := []map[string]any{config}
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			return
		}
		current = next
		parents = append(parents, current)
	}
	delete(current, parts[len(parts)-1])
	for index := len(parts) - 2; index >= 0; index-- {
		parent := parents[index]
		childKey := parts[index]
		child, ok := parent[childKey].(map[string]any)
		if !ok || len(child) > 0 {
			break
		}
		delete(parent, childKey)
	}
}

func cloneNativeConfigMap(input map[string]any) map[string]any {
	clone := map[string]any{}
	for key, value := range input {
		clone[key] = cloneNativeConfigValue(value)
	}

	return clone
}

func cloneNativeConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneNativeConfigMap(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, cloneNativeConfigValue(item))
		}
		return values
	default:
		return typed
	}
}

func joinNativeConfigKey(prefix string, key string) string {
	if prefix == "" {
		return key
	}

	return prefix + "." + key
}

func isFrameworkConfigOutput(output FrameworkActivationOutput) bool {
	return output.ArtifactType == "agent-config" || output.Mode == "merge-config" || output.Mode == "patch-global-config"
}
