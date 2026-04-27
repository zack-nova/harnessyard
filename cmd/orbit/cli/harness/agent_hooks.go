package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	defaultAgentHookUnsupportedBehavior = "skip"
	defaultAgentHookRunner              = "hyard"
	defaultAgentHookTimeoutSeconds      = 30
)

// AgentUnifiedHooks stores runtime-level hook truth from .harness/agents/config.yaml.
type AgentUnifiedHooks struct {
	Enabled             bool              `json:"enabled"`
	UnsupportedBehavior string            `json:"unsupported_behavior,omitempty"`
	Defaults            AgentHookDefaults `json:"defaults,omitempty"`
	Entries             []AgentHookEntry  `json:"entries,omitempty"`
}

// AgentHookDefaults captures shared hook defaults.
type AgentHookDefaults struct {
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Runner         string `json:"runner,omitempty"`
}

// AgentHookEntry captures one unified hook intent.
type AgentHookEntry struct {
	ID          string           `json:"id"`
	Enabled     bool             `json:"enabled"`
	Description string           `json:"description,omitempty"`
	Event       AgentHookEvent   `json:"event"`
	Match       AgentHookMatch   `json:"match,omitempty"`
	Handler     AgentHookHandler `json:"handler"`
	Targets     map[string]bool  `json:"targets,omitempty"`
}

// AgentHookEvent captures the unified hook event.
type AgentHookEvent struct {
	Kind string `json:"kind"`
}

// AgentHookMatch captures framework-neutral match rules.
type AgentHookMatch struct {
	Tools           []string `json:"tools,omitempty"`
	CommandPatterns []string `json:"command_patterns,omitempty"`
}

// AgentHookHandler captures the project-local hook implementation.
type AgentHookHandler struct {
	Type           string `json:"type"`
	Path           string `json:"path"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	StatusMessage  string `json:"status_message,omitempty"`
}

// AgentHookRunInput captures one native-to-Harness hook bridge invocation.
type AgentHookRunInput struct {
	RepoRoot    string
	Target      string
	HookID      string
	NativeStdin []byte
}

// AgentHookRunResult captures native stdout/stderr/exit behavior after protocol translation.
type AgentHookRunResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type agentHookProtocolInput struct {
	SchemaVersion int            `json:"schema_version"`
	Target        string         `json:"target"`
	Hook          string         `json:"hook"`
	Event         AgentHookEvent `json:"event"`
	NativeEvent   string         `json:"native_event,omitempty"`
	Match         AgentHookMatch `json:"match,omitempty"`
	NativeInput   any            `json:"native_input,omitempty"`
}

type agentHookProtocolOutput struct {
	Decision string `json:"decision,omitempty"`
	Message  string `json:"message,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type agentHookRouteSet struct {
	ProjectOutputs        []FrameworkRoutePlanOutput
	HybridProjectOutputs  []FrameworkRoutePlanOutput
	HybridGlobalOutputs   []FrameworkRoutePlanOutput
	OptionalGlobalOutputs []FrameworkRoutePlanOutput
	UnsupportedOutputs    []FrameworkRoutePlanOutput
	HandlerPreview        []FrameworkRoutePlanOutput
}

func parseAgentUnifiedHooks(node *yaml.Node) (AgentUnifiedHooks, error) {
	if node.Kind != yaml.MappingNode {
		return AgentUnifiedHooks{}, fmt.Errorf("hooks must be a YAML mapping")
	}
	hooks := AgentUnifiedHooks{
		Enabled:             true,
		UnsupportedBehavior: defaultAgentHookUnsupportedBehavior,
		Defaults: AgentHookDefaults{
			TimeoutSeconds: defaultAgentHookTimeoutSeconds,
			Runner:         defaultAgentHookRunner,
		},
		Entries: []AgentHookEntry{},
	}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		switch keyNode.Value {
		case "enabled":
			enabled, err := yamlNodeBool(valueNode, "hooks.enabled")
			if err != nil {
				return AgentUnifiedHooks{}, err
			}
			hooks.Enabled = enabled
		case "unsupported_behavior":
			behavior, err := yamlNodeString(valueNode, "hooks.unsupported_behavior")
			if err != nil {
				return AgentUnifiedHooks{}, err
			}
			hooks.UnsupportedBehavior = strings.TrimSpace(behavior)
			if hooks.UnsupportedBehavior != "skip" && hooks.UnsupportedBehavior != "block" {
				return AgentUnifiedHooks{}, fmt.Errorf("hooks.unsupported_behavior must be skip or block")
			}
		case "defaults":
			defaults, err := parseAgentHookDefaults(valueNode)
			if err != nil {
				return AgentUnifiedHooks{}, err
			}
			hooks.Defaults = defaults
		case "entries":
			entries, err := parseAgentHookEntries(valueNode)
			if err != nil {
				return AgentUnifiedHooks{}, err
			}
			hooks.Entries = entries
		default:
			return AgentUnifiedHooks{}, fmt.Errorf("unknown field hooks.%s", keyNode.Value)
		}
	}
	if hooks.Defaults.TimeoutSeconds == 0 {
		hooks.Defaults.TimeoutSeconds = defaultAgentHookTimeoutSeconds
	}
	if strings.TrimSpace(hooks.Defaults.Runner) == "" {
		hooks.Defaults.Runner = defaultAgentHookRunner
	}

	return hooks, nil
}

