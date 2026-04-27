package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

// NewLeaveCommand creates the orbit leave command.
func NewLeaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Restore the full tracked workspace view",
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

			result, err := viewpkg.Leave(cmd.Context(), repo, store)
			if err != nil {
				return fmt.Errorf("leave orbit: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), result)
			}

			if err := printWarnings(cmd, result.WarningMessages); err != nil {
				return err
			}

			message := "no current orbit"
			switch {
			case result.Orbit != "":
				message = fmt.Sprintf("left orbit %s", result.Orbit)
			case result.ProjectionRestored:
				message = "restored full workspace view"
			case result.StateCleared:
				message = "cleared stale orbit state"
			}

			if _, err := fmt.Fprintln(cmd.OutOrStdout(), message); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
