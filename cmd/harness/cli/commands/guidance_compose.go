package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type guidanceComposeJSON struct {
	HarnessRoot   string                        `json:"harness_root"`
	Target        string                        `json:"target"`
	MemberCount   int                           `json:"member_count"`
	ArtifactCount int                           `json:"artifact_count"`
	Artifacts     []guidanceComposeArtifactJSON `json:"artifacts"`
	Forced        bool                          `json:"forced"`
	Notes         []string                      `json:"notes,omitempty"`
	Readiness     harnesspkg.ReadinessReport    `json:"readiness"`
}

type guidanceComposeArtifactJSON struct {
	Target         string   `json:"target"`
	Path           string   `json:"path"`
	ComposedCount  int      `json:"composed_count"`
	SkippedCount   int      `json:"skipped_count"`
	ChangedCount   int      `json:"changed_count"`
	ComposedOrbits []string `json:"composed_orbits"`
	SkippedOrbits  []string `json:"skipped_orbits"`
}

const guidanceComposeRunViewOutputNote = "standalone Run View guidance output is presentation output, not authored reconciliation"

// NewGuidanceComposeCommand creates the harness guidance compose command.
func NewGuidanceComposeCommand() *cobra.Command {
	var force bool
	var target string
	var audience string
	var orbitIDs []string
	var output bool

	cmd := &cobra.Command{
		Use:   "compose",
		Short: "Compose runtime guidance artifacts for one or more targets",
		Long: "Compose current runtime orbit guidance into root guidance artifacts for the requested target.\n" +
			"`agents` targets AGENTS.md, `humans` targets HUMANS.md, `bootstrap` targets BOOTSTRAP.md,\n" +
			"and `all` composes all three. In Run View, standalone compose is presentation output\n" +
			"and requires interactive confirmation or --output.\n" +
			"Unrelated prose and non-target orbit blocks are preserved.",
		Example: "" +
			"  harness guidance compose --target all --output\n" +
			"  harness guidance compose --target humans --output --json\n" +
			"  harness guidance compose --target bootstrap --output --force\n",
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

			composeTarget := normalizeGuidanceComposeCLIValue(strings.TrimSpace(target))
			if legacyAudience := strings.TrimSpace(audience); legacyAudience != "" {
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "warning: --audience is deprecated; use --target"); err != nil {
					return fmt.Errorf("write command warning: %w", err)
				}
				legacyTarget := normalizeGuidanceComposeCLIValue(legacyAudience)
				if composeTarget != "" && composeTarget != string(harnesspkg.GuidanceTargetAll) && composeTarget != legacyTarget {
					return fmt.Errorf("cannot combine --target %q with legacy --audience %q", composeTarget, legacyAudience)
				}
				composeTarget = legacyTarget
			}

			result, readiness, jsonOutput, err := runGuidanceCompose(cmd, resolved.Repo.Root, harnesspkg.GuidanceTarget(composeTarget), force, orbitIDs)
			if err != nil {
				return err
			}

			payload := guidanceComposeJSON{
				HarnessRoot:   resolved.Repo.Root,
				Target:        string(result.Target),
				MemberCount:   result.MemberCount,
				ArtifactCount: len(result.Artifacts),
				Artifacts:     make([]guidanceComposeArtifactJSON, 0, len(result.Artifacts)),
				Forced:        result.Forced,
				Readiness:     readiness,
			}
			if runViewOutput && explicitOutput {
				payload.Notes = append(payload.Notes, guidanceComposeRunViewOutputNote)
			}
			for _, artifact := range result.Artifacts {
				payload.Artifacts = append(payload.Artifacts, guidanceComposeArtifactJSON{
					Target:         string(artifact.Target),
					Path:           artifact.Path,
					ComposedCount:  len(artifact.ComposedOrbitIDs),
					SkippedCount:   len(artifact.SkippedOrbitIDs),
					ChangedCount:   artifact.ChangedCount,
					ComposedOrbits: append([]string(nil), artifact.ComposedOrbitIDs...),
					SkippedOrbits:  append([]string(nil), artifact.SkippedOrbitIDs...),
				})
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "composed guidance artifacts for harness %s\n", resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if runViewOutput && explicitOutput {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "note: "+guidanceComposeRunViewOutputNote); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "target: %s\n", result.Target); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", result.MemberCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact_count: %d\n", len(result.Artifacts)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, artifact := range result.Artifacts {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"artifact: %s path=%s composed_count=%d skipped_count=%d changed_count=%d\n",
					artifact.Target,
					artifact.Path,
					len(artifact.ComposedOrbitIDs),
					len(artifact.SkippedOrbitIDs),
					artifact.ChangedCount,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&target, "target", string(harnesspkg.GuidanceTargetAll), "Target to compose: agents, humans, bootstrap, or all")
	cmd.Flags().StringSliceVar(&orbitIDs, "orbit", nil, "Limit compose to one or more orbit ids")
	cmd.Flags().StringVar(&audience, "audience", "", "Deprecated alias for --target")
	if err := cmd.Flags().MarkHidden("audience"); err != nil {
		panic(err)
	}
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite drifted guidance blocks instead of failing closed")
	cmd.Flags().BoolVar(&output, "output", false, "Output standalone Run View guidance presentation")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}

