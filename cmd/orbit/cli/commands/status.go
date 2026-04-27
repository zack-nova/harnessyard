package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

type statusOutput struct {
	CurrentOrbit string                   `json:"current_orbit"`
	Snapshot     *statepkg.StatusSnapshot `json:"snapshot"`
}

// NewStatusCommand creates the orbit status command.
func NewStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show in-scope and out-of-scope changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}

			current, err := store.ReadCurrentOrbit()
			if err != nil {
				if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
					return renderStatus(cmd, statusOutput{CurrentOrbit: "none", Snapshot: nil})
				}
				return fmt.Errorf("read current orbit state: %w", err)
			}

			config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}

			definition, err := viewpkg.CurrentDefinition(config, current)
			if err != nil {
				return fmt.Errorf("resolve current orbit definition: %w", err)
			}

			result, err := viewpkg.Status(cmd.Context(), repo, store, config, current, definition)
			if err != nil {
				return fmt.Errorf("load orbit status: %w", err)
			}
			if err := printWarnings(cmd, result.WarningMessages); err != nil {
				return err
			}

			return renderStatus(cmd, statusOutput{
				CurrentOrbit: result.CurrentOrbit,
				Snapshot:     &result.Snapshot,
			})
		},
	}
	addJSONFlag(cmd)

	return cmd
}

func renderStatus(cmd *cobra.Command, output statusOutput) error {
	jsonOutput, err := wantJSON(cmd)
	if err != nil {
		return err
	}
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), output)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "current: %s\n", output.CurrentOrbit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if output.Snapshot == nil {
		return nil
	}

	if err := printPathChanges(cmd, "in-scope", output.Snapshot.InScope); err != nil {
		return err
	}
	if err := printPathChanges(cmd, "out-of-scope", output.Snapshot.OutOfScope); err != nil {
		return err
	}
	if err := printStringSection(cmd, "hidden-dirty-risk", output.Snapshot.HiddenDirtyRisk); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "safe-to-switch: %t\n", output.Snapshot.SafeToSwitch); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := printStringSection(cmd, "commit-warnings", output.Snapshot.CommitWarnings); err != nil {
		return err
	}

	return nil
}

func printPathChanges(cmd *cobra.Command, title string, changes []statepkg.PathChange) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(changes) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	for _, change := range changes {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", change.Code, change.Path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func printStringSection(cmd *cobra.Command, title string, values []string) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", title); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(values) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	for _, value := range values {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), value); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}
