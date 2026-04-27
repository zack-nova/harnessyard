package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type rootOutput struct {
	HarnessRoot string `json:"harness_root"`
}

// NewRootPathCommand creates the harness root command.
func NewRootPathCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "root",
		Short: "Print the harness root for the current runtime",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), rootOutput{HarnessRoot: resolved.Repo.Root})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