func requireStandaloneRunViewGuidanceOutputIntent(cmd *cobra.Command, resolved harnesspkg.ResolvedRoot, output bool) (bool, bool, error) {
	jsonOutput, err := wantJSON(cmd)
	if err != nil {
		return false, false, err
	}
	runViewOutput, err := guidanceComposeRunViewSelected(resolved)
	if err != nil {
		return false, false, err
	}
	if !runViewOutput {
		return false, output, nil
	}
	if output {
		return true, true, nil
	}
	if jsonOutput || !shouldPromptGuidanceComposeOutput(cmd) {
		return true, false, fmt.Errorf("standalone Run View guidance output requires explicit output intent; rerun with --output")
	}
	confirmed, err := confirmGuidanceComposeOutput(cmd)
	if err != nil {
		return true, false, err
	}
	if !confirmed {
		return true, false, fmt.Errorf("standalone Run View guidance output canceled")
	}

	return true, true, nil
}

func guidanceComposeRunViewSelected(resolved harnesspkg.ResolvedRoot) (bool, error) {
	store, err := statepkg.NewFSStore(resolved.Repo.GitDir)
	if err != nil {
		return false, fmt.Errorf("create state store: %w", err)
	}
	selection, err := store.ReadRuntimeViewSelection()
	if err != nil {
		return false, fmt.Errorf("read runtime view selection: %w", err)
	}

	return selection.View == statepkg.RuntimeViewRun, nil
}

func shouldPromptGuidanceComposeOutput(cmd *cobra.Command) bool {
	if guidanceComposeInteractiveFromContext(cmd.Context()) {
		return true
	}
	return templatePublishStreamIsTerminal(cmd.InOrStdin()) && templatePublishStreamIsTerminal(cmd.ErrOrStderr())
}

func confirmGuidanceComposeOutput(cmd *cobra.Command) (bool, error) {
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Standalone Run View guidance output writes presentation text, not authored reconciliation."); err != nil {
		return false, fmt.Errorf("write command output: %w", err)
	}
	prompter := orbittemplate.LineConfirmPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
	confirmed, err := prompter.Confirm(cmd.Context(), "Output standalone Run View guidance presentation? [y/N] ")
	if err != nil {
		return false, fmt.Errorf("confirm standalone Run View guidance output: %w", err)
	}

	return confirmed, nil
}

func runGuidanceCompose(
	cmd *cobra.Command,
	repoRoot string,
	target harnesspkg.GuidanceTarget,
	force bool,
	orbitIDs []string,
) (harnesspkg.ComposeRuntimeGuidanceResult, harnesspkg.ReadinessReport, bool, error) {
	jsonOutput, err := wantJSON(cmd)
	if err != nil {
		return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, err
	}

	tx, err := harnesspkg.BeginInstallTransaction(cmd.Context(), repoRoot, []string{"AGENTS.md", "HUMANS.md", "BOOTSTRAP.md"})
	if err != nil {
		return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, fmt.Errorf("begin guidance sync transaction: %w", err)
	}
	result, err := harnesspkg.ComposeRuntimeGuidance(cmd.Context(), harnesspkg.ComposeRuntimeGuidanceInput{
		RepoRoot: repoRoot,
		Force:    force,
		Target:   target,
		OrbitIDs: orbitIDs,
	})
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, errors.Join(
				fmt.Errorf("compose runtime guidance: %w", err),
				fmt.Errorf("rollback guidance sync: %w", rollbackErr),
			)
		}
		return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, fmt.Errorf("compose runtime guidance: %w", err)
	}
	if _, err := harnesspkg.ApplyRunViewPresentationDefault(cmd.Context(), repoRoot); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, errors.Join(
				fmt.Errorf("apply Run View presentation: %w", err),
				fmt.Errorf("rollback guidance sync: %w", rollbackErr),
			)
		}
		return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, fmt.Errorf("apply Run View presentation: %w", err)
	}
	tx.Commit()

	readiness, err := evaluateCommandReadiness(cmd.Context(), repoRoot)
	if err != nil {
		return harnesspkg.ComposeRuntimeGuidanceResult{}, harnesspkg.ReadinessReport{}, false, err
	}

	return result, readiness, jsonOutput, nil
}

func normalizeGuidanceComposeCLIValue(raw string) string {
	switch raw {
	case "agent":
		return string(harnesspkg.GuidanceTargetAgents)
	case "human":
		return string(harnesspkg.GuidanceTargetHumans)
	default:
		return raw
	}
}
