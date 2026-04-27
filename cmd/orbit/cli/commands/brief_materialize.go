package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type briefMaterializeOutput struct {
	RepoRoot   string `json:"repo_root"`
	OrbitID    string `json:"orbit_id"`
	AgentsPath string `json:"agents_path"`
	Changed    bool   `json:"changed"`
	Forced     bool   `json:"forced"`
}

type briefCheckOutput struct {
	RepoRoot                 string `json:"repo_root"`
	OrbitID                  string `json:"orbit_id"`
	RevisionKind             string `json:"revision_kind"`
	State                    string `json:"state"`
	AgentsPath               string `json:"agents_path"`
	HasAuthoredTruth         bool   `json:"has_authored_truth"`
	HasRootAgents            bool   `json:"has_root_agents"`
	HasOrbitBlock            bool   `json:"has_orbit_block"`
	MaterializeAllowed       bool   `json:"materialize_allowed"`
	MaterializeRequiresForce bool   `json:"materialize_requires_force"`
	BackfillAllowed          bool   `json:"backfill_allowed"`
}

// NewBriefMaterializeCommand creates the orbit brief materialize command.
func NewBriefMaterializeCommand() *cobra.Command {
	var requestedOrbitID string
	var force bool
	var check bool

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Materialize the authored orbit brief into the repo root AGENTS container",
		Long: "Render the hosted orbit brief from structured authored truth into the repo root AGENTS.md,\n" +
			"preserving unrelated prose and other orbit blocks.\n" +
			"Supported revision kinds: runtime, source, orbit_template.",
		Example: "" +
			"  orbit brief materialize\n" +
			"  orbit brief materialize --orbit docs\n" +
			"  orbit brief materialize --orbit docs --check\n" +
			"  orbit brief materialize --orbit docs --force\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			orbitID, err := resolveBriefOrbitID(cmd, repo, requestedOrbitID)
			if err != nil {
				return err
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if check {
				if force {
					return fmt.Errorf("--force cannot be used with --check")
				}

				status, err := orbittemplate.InspectOrbitBriefLane(cmd.Context(), repo.Root, orbitID)
				if err != nil {
					return fmt.Errorf("inspect orbit brief: %w", err)
				}
				return emitBriefCheckStatus(cmd, status, jsonOutput)
			}

			result, err := orbittemplate.MaterializeOrbitBrief(cmd.Context(), orbittemplate.BriefMaterializeInput{
				RepoRoot: repo.Root,
				OrbitID:  orbitID,
				Force:    force,
			})
			if err != nil {
				return fmt.Errorf("materialize orbit brief: %w", err)
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), briefMaterializeOutput{
					RepoRoot:   repo.Root,
					OrbitID:    result.OrbitID,
					AgentsPath: result.AgentsPath,
					Changed:    result.Changed,
					Forced:     result.Forced,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "materialized orbit brief %s into %s\n", result.OrbitID, result.AgentsPath); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if !result.Changed {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "status: no_change"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Override the target orbit id instead of using the current orbit")
	cmd.Flags().BoolVar(&check, "check", false, "Report the current brief-lane state without modifying files")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite a drifted current orbit block instead of failing closed")
	addJSONFlag(cmd)

	return cmd
}

func briefCheckPayload(status orbittemplate.BriefLaneStatus) briefCheckOutput {
	return briefCheckOutput{
		RepoRoot:                 status.RepoRoot,
		OrbitID:                  status.OrbitID,
		RevisionKind:             status.RevisionKind,
		State:                    string(status.State),
		AgentsPath:               status.AgentsPath,
		HasAuthoredTruth:         status.HasAuthoredTruth,
		HasRootAgents:            status.HasRootAgents,
		HasOrbitBlock:            status.HasOrbitBlock,
		MaterializeAllowed:       status.MaterializeAllowed,
		MaterializeRequiresForce: status.MaterializeRequiresForce,
		BackfillAllowed:          status.BackfillAllowed,
	}
}

func emitBriefCheckStatus(cmd *cobra.Command, status orbittemplate.BriefLaneStatus, jsonOutput bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), briefCheckPayload(status))
	}

	return emitBriefCheckText(cmd, status)
}

func emitBriefCheckText(cmd *cobra.Command, status orbittemplate.BriefLaneStatus) error {
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"orbit_id: %s\nrevision_kind: %s\nstate: %s\nagents_path: %s\nhas_authored_truth: %t\nhas_root_agents: %t\nhas_orbit_block: %t\nmaterialize.allowed: %t\nmaterialize.requires_force: %t\nbackfill.allowed: %t\n",
		status.OrbitID,
		status.RevisionKind,
		status.State,
		status.AgentsPath,
		status.HasAuthoredTruth,
		status.HasRootAgents,
		status.HasOrbitBlock,
		status.MaterializeAllowed,
		status.MaterializeRequiresForce,
		status.BackfillAllowed,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	return nil
}
