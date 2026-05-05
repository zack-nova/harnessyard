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
			if !printPrompt && !dryRun {
				return fmt.Errorf("hyard start currently supports --print-prompt or --dry-run --json")
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
