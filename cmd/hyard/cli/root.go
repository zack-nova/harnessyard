package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	harnesscli "github.com/zack-nova/harnessyard/cmd/harness/cli"
	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcli "github.com/zack-nova/harnessyard/cmd/orbit/cli"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
)

type exitCodeCarrier interface {
	ExitCode() int
}

// NewRootCommand builds the top-level hyard command tree.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "hyard",
		Short: "Harness Yard CLI (hyard)",
		Long: "Harness Yard CLI (hyard)\n" +
			"Canonical user-facing CLI for runtime bootstrap, install, current-worktree flow,\n" +
			"and agent activation on top of Orbit's Git-native runtime substrate.\n" +
			"Use `hyard plumbing` for raw compatibility and low-level command trees.",
		Example: "" +
			"  hyard create runtime demo-repo\n" +
			"  hyard create source ./research-source --orbit research\n" +
			"  hyard clone ../starter-template --ref harness-template/workspace\n" +
			"  hyard install orbit-template/docs --bindings .harness/vars.yaml\n" +
			"  hyard current\n" +
			"  hyard enter docs\n" +
			"  hyard status\n" +
			"  hyard agent inspect\n" +
			"  hyard plumbing harness inspect\n",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.Version = Version()
	rootCmd.SetVersionTemplate("{{ .Version }}\n")

	rootCmd.AddCommand(
		newCreateCommand(),
		newCloneCommand(),
		newAdoptCommand(),
		newLayoutCommand(),
		newInitCommand(),
		newInstallCommand(),
		newUninstallCommand(),
		newRemoveCommand(),
		newPublishCommand(),
		newPrepareCommand(),
		newBootstrapCommand(),
		newOrbitCommand(),
		newAssignCommand(),
		newUnassignCommand(),
		harnesscommands.NewCheckCommand(),
		harnesscommands.NewReadyCommand(),
		harnesscommands.NewAgentCommand(),
		newHooksCommand(),
		newGuideCommand(),
		newViewCommand(),
		orbitcommands.NewCurrentCommand(),
		orbitcommands.NewEnterCommand(),
		orbitcommands.NewLeaveCommand(),
		orbitcommands.NewStatusCommand(),
		orbitcommands.NewDiffCommand(),
		orbitcommands.NewLogCommand(),
		orbitcommands.NewCommitCommand(),
		orbitcommands.NewRestoreCommand(),
		newPlumbingCommand(),
	)

	return rootCmd
}

func newPlumbingCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plumbing",
		Short: "Show raw compatibility and low-level command trees",
		Long: "Show raw compatibility and low-level command trees.\n" +
			"Use this surface for historical `orbit` / `harness` command families, fine-grained\n" +
			"diagnostics, and migration-oriented compatibility paths that are not part of the\n" +
			"headline `hyard` user layer.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		harnesscli.NewCompatibilityRootCommand(),
		orbitcli.NewCompatibilityRootCommand(),
	)

	return cmd
}

// Execute runs the root command with the provided context.
func Execute(ctx context.Context) error {
	if err := NewRootCommand().ExecuteContext(ctx); err != nil {
		return fmt.Errorf("execute root command: %w", err)
	}

	return nil
}

// ErrorExitCode extracts a command-specific process exit code from an error.
func ErrorExitCode(err error) (int, bool) {
	var carrier exitCodeCarrier
	if errors.As(err, &carrier) {
		return carrier.ExitCode(), true
	}

	return 0, false
}
