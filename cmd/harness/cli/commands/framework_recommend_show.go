package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkRecommendShowOutput struct {
	HarnessRoot          string `json:"harness_root"`
	HarnessID            string `json:"harness_id"`
	FrameworksPath       string `json:"frameworks_path"`
	RecommendedFramework string `json:"recommended_framework,omitempty"`
}

// NewFrameworkRecommendShowCommand creates the harness framework recommend show command.
func NewFrameworkRecommendShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the runtime's recommended framework truth",
		Example: "" +
			"  harness framework recommend show\n" +
			"  harness framework recommend show --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			file, err := harnesspkg.LoadOptionalFrameworksFile(resolved.Repo.Root)
			if err != nil {
				return fmt.Errorf("load frameworks file: %w", err)
			}

			output := frameworkRecommendShowOutput{
				HarnessRoot:          resolved.Repo.Root,
				HarnessID:            resolved.Manifest.Runtime.ID,
				FrameworksPath:       harnesspkg.FrameworksPath(resolved.Repo.Root),
				RecommendedFramework: file.RecommendedFramework,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if output.RecommendedFramework == "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "recommended framework: none\n"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "recommended framework: %s\n", output.RecommendedFramework); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
