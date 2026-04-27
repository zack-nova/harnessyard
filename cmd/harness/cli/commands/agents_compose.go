package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type agentsComposeJSON struct {
	HarnessRoot    string                     `json:"harness_root"`
	AgentsPath     string                     `json:"agents_path"`
	MemberCount    int                        `json:"member_count"`
	ComposedCount  int                        `json:"composed_count"`
	SkippedCount   int                        `json:"skipped_count"`
	ChangedCount   int                        `json:"changed_count"`
	ComposedOrbits []string                   `json:"composed_orbits"`
	SkippedOrbits  []string                   `json:"skipped_orbits"`
	Forced         bool                       `json:"forced"`
	Readiness      harnesspkg.ReadinessReport `json:"readiness"`
}

// NewAgentsComposeCommand creates the harness agents compose command.
func NewAgentsComposeCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Compose current runtime orbit briefs into the root AGENTS container",
		Long: "Compose current runtime orbit briefs into the root AGENTS.md container,\n" +
			"preserving unrelated prose and non-target blocks.\n" +
			"This command is the agents-target compatibility alias for `harness guidance compose --target agents`.\n" +
			"Only runtime members with authored brief truth are materialized.",
		Example: "" +
			"  harness agents compose\n" +
			"  harness agents compose --force\n" +
			"  harness agents compose --json\n",
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

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			result, err := harnesspkg.ComposeRuntimeAgents(cmd.Context(), harnesspkg.ComposeRuntimeAgentsInput{
				RepoRoot: resolved.Repo.Root,
				Force:    force,
			})
			if err != nil {
				return fmt.Errorf("compose runtime AGENTS: %w", err)
			}
			readiness, err := evaluateCommandReadiness(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return err
			}

			payload := agentsComposeJSON{
				HarnessRoot:    resolved.Repo.Root,
				AgentsPath:     result.AgentsPath,
				MemberCount:    result.MemberCount,
				ComposedCount:  len(result.ComposedOrbitIDs),
				SkippedCount:   len(result.SkippedOrbitIDs),
				ChangedCount:   result.ChangedCount,
				ComposedOrbits: append([]string(nil), result.ComposedOrbitIDs...),
				SkippedOrbits:  append([]string(nil), result.SkippedOrbitIDs...),
				Forced:         result.Forced,
				Readiness:      readiness,
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed root AGENTS.md for harness %s\n", resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agents_path: %s\n", result.AgentsPath); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", result.MemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed_count: %d\n", len(result.ComposedOrbitIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skipped_count: %d\n", len(result.SkippedOrbitIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "changed_count: %d\n", result.ChangedCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(result.ComposedOrbitIDs) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "composed_orbits: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed_orbits: %s\n", strings.Join(result.ComposedOrbitIDs, ", ")); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if len(result.SkippedOrbitIDs) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "skipped_orbits: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skipped_orbits: %s\n", strings.Join(result.SkippedOrbitIDs, ", ")); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite drifted orbit blocks instead of failing closed")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
