package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

func newStartCommand() *cobra.Command {
	var printPrompt bool
	var dryRun bool
	var frameworkOverride string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start an agent handoff for this runtime repo",
		Long: "Start an agent handoff for this runtime repo.\n" +
			"Prompt-only mode prints the shared Start Prompt without mutating the Harness Runtime.\n" +
			"Dry-run JSON mode prints the planned Harness Start handoff without writing files or launching an agent.\n" +
			"Default mode applies project-only framework activation, installs the Bootstrap Agent Skill, and hands off to the selected launcher.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if printPrompt && dryRun {
				return fmt.Errorf("hyard start cannot combine --print-prompt and --dry-run")
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if dryRun && !jsonOutput {
				return fmt.Errorf("hyard start --dry-run currently requires --json")
			}
			if jsonOutput && !dryRun {
				return fmt.Errorf("hyard start --json requires --dry-run")
			}

			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("cannot run hyard start outside a Harness Runtime: %w", err)
			}

			if dryRun {
				plan, err := harnesspkg.BuildStartPlan(cmd.Context(), harnesspkg.StartPlanInput{
					RepoRoot:          resolved.Repo.Root,
					GitDir:            resolved.Repo.GitDir,
					HarnessID:         resolved.Manifest.Runtime.ID,
					FrameworkOverride: frameworkOverride,
				})
				if err != nil {
					return fmt.Errorf("build start plan: %w", err)
				}

				return emitHyardJSON(cmd, plan)
			}

			if printPrompt {
				prompt := harnesspkg.BuildStartPrompt(harnesspkg.StartPromptInput{
					RepoRoot: resolved.Repo.Root,
				})
				if _, err := fmt.Fprint(cmd.OutOrStdout(), prompt); err != nil {
					return fmt.Errorf("write start prompt: %w", err)
				}

				return nil
			}

			result, err := harnesspkg.ExecuteStart(cmd.Context(), harnesspkg.StartExecutionInput{
				RepoRoot:          resolved.Repo.Root,
				GitDir:            resolved.Repo.GitDir,
				HarnessID:         resolved.Manifest.Runtime.ID,
				FrameworkOverride: frameworkOverride,
				Launcher:          startLauncherFromCommand(cmd),
			})
			if err != nil {
				if result.StartPrompt != "" && result.Launcher.Status != "" && !result.Launcher.Launchable {
					if emitErr := emitHyardStartManualFallback(cmd, result); emitErr != nil {
						return emitErr
					}
				}
				return fmt.Errorf("execute harness start: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness start handed off to %s\n", result.Launcher.Framework); err != nil {
				return fmt.Errorf("write start handoff output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&printPrompt, "print-prompt", false, "Print the Start Prompt without mutating or launching")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview Harness Start without writing files or launching")
	cmd.Flags().StringVar(&frameworkOverride, "with", "", "Use this Agent Framework for Harness Start planning")
	addHyardJSONFlag(cmd)

	return cmd
}

func startLauncherFromCommand(cmd *cobra.Command) harnesspkg.StartLauncher {
	if cmd.Context() != nil {
		if launcher, ok := cmd.Context().Value(hyardStartLauncherContextKey).(harnesspkg.StartLauncher); ok && launcher != nil {
			return launcher
		}
	}

	return harnesspkg.DefaultStartLauncher()
}

func emitHyardStartManualFallback(cmd *cobra.Command, result harnesspkg.StartExecutionResult) error {
	frameworkID := result.Launcher.Framework
	if frameworkID == "" {
		frameworkID = result.FrameworkResolution.SelectedFramework
	}

	if frameworkID == "" {
		if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Harness Start cannot resolve a launchable Agent Framework."); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	} else {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Harness Start cannot launch %s interactively.\n", frameworkID); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "framework_resolution: %s\n", result.FrameworkResolution.Status); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if result.FrameworkResolution.SelectionSource != "" {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "selection_source: %s\n", result.FrameworkResolution.SelectionSource); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if frameworkID != "" {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "framework: %s\n", frameworkID); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "launcher_status: %s\n", result.Launcher.Status); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if result.Launcher.DetectionStatus != "" {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "launcher_detection_status: %s\n", result.Launcher.DetectionStatus); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "terminal_cli_detected: %t\n", result.Launcher.TerminalCLIDetected); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	for _, instruction := range result.Launcher.ManualFallbackInstructions {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "manual_next_action: %s\n", instruction); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "usage:"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "  hyard start --print-prompt"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "  hyard start --dry-run --json"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), result.StartPrompt); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}

	return nil
}