func parseAgentHookDefaults(node *yaml.Node) (AgentHookDefaults, error) {
	if node.Kind != yaml.MappingNode {
		return AgentHookDefaults{}, fmt.Errorf("hooks.defaults must be a YAML mapping")
	}
	defaults := AgentHookDefaults{
		TimeoutSeconds: defaultAgentHookTimeoutSeconds,
		Runner:         defaultAgentHookRunner,
	}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		switch keyNode.Value {
		case "timeout_seconds":
			timeoutSeconds, err := yamlNodeInt(valueNode, "hooks.defaults.timeout_seconds")
			if err != nil {
				return AgentHookDefaults{}, err
			}
			if timeoutSeconds <= 0 {
				return AgentHookDefaults{}, fmt.Errorf("hooks.defaults.timeout_seconds must be positive")
			}
			defaults.TimeoutSeconds = timeoutSeconds
		case "runner":
			runner, err := yamlNodeString(valueNode, "hooks.defaults.runner")
			if err != nil {
				return AgentHookDefaults{}, err
			}
			defaults.Runner = strings.TrimSpace(runner)
			if defaults.Runner != defaultAgentHookRunner {
				return AgentHookDefaults{}, fmt.Errorf("hooks.defaults.runner must be %q", defaultAgentHookRunner)
			}
		default:
			return AgentHookDefaults{}, fmt.Errorf("unknown field hooks.defaults.%s", keyNode.Value)
		}
	}

	return defaults, nil
}

func parseAgentHookEntries(node *yaml.Node) ([]AgentHookEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("hooks.entries must be a YAML sequence")
	}
	entries := make([]AgentHookEntry, 0, len(node.Content))
	seen := map[string]struct{}{}
	for index, child := range node.Content {
		entry, err := parseAgentHookEntry(child, index)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[entry.ID]; ok {
			return nil, fmt.Errorf("hooks.entries[%d].id %q is duplicated", index, entry.ID)
		}
		seen[entry.ID] = struct{}{}
		entries = append(entries, entry)
	}

	return entries, nil
}

func parseAgentHookEntry(node *yaml.Node, index int) (AgentHookEntry, error) {
	if node.Kind != yaml.MappingNode {
		return AgentHookEntry{}, fmt.Errorf("hooks.entries[%d] must be a YAML mapping", index)
	}
	entry := AgentHookEntry{
		Enabled: true,
		Targets: map[string]bool{},
	}
	prefix := fmt.Sprintf("hooks.entries[%d]", index)
	for child := 0; child < len(node.Content); child += 2 {
		keyNode := node.Content[child]
		valueNode := node.Content[child+1]
		switch keyNode.Value {
		case "id":
			id, err := yamlNodeString(valueNode, prefix+".id")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.ID = strings.TrimSpace(id)
		case "enabled":
			enabled, err := yamlNodeBool(valueNode, prefix+".enabled")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Enabled = enabled
		case "description":
			description, err := yamlNodeString(valueNode, prefix+".description")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Description = strings.TrimSpace(description)
		case "event":
			event, err := parseAgentHookEvent(valueNode, prefix+".event")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Event = event
		case "match":
			match, err := parseAgentHookMatch(valueNode, prefix+".match")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Match = match
		case "handler":
			handler, err := parseAgentHookHandler(valueNode, prefix+".handler")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Handler = handler
		case "targets":
			targets, err := parseAgentHookTargets(valueNode, prefix+".targets")
			if err != nil {
				return AgentHookEntry{}, err
			}
			entry.Targets = targets
		default:
			return AgentHookEntry{}, fmt.Errorf("unknown field %s.%s", prefix, keyNode.Value)
		}
	}
	if entry.ID == "" {
		return AgentHookEntry{}, fmt.Errorf("%s.id must not be empty", prefix)
	}
	if err := ids.ValidateOrbitID(entry.ID); err != nil {
		return AgentHookEntry{}, fmt.Errorf("%s.id: %w", prefix, err)
	}
	if !isSupportedUnifiedHookEvent(entry.Event.Kind) {
		return AgentHookEntry{}, fmt.Errorf("%s.event.kind %q is not supported", prefix, entry.Event.Kind)
	}
	if entry.Handler.Type == "" {
		return AgentHookEntry{}, fmt.Errorf("%s.handler.type must not be empty", prefix)
	}
	if entry.Handler.Path == "" {
		return AgentHookEntry{}, fmt.Errorf("%s.handler.path must not be empty", prefix)
	}
	if err := validateAgentHookHandlerPath(entry.Handler.Path); err != nil {
		return AgentHookEntry{}, fmt.Errorf("%s.handler.path: %w", prefix, err)
	}

	return entry, nil
}

func parseAgentHookEvent(node *yaml.Node, path string) (AgentHookEvent, error) {
	if node.Kind != yaml.MappingNode {
		return AgentHookEvent{}, fmt.Errorf("%s must be a YAML mapping", path)
	}
	event := AgentHookEvent{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		switch keyNode.Value {
		case "kind":
			kind, err := yamlNodeString(valueNode, path+".kind")
			if err != nil {
				return AgentHookEvent{}, err
			}
			event.Kind = strings.TrimSpace(kind)
		default:
			return AgentHookEvent{}, fmt.Errorf("unknown field %s.%s", path, keyNode.Value)
		}
	}
	if event.Kind == "" {
		return AgentHookEvent{}, fmt.Errorf("%s.kind must not be empty", path)
	}

	return event, nil
}

func parseAgentHookMatch(node *yaml.Node, path string) (AgentHookMatch, error) {
	if node.Kind != yaml.MappingNode {
		return AgentHookMatch{}, fmt.Errorf("%s must be a YAML mapping", path)
	}
	match := AgentHookMatch{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		switch keyNode.Value {
		case "tools":
			values, err := yamlNodeStringSlice(valueNode, path+".tools")
			if err != nil {
				return AgentHookMatch{}, err
			}
			match.Tools = values
		case "command_patterns":
			values, err := yamlNodeStringSlice(valueNode, path+".command_patterns")
			if err != nil {
				return AgentHookMatch{}, err
			}
			match.CommandPatterns = values
		default:
			return AgentHookMatch{}, fmt.Errorf("unknown field %s.%s", path, keyNode.Value)
		}
	}

	return match, nil
}

