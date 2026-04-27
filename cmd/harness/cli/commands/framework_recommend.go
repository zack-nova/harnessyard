package commands

import "github.com/spf13/cobra"

// NewFrameworkRecommendCommand creates the harness framework recommend command tree.
func NewFrameworkRecommendCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Author the runtime's recommended framework truth",
		Example: "" +
			"  harness framework recommend set codex\n" +
			"  harness framework recommend show --json\n",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		NewFrameworkRecommendSetCommand(),
		NewFrameworkRecommendShowCommand(),
	)

	return cmd
}
