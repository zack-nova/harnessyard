package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templateInitSourceResultJSON struct {
	RepoRoot           string              `json:"repo_root"`
	SourceManifestPath string              `json:"source_manifest_path"`
	SourceBranch       string              `json:"source_branch"`
	Package            packageIdentityJSON `json:"package"`
	Changed            bool                `json:"changed"`
}

// NewTemplateInitSourceCommand creates the orbit template init-source command.
func NewTemplateInitSourceCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:   "init-source",
		Short: "Initialize the current branch as a single-orbit source branch",
		Long: "Initialize the current branch as a single-orbit source branch for template authoring.\n" +
			"The command writes .harness/manifest.yaml with kind=source, locks source.package to the single source orbit package,\n" +
			"and fails closed for detached HEAD, template branches, or .harness metadata.",
		Example: "" +
			"  orbit template init-source\n" +
			"  orbit template init-source --json\n",
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
				"initialized template source in %s\nsource_branch: %s\npackage: %s\npackage_type: orbit\nchanged: %t\n",
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
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target orbit package name for the source branch")
	cmd.Flags().StringVar(&orbitName, "name", "", "Create the initial orbit with this display name when needed")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Create the initial orbit with this description when needed")
	cmd.Flags().BoolVar(&withSpec, "with-spec", false, "When creating the initial orbit, also add docs/<orbit-package>.md as a rule member keyed spec")

	return cmd
}
