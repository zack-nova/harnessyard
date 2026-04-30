package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkRemoveOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.FrameworkRemoveResult
}

// NewFrameworkRemoveCommand creates the harness framework remove command.
func NewFrameworkRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove framework activation side effects still owned by the current runtime",
		Long: "Remove framework activation owned side effects for the current runtime and clear\n" +
			"their repo-local activation ledger entries without deleting files that have been manually taken over.",
		Example: "" +
			"  harness framework remove\n" +
			"  harness framework remove --json\n",
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

			result, err := harnesspkg.RemoveFrameworkActivations(cmd.Context(), resolved.Repo.Root, resolved.Repo.GitDir)
			if err != nil {
				return fmt.Errorf("remove framework activations: %w", err)
			}
			for _, warning := range result.Warnings {
				if !strings.Contains(warning, "global agent config") {
					continue
				}
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning); err != nil {
					return fmt.Errorf("write command warning: %w", err)
				}
			}

			output := frameworkRemoveOutput{
				HarnessRoot:           resolved.Repo.Root,
				HarnessID:             resolved.Manifest.Runtime.ID,
				FrameworkRemoveResult: result,
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
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed_activation_count: %d\n", output.RemovedActivationCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed_output_count: %d\n", output.RemovedOutputCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skipped_output_count: %d\n", output.SkippedOutputCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
