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
	Notes          []string                   `json:"notes,omitempty"`
	Readiness      harnesspkg.ReadinessReport `json:"readiness"`
}

// NewAgentsComposeCommand creates the harness agents compose command.
func NewAgentsComposeCommand() *cobra.Command {
	var force bool
	var output bool

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Compose current runtime orbit briefs into the root AGENTS container",
		Long: "Compose current runtime orbit briefs into the root AGENTS.md container,\n" +
			"preserving unrelated prose and non-target blocks.\n" +
			"This command is the agents-target compatibility alias for `harness guidance compose --target agents`.\n" +
			"Only runtime members with authored brief truth are materialized. In Run View, standalone compose\n" +
			"is presentation output and requires interactive confirmation or --output.",
		Example: "" +
			"  harness agents compose --output\n" +
			"  harness agents compose --output --force\n" +
			"  harness agents compose --output --json\n",
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

			runViewOutput, explicitOutput, err := requireStandaloneRunViewGuidanceOutputIntent(cmd, resolved, output)
			if err != nil {
				return err
			}

			result, readiness, jsonOutput, err := runGuidanceCompose(cmd, resolved.Repo.Root, harnesspkg.GuidanceTargetAgents, force, nil)
			if err != nil {
				return fmt.Errorf("compose runtime AGENTS: %w", err)
			}
			if len(result.Artifacts) != 1 {
				return fmt.Errorf("compose runtime AGENTS: expected exactly one agent artifact, got %d", len(result.Artifacts))
			}
			artifact := result.Artifacts[0]

			payload := agentsComposeJSON{
				HarnessRoot:    resolved.Repo.Root,
				AgentsPath:     artifact.Path,
				MemberCount:    result.MemberCount,
				ComposedCount:  len(artifact.ComposedOrbitIDs),
				SkippedCount:   len(artifact.SkippedOrbitIDs),
				ChangedCount:   artifact.ChangedCount,
				ComposedOrbits: append([]string(nil), artifact.ComposedOrbitIDs...),
				SkippedOrbits:  append([]string(nil), artifact.SkippedOrbitIDs...),
				Forced:         result.Forced,
				Readiness:      readiness,
			}
			if runViewOutput && explicitOutput {
				payload.Notes = append(payload.Notes, guidanceComposeRunViewOutputNote)
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed root AGENTS.md for harness %s\n", resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if runViewOutput && explicitOutput {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "note: "+guidanceComposeRunViewOutputNote); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agents_path: %s\n", artifact.Path); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", result.MemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed_count: %d\n", len(artifact.ComposedOrbitIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skipped_count: %d\n", len(artifact.SkippedOrbitIDs)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "changed_count: %d\n", artifact.ChangedCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(artifact.ComposedOrbitIDs) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "composed_orbits: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed_orbits: %s\n", strings.Join(artifact.ComposedOrbitIDs, ", ")); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if len(artifact.SkippedOrbitIDs) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "skipped_orbits: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skipped_orbits: %s\n", strings.Join(artifact.SkippedOrbitIDs, ", ")); err != nil {
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
	cmd.Flags().BoolVar(&output, "output", false, "Output standalone Run View guidance presentation")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
