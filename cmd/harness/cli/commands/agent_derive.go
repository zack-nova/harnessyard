package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type agentDeriveOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.AgentDeriveResult
}

// NewAgentDeriveCommand creates the harness agent derive command.
func NewAgentDeriveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "derive",
		Short: "Derive runtime agent truth from installed harness package truth",
		Long: "Derive versioned runtime agent truth under .harness/agents/* from the active\n" +
			"harness package truth snapshots currently represented in this runtime.",
		Example: "" +
			"  harness agent derive\n" +
			"  harness agent derive --json\n",
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

			result, err := harnesspkg.DeriveAgentTruth(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return fmt.Errorf("derive agent truth: %w", err)
			}

			output := agentDeriveOutput{
				HarnessRoot:       resolved.Repo.Root,
				HarnessID:         resolved.Manifest.Runtime.ID,
				AgentDeriveResult: result,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_root: %s\n", output.HarnessRoot); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", output.HarnessID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if output.RecommendedFramework == "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "recommended_framework: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "recommended_framework: %s\n", output.RecommendedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, path := range output.WrittenPaths {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "written_path: %s\n", path); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, warning := range output.Warnings {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
