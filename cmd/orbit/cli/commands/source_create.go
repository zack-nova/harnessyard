package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type sourceCreateResultJSON struct {
	RepoRoot           string              `json:"repo_root"`
	SourceManifestPath string              `json:"source_manifest_path"`
	SourceBranch       string              `json:"source_branch"`
	Package            packageIdentityJSON `json:"package"`
	GitInitialized     bool                `json:"git_initialized"`
	Changed            bool                `json:"changed"`
}

// NewSourceCreateCommand creates the orbit source create command.
func NewSourceCreateCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:   "create <path>",
		Short: "Create a new source authoring repository",
		Long: "Create a new source authoring repository from an empty directory, a non-Git directory,\n" +
			"or an existing plain Git repository. The command initializes a Git repo when needed and then\n" +
			"bootstraps the target repo as a single-orbit source branch.",
		Example: "" +
			"  orbit source create ./research-source --orbit research\n" +
			"  orbit source create ./research-source --orbit research --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := absolutePathFromArg(cmd, args[0])
			if err != nil {
				return err
			}

			result, err := orbittemplate.CreateSourceRepo(cmd.Context(), orbittemplate.CreateSourceInput{
				TargetPath:  targetPath,
				OrbitID:     orbitID,
				Name:        orbitName,
				Description: orbitDescription,
				WithSpec:    withSpec,
			})
			if err != nil {
				return fmt.Errorf("create source authoring repo: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), sourceCreateResultJSON{
					RepoRoot:           result.RepoRoot,
					SourceManifestPath: result.SourceManifestPath,
					SourceBranch:       result.SourceBranch,
					Package:            orbitPackageOutput(result.PublishOrbitID),
					GitInitialized:     result.GitInitialized,
					Changed:            result.Changed,
				})
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"created source authoring repo in %s\nsource_branch: %s\npackage: %s\npackage_type: orbit\ngit_initialized: %t\nchanged: %t\n",
				result.RepoRoot,
				result.SourceBranch,
				result.PublishOrbitID,
				result.GitInitialized,
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
