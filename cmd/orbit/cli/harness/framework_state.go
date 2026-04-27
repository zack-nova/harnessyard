package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type frameworkGuidanceSnapshot struct {
	OrbitID        string `json:"orbit_id"`
	AgentsTemplate string `json:"agents_template,omitempty"`
	HumansTemplate string `json:"humans_template,omitempty"`
}

type frameworkAgentOverlaySnapshot struct {
	AgentID string `json:"agent_id"`
	Content []byte `json:"content,omitempty"`
}

type frameworkAgentSidecarSnapshot struct {
	Path    string `json:"path"`
	Content []byte `json:"content,omitempty"`
}

type frameworkRuntimeAgentTruthSnapshot struct {
	HasFrameworksFile bool                            `json:"has_frameworks_file"`
	FrameworksFile    FrameworksFile                  `json:"frameworks_file"`
	HasAgentConfig    bool                            `json:"has_agent_config"`
	AgentConfig       AgentConfigFile                 `json:"agent_config"`
	Overlays          []frameworkAgentOverlaySnapshot `json:"overlays,omitempty"`
	HasUnifiedConfig  bool                            `json:"has_unified_config"`
	UnifiedConfig     AgentUnifiedConfigFile          `json:"unified_config"`
	Sidecars          []frameworkAgentSidecarSnapshot `json:"sidecars,omitempty"`
}

type frameworkDesiredState struct {
	Summary            FrameworkInspectSummary
	Guidance           []frameworkGuidanceSnapshot
	CapabilityFindings []FrameworkCheckFinding
}

