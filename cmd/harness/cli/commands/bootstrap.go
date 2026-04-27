package commands

import "github.com/spf13/cobra"

// NewBootstrapCommand creates the harness bootstrap command tree.
func NewBootstrapCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Manage runtime bootstrap completion and cleanup",
		Example: "" +
			"  harness bootstrap complete --orbit docs\n" +
			"  harness bootstrap complete --all --json\n" +
			"  harness bootstrap reopen --orbit docs\n",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		NewBootstrapCompleteCommand(),
		NewBootstrapReopenCommand(),
	)

	return cmd
}
