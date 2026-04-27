package commands

import "github.com/spf13/cobra"

// NewGuidanceCommand creates the orbit guidance command tree.
func NewGuidanceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "guidance",
		Short: "Manage orbit guidance artifacts across agents, humans, and bootstrap targets",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewGuidanceMaterializeCommand(),
		NewGuidanceBackfillCommand(),
	)

	return cmd
}
