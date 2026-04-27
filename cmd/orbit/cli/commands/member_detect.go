package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type memberDetectOutput struct {
	RepoRoot     string                        `json:"repo_root"`
	OrbitID      string                        `json:"orbit_id"`
	RevisionKind string                        `json:"revision_kind"`
	HintCount    int                           `json:"hint_count"`
	Hints        []orbitpkg.DetectedMemberHint `json:"hints"`
}

type memberBackfillCheckOutput struct {
	RepoRoot        string                        `json:"repo_root"`
	OrbitID         string                        `json:"orbit_id"`
	RevisionKind    string                        `json:"revision_kind"`
	HintCount       int                           `json:"hint_count"`
	DriftDetected   bool                          `json:"drift_detected"`
	BackfillAllowed bool                          `json:"backfill_allowed"`
	Hints           []orbitpkg.DetectedMemberHint `json:"hints"`
}

type memberBackfillOutput struct {
	RepoRoot           string   `json:"repo_root"`
	OrbitID            string   `json:"orbit_id"`
	RevisionKind       string   `json:"revision_kind"`
	DefinitionPath     string   `json:"definition_path"`
	UpdatedMemberCount int      `json:"updated_member_count"`
	UpdatedMembers     []string `json:"updated_members,omitempty"`
	ConsumedHintCount  int      `json:"consumed_hint_count"`
	ConsumedHints      []string `json:"consumed_hints,omitempty"`
}

// NewMemberDetectCommand creates the orbit member detect command.
func NewMemberDetectCommand() *cobra.Command {
	var requestedOrbitID string

	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Preview member hints without modifying authored truth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			inspection, revisionKind, orbitID, err := inspectMemberHintsForCommand(cmd, repo, requestedOrbitID, "detect")
			if err != nil {
				return err
			}

			payload := memberDetectOutput{
				RepoRoot:     repo.Root,
				OrbitID:      orbitID,
				RevisionKind: revisionKind,
				HintCount:    len(inspection.Hints),
				Hints:        inspection.Hints,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"orbit %s (%s): %d member hint(s)\n",
				payload.OrbitID,
				payload.RevisionKind,
				payload.HintCount,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, hint := range payload.Hints {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s\t%s\t%s\t%s\n",
					hint.Action,
					hint.Kind,
					hint.HintPath,
					hint.ResolvedName,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")

	return cmd
}

// NewMemberBackfillCommand creates the orbit member backfill command.
func NewMemberBackfillCommand() *cobra.Command {
	var requestedOrbitID string
	var check bool

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill member hints into authored member truth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			inspection, revisionKind, orbitID, err := inspectMemberHintsForCommand(cmd, repo, requestedOrbitID, "backfill")
			if err != nil {
				return err
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			if check {
				payload := memberBackfillCheckOutput{
					RepoRoot:        repo.Root,
					OrbitID:         orbitID,
					RevisionKind:    revisionKind,
					HintCount:       len(inspection.Hints),
					DriftDetected:   inspection.DriftDetected,
					BackfillAllowed: inspection.BackfillAllowed,
					Hints:           inspection.Hints,
				}
				if jsonOutput {
					return emitJSON(cmd.OutOrStdout(), payload)
				}

				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"orbit %s (%s): %d member hint(s), drift=%t, backfill_allowed=%t\n",
					payload.OrbitID,
					payload.RevisionKind,
					payload.HintCount,
					payload.DriftDetected,
					payload.BackfillAllowed,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				for _, hint := range payload.Hints {
					if _, err := fmt.Fprintf(
						cmd.OutOrStdout(),
						"%s\t%s\t%s\t%s\n",
						hint.Action,
						hint.Kind,
						hint.HintPath,
						hint.ResolvedName,
					); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}

				return nil
			}

			spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, orbitID)
			if err != nil {
				return fmt.Errorf("load orbit spec: %w", err)
			}

			worktreeFiles, err := gitpkg.WorktreeFiles(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("list worktree files: %w", err)
			}

			result, err := orbitpkg.BackfillMemberHints(repo.Root, spec, worktreeFiles)
			if err != nil {
				return fmt.Errorf("backfill member hints: %w", err)
			}

			payload := memberBackfillOutput{
				RepoRoot:           repo.Root,
				OrbitID:            orbitID,
				RevisionKind:       revisionKind,
				DefinitionPath:     result.DefinitionPath,
				UpdatedMemberCount: len(result.UpdatedMembers),
				UpdatedMembers:     result.UpdatedMembers,
				ConsumedHintCount:  len(result.ConsumedHints),
				ConsumedHints:      result.ConsumedHints,
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"backfilled %d member(s) and consumed %d hint(s) for orbit %s (%s)\n",
				payload.UpdatedMemberCount,
				payload.ConsumedHintCount,
				payload.OrbitID,
				payload.RevisionKind,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().BoolVar(&check, "check", false, "Report member hint drift without modifying files")

	return cmd
}

func inspectMemberHintsForCommand(
	cmd *cobra.Command,
	repo gitpkg.Repo,
	requestedOrbitID string,
	operation string,
) (orbitpkg.MemberHintInspection, string, string, error) {
	target, err := resolveAllowedMemberHintTarget(cmd, repo, requestedOrbitID, operation)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", err
	}
	orbitID := target.OrbitID

	config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", err
	}
	definition, err := definitionByID(config, orbitID)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", fmt.Errorf("load orbit spec: %w", err)
	}

	worktreeFiles, err := gitpkg.WorktreeFiles(cmd.Context(), repo.Root)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", fmt.Errorf("list worktree files: %w", err)
	}

	inspection, err := orbitpkg.InspectMemberHints(repo.Root, spec, worktreeFiles)
	if err != nil {
		return orbitpkg.MemberHintInspection{}, "", "", fmt.Errorf("inspect member hints: %w", err)
	}

	return inspection, target.RepoState.Kind, orbitID, nil
}

func resolveAllowedMemberHintTarget(
	cmd *cobra.Command,
	repo gitpkg.Repo,
	requestedOrbitID string,
	operation string,
) (resolvedOrbitTarget, error) {
	state, err := orbittemplate.LoadCurrentRepoState(cmd.Context(), repo.Root)
	if err != nil {
		return resolvedOrbitTarget{}, fmt.Errorf("load current repo state: %w", err)
	}

	switch state.Kind {
	case harnesspkg.ManifestKindSource, harnesspkg.ManifestKindOrbitTemplate:
		return resolveAuthoringBranchOrbitTarget(cmd, repo, state, strings.TrimSpace(requestedOrbitID))
	default:
		return resolvedOrbitTarget{}, fmt.Errorf(
			"member %s supports only source or orbit_template revisions; current revision kind is %q",
			operation,
			state.Kind,
		)
	}
}