func loadFrameworkDesiredState(ctx context.Context, repoRoot string, gitDir string) (frameworkDesiredState, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return frameworkDesiredState{}, fmt.Errorf("load runtime file: %w", err)
	}
	repoConfig, err := orbitpkg.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return frameworkDesiredState{}, fmt.Errorf("load runtime repository config: %w", err)
	}
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return frameworkDesiredState{}, fmt.Errorf("load tracked files: %w", err)
	}

	resolution, err := ResolveFramework(ctx, FrameworkResolutionInput{
		RepoRoot: repoRoot,
		GitDir:   gitDir,
	})
	if err != nil {
		return frameworkDesiredState{}, err
	}

	state := frameworkDesiredState{
		Summary: FrameworkInspectSummary{
			RecommendedFramework:   resolution.RecommendedFramework,
			ResolvedFramework:      resolution.Framework,
			ResolutionSource:       resolution.Source,
			PackageRecommendations: append([]FrameworkPackageRecommendation(nil), resolution.PackageRecommendations...),
			SupportedFrameworks:    append([]string(nil), resolution.SupportedFrameworks...),
			Warnings:               append([]string(nil), resolution.Warnings...),
			OrbitCount:             len(runtimeFile.Members),
			OrbitIDs:               make([]string, 0, len(runtimeFile.Members)),
			Commands:               []FrameworkCommandSummary{},
			Skills:                 []FrameworkSkillSummary{},
			RemoteSkills:           []FrameworkRemoteSkillSummary{},
		},
		Guidance:           make([]frameworkGuidanceSnapshot, 0, len(runtimeFile.Members)),
		CapabilityFindings: []FrameworkCheckFinding{},
	}
	runtimeOrbitIDs := make([]string, 0, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		runtimeOrbitIDs = append(runtimeOrbitIDs, member.OrbitID)
	}

	hasPendingBootstrapGuidance, err := hasPendingFrameworkBootstrapGuidance(ctx, repoRoot, gitDir, runtimeOrbitIDs)
	if err != nil {
		return frameworkDesiredState{}, err
	}
	state.Summary.HasPendingBootstrapGuidance = hasPendingBootstrapGuidance

	agentConfig, hasAgentConfig, err := LoadOptionalAgentUnifiedConfigFile(repoRoot)
	if err != nil {
		return frameworkDesiredState{}, fmt.Errorf("load runtime unified agent config: %w", err)
	}
	if hasAgentConfig {
		sidecars, err := frameworkAgentConfigSidecars(repoRoot)
		if err != nil {
			return frameworkDesiredState{}, err
		}
		state.Summary.HasAgentConfig = true
		hooks := cloneAgentUnifiedHooks(agentConfig.Hooks)
		state.Summary.AgentConfig = &FrameworkAgentConfigSummary{
			Source:   AgentUnifiedConfigRepoPath(),
			Targets:  cloneAgentUnifiedConfigTargets(agentConfig.Targets),
			Sidecars: sidecars,
			Hooks:    &hooks,
		}
		if agentConfig.Hooks.Enabled && len(agentConfig.Hooks.Entries) > 0 {
			state.Summary.HasAgentHooks = true
			for _, entry := range agentConfig.Hooks.Entries {
				if entry.Enabled {
					state.Summary.AgentHookCount++
				}
			}
		}
	}

	for _, member := range runtimeFile.Members {
		state.Summary.OrbitIDs = append(state.Summary.OrbitIDs, member.OrbitID)

		spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, member.OrbitID)
		if err != nil {
			return frameworkDesiredState{}, fmt.Errorf("load hosted orbit spec %q: %w", member.OrbitID, err)
		}
		guidance := frameworkGuidanceSnapshot{OrbitID: member.OrbitID}
		if spec.Meta != nil && spec.Meta.AgentsTemplate != "" {
			state.Summary.HasAgentGuidance = true
			guidance.AgentsTemplate = spec.Meta.AgentsTemplate
		}
		if spec.Meta != nil && spec.Meta.HumansTemplate != "" {
			state.Summary.HasHumanGuidance = true
			guidance.HumansTemplate = spec.Meta.HumansTemplate
		}
		state.Guidance = append(state.Guidance, guidance)
		if spec.Capabilities == nil && spec.AgentAddons == nil {
			continue
		}
		plan, err := orbitpkg.ResolveProjectionPlan(repoConfig, spec, trackedFiles)
		if err != nil {
			return frameworkDesiredState{}, fmt.Errorf("resolve projection plan for orbit %q: %w", member.OrbitID, err)
		}
		agentAddonsSnapshot, hasAgentAddonsSnapshot, err := loadMemberAgentAddonsSnapshot(repoRoot, member)
		if err != nil {
			return frameworkDesiredState{}, err
		}
		capabilitySpec := spec
		if hasAgentAddonsSnapshot {
			capabilitySpec.AgentAddons = nil
		}
		commands, localSkills, remoteSkills, packageAgentHooks, findings := resolveFrameworkOrbitCapabilities(
			repoRoot,
			member.OrbitID,
			capabilitySpec,
			trackedFiles,
			plan.ExportPaths,
		)
		if hasAgentAddonsSnapshot {
			packageAgentHooks = frameworkPackageAgentHooksFromSnapshot(member, agentAddonsSnapshot)
		}
		state.Summary.Commands = append(state.Summary.Commands, commands...)
		state.Summary.Skills = append(state.Summary.Skills, localSkills...)
		state.Summary.RemoteSkills = append(state.Summary.RemoteSkills, remoteSkills...)
		state.Summary.PackageAgentHooks = append(state.Summary.PackageAgentHooks, packageAgentHooks...)
		state.CapabilityFindings = append(state.CapabilityFindings, findings...)
	}

	sort.Strings(state.Summary.OrbitIDs)
	sort.Slice(state.Summary.Commands, func(left, right int) bool {
		if state.Summary.Commands[left].OrbitID == state.Summary.Commands[right].OrbitID {
			return state.Summary.Commands[left].ID < state.Summary.Commands[right].ID
		}
		return state.Summary.Commands[left].OrbitID < state.Summary.Commands[right].OrbitID
	})
	sort.Slice(state.Summary.Skills, func(left, right int) bool {
		if state.Summary.Skills[left].OrbitID == state.Summary.Skills[right].OrbitID {
			return state.Summary.Skills[left].ID < state.Summary.Skills[right].ID
		}
		return state.Summary.Skills[left].OrbitID < state.Summary.Skills[right].OrbitID
	})
	sort.Slice(state.Summary.RemoteSkills, func(left, right int) bool {
		if state.Summary.RemoteSkills[left].OrbitID == state.Summary.RemoteSkills[right].OrbitID {
			return state.Summary.RemoteSkills[left].URI < state.Summary.RemoteSkills[right].URI
		}
		return state.Summary.RemoteSkills[left].OrbitID < state.Summary.RemoteSkills[right].OrbitID
	})
	sort.Slice(state.Summary.PackageAgentHooks, func(left, right int) bool {
		if state.Summary.PackageAgentHooks[left].OrbitID == state.Summary.PackageAgentHooks[right].OrbitID {
			return state.Summary.PackageAgentHooks[left].DisplayID < state.Summary.PackageAgentHooks[right].DisplayID
		}
		return state.Summary.PackageAgentHooks[left].OrbitID < state.Summary.PackageAgentHooks[right].OrbitID
	})
	sort.Slice(state.Guidance, func(left, right int) bool {
		return state.Guidance[left].OrbitID < state.Guidance[right].OrbitID
	})
	state.Summary.CommandCount = len(state.Summary.Commands)
	state.Summary.SkillCount = len(state.Summary.Skills)
	state.Summary.RemoteSkillCount = len(state.Summary.RemoteSkills)
	state.Summary.PackageAgentHookCount = len(state.Summary.PackageAgentHooks)
	state.CapabilityFindings = appendFrameworkCapabilityFindings(state.Summary, state.CapabilityFindings)

	return state, nil
}

