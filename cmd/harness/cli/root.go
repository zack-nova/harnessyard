package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
)

// NewRootCommand builds the direct compatibility harness command tree.
func NewRootCommand() *cobra.Command {
	return newRootCommand("harness", true)
}

// NewCompatibilityRootCommand builds the compatibility/plumbing harness command tree.
func NewCompatibilityRootCommand() *cobra.Command {
	return newRootCommand("harness", false)
}

func newRootCommand(use string, includeCompatibilityLead bool) *cobra.Command {
	longText := "Git-native CLI for harness runtime bootstrap, install flows, member management, diagnostics, and harness template export/publish.\n" +
		"Use harness when you are operating on the runtime as a whole rather than on one projected orbit view."
	if includeCompatibilityLead {
		longText = "Compatibility command surface for historical `harness` workflows.\n" +
			"`hyard` is the canonical public CLI; use `harness` when you need compatibility or plumbing-oriented runtime commands.\n\n" +
			longText
	}

	rootCmd := &cobra.Command{
		Use:   use,
		Short: "Compatibility CLI for harness runtime engineering",
		Long:  longText,
		Example: "" +
			"  harness create demo-repo\n" +
			"  harness bindings plan orbit-template/docs orbit-template/cmd\n" +
			"  harness init\n" +
			"  harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml\n" +
			"  harness guidance compose --target all --output\n" +
			"  harness bootstrap complete --orbit docs\n" +
			"  harness bootstrap reopen --orbit docs\n" +
			"  harness ready\n" +
			"  harness check\n" +
			"  harness template save --to harness-template/workspace\n" +
			"  harness template publish --to harness-template/workspace --push --remote origin\n",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(
		commands.NewAddCommand(),
		commands.NewAgentCommand(),
		commands.NewAgentsCommand(),
		commands.NewBootstrapCommand(),
		commands.NewBindingsCommand(),
		commands.NewCheckCommand(),
		commands.NewCreateCommand(),
		commands.NewFrameworkCommand(),
		commands.NewGuidanceCommand(),
		commands.NewHumansCommand(),
		commands.NewInitCommand(),
		commands.NewInspectCommand(),
		commands.NewInstallCommand(),
		commands.NewMemberCommand(),
		commands.NewReadyCommand(),
		commands.NewRemoveCommand(),
		commands.NewRootPathCommand(),
		commands.NewTemplateCommand(),
	)

	return rootCmd
}

// Execute runs the direct compatibility root command with the provided context.
func Execute(ctx context.Context) error {
	if err := NewRootCommand().ExecuteContext(ctx); err != nil {
		return fmt.Errorf("execute root command: %w", err)
	}

	return nil
}
