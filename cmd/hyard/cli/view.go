package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func newViewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Inspect Runtime View selection and presentation",
		Long: "Inspect Runtime View selection and presentation.\n" +
			"Runtime View status reports repository-local Run View or Author View selection,\n" +
			"visible authoring scaffolds, publication permissions, and next actions.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newViewAuthorCommand(),
		newViewRunCommand(),
		newViewStatusCommand(),
	)

	return cmd
}

func newViewRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Apply or preview Run View cleanup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			check, err := cmd.Flags().GetBool("check")
			if err != nil {
				return fmt.Errorf("read --check flag: %w", err)
			}
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}
			resolveMarkedValue, err := cmd.Flags().GetString("resolve-marked")
			if err != nil {
				return fmt.Errorf("read --resolve-marked flag: %w", err)
			}
			resolveMarked, err := harnesspkg.NormalizeRuntimeViewMarkedGuidanceResolution(resolveMarkedValue)
			if err != nil {
				return fmt.Errorf("read --resolve-marked flag: %w", err)
			}

			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}

			result, cleanupErr := harnesspkg.RuntimeViewCleanupWithOptions(cmd.Context(), repo, store, harnesspkg.RuntimeViewCleanupInput{
				Check:                    check,
				MarkedGuidanceResolution: resolveMarked,
			})
			if jsonOutput {
				var blocked harnesspkg.RuntimeViewCleanupBlockedError
				if cleanupErr == nil || errors.As(cleanupErr, &blocked) {
					if err := emitHyardJSON(cmd, result); err != nil {
						return err
					}
				}
				if cleanupErr != nil {
					return fmt.Errorf("run Run View cleanup: %w", cleanupErr)
				}

				return nil
			}
			if cleanupErr != nil {
				return fmt.Errorf("run Run View cleanup: %w", cleanupErr)
			}

			return renderHyardViewRun(cmd, result)
		},
	}
	cmd.Flags().Bool("check", false, "Preview Run View cleanup without writing files")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	cmd.Flags().String("resolve-marked", "", "Resolve drifted marked guidance before cleanup: save, render, or strip")

	return cmd
}

type hyardViewAuthorResult struct {
	SelectedView       statepkg.RuntimeView              `json:"selected_view"`
	SelectionPersisted bool                              `json:"selection_persisted"`
	SelectedAt         time.Time                         `json:"selected_at"`
	Materialized       hyardViewAuthorMaterializedResult `json:"materialized"`
	NextActions        []string                          `json:"next_actions"`
}

type hyardViewAuthorMaterializedResult struct {
	GuidanceMarkers bool `json:"guidance_markers"`
	MarkdownContent bool `json:"markdown_content"`
	MemberHints     bool `json:"member_hints"`
}

func newViewAuthorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "author",
		Short: "Select Author View",
		Long: "Select Author View for this Harness Runtime.\n" +
			"Selecting Author View records repository-local intent only; it does not render\n" +
			"root guidance markers, Markdown Member Hints, or Member Hint sidecars.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			if _, err := harnesspkg.LoadRuntimeFile(repo.Root); err != nil {
				return fmt.Errorf("load harness runtime: %w", err)
			}
			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}

			result, err := selectHyardAuthorView(store, time.Now().UTC())
			if err != nil {
				return err
			}

			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}
			if jsonOutput {
				return emitHyardJSON(cmd, result)
			}

			return renderHyardViewAuthor(cmd, result)
		},
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")

	return cmd
}

func selectHyardAuthorView(store statepkg.FSStore, now time.Time) (hyardViewAuthorResult, error) {
	selection := statepkg.RuntimeViewSelection{
		View:       statepkg.RuntimeViewAuthor,
		SelectedAt: now,
	}
	if err := store.WriteRuntimeViewSelection(selection); err != nil {
		return hyardViewAuthorResult{}, fmt.Errorf("write runtime view selection: %w", err)
	}

	return hyardViewAuthorResult{
		SelectedView:       statepkg.RuntimeViewAuthor,
		SelectionPersisted: true,
		SelectedAt:         now,
		Materialized:       hyardViewAuthorMaterializedResult{},
		NextActions: []string{
			"render editable guidance with `hyard guide render`",
			"publish an Orbit Package",
			"publish current runtime as a Harness Package",
		},
	}, nil
}

func renderHyardViewAuthor(cmd *cobra.Command, result hyardViewAuthorResult) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "selected_view: %s (stored)\n", result.SelectedView); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"materialized: guidance_markers=%t markdown_content=%t member_hints=%t\n",
		result.Materialized.GuidanceMarkers,
		result.Materialized.MarkdownContent,
		result.Materialized.MemberHints,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := renderHyardViewStatusList(cmd, "next_actions", result.NextActions); err != nil {
		return err
	}

	return nil
}

func newViewStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Runtime View status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}

			result, err := harnesspkg.RuntimeViewStatus(cmd.Context(), repo, store)
			if err != nil {
				return fmt.Errorf("load runtime view status: %w", err)
			}

			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read --json flag: %w", err)
			}
			if jsonOutput {
				return emitHyardJSON(cmd, result)
			}

			return renderHyardViewStatus(cmd, result)
		},
	}
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")

	return cmd
}

func renderHyardViewStatus(cmd *cobra.Command, result harnesspkg.RuntimeViewStatusResult) error {
	selectionSuffix := " (stored)"
	if !result.SelectionPersisted {
		selectionSuffix = " (default)"
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "selected_view: %s%s\n", result.SelectedView, selectionSuffix); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "actual_presentation: %s\n", result.ActualPresentation.Mode); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if result.ActualPresentation.CurrentOrbit != "" {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"current_orbit: %s sparse_checkout=%t\n",
			result.ActualPresentation.CurrentOrbit,
			result.ActualPresentation.CurrentOrbitSparseCheckout,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "guidance_markers:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, marker := range result.GuidanceMarkers {
		presence := "absent"
		if marker.Present {
			presence = "present"
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"  %s %s %s blocks=%d orbit=%d harness=%d\n",
			marker.Target,
			marker.Path,
			presence,
			marker.BlockCount,
			marker.OrbitBlockCount,
			marker.HarnessBlockCount,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if marker.ParseError != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "    parse_error: %s\n", marker.ParseError); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"member_hints: %d drift=%t backfill_allowed=%t blockers=%d\n",
		result.MemberHints.HintCount,
		result.MemberHints.DriftDetected,
		result.MemberHints.BackfillAllowed,
		result.MemberHints.BlockerCount,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.DriftBlockers) > 0 {
		if err := renderHyardViewStatusList(cmd, "drift_blockers", result.DriftBlockers); err != nil {
			return err
		}
	}
	if err := renderHyardViewStatusList(cmd, "allowed_publication_actions", result.AllowedPublicationActions); err != nil {
		return err
	}
	if err := renderHyardViewStatusList(cmd, "next_actions", result.NextActions); err != nil {
		return err
	}

	return nil
}

func renderHyardViewStatusList(cmd *cobra.Command, title string, values []string) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(values) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	for _, value := range values {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", value); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func renderHyardViewRun(cmd *cobra.Command, result harnesspkg.RuntimeViewCleanupPlanResult) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "check: %t\n", result.Check); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "ready: %t\n", result.Ready); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "changed: %t\n", result.Changed); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "cleanup_candidates:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.CleanupCandidates) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, candidate := range result.CleanupCandidates {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"  %s %s action=%s",
			candidate.Kind,
			candidate.Path,
			candidate.Action,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if candidate.Target != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " target=%s", candidate.Target); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
		if candidate.OrbitID != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " orbit=%s", candidate.OrbitID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "changed_files:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.ChangedFiles) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, changedFile := range result.ChangedFiles {
		if changedFile.Target == "" {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"  %s action=%s\n",
				changedFile.Path,
				changedFile.Action,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			continue
		}
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"  %s %s action=%s blocks=%d\n",
			changedFile.Target,
			changedFile.Path,
			changedFile.Action,
			changedFile.BlockCount,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "skipped_targets:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.SkippedTargets) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, skippedTarget := range result.SkippedTargets {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"  %s %s reason=%s\n",
			skippedTarget.Target,
			skippedTarget.Path,
			skippedTarget.Reason,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if err := renderHyardViewStatusList(cmd, "blockers", result.Blockers); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "drift_diagnostics:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(result.DriftDiagnostics) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "  none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, diagnostic := range result.DriftDiagnostics {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"  %s %s",
			diagnostic.Kind,
			diagnostic.Path,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		if diagnostic.OrbitID != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " orbit=%s", diagnostic.OrbitID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
		if diagnostic.RecoveryCommand != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), " recovery=%q", diagnostic.RecoveryCommand); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), " %s\n", diagnostic.Message); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if err := renderHyardViewStatusList(cmd, "next_actions", result.NextActions); err != nil {
		return err
	}
	if len(result.Notes) > 0 {
		if err := renderHyardViewStatusList(cmd, "notes", result.Notes); err != nil {
			return err
		}
	}

	return nil
}
