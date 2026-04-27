package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	scopedpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/scoped"
)

type diffOutput struct {
	CurrentOrbit string   `json:"current_orbit"`
	Outside      bool     `json:"outside"`
	Paths        []string `json:"paths"`
	Diff         string   `json:"diff"`
}

// NewDiffCommand creates the orbit diff command.
func NewDiffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show the diff for the current orbit scope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			commandContext, err := loadCurrentOrbitCommandContext(cmd)
			if err != nil {
				return err
			}

			outside, err := cmd.Flags().GetBool("outside")
			if err != nil {
				return fmt.Errorf("read outside flag: %w", err)
			}

			result, err := scopedpkg.Diff(
				cmd.Context(),
				commandContext.Repo,
				commandContext.Store,
				commandContext.Config,
				commandContext.Current,
				commandContext.Definition,
				scopedpkg.DiffOptions{Outside: outside},
			)
			if err != nil {
				return fmt.Errorf("load orbit diff: %w", err)
			}

			output := diffOutput{
				CurrentOrbit: result.CurrentOrbit,
				Outside:      result.Outside,
				Paths:        result.Paths,
				Diff:         result.Diff,
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), result.Diff); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().Bool("outside", false, "Show out-of-scope changes")
	addJSONFlag(cmd)

	return cmd
}