func parseAgentHookHandler(node *yaml.Node, path string) (AgentHookHandler, error) {
	if node.Kind != yaml.MappingNode {
		return AgentHookHandler{}, fmt.Errorf("%s must be a YAML mapping", path)
	}
	handler := AgentHookHandler{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		switch keyNode.Value {
		case "type":
			handlerType, err := yamlNodeString(valueNode, path+".type")
			if err != nil {
				return AgentHookHandler{}, err
			}
			handler.Type = strings.TrimSpace(handlerType)
			if handler.Type != "command" {
				return AgentHookHandler{}, fmt.Errorf("%s.type must be command", path)
			}
		case "path":
			handlerPath, err := yamlNodeString(valueNode, path+".path")
			if err != nil {
				return AgentHookHandler{}, err
			}
			handler.Path = strings.TrimSpace(handlerPath)
		case "timeout_seconds":
			timeoutSeconds, err := yamlNodeInt(valueNode, path+".timeout_seconds")
			if err != nil {
				return AgentHookHandler{}, err
			}
			if timeoutSeconds <= 0 {
				return AgentHookHandler{}, fmt.Errorf("%s.timeout_seconds must be positive", path)
			}
			handler.TimeoutSeconds = timeoutSeconds
		case "status_message":
			statusMessage, err := yamlNodeString(valueNode, path+".status_message")
			if err != nil {
				return AgentHookHandler{}, err
			}
			handler.StatusMessage = strings.TrimSpace(statusMessage)
		default:
			return AgentHookHandler{}, fmt.Errorf("unknown field %s.%s", path, keyNode.Value)
		}
	}

	return handler, nil
}

func parseAgentHookTargets(node *yaml.Node, path string) (map[string]bool, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a YAML mapping", path)
	}
	targets := map[string]bool{}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		valueNode := node.Content[index+1]
		frameworkID, ok := normalizeAgentConfigTargetID(keyNode.Value)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not supported by this build", path, keyNode.Value)
		}
		enabled, err := yamlNodeBool(valueNode, path+"."+keyNode.Value)
		if err != nil {
			return nil, err
		}
		targets[frameworkID] = enabled
	}

	return targets, nil
}

func yamlNodeStringSlice(node *yaml.Node, path string) ([]string, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a YAML sequence", path)
	}
	values := make([]string, 0, len(node.Content))
	for index, child := range node.Content {
		value, err := yamlNodeString(child, fmt.Sprintf("%s[%d]", path, index))
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, fmt.Errorf("%s[%d] must not be empty", path, index)
		}
		values = append(values, trimmed)
	}

	return values, nil
}

func isSupportedUnifiedHookEvent(kind string) bool {
	switch kind {
	case "session.start",
		"prompt.before_submit",
		"tool.before",
		"permission.request",
		"tool.after",
		"turn.stop",
		"compact.before",
		"compact.after":
		return true
	default:
		return false
	}
}

func validateAgentHookHandlerPath(repoPath string) error {
	trimmed := strings.TrimSpace(repoPath)
	if trimmed == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.Contains(trimmed, "://") {
		return fmt.Errorf("remote handler URLs are not supported")
	}
	if filepath.IsAbs(trimmed) {
		return fmt.Errorf("must be repo-relative")
	}
	clean := filepath.ToSlash(filepath.Clean(trimmed))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("must not escape the repository")
	}

	return nil
}

func cloneAgentUnifiedHooks(input AgentUnifiedHooks) AgentUnifiedHooks {
	clone := input
	clone.Entries = make([]AgentHookEntry, 0, len(input.Entries))
	for _, entry := range input.Entries {
		entryClone := entry
		entryClone.Match.Tools = append([]string(nil), entry.Match.Tools...)
		entryClone.Match.CommandPatterns = append([]string(nil), entry.Match.CommandPatterns...)
		if entry.Targets != nil {
			entryClone.Targets = make(map[string]bool, len(entry.Targets))
			for key, value := range entry.Targets {
				entryClone.Targets[key] = value
			}
		}
		clone.Entries = append(clone.Entries, entryClone)
	}

	return clone
}

func frameworkAgentHooksEnabled(summary FrameworkInspectSummary, frameworkID string) (AgentUnifiedHooks, bool) {
	if summary.AgentConfig == nil || summary.AgentConfig.Hooks == nil {
		return AgentUnifiedHooks{}, false
	}
	if _, ok := frameworkAgentConfigEnabled(summary, frameworkID); !ok {
		return AgentUnifiedHooks{}, false
	}
	hooks := *summary.AgentConfig.Hooks
	if !hooks.Enabled || len(hooks.Entries) == 0 {
		return AgentUnifiedHooks{}, false
	}

	return hooks, true
}

