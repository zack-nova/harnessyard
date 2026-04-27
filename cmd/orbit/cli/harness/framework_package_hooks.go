package harness

import (
	"sort"
	"strings"
)

type effectiveAgentHookEntry struct {
	Entry                 AgentHookEntry
	OrbitID               string
	Package               string
	AddonID               string
	DisplayID             string
	Required              bool
	Source                string
	HandlerDigest         string
	UnsupportedBehavior   string
	DefaultTimeoutSeconds int
	PackageHook           bool
}

func effectiveAgentHooksForFramework(summary FrameworkInspectSummary, frameworkID string) []effectiveAgentHookEntry {
	entries := []effectiveAgentHookEntry{}
	if hooks, ok := frameworkAgentHooksEnabled(summary, frameworkID); ok {
		entries = append(entries, effectiveRuntimeAgentHooksForFramework(hooks, frameworkID)...)
	}
	for _, hook := range summary.PackageAgentHooks {
		if !packageAgentHookTargetsFramework(hook, frameworkID) {
			continue
		}
		entries = append(entries, effectiveAgentHookEntry{
			Entry: AgentHookEntry{
				ID:          hook.DisplayID,
				Enabled:     true,
				Description: hook.Description,
				Event:       AgentHookEvent{Kind: hook.EventKind},
				Match: AgentHookMatch{
					Tools:           append([]string(nil), hook.Tools...),
					CommandPatterns: append([]string(nil), hook.CommandPatterns...),
				},
				Handler: AgentHookHandler{
					Type:           hook.HandlerType,
					Path:           hook.HandlerPath,
					TimeoutSeconds: hook.TimeoutSeconds,
					StatusMessage:  hook.StatusMessage,
				},
				Targets: cloneFrameworkPackageAgentHookTargets(hook.Targets),
			},
			OrbitID:               hook.OrbitID,
			Package:               hook.Package,
			AddonID:               hook.ID,
			DisplayID:             hook.DisplayID,
			Required:              hook.Required,
			Source:                hook.Source,
			HandlerDigest:         hook.HandlerDigest,
			UnsupportedBehavior:   normalizedPackageHookUnsupportedBehavior(hook.UnsupportedBehavior),
			DefaultTimeoutSeconds: defaultAgentHookTimeoutSeconds,
			PackageHook:           true,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].PackageHook != entries[right].PackageHook {
			return !entries[left].PackageHook
		}
		return entries[left].DisplayID < entries[right].DisplayID
	})

	return entries
}

func effectiveRuntimeAgentHooksForFramework(hooks AgentUnifiedHooks, frameworkID string) []effectiveAgentHookEntry {
	if !hooks.Enabled || len(hooks.Entries) == 0 {
		return nil
	}
	entries := []effectiveAgentHookEntry{}
	for _, entry := range hooks.Entries {
		if !entry.Enabled || !agentHookTargetsFramework(entry, frameworkID) {
			continue
		}
		entries = append(entries, effectiveAgentHookEntry{
			Entry:                 entry,
			DisplayID:             entry.ID,
			Source:                AgentUnifiedConfigRepoPath(),
			UnsupportedBehavior:   normalizedPackageHookUnsupportedBehavior(hooks.UnsupportedBehavior),
			DefaultTimeoutSeconds: hooks.Defaults.TimeoutSeconds,
		})
	}

	return entries
}

func supportedEffectiveAgentHookEntries(
	frameworkID string,
	entries []effectiveAgentHookEntry,
) ([]supportedAgentHookEntry, []effectiveAgentHookEntry) {
	supported := []supportedAgentHookEntry{}
	unsupported := []effectiveAgentHookEntry{}
	for _, entry := range entries {
		nativeEvent, ok := nativeHookEventName(frameworkID, entry.Entry.Event.Kind)
		if !ok {
			unsupported = append(unsupported, entry)
			continue
		}
		supported = append(supported, supportedAgentHookEntry{
			Entry:               entry.Entry,
			NativeEvent:         nativeEvent,
			OrbitID:             entry.OrbitID,
			Package:             entry.Package,
			AddonID:             entry.AddonID,
			DisplayID:           entry.DisplayID,
			Required:            entry.Required,
			Source:              entry.Source,
			HandlerDigest:       entry.HandlerDigest,
			UnsupportedBehavior: entry.UnsupportedBehavior,
			PackageHook:         entry.PackageHook,
		})
	}

	return supported, unsupported
}

func packageAgentHookTargetsFramework(hook FrameworkPackageAgentHookSummary, frameworkID string) bool {
	if len(hook.Targets) == 0 {
		return true
	}

	return hook.Targets[frameworkID]
}

func effectiveHookSourceFiles(entries []effectiveAgentHookEntry) []string {
	seen := map[string]struct{}{}
	sourceFiles := []string{}
	for _, entry := range entries {
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		sourceFiles = append(sourceFiles, source)
	}
	sort.Strings(sourceFiles)

	return sourceFiles
}

func packageHookApplyMode(frameworkID string, routeChoice FrameworkApplyRouteChoice) string {
	if frameworkID == "openclaw" {
		return "hybrid_hook_activation"
	}
	if routeChoice == FrameworkApplyRouteGlobal {
		return "global_hooks"
	}

	return "project_hooks"
}

func frameworkActivationPackageHooks(summary FrameworkInspectSummary, routeChoice FrameworkApplyRouteChoice) []FrameworkActivationPackageHook {
	frameworkID := summary.ResolvedFramework
	effectiveHooks := effectiveAgentHooksForFramework(summary, frameworkID)
	supported, _ := supportedEffectiveAgentHookEntries(frameworkID, effectiveHooks)
	hooks := []FrameworkActivationPackageHook{}
	for _, entry := range supported {
		if !entry.PackageHook {
			continue
		}
		hooks = append(hooks, FrameworkActivationPackageHook{
			OrbitID:        entry.OrbitID,
			Package:        entry.Package,
			AddonID:        entry.AddonID,
			DisplayID:      entry.DisplayID,
			Required:       entry.Required,
			Source:         entry.Source,
			EventKind:      entry.Entry.Event.Kind,
			NativeEvent:    entry.NativeEvent,
			HandlerPath:    entry.Entry.Handler.Path,
			HandlerDigest:  entry.HandlerDigest,
			HookApplyMode:  packageHookApplyMode(frameworkID, routeChoice),
			EffectiveScope: hookEffectiveScope(frameworkID),
		})
	}
	sort.Slice(hooks, func(left, right int) bool {
		return hooks[left].DisplayID < hooks[right].DisplayID
	})

	return hooks
}
