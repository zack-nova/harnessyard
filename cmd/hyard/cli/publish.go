package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// These legacy automation/migration flags remain part of the supported
// hyard publish orbit compatibility contract, but stay off the headline help.
var publishOrbitHiddenCompatibilityFlags = []string{
	"orbit",
	"backfill-brief",
	"aggregate-detected-skills",
	"allow-out-of-range-skills",
}

func newPublishCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish installable orbit or harness results",
		Long: "Publish installable orbit or harness results through the canonical hyard user surface.\n" +
			"Use `publish orbit` for authored single-orbit template publication and `publish harness`\n" +
			"for runtime-as-template publication.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newPublishOrbitCommand(),
		newPublishHarnessCommand(),
	)

	return cmd
}

func newPublishOrbitCommand() *cobra.Command {
	var checkpoint bool
	var checkpointMessage string
	var prepare bool
	var trackNew bool
	var yes bool

	cmd := orbitcommands.NewTemplatePublishCommand()
	originalRunE := cmd.RunE
	cmd.Use = "orbit [package]"
	cmd.Short = "Publish the current orbit source or template revision"
	cmd.Long = "Publish the current orbit source or template revision through the hyard user surface.\n" +
		"This command wraps the existing orbit template publish lane.\n" +
		"Legacy automation and migration compatibility flags remain supported, but stay hidden\n" +
		"from the default help surface."
	cmd.Example = "" +
		"  hyard publish orbit docs\n" +
		"  hyard publish orbit docs --prepare --checkpoint -m \"Update docs\"\n" +
		"  hyard publish orbit docs@0.1.0\n" +
		"  hyard publish orbit docs --default\n" +
		"  hyard publish orbit docs --push --remote origin\n" +
		"  hyard publish orbit docs --json\n"
	cmd.Args = cobra.MaximumNArgs(1)
	for _, flagName := range publishOrbitHiddenCompatibilityFlags {
		mustMarkFlagHidden(cmd, flagName)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		packageName := ""
		if len(args) == 1 {
			coordinate, err := parseHyardPackageCoordinate(args[0])
			if err != nil {
				return err
			}
			packageName = coordinate.Name
			if err := bindPublishOrbitPackage(cmd, coordinate); err != nil {
				return err
			}
		}
		if strings.TrimSpace(checkpointMessage) != "" && !checkpoint {
			return fmt.Errorf("-m/--message can only be used with --checkpoint")
		}
		if trackNew && !checkpoint {
			return fmt.Errorf("--track-new can only be used with --checkpoint")
		}
		if yes && !prepare && !checkpoint {
			return fmt.Errorf("--yes can only be used with --prepare or --checkpoint")
		}
		if checkpoint && strings.TrimSpace(checkpointMessage) == "" {
			return fmt.Errorf("publish orbit --checkpoint requires -m/--message")
		}
		if (prepare || checkpoint || trackNew) && packageName == "" {
			return fmt.Errorf("publish orbit --prepare/--checkpoint/--track-new requires a package argument")
		}
		jsonOutput, err := wantHyardJSON(cmd)
		if err != nil {
			return err
		}
		if !prepare && !checkpoint && packageName != "" {
			if jsonOutput {
				readiness, err := buildHyardOrbitPrepareOutput(cmd.Context(), cmd, packageName)
				if err != nil {
					return err
				}
				if !readiness.Ready {
					if err := emitHyardJSON(cmd, readiness); err != nil {
						return err
					}
					actions := append([]string{fmt.Sprintf("hyard orbit prepare %s --check --json", packageName)}, readiness.NextActions...)
					return fmt.Errorf("publish orbit %q is not ready; next actions: %s", packageName, strings.Join(actions, "; "))
				}
			} else if shouldOfferHyardPublishInteractiveRecovery(cmd, jsonOutput) {
				decision, err := runHyardPublishInteractiveRecovery(cmd.Context(), cmd, packageName)
				if err != nil {
					return err
				}
				if decision == hyardPublishRecoveryStop {
					return nil
				}
			}
		}
		if prepare || checkpoint {
			if err := runHyardPublishPrepareCheckpointFlow(cmd.Context(), cmd, packageName, prepare, checkpoint, trackNew, checkpointMessage); err != nil {
				return err
			}
		}
		if err := originalRunE(cmd, args); err != nil {
			return rewriteHyardPublishOrbitError(err, packageName)
		}
		return nil
	}
	cmd.Flags().BoolVar(&prepare, "prepare", false, "Apply deterministic package readiness fixes before publishing")
	cmd.Flags().BoolVar(&trackNew, "track-new", false, "Track safe new package files before checkpointing")
	cmd.Flags().BoolVar(&checkpoint, "checkpoint", false, "Commit package-relevant authoring changes before publishing")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm explicit prepare/checkpoint mutations in scripted publish flows")
	cmd.Flags().StringVarP(&checkpointMessage, "message", "m", "", "Checkpoint commit message")
	return cmd
}

