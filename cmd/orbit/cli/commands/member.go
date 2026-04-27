package commands

import "github.com/spf13/cobra"

// NewMemberCommand creates the orbit member command tree.
func NewMemberCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage hosted orbit members",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewMemberAddCommand(),
		NewMemberBackfillCommand(),
		NewMemberDetectCommand(),
		NewMemberRemoveCommand(),
	)

	return cmd
}
