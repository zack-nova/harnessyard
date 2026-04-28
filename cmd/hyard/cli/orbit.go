package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/packageops"
)

func newOrbitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orbit",
		Short: "Create and manage hosted orbit authored truth",
		Long: "Create and manage hosted orbit authored truth through the canonical hyard user surface.\n" +
			"This surface exposes the stable authored-truth commands without dropping users back into\n" +
			"the larger compatibility-oriented orbit command tree.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newOrbitCreateCommand(),
		newOrbitRenameCommand(),
		orbitcommands.NewListCommand(),
		orbitcommands.NewShowCommand(),
		orbitcommands.NewFilesCommand(),
		orbitcommands.NewSetCommand(),
		orbitcommands.NewValidateCommand(),
		newOrbitPrepareCommand(),
		newOrbitCheckpointCommand(),
		newOrbitContentCommand(),
		newOrbitMemberCommand(),
		newOrbitSkillCommand(),
		newOrbitAgentCommand(),
	)

	return cmd
}

func newOrbitRenameCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "rename <old-package> <new-package>",
		Short: "Rename a hosted orbit package",
		Long: "Rename a hosted orbit package in the current workspace.\n" +
			"The command updates the hosted OrbitSpec filename, package identity, meta.file,\n" +
			"and the current source/orbit-template manifest when it points at the renamed package.",
		Example: "" +
			"  hyard orbit rename docs api\n" +
			"  hyard orbit rename docs api --json\n",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}

			result, err := packageops.RenameHostedOrbitPackage(cmd.Context(), repo.Root, args[0], args[1])
			if err != nil {
				return fmt.Errorf("rename orbit package: %w", err)
			}

			if jsonOutput {
				return emitHyardJSON(cmd, result)
			}
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"renamed orbit package %s to %s\nold_definition: %s\nnew_definition: %s\nmanifest_changed: %t\n",
				result.OldPackage,
				result.NewPackage,
				result.OldDefinitionPath,
				result.NewDefinitionPath,
				result.ManifestChanged,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output machine-readable JSON")

	return cmd
}

func newOrbitCreateCommand() *cobra.Command {
	cmd := orbitcommands.NewAddCommand()
	originalRunE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if err := originalRunE(cmd, args); err != nil {
			return err
		}
		if len(args) != 1 {
			return nil
		}
		if err := activateRuntimeLocalOrbit(cmd, args[0]); err != nil {
			return err
		}

		return nil
	}

	return cmd
}

func activateRuntimeLocalOrbit(cmd *cobra.Command, orbitID string) error {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return err
	}

	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err != nil {
		return fmt.Errorf("discover git repository for runtime orbit activation: %w", err)
	}

	manifest, err := harnesspkg.LoadManifestFile(repo.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load harness manifest for runtime orbit activation: %w", err)
	}
	if manifest.Kind != harnesspkg.ManifestKindRuntime {
		return nil
	}

	runtimeFile, err := harnesspkg.RuntimeFileFromManifestFile(manifest)
	if err != nil {
		return fmt.Errorf("load runtime manifest for orbit activation: %w", err)
	}
	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			return nil
		}
	}

	if _, err := harnesspkg.AddManualMember(cmd.Context(), repo.Root, orbitID, time.Now().UTC()); err != nil {
		return fmt.Errorf("activate runtime orbit %q: %w", orbitID, err)
	}

	return nil
}

func newOrbitContentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "content",
		Short: "Manage authored orbit content/assets",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newOrbitContentApplyCommand(),
	)

	return cmd
}

func newOrbitContentApplyCommand() *cobra.Command {
	cmd := orbitcommands.NewMemberBackfillCommand()
	originalRunE := cmd.RunE
	cmd.Use = "apply [package]"
	cmd.Short = "Apply tracked content hints into authored package truth"
	cmd.Long = "Apply tracked content hints into authored package truth through the hyard user surface.\n" +
		"This command wraps the existing hint backfill lane without exposing the lower-level member vocabulary."
	cmd.Example = "" +
		"  hyard orbit content apply docs\n" +
		"  hyard orbit content apply docs --check\n" +
		"  hyard orbit content apply docs --json\n"
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			if err := bindOrbitContentApplyPackage(cmd, args[0]); err != nil {
				return err
			}
		}
		return originalRunE(cmd, args)
	}

	return cmd
}

func bindOrbitContentApplyPackage(cmd *cobra.Command, packageName string) error {
	if cmd.Flags().Changed("orbit") {
		orbitID, err := cmd.Flags().GetString("orbit")
		if err != nil {
			return fmt.Errorf("read --orbit flag: %w", err)
		}
		if orbitID != "" && orbitID != packageName {
			return fmt.Errorf("package target %q conflicts with --orbit %q", packageName, orbitID)
		}
		return nil
	}

	return setFlagString(cmd, "orbit", packageName)
}

func newOrbitMemberCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage authored orbit content/assets",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		orbitcommands.NewMemberAddCommand(),
		newOrbitMemberApplyCommand(),
		orbitcommands.NewMemberRemoveCommand(),
	)

	return cmd
}

func newOrbitMemberApplyCommand() *cobra.Command {
	cmd := orbitcommands.NewMemberBackfillCommand()
	cmd.Use = "apply"
	cmd.Short = "Apply tracked member hints into authored member truth"
	cmd.Long = "Apply tracked member hints into authored member truth through the hyard user surface.\n" +
		"This command wraps the existing member-hint backfill lane and keeps detect-style diagnostics\n" +
		"out of the headline authoring path."
	cmd.Example = "" +
		"  hyard orbit member apply\n" +
		"  hyard orbit member apply --orbit docs --check\n" +
		"  hyard orbit member apply --orbit docs --json\n"

	return cmd
}
