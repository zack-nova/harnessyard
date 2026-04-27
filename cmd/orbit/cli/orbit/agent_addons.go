package orbit

import (
	"fmt"
	"sort"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ResolveAgentAddonHooks derives package-scoped hook add-ons from authored OrbitSpec truth.
func ResolveAgentAddonHooks(
	spec OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) ([]ResolvedAgentAddonHook, error) {
	if spec.AgentAddons == nil || spec.AgentAddons.Hooks == nil || len(spec.AgentAddons.Hooks.Entries) == 0 {
		return nil, nil
	}
	if err := validateOrbitAgentAddons(spec.AgentAddons); err != nil {
		return nil, err
	}

	packageName := spec.ID
	if spec.Package != nil && spec.Package.Name != "" {
		packageName = spec.Package.Name
	}
	if err := ids.ValidateOrbitID(packageName); err != nil {
		return nil, fmt.Errorf("validate package id: %w", err)
	}
	unsupportedBehavior := spec.AgentAddons.Hooks.UnsupportedBehavior

	trackedSet, err := normalizedPathSet(trackedFiles)
	if err != nil {
		return nil, fmt.Errorf("normalize tracked files: %w", err)
	}
	exportSet, err := normalizedPathSet(exportPaths)
	if err != nil {
		return nil, fmt.Errorf("normalize export paths: %w", err)
	}

	resolved := make([]ResolvedAgentAddonHook, 0, len(spec.AgentAddons.Hooks.Entries))
	for index, entry := range spec.AgentAddons.Hooks.Entries {
		handlerPath, err := ids.NormalizeRepoRelativePath(entry.Handler.Path)
		if err != nil {
			return nil, fmt.Errorf("agent_addons.hooks.entries[%d].handler.path: %w", index, err)
		}
		if _, ok := trackedSet[handlerPath]; !ok {
			return nil, fmt.Errorf("agent_addons.hooks.entries[%d].handler.path %q must reference one tracked package asset", index, handlerPath)
		}
		if _, ok := exportSet[handlerPath]; !ok {
			return nil, fmt.Errorf("agent_addons.hooks.entries[%d].handler.path %q must resolve inside the export surface", index, handlerPath)
		}

		resolved = append(resolved, ResolvedAgentAddonHook{
			Package:             packageName,
			ID:                  entry.ID,
			DisplayID:           packageName + ":" + entry.ID,
			Required:            entry.Required,
			Description:         entry.Description,
			EventKind:           entry.Event.Kind,
			Tools:               append([]string(nil), entry.Match.Tools...),
			CommandPatterns:     append([]string(nil), entry.Match.CommandPatterns...),
			HandlerType:         entry.Handler.Type,
			HandlerPath:         handlerPath,
			TimeoutSeconds:      entry.Handler.TimeoutSeconds,
			StatusMessage:       entry.Handler.StatusMessage,
			Targets:             cloneAgentAddonTargets(entry.Targets),
			UnsupportedBehavior: unsupportedBehavior,
		})
	}

	sort.Slice(resolved, func(left, right int) bool {
		return resolved[left].DisplayID < resolved[right].DisplayID
	})

	return resolved, nil
}

func cloneAgentAddonTargets(targets map[string]bool) map[string]bool {
	if len(targets) == 0 {
		return nil
	}
	cloned := make(map[string]bool, len(targets))
	for key, value := range targets {
		cloned[key] = value
	}

	return cloned
}
