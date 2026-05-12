package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type varsPathOutput struct {
	Path string `json:"path"`
}

type varsValidateOutput struct {
	Path  string `json:"path"`
	Valid bool   `json:"valid"`
}

func newVarsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "Manage Runtime Bindings",
		Long: "Manage Runtime Bindings for Package Variables.\n" +
			"The canonical Runtime Bindings file is .harness/vars.yaml.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newVarsPathCommand(),
		newVarsValidateCommand(),
	)

	return cmd
}

func newVarsPathCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print the canonical Runtime Bindings path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, varsPathOutput{Path: harnesspkg.VarsRepoPath()})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", harnesspkg.VarsRepoPath()); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addHyardJSONFlag(cmd)

	return cmd
}

func newVarsValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the canonical Runtime Bindings file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}
			if err := harnesspkg.ValidateVarsFile(resolved.Repo.Root); err != nil {
				return fmt.Errorf("validate Runtime Bindings file: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, varsValidateOutput{Path: harnesspkg.VarsRepoPath(), Valid: true})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "validated Runtime Bindings file: %s\n", harnesspkg.VarsRepoPath()); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addHyardJSONFlag(cmd)

	return cmd
}