func newPublishHarnessCommand() *cobra.Command {
	cmd := harnesscommands.NewTemplatePublishCommand()
	originalRunE := cmd.RunE
	cmd.Use = "harness [package]"
	cmd.Short = "Publish the current runtime as a harness template branch"
	cmd.Long = "Publish the current runtime as a harness template branch through the hyard user surface.\n" +
		"This command wraps the existing harness template publish lane."
	cmd.Example = "" +
		"  hyard publish harness frontend-lab\n" +
		"  hyard publish harness frontend-lab@0.1.0\n" +
		"  hyard publish harness workspace\n" +
		"  hyard publish harness workspace --default\n" +
		"  hyard publish harness workspace --push --remote origin\n" +
		"  hyard publish harness workspace --json\n"
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			coordinate, err := parseHyardPackageCoordinate(args[0])
			if err != nil {
				return err
			}
			if err := bindPublishHarnessPackage(cmd, coordinate); err != nil {
				return err
			}
		}
		return originalRunE(cmd, args)
	}
	return cmd
}

func bindPublishOrbitPackage(cmd *cobra.Command, coordinate ids.PackageCoordinate) error {
	if cmd.Flags().Changed("orbit") {
		orbitID, err := cmd.Flags().GetString("orbit")
		if err != nil {
			return fmt.Errorf("read --orbit flag: %w", err)
		}
		if orbitID != "" && orbitID != coordinate.Name {
			return fmt.Errorf("package target %q conflicts with --orbit %q", coordinate.Name, orbitID)
		}
	} else if err := setFlagString(cmd, "orbit", coordinate.Name); err != nil {
		return err
	}
	metadata := packageMetadataFromCoordinate(coordinate)
	if coordinate.Kind == ids.PackageCoordinateGitLocator {
		locator := normalizePackageGitLocatorRef(coordinate.Locator)
		if err := setFlagString(cmd, "target-branch", locator); err != nil {
			return err
		}
		metadata = packageMetadataFromCoordinateWithLocator(coordinate, locator)
	}

	return bindPackageMetadata(cmd, metadata)
}

func bindPublishHarnessPackage(cmd *cobra.Command, coordinate ids.PackageCoordinate) error {
	if cmd.Flags().Changed("to") {
		return fmt.Errorf("package target %q cannot be combined with --to; use one publish target form", coordinate.String())
	}
	targetBranch := "harness-template/" + coordinate.Name
	metadata := packageMetadataFromCoordinate(coordinate)
	if coordinate.Kind == ids.PackageCoordinateGitLocator {
		targetBranch = normalizePackageGitLocatorRef(coordinate.Locator)
		metadata = packageMetadataFromCoordinateWithLocator(coordinate, targetBranch)
	}
	if err := setFlagString(cmd, "to", targetBranch); err != nil {
		return err
	}

	return bindPackageMetadata(cmd, metadata)
}

func rewriteHyardPublishOrbitError(err error, packageName string) error {
	message := err.Error()
	message = strings.ReplaceAll(
		message,
		"inspect with `orbit member detect --orbit ",
		"inspect with `hyard orbit content apply ",
	)
	message = strings.ReplaceAll(
		message,
		" --json`, resolve the reported hint diagnostics, then run `orbit member backfill --orbit ",
		" --check --json`, resolve the reported hint diagnostics, then run `hyard orbit content apply ",
	)
	message = strings.ReplaceAll(
		message,
		"run `orbit member backfill --orbit ",
		"run `hyard orbit content apply ",
	)
	if packageName != "" && strings.Contains(message, "publish requires a clean tracked worktree") {
		message = message + fmt.Sprintf(
			"; inspect readiness with `hyard orbit prepare %s --check --json` or checkpoint package changes with `hyard orbit checkpoint %s -m %q`",
			packageName,
			packageName,
			"Update "+packageName,
		)
	}

	if message == err.Error() {
		return err
	}

	return fmt.Errorf("%s", message)
}
