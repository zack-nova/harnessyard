package commands

import "github.com/spf13/cobra"

// NewBindingsCommand creates the harness bindings command tree.
func NewBindingsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bindings",
		Short: "Manage harness-level shared bindings helpers",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewBindingsApplyCommand(),
		NewBindingsMissingCommand(),
		NewBindingsPlanCommand(),
		NewBindingsScanRuntimeCommand(),
	)

	return cmd
}
