package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// NewEnterCommand creates the orbit enter command.
func NewEnterCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enter <orbit-id>",
		Short: "Project an orbit into the current workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}

			definition, err := definitionByID(config, args[0])
			if err != nil {
				return err
			}

			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}
			readinessWarnings := harnessReadinessEnterWarnings(cmd, repo.Root)

			result, err := viewpkg.Enter(cmd.Context(), repo, store, config, definition)
			if err != nil {
				result.WarningMessages = append(result.WarningMessages, readinessWarnings...)
				if !jsonOutput {
					if warningErr := printWarnings(cmd, result.WarningMessages); warningErr != nil {
						return warningErr
					}
				}
				return fmt.Errorf("enter orbit: %w", err)
			}
			result.WarningMessages = append(result.WarningMessages, readinessWarnings...)

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), result)
			}

			if err := printWarnings(cmd, result.WarningMessages); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "entered orbit %s (%d file(s))\n", result.Orbit, result.ScopeCount); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}

func printWarnings(cmd *cobra.Command, warnings []string) error {
	for _, warningMessage := range warnings {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warningMessage); err != nil {
			return fmt.Errorf("write warning output: %w", err)
		}
	}

	return nil
}

func harnessReadinessEnterWarnings(cmd *cobra.Command, repoRoot string) []string {
	manifestPath := harnesspkg.ManifestPath(repoRoot)
	if _, err := os.Stat(manifestPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []string{"harness runtime readiness could not be evaluated; run `hyard check --json` before worker handoff"}
	}

	report, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), repoRoot)
	if err != nil {
		return []string{"harness runtime readiness could not be evaluated; run `hyard check --json` before worker handoff"}
	}
	if report.Status == harnesspkg.ReadinessStatusReady {
		return nil
	}

	return []string{
		fmt.Sprintf("harness runtime readiness is %s; run `hyard ready` for detailed reasons before worker handoff", report.Status),
	}
}
