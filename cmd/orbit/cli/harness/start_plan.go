package harness

import (
	"context"
	"fmt"
)

// StartPlanInput captures one mutation-free Harness Start planning request.
type StartPlanInput struct {
	RepoRoot          string
	GitDir            string
	HarnessID         string
	FrameworkOverride string
}

// StartPlan captures the stable dry-run Harness Start handoff contract.
type StartPlan struct {
	SchemaVersion       int                          `json:"schema_version"`
	DryRun              bool                         `json:"dry_run"`
	HarnessRoot         string                       `json:"harness_root"`
	HarnessID           string                       `json:"harness_id"`
	FrameworkResolution StartFrameworkResolutionPlan `json:"framework_resolution"`
	Activation          StartActivationPlan          `json:"activation"`
	BootstrapAgentSkill BootstrapAgentSkillSetupPlan `json:"bootstrap_agent_skill"`
	Launcher            StartLauncherPlan            `json:"launcher"`
	StartPrompt         string                       `json:"start_prompt"`
	Warnings            []string                     `json:"warnings,omitempty"`
}

// StartFrameworkResolutionPlan reports how Harness Start chose or failed to choose a framework.
type StartFrameworkResolutionPlan struct {
	Status                 string                           `json:"status"`
	SelectedFramework      string                           `json:"selected_framework,omitempty"`
	SelectionSource        FrameworkSelectionSource         `json:"selection_source"`
	Candidates             []string                         `json:"candidates,omitempty"`
	RecommendedFramework   string                           `json:"recommended_framework,omitempty"`
	SupportedFrameworks    []string                         `json:"supported_frameworks,omitempty"`
	PackageRecommendations []FrameworkPackageRecommendation `json:"package_recommendations,omitempty"`
	Warnings               []string                         `json:"warnings,omitempty"`
}

// StartActivationPlan previews the project-only Framework Activation route.
type StartActivationPlan struct {
	Status                    string                     `json:"status"`
	Route                     string                     `json:"route"`
	Framework                 string                     `json:"framework,omitempty"`
	GuidanceOutputs           []FrameworkPlanOutput      `json:"guidance_outputs,omitempty"`
	RecommendedProjectOutputs []FrameworkRoutePlanOutput `json:"recommended_project_outputs,omitempty"`
	CompatibilityOutputs      []FrameworkRoutePlanOutput `json:"compatibility_outputs,omitempty"`
	Warnings                  []string                   `json:"warnings,omitempty"`
}

// StartLauncherPlan previews whether Harness Start can launch the selected framework.
type StartLauncherPlan struct {
	Framework                  string   `json:"framework,omitempty"`
	Status                     string   `json:"status"`
	Launchable                 bool     `json:"launchable"`
	ManualFallbackInstructions []string `json:"manual_fallback_instructions,omitempty"`
	Warnings                   []string `json:"warnings,omitempty"`
}

// BuildStartPlan returns a mutation-free Harness Start plan for automation callers.
func BuildStartPlan(ctx context.Context, input StartPlanInput) (StartPlan, error) {
	resolution, err := resolveFrameworkForStartPlan(ctx, input)
	if err != nil {
		return StartPlan{}, fmt.Errorf("resolve framework: %w", err)
	}

	plan := StartPlan{
		SchemaVersion: 1,
		DryRun:        true,
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
		Activation: StartActivationPlan{
			Status: "skipped",
			Route:  "project",
		},
		BootstrapAgentSkill: BootstrapAgentSkillSetupPlan{
			RepoRoot:  input.RepoRoot,
			SkillName: BootstrapAgentSkillName,
			Action:    "skipped",
			Changed:   false,
			Remove:    false,
		},
		Launcher:    buildStartLauncherPlan(resolution.Framework),
		StartPrompt: BuildStartPrompt(StartPromptInput{RepoRoot: input.RepoRoot}),
		Warnings:    append([]string(nil), resolution.Warnings...),
	}
	if resolution.Source == FrameworkSelectionSourceUnresolvedConflict || resolution.Framework == "" {
		return plan, nil
	}

	frameworkPlan, err := buildStartFrameworkPlan(ctx, input, resolution.Framework)
	if err != nil {
		return StartPlan{}, fmt.Errorf("build project-only activation plan: %w", err)
	}
	plan.Activation = StartActivationPlan{
		Status:                    "planned",
		Route:                     "project",
		Framework:                 resolution.Framework,
		GuidanceOutputs:           append([]FrameworkPlanOutput(nil), frameworkPlan.ProjectOutputs...),
		RecommendedProjectOutputs: append([]FrameworkRoutePlanOutput(nil), frameworkPlan.RecommendedProjectOutputs...),
		CompatibilityOutputs:      append([]FrameworkRoutePlanOutput(nil), frameworkPlan.CompatibilityOutputs...),
		Warnings:                  append([]string(nil), frameworkPlan.Warnings...),
	}

	bootstrapPlan, err := PlanBootstrapAgentSkillSetup(BootstrapAgentSkillSetupInput{
		RepoRoot:  input.RepoRoot,
		GitDir:    input.GitDir,
		Framework: resolution.Framework,
	})
	if err != nil {
		return StartPlan{}, fmt.Errorf("plan bootstrap agent skill setup: %w", err)
	}
	plan.BootstrapAgentSkill = bootstrapPlan

	return plan, nil
}

