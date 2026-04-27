package commands

import (
	"context"
	"fmt"
	"io"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type readinessSummaryJSON struct {
	Status  harnesspkg.ReadinessStatus  `json:"status"`
	Summary harnesspkg.ReadinessSummary `json:"summary"`
	Hint    string                      `json:"hint,omitempty"`
}

func evaluateCommandReadiness(ctx context.Context, repoRoot string) (harnesspkg.ReadinessReport, error) {
	report, err := harnesspkg.EvaluateRuntimeReadiness(ctx, repoRoot)
	if err != nil {
		return harnesspkg.ReadinessReport{}, fmt.Errorf("evaluate harness readiness: %w", err)
	}

	return report, nil
}

func summarizeReadiness(report harnesspkg.ReadinessReport) readinessSummaryJSON {
	summary := readinessSummaryJSON{
		Status:  report.Status,
		Summary: report.Summary,
	}
	if report.Status != harnesspkg.ReadinessStatusReady {
		summary.Hint = "run `hyard ready` for detailed readiness reasons"
	}

	return summary
}

func emitReadinessSummaryText(writer io.Writer, report harnesspkg.ReadinessReport, includeHint bool) error {
	if _, err := fmt.Fprintf(writer, "readiness_status: %s\n", report.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "readiness_orbit_count: %d\n", report.Summary.OrbitCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "readiness_blocking_reason_count: %d\n", report.Summary.BlockingReasonCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "readiness_advisory_reason_count: %d\n", report.Summary.AdvisoryReasonCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if includeHint && report.Status != harnesspkg.ReadinessStatusReady {
		if _, err := fmt.Fprintln(writer, "readiness_hint: run `hyard ready` for detailed readiness reasons"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitPostActionReadinessText(writer io.Writer, report harnesspkg.ReadinessReport) error {
	if _, err := fmt.Fprintf(writer, "readiness_status: %s\n", report.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if report.Status != harnesspkg.ReadinessStatusReady {
		if _, err := fmt.Fprintln(writer, "readiness_hint: run `hyard ready` for detailed readiness reasons"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, step := range report.NextSteps {
		if _, err := fmt.Fprintf(writer, "next_step: %s intent=%s\n", step.Command, step.Intent); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}
