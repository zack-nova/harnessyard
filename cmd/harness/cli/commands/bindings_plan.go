package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type bindingsPlanSourceJSON struct {
	Kind    string `json:"kind"`
	Repo    string `json:"repo,omitempty"`
	Ref     string `json:"ref"`
	Commit  string `json:"commit"`
	OrbitID string `json:"orbit_id"`
}

type bindingsPlanVariableJSON struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type bindingsPlanBindingsJSON struct {
	SchemaVersion   int                                        `json:"schema_version"`
	Variables       map[string]bindingsPlanVariableJSON        `json:"variables"`
	ScopedVariables map[string]bindingsPlanScopedVariablesJSON `json:"scoped_variables,omitempty"`
}

type bindingsPlanScopedVariablesJSON struct {
	Variables map[string]bindingsPlanVariableJSON `json:"variables"`
}

type bindingsPlanOutput struct {
	RepoRoot        string                   `json:"repo_root"`
	SourceCount     int                      `json:"source_count"`
	Sources         []bindingsPlanSourceJSON `json:"sources"`
	OutputPath      string                   `json:"output_path,omitempty"`
	ReusedValues    []string                 `json:"reused_values,omitempty"`
	MissingRequired []string                 `json:"missing_required,omitempty"`
	Bindings        bindingsPlanBindingsJSON `json:"bindings"`
}

// NewBindingsPlanCommand creates the harness bindings plan command.
func NewBindingsPlanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan <template-source>...",
		Short: "Plan one shared bindings skeleton for multiple template sources",
		Long: "Build one shared bindings skeleton for multiple orbit template sources.\n" +
			"The command reuses current runtime values from .harness/vars.yaml when available\n" +
			"and merges variable declarations into one shared bindings skeleton.",
		Example: "" +
			"  harness bindings plan orbit-template/docs orbit-template/cmd\n" +
			"  harness bindings plan orbit-template/docs orbit-template/cmd --out .harness/vars.yaml\n" +
			"  harness bindings plan orbit-template/docs orbit-template/cmd --json\n",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}
			outputArg, err := cmd.Flags().GetString("out")
			if err != nil {
				return fmt.Errorf("read --out flag: %w", err)
			}

			repoVars, err := loadOptionalBindingsPlanRepoVars(cmd.Context(), resolved.Repo.Root)
			if err != nil {
				return err
			}
			previews, err := buildBindingsPlanPreviews(cmd.Context(), resolved.Repo.Root, args, progress.Stage)
			if err != nil {
				return err
			}

			if err := stageProgress(progress, "merging bindings plan"); err != nil {
				return err
			}
			result, err := harnesspkg.BuildBindingsPlan(previews, repoVars)
			if err != nil {
				return fmt.Errorf("build bindings plan: %w", err)
			}

			data, err := bindings.MarshalVarsFile(result.Bindings)
			if err != nil {
				return fmt.Errorf("encode bindings plan: %w", err)
			}

			outputPath := ""
			if strings.TrimSpace(outputArg) != "" {
				outputPath, err = absolutePathFromArg(cmd, outputArg)
				if err != nil {
					return err
				}
				if err := stageProgress(progress, "writing bindings plan"); err != nil {
					return err
				}
				if _, err := bindings.WriteVarsFileAtPath(outputPath, result.Bindings); err != nil {
					return fmt.Errorf("write bindings plan: %w", err)
				}
			}

			if jsonOutput {
				if err := emitJSON(cmd.OutOrStdout(), bindingsPlanPayload(resolved.Repo.Root, outputPath, result)); err != nil {
					return err
				}
				if err := stageProgress(progress, "bindings plan complete"); err != nil {
					return err
				}
				return nil
			}
			if outputPath != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote bindings plan to %s\n", outputPath); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				if err := stageProgress(progress, "bindings plan complete"); err != nil {
					return err
				}
				return nil
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if err := stageProgress(progress, "bindings plan complete"); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String("out", "", "Write the merged shared bindings skeleton to a file instead of stdout")
	addPathFlag(cmd)
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

