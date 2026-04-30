package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FrameworkCommandSummary captures one runtime command capability.
type FrameworkCommandSummary struct {
	OrbitID     string `json:"orbit_id"`
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	Untracked   bool   `json:"untracked,omitempty"`
}

// FrameworkSkillSummary captures one runtime skill capability.
type FrameworkSkillSummary struct {
	OrbitID     string `json:"orbit_id"`
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description,omitempty"`
	Untracked   bool   `json:"untracked,omitempty"`
}

// FrameworkRemoteSkillSummary captures one runtime remote skill capability.
type FrameworkRemoteSkillSummary struct {
	OrbitID  string `json:"orbit_id"`
	URI      string `json:"uri"`
	Required bool   `json:"required,omitempty"`
}

// FrameworkPackageAgentHookSummary captures one package-scoped agent hook add-on.
type FrameworkPackageAgentHookSummary struct {
	OrbitID             string          `json:"orbit_id"`
	Package             string          `json:"package"`
	ID                  string          `json:"id"`
	DisplayID           string          `json:"display_id"`
	Required            bool            `json:"required,omitempty"`
	Description         string          `json:"description,omitempty"`
	EventKind           string          `json:"event_kind"`
	Tools               []string        `json:"tools,omitempty"`
	CommandPatterns     []string        `json:"command_patterns,omitempty"`
	HandlerType         string          `json:"handler_type"`
	HandlerPath         string          `json:"handler_path"`
	HandlerDigest       string          `json:"handler_digest,omitempty"`
	TimeoutSeconds      int             `json:"timeout_seconds,omitempty"`
	StatusMessage       string          `json:"status_message,omitempty"`
	Targets             map[string]bool `json:"targets,omitempty"`
	UnsupportedBehavior string          `json:"unsupported_behavior,omitempty"`
	Source              string          `json:"source,omitempty"`
	Activation          string          `json:"activation"`
	Untracked           bool            `json:"untracked,omitempty"`
}

// FrameworkAgentConfigSummary captures runtime-level unified agent config truth.
type FrameworkAgentConfigSummary struct {
	Source   string                              `json:"source"`
	Targets  map[string]AgentUnifiedConfigTarget `json:"targets,omitempty"`
	Sidecars map[string]string                   `json:"sidecars,omitempty"`
	Hooks    *AgentUnifiedHooks                  `json:"hooks,omitempty"`
}

// FrameworkInspectSummary captures aggregated runtime framework truth.
type FrameworkInspectSummary struct {
	RecommendedFramework        string                             `json:"recommended_framework,omitempty"`
	ResolvedFramework           string                             `json:"resolved_framework,omitempty"`
	ResolutionSource            FrameworkSelectionSource           `json:"resolution_source"`
	PackageRecommendations      []FrameworkPackageRecommendation   `json:"package_recommendations,omitempty"`
	SupportedFrameworks         []string                           `json:"supported_frameworks,omitempty"`
	OrbitCount                  int                                `json:"orbit_count"`
	OrbitIDs                    []string                           `json:"orbit_ids,omitempty"`
	CommandCount                int                                `json:"command_count"`
	SkillCount                  int                                `json:"skill_count"`
	RemoteSkillCount            int                                `json:"remote_skill_count"`
	UntrackedCommandCount       int                                `json:"untracked_command_count,omitempty"`
	UntrackedSkillCount         int                                `json:"untracked_skill_count,omitempty"`
	UntrackedPackageHookCount   int                                `json:"untracked_package_agent_hook_count,omitempty"`
	HasAgentGuidance            bool                               `json:"has_agent_guidance"`
	HasHumanGuidance            bool                               `json:"has_human_guidance"`
	HasPendingBootstrapGuidance bool                               `json:"has_pending_bootstrap_guidance"`
	HasAgentConfig              bool                               `json:"has_agent_config"`
	HasAgentHooks               bool                               `json:"has_agent_hooks"`
	AgentHookCount              int                                `json:"agent_hook_count,omitempty"`
	AgentConfig                 *FrameworkAgentConfigSummary       `json:"agent_config,omitempty"`
	Commands                    []FrameworkCommandSummary          `json:"commands,omitempty"`
	Skills                      []FrameworkSkillSummary            `json:"skills,omitempty"`
	RemoteSkills                []FrameworkRemoteSkillSummary      `json:"remote_skills,omitempty"`
	PackageAgentHookCount       int                                `json:"package_agent_hook_count,omitempty"`
	PackageAgentHooks           []FrameworkPackageAgentHookSummary `json:"package_agent_hooks,omitempty"`
	Warnings                    []string                           `json:"warnings,omitempty"`
}

