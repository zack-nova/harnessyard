package commands

import "github.com/spf13/cobra"

// NewFrameworkCommand creates the harness framework command tree.
func NewFrameworkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "framework",
		Short: "Inspect and select agent framework activation for the current runtime",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewFrameworkApplyCommand(),
		NewFrameworkCheckCommand(),
		NewAgentDetectCommand(),
		NewFrameworkInspectCommand(),
		NewFrameworkListCommand(),
		NewFrameworkPlanCommand(),
		NewFrameworkRecommendCommand(),
		NewFrameworkRemoveCommand(),
		NewFrameworkUseCommand(),
	)

	return cmd
}
