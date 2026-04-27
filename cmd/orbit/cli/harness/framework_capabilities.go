package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func resolveFrameworkOrbitCapabilities(
	repoRoot string,
	orbitID string,
	spec orbitpkg.OrbitSpec,
	trackedFiles []string,
	exportPaths []string,
) ([]FrameworkCommandSummary, []FrameworkSkillSummary, []FrameworkRemoteSkillSummary, []FrameworkPackageAgentHookSummary, []FrameworkCheckFinding) {
	findings := []FrameworkCheckFinding{}

	commands, err := orbitpkg.ResolveCommandCapabilities(spec, trackedFiles, exportPaths)
	if err != nil {
		findings = append(findings, classifyFrameworkCommandFinding(orbitID, err))
	}

	localSkills, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, exportPaths)
	if err != nil {
		findings = append(findings, classifyFrameworkSkillFinding(orbitID, err))
	}

	remoteSkills, err := orbitpkg.ResolveRemoteSkillCapabilities(spec)
	if err != nil {
		findings = append(findings, classifyFrameworkRemoteSkillFinding(orbitID, err))
	}
	agentHooks, err := orbitpkg.ResolveAgentAddonHooks(spec, trackedFiles, exportPaths)
	if err != nil {
		findings = append(findings, classifyFrameworkAgentAddonHookFinding(orbitID, err))
	}

	resolvedCommands := make([]FrameworkCommandSummary, 0, len(commands))
	for _, command := range commands {
		resolvedCommands = append(resolvedCommands, FrameworkCommandSummary{
			OrbitID: orbitID,
			ID:      command.Name,
			Path:    command.Path,
		})
	}

	resolvedSkills := make([]FrameworkSkillSummary, 0, len(localSkills))
	for _, skill := range localSkills {
		resolvedSkills = append(resolvedSkills, FrameworkSkillSummary{
			OrbitID: orbitID,
			ID:      skill.Name,
			Path:    skill.RootPath,
		})
	}

	resolvedRemoteSkills := make([]FrameworkRemoteSkillSummary, 0, len(remoteSkills))
	for _, skill := range remoteSkills {
		resolvedRemoteSkills = append(resolvedRemoteSkills, FrameworkRemoteSkillSummary{
			OrbitID:  orbitID,
			URI:      skill.URI,
			Required: skill.Required,
		})
	}

	resolvedAgentHooks := make([]FrameworkPackageAgentHookSummary, 0, len(agentHooks))
	for _, hook := range agentHooks {
		handlerDigest, err := frameworkPackageHookHandlerDigest(repoRoot, hook.HandlerPath)
		if err != nil {
			findings = append(findings, FrameworkCheckFinding{
				Kind:    "package_hook_handler_unreadable",
				OrbitID: orbitID,
				Path:    hook.HandlerPath,
				Message: err.Error(),
			})
		}
		resolvedAgentHooks = append(resolvedAgentHooks, FrameworkPackageAgentHookSummary{
			OrbitID:             orbitID,
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
			HandlerDigest:       handlerDigest,
			TimeoutSeconds:      hook.TimeoutSeconds,
			StatusMessage:       hook.StatusMessage,
			Targets:             cloneFrameworkPackageAgentHookTargets(hook.Targets),
			UnsupportedBehavior: hook.UnsupportedBehavior,
			Source:              frameworkPackageHookSource(spec, orbitID),
			Activation:          "not_applied",
		})
	}

	return resolvedCommands, resolvedSkills, resolvedRemoteSkills, resolvedAgentHooks, findings
}