// FrameworkPlanOutput captures one planned project/global side effect.
type FrameworkPlanOutput struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Kind   string `json:"kind"`
}

// FrameworkRoutePlanOutput captures one v0.78 explicit activation route.
type FrameworkRoutePlanOutput struct {
	OrbitID        string   `json:"orbit_id,omitempty"`
	Package        string   `json:"package,omitempty"`
	AddonID        string   `json:"addon_id,omitempty"`
	Artifact       string   `json:"artifact"`
	ArtifactType   string   `json:"artifact_type"`
	Route          string   `json:"route"`
	Recommended    bool     `json:"recommended"`
	Required       bool     `json:"required,omitempty"`
	Source         string   `json:"source,omitempty"`
	Sidecar        string   `json:"sidecar,omitempty"`
	SourceFiles    []string `json:"source_files,omitempty"`
	Path           string   `json:"path,omitempty"`
	Action         string   `json:"action,omitempty"`
	Kind           string   `json:"kind"`
	Mode           string   `json:"mode,omitempty"`
	EffectiveScope string   `json:"effective_scope"`
	Invocation     []string `json:"invocation,omitempty"`
	GeneratedKeys  []string `json:"generated_keys,omitempty"`
	PatchOwnedKeys []string `json:"patch_owned_keys,omitempty"`
	HandlerDigest  string   `json:"handler_digest,omitempty"`
}

// FrameworkDesiredTruth summarizes the versioned/runtime truth consumed by plan.
type FrameworkDesiredTruth struct {
	RecommendedFramework  string `json:"recommended_framework,omitempty"`
	OrbitCount            int    `json:"orbit_count"`
	CommandCount          int    `json:"command_count"`
	SkillCount            int    `json:"skill_count"`
	RemoteSkillCount      int    `json:"remote_skill_count"`
	UntrackedCommandCount int    `json:"untracked_command_count,omitempty"`
	UntrackedSkillCount   int    `json:"untracked_skill_count,omitempty"`
	UntrackedHookCount    int    `json:"untracked_package_agent_hook_count,omitempty"`
	HasAgentGuidance      bool   `json:"has_agent_guidance"`
	HasHumanGuidance      bool   `json:"has_human_guidance"`
	HasPendingBootstrap   bool   `json:"has_pending_bootstrap_guidance"`
	HasAgentConfig        bool   `json:"has_agent_config"`
	HasAgentHooks         bool   `json:"has_agent_hooks"`
	AgentHookCount        int    `json:"agent_hook_count,omitempty"`
	PackageAgentHookCount int    `json:"package_agent_hook_count,omitempty"`
}

