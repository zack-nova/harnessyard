package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
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

type varsInitOutput struct {
	Path            string   `json:"path"`
	Source          string   `json:"source"`
	VariableCount   int      `json:"variable_count"`
	MissingRequired []string `json:"missing_required,omitempty"`
	ReusedValues    []string `json:"reused_values,omitempty"`
}

func newVarsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vars",
		Short: "Manage Runtime Bindings",
		Long: "Manage Runtime Bindings for Package Variables.\n" +
			"The canonical Runtime Bindings file is .harness/vars.yaml.\n" +
			"Runtime Bindings use schema_version: 2 and satisfy Package Variables\n" +
			"referenced by strict {{ vars.<name> }} Package Template References.",
		Example: "" +
			"  hyard vars path\n" +
			"  hyard vars init <package-source> --out .harness/vars.yaml\n" +
			"  hyard vars init <package-source> --defaults\n" +
			"  hyard vars validate\n" +
			"  hyard vars doctor\n" +
			"  hyard vars explain project_name\n",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newVarsInitCommand(),
		newVarsPathCommand(),
		newVarsValidateCommand(),
		newVarsDoctorCommand(),
		newVarsExplainCommand(),
	)

	return cmd
}

func newVarsInitCommand() *cobra.Command {
	var outputPath string
	var requestedRef string
	var materializeDefaults bool

	cmd := &cobra.Command{
		Use:   "init <package-source>",
		Short: "Generate a Runtime Bindings skeleton",
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

			preview, err := buildVarsInitPreview(cmd, resolved.Repo.Root, args[0], requestedRef)
			if err != nil {
				return err
			}
			repoVars, err := loadVarsInitExistingFile(resolved.Repo.Root)
			if err != nil {
				return err
			}
			result, err := harnesspkg.BuildBindingsPlanWithOptions([]orbittemplate.BindingsInitPreview{preview}, repoVars, harnesspkg.BindingsPlanOptions{
				MaterializeDefaults: materializeDefaults,
			})
			if err != nil {
				return fmt.Errorf("build Runtime Bindings skeleton: %w", err)
			}

			destination := resolveVarsInitOutputPath(resolved.Repo.Root, outputPath)
			if _, err := bindings.WriteVarsFileAtPath(destination, result.Bindings); err != nil {
				return fmt.Errorf("write Runtime Bindings skeleton: %w", err)
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, varsInitOutput{
					Path:            outputPath,
					Source:          args[0],
					VariableCount:   len(result.Bindings.Variables),
					MissingRequired: result.MissingRequired,
					ReusedValues:    result.ReusedValues,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote Runtime Bindings skeleton to %s\n", outputPath); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&outputPath, "out", harnesspkg.VarsRepoPath(), "Runtime Bindings output path")
	cmd.Flags().StringVar(&requestedRef, "ref", "", "Git ref to read when package-source is a remote repository")
	cmd.Flags().BoolVar(&materializeDefaults, "defaults", false, "Materialize declaration defaults as inline values")
	addHyardJSONFlag(cmd)

	return cmd
}

func buildVarsInitPreview(cmd *cobra.Command, repoRoot string, source string, requestedRef string) (orbittemplate.BindingsInitPreview, error) {
	preview, localErr := orbittemplate.BuildLocalBindingsInitPreview(cmd.Context(), orbittemplate.LocalBindingsInitInput{
		RepoRoot:  repoRoot,
		SourceRef: source,
	})
	if localErr == nil {
		return preview, nil
	}

	preview, remoteErr := orbittemplate.BuildRemoteBindingsInitPreview(cmd.Context(), orbittemplate.RemoteBindingsInitInput{
		RepoRoot:     repoRoot,
		RemoteURL:    source,
		RequestedRef: requestedRef,
	})
	if remoteErr == nil {
		return preview, nil
	}

	return orbittemplate.BindingsInitPreview{}, fmt.Errorf(
		"resolve package source %q: %w",
		source,
		errors.Join(
			fmt.Errorf("local branch: %w", localErr),
			fmt.Errorf("remote source: %w", remoteErr),
		),
	)
}

func loadVarsInitExistingFile(repoRoot string) (bindings.VarsFile, error) {
	if _, err := os.Stat(harnesspkg.VarsPath(repoRoot)); err == nil {
		file, err := harnesspkg.LoadVarsFile(repoRoot)
		if err != nil {
			return bindings.VarsFile{}, fmt.Errorf("load %s: %w", harnesspkg.VarsRepoPath(), err)
		}
		return file, nil
	} else if !os.IsNotExist(err) {
		return bindings.VarsFile{}, fmt.Errorf("stat %s: %w", harnesspkg.VarsRepoPath(), err)
	}

	return bindings.VarsFile{
		SchemaVersion: bindings.VarsSchemaVersion,
		Variables:     map[string]bindings.VariableBinding{},
	}, nil
}

func resolveVarsInitOutputPath(repoRoot string, outputPath string) string {
	if filepath.IsAbs(outputPath) {
		return filepath.Clean(outputPath)
	}
	return filepath.Join(repoRoot, filepath.FromSlash(outputPath))
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
