package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

// NewAgentDetectCommand creates the harness agent detect command.
func NewAgentDetectCommand() *cobra.Command {
	var deep bool
	var refresh bool

	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Detect local agent frameworks without changing selection",
		Long: "Detect local agent frameworks for this runtime without writing selection, activation,\n" +
			"guidance, or target-agent configuration.",
		Example: "" +
			"  harness agent detect\n" +
			"  harness agent detect --json\n",
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

			report, err := harnesspkg.DetectAgents(cmd.Context(), harnesspkg.AgentDetectionInput{
				RepoRoot: resolved.Repo.Root,
				GitDir:   resolved.Repo.GitDir,
				Deep:     deep,
				Refresh:  refresh,
			})
			if err != nil {
				return fmt.Errorf("detect agents: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), report)
			}

			return emitAgentDetectionText(cmd, report)
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)
	cmd.Flags().BoolVar(&deep, "deep", false, "Run slower best-effort package and service checks")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Bypass cached detection data when caching is available")

	return cmd
}

func emitAgentDetectionText(cmd *cobra.Command, report harnesspkg.AgentDetectionReport) error {
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintln(out, "Agent Detection"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if report.RuntimeRecommendation == "" {
		if _, err := fmt.Fprintln(out, "Current runtime recommendation: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else if _, err := fmt.Fprintf(out, "Current runtime recommendation: %s\n", report.RuntimeRecommendation); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if report.LocalSelection == "" {
		if _, err := fmt.Fprintln(out, "Current local selection: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else if _, err := fmt.Fprintf(out, "Current local selection: %s\n", report.LocalSelection); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	for _, tool := range report.Tools {
		if _, err := fmt.Fprintln(out, agentDetectionDisplayName(tool.Agent)); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if _, err := fmt.Fprintf(out, "  Status: %s\n", humanAgentDetectionStatus(tool.Summary.Status)); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, component := range tool.Components {
			if component.Status == harnesspkg.AgentDetectionStatusNotFound {
				continue
			}
			if component.Version != "" {
				if _, err := fmt.Fprintf(out, "  %s: %s, version %s\n", component.Component, humanAgentDetectionStatus(component.Status), component.Version); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				continue
			}
			if _, err := fmt.Fprintf(out, "  %s: %s\n", component.Component, humanAgentDetectionStatus(component.Status)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, evidence := range component.Evidence {
				if evidence.Path == "" {
					continue
				}
				if _, err := fmt.Fprintf(out, "    - %s\n", evidence.Path); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if len(report.SuggestedActions) > 0 {
		if _, err := fmt.Fprintln(out, "Suggested next step:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, action := range report.SuggestedActions {
			if _, err := fmt.Fprintf(out, "  %s\n", action.Command); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	} else if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintln(out, "Warnings:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(out, "  %s\n", warning); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}

func agentDetectionDisplayName(agentID string) string {
	switch agentID {
	case "codex":
		return "Codex"
	case "claudecode":
		return "Claude Code"
	case "openclaw":
		return "OpenClaw"
	default:
		return agentID
	}
}

func humanAgentDetectionStatus(status harnesspkg.AgentDetectionStatus) string {
	switch status {
	case harnesspkg.AgentDetectionStatusInstalledCLI,
		harnesspkg.AgentDetectionStatusInstalledDesktop,
		harnesspkg.AgentDetectionStatusRunning:
		return "ready"
	case harnesspkg.AgentDetectionStatusInstalledUnverified:
		return "installed"
	case harnesspkg.AgentDetectionStatusConfigured,
		harnesspkg.AgentDetectionStatusFootprintOnly:
		return "footprint only"
	case harnesspkg.AgentDetectionStatusStaleOrRemoved:
		return "stale"
	case harnesspkg.AgentDetectionStatusAmbiguous:
		return "ambiguous"
	case harnesspkg.AgentDetectionStatusNotFound:
		return "not found"
	default:
		return strings.ReplaceAll(string(status), "_", " ")
	}
}
