package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type auditExitError struct {
	status string
	code   int
}

func (err auditExitError) Error() string {
	return fmt.Sprintf("hyard audit status: %s", err.status)
}

func (err auditExitError) ExitCode() int {
	return err.code
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit the current Harness Yard revision",
		Long: "Audit the current Harness Yard revision without mutating repository state.\n" +
			"Ordinary Git repositories are reported as not_hyard_revision.",
		Example: "" +
			"  # Audit the current source revision\n" +
			"  hyard audit --json\n\n" +
			"  # Audit the current runtime revision\n" +
			"  hyard audit\n\n" +
			"  # Audit the current orbit-template revision\n" +
			"  hyard audit --json\n\n" +
			"  # Audit the current harness-template revision\n" +
			"  hyard audit --json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			result, err := harnesspkg.AuditRevision(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("audit harness yard revision: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := emitHyardJSON(cmd, result); err != nil {
					return err
				}
			} else if err := emitAuditText(cmd, result); err != nil {
				return err
			}

			if auditStatusExitCode(result.Status) != 0 {
				return auditExitError{status: result.Status, code: auditStatusExitCode(result.Status)}
			}

			return nil
		},
	}
	addHyardJSONFlag(cmd)

	return cmd
}

func auditStatusExitCode(status string) int {
	switch status {
	case harnesspkg.AuditStatusPass, harnesspkg.AuditStatusWarn:
		return 0
	default:
		return 1
	}
}

func emitAuditText(cmd *cobra.Command, result harnesspkg.AuditResult) error {
	if result.RepoRoot != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "repo_root: %s\n", result.RepoRoot); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", result.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "revision_kind: %s\n", result.RevisionKind); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.Packages) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "packages: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "packages:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, pkg := range result.Packages {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", auditPackageSummaryText(pkg)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}
	if len(result.Findings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "findings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "findings:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, finding := range result.Findings {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", auditFindingText(finding)); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func auditPackageSummaryText(pkg harnesspkg.AuditPackageSummary) string {
	fields := []string{
		"type=" + emptyTextFallback(pkg.Type),
		"name=" + emptyTextFallback(pkg.Name),
		"revision_role=" + emptyTextFallback(pkg.RevisionRole),
	}
	if pkg.Version != "" {
		fields = append(fields, "version="+pkg.Version)
	}
	if pkg.OrbitID != "" {
		fields = append(fields, "orbit_id="+pkg.OrbitID)
	}
	if pkg.HarnessID != "" {
		fields = append(fields, "harness_id="+pkg.HarnessID)
	}
	if pkg.Source != "" {
		fields = append(fields, "source="+pkg.Source)
	}

	return strings.Join(fields, " ")
}

func auditFindingText(finding harnesspkg.AuditFinding) string {
	fields := []string{
		"severity=" + emptyTextFallback(finding.Severity),
		"kind=" + emptyTextFallback(finding.Kind),
	}
	if finding.Package != "" {
		fields = append(fields, "package="+finding.Package)
	}
	if finding.Path != "" {
		fields = append(fields, "path="+finding.Path)
	}
	fields = append(fields, "message="+normalizeFindingMessage(finding.Message))

	return strings.Join(fields, " ")
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
