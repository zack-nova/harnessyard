package harness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

	switch request.Framework {
	case "codex":
		return launchCodexInteractive(ctx, request, plan)
	default:
		return result, fmt.Errorf("framework %s cannot be launched interactively from hyard start yet", request.Framework)
	}
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

func launchCodexInteractive(ctx context.Context, request StartLaunchRequest, plan StartLauncherPlan) (StartLaunchResult, error) {
	executablePath, detection := startLauncherExecutablePath(ctx, request.RepoRoot, request.Framework)
	if executablePath == "" {
		result := startLaunchFailureResult(request.Framework, detection, plan)
		return result, fmt.Errorf("codex executable is not a verified terminal CLI launcher")
	}

	//nolint:gosec // The executable path is resolved from PATH and verified with `codex --version`.
	command := exec.CommandContext(ctx, executablePath, codexInteractiveLaunchArgs(request)...)
	command.Dir = request.RepoRoot
	command.Env = os.Environ()
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		result := startLaunchFailureResult(request.Framework, detection, plan)
		return result, fmt.Errorf("codex interactive launch failed: %w", err)
	}

	result := startLaunchResultFromPlan(plan)
	result.Status = "launched"
	result.Launchable = true
	result.ManualFallbackInstructions = nil

	return result, nil
}

func codexInteractiveLaunchArgs(request StartLaunchRequest) []string {
	return []string{
		"--cd", request.RepoRoot,
		"--sandbox", "workspace-write",
		"--ask-for-approval", "on-request",
		request.StartPrompt,
	}
}

func startLauncherExecutablePath(ctx context.Context, repoRoot string, frameworkID string) (string, startLauncherDetection) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	signature, ok := startLauncherAgentSignature(homeDir, frameworkID)
	if !ok {
		return "", startLauncherDetection{}
	}
	tool := detectAgentTool(ctx, repoRoot, homeDir, signature, false)
	detection := startLauncherDetection{
		Status:              tool.Summary.Status,
		TerminalCLIDetected: startLauncherTerminalCLIDetected(tool),
	}
	if !detection.TerminalCLIDetected {
		return "", detection
	}

	candidates := findAgentExecutableCandidates(signature)
	if len(candidates) == 0 {
		return "", detection
	}

	return candidates[0], detection
}

func startLaunchFailureResult(frameworkID string, detection startLauncherDetection, fallbackPlan StartLauncherPlan) StartLaunchResult {
	if detection.Status == "" {
		detection.Status = fallbackPlan.DetectionStatus
		detection.TerminalCLIDetected = fallbackPlan.TerminalCLIDetected
	}

	return StartLaunchResult{
		Framework:                  frameworkID,
		Status:                     "failed",
		Launchable:                 false,
		DetectionStatus:            detection.Status,
		TerminalCLIDetected:        detection.TerminalCLIDetected,
		ManualFallbackInstructions: startLauncherManualFallbackInstructions(frameworkID, detection),
	}
}