func buildAgentHookRouteOutputs(frameworkID string, summary FrameworkInspectSummary) agentHookRouteSet {
	effectiveHooks := effectiveAgentHooksForFramework(summary, frameworkID)
	if len(effectiveHooks) == 0 {
		return agentHookRouteSet{}
	}
	supported, unsupported := supportedEffectiveAgentHookEntries(frameworkID, effectiveHooks)
	routeSet := agentHookRouteSet{}
	sourceFiles := effectiveHookSourceFiles(effectiveHooks)
	for _, entry := range supported {
		routeSet.HandlerPreview = append(routeSet.HandlerPreview, FrameworkRoutePlanOutput{
			OrbitID:        entry.OrbitID,
			Package:        entry.Package,
			AddonID:        entry.AddonID,
			Artifact:       hookDisplayID(entry),
			ArtifactType:   "hook-implementation",
			Route:          "execute_later",
			Recommended:    true,
			Required:       entry.Required,
			Source:         entry.Source,
			SourceFiles:    sourceFiles,
			Path:           entry.Entry.Handler.Path,
			Action:         "execute-later",
			Kind:           "hook_handler",
			Mode:           "execute-later",
			EffectiveScope: hookEffectiveScope(frameworkID),
			Invocation:     []string{entry.Entry.Handler.Path},
			HandlerDigest:  entry.HandlerDigest,
		})
	}
	for _, unsupportedEntry := range unsupported {
		routeSet.UnsupportedOutputs = append(routeSet.UnsupportedOutputs, FrameworkRoutePlanOutput{
			OrbitID:        unsupportedEntry.OrbitID,
			Package:        unsupportedEntry.Package,
			AddonID:        unsupportedEntry.AddonID,
			Artifact:       hookDisplayIDFromEffective(unsupportedEntry),
			ArtifactType:   "hook-config",
			Route:          "unsupported_event",
			Recommended:    false,
			Required:       unsupportedEntry.Required,
			Source:         unsupportedEntry.Source,
			SourceFiles:    sourceFiles,
			Kind:           "hook_config",
			Mode:           unsupportedPackageHookMode(unsupportedEntry),
			EffectiveScope: hookEffectiveScope(frameworkID),
			HandlerDigest:  unsupportedEntry.HandlerDigest,
		})
	}
	if len(supported) == 0 {
		sortAgentHookRouteSet(&routeSet)
		return routeSet
	}

	switch frameworkID {
	case "codex":
		routeSet.ProjectOutputs = append(routeSet.ProjectOutputs,
			agentHookRoute("codex-hooks", "project_hooks", ".codex/config.toml", "merge-config", "merge-config", "project", sourceFiles),
			agentHookRoute("codex-hooks", "project_hooks", ".codex/hooks.json", "generate", "generate", "project", sourceFiles),
		)
		routeSet.OptionalGlobalOutputs = append(routeSet.OptionalGlobalOutputs,
			agentHookRoute("codex-hooks", "global_hooks", "~/.codex/config.toml", "merge-config", "merge-config", "global", sourceFiles),
			agentHookRoute("codex-hooks", "global_hooks", "~/.codex/hooks.json", "generate", "generate", "global", sourceFiles),
		)
	case "claude":
		routeSet.ProjectOutputs = append(routeSet.ProjectOutputs,
			agentHookRoute("claude-hooks", "project_hooks", ".claude/settings.json", "merge-config", "merge-config", "project", sourceFiles),
		)
		routeSet.OptionalGlobalOutputs = append(routeSet.OptionalGlobalOutputs,
			agentHookRoute("claude-hooks", "global_hooks", "~/.claude/settings.json", "merge-config", "merge-config", "global", sourceFiles),
		)
	case "openclaw":
		for _, entry := range supported {
			fileID := nativeSafeHookID(entry.Entry.ID)
			routeSet.HybridProjectOutputs = append(routeSet.HybridProjectOutputs,
				agentHookImplementationRoute(entry.Entry.ID, "hooks/"+fileID+"/HOOK.md", sourceFiles),
				agentHookImplementationRoute(entry.Entry.ID, "hooks/"+fileID+"/handler.ts", sourceFiles),
			)
		}
		routeSet.HybridGlobalOutputs = append(routeSet.HybridGlobalOutputs,
			agentHookRoute("openclaw-hooks", "hybrid_hook_activation", "~/.openclaw/openclaw.json", "patch-global-config", "patch-global-config", "hybrid", sourceFiles),
		)
	}
	sortAgentHookRouteSet(&routeSet)

	return routeSet
}

func nativeSafeHookID(id string) string {
	return strings.NewReplacer(":", "__", "/", "__", "\\", "__").Replace(id)
}

func hookDisplayID(entry supportedAgentHookEntry) string {
	if entry.DisplayID != "" {
		return entry.DisplayID
	}

	return entry.Entry.ID
}

func hookDisplayIDFromEffective(entry effectiveAgentHookEntry) string {
	if entry.DisplayID != "" {
		return entry.DisplayID
	}

	return entry.Entry.ID
}

func unsupportedPackageHookMode(entry effectiveAgentHookEntry) string {
	if entry.Required {
		return "block"
	}

	return normalizedPackageHookUnsupportedBehavior(entry.UnsupportedBehavior)
}

func agentHookRoute(artifact string, route string, path string, action string, mode string, effectiveScope string, sourceFiles []string) FrameworkRoutePlanOutput {
	return FrameworkRoutePlanOutput{
		Artifact:       artifact,
		ArtifactType:   "hook-config",
		Route:          route,
		Recommended:    route != "global_hooks",
		Source:         AgentUnifiedConfigRepoPath(),
		SourceFiles:    append([]string(nil), sourceFiles...),
		Path:           path,
		Action:         action,
		Kind:           "hook_config",
		Mode:           mode,
		EffectiveScope: effectiveScope,
	}
}

func agentHookImplementationRoute(artifact string, path string, sourceFiles []string) FrameworkRoutePlanOutput {
	output := agentHookRoute(artifact, "hybrid_hook_activation", path, "generate", "generate", "project_workspace", sourceFiles)
	output.ArtifactType = "hook-implementation"
	output.Kind = "hook_implementation"

	return output
}

func sortAgentHookRouteSet(routeSet *agentHookRouteSet) {
	sortFrameworkRoutePlanOutputs(routeSet.ProjectOutputs)
	sortFrameworkRoutePlanOutputs(routeSet.HybridProjectOutputs)
	sortFrameworkRoutePlanOutputs(routeSet.HybridGlobalOutputs)
	sortFrameworkRoutePlanOutputs(routeSet.OptionalGlobalOutputs)
	sortFrameworkRoutePlanOutputs(routeSet.UnsupportedOutputs)
	sortFrameworkRoutePlanOutputs(routeSet.HandlerPreview)
}

type supportedAgentHookEntry struct {
	Entry               AgentHookEntry
	NativeEvent         string
	OrbitID             string
	Package             string
	AddonID             string
	DisplayID           string
	Required            bool
	Source              string
	HandlerDigest       string
	UnsupportedBehavior string
	PackageHook         bool
}

func agentHookTargetsFramework(entry AgentHookEntry, frameworkID string) bool {
	if len(entry.Targets) == 0 {
		return true
	}
	return entry.Targets[frameworkID]
}

