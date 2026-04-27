package commands

import "github.com/spf13/cobra"

// NewSourceCommand creates the orbit source command tree.
func NewSourceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage source authoring commands",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewSourceCreateCommand(),
		NewSourceInitCommand(),
	)

	return cmd
}
