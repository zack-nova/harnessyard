package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type readyOutput struct {
	HarnessRoot string `json:"harness_root"`
	harnesspkg.ReadinessReport
}

func readinessIssueLabel(severity harnesspkg.ReadinessReasonSeverity, orbitScoped bool) string {
	switch severity {
	case harnesspkg.ReadinessReasonSeverityBlocking:
		if orbitScoped {
			return "orbit_blocking_issue"
		}
		return "blocking_issue"
	default:
		if orbitScoped {
			return "orbit_warning"
		}
		return "warning"
	}
}

func formatRuntimeReadinessIssue(reason harnesspkg.ReadinessReason) string {
	details := []string{fmt.Sprintf("code=%s", reason.Code)}
	if len(reason.OrbitIDs) > 0 {
		details = append([]string{fmt.Sprintf("orbit_ids=%s", strings.Join(reason.OrbitIDs, ","))}, details...)
	}
	return fmt.Sprintf("%s: %s (%s)", readinessIssueLabel(reason.Severity, false), reason.Message, strings.Join(details, ", "))
}

func formatOrbitReadinessIssue(orbitID string, reason harnesspkg.ReadinessReason) string {
	return fmt.Sprintf(
		"%s: %s %s (code=%s)",
		readinessIssueLabel(reason.Severity, true),
		orbitID,
		reason.Message,
		reason.Code,
	)
}

func formatSuggestedCommand(step harnesspkg.ReadinessNextStep) string {
	if strings.TrimSpace(step.Intent) == "" {
		return fmt.Sprintf("suggested_command: %s", step.Command)
	}
	return fmt.Sprintf("suggested_command: %s (%s)", step.Command, step.Intent)
}

// NewReadyCommand creates the harness ready command.
func NewReadyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Explain whether the current runtime is broken, usable, or ready",
		Long: "Explain the current harness runtime readiness using the frozen v0.5 readiness lane.\n" +
			"The command is read-only and summarizes whether the runtime is currently in a broken / usable / ready state,\n" +
			"plus the current readiness issues, per-orbit status, and suggested commands.",
		Example: "" +
			"  harness ready\n" +
			"  harness ready --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			repo, err := gitpkg.DiscoverRepo(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}

			report, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("evaluate harness readiness: %w", err)
			}

			output := readyOutput{
				HarnessRoot:     repo.Root,
				ReadinessReport: report,
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
			if output.HarnessID != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", output.HarnessID); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", output.Status); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "runtime_status: %s\n", output.Runtime.Status); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent_status: %s\n", output.Agent.Status); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent_activation: %s\n", output.Agent.ActivationStatus); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if output.Agent.ResolvedAgent != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent_resolved: %s\n", output.Agent.ResolvedAgent); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, warning := range output.Agent.Warnings {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_count: %d\n", output.Summary.OrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ready_orbit_count: %d\n", output.Summary.ReadyOrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "usable_orbit_count: %d\n", output.Summary.UsableOrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "broken_orbit_count: %d\n", output.Summary.BrokenOrbitCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			for _, reason := range output.RuntimeReasons {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatRuntimeReadinessIssue(reason)); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			for _, orbitReport := range output.OrbitReports {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"orbit: %s source=%s status=%s\n",
					orbitReport.OrbitID,
					orbitReport.MemberSource,
					orbitReport.Status,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				for _, reason := range orbitReport.Reasons {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatOrbitReadinessIssue(orbitReport.OrbitID, reason)); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}
			}

			for _, step := range output.NextSteps {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatSuggestedCommand(step)); err != nil {
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
