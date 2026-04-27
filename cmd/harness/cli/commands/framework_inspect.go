package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkInspectOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.FrameworkInspectSummary
}

// NewFrameworkInspectCommand creates the harness framework inspect command.
func NewFrameworkInspectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect framework truth, current resolution, guidance, and capabilities",
		Long: "Inspect the current runtime's recommended framework, resolved framework, guidance presence,\n" +
			"and orbit-scoped commands/skills summary.",
		Example: "" +
			"  harness framework inspect\n" +
			"  harness framework inspect --json\n",
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

			summary, err := harnesspkg.BuildFrameworkInspectSummary(cmd.Context(), resolved.Repo.Root, resolved.Repo.GitDir)
			if err != nil {
				return fmt.Errorf("inspect framework state: %w", err)
			}

			output := frameworkInspectOutput{
				HarnessRoot:             resolved.Repo.Root,
				HarnessID:               resolved.Manifest.Runtime.ID,
				FrameworkInspectSummary: summary,
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
			if output.ResolvedFramework == "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "resolved_framework: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved_framework: %s\n", output.ResolvedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolution_source: %s\n", output.ResolutionSource); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_count: %d\n", output.OrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "command_count: %d\n", output.CommandCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skill_count: %d\n", output.SkillCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote_skill_count: %d\n", output.RemoteSkillCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_agent_hook_count: %d\n", output.PackageAgentHookCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "has_agent_guidance: %t\n", output.HasAgentGuidance); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "has_human_guidance: %t\n", output.HasHumanGuidance); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, recommendation := range output.PackageRecommendations {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_recommendation: %s=%s\n", recommendation.HarnessID, recommendation.RecommendedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
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

	return cmd
}
