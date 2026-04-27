package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type briefBackfillOutput struct {
	RepoRoot       string                             `json:"repo_root"`
	OrbitID        string                             `json:"orbit_id"`
	DefinitionPath string                             `json:"definition_path"`
	Status         string                             `json:"status"`
	UpdatedField   string                             `json:"updated_field"`
	Replacements   []orbittemplate.ReplacementSummary `json:"replacements,omitempty"`
}

// NewBriefBackfillCommand creates the orbit brief backfill command.
func NewBriefBackfillCommand() *cobra.Command {
	var requestedOrbitID string
	var check bool

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill the current orbit AGENTS brief into hosted metadata",
		Long: "Extract the current orbit block from the repo root AGENTS.md, reverse-variableize it,\n" +
			"and write the result into meta.agents_template in the hosted OrbitSpec.\n" +
			"Supported revision kinds: runtime, source, orbit_template.",
		Example: "" +
			"  orbit brief backfill\n" +
			"  orbit brief backfill --orbit docs\n" +
			"  orbit brief backfill --orbit docs --check\n" +
			"  orbit brief backfill --orbit docs --json\n",
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
				status, err := orbittemplate.InspectOrbitBriefLaneForOperation(cmd.Context(), repo.Root, orbitID, "backfill")
				if err != nil {
					return fmt.Errorf("inspect orbit brief: %w", err)
				}

				return emitBriefCheckStatus(cmd, status, jsonOutput)
			}

			result, err := orbittemplate.BackfillOrbitBrief(cmd.Context(), orbittemplate.BriefBackfillInput{
				RepoRoot: repo.Root,
				OrbitID:  orbitID,
			})
			if err != nil {
				return fmt.Errorf("backfill orbit brief: %w", err)
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), briefBackfillOutput{
					RepoRoot:       repo.Root,
					OrbitID:        result.OrbitID,
					DefinitionPath: result.DefinitionPath,
					Status:         string(result.Status),
					UpdatedField:   "meta.agents_template",
					Replacements:   result.Replacements,
				})
			}

			if _, err := fmt.Fprintln(cmd.OutOrStdout(), describeBriefBackfillStatus(result.Status, result.OrbitID, result.DefinitionPath)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", result.Status); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(result.Replacements) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "replacements: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}
			names := make([]string, 0, len(result.Replacements))
			for _, replacement := range result.Replacements {
				names = append(names, "$"+replacement.Variable)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "replacements: %s\n", strings.Join(names, ", ")); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Override the target orbit id instead of using the current orbit")
	cmd.Flags().BoolVar(&check, "check", false, "Report the current brief-lane state without modifying files")
	addJSONFlag(cmd)

	return cmd
}

func describeBriefBackfillStatus(status orbittemplate.GuidanceBackfillStatus, orbitID string, definitionPath string) string {
	switch status {
	case orbittemplate.GuidanceBackfillStatusRemoved:
		return fmt.Sprintf("removed orbit brief %s from %s", orbitID, definitionPath)
	case orbittemplate.GuidanceBackfillStatusSkipped:
		return fmt.Sprintf("orbit brief %s already matches hosted truth at %s", orbitID, definitionPath)
	default:
		return fmt.Sprintf("backfilled orbit brief %s into %s", orbitID, definitionPath)
	}
}
