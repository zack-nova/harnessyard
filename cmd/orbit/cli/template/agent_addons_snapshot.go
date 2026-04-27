package orbittemplate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// AgentAddonsSnapshot stores package-owned agent add-on provenance captured at install time.
type AgentAddonsSnapshot struct {
	Hooks []AgentAddonHookSnapshot `json:"hooks,omitempty" yaml:"hooks,omitempty"`
}

// AgentAddonHookSnapshot stores one package hook add-on and its rendered handler digest.
type AgentAddonHookSnapshot struct {
	OrbitID             string          `json:"orbit_id,omitempty" yaml:"orbit_id,omitempty"`
	Package             string          `json:"package" yaml:"package"`
	ID                  string          `json:"id" yaml:"id"`
	DisplayID           string          `json:"display_id" yaml:"display_id"`
	Required            bool            `json:"required,omitempty" yaml:"required,omitempty"`
	Description         string          `json:"description,omitempty" yaml:"description,omitempty"`
	EventKind           string          `json:"event_kind" yaml:"event_kind"`
	Tools               []string        `json:"tools,omitempty" yaml:"tools,omitempty"`
	CommandPatterns     []string        `json:"command_patterns,omitempty" yaml:"command_patterns,omitempty"`
	HandlerType         string          `json:"handler_type" yaml:"handler_type"`
	HandlerPath         string          `json:"handler_path" yaml:"handler_path"`
	HandlerDigest       string          `json:"handler_digest" yaml:"handler_digest"`
	TimeoutSeconds      int             `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	StatusMessage       string          `json:"status_message,omitempty" yaml:"status_message,omitempty"`
	Targets             map[string]bool `json:"targets,omitempty" yaml:"targets,omitempty"`
	UnsupportedBehavior string          `json:"unsupported_behavior,omitempty" yaml:"unsupported_behavior,omitempty"`
}

// BuildAgentAddonsSnapshot validates package add-ons against the package export surface
// and snapshots rendered handler content digests for install provenance.
func BuildAgentAddonsSnapshot(spec orbit.OrbitSpec, files []CandidateFile) (*AgentAddonsSnapshot, error) {
	if spec.AgentAddons == nil || spec.AgentAddons.Hooks == nil || len(spec.AgentAddons.Hooks.Entries) == 0 {
		return &AgentAddonsSnapshot{}, nil
	}

	definition, err := orbit.CompatibilityDefinitionFromOrbitSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("build compatibility definition for agent add-on snapshot: %w", err)
	}

	trackedFiles := make([]string, 0, len(files))
	fileContents := make(map[string][]byte, len(files))
	for _, file := range files {
		trackedFiles = append(trackedFiles, file.Path)
		fileContents[file.Path] = append([]byte(nil), file.Content...)
	}

	plan, err := orbit.ResolveProjectionPlan(orbit.RepositoryConfig{
		Global: orbit.DefaultGlobalConfig(),
		Orbits: []orbit.Definition{definition},
	}, spec, trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("resolve agent add-on projection plan: %w", err)
	}

	resolvedHooks, err := orbit.ResolveAgentAddonHooks(spec, trackedFiles, plan.ExportPaths)
	if err != nil {
		return nil, fmt.Errorf("resolve agent add-on hooks: %w", err)
	}
	if len(resolvedHooks) == 0 {
		return &AgentAddonsSnapshot{}, nil
	}

	snapshot := AgentAddonsSnapshot{
		Hooks: make([]AgentAddonHookSnapshot, 0, len(resolvedHooks)),
	}
	for _, hook := range resolvedHooks {
		content, ok := fileContents[hook.HandlerPath]
		if !ok {
			return nil, fmt.Errorf("agent add-on hook %q handler %q is missing from rendered package files", hook.DisplayID, hook.HandlerPath)
		}
		snapshot.Hooks = append(snapshot.Hooks, AgentAddonHookSnapshot{
			OrbitID:             spec.ID,
			Package:             hook.Package,
			ID:                  hook.ID,
			DisplayID:           hook.DisplayID,
			Required:            hook.Required,
			Description:         hook.Description,
			EventKind:           hook.EventKind,
			Tools:               append([]string(nil), hook.Tools...),
			CommandPatterns:     append([]string(nil), hook.CommandPatterns...),
			HandlerType:         hook.HandlerType,
			HandlerPath:         hook.HandlerPath,
			HandlerDigest:       installContentDigest(content),
			TimeoutSeconds:      hook.TimeoutSeconds,
			StatusMessage:       hook.StatusMessage,
			Targets:             cloneAgentAddonSnapshotTargets(hook.Targets),
			UnsupportedBehavior: hook.UnsupportedBehavior,
		})
	}
	sortAgentAddonHookSnapshots(snapshot.Hooks)
	if err := ValidateAgentAddonsSnapshot(snapshot); err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// ValidateAgentAddonsSnapshot validates install/bundle agent add-on provenance.
func ValidateAgentAddonsSnapshot(snapshot AgentAddonsSnapshot) error {
	seenHooks := make(map[string]struct{}, len(snapshot.Hooks))
	for index, hook := range snapshot.Hooks {
		prefix := fmt.Sprintf("hooks[%d]", index)
		if strings.TrimSpace(hook.OrbitID) != "" {
			if err := ids.ValidateOrbitID(hook.OrbitID); err != nil {
				return fmt.Errorf("%s.orbit_id: %w", prefix, err)
			}
		}
		if err := ids.ValidateOrbitID(hook.Package); err != nil {
			return fmt.Errorf("%s.package: %w", prefix, err)
		}
		if err := ids.ValidateOrbitID(hook.ID); err != nil {
			return fmt.Errorf("%s.id: %w", prefix, err)
		}
		if strings.TrimSpace(hook.DisplayID) == "" {
			return fmt.Errorf("%s.display_id must not be empty", prefix)
		}
		if _, ok := seenHooks[hook.DisplayID]; ok {
			return fmt.Errorf("%s.display_id %q is duplicated", prefix, hook.DisplayID)
		}
		seenHooks[hook.DisplayID] = struct{}{}
		if strings.TrimSpace(hook.EventKind) == "" {
			return fmt.Errorf("%s.event_kind must not be empty", prefix)
		}
		if strings.TrimSpace(hook.HandlerType) == "" {
			return fmt.Errorf("%s.handler_type must not be empty", prefix)
		}
		if strings.TrimSpace(hook.HandlerPath) == "" {
			return fmt.Errorf("%s.handler_path must not be empty", prefix)
		}
		if _, err := ids.NormalizeRepoRelativePath(hook.HandlerPath); err != nil {
			return fmt.Errorf("%s.handler_path: %w", prefix, err)
		}
		if strings.TrimSpace(hook.HandlerDigest) == "" {
			return fmt.Errorf("%s.handler_digest must not be empty", prefix)
		}
		switch hook.UnsupportedBehavior {
		case "", "skip", "block":
		default:
			return fmt.Errorf("%s.unsupported_behavior must be skip or block", prefix)
		}
		for target := range hook.Targets {
			if strings.TrimSpace(target) == "" {
				return fmt.Errorf("%s.targets must not contain empty ids", prefix)
			}
		}
	}

	return nil
}

// AgentAddonsSnapshotNode returns a stable YAML node for an agent add-ons snapshot.
func AgentAddonsSnapshotNode(snapshot AgentAddonsSnapshot) *yaml.Node {
	root := contractutil.MappingNode()
	hooksNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	hooks := append([]AgentAddonHookSnapshot(nil), snapshot.Hooks...)
	sortAgentAddonHookSnapshots(hooks)
	for _, hook := range hooks {
		hooksNode.Content = append(hooksNode.Content, agentAddonHookSnapshotNode(hook))
	}
	contractutil.AppendMapping(root, "hooks", hooksNode)

	return root
}

func agentAddonHookSnapshotNode(hook AgentAddonHookSnapshot) *yaml.Node {
	node := contractutil.MappingNode()
	if strings.TrimSpace(hook.OrbitID) != "" {
		contractutil.AppendMapping(node, "orbit_id", contractutil.StringNode(hook.OrbitID))
	}
	contractutil.AppendMapping(node, "package", contractutil.StringNode(hook.Package))
	contractutil.AppendMapping(node, "id", contractutil.StringNode(hook.ID))
	contractutil.AppendMapping(node, "display_id", contractutil.StringNode(hook.DisplayID))
	if hook.Required {
		contractutil.AppendMapping(node, "required", contractutil.BoolNode(hook.Required))
	}
	if strings.TrimSpace(hook.Description) != "" {
		contractutil.AppendMapping(node, "description", contractutil.StringNode(hook.Description))
	}
	contractutil.AppendMapping(node, "event_kind", contractutil.StringNode(hook.EventKind))
	appendStringSequenceMapping(node, "tools", hook.Tools)
	appendStringSequenceMapping(node, "command_patterns", hook.CommandPatterns)
	contractutil.AppendMapping(node, "handler_type", contractutil.StringNode(hook.HandlerType))
	contractutil.AppendMapping(node, "handler_path", contractutil.StringNode(hook.HandlerPath))
	contractutil.AppendMapping(node, "handler_digest", contractutil.StringNode(hook.HandlerDigest))
	if hook.TimeoutSeconds != 0 {
		contractutil.AppendMapping(node, "timeout_seconds", contractutil.IntNode(hook.TimeoutSeconds))
	}
	if strings.TrimSpace(hook.StatusMessage) != "" {
		contractutil.AppendMapping(node, "status_message", contractutil.StringNode(hook.StatusMessage))
	}
	if strings.TrimSpace(hook.UnsupportedBehavior) != "" {
		contractutil.AppendMapping(node, "unsupported_behavior", contractutil.StringNode(hook.UnsupportedBehavior))
	}
	if len(hook.Targets) > 0 {
		targetsNode := contractutil.MappingNode()
		targets := make([]string, 0, len(hook.Targets))
		for target := range hook.Targets {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			contractutil.AppendMapping(targetsNode, target, contractutil.BoolNode(hook.Targets[target]))
		}
		contractutil.AppendMapping(node, "targets", targetsNode)
	}

	return node
}

func appendStringSequenceMapping(node *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		return
	}
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		sequence.Content = append(sequence.Content, contractutil.StringNode(value))
	}
	contractutil.AppendMapping(node, key, sequence)
}

func sortAgentAddonHookSnapshots(hooks []AgentAddonHookSnapshot) {
	sort.Slice(hooks, func(left, right int) bool {
		return hooks[left].DisplayID < hooks[right].DisplayID
	})
}

func cloneAgentAddonSnapshotTargets(targets map[string]bool) map[string]bool {
	if len(targets) == 0 {
		return nil
	}
	cloned := make(map[string]bool, len(targets))
	for target, enabled := range targets {
		cloned[target] = enabled
	}

	return cloned
}

func installContentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
