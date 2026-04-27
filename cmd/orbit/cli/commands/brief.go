package commands

import "github.com/spf13/cobra"

// NewBriefCommand creates the brief command tree.
func NewBriefCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Manage orbit brief materialize and backfill commands",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewBriefMaterializeCommand(),
		NewBriefBackfillCommand(),
	)

	return cmd
}