// FrameworkPlan captures one framework activation preview.
type FrameworkPlan struct {
	Framework                 string                             `json:"framework,omitempty"`
	ResolutionSource          FrameworkSelectionSource           `json:"resolution_source"`
	PackageRecommendations    []FrameworkPackageRecommendation   `json:"package_recommendations,omitempty"`
	DesiredTruth              FrameworkDesiredTruth              `json:"desired_truth"`
	ProjectOutputs            []FrameworkPlanOutput              `json:"project_outputs,omitempty"`
	GlobalOutputs             []FrameworkPlanOutput              `json:"global_outputs,omitempty"`
	RecommendedProjectOutputs []FrameworkRoutePlanOutput         `json:"recommended_project_outputs,omitempty"`
	RecommendedHybridOutputs  []FrameworkRoutePlanOutput         `json:"recommended_hybrid_outputs,omitempty"`
	OptionalGlobalOutputs     []FrameworkRoutePlanOutput         `json:"optional_global_outputs,omitempty"`
	CompatibilityOutputs      []FrameworkRoutePlanOutput         `json:"compatibility_outputs,omitempty"`
	HookPreview               []FrameworkRoutePlanOutput         `json:"hook_preview,omitempty"`
	PackageAgentHooks         []FrameworkPackageAgentHookSummary `json:"package_agent_hooks,omitempty"`
	RemoteSkills              []FrameworkRemoteSkillSummary      `json:"remote_skills,omitempty"`
	Warnings                  []string                           `json:"warnings,omitempty"`
}

// BuildFrameworkInspectSummary loads current runtime truth and resolves the active framework.
func BuildFrameworkInspectSummary(ctx context.Context, repoRoot string, gitDir string) (FrameworkInspectSummary, error) {
	state, err := loadFrameworkDesiredState(ctx, repoRoot, gitDir)
	if err != nil {
		return FrameworkInspectSummary{}, err
	}

	return state.Summary, nil
}

