package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkCheckOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.FrameworkCheckResult
}

// NewFrameworkCheckCommand creates the harness framework check command.
func NewFrameworkCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check current framework activation, side effects, and operational prerequisites",
		Long: "Check current framework activation ledger state, side effect ownership, and operational prerequisites\n" +
			"such as executable availability and writable output locations.",
		Example: "" +
			"  harness framework check\n" +
			"  harness framework check --json\n",
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

			result, err := harnesspkg.CheckFramework(cmd.Context(), resolved.Repo.Root, resolved.Repo.GitDir)
			if err != nil {
				return fmt.Errorf("check framework activation: %w", err)
			}

			output := frameworkCheckOutput{
				HarnessRoot:          resolved.Repo.Root,
				HarnessID:            resolved.Manifest.Runtime.ID,
				FrameworkCheckResult: result,
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
			if output.Framework == "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "framework: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework: %s\n", output.Framework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "configured: %t\n", output.Configured); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "stale: %t\n", output.Stale); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ok: %t\n", output.OK); err != nil {
				return fmt.Errorf("write command output: %w", err)
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
