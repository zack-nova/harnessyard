package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templateCreateResultJSON struct {
	RepoRoot       string              `json:"repo_root"`
	ManifestPath   string              `json:"manifest_path"`
	CurrentBranch  string              `json:"current_branch"`
	Package        packageIdentityJSON `json:"package"`
	GitInitialized bool                `json:"git_initialized"`
	Changed        bool                `json:"changed"`
}

// NewTemplateCreateCommand creates the orbit template create command.
func NewTemplateCreateCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:   "create <path>",
		Short: "Create a new orbit template authoring repository",
		Long: "Create a new orbit template authoring repository from an empty directory, a non-Git directory,\n" +
			"or an existing plain Git repository. The command initializes a Git repo when needed and then\n" +
			"bootstraps the target repo as a single-orbit orbit_template branch.",
		Example: "" +
			"  orbit template create ./research-template --orbit research\n" +
			"  orbit template create ./research-template --orbit research --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := absolutePathFromArg(cmd, args[0])
			if err != nil {
				return err
			}

			result, err := orbittemplate.CreateTemplateRepo(cmd.Context(), orbittemplate.CreateTemplateInput{
				TargetPath:  targetPath,
				OrbitID:     orbitID,
				Name:        orbitName,
				Description: orbitDescription,
				WithSpec:    withSpec,
			})
			if err != nil {
				return fmt.Errorf("create orbit template authoring repo: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), templateCreateResultJSON{
					RepoRoot:       result.RepoRoot,
					ManifestPath:   result.ManifestPath,
					CurrentBranch:  result.CurrentBranch,
					Package:        orbitPackageOutput(result.OrbitID),
					GitInitialized: result.GitInitialized,
					Changed:        result.Changed,
				})
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"created orbit template authoring repo in %s\ncurrent_branch: %s\npackage: %s\npackage_type: orbit\ngit_initialized: %t\nchanged: %t\n",
				result.RepoRoot,
				result.CurrentBranch,
				result.OrbitID,
				result.GitInitialized,
				result.Changed,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target orbit package name for the orbit_template branch")
	cmd.Flags().StringVar(&orbitName, "name", "", "Create the initial orbit with this display name when needed")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Create the initial orbit with this description when needed")
	cmd.Flags().BoolVar(&withSpec, "with-spec", false, "When creating the initial orbit, also add docs/<orbit-package>.md as a rule member keyed spec")

	return cmd
}
