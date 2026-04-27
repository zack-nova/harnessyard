package commands

import "github.com/spf13/cobra"

// NewCapabilityCommand creates the orbit capability command tree.
func NewCapabilityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capability",
		Short: "Manage hosted orbit capabilities",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewCapabilityRemoveCommand(),
		NewCapabilityMigrateCommand(),
		NewCapabilitySetCommand(),
		NewCapabilityListCommand(),
		NewCapabilityAddCommand(),
	)

	return cmd
}
