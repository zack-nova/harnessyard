package harness

import (
	"context"
	"fmt"
)

// StartLauncher owns the framework-specific Interactive Agent Session handoff.
type StartLauncher interface {
	Plan(context.Context, StartPlanInput, string) StartLauncherPlan
	Launch(context.Context, StartLaunchRequest) (StartLaunchResult, error)
}

// StartLaunchRequest is the stable handoff payload passed to an Agent Framework Launcher.
type StartLaunchRequest struct {
	RepoRoot    string
	GitDir      string
	HarnessID   string
	Framework   string
	StartPrompt string
}

// StartLaunchResult reports the launcher handoff outcome.
type StartLaunchResult struct {
	Framework                  string               `json:"framework,omitempty"`
	Status                     string               `json:"status"`
	Launchable                 bool                 `json:"launchable"`
	DetectionStatus            AgentDetectionStatus `json:"detection_status,omitempty"`
	TerminalCLIDetected        bool                 `json:"terminal_cli_detected"`
	ManualFallbackInstructions []string             `json:"manual_fallback_instructions,omitempty"`
}

type defaultStartLauncher struct{}

// DefaultStartLauncher returns the production launcher registry for Harness Start.
func DefaultStartLauncher() StartLauncher {
	return defaultStartLauncher{}
}

func (defaultStartLauncher) Plan(ctx context.Context, input StartPlanInput, frameworkID string) StartLauncherPlan {
	return buildStartLauncherPlan(ctx, input, frameworkID)
}

func (launcher defaultStartLauncher) Launch(ctx context.Context, request StartLaunchRequest) (StartLaunchResult, error) {
	plan := launcher.Plan(ctx, StartPlanInput{
		RepoRoot:  request.RepoRoot,
		GitDir:    request.GitDir,
		HarnessID: request.HarnessID,
	}, request.Framework)
	result := startLaunchResultFromPlan(plan)
	if !plan.Launchable {
		return result, fmt.Errorf("framework %s cannot be launched interactively from hyard start yet", request.Framework)
	}

	return result, nil
}

func startLaunchResultFromPlan(plan StartLauncherPlan) StartLaunchResult {
	return StartLaunchResult{
		Framework:                  plan.Framework,
		Status:                     plan.Status,
		Launchable:                 plan.Launchable,
		DetectionStatus:            plan.DetectionStatus,
		TerminalCLIDetected:        plan.TerminalCLIDetected,
		ManualFallbackInstructions: append([]string(nil), plan.ManualFallbackInstructions...),
	}
}
