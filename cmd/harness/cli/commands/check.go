package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type checkOutput struct {
	HarnessRoot     string                           `json:"harness_root"`
	HarnessID       string                           `json:"harness_id,omitempty"`
	OK              bool                             `json:"ok"`
	FindingCount    int                              `json:"finding_count"`
	Findings        []harnesspkg.CheckFinding        `json:"findings"`
	BindingsSummary *harnesspkg.CheckBindingsSummary `json:"bindings_summary,omitempty"`
	Readiness       harnesspkg.ReadinessReport       `json:"readiness"`
}

// NewCheckCommand creates the harness check command.
func NewCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Diagnose harness runtime schema, membership, and install consistency",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			repo, err := gitpkg.DiscoverRepo(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}

			result, err := harnesspkg.CheckRuntimeWithProgress(cmd.Context(), repo.Root, progress.Stage)
			if err != nil {
				return fmt.Errorf("check harness runtime: %w", err)
			}

			output := checkOutput{
				HarnessRoot:     repo.Root,
				HarnessID:       result.HarnessID,
				OK:              result.OK,
				FindingCount:    result.FindingCount,
				Findings:        result.Findings,
				BindingsSummary: result.BindingsSummary,
			}
			readiness, err := evaluateCommandReadiness(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}
			output.Readiness = readiness

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
			if err := emitReadinessSummaryText(cmd.OutOrStdout(), output.Readiness, false); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ok: %t\n", output.OK); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "finding_count: %d\n", output.FindingCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if output.BindingsSummary != nil {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), unresolvedBindingsSummaryLine(output.BindingsSummary)); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if len(output.Findings) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "findings: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}

			for _, finding := range output.Findings {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"finding: %s orbit_id=%s path=%s message=%s\n",
					finding.Kind,
					emptyTextFallback(finding.OrbitID),
					emptyTextFallback(finding.Path),
					normalizeFindingMessage(finding.Message),
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

func unresolvedBindingsSummaryLine(summary *harnesspkg.CheckBindingsSummary) string {
	line := fmt.Sprintf(
		"unresolved_bindings: installs=%d variables=%d orbit_ids=%s",
		summary.UnresolvedInstallCount,
		summary.UnresolvedVariableCount,
		strings.Join(summary.OrbitIDs, ","),
	)
	if len(summary.BundleIDs) > 0 {
		line += fmt.Sprintf(" bundle_ids=%s", strings.Join(summary.BundleIDs, ","))
	}
	return line
}

func emptyTextFallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}

	return value
}

func normalizeFindingMessage(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
}
