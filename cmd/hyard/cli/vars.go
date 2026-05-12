package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type varsDoctorExitError struct {
	status string
}

func (err varsDoctorExitError) Error() string {
	return fmt.Sprintf("hyard vars doctor status: %s", err.status)
}

func (err varsDoctorExitError) ExitCode() int {
	return 1
}

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
		newVarsDoctorCommand(),
		newVarsExplainCommand(),
	)

	return cmd
}

func newVarsDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose Runtime Bindings resolution problems",
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
			result, err := harnesspkg.DoctorRuntimeBindings(cmd.Context(), harnesspkg.RuntimeBindingsDoctorInput{
				RepoRoot: resolved.Repo.Root,
			})
			if err != nil {
				return fmt.Errorf("doctor Runtime Bindings: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				if err := emitHyardJSON(cmd, result); err != nil {
					return err
				}
			} else if err := emitVarsDoctorText(cmd, result); err != nil {
				return err
			}

			if result.Status == harnesspkg.RuntimeBindingsDoctorStatusError {
				return varsDoctorExitError{status: result.Status}
			}

			return nil
		},
	}
	addHyardJSONFlag(cmd)

	return cmd
}

func newVarsExplainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <name>",
		Short: "Explain one Package Variable Runtime Binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}
			result, err := harnesspkg.ExplainRuntimeBinding(cmd.Context(), harnesspkg.RuntimeBindingExplainInput{
				RepoRoot: resolved.Repo.Root,
				Name:     args[0],
			})
			if err != nil {
				return fmt.Errorf("explain Runtime Binding: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, result)
			}

			return emitVarsExplainText(cmd, result)
		},
	}
	addHyardJSONFlag(cmd)

	return cmd
}

func emitVarsDoctorText(cmd *cobra.Command, result harnesspkg.RuntimeBindingsDoctorResult) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "status: %s\n", result.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", result.Path); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitVarsDiagnosticsText(cmd, "errors", result.Errors); err != nil {
		return err
	}
	return emitVarsDiagnosticsText(cmd, "warnings", result.Warnings)
}

func emitVarsDiagnosticsText(cmd *cobra.Command, heading string, diagnostics []harnesspkg.RuntimeBindingDiagnostic) error {
	if len(diagnostics) == 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: none\n", heading); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", heading); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, diagnostic := range diagnostics {
		scope := diagnostic.Scope
		if scope == "" {
			scope = "global"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- %s %s scope=%s: %s\n", diagnostic.Code, diagnostic.Variable, scope, diagnostic.Message); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	return nil
}

func emitVarsExplainText(cmd *cobra.Command, result harnesspkg.RuntimeBindingExplainResult) error {
	lines := []string{
		fmt.Sprintf("name: %s", result.Name),
		fmt.Sprintf("status: %s", result.Status),
	}
	if result.ValueSource != "" {
		lines = append(lines, fmt.Sprintf("value_source: %s", result.ValueSource))
	}
	if result.Value != "" {
		lines = append(lines, fmt.Sprintf("value: %s", result.Value))
	}
	lines = append(lines,
		fmt.Sprintf("required: %t", result.Required),
		fmt.Sprintf("sensitive: %t", result.Sensitive),
	)
	if result.SelectedScope != "" {
		lines = append(lines, fmt.Sprintf("selected_scope: %s", result.SelectedScope))
	}
	declaring := "none"
	if len(result.DeclaringOrbits) > 0 {
		declaring = strings.Join(result.DeclaringOrbits, ", ")
	}
	lines = append(lines, fmt.Sprintf("declaring_orbits: %s", declaring))

	for _, line := range lines {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", line); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	return nil
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
