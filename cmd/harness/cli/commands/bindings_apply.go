package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type bindingsApplyJSON struct {
	DryRun       bool                       `json:"dry_run"`
	HarnessRoot  string                     `json:"harness_root"`
	HarnessID    string                     `json:"harness_id"`
	OrbitID      string                     `json:"orbit_id"`
	Forced       bool                       `json:"forced"`
	ChangedCount int                        `json:"changed_count"`
	ChangedPaths []string                   `json:"changed_paths"`
	WrittenCount int                        `json:"written_count"`
	WrittenPaths []string                   `json:"written_paths"`
	WarningCount int                        `json:"warning_count"`
	Warnings     []string                   `json:"warnings,omitempty"`
	Readiness    harnesspkg.ReadinessReport `json:"readiness"`
}

// NewBindingsApplyCommand creates the harness bindings apply command.
func NewBindingsApplyCommand() *cobra.Command {
	var orbitID string
	var force bool
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Reapply current .harness/vars.yaml to one install-backed orbit",
		Long: "Reapply the current .harness/vars.yaml file to one install-backed orbit.\n" +
			"This command rebuilds the orbit from its recorded source pin, previews drift, and only rewrites install-owned paths.",
		Example: "" +
			"  harness bindings apply --orbit docs --dry-run\n" +
			"  harness bindings apply --orbit docs\n" +
			"  harness bindings apply --orbit docs --force\n",
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
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}

			input := harnesspkg.BindingsApplyInput{
				RepoRoot: resolved.Repo.Root,
				OrbitID:  orbitID,
				Force:    force,
				Now:      time.Now().UTC(),
				Progress: progress.Stage,
			}

			if dryRun {
				result, err := harnesspkg.PreviewBindingsApply(cmd.Context(), input)
				if err != nil {
					return fmt.Errorf("preview bindings apply: %w", err)
				}

				payload := bindingsApplyJSON{
					DryRun:       true,
					HarnessRoot:  resolved.Repo.Root,
					HarnessID:    result.HarnessID,
					OrbitID:      result.OrbitID,
					Forced:       result.Forced,
					ChangedCount: len(result.ChangedPaths),
					ChangedPaths: result.ChangedPaths,
					WarningCount: len(result.Warnings),
					Warnings:     result.Warnings,
				}
				readiness, err := evaluateCommandReadiness(cmd.Context(), resolved.Repo.Root)
				if err != nil {
					return err
				}
				payload.Readiness = readiness
				if jsonOutput {
					return emitJSON(cmd.OutOrStdout(), payload)
				}

				_, err = fmt.Fprintf(
					cmd.OutOrStdout(),
					"previewed bindings apply for orbit %s in harness %s\nchanged: %d\nwritten: 0\nwarnings: %d\n",
					payload.OrbitID,
					payload.HarnessRoot,
					payload.ChangedCount,
					payload.WarningCount,
				)
				if err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
					return err
				}
				return nil
			}

			result, err := harnesspkg.ApplyBindings(cmd.Context(), input)
			if err != nil {
				return fmt.Errorf("apply bindings: %w", err)
			}

			payload := bindingsApplyJSON{
				DryRun:       false,
				HarnessRoot:  resolved.Repo.Root,
				HarnessID:    result.Preview.HarnessID,
				OrbitID:      result.Preview.OrbitID,
				Forced:       result.Preview.Forced,
				ChangedCount: len(result.Preview.ChangedPaths),
				ChangedPaths: result.Preview.ChangedPaths,
				WrittenCount: len(result.WrittenPaths),
				WrittenPaths: result.WrittenPaths,
				WarningCount: len(result.Preview.Warnings),
				Warnings:     result.Preview.Warnings,
			}
			readiness, err := evaluateCommandReadiness(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return err
			}
			payload.Readiness = readiness
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), payload)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"applied bindings to orbit %s in harness %s\nchanged: %d\nwritten: %d\nwarnings: %d\n",
				payload.OrbitID,
				payload.HarnessRoot,
				payload.ChangedCount,
				payload.WrittenCount,
				payload.WarningCount,
			)
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Apply current bindings to one install-backed orbit")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite drifted install-owned paths when reapplying bindings")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview bindings apply without writing runtime files")
	addPathFlag(cmd)
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}
