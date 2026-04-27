package commands

import "github.com/spf13/cobra"

// NewAgentsCommand creates the harness agents command tree.
func NewAgentsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage agent-facing runtime guidance artifacts",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewAgentsComposeCommand(),
	)

	return cmd
}
