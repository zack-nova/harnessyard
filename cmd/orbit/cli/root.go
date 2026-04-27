package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
)

// NewRootCommand builds the direct compatibility orbit command tree.
func NewRootCommand() *cobra.Command {
	return newRootCommand("orbit", true)
}

// NewCompatibilityRootCommand builds the compatibility/plumbing orbit command tree.
func NewCompatibilityRootCommand() *cobra.Command {
	return newRootCommand("orbit", false)
}

func newRootCommand(use string, includeCompatibilityLead bool) *cobra.Command {
	longText := "Git-native CLI for orbit definition, projection, scoped operations, and single-orbit template authoring.\n" +
		"Use orbit when you are working with orbit definitions, current projection state, or orbit template branches."
	if includeCompatibilityLead {
		longText = "Compatibility command surface for historical `orbit` workflows.\n" +
			"`hyard` is the canonical public CLI; use `orbit` when you need compatibility or plumbing-oriented orbit commands.\n\n" +
			longText
	}

	rootCmd := &cobra.Command{
		Use:   use,
		Short: "Compatibility CLI for file-scoped workspace views",
		Long:  longText,
		Example: "" +
			"  orbit create docs\n" +
			"  orbit enter docs\n" +
			"  orbit template save docs --to orbit-template/docs\n" +
			"  orbit bindings init orbit-template/docs --out .harness/vars.yaml\n",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(
		commands.NewInitCommand(),
		commands.NewAddCommand(),
		commands.NewCapabilityCommand(),
		commands.NewSetCommand(),
		commands.NewMemberCommand(),
		commands.NewValidateCommand(),
		commands.NewListCommand(),
		commands.NewShowCommand(),
		commands.NewFilesCommand(),
		commands.NewCurrentCommand(),
		commands.NewEnterCommand(),
		commands.NewLeaveCommand(),
		commands.NewStatusCommand(),
		commands.NewDiffCommand(),
		commands.NewLogCommand(),
		commands.NewCommitCommand(),
		commands.NewRestoreCommand(),
		commands.NewBranchCommand(),
		commands.NewGuidanceCommand(),
		commands.NewBriefCommand(),
		commands.NewBindingsCommand(),
		commands.NewSourceCommand(),
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
