package commands

import "github.com/spf13/cobra"

// NewHumansCommand creates the harness humans command tree.
func NewHumansCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "humans",
		Short: "Manage human-facing runtime guidance artifacts",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewHumansComposeCommand(),
	)

	return cmd
}
