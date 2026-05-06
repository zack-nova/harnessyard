package harness

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// StartExecutionInput captures one mutating Harness Start request.
type StartExecutionInput struct {
	RepoRoot          string
	GitDir            string
	HarnessID         string
	FrameworkOverride string
	Launcher          StartLauncher
}

// StartExecutionResult reports the completed mutating Harness Start handoff.
type StartExecutionResult struct {
	SchemaVersion       int                          `json:"schema_version"`
	HarnessRoot         string                       `json:"harness_root"`
	HarnessID           string                       `json:"harness_id"`
	FrameworkResolution StartFrameworkResolutionPlan `json:"framework_resolution"`
	SelectionPath       string                       `json:"selection_path,omitempty"`
	Activation          FrameworkApplyResult         `json:"activation"`
	BootstrapAgentSkill BootstrapAgentSkillSetupPlan `json:"bootstrap_agent_skill"`
	Launcher            StartLaunchResult            `json:"launcher"`
	WriteConflicts      []StartWriteConflict         `json:"write_conflicts,omitempty"`
	StartPrompt         string                       `json:"start_prompt"`
	Warnings            []string                     `json:"warnings,omitempty"`
}

// ExecuteStart performs Harness Start mutations, then hands off to the selected launcher.
func ExecuteStart(ctx context.Context, input StartExecutionInput) (StartExecutionResult, error) {
	launcher := input.Launcher
	if launcher == nil {
		launcher = DefaultStartLauncher()
	}

	planInput := StartPlanInput{
		RepoRoot:          input.RepoRoot,
		GitDir:            input.GitDir,
		HarnessID:         input.HarnessID,
		FrameworkOverride: input.FrameworkOverride,
	}
	resolution, err := resolveFrameworkForStartPlan(ctx, planInput)
	if err != nil {
		return StartExecutionResult{}, fmt.Errorf("resolve framework: %w", err)
	}

	result := StartExecutionResult{
		SchemaVersion: 1,
		HarnessRoot:   input.RepoRoot,
		HarnessID:     input.HarnessID,
		FrameworkResolution: StartFrameworkResolutionPlan{
			Status:                 startFrameworkResolutionStatus(resolution),
			SelectedFramework:      resolution.Framework,
			SelectionSource:        resolution.Source,
			Candidates:             append([]string(nil), resolution.Candidates...),
			RecommendedFramework:   resolution.RecommendedFramework,
			SupportedFrameworks:    append([]string(nil), resolution.SupportedFrameworks...),
			PackageRecommendations: append([]FrameworkPackageRecommendation(nil), resolution.PackageRecommendations...),
			Warnings:               append([]string(nil), resolution.Warnings...),
		},
		StartPrompt: BuildStartPrompt(StartPromptInput{RepoRoot: input.RepoRoot}),
		Warnings:    append([]string(nil), resolution.Warnings...),
	}
	if resolution.Source == FrameworkSelectionSourceUnresolvedConflict {
		result.Launcher = startLaunchResultFromPlan(launcher.Plan(ctx, planInput, ""))
		return result, fmt.Errorf("framework resolution is ambiguous: %s", strings.Join(resolution.Candidates, ", "))
	}
	if resolution.Framework == "" {
		result.Launcher = startLaunchResultFromPlan(launcher.Plan(ctx, planInput, ""))
		return result, fmt.Errorf("framework resolution is unresolved")
	}

	launcherPlan := launcher.Plan(ctx, planInput, resolution.Framework)
	if !launcherPlan.Launchable {
		result.Launcher = startLaunchResultFromPlan(launcherPlan)
		return result, fmt.Errorf("cannot launch %s interactively: launcher status %s", resolution.Framework, launcherPlan.Status)
	}

	frameworkPlan, err := buildStartFrameworkPlan(ctx, planInput, resolution.Framework)
	if err != nil {
		return result, fmt.Errorf("build project-only activation plan: %w", err)
	}
	bootstrapPlan, err := PlanBootstrapAgentSkillSetup(BootstrapAgentSkillSetupInput{
		RepoRoot:  input.RepoRoot,
		GitDir:    input.GitDir,
		Framework: resolution.Framework,
	})
	if err != nil {
		return result, fmt.Errorf("plan bootstrap agent skill setup: %w", err)
	}
	result.BootstrapAgentSkill = bootstrapPlan
	conflicts, err := detectStartWriteConflicts(ctx, planInput, resolution, frameworkPlan, bootstrapPlan)
	if err != nil {
		return result, fmt.Errorf("detect start write conflicts: %w", err)
	}
	if len(conflicts) > 0 {
		result.WriteConflicts = conflicts
		return result, startWriteConflictsError(conflicts)
	}

	if strings.TrimSpace(input.FrameworkOverride) != "" {
		selectionPath, err := WriteFrameworkSelection(input.GitDir, FrameworkSelection{
			SelectedFramework: resolution.Framework,
			SelectionSource:   FrameworkSelectionSourceExplicitLocal,
			UpdatedAt:         time.Now().UTC(),
		})
		if err != nil {
			return result, fmt.Errorf("write framework selection: %w", err)
		}
		result.SelectionPath = selectionPath
	}

	applyFrameworkOverride, applyResolutionSource, err := frameworkOverrideForStartApply(ctx, input, resolution)
	if err != nil {
		return result, err
	}
	activation, err := ApplyFramework(ctx, FrameworkApplyInput{
		RepoRoot:            input.RepoRoot,
		GitDir:              input.GitDir,
		HarnessID:           input.HarnessID,
		FrameworkOverride:   applyFrameworkOverride,
		ResolutionSource:    applyResolutionSource,
		RouteChoice:         FrameworkApplyRouteProject,
		AllowGlobalFallback: false,
		EnableHooks:         false,
	})
	if err != nil {
		return result, fmt.Errorf("apply project-only framework activation: %w", err)
	}
	result.Activation = activation
	result.Warnings = append(result.Warnings, activation.Warnings...)

	bootstrap, err := ApplyBootstrapAgentSkillSetup(BootstrapAgentSkillSetupInput{
		RepoRoot:  input.RepoRoot,
		GitDir:    input.GitDir,
		Framework: resolution.Framework,
	})
	if err != nil {
		return result, fmt.Errorf("apply bootstrap agent skill setup: %w", err)
	}
	result.BootstrapAgentSkill = bootstrap

	launchResult, err := launcher.Launch(ctx, StartLaunchRequest{
		RepoRoot:    input.RepoRoot,
		GitDir:      input.GitDir,
		HarnessID:   input.HarnessID,
		Framework:   resolution.Framework,
		StartPrompt: result.StartPrompt,
	})
	result.Launcher = launchResult
	if err != nil {
		return result, fmt.Errorf("launch %s: %w", resolution.Framework, err)
	}

	return result, nil
}

func frameworkOverrideForStartApply(ctx context.Context, input StartExecutionInput, resolution FrameworkResolution) (string, FrameworkSelectionSource, error) {
	if strings.TrimSpace(input.FrameworkOverride) != "" {
		return "", "", nil
	}

	baseResolution, err := ResolveFramework(ctx, FrameworkResolutionInput{
		RepoRoot: input.RepoRoot,
		GitDir:   input.GitDir,
	})
	if err != nil {
		return "", "", fmt.Errorf("resolve framework for activation: %w", err)
	}
	if baseResolution.Framework == resolution.Framework {
		return "", "", nil
	}

	return resolution.Framework, resolution.Source, nil
}
