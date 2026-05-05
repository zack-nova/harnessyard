package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

func newStartCommand() *cobra.Command {
	var printPrompt bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start an agent handoff for this runtime repo",
		Long: "Start an agent handoff for this runtime repo.\n" +
			"Prompt-only mode prints the shared Start Prompt without mutating the Harness Runtime.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !printPrompt {
				return fmt.Errorf("hyard start currently supports --print-prompt")
			}

			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("cannot run hyard start outside a Harness Runtime: %w", err)
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

	return cmd
}
