package commands

import "github.com/spf13/cobra"

// NewBranchCommand creates the Phase 2 branch-info command tree.
func NewBranchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Inspect and classify branches",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewBranchInspectCommand(),
		NewBranchListCommand(),
		NewBranchStatusCommand(),
	)

	return cmd
}
