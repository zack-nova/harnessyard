package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
)

func newCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new runtime or authoring repository",
		Long: "Create a new runtime or authoring repository through the canonical hyard user surface.\n" +
			"Use `create runtime` to bootstrap a runtime repo, `create source` to bootstrap a source\n" +
			"authoring repo, and `create orbit-template` to bootstrap an orbit_template authoring repo.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch len(args) {
			case 0:
				return cmd.Help()
			case 1:
				return fmt.Errorf(
					"hyard create <path> is no longer supported; use `hyard create runtime %s`",
					args[0],
				)
			default:
				return fmt.Errorf(
					"hyard create requires an explicit kind; use `hyard create <runtime|source|orbit-template> ...`",
				)
			}
		},
	}

	cmd.AddCommand(
		newCreateRuntimeCommand(),
		newCreateSourceCommand(),
		newCreateOrbitTemplateCommand(),
	)

	return cmd
}

func newCreateRuntimeCommand() *cobra.Command {
	cmd := harnesscommands.NewCreateCommand()
	cmd.Use = "runtime <path>"
	cmd.Example = "" +
		"  hyard create runtime demo-repo\n" +
		"  hyard create runtime ./workspaces/demo-repo --json\n"
	return cmd
}

func newCreateSourceCommand() *cobra.Command {
	cmd := orbitcommands.NewSourceCreateCommand()
	cmd.Use = "source <path>"
	cmd.Example = "" +
		"  hyard create source ./research-source --orbit research\n" +
		"  hyard create source ./research-source --orbit research --json\n"
	return cmd
}

func newCreateOrbitTemplateCommand() *cobra.Command {
	cmd := orbitcommands.NewTemplateCreateCommand()
	cmd.Use = "orbit-template <path>"
	cmd.Example = "" +
		"  hyard create orbit-template ./research-template --orbit research\n" +
		"  hyard create orbit-template ./research-template --orbit research --json\n"
	return cmd
}

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current Git repo as a runtime or authoring revision",
		Long: "Initialize the current Git repo as a runtime or authoring revision through the canonical hyard user surface.\n" +
			"Use `init runtime` for runtime metadata, `init source` for source authoring, and\n" +
			"`init orbit-template` for orbit_template authoring.",
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("hyard init no longer defaults to runtime; use `hyard init runtime`")
			}

			return fmt.Errorf(
				"hyard init requires an explicit kind; use `hyard init <runtime|source|orbit-template>`",
			)
		},
	}

	cmd.AddCommand(
		newInitRuntimeCommand(),
		newInitSourceCommand(),
		newInitOrbitTemplateCommand(),
	)

	return cmd
}

func newInitRuntimeCommand() *cobra.Command {
	cmd := harnesscommands.NewInitCommand()
	cmd.Use = "runtime"
	cmd.Example = "" +
		"  hyard init runtime\n" +
		"  hyard init runtime --path ../existing-runtime --json\n"
	return cmd
}

func newInitSourceCommand() *cobra.Command {
	cmd := orbitcommands.NewSourceInitCommand()
	cmd.Use = "source"
	cmd.Example = "" +
		"  hyard init source --orbit research\n" +
		"  hyard init source --json\n" +
		"  hyard init source --orbit research --json\n"
	return cmd
}

func newInitOrbitTemplateCommand() *cobra.Command {
	cmd := orbitcommands.NewTemplateInitCommand()
	cmd.Use = "orbit-template"
	cmd.Example = "" +
		"  hyard init orbit-template --orbit research\n" +
		"  hyard init orbit-template --json\n" +
		"  hyard init orbit-template --orbit research --json\n"
	return cmd
}
