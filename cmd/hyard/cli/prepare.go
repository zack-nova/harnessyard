package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

func newPrepareCommand() *cobra.Command {
	var yes bool
	var check bool

	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Prepare this runtime repo for agent handoff",
		Long: "Prepare this runtime repo for agent handoff.\n" +
			"This user-level command applies safe runtime preparation steps before an agent starts work.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}

			var repoGuidance harnesspkg.RepoPrepareGuidancePlan
			if check {
				repoGuidance, err = harnesspkg.PlanRepoPrepareGuidance(resolved.Repo.Root)
			} else {
				repoGuidance, err = harnesspkg.ApplyRepoPrepareGuidance(resolved.Repo.Root)
			}
			if err != nil {
				return fmt.Errorf("prepare repo-level guidance: %w", err)
			}

			agentSelection := prepareAgentSelectionOutput{
				Action:      "not_checked",
				ReadyAgents: []string{},
			}
			if !check {
				agentSelection, err = prepareAgentSelection(cmd, resolved.Repo.Root, resolved.Repo.GitDir, yes, jsonOutput)
				if err != nil {
					return fmt.Errorf("prepare agent selection: %w", err)
				}
			}

			readiness, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return fmt.Errorf("evaluate runtime readiness: %w", err)
			}

			output := prepareOutput{
				RepoRoot:       resolved.Repo.Root,
				Check:          check,
				AutoApprove:    yes,
				RepoGuidance:   repoGuidance,
				AgentSelection: agentSelection,
				Readiness:      readiness,
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			if check {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "checked harness runtime preparation at %s\n", resolved.Repo.Root)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "prepared harness runtime at %s\n", resolved.Repo.Root)
			}
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", readiness.Status); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if !check {
				if err := emitPrepareAgentSelectionText(cmd, agentSelection); err != nil {
					return err
				}
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply preparation steps without interactive confirmation")
	cmd.Flags().BoolVar(&check, "check", false, "Preview preparation steps without writing files")
	addHyardJSONFlag(cmd)

	return cmd
}

type prepareOutput struct {
	RepoRoot       string                             `json:"repo_root"`
	Check          bool                               `json:"check"`
	AutoApprove    bool                               `json:"auto_approve"`
	RepoGuidance   harnesspkg.RepoPrepareGuidancePlan `json:"repo_guidance"`
	AgentSelection prepareAgentSelectionOutput        `json:"agent_selection"`
	Readiness      harnesspkg.ReadinessReport         `json:"readiness"`
}

