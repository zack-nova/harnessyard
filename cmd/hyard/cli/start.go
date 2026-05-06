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
			"Dry-run JSON mode prints the planned Harness Start handoff without writing files or launching an agent.",
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

			if !printPrompt {
				plan, err := harnesspkg.BuildStartPlan(cmd.Context(), harnesspkg.StartPlanInput{
					RepoRoot:          resolved.Repo.Root,
					GitDir:            resolved.Repo.GitDir,
					HarnessID:         resolved.Manifest.Runtime.ID,
					FrameworkOverride: frameworkOverride,
				})
				if err != nil {
					return fmt.Errorf("build start plan: %w", err)
				}

				return emitHyardStartFallback(cmd, plan)
			}

			prompt := harnesspkg.BuildStartPrompt(harnesspkg.StartPromptInput{
				RepoRoot: resolved.Repo.Root,
			})
			if _, err := fmt.Fprint(cmd.OutOrStdout(), prompt); err != nil {
				return fmt.Errorf("write start prompt: %w", err)
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

func emitHyardStartFallback(cmd *cobra.Command, plan harnesspkg.StartPlan) error {
	frameworkID := plan.Launcher.Framework
	if frameworkID == "" {
		frameworkID = plan.FrameworkResolution.SelectedFramework
	}

	if frameworkID == "" {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Harness Start cannot resolve a launchable Agent Framework."); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	} else {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Harness Start cannot launch %s interactively.\n", frameworkID); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework_resolution: %s\n", plan.FrameworkResolution.Status); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if plan.FrameworkResolution.SelectionSource != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "selection_source: %s\n", plan.FrameworkResolution.SelectionSource); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if frameworkID != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework: %s\n", frameworkID); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "launcher_status: %s\n", plan.Launcher.Status); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if plan.Launcher.DetectionStatus != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "launcher_detection_status: %s\n", plan.Launcher.DetectionStatus); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "terminal_cli_detected: %t\n", plan.Launcher.TerminalCLIDetected); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	for _, instruction := range plan.Launcher.ManualFallbackInstructions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "manual_next_action: %s\n", instruction); err != nil {
			return fmt.Errorf("write start fallback: %w", err)
		}
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "usage:"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  hyard start --print-prompt"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  hyard start --dry-run --json"); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), plan.StartPrompt); err != nil {
		return fmt.Errorf("write start fallback: %w", err)
	}

	if frameworkID == "" {
		return fmt.Errorf("cannot launch interactively without a resolved Agent Framework")
	}

	return fmt.Errorf("cannot launch %s interactively: launcher status %s", frameworkID, plan.Launcher.Status)
}
