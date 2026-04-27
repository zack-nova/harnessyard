package commands

import "github.com/spf13/cobra"

// NewBindingsCommand creates the Phase 2 bindings command tree.
func NewBindingsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Manage Orbit template bindings helpers",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewBindingsInitCommand(),
	)

	return cmd
}