type prepareAgentSelectionOutput struct {
	Action            string   `json:"action"`
	LocalSelection    string   `json:"local_selection,omitempty"`
	RecommendedAgent  string   `json:"recommended_agent,omitempty"`
	ReadyAgents       []string `json:"ready_agents"`
	SelectedAgent     string   `json:"selected_agent,omitempty"`
	SelectedFramework string   `json:"selected_framework,omitempty"`
	SelectionSource   string   `json:"selection_source,omitempty"`
	SelectionPath     string   `json:"selection_path,omitempty"`
	SuggestedCommand  string   `json:"suggested_command,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

func prepareAgentSelection(cmd *cobra.Command, repoRoot, gitDir string, yes bool, jsonOutput bool) (prepareAgentSelectionOutput, error) {
	if selection, err := harnesspkg.LoadFrameworkSelection(gitDir); err == nil {
		selectedAgent := prepareAgentIDForFramework(selection.SelectedFramework)
		return prepareAgentSelectionOutput{
			Action:            "already_selected",
			LocalSelection:    selectedAgent,
			ReadyAgents:       []string{},
			SelectedAgent:     selectedAgent,
			SelectedFramework: selection.SelectedFramework,
			SelectionSource:   string(selection.SelectionSource),
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return prepareAgentSelectionOutput{}, fmt.Errorf("load local agent selection: %w", err)
	}

	report, err := harnesspkg.DetectAgents(cmd.Context(), harnesspkg.AgentDetectionInput{
		RepoRoot: repoRoot,
		GitDir:   gitDir,
		Refresh:  true,
	})
	if err != nil {
		return prepareAgentSelectionOutput{}, fmt.Errorf("detect agents: %w", err)
	}

	output := prepareAgentSelectionOutput{
		Action:           "needs_selection",
		LocalSelection:   report.LocalSelection,
		RecommendedAgent: report.RuntimeRecommendation,
		ReadyAgents:      readyAgentsFromDetection(report),
	}
	if report.LocalSelection != "" {
		output.Action = "already_selected"
		output.SelectedAgent = report.LocalSelection
		return output, nil
	}

	candidate, source, reason := choosePrepareAgentSelectionCandidate(output.ReadyAgents, report.RuntimeRecommendation)
	if candidate == "" {
		if len(output.ReadyAgents) == 0 {
			output.Action = "no_ready_agents"
			output.Reason = "no ready detected agents"
		} else {
			output.Action = "multiple_ready_agents"
			output.Reason = "multiple ready detected agents and no ready recommended agent"
		}
		return output, nil
	}
	output.SelectedAgent = candidate
	output.SelectionSource = string(source)
	output.SuggestedCommand = "hyard agent use " + candidate
	output.Reason = reason

	shouldSelect := yes
	if !shouldSelect && !jsonOutput {
		shouldSelect, err = confirmPrepareAgentSelection(cmd, candidate, reason)
		if err != nil {
			return prepareAgentSelectionOutput{}, err
		}
	}
	if !shouldSelect {
		output.Action = "suggested"
		return output, nil
	}

	frameworkID, ok := harnesspkg.NormalizeFrameworkID(candidate)
	if !ok {
		return prepareAgentSelectionOutput{}, fmt.Errorf("detected agent %q is not supported by this build", candidate)
	}
	selectionPath, err := harnesspkg.WriteFrameworkSelection(gitDir, harnesspkg.FrameworkSelection{
		SelectedFramework: frameworkID,
		SelectionSource:   source,
		UpdatedAt:         time.Now().UTC(),
	})
	if err != nil {
		return prepareAgentSelectionOutput{}, fmt.Errorf("write agent selection: %w", err)
	}

	output.Action = "selected"
	output.SelectedFramework = frameworkID
	output.SelectionPath = selectionPath

	return output, nil
}

func readyAgentsFromDetection(report harnesspkg.AgentDetectionReport) []string {
	readyAgents := make([]string, 0)
	for _, tool := range report.Tools {
		if tool.Summary.Ready {
			readyAgents = append(readyAgents, tool.Agent)
		}
	}
	sort.Strings(readyAgents)

	return readyAgents
}

func choosePrepareAgentSelectionCandidate(readyAgents []string, recommendedAgent string) (string, harnesspkg.FrameworkSelectionSource, string) {
	if len(readyAgents) == 1 {
		return readyAgents[0], harnesspkg.FrameworkSelectionSourceProjectDetection, readyAgents[0] + " is the only ready detected agent"
	}
	if recommendedAgent == "" {
		return "", "", ""
	}
	for _, readyAgent := range readyAgents {
		if readyAgent == recommendedAgent {
			return recommendedAgent, harnesspkg.FrameworkSelectionSourceRecommendedDefault, recommendedAgent + " is the recommended ready agent"
		}
	}

	return "", "", ""
}

func prepareAgentIDForFramework(frameworkID string) string {
	if normalized, ok := harnesspkg.NormalizeAgentID(frameworkID); ok {
		return normalized
	}

	return frameworkID
}

func confirmPrepareAgentSelection(cmd *cobra.Command, agentID string, reason string) (bool, error) {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Use detected agent %s now? %s [y/N] ", agentID, reason); err != nil {
		return false, fmt.Errorf("write command output: %w", err)
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))

	return answer == "y" || answer == "yes", nil
}

func emitPrepareAgentSelectionText(cmd *cobra.Command, selection prepareAgentSelectionOutput) error {
	switch selection.Action {
	case "selected", "already_selected":
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "selected agent: %s\n", selection.SelectedAgent); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	case "suggested":
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "suggested_command: %s\n", selection.SuggestedCommand); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	case "multiple_ready_agents", "no_ready_agents":
		if selection.Reason != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent_selection: %s\n", selection.Reason); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}
