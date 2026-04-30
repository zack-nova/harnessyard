package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// AgentConfigClearOptions controls clearing versioned agent config truth.
type AgentConfigClearOptions struct {
	RepoRoot       string
	Target         string
	RemoveSidecars bool
	All            bool
}

// AgentConfigClearResult reports one agent config clear mutation.
type AgentConfigClearResult struct {
	ConfigPath      string   `json:"config_path"`
	ClearedTargets  []string `json:"cleared_targets,omitempty"`
	RemovedSidecars []string `json:"removed_sidecars,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ClearAgentConfig removes selected agent config truth while preserving sidecars unless requested.
func ClearAgentConfig(options AgentConfigClearOptions) (AgentConfigClearResult, error) {
	if strings.TrimSpace(options.RepoRoot) == "" {
		return AgentConfigClearResult{}, fmt.Errorf("repo root must not be empty")
	}
	if options.All && strings.TrimSpace(options.Target) != "" {
		return AgentConfigClearResult{}, fmt.Errorf("--all cannot be combined with --target")
	}

	configPath := AgentUnifiedConfigPath(options.RepoRoot)
	data, err := os.ReadFile(configPath) //nolint:gosec // Path is repo-local and built from the fixed config truth path.
	if err != nil {
		return AgentConfigClearResult{}, fmt.Errorf("read %s: %w", configPath, err)
	}
	configFile, err := ParseAgentUnifiedConfigFileData(data)
	if err != nil {
		return AgentConfigClearResult{}, fmt.Errorf("validate %s: %w", configPath, err)
	}

	targets, err := agentConfigClearTargets(configFile, options)
	if err != nil {
		return AgentConfigClearResult{}, err
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return AgentConfigClearResult{}, fmt.Errorf("decode %s: %w", configPath, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return AgentConfigClearResult{}, fmt.Errorf("agent config file must be a YAML mapping")
	}
	root := document.Content[0]
	targetSet := agentConfigClearStringSet(targets)
	removeAgentConfigTargets(root, targetSet)
	if options.All {
		removeMappingKey(root, "config")
		removeMappingKey(root, "hooks")
	}
	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return AgentConfigClearResult{}, fmt.Errorf("encode %s: %w", configPath, err)
	}
	if err := contractutil.AtomicWriteFile(configPath, encoded); err != nil {
		return AgentConfigClearResult{}, fmt.Errorf("write %s: %w", configPath, err)
	}

	result := AgentConfigClearResult{
		ConfigPath:     configPath,
		ClearedTargets: targets,
	}
	for _, target := range targets {
		if configFile.Targets[target].Scope == "global" || configFile.Targets[target].Scope == "hybrid" {
			result.Warnings = append(result.Warnings, "clearing global agent config target "+target)
		}
	}
	if options.RemoveSidecars || options.All {
		removed, err := removeAgentConfigSidecars(options.RepoRoot, targets)
		if err != nil {
			return AgentConfigClearResult{}, err
		}
		result.RemovedSidecars = removed
	}
	sort.Strings(result.Warnings)

	return result, nil
}

func agentConfigClearTargets(configFile AgentUnifiedConfigFile, options AgentConfigClearOptions) ([]string, error) {
	if options.All || strings.TrimSpace(options.Target) == "" {
		targets := make([]string, 0, len(configFile.Targets))
		for target := range configFile.Targets {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		return targets, nil
	}
	target, ok := NormalizeFrameworkID(options.Target)
	if !ok {
		return nil, fmt.Errorf("agent config target %q is not supported by this build", options.Target)
	}
	if _, ok := configFile.Targets[target]; !ok {
		return nil, fmt.Errorf("agent config target %q is not present", target)
	}

	return []string{target}, nil
}

func removeAgentConfigTargets(root *yaml.Node, targets map[string]struct{}) {
	targetsNode := mappingValue(root, "targets")
	if targetsNode == nil || targetsNode.Kind != yaml.MappingNode {
		return
	}
	filtered := targetsNode.Content[:0]
	for index := 0; index < len(targetsNode.Content); index += 2 {
		keyNode := targetsNode.Content[index]
		valueNode := targetsNode.Content[index+1]
		target, ok := normalizeAgentConfigTargetID(keyNode.Value)
		if ok {
			if _, remove := targets[target]; remove {
				continue
			}
		}
		filtered = append(filtered, keyNode, valueNode)
	}
	targetsNode.Content = filtered
	if len(targetsNode.Content) == 0 {
		removeMappingKey(root, "targets")
	}
}

func removeAgentConfigSidecars(repoRoot string, targets []string) ([]string, error) {
	removed := []string{}
	for _, target := range targets {
		repoPath, ok := agentConfigSidecarRepoPath(target)
		if !ok {
			continue
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if err := os.Remove(filename); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("remove agent config sidecar %s: %w", repoPath, err)
		}
		removed = append(removed, repoPath)
	}
	sort.Strings(removed)

	return removed, nil
}

func mappingValue(root *yaml.Node, key string) *yaml.Node {
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value == key {
			return root.Content[index+1]
		}
	}

	return nil
}

func removeMappingKey(root *yaml.Node, key string) {
	filtered := root.Content[:0]
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value == key {
			continue
		}
		filtered = append(filtered, root.Content[index], root.Content[index+1])
	}
	root.Content = filtered
}

func agentConfigClearStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}

	return set
}
