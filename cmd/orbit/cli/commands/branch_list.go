package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/branchinfo"
)

type branchListOutput struct {
	Branches []branchinfo.ListedBranch `json:"branches"`
}

// NewBranchListCommand creates the orbit branch list command.
func NewBranchListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local branches with Orbit classification",
		Long: "List local branches and classify each one with the same Orbit rules used by branch status\n" +
			"and branch inspect. This command only lists local branches in Phase 2.",
		Example: "" +
			"  orbit branch list\n" +
			"  orbit branch list --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			listedBranches, err := branchinfo.ListLocalBranches(cmd.Context(), repo.Root)
			if err != nil {
				return fmt.Errorf("list local branches: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), branchListOutput{Branches: listedBranches})
			}

			for _, listedBranch := range listedBranches {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s\t%s\t%s\n",
					listedBranch.Name,
					listedBranch.Classification.Kind,
					listedBranch.Classification.Reason,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	addJSONFlag(cmd)

	return cmd
}