func loadMemberAgentAddonsSnapshot(repoRoot string, member RuntimeMember) (*orbittemplate.AgentAddonsSnapshot, bool, error) {
	switch member.Source {
	case MemberSourceInstallOrbit:
		record, err := LoadInstallRecord(repoRoot, member.OrbitID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("load install record agent add-ons for %q: %w", member.OrbitID, err)
		}
		if record.AgentAddons == nil || len(record.AgentAddons.Hooks) == 0 {
			return nil, false, nil
		}
		filtered := filterAgentAddonsSnapshotForOrbit(record.AgentAddons, member.OrbitID)
		if filtered == nil || len(filtered.Hooks) == 0 {
			return nil, false, nil
		}
		return filtered, true, nil
	case MemberSourceInstallBundle:
		if strings.TrimSpace(member.OwnerHarnessID) == "" {
			return nil, false, nil
		}
		record, err := LoadBundleRecord(repoRoot, member.OwnerHarnessID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("load bundle record agent add-ons for %q: %w", member.OrbitID, err)
		}
		if record.AgentAddons == nil || len(record.AgentAddons.Hooks) == 0 {
			return nil, false, nil
		}
		filtered := filterAgentAddonsSnapshotForOrbit(record.AgentAddons, member.OrbitID)
		if filtered == nil || len(filtered.Hooks) == 0 {
			return nil, false, nil
		}
		return filtered, true, nil
	default:
		return nil, false, nil
	}
}

func filterAgentAddonsSnapshotForOrbit(snapshot *orbittemplate.AgentAddonsSnapshot, orbitID string) *orbittemplate.AgentAddonsSnapshot {
	if snapshot == nil || len(snapshot.Hooks) == 0 {
		return nil
	}
	filtered := &orbittemplate.AgentAddonsSnapshot{}
	for _, hook := range snapshot.Hooks {
		if strings.TrimSpace(hook.OrbitID) != "" && hook.OrbitID != orbitID {
			continue
		}
		filtered.Hooks = append(filtered.Hooks, hook)
	}
	if len(filtered.Hooks) == 0 {
		return nil
	}

	return filtered
}

func frameworkPackageAgentHooksFromSnapshot(
	member RuntimeMember,
	snapshot *orbittemplate.AgentAddonsSnapshot,
) []FrameworkPackageAgentHookSummary {
	if snapshot == nil || len(snapshot.Hooks) == 0 {
		return nil
	}
	hooks := make([]FrameworkPackageAgentHookSummary, 0, len(snapshot.Hooks))
	for _, hook := range snapshot.Hooks {
		hooks = append(hooks, FrameworkPackageAgentHookSummary{
			OrbitID:             member.OrbitID,
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
			HandlerDigest:       hook.HandlerDigest,
			TimeoutSeconds:      hook.TimeoutSeconds,
			StatusMessage:       hook.StatusMessage,
			Targets:             cloneFrameworkPackageAgentHookTargets(hook.Targets),
			UnsupportedBehavior: normalizedPackageHookUnsupportedBehavior(hook.UnsupportedBehavior),
			Source:              packageHookSnapshotSource(member),
			Activation:          "not_applied",
		})
	}

	return hooks
}

func normalizedPackageHookUnsupportedBehavior(behavior string) string {
	behavior = strings.TrimSpace(behavior)
	if behavior == "" {
		return defaultAgentHookUnsupportedBehavior
	}

	return behavior
}

func packageHookSnapshotSource(member RuntimeMember) string {
	switch member.Source {
	case MemberSourceInstallOrbit:
		return ".harness/installs/" + member.OrbitID + ".yaml"
	case MemberSourceInstallBundle:
		if strings.TrimSpace(member.OwnerHarnessID) != "" {
			return ".harness/bundles/" + member.OwnerHarnessID + ".yaml"
		}
	}

	return ".harness/orbits/" + member.OrbitID + ".yaml"
}

func hasPendingFrameworkBootstrapGuidance(ctx context.Context, repoRoot string, gitDir string, orbitIDs []string) (bool, error) {
	statuses, err := orbittemplate.ListBootstrapEnabledOrbits(ctx, repoRoot, gitDir, orbitIDs)
	if err != nil {
		return false, fmt.Errorf("list bootstrap-enabled orbits: %w", err)
	}
	for _, status := range statuses {
		if orbittemplate.PlanBootstrapFrameworkApply(status).Action == orbittemplate.BootstrapActionAllow {
			return true, nil
		}
	}

	return false, nil
}