func nativeHookEventName(frameworkID string, kind string) (string, bool) {
	switch frameworkID {
	case "codex":
		switch kind {
		case "session.start":
			return "SessionStart", true
		case "prompt.before_submit":
			return "UserPromptSubmit", true
		case "tool.before":
			return "PreToolUse", true
		case "permission.request":
			return "PermissionRequest", true
		case "tool.after":
			return "PostToolUse", true
		case "turn.stop":
			return "Stop", true
		default:
			return "", false
		}
	case "claude":
		switch kind {
		case "session.start":
			return "SessionStart", true
		case "prompt.before_submit":
			return "UserPromptSubmit", true
		case "tool.before":
			return "PreToolUse", true
		case "permission.request":
			return "PermissionRequest", true
		case "tool.after":
			return "PostToolUse", true
		case "turn.stop":
			return "Stop", true
		case "compact.before":
			return "PreCompact", true
		case "compact.after":
			return "PostCompact", true
		default:
			return "", false
		}
	case "openclaw":
		switch kind {
		case "compact.before":
			return "session:compact:before", true
		case "compact.after":
			return "session:compact:after", true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func hookEffectiveScope(frameworkID string) string {
	if frameworkID == "openclaw" {
		return "hybrid"
	}

	return "project"
}

func activationOutputFromAgentHookRoute(repoRoot string, gitDir string, homeDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, summary FrameworkInspectSummary) (FrameworkActivationOutput, error) {
	if routeOutput.Mode == "generate" {
		compiledPath, err := compileAgentNativeHookFile(repoRoot, gitDir, frameworkID, routeOutput, summary)
		if err != nil {
			return FrameworkActivationOutput{}, err
		}
		return frameworkActivationOutputFromHookRoute(repoRoot, homeDir, routeOutput, compiledPath, nil, nil), nil
	}

	compiledPath, generatedKeys, err := compileAgentNativeHookConfig(repoRoot, gitDir, frameworkID, routeOutput, summary)
	if err != nil {
		return FrameworkActivationOutput{}, err
	}
	backupPath := ""
	if strings.HasPrefix(routeOutput.Path, "~/") {
		backupPath = frameworkConfigBackupPath(gitDir, frameworkID, routeOutput)
	}
	output := frameworkActivationOutputFromHookRoute(repoRoot, homeDir, routeOutput, compiledPath, generatedKeys, generatedKeys)
	output.BackupPath = backupPath

	return output, nil
}

func frameworkActivationOutputFromHookRoute(repoRoot string, homeDir string, routeOutput FrameworkRoutePlanOutput, target string, generatedKeys []string, patchOwnedKeys []string) FrameworkActivationOutput {
	return FrameworkActivationOutput{
		Path:           routeOutput.Path,
		AbsolutePath:   frameworkRouteAbsolutePath(repoRoot, homeDir, routeOutput.Path),
		Kind:           routeOutput.Kind,
		Action:         routeOutput.Action,
		Target:         target,
		OrbitID:        routeOutput.OrbitID,
		Package:        routeOutput.Package,
		AddonID:        routeOutput.AddonID,
		Artifact:       routeOutput.Artifact,
		ArtifactType:   routeOutput.ArtifactType,
		Source:         routeOutput.Source,
		SourceFiles:    append([]string(nil), routeOutput.SourceFiles...),
		Route:          routeOutput.Route,
		Mode:           routeOutput.Mode,
		EffectiveScope: routeOutput.EffectiveScope,
		GeneratedKeys:  append([]string(nil), generatedKeys...),
		PatchOwnedKeys: append([]string(nil), patchOwnedKeys...),
		HandlerDigest:  routeOutput.HandlerDigest,
	}
}

func compileAgentNativeHookConfig(repoRoot string, gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, summary FrameworkInspectSummary) (string, []string, error) {
	effectiveHooks := effectiveAgentHooksForFramework(summary, frameworkID)
	supported, _ := supportedEffectiveAgentHookEntries(frameworkID, effectiveHooks)
	if len(supported) == 0 {
		return "", nil, fmt.Errorf("framework %q has no supported hooks to compile", frameworkID)
	}
	if err := validateSupportedAgentHookHandlers(repoRoot, supported); err != nil {
		return "", nil, err
	}
	var config map[string]any
	var format nativeConfigFormat
	switch {
	case frameworkID == "codex" && strings.HasSuffix(routeOutput.Path, "config.toml"):
		format = nativeConfigFormatTOML
		config = map[string]any{"features": map[string]any{"codex_hooks": true}}
	case frameworkID == "claude" && strings.HasSuffix(routeOutput.Path, "settings.json"):
		format = nativeConfigFormatJSON
		config = map[string]any{"hooks": buildClaudeNativeHooks(supported)}
	case frameworkID == "openclaw" && strings.HasSuffix(routeOutput.Path, "openclaw.json"):
		format = nativeConfigFormatJSON
		config = map[string]any{"hooks": map[string]any{"internal": buildOpenClawInternalHookConfig(supported)}}
	default:
		return "", nil, fmt.Errorf("unsupported hook config target %s for framework %q", routeOutput.Path, frameworkID)
	}
	generatedKeys := nativeConfigKeyPaths(config)
	data, err := renderNativeConfigMap(config, format)
	if err != nil {
		return "", nil, err
	}
	compiledPath := frameworkCompiledHookPath(gitDir, frameworkID, routeOutput, string(format))
	if err := contractutil.AtomicWriteFileMode(compiledPath, data, 0o600); err != nil {
		return "", nil, fmt.Errorf("write compiled native hook config: %w", err)
	}

	return compiledPath, generatedKeys, nil
}

func compileAgentNativeHookFile(repoRoot string, gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, summary FrameworkInspectSummary) (string, error) {
	effectiveHooks := effectiveAgentHooksForFramework(summary, frameworkID)
	supported, _ := supportedEffectiveAgentHookEntries(frameworkID, effectiveHooks)
	if err := validateSupportedAgentHookHandlers(repoRoot, supported); err != nil {
		return "", err
	}
	var data []byte
	var err error
	var extension string
	switch {
	case frameworkID == "codex" && strings.HasSuffix(routeOutput.Path, "hooks.json"):
		data, err = json.MarshalIndent(buildCodexNativeHooks(supported), "", "  ")
		extension = "json"
	case frameworkID == "openclaw" && strings.HasSuffix(routeOutput.Path, "HOOK.md"):
		entry, ok := findSupportedHookEntry(supported, routeOutput.Artifact)
		if !ok {
			return "", fmt.Errorf("hook %q is not supported by framework %q", routeOutput.Artifact, frameworkID)
		}
		data = []byte(renderOpenClawHookMarkdown(entry))
		extension = "md"
	case frameworkID == "openclaw" && strings.HasSuffix(routeOutput.Path, "handler.ts"):
		entry, ok := findSupportedHookEntry(supported, routeOutput.Artifact)
		if !ok {
			return "", fmt.Errorf("hook %q is not supported by framework %q", routeOutput.Artifact, frameworkID)
		}
		data = []byte(renderOpenClawHookHandler(entry))
		extension = "ts"
	default:
		return "", fmt.Errorf("unsupported hook file target %s for framework %q", routeOutput.Path, frameworkID)
	}
	if err != nil {
		return "", fmt.Errorf("encode Codex native hooks: %w", err)
	}
	if extension == "json" {
		data = append(data, '\n')
	}
	compiledPath := frameworkCompiledHookPath(gitDir, frameworkID, routeOutput, extension)
	if err := contractutil.AtomicWriteFileMode(compiledPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write compiled native hook file: %w", err)
	}

	return compiledPath, nil
}

func findSupportedHookEntry(entries []supportedAgentHookEntry, id string) (supportedAgentHookEntry, bool) {
	for _, entry := range entries {
		if entry.Entry.ID == id {
			return entry, true
		}
	}

	return supportedAgentHookEntry{}, false
}

func buildCodexNativeHooks(entries []supportedAgentHookEntry) map[string]any {
	hooks := map[string]any{}
	for _, entry := range entries {
		hooksForEvent, ok := hooks[entry.NativeEvent].([]any)
		if !ok {
			hooksForEvent = []any{}
		}
		hooksForEvent = append(hooksForEvent, map[string]any{
			"id":          entry.Entry.ID,
			"description": entry.Entry.Description,
			"command":     agentHookRunnerCommand("codex", entry.Entry.ID),
		})
		hooks[entry.NativeEvent] = hooksForEvent
	}

	return hooks
}

func buildClaudeNativeHooks(entries []supportedAgentHookEntry) map[string]any {
	hooks := map[string]any{}
	for _, entry := range entries {
		hooksForEvent, ok := hooks[entry.NativeEvent].([]any)
		if !ok {
			hooksForEvent = []any{}
		}
		hooksForEvent = append(hooksForEvent, map[string]any{
			"matcher": claudeHookMatcher(entry.Entry),
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": agentHookRunnerCommand("claude", entry.Entry.ID),
				},
			},
		})
		hooks[entry.NativeEvent] = hooksForEvent
	}

	return hooks
}

func buildOpenClawInternalHookConfig(entries []supportedAgentHookEntry) map[string]any {
	entryMap := map[string]any{}
	for _, entry := range entries {
		entryMap[entry.Entry.ID] = map[string]any{
			"events":      []any{entry.NativeEvent},
			"handler":     "hooks/" + nativeSafeHookID(entry.Entry.ID) + "/handler.ts",
			"description": entry.Entry.Description,
		}
	}

	return map[string]any{
		"enabled": true,
		"entries": entryMap,
	}
}

func claudeHookMatcher(entry AgentHookEntry) string {
	if len(entry.Match.Tools) == 0 {
		return ""
	}
	mapped := make([]string, 0, len(entry.Match.Tools))
	for _, tool := range entry.Match.Tools {
		switch strings.ToLower(tool) {
		case "shell", "bash":
			mapped = append(mapped, "Bash")
		default:
			mapped = append(mapped, tool)
		}
	}
	sort.Strings(mapped)

	return strings.Join(mapped, "|")
}

func renderOpenClawHookMarkdown(entry supportedAgentHookEntry) string {
	description := entry.Entry.Description
	if description == "" {
		description = "Harness-managed OpenClaw hook."
	}

	return fmt.Sprintf("# %s\n\n%s\n\nNative event: `%s`\n", entry.Entry.ID, description, entry.NativeEvent)
}

func renderOpenClawHookHandler(entry supportedAgentHookEntry) string {
	return fmt.Sprintf("import { spawnSync } from \"node:child_process\";\nimport { readFileSync } from \"node:fs\";\n\nconst input = readFileSync(0, \"utf8\");\nconst rootProbe = spawnSync(\"git\", [\"rev-parse\", \"--show-toplevel\"], { encoding: \"utf8\" });\nconst root = rootProbe.status === 0 ? rootProbe.stdout.trim() : process.cwd();\nconst result = spawnSync(\"hyard\", [\"hooks\", \"run\", \"--root\", root, \"--target\", \"openclaw\", \"--hook\", %q], { input, encoding: \"utf8\" });\nprocess.stdout.write(result.stdout ?? \"\");\nprocess.stderr.write(result.stderr ?? \"\");\nprocess.exit(result.status ?? 1);\n// hyard hooks run --root \"$(git rev-parse --show-toplevel)\" --target openclaw --hook %s\n", entry.Entry.ID, entry.Entry.ID)
}

func agentHookRunnerCommand(frameworkID string, hookID string) string {
	return fmt.Sprintf("hyard hooks run --root \"$(git rev-parse --show-toplevel)\" --target %s --hook %s", frameworkID, hookID)
}

func frameworkCompiledHookPath(gitDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, extension string) string {
	name := strings.NewReplacer("~", "home", "/", "_", "\\", "_").Replace(routeOutput.Path)
	name = strings.Trim(name, "_")
	if name == "" {
		name = routeOutput.Artifact
	}

	return filepath.Join(gitDir, "orbit", "state", "agents", "compiled", frameworkID, "hooks", name+"."+extension)
}

func ensureFrameworkGeneratedFileOutput(output FrameworkActivationOutput) (bool, error) {
	compiledData, err := os.ReadFile(output.Target)
	if err != nil {
		return false, fmt.Errorf("read compiled generated output for %s: %w", output.Path, err)
	}
	if err := os.MkdirAll(filepath.Dir(output.AbsolutePath), 0o750); err != nil {
		return false, fmt.Errorf("create parent directory for generated output %s: %w", output.Path, err)
	}
	existingData, err := os.ReadFile(output.AbsolutePath)
	if err == nil {
		if bytes.Equal(existingData, compiledData) {
			return false, nil
		}
		return false, fmt.Errorf("framework output %s already exists but has different generated content", output.Path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read generated output %s: %w", output.Path, err)
	}
	if err := contractutil.AtomicWriteFileMode(output.AbsolutePath, compiledData, 0o644); err != nil {
		return false, fmt.Errorf("write generated output %s: %w", output.Path, err)
	}

	return true, nil
}

func frameworkGeneratedFileOutputOwned(output FrameworkActivationOutput) (bool, error) {
	compiledData, err := os.ReadFile(output.Target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read compiled generated output for %s: %w", output.Path, err)
	}
	existingData, err := os.ReadFile(output.AbsolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read generated output %s: %w", output.Path, err)
	}

	return bytes.Equal(existingData, compiledData), nil
}

func removeFrameworkGeneratedFileOutput(output FrameworkActivationOutput) (bool, error) {
	owned, err := frameworkGeneratedFileOutputOwned(output)
	if err != nil {
		return false, err
	}
	if !owned {
		return false, nil
	}
	if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove generated output %s: %w", output.Path, err)
	}

	return true, nil
}

func isFrameworkGeneratedFileOutput(output FrameworkActivationOutput) bool {
	return output.Mode == "generate"
}

// RunAgentHook translates native hook input to the stable Harness hook protocol and executes the configured handler.
func RunAgentHook(ctx context.Context, input AgentHookRunInput) (AgentHookRunResult, error) {
	repoRoot := filepath.Clean(input.RepoRoot)
	if repoRoot == "" || repoRoot == "." {
		return AgentHookRunResult{}, fmt.Errorf("repo root must not be empty")
	}
	frameworkID, ok := normalizeAgentConfigTargetID(input.Target)
	if !ok {
		return AgentHookRunResult{}, fmt.Errorf("target %q is not supported by this build", input.Target)
	}
	effectiveHooks, err := effectiveAgentHooksForRun(ctx, repoRoot, frameworkID)
	if err != nil {
		return AgentHookRunResult{}, err
	}
	if len(effectiveHooks) == 0 {
		return AgentHookRunResult{}, fmt.Errorf("hooks are not enabled")
	}
	entry, nativeEvent, err := findRunnableEffectiveAgentHook(effectiveHooks, frameworkID, input.HookID)
	if err != nil {
		return AgentHookRunResult{}, err
	}
	nativeInput, err := parseNativeHookInput(input.NativeStdin)
	if err != nil {
		return AgentHookRunResult{}, err
	}
	if !agentHookEntryMatches(entry.Entry, nativeInput) {
		stdout, marshalErr := json.MarshalIndent(agentHookProtocolOutput{Decision: "skipped", Reason: "match_not_satisfied"}, "", "  ")
		if marshalErr != nil {
			return AgentHookRunResult{}, fmt.Errorf("encode skipped hook protocol output: %w", marshalErr)
		}
		return AgentHookRunResult{Stdout: append(stdout, '\n'), ExitCode: 0}, nil
	}
	if err := validateAgentHookHandlerExecutable(repoRoot, entry.Entry.Handler.Path); err != nil {
		return AgentHookRunResult{}, err
	}
	timeoutSeconds := entry.Entry.Handler.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = entry.DefaultTimeoutSeconds
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultAgentHookTimeoutSeconds
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	protocolInput := agentHookProtocolInput{
		SchemaVersion: 1,
		Target:        frameworkID,
		Hook:          entry.Entry.ID,
		Event:         entry.Entry.Event,
		NativeEvent:   nativeEvent,
		Match:         entry.Entry.Match,
		NativeInput:   nativeInput,
	}
	protocolData, err := json.MarshalIndent(protocolInput, "", "  ")
	if err != nil {
		return AgentHookRunResult{}, fmt.Errorf("encode hook protocol input: %w", err)
	}
	command := exec.CommandContext(runCtx, filepath.Join(repoRoot, filepath.FromSlash(entry.Entry.Handler.Path))) //nolint:gosec // Handler path is validated repo-relative versioned truth.
	command.Dir = repoRoot
	command.Stdin = bytes.NewReader(append(protocolData, '\n'))
	command.Env = append(os.Environ(),
		"HARNESS_HOOK_ID="+entry.Entry.ID,
		"HARNESS_HOOK_TARGET="+frameworkID,
		"HARNESS_HOOK_EVENT="+entry.Entry.Event.Kind,
	)
	stdout, stderr := bytes.Buffer{}, bytes.Buffer{}
	command.Stdout = &stdout
	command.Stderr = &stderr
	runErr := command.Run()
	if runCtx.Err() != nil {
		return AgentHookRunResult{Stderr: []byte("hook handler timed out\n"), ExitCode: 124}, nil
	}
	if runErr != nil {
		exitCode := 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return AgentHookRunResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: exitCode}, nil
	}

	return translateAgentHookProtocolOutput(stdout.Bytes(), stderr.Bytes())
}

