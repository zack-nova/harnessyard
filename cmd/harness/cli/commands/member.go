package commands

import "github.com/spf13/cobra"

// NewMemberCommand creates the harness member command tree.
func NewMemberCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage individual runtime members",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewMemberExtractCommand(),
	)

	return cmd
}