// BuildFrameworkPlan builds one framework activation preview for the current runtime.
func BuildFrameworkPlan(ctx context.Context, repoRoot string, gitDir string, harnessID string) (FrameworkPlan, error) {
	state, err := loadFrameworkDesiredState(ctx, repoRoot, gitDir)
	if err != nil {
		return FrameworkPlan{}, err
	}
	summary := state.Summary

	plan := FrameworkPlan{
		Framework:              summary.ResolvedFramework,
		ResolutionSource:       summary.ResolutionSource,
		PackageRecommendations: append([]FrameworkPackageRecommendation(nil), summary.PackageRecommendations...),
		DesiredTruth: FrameworkDesiredTruth{
			RecommendedFramework:  summary.RecommendedFramework,
			OrbitCount:            summary.OrbitCount,
			CommandCount:          summary.CommandCount,
			SkillCount:            summary.SkillCount,
			RemoteSkillCount:      summary.RemoteSkillCount,
			UntrackedCommandCount: summary.UntrackedCommandCount,
			UntrackedSkillCount:   summary.UntrackedSkillCount,
			UntrackedHookCount:    summary.UntrackedPackageHookCount,
			HasAgentGuidance:      summary.HasAgentGuidance,
			HasHumanGuidance:      summary.HasHumanGuidance,
			HasPendingBootstrap:   summary.HasPendingBootstrapGuidance,
			HasAgentConfig:        summary.HasAgentConfig,
			HasAgentHooks:         summary.HasAgentHooks,
			AgentHookCount:        summary.AgentHookCount,
			PackageAgentHookCount: summary.PackageAgentHookCount,
		},
		PackageAgentHooks: append([]FrameworkPackageAgentHookSummary(nil), summary.PackageAgentHooks...),
		RemoteSkills:      append([]FrameworkRemoteSkillSummary(nil), summary.RemoteSkills...),
		Warnings:          append([]string(nil), summary.Warnings...),
	}
	if summary.ResolutionSource == FrameworkSelectionSourceUnresolvedConflict {
		return FrameworkPlan{}, fmt.Errorf("framework plan is blocked: framework resolution is unresolved_conflict")
	}
	if summary.ResolvedFramework == "" {
		return plan, nil
	}
	if err := blockingFrameworkCapabilityError("framework plan", state.CapabilityFindings); err != nil {
		return FrameworkPlan{}, err
	}

	if summary.HasAgentGuidance {
		plan.ProjectOutputs = append(plan.ProjectOutputs, FrameworkPlanOutput{
			Path:   "AGENTS.md",
			Action: projectOutputAction(repoRoot, "AGENTS.md"),
			Kind:   "guidance",
		})
	}
	if summary.HasHumanGuidance {
		plan.ProjectOutputs = append(plan.ProjectOutputs, FrameworkPlanOutput{
			Path:   "HUMANS.md",
			Action: projectOutputAction(repoRoot, "HUMANS.md"),
			Kind:   "guidance",
		})
	}
	if summary.HasPendingBootstrapGuidance {
		plan.ProjectOutputs = append(plan.ProjectOutputs, FrameworkPlanOutput{
			Path:   rootBootstrapPath,
			Action: projectOutputAction(repoRoot, rootBootstrapPath),
			Kind:   "guidance",
		})
	}
	if summary.ResolvedFramework == "" {
		return plan, nil
	}

	adapter, ok := LookupFrameworkAdapter(summary.ResolvedFramework)
	if !ok {
		return plan, nil
	}

	if adapter.ProjectAliasPath != "" && summary.HasAgentGuidance {
		plan.ProjectOutputs = append(plan.ProjectOutputs, FrameworkPlanOutput{
			Path:   adapter.ProjectAliasPath,
			Action: "symlink",
			Kind:   "framework_alias",
		})
	}
	if adapter.CommandsGlobal {
		for _, command := range summary.Commands {
			plan.GlobalOutputs = append(plan.GlobalOutputs, FrameworkPlanOutput{
				Path:   fmt.Sprintf("~/.%s/commands/%s__%s__%s.md", adapter.ID, harnessID, command.OrbitID, command.ID),
				Action: "symlink",
				Kind:   "command",
			})
		}
		if len(summary.Commands) > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("framework %s requires global command registration", adapter.ID))
		}
	}
	if adapter.SkillsGlobal {
		for _, skill := range summary.Skills {
			plan.GlobalOutputs = append(plan.GlobalOutputs, FrameworkPlanOutput{
				Path:   fmt.Sprintf("~/.%s/skills/%s__%s__%s", adapter.ID, harnessID, skill.OrbitID, skill.ID),
				Action: "symlink",
				Kind:   "skill",
			})
		}
		if len(summary.Skills) > 0 {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("framework %s requires global skill registration", adapter.ID))
		}
	}
	plan.RecommendedProjectOutputs = buildRecommendedProjectRouteOutputs(adapter.ID, summary)
	plan.OptionalGlobalOutputs = buildOptionalGlobalRouteOutputs(adapter.ID, harnessID, summary)
	plan.CompatibilityOutputs = buildCompatibilityRouteOutputs(adapter.ID, summary)
	hookRoutes := buildAgentHookRouteOutputs(adapter.ID, summary)
	plan.RecommendedHybridOutputs = append(plan.RecommendedHybridOutputs, hookRoutes.HybridProjectOutputs...)
	plan.RecommendedHybridOutputs = append(plan.RecommendedHybridOutputs, hookRoutes.HybridGlobalOutputs...)
	plan.HookPreview = append(plan.HookPreview, hookRoutes.ProjectOutputs...)
	plan.HookPreview = append(plan.HookPreview, hookRoutes.HybridProjectOutputs...)
	plan.HookPreview = append(plan.HookPreview, hookRoutes.HybridGlobalOutputs...)
	plan.HookPreview = append(plan.HookPreview, hookRoutes.HandlerPreview...)
	plan.HookPreview = append(plan.HookPreview, hookRoutes.UnsupportedOutputs...)

	sort.Slice(plan.ProjectOutputs, func(left, right int) bool {
		return plan.ProjectOutputs[left].Path < plan.ProjectOutputs[right].Path
	})
	sort.Slice(plan.GlobalOutputs, func(left, right int) bool {
		return plan.GlobalOutputs[left].Path < plan.GlobalOutputs[right].Path
	})
	sortFrameworkRoutePlanOutputs(plan.RecommendedProjectOutputs)
	sortFrameworkRoutePlanOutputs(plan.RecommendedHybridOutputs)
	sortFrameworkRoutePlanOutputs(plan.OptionalGlobalOutputs)
	sortFrameworkRoutePlanOutputs(plan.CompatibilityOutputs)
	sortFrameworkRoutePlanOutputs(plan.HookPreview)
	sort.Strings(plan.Warnings)

	return plan, nil
}

