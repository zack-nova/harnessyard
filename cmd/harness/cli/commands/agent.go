package commands

import "github.com/spf13/cobra"

// NewAgentCommand creates the harness agent command tree.
func NewAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Inspect and select agent activation for the current runtime",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewAgentConfigCommand(),
		NewAgentDetectCommand(),
		NewAgentDeriveCommand(),
		NewFrameworkApplyCommand(),
		NewFrameworkCheckCommand(),
		NewFrameworkInspectCommand(),
		NewFrameworkListCommand(),
		NewFrameworkPlanCommand(),
		NewFrameworkRecommendCommand(),
		NewFrameworkRemoveCommand(),
		NewFrameworkUseCommand(),
	)

	return cmd
}