func buildBindingsPlanPreviews(
	ctx context.Context,
	repoRoot string,
	sources []string,
	progress func(string) error,
) ([]orbittemplate.BindingsInitPreview, error) {
	previews := make([]orbittemplate.BindingsInitPreview, 0, len(sources))
	for index, sourceArg := range sources {
		if err := stageProgressFunc(progress, fmt.Sprintf("preflighting source %d/%d", index+1, len(sources))); err != nil {
			return nil, err
		}
		localSource, err := installSourceUsesLocalRevision(ctx, repoRoot, sourceArg)
		if err != nil {
			return nil, err
		}

		if localSource {
			preview, err := orbittemplate.BuildLocalBindingsInitPreview(ctx, orbittemplate.LocalBindingsInitInput{
				RepoRoot:  repoRoot,
				SourceRef: sourceArg,
			})
			if err != nil {
				return nil, fmt.Errorf("build bindings preview for %q: %w", sourceArg, err)
			}
			previews = append(previews, preview)
			continue
		}

		preview, err := orbittemplate.BuildRemoteBindingsInitPreview(ctx, orbittemplate.RemoteBindingsInitInput{
			RepoRoot:  repoRoot,
			RemoteURL: sourceArg,
		})
		if err != nil {
			return nil, fmt.Errorf("build bindings preview for %q: %w", sourceArg, err)
		}
		previews = append(previews, preview)
	}

	return previews, nil
}

func loadOptionalBindingsPlanRepoVars(ctx context.Context, repoRoot string) (bindings.VarsFile, error) {
	empty := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
	}

	if _, err := os.Stat(harnesspkg.VarsPath(repoRoot)); err == nil {
		file, loadErr := harnesspkg.LoadVarsFile(repoRoot)
		if loadErr != nil {
			return bindings.VarsFile{}, fmt.Errorf("load harness vars from worktree: %w", loadErr)
		}
		return file, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{}, fmt.Errorf("stat harness vars in worktree: %w", err)
	}

	existsAtHead, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", harnesspkg.VarsRepoPath())
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("check harness vars at HEAD: %w", err)
	}
	if !existsAtHead {
		return empty, nil
	}

	file, err := harnesspkg.LoadVarsFileWorktreeOrHEAD(ctx, repoRoot)
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("load harness vars from worktree or HEAD: %w", err)
	}

	return file, nil
}

func bindingsPlanPayload(repoRoot string, outputPath string, result harnesspkg.BindingsPlanResult) bindingsPlanOutput {
	sources := make([]bindingsPlanSourceJSON, 0, len(result.Sources))
	for _, source := range result.Sources {
		sources = append(sources, bindingsPlanSourceJSON{
			Kind:    source.Kind,
			Repo:    source.Repo,
			Ref:     source.Ref,
			Commit:  source.Commit,
			OrbitID: source.OrbitID,
		})
	}

	variables := make(map[string]bindingsPlanVariableJSON, len(result.Bindings.Variables))
	for name, binding := range result.Bindings.Variables {
		variables[name] = bindingsPlanVariableJSON{
			Value:       binding.Value,
			Description: binding.Description,
		}
	}
	scopedVariables := make(map[string]bindingsPlanScopedVariablesJSON, len(result.Bindings.ScopedVariables))
	for namespace, scoped := range result.Bindings.ScopedVariables {
		variables := make(map[string]bindingsPlanVariableJSON, len(scoped.Variables))
		for name, binding := range scoped.Variables {
			variables[name] = bindingsPlanVariableJSON{
				Value:       binding.Value,
				Description: binding.Description,
			}
		}
		scopedVariables[namespace] = bindingsPlanScopedVariablesJSON{Variables: variables}
	}
	if len(scopedVariables) == 0 {
		scopedVariables = nil
	}

	return bindingsPlanOutput{
		RepoRoot:        repoRoot,
		SourceCount:     len(result.Sources),
		Sources:         sources,
		OutputPath:      outputPath,
		ReusedValues:    append([]string(nil), result.ReusedValues...),
		MissingRequired: append([]string(nil), result.MissingRequired...),
		Bindings: bindingsPlanBindingsJSON{
			SchemaVersion:   result.Bindings.SchemaVersion,
			Variables:       variables,
			ScopedVariables: scopedVariables,
		},
	}
}
