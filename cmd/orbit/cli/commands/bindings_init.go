package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type bindingsInitSourceJSON struct {
	Kind   string `json:"kind"`
	Repo   string `json:"repo,omitempty"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

type bindingsInitVariableJSON struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

type bindingsInitBindingsJSON struct {
	SchemaVersion int                                 `json:"schema_version"`
	Variables     map[string]bindingsInitVariableJSON `json:"variables"`
}

type bindingsInitOutput struct {
	Source     bindingsInitSourceJSON   `json:"source"`
	OutputPath string                   `json:"output_path,omitempty"`
	Bindings   bindingsInitBindingsJSON `json:"bindings"`
}

// NewBindingsInitCommand creates the orbit bindings init command.
func NewBindingsInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <template-source>",
		Short: "Generate a fillable bindings skeleton from a local branch or external Git source",
		Long: "Generate a bindings skeleton from a local template branch or an external Git source\n" +
			"without modifying the current repository. By default the YAML skeleton is written to stdout.",
		Example: "" +
			"  orbit bindings init orbit-template/docs\n" +
			"  orbit bindings init orbit-template/docs --out .harness/vars.yaml\n" +
			"  orbit bindings init https://example.com/acme/templates.git --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			outputArg, err := cmd.Flags().GetString("out")
			if err != nil {
				return fmt.Errorf("read --out flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}

			sourceArg := args[0]
			localSource, err := templateSourceUsesLocalRevision(cmd, repo.Root, sourceArg)
			if err != nil {
				return err
			}

			var preview orbittemplate.BindingsInitPreview
			if localSource {
				if err := stageProgress(progress, "resolving local template source"); err != nil {
					return err
				}
				preview, err = orbittemplate.BuildLocalBindingsInitPreview(cmd.Context(), orbittemplate.LocalBindingsInitInput{
					RepoRoot:  repo.Root,
					SourceRef: sourceArg,
				})
			} else {
				if err := stageProgress(progress, "resolving external template source"); err != nil {
					return err
				}
				preview, err = orbittemplate.BuildRemoteBindingsInitPreview(cmd.Context(), orbittemplate.RemoteBindingsInitInput{
					RepoRoot:  repo.Root,
					RemoteURL: sourceArg,
				})
			}
			if err != nil {
				return fmt.Errorf("build bindings skeleton: %w", err)
			}
			if err := stageProgress(progress, "building bindings skeleton"); err != nil {
				return err
			}

			data, err := bindings.MarshalVarsFile(preview.Skeleton)
			if err != nil {
				return fmt.Errorf("encode bindings skeleton: %w", err)
			}

			outputPath, err := resolveBindingsOutputPath(cmd, outputArg)
			if err != nil {
				return err
			}
			if outputPath != "" {
				if err := stageProgress(progress, "writing bindings skeleton"); err != nil {
					return err
				}
				if err := contractutil.AtomicWriteFile(outputPath, data); err != nil {
					return fmt.Errorf("write bindings skeleton: %w", err)
				}
			}

			if jsonOutput {
				if err := emitJSON(cmd.OutOrStdout(), bindingsInitPayload(preview, outputPath)); err != nil {
					return err
				}
				if err := stageProgress(progress, "bindings init complete"); err != nil {
					return err
				}
				return nil
			}

			if outputPath != "" {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote bindings skeleton to %s\n", outputPath); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				if err := stageProgress(progress, "bindings init complete"); err != nil {
					return err
				}
				return nil
			}

			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if err := stageProgress(progress, "bindings init complete"); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String("out", "", "Write the generated bindings skeleton to a file instead of stdout")
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

func resolveBindingsOutputPath(cmd *cobra.Command, outputArg string) (string, error) {
	trimmed := strings.TrimSpace(outputArg)
	if trimmed == "" {
		return "", nil
	}

	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed), nil
	}

	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}

	return filepath.Join(workingDir, filepath.FromSlash(trimmed)), nil
}

func bindingsInitPayload(preview orbittemplate.BindingsInitPreview, outputPath string) bindingsInitOutput {
	variables := make(map[string]bindingsInitVariableJSON, len(preview.Skeleton.Variables))
	for name, binding := range preview.Skeleton.Variables {
		variables[name] = bindingsInitVariableJSON{
			Value:       binding.Value,
			Description: binding.Description,
		}
	}

	return bindingsInitOutput{
		Source: bindingsInitSourceJSON{
			Kind:   preview.Source.SourceKind,
			Repo:   preview.Source.SourceRepo,
			Ref:    preview.Source.SourceRef,
			Commit: preview.Source.TemplateCommit,
		},
		OutputPath: outputPath,
		Bindings: bindingsInitBindingsJSON{
			SchemaVersion: preview.Skeleton.SchemaVersion,
			Variables:     variables,
		},
	}
}
