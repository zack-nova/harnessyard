package cli

import (
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
		newViewStatusCommand(),
	)

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
