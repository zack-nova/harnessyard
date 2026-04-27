package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	scopedpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/scoped"
)

// NewCommitCommand creates the orbit commit command.
func NewCommitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit only the current orbit scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			message, err := cmd.Flags().GetString("message")
			if err != nil {
				return fmt.Errorf("read commit message flag: %w", err)
			}
			message = strings.TrimSpace(message)
			if message == "" {
				return errorsNewRequiredFlag("message")
			}

			commandContext, err := loadCurrentOrbitCommandContext(cmd)
			if err != nil {
				return err
			}

			result, err := scopedpkg.Commit(
				cmd.Context(),
				commandContext.Repo,
				commandContext.Store,
				commandContext.Config,
				commandContext.Current,
				commandContext.Definition,
				message,
			)
			if err != nil {
				return fmt.Errorf("commit current orbit: %w", err)
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), result)
			}

			if err := printWarnings(cmd, result.WarningMessages); err != nil {
				return err
			}

			messageLine := "nothing to commit"
			if result.Committed {
				messageLine = fmt.Sprintf("committed orbit %s", result.CurrentOrbit)
			}

			if _, err := fmt.Fprintln(cmd.OutOrStdout(), messageLine); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().StringP("message", "m", "", "Commit message")
	addJSONFlag(cmd)
	if err := cmd.MarkFlagRequired("message"); err != nil {
		panic(err)
	}

	return cmd
}
