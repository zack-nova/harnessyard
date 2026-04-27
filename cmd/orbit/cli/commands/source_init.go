package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// NewSourceInitCommand creates the orbit source init command.
func NewSourceInitCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current branch as a single-orbit source branch",
		Long: "Initialize the current Git repo as a single-orbit source branch.\n" +
			"Pass --orbit when creating the first hosted orbit definition. Optional when the current Git repo already contains exactly one hosted orbit definition.",
		Example: "" +
			"  orbit source init --orbit research\n" +
			"  orbit source init --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromAuthoringCommand(
				cmd,
				"source authoring",
				"orbit source create",
				orbitID,
				orbitName,
				orbitDescription,
			)
			if err != nil {
				return err
			}

			result, err := orbittemplate.InitSourceBranchWithInput(cmd.Context(), repo.Root, orbittemplate.InitSourceInput{
				OrbitID:     orbitID,
				Name:        orbitName,
				Description: orbitDescription,
				WithSpec:    withSpec,
			})
			if err != nil {
				return fmt.Errorf("initialize source branch: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), templateInitSourceResultJSON{
					RepoRoot:           result.RepoRoot,
					SourceManifestPath: result.SourceManifestPath,
					SourceBranch:       result.SourceBranch,
					Package:            orbitPackageOutput(result.PublishOrbitID),
					Changed:            result.Changed,
				})
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"initialized source in %s\nsource_branch: %s\npackage: %s\npackage_type: orbit\nchanged: %t\n",
				result.RepoRoot,
				result.SourceBranch,
				result.PublishOrbitID,
				result.Changed,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target orbit package name for the source branch; optional when exactly one hosted orbit definition exists")
	cmd.Flags().StringVar(&orbitName, "name", "", "Create the initial orbit with this display name when needed")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Create the initial orbit with this description when needed")
	cmd.Flags().BoolVar(&withSpec, "with-spec", false, "When creating the initial orbit, also add docs/<orbit-package>.md as a rule member keyed spec")

	return cmd
}