func buildRecommendedProjectRouteOutputs(frameworkID string, summary FrameworkInspectSummary) []FrameworkRoutePlanOutput {
	outputs := []FrameworkRoutePlanOutput{}
	for _, command := range summary.Commands {
		path, invocation, ok := projectCommandSkillTarget(frameworkID, command.ID)
		if !ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        command.OrbitID,
			Artifact:       command.ID,
			ArtifactType:   "prompt-command",
			Route:          "project_skill",
			Recommended:    true,
			Source:         command.Path,
			Path:           path,
			Action:         "symlink",
			Kind:           "command_as_skill",
			Mode:           "symlink",
			EffectiveScope: projectSkillEffectiveScope(frameworkID),
			Invocation:     invocation,
		})
	}
	for _, skill := range summary.Skills {
		path, invocation, ok := projectLocalSkillTarget(frameworkID, skill.ID)
		if !ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        skill.OrbitID,
			Artifact:       skill.ID,
			ArtifactType:   "local-skill",
			Route:          "project_skill",
			Recommended:    true,
			Source:         skill.Path,
			Path:           path,
			Action:         "symlink",
			Kind:           "skill",
			Mode:           "symlink",
			EffectiveScope: projectSkillEffectiveScope(frameworkID),
			Invocation:     invocation,
		})
	}
	if output, ok := buildProjectAgentConfigRouteOutput(frameworkID, summary); ok {
		outputs = append(outputs, output)
	}

	return outputs
}

func buildOptionalGlobalRouteOutputs(frameworkID string, harnessID string, summary FrameworkInspectSummary) []FrameworkRoutePlanOutput {
	outputs := []FrameworkRoutePlanOutput{}
	for _, command := range summary.Commands {
		path, kind, invocation, ok := globalCommandTarget(frameworkID, harnessID, command)
		if !ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        command.OrbitID,
			Artifact:       command.ID,
			ArtifactType:   "prompt-command",
			Route:          "global_registration",
			Recommended:    false,
			Source:         command.Path,
			Path:           path,
			Action:         "symlink",
			Kind:           kind,
			Mode:           "symlink",
			EffectiveScope: "global",
			Invocation:     invocation,
		})
	}
	for _, skill := range summary.Skills {
		path, invocation, ok := globalLocalSkillTarget(frameworkID, harnessID, skill)
		if !ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        skill.OrbitID,
			Artifact:       skill.ID,
			ArtifactType:   "local-skill",
			Route:          "global_registration",
			Recommended:    false,
			Source:         skill.Path,
			Path:           path,
			Action:         "symlink",
			Kind:           "skill",
			Mode:           "symlink",
			EffectiveScope: "global",
			Invocation:     invocation,
		})
	}
	if output, ok := buildGlobalAgentConfigRouteOutput(frameworkID, summary); ok {
		outputs = append(outputs, output)
	}

	return outputs
}

func buildCompatibilityRouteOutputs(frameworkID string, summary FrameworkInspectSummary) []FrameworkRoutePlanOutput {
	outputs := []FrameworkRoutePlanOutput{}
	for _, command := range summary.Commands {
		if _, _, ok := projectCommandSkillTarget(frameworkID, command.ID); ok {
			continue
		}
		if _, _, _, ok := globalCommandTarget(frameworkID, "", command); ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        command.OrbitID,
			Artifact:       command.ID,
			ArtifactType:   "prompt-command",
			Route:          "project_compatibility",
			Recommended:    false,
			Source:         command.Path,
			Kind:           "command_as_skill",
			Mode:           "not_implemented",
			EffectiveScope: "project",
		})
	}
	for _, skill := range summary.Skills {
		if _, _, ok := projectLocalSkillTarget(frameworkID, skill.ID); ok {
			continue
		}
		if _, _, ok := globalLocalSkillTarget(frameworkID, "", skill); ok {
			continue
		}
		outputs = append(outputs, FrameworkRoutePlanOutput{
			OrbitID:        skill.OrbitID,
			Artifact:       skill.ID,
			ArtifactType:   "local-skill",
			Route:          "project_compatibility",
			Recommended:    false,
			Source:         skill.Path,
			Kind:           "skill",
			Mode:           "not_implemented",
			EffectiveScope: "project",
		})
	}
	if output, ok := buildCompatibilityAgentConfigRouteOutput(frameworkID, summary); ok {
		outputs = append(outputs, output)
	}

	return outputs
}

