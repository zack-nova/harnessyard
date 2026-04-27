package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	scopedpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/scoped"
)

// NewRestoreCommand creates the orbit restore command.
func NewRestoreCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore the current orbit scope to a revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			revision, err := cmd.Flags().GetString("to")
			if err != nil {
				return fmt.Errorf("read restore target flag: %w", err)
			}
			revision = strings.TrimSpace(revision)
			if revision == "" {
				return errorsNewRequiredFlag("to")
			}

			allowDeleteCurrentOrbit, err := cmd.Flags().GetBool("allow-delete-current-orbit")
			if err != nil {
				return fmt.Errorf("read allow-delete-current-orbit flag: %w", err)
			}

			commandContext, err := loadCurrentOrbitCommandContext(cmd)
			if err != nil {
				return err
			}

			result, err := scopedpkg.Restore(
				cmd.Context(),
				commandContext.Repo,
				commandContext.Store,
				commandContext.Config,
				commandContext.Current,
				commandContext.Definition,
				revision,
				scopedpkg.RestoreOptions{
					AllowDeleteCurrentOrbit: allowDeleteCurrentOrbit,
				},
			)
			if err != nil {
				return fmt.Errorf("restore current orbit: %w", err)
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), result)
			}

			if err := printWarnings(cmd, result.WarningMessages); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "restored orbit %s to %s\n", result.CurrentOrbit, result.TargetRevision); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().String("to", "", "Revision to restore the current scope from")
	cmd.Flags().Bool("allow-delete-current-orbit", false, "Allow restore to remove the current orbit definition and automatically leave orbit view")
	addJSONFlag(cmd)
	if err := cmd.MarkFlagRequired("to"); err != nil {
		panic(err)
	}

	return cmd
}