func computeFrameworkDesiredHashes(repoRoot string, state frameworkDesiredState) (string, string, string, string, error) {
	guidanceHash, err := hashFrameworkSnapshot(state.Guidance)
	if err != nil {
		return "", "", "", "", fmt.Errorf("hash framework guidance snapshot: %w", err)
	}
	capabilitiesHash, err := hashFrameworkSnapshot(struct {
		Commands          []FrameworkCommandSummary          `json:"commands,omitempty"`
		Skills            []FrameworkSkillSummary            `json:"skills,omitempty"`
		RemoteSkills      []FrameworkRemoteSkillSummary      `json:"remote_skills,omitempty"`
		PackageAgentHooks []FrameworkPackageAgentHookSummary `json:"package_agent_hooks,omitempty"`
	}{
		Commands:          state.Summary.Commands,
		Skills:            state.Summary.Skills,
		RemoteSkills:      state.Summary.RemoteSkills,
		PackageAgentHooks: state.Summary.PackageAgentHooks,
	})
	if err != nil {
		return "", "", "", "", fmt.Errorf("hash framework capabilities snapshot: %w", err)
	}
	selectionHash, err := hashFrameworkSnapshot(struct {
		ResolvedFramework      string                           `json:"resolved_framework,omitempty"`
		RecommendedFramework   string                           `json:"recommended_framework,omitempty"`
		PackageRecommendations []FrameworkPackageRecommendation `json:"package_recommendations,omitempty"`
	}{
		ResolvedFramework:      state.Summary.ResolvedFramework,
		RecommendedFramework:   state.Summary.RecommendedFramework,
		PackageRecommendations: state.Summary.PackageRecommendations,
	})
	if err != nil {
		return "", "", "", "", fmt.Errorf("hash framework selection snapshot: %w", err)
	}
	runtimeAgentTruthHash, err := computeRuntimeAgentTruthHash(repoRoot)
	if err != nil {
		return "", "", "", "", err
	}

	return guidanceHash, capabilitiesHash, selectionHash, runtimeAgentTruthHash, nil
}

func hashFrameworkSnapshot(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode framework snapshot: %w", err)
	}
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

func computeRuntimeAgentTruthHash(repoRoot string) (string, error) {
	snapshot := frameworkRuntimeAgentTruthSnapshot{}

	frameworksFile, hasFrameworksFile, err := loadOptionalFrameworksFileWithPresence(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load runtime frameworks file: %w", err)
	}
	snapshot.HasFrameworksFile = hasFrameworksFile
	snapshot.FrameworksFile = frameworksFile

	agentConfig, hasAgentConfig, err := LoadOptionalAgentConfigFile(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load runtime agent config: %w", err)
	}
	snapshot.HasAgentConfig = hasAgentConfig
	snapshot.AgentConfig = agentConfig

	overlayIDs, err := ListAgentOverlayIDs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("list runtime agent overlays: %w", err)
	}
	snapshot.Overlays = make([]frameworkAgentOverlaySnapshot, 0, len(overlayIDs))
	for _, agentID := range overlayIDs {
		overlay, err := LoadAgentOverlayFile(repoRoot, agentID)
		if err != nil {
			return "", fmt.Errorf("load runtime agent overlay %q: %w", agentID, err)
		}
		snapshot.Overlays = append(snapshot.Overlays, frameworkAgentOverlaySnapshot{
			AgentID: agentID,
			Content: append([]byte(nil), overlay.Content...),
		})
	}

	unifiedConfig, hasUnifiedConfig, err := LoadOptionalAgentUnifiedConfigFile(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load runtime unified agent config: %w", err)
	}
	snapshot.HasUnifiedConfig = hasUnifiedConfig
	snapshot.UnifiedConfig = unifiedConfig
	if hasUnifiedConfig {
		sidecars, err := frameworkAgentConfigSidecars(repoRoot)
		if err != nil {
			return "", err
		}
		sidecarPaths := make([]string, 0, len(sidecars))
		for _, repoPath := range sidecars {
			sidecarPaths = append(sidecarPaths, repoPath)
		}
		sort.Strings(sidecarPaths)
		snapshot.Sidecars = make([]frameworkAgentSidecarSnapshot, 0, len(sidecarPaths))
		for _, repoPath := range sidecarPaths {
			content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(repoPath))) //nolint:gosec // Sidecar paths come from the fixed adapter matrix.
			if err != nil {
				return "", fmt.Errorf("read runtime agent sidecar %s: %w", repoPath, err)
			}
			snapshot.Sidecars = append(snapshot.Sidecars, frameworkAgentSidecarSnapshot{
				Path:    repoPath,
				Content: append([]byte(nil), content...),
			})
		}
	}

	hash, err := hashFrameworkSnapshot(snapshot)
	if err != nil {
		return "", fmt.Errorf("hash runtime agent truth snapshot: %w", err)
	}

	return hash, nil
}
