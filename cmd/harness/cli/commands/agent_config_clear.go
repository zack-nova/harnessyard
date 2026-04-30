package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type agentConfigClearOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.AgentConfigClearResult
}

// NewAgentConfigClearCommand creates the harness agent config clear command.
func NewAgentConfigClearCommand() *cobra.Command {
	var target string
	var sidecars bool
	var all bool

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear versioned agent configuration truth",
		Long: "Clear versioned agent configuration truth while preserving agent-specific sidecars by default.\n" +
			"Use --sidecars or --all to remove sidecar files.",
		Example: "" +
			"  harness agent config clear --target codex\n" +
			"  harness agent config clear --target codex --sidecars\n" +
			"  harness agent config clear --all\n",
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

			result, err := harnesspkg.ClearAgentConfig(harnesspkg.AgentConfigClearOptions{
				RepoRoot:       resolved.Repo.Root,
				Target:         target,
				RemoveSidecars: sidecars,
				All:            all,
			})
			if err != nil {
				return fmt.Errorf("clear agent config: %w", err)
			}
			if err := emitAgentConfigClearWarnings(cmd, result.Warnings); err != nil {
				return err
			}

			output := agentConfigClearOutput{
				HarnessRoot:            resolved.Repo.Root,
				HarnessID:              resolved.Manifest.Runtime.ID,
				AgentConfigClearResult: result,
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
			for _, clearedTarget := range output.ClearedTargets {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "cleared_target: %s\n", clearedTarget); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, removedSidecar := range output.RemovedSidecars {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed_sidecar: %s\n", removedSidecar); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)
	cmd.Flags().StringVar(&target, "target", "", "Clear one agent config target")
	cmd.Flags().BoolVar(&sidecars, "sidecars", false, "Also remove sidecar files for cleared targets")
	cmd.Flags().BoolVar(&all, "all", false, "Clear all agent config targets, hooks, unified config, and sidecar files")

	return cmd
}

func emitAgentConfigClearWarnings(cmd *cobra.Command, warnings []string) error {
	for _, warning := range warnings {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning); err != nil {
			return fmt.Errorf("write command warning: %w", err)
		}
	}

	return nil
}