func resolveFrameworkForStartPlan(ctx context.Context, input StartPlanInput) (FrameworkResolution, error) {
	resolution, err := ResolveFramework(ctx, FrameworkResolutionInput{
		RepoRoot:          input.RepoRoot,
		GitDir:            input.GitDir,
		FrameworkOverride: input.FrameworkOverride,
	})
	if err != nil {
		return FrameworkResolution{}, err
	}
	if resolution.Framework != "" || resolution.Source == FrameworkSelectionSourceUnresolvedConflict {
		return resolution, nil
	}

	agentReport, err := DetectAgents(ctx, AgentDetectionInput{
		RepoRoot: input.RepoRoot,
		GitDir:   input.GitDir,
		Refresh:  true,
		NoCache:  true,
	})
	if err != nil {
		return FrameworkResolution{}, fmt.Errorf("detect ready agents: %w", err)
	}

	readyAgents := readyAgentIDs(agentReport)
	resolution.Warnings = append(resolution.Warnings, agentReport.Warnings...)
	switch len(readyAgents) {
	case 0:
		return resolution, nil
	case 1:
		resolution.Framework = readyAgents[0]
		resolution.Source = FrameworkSelectionSourceProjectDetection
		return resolution, nil
	default:
		resolution.Candidates = readyAgents
		resolution.Source = FrameworkSelectionSourceUnresolvedConflict
		return resolution, nil
	}
}

func readyAgentIDs(report AgentDetectionReport) []string {
	readyAgents := make([]string, 0, len(report.Tools))
	for _, tool := range report.Tools {
		if tool.Summary.Ready {
			readyAgents = append(readyAgents, tool.Agent)
		}
	}

	return readyAgents
}

func buildStartFrameworkPlan(ctx context.Context, input StartPlanInput, frameworkID string) (FrameworkPlan, error) {
	if input.FrameworkOverride != "" {
		return BuildFrameworkPlanForFramework(ctx, input.RepoRoot, input.GitDir, input.HarnessID, frameworkID)
	}

	return BuildFrameworkPlan(ctx, input.RepoRoot, input.GitDir, input.HarnessID)
}

func startFrameworkResolutionStatus(resolution FrameworkResolution) string {
	if resolution.Source == FrameworkSelectionSourceUnresolvedConflict {
		return "ambiguous"
	}
	if resolution.Framework == "" {
		return "unresolved"
	}

	return "resolved"
}

func buildStartLauncherPlan(frameworkID string) StartLauncherPlan {
	if frameworkID == "" {
		return StartLauncherPlan{
			Status:     "skipped",
			Launchable: false,
			ManualFallbackInstructions: []string{
				"Resolve a Harness Start framework, then run `hyard start --print-prompt` and paste the Start Prompt into that agent.",
			},
		}
	}

	switch frameworkID {
	case "codex":
		return StartLauncherPlan{
			Framework:  frameworkID,
			Status:     "unverified",
			Launchable: false,
			ManualFallbackInstructions: []string{
				"From the runtime root, start Codex manually.",
				"Run `hyard start --print-prompt` and paste the Start Prompt into Codex.",
			},
		}
	default:
		return StartLauncherPlan{
			Framework:  frameworkID,
			Status:     "unsupported",
			Launchable: false,
			ManualFallbackInstructions: []string{
				fmt.Sprintf("Harness Start does not yet have an interactive launcher for %s.", frameworkID),
				"Run `hyard start --print-prompt` and paste the Start Prompt into the selected agent.",
			},
		}
	}
}
