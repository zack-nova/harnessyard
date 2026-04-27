package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	scopedpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/scoped"
)

type logOutput struct {
	CurrentOrbit string   `json:"current_orbit"`
	Paths        []string `json:"paths"`
	GitArgs      []string `json:"git_args"`
	Log          string   `json:"log"`
}

// NewLogCommand creates the orbit log command.
func NewLogCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [git-log-args...]",
		Short: "Show path-limited history for the current orbit",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			commandContext, err := loadCurrentOrbitCommandContext(cmd)
			if err != nil {
				return err
			}

			result, err := scopedpkg.Log(
				cmd.Context(),
				commandContext.Repo,
				commandContext.Store,
				commandContext.Config,
				commandContext.Current,
				commandContext.Definition,
				args,
			)
			if err != nil {
				return fmt.Errorf("load orbit log: %w", err)
			}

			output := logOutput{
				CurrentOrbit: result.CurrentOrbit,
				Paths:        result.Paths,
				GitArgs:      result.GitArgs,
				Log:          result.Log,
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), result.Log); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
