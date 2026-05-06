package harness

import (
	"context"
	"fmt"
)

// StartLauncher owns the framework-specific Interactive Agent Session handoff.
type StartLauncher interface {
	Plan(frameworkID string) StartLauncherPlan
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
	Framework                  string   `json:"framework,omitempty"`
	Status                     string   `json:"status"`
	Launchable                 bool     `json:"launchable"`
	ManualFallbackInstructions []string `json:"manual_fallback_instructions,omitempty"`
}

type defaultStartLauncher struct{}

// DefaultStartLauncher returns the production launcher registry for Harness Start.
func DefaultStartLauncher() StartLauncher {
	return defaultStartLauncher{}
}

func (defaultStartLauncher) Plan(frameworkID string) StartLauncherPlan {
	return buildStartLauncherPlan(frameworkID)
}

func (launcher defaultStartLauncher) Launch(_ context.Context, request StartLaunchRequest) (StartLaunchResult, error) {
	plan := launcher.Plan(request.Framework)
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
		ManualFallbackInstructions: append([]string(nil), plan.ManualFallbackInstructions...),
	}
}
