package commands

import "github.com/spf13/cobra"

// NewAgentConfigCommand creates the harness agent config command tree.
func NewAgentConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Import local agent configuration into harness truth",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(NewAgentConfigImportCommand())

	return cmd
}
