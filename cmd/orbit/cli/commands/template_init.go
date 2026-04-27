package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templateInitResultJSON struct {
	RepoRoot      string              `json:"repo_root"`
	ManifestPath  string              `json:"manifest_path"`
	CurrentBranch string              `json:"current_branch"`
	Package       packageIdentityJSON `json:"package"`
	Changed       bool                `json:"changed"`
}

// NewTemplateInitCommand creates the orbit template init command.
func NewTemplateInitCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the current branch as an orbit_template branch",
		Long: "Initialize the current Git repo as a single-orbit orbit_template branch.\n" +
			"Pass --orbit when creating the first hosted orbit definition. Optional when the current Git repo already contains exactly one hosted orbit definition.",
		Example: "" +
			"  orbit template init --orbit research\n" +
			"  orbit template init --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromAuthoringCommand(
				cmd,
				"orbit template authoring",
				"orbit template create",
				orbitID,
				orbitName,
				orbitDescription,
			)
			if err != nil {
				return err
			}

			result, err := orbittemplate.InitTemplateBranch(cmd.Context(), repo.Root, orbittemplate.InitTemplateInput{
				OrbitID:     orbitID,
				Name:        orbitName,
				Description: orbitDescription,
				WithSpec:    withSpec,
				Now:         time.Now().UTC(),
			})
			if err != nil {
				return fmt.Errorf("initialize orbit template branch: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), templateInitResultJSON{
					RepoRoot:      result.RepoRoot,
					ManifestPath:  result.ManifestPath,
					CurrentBranch: result.CurrentBranch,
					Package:       orbitPackageOutput(result.OrbitID),
					Changed:       result.Changed,
				})
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"initialized orbit template in %s\ncurrent_branch: %s\npackage: %s\npackage_type: orbit\nchanged: %t\n",
				result.RepoRoot,
				result.CurrentBranch,
				result.OrbitID,
				result.Changed,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target orbit package name for the orbit_template branch; optional when exactly one hosted orbit definition exists")
	cmd.Flags().StringVar(&orbitName, "name", "", "Create the initial orbit with this display name when needed")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Create the initial orbit with this description when needed")
	cmd.Flags().BoolVar(&withSpec, "with-spec", false, "When creating the initial orbit, also add docs/<orbit-package>.md as a rule member keyed spec")

	return cmd
}
