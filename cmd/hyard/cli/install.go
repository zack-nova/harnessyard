package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func newInstallCommand() *cobra.Command {
	cmd := harnesscommands.NewInstallCommand()
	originalRunE := cmd.RunE
	cmd.Use = "install <package@git:ref|template-branch|git-source>"
	cmd.Example = "" +
		"  hyard install docs@git:orbit-template/docs\n" +
		"  hyard install orbit-template/docs --bindings .harness/vars.yaml\n" +
		"  hyard install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml\n" +
		"  hyard install orbit-template/docs --overwrite-existing --bindings .harness/vars.yaml --json\n"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && shouldParseHyardPackageCoordinateArg(args[0]) {
			coordinate, err := parseHyardPackageCoordinate(args[0])
			if err != nil {
				return err
			}
			if coordinate.Kind != ids.PackageCoordinateGitLocator {
				return fmt.Errorf("hyard install package coordinate %s is not supported yet; use %s@git:<ref> or the existing explicit source form", coordinate.String(), coordinate.Name)
			}
			if cmd.Flags().Changed("ref") {
				return fmt.Errorf("package coordinate %s cannot be combined with --ref; put the git locator after @git", coordinate.String())
			}
			locator := normalizePackageGitLocatorRef(coordinate.Locator)
			args[0] = locator
			if err := bindPackageMetadata(cmd, packageMetadataFromCoordinateWithLocator(coordinate, locator)); err != nil {
				return err
			}
		}

		return originalRunE(cmd, args)
	}

	return cmd
}
