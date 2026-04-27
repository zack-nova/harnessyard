package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/branchinfo"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type branchStatusOutput struct {
	Branch         string                    `json:"branch"`
	Detached       bool                      `json:"detached"`
	HeadExists     bool                      `json:"head_exists"`
	Classification branchinfo.Classification `json:"classification"`
}

// NewBranchStatusCommand creates the orbit branch status command.
func NewBranchStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Classify the current branch as template, runtime, or plain",
		Long: "Classify the current checkout worktree using Orbit's file-contract rules rather than branch naming or historical HEAD snapshots.\n" +
			"Use `orbit branch inspect <rev>` when you want a revision-scoped answer for a specific branch or commit.",
		Example: "" +
			"  orbit branch status\n" +
			"  orbit branch status --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			state, err := orbittemplate.LoadCurrentRepoState(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("load current repo state: %w", err)
			}

			classification, err := branchinfo.ClassifyCurrentWorktree(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("classify current worktree: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), branchStatusOutput{
					Branch:         state.CurrentBranch,
					Detached:       state.Detached,
					HeadExists:     state.HeadExists,
					Classification: classification,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "kind: %s\nreason: %s\n", classification.Kind, classification.Reason); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)

	return cmd
}
