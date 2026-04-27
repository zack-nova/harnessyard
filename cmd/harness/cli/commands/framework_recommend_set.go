package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkRecommendSetOutput struct {
	HarnessRoot          string `json:"harness_root"`
	HarnessID            string `json:"harness_id"`
	FrameworksPath       string `json:"frameworks_path"`
	RecommendedFramework string `json:"recommended_framework"`
}

// NewFrameworkRecommendSetCommand creates the harness framework recommend set command.
func NewFrameworkRecommendSetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <framework>",
		Short: "Set the runtime's recommended framework",
		Long: "Write the runtime's versioned framework recommendation truth to .harness/agents/manifest.yaml.\n" +
			"This does not change the current machine's local framework selection.",
		Example: "" +
			"  harness framework recommend set codex\n" +
			"  harness framework recommend set claude --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			frameworkID := args[0]
			if _, ok := harnesspkg.LookupFrameworkAdapter(frameworkID); !ok {
				return fmt.Errorf("framework %q is not supported by this build", frameworkID)
			}

			filename, err := harnesspkg.WriteFrameworksFile(resolved.Repo.Root, harnesspkg.FrameworksFile{
				SchemaVersion:        1,
				RecommendedFramework: frameworkID,
			})
			if err != nil {
				return fmt.Errorf("write frameworks file: %w", err)
			}

			output := frameworkRecommendSetOutput{
				HarnessRoot:          resolved.Repo.Root,
				HarnessID:            resolved.Manifest.Runtime.ID,
				FrameworksPath:       filename,
				RecommendedFramework: frameworkID,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "recommended framework %s for harness %s\n", frameworkID, resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
