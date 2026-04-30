package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkPlanOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.FrameworkPlan
}

// NewFrameworkPlanCommand creates the harness framework plan command.
func NewFrameworkPlanCommand() *cobra.Command {
	var hooks bool

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview project and global framework activation outputs",
		Long: "Preview the current runtime's framework activation plan, including desired truth,\n" +
			"project materialization plan, and global registration plan.",
		Example: "" +
			"  harness framework plan\n" +
			"  harness framework plan --json\n",
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

			plan, err := harnesspkg.BuildFrameworkPlan(cmd.Context(), resolved.Repo.Root, resolved.Repo.GitDir, resolved.Manifest.Runtime.ID)
			if err != nil {
				return fmt.Errorf("build framework plan: %w", err)
			}

			output := frameworkPlanOutput{
				HarnessRoot:   resolved.Repo.Root,
				HarnessID:     resolved.Manifest.Runtime.ID,
				FrameworkPlan: plan,
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
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolution_source: %s\n", output.ResolutionSource); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_agent_hook_count: %d\n", output.DesiredTruth.PackageAgentHookCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, hook := range output.PackageAgentHooks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_agent_hook: %s handler=%s activation=%s\n", hook.DisplayID, hook.HandlerPath, hook.Activation); err != nil {
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
	cmd.Flags().BoolVar(&hooks, "hooks", false, "Include hook activation preview outputs")

	return cmd
}