func effectiveAgentHooksForRun(ctx context.Context, repoRoot string, frameworkID string) ([]effectiveAgentHookEntry, error) {
	repo, err := gitpkg.DiscoverRepo(ctx, repoRoot)
	if err == nil {
		state, stateErr := loadFrameworkDesiredState(ctx, repo.Root, repo.GitDir)
		if stateErr == nil {
			return effectiveAgentHooksForFramework(state.Summary, frameworkID), nil
		}
		configFile, configErr := LoadAgentUnifiedConfigFile(repo.Root)
		if configErr != nil {
			return nil, stateErr
		}

		return effectiveRuntimeAgentHooksForFramework(configFile.Hooks, frameworkID), nil
	}
	if !gitpkg.IsNotGitRepositoryError(err) {
		return nil, fmt.Errorf("discover git repo for hook run: %w", err)
	}
	configFile, configErr := LoadAgentUnifiedConfigFile(repoRoot)
	if configErr != nil {
		return nil, configErr
	}

	return effectiveRuntimeAgentHooksForFramework(configFile.Hooks, frameworkID), nil
}

func findRunnableEffectiveAgentHook(entries []effectiveAgentHookEntry, frameworkID string, hookID string) (effectiveAgentHookEntry, string, error) {
	for _, entry := range entries {
		if entry.Entry.ID != hookID {
			continue
		}
		if !entry.Entry.Enabled {
			return effectiveAgentHookEntry{}, "", fmt.Errorf("hook %q is disabled", hookID)
		}
		if !agentHookTargetsFramework(entry.Entry, frameworkID) {
			return effectiveAgentHookEntry{}, "", fmt.Errorf("hook %q does not target %s", hookID, frameworkID)
		}
		nativeEvent, ok := nativeHookEventName(frameworkID, entry.Entry.Event.Kind)
		if !ok {
			return effectiveAgentHookEntry{}, "", fmt.Errorf("hook %q event %q is not supported by %s", hookID, entry.Entry.Event.Kind, frameworkID)
		}

		return entry, nativeEvent, nil
	}

	return effectiveAgentHookEntry{}, "", fmt.Errorf("hook %q was not found", hookID)
}