func projectCommandSkillTarget(frameworkID string, name string) (string, []string, bool) {
	switch frameworkID {
	case "codex":
		return fmt.Sprintf(".codex/skills/%s", name), []string{"$" + name}, true
	case "claude":
		return fmt.Sprintf(".claude/skills/%s", name), []string{"/" + name}, true
	case "openclaw":
		return fmt.Sprintf("skills/%s", name), []string{"/skill " + name}, true
	default:
		return "", nil, false
	}
}

func projectLocalSkillTarget(frameworkID string, name string) (string, []string, bool) {
	switch frameworkID {
	case "codex":
		return fmt.Sprintf(".codex/skills/%s", name), []string{"$" + name}, true
	case "claude":
		return fmt.Sprintf(".claude/skills/%s", name), []string{"/" + name}, true
	case "openclaw":
		return fmt.Sprintf("skills/%s", name), []string{"/skill " + name}, true
	default:
		return "", nil, false
	}
}

func projectSkillEffectiveScope(frameworkID string) string {
	if frameworkID == "openclaw" {
		return "project_workspace"
	}

	return "project"
}

func globalCommandTarget(frameworkID string, harnessID string, command FrameworkCommandSummary) (string, string, []string, bool) {
	name := fmt.Sprintf("%s__%s__%s", harnessID, command.OrbitID, command.ID)
	switch frameworkID {
	case "codex":
		return fmt.Sprintf("~/.codex/prompts/%s.md", name), "command_prompt", []string{"/prompts:" + name}, true
	case "claude":
		return fmt.Sprintf("~/.claude/skills/%s", name), "command_as_skill", []string{"/" + name}, true
	case "openclaw":
		return fmt.Sprintf("~/.agents/skills/%s", name), "command_as_skill", []string{"/skill " + name}, true
	default:
		return "", "", nil, false
	}
}

func globalLocalSkillTarget(frameworkID string, harnessID string, skill FrameworkSkillSummary) (string, []string, bool) {
	name := fmt.Sprintf("%s__%s__%s", harnessID, skill.OrbitID, skill.ID)
	switch frameworkID {
	case "codex":
		return fmt.Sprintf("~/.codex/skills/%s", name), []string{"$" + name}, true
	case "claude":
		return fmt.Sprintf("~/.claude/skills/%s", name), []string{"/" + name}, true
	case "openclaw":
		return fmt.Sprintf("~/.agents/skills/%s", name), []string{"/skill " + name}, true
	default:
		return "", nil, false
	}
}

func sortFrameworkRoutePlanOutputs(outputs []FrameworkRoutePlanOutput) {
	sort.Slice(outputs, func(left, right int) bool {
		if outputs[left].EffectiveScope != outputs[right].EffectiveScope {
			return outputs[left].EffectiveScope < outputs[right].EffectiveScope
		}
		if outputs[left].Path != outputs[right].Path {
			return outputs[left].Path < outputs[right].Path
		}
		if outputs[left].ArtifactType != outputs[right].ArtifactType {
			return outputs[left].ArtifactType < outputs[right].ArtifactType
		}
		return outputs[left].Artifact < outputs[right].Artifact
	})
}

func projectOutputAction(repoRoot string, repoPath string) string {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	if _, err := os.Stat(filename); err == nil {
		return "update"
	}

	return "create"
}
