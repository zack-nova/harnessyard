package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type guidanceMaterializeOutput struct {
	RepoRoot      string                        `json:"repo_root"`
	OrbitID       string                        `json:"orbit_id"`
	Target        string                        `json:"target"`
	ArtifactCount int                           `json:"artifact_count"`
	Artifacts     []guidanceMaterializeArtifact `json:"artifacts"`
}

type guidanceMaterializeAggregateOutput struct {
	RepoRoot      string                            `json:"repo_root"`
	Target        string                            `json:"target"`
	OrbitCount    int                               `json:"orbit_count"`
	ArtifactCount int                               `json:"artifact_count"`
	Orbits        []guidanceMaterializeOrbitSummary `json:"orbits"`
}

type guidanceMaterializeOrbitSummary struct {
	OrbitID       string                        `json:"orbit_id"`
	ArtifactCount int                           `json:"artifact_count"`
	Artifacts     []guidanceMaterializeArtifact `json:"artifacts"`
}

type guidanceMaterializeArtifact struct {
	Target           string `json:"target"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	Path             string `json:"path"`
	Changed          bool   `json:"changed"`
	Forced           bool   `json:"forced"`
	SeedEmptyAllowed bool   `json:"seed_empty_allowed"`
}

// NewGuidanceMaterializeCommand creates the orbit guidance materialize command.
func NewGuidanceMaterializeCommand() *cobra.Command {
	return NewGuidanceMaterializeCommandWithOptions(GuidanceCommandOptions{})
}

// NewGuidanceMaterializeCommandWithOptions creates the orbit guidance
// materialize command with optional public-wrapper behavior.
func NewGuidanceMaterializeCommandWithOptions(options GuidanceCommandOptions) *cobra.Command {
	var requestedOrbitID string
	var target string
	var force bool
	var check bool
	var seedEmpty bool
	var strict bool

	cmd := &cobra.Command{
		Use:   "materialize",
		Short: "Materialize orbit guidance templates into root AGENTS, HUMANS, or BOOTSTRAP artifacts",
		Long: "Render authored orbit guidance from structured truth into root guidance artifacts,\n" +
			"preserving unrelated prose and other orbit blocks. The default all target renders\n" +
			"applicable guidance and skips missing authored guidance; use --strict to require\n" +
			"every selected target to be renderable.\n" +
			"Supported revision kinds: runtime, source, orbit_template.",
		Example: "" +
			"  orbit guidance materialize --orbit docs\n" +
			"  orbit guidance materialize --orbit docs --target all\n" +
			"  orbit guidance materialize --orbit docs --target all --strict\n" +
			"  orbit guidance materialize --orbit docs --target all --seed-empty\n" +
			"  orbit guidance materialize --orbit docs --target bootstrap\n" +
			"  orbit guidance materialize --orbit docs --target humans --check\n" +
			"  orbit guidance materialize --orbit docs --target agents --force\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			resolvedTarget, targets, err := resolveOrbitGuidanceTargets(target)
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
				if seedEmpty {
					return fmt.Errorf("--seed-empty cannot be used with --check")
				}
				if strict {
					return fmt.Errorf("--strict cannot be used with --check")
				}
				if options.DefaultAllOrbitsWhenOrbitOmitted && requestedOrbitID == "" {
					orbits, err := inspectAllOrbitGuidanceTargets(cmd, repo.Root, resolvedTarget, targets, "materialize")
					if err != nil {
						return fmt.Errorf("inspect orbit guidance: %w", err)
					}
					return emitOrbitGuidanceAggregateCheck(cmd, repo.Root, resolvedTarget, orbits, jsonOutput)
				}

				orbitID, err := resolveBriefOrbitID(cmd, repo, requestedOrbitID)
				if err != nil {
					return err
				}
				statuses, err := inspectOrbitGuidanceTargets(cmd, repo.Root, orbitID, targets, "materialize")
				if err != nil {
					return fmt.Errorf("inspect orbit guidance: %w", err)
				}
				return emitOrbitGuidanceCheck(cmd, repo.Root, orbitID, resolvedTarget, statuses, jsonOutput)
			}
			if strict && seedEmpty {
				return fmt.Errorf("--strict cannot be used with --seed-empty")
			}
			if options.DefaultAllOrbitsWhenOrbitOmitted && requestedOrbitID == "" {
				orbits, err := materializeAllOrbitGuidanceTargets(cmd, repo.Root, resolvedTarget, targets, force, seedEmpty, strict)
				if err != nil {
					return err
				}
				return emitOrbitGuidanceAggregateMaterialize(cmd, repo.Root, resolvedTarget, orbits, jsonOutput)
			}

			orbitID, err := resolveBriefOrbitID(cmd, repo, requestedOrbitID)
			if err != nil {
				return err
			}
			statuses, err := inspectOrbitGuidanceTargets(cmd, repo.Root, orbitID, targets, "materialize")
			if err != nil {
				return fmt.Errorf("inspect orbit guidance: %w", err)
			}
			artifacts, err := materializeOrbitGuidanceTargets(cmd, repo.Root, orbitID, resolvedTarget, targets, statuses, force, seedEmpty, strict)
			if err != nil {
				return err
			}
			return emitOrbitGuidanceMaterialize(cmd, repo.Root, orbitID, resolvedTarget, artifacts, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Override the target orbit id instead of using the current orbit")
	if options.DefaultAllOrbitsWhenOrbitOmitted {
		cmd.Flags().Lookup("orbit").Usage = "Limit the operation to one orbit id; omitted processes all applicable orbits"
	}
	cmd.Flags().StringVar(&target, "target", string(orbittemplate.GuidanceTargetAll), "Target to materialize: agents, humans, bootstrap, or all")
	cmd.Flags().BoolVar(&check, "check", false, "Report the current guidance-lane state without modifying files")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite drifted orbit blocks instead of failing closed")
	cmd.Flags().BoolVar(&seedEmpty, "seed-empty", false, "When the authored template is empty, create an empty orbit block so the first draft can be edited and backfilled")
	cmd.Flags().BoolVar(&strict, "strict", false, "Require every selected guidance target to be renderable instead of skipping inapplicable targets")
	addJSONFlag(cmd)

	return cmd
}

func materializeAllOrbitGuidanceTargets(
	cmd *cobra.Command,
	repoRoot string,
	resolvedTarget orbittemplate.GuidanceTarget,
	targets []orbittemplate.GuidanceTarget,
	force bool,
	seedEmpty bool,
	strict bool,
) ([]guidanceMaterializeOrbitSummary, error) {
	orbitIDs, err := guidanceOrbitIDsForAll(cmd, repoRoot)
	if err != nil {
		return nil, err
	}

	statusesByOrbit := make(map[string][]orbitGuidanceCheckArtifact, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		statuses, err := inspectOrbitGuidanceTargets(cmd, repoRoot, orbitID, targets, "materialize")
		if err != nil {
			return nil, fmt.Errorf("inspect orbit %q guidance: %w", orbitID, err)
		}
		if err := preflightMaterializeOrbitGuidanceTargets(orbitID, resolvedTarget, targets, statuses, force, seedEmpty, strict); err != nil {
			return nil, err
		}
		statusesByOrbit[orbitID] = statuses
	}

	orbits := make([]guidanceMaterializeOrbitSummary, 0, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		artifacts, err := materializeOrbitGuidanceTargets(cmd, repoRoot, orbitID, resolvedTarget, targets, statusesByOrbit[orbitID], force, seedEmpty, strict)
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, guidanceMaterializeOrbitSummary{
			OrbitID:       orbitID,
			ArtifactCount: len(artifacts),
			Artifacts:     artifacts,
		})
	}

	return orbits, nil
}