func parseNativeHookInput(data []byte) (any, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return map[string]any{}, nil
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, fmt.Errorf("decode native hook input: %w", err)
	}

	return normalizeNativeJSONValue(value), nil
}

func agentHookEntryMatches(entry AgentHookEntry, nativeInput any) bool {
	if len(entry.Match.Tools) > 0 {
		toolValues := collectNativeHookStrings(nativeInput, map[string]struct{}{
			"tool":      {},
			"tool_name": {},
			"toolName":  {},
			"name":      {},
		})
		if !stringSetIntersectsFold(entry.Match.Tools, toolValues) {
			return false
		}
	}
	if len(entry.Match.CommandPatterns) > 0 {
		commandValues := collectNativeHookStrings(nativeInput, map[string]struct{}{
			"command": {},
			"cmd":     {},
		})
		if !commandPatternsMatch(entry.Match.CommandPatterns, commandValues) {
			return false
		}
	}

	return true
}

func collectNativeHookStrings(value any, names map[string]struct{}) []string {
	values := []string{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, ok := names[key]; ok {
					if text, textOK := child.(string); textOK {
						values = append(values, text)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)

	return values
}

func stringSetIntersectsFold(expected []string, actual []string) bool {
	for _, left := range expected {
		for _, right := range actual {
			if strings.EqualFold(left, right) {
				return true
			}
		}
	}

	return false
}

func commandPatternsMatch(patterns []string, commands []string) bool {
	for _, pattern := range patterns {
		for _, command := range commands {
			if wildcardMatch(pattern, command) {
				return true
			}
		}
	}

	return false
}

func wildcardMatch(pattern string, value string) bool {
	matched, err := filepath.Match(pattern, value)
	if err == nil && matched {
		return true
	}
	quoted := strings.Trim(pattern, "*")
	if quoted != "" && strings.Contains(value, quoted) {
		return true
	}

	return false
}

func validateAgentHookHandlerExecutable(repoRoot string, repoPath string) error {
	if err := validateAgentHookHandlerPath(repoPath); err != nil {
		return err
	}
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("stat hook handler %s: %w", repoPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("hook handler %s is a directory", repoPath)
	}
	if info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("hook handler %s is not executable", repoPath)
	}

	return nil
}

func validateSupportedAgentHookHandlers(repoRoot string, entries []supportedAgentHookEntry) error {
	for _, entry := range entries {
		if err := validateAgentHookHandlerExecutable(repoRoot, entry.Entry.Handler.Path); err != nil {
			return fmt.Errorf("hook %q handler: %w", entry.Entry.ID, err)
		}
	}

	return nil
}

func translateAgentHookProtocolOutput(stdout []byte, stderr []byte) (AgentHookRunResult, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return AgentHookRunResult{Stderr: stderr, ExitCode: 0}, nil
	}
	var output agentHookProtocolOutput
	if err := json.Unmarshal(trimmed, &output); err != nil {
		return AgentHookRunResult{}, fmt.Errorf("decode hook protocol output: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(output.Decision)) {
	case "", "allow":
		return AgentHookRunResult{Stdout: append(trimmed, '\n'), Stderr: stderr, ExitCode: 0}, nil
	case "skip", "skipped":
		return AgentHookRunResult{Stdout: append(trimmed, '\n'), Stderr: stderr, ExitCode: 0}, nil
	case "block", "deny":
		message := strings.TrimSpace(output.Message)
		if message == "" {
			message = strings.TrimSpace(output.Reason)
		}
		if message == "" {
			message = "hook blocked the native action"
		}
		return AgentHookRunResult{Stderr: append([]byte(message), '\n'), ExitCode: 2}, nil
	default:
		return AgentHookRunResult{}, fmt.Errorf("unsupported hook protocol decision %q", output.Decision)
	}
}
