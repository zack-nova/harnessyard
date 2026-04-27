package commands

import "github.com/spf13/cobra"

// NewGuidanceCommand creates the harness guidance command tree.
func NewGuidanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guidance",
		Short: "Manage runtime guidance artifacts for agent and human targets",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewGuidanceComposeCommand(),
	)

	return cmd
}