func frameworkPackageHookHandlerDigest(repoRoot string, repoPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(repoPath))) //nolint:gosec // Handler paths are repo-relative Orbit add-on truth validated before resolution.
	if err != nil {
		return "", fmt.Errorf("read package hook handler %s: %w", repoPath, err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func frameworkPackageHookSource(spec orbitpkg.OrbitSpec, orbitID string) string {
	if spec.Meta != nil && strings.TrimSpace(spec.Meta.File) != "" {
		return spec.Meta.File
	}

	return ".harness/orbits/" + orbitID + ".yaml"
}

func appendFrameworkCapabilityFindings(
	summary FrameworkInspectSummary,
	findings []FrameworkCheckFinding,
) []FrameworkCheckFinding {
	if summary.ResolvedFramework == "" {
		return sortFrameworkCheckFindings(findings)
	}

	adapter, ok := LookupFrameworkAdapter(summary.ResolvedFramework)
	if !ok {
		return sortFrameworkCheckFindings(findings)
	}

	if adapter.CommandsGlobal {
		findings = append(findings, buildFrameworkNameCollisionFindings(
			"command_name_collision",
			summary.Commands,
			func(command FrameworkCommandSummary) string { return command.ID },
			func(command FrameworkCommandSummary) string { return command.OrbitID },
			func(name string, orbitIDs []string) string {
				return fmt.Sprintf(
					"resolved command name %q is declared by multiple orbits: %s",
					name,
					joinQuotedOrbitIDs(orbitIDs),
				)
			},
		)...)
	}
	if adapter.SkillsGlobal {
		findings = append(findings, buildFrameworkNameCollisionFindings(
			"skill_name_collision",
			summary.Skills,
			func(skill FrameworkSkillSummary) string { return skill.ID },
			func(skill FrameworkSkillSummary) string { return skill.OrbitID },
			func(name string, orbitIDs []string) string {
				return fmt.Sprintf(
					"resolved local skill name %q is declared by multiple orbits: %s",
					name,
					joinQuotedOrbitIDs(orbitIDs),
				)
			},
		)...)
	}
	if len(summary.RemoteSkills) > 0 && !adapter.RemoteSkillsSupported {
		for _, remoteSkill := range summary.RemoteSkills {
			findings = append(findings, FrameworkCheckFinding{
				Kind:     "skill_remote_uri_unsupported",
				OrbitID:  remoteSkill.OrbitID,
				Path:     remoteSkill.URI,
				Blocking: remoteSkill.Required,
				Message: fmt.Sprintf(
					"framework %q does not support remote skill URI %q",
					adapter.ID,
					remoteSkill.URI,
				),
			})
		}
	}

	return sortFrameworkCheckFindings(findings)
}

func buildFrameworkNameCollisionFindings[T any](
	kind string,
	items []T,
	nameOf func(T) string,
	orbitOf func(T) string,
	message func(name string, orbitIDs []string) string,
) []FrameworkCheckFinding {
	nameToOrbits := make(map[string]map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(nameOf(item))
		if name == "" {
			continue
		}
		if _, ok := nameToOrbits[name]; !ok {
			nameToOrbits[name] = map[string]struct{}{}
		}
		nameToOrbits[name][orbitOf(item)] = struct{}{}
	}

	findings := []FrameworkCheckFinding{}
	for name, orbitSet := range nameToOrbits {
		if len(orbitSet) < 2 {
			continue
		}
		orbitIDs := make([]string, 0, len(orbitSet))
		for orbitID := range orbitSet {
			orbitIDs = append(orbitIDs, orbitID)
		}
		sort.Strings(orbitIDs)
		findings = append(findings, FrameworkCheckFinding{
			Kind:    kind,
			Path:    name,
			Message: message(name, orbitIDs),
		})
	}

	return findings
}

func classifyFrameworkCommandFinding(orbitID string, err error) FrameworkCheckFinding {
	message := err.Error()
	if strings.Contains(message, `resolved command name "`) && strings.Contains(message, "declared by multiple files") {
		return FrameworkCheckFinding{
			Kind:    "command_name_collision",
			OrbitID: orbitID,
			Message: message,
		}
	}

	return FrameworkCheckFinding{
		Kind:    "command_invalid",
		OrbitID: orbitID,
		Message: message,
	}
}

func classifyFrameworkSkillFinding(orbitID string, err error) FrameworkCheckFinding {
	message := err.Error()
	rootPath := quotedValueAfter(message, `local skill root "`)
	skillMDPath := rootPath
	if skillMDPath != "" {
		skillMDPath += "/SKILL.md"
	}

	switch {
	case strings.Contains(message, "SKILL.md must exist and be tracked"):
		return FrameworkCheckFinding{
			Kind:    "skill_missing_skill_md",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: fmt.Sprintf("local skill root %q must include one tracked SKILL.md", rootPath),
		}
	case strings.Contains(message, "must define non-empty name"):
		return FrameworkCheckFinding{
			Kind:    "skill_missing_name",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: fmt.Sprintf("local skill root %q must define a non-empty name in SKILL.md frontmatter", rootPath),
		}
	case strings.Contains(message, "must define non-empty description"):
		return FrameworkCheckFinding{
			Kind:    "skill_missing_description",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: fmt.Sprintf("local skill root %q must define a non-empty description in SKILL.md frontmatter", rootPath),
		}
	case strings.Contains(message, "must start with YAML frontmatter"),
		strings.Contains(message, "frontmatter must terminate"),
		strings.Contains(message, "frontmatter is invalid YAML"):
		return FrameworkCheckFinding{
			Kind:    "skill_invalid_frontmatter",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: fmt.Sprintf("local skill root %q has invalid SKILL.md frontmatter", rootPath),
		}
	case strings.Contains(message, "must match directory basename"):
		return FrameworkCheckFinding{
			Kind:    "skill_name_mismatch",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: message,
		}
	case strings.Contains(message, `resolved local skill name "`) && strings.Contains(message, "declared by multiple roots"):
		return FrameworkCheckFinding{
			Kind:    "skill_name_collision",
			OrbitID: orbitID,
			Message: message,
		}
	default:
		return FrameworkCheckFinding{
			Kind:    "skill_invalid",
			OrbitID: orbitID,
			Path:    skillMDPath,
			Message: message,
		}
	}
}

func classifyFrameworkRemoteSkillFinding(orbitID string, err error) FrameworkCheckFinding {
	return FrameworkCheckFinding{
		Kind:     "skill_remote_uri_unsupported",
		OrbitID:  orbitID,
		Blocking: true,
		Message:  err.Error(),
	}
}

func classifyFrameworkAgentAddonHookFinding(orbitID string, err error) FrameworkCheckFinding {
	return FrameworkCheckFinding{
		Kind:    "agent_addon_hook_invalid",
		OrbitID: orbitID,
		Message: err.Error(),
	}
}

func cloneFrameworkPackageAgentHookTargets(targets map[string]bool) map[string]bool {
	if len(targets) == 0 {
		return nil
	}
	cloned := make(map[string]bool, len(targets))
	for key, value := range targets {
		cloned[key] = value
	}

	return cloned
}

func quotedValueAfter(message string, prefix string) string {
	start := strings.Index(message, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	rest := message[start:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}

	return rest[:end]
}

func joinQuotedOrbitIDs(orbitIDs []string) string {
	quoted := make([]string, 0, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		quoted = append(quoted, fmt.Sprintf("%q", orbitID))
	}

	return strings.Join(quoted, " and ")
}

func sortFrameworkCheckFindings(findings []FrameworkCheckFinding) []FrameworkCheckFinding {
	sorted := append([]FrameworkCheckFinding(nil), findings...)
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].Kind != sorted[right].Kind {
			return sorted[left].Kind < sorted[right].Kind
		}
		if sorted[left].OrbitID != sorted[right].OrbitID {
			return sorted[left].OrbitID < sorted[right].OrbitID
		}
		if sorted[left].Path != sorted[right].Path {
			return sorted[left].Path < sorted[right].Path
		}
		return sorted[left].Message < sorted[right].Message
	})

	return sorted
}

func blockingFrameworkCapabilityError(operation string, findings []FrameworkCheckFinding) error {
	messages := make([]string, 0, len(findings))
	for _, finding := range findings {
		if !isBlockingFrameworkCapabilityFinding(finding) {
			continue
		}
		messages = append(messages, finding.Message)
	}
	if len(messages) == 0 {
		return nil
	}

	return fmt.Errorf("%s blocked by capability findings: %s", operation, strings.Join(messages, "; "))
}

func isBlockingFrameworkCapabilityFinding(finding FrameworkCheckFinding) bool {
	if finding.Kind == "skill_remote_uri_unsupported" {
		return finding.Blocking
	}

	return true
}
