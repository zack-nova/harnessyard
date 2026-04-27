package commands

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const legacyTemplateApplyPreferredCommand = "harness install"

type templateApplySourceJSON struct {
	Kind   string `json:"kind"`
	Repo   string `json:"repo,omitempty"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
}

type templateApplyBindingJSON struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Namespace string `json:"namespace,omitempty"`
}

type templateApplyManifestJSON struct {
	OrbitID           string `json:"orbit_id"`
	DefaultTemplate   bool   `json:"default_template"`
	CreatedFromBranch string `json:"created_from_branch"`
	CreatedFromCommit string `json:"created_from_commit"`
	VariableCount     int    `json:"variable_count"`
}

type templateApplyPreviewJSON struct {
	DryRun            bool                          `json:"dry_run"`
	OverwriteExisting bool                          `json:"overwrite_existing"`
	Source            templateApplySourceJSON       `json:"source"`
	OrbitID           string                        `json:"orbit_id"`
	Manifest          templateApplyManifestJSON     `json:"manifest"`
	Bindings          []templateApplyBindingJSON    `json:"bindings"`
	Files             []string                      `json:"files"`
	Warnings          []string                      `json:"warnings,omitempty"`
	Conflicts         []orbittemplate.ApplyConflict `json:"conflicts"`
}

type templateApplyResultJSON struct {
	DryRun       bool                    `json:"dry_run"`
	Source       templateApplySourceJSON `json:"source"`
	OrbitID      string                  `json:"orbit_id"`
	WrittenPaths []string                `json:"written_paths"`
	Warnings     []string                `json:"warnings,omitempty"`
}

// NewTemplateApplyCommand creates the orbit template apply command.
func NewTemplateApplyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply <template-branch|git-source>",
		Short: "Legacy compatibility wrapper for harness install",
		Long: "Legacy compatibility wrapper around `harness install`.\n" +
			"Prefer `harness install` for new runtime installs.\n" +
			"This hidden command remains available so existing internal bridges can still target the shared apply pipeline.\n" +
			"Use a local template branch name directly, or pass an external Git source. --ref is only valid for external Git sources.",
		Example: "" +
			"  harness install orbit-template/docs --bindings .harness/vars.yaml\n" +
			"  harness install https://example.com/acme/templates.git --ref orbit-template/docs --bindings .harness/vars.yaml\n" +
			"  harness install orbit-template/docs --overwrite-existing --bindings .harness/vars.yaml --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			bindingsPath, err := cmd.Flags().GetString("bindings")
			if err != nil {
				return fmt.Errorf("read --bindings flag: %w", err)
			}
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read --dry-run flag: %w", err)
			}
			overwriteExisting, err := cmd.Flags().GetBool("overwrite-existing")
			if err != nil {
				return fmt.Errorf("read --overwrite-existing flag: %w", err)
			}
			allowUnresolvedBindings, err := templateApplyAllowUnresolvedBindingsFromFlags(cmd)
			if err != nil {
				return err
			}
			interactive, err := cmd.Flags().GetBool("interactive")
			if err != nil {
				return fmt.Errorf("read --interactive flag: %w", err)
			}
			editorMode, err := cmd.Flags().GetBool("editor")
			if err != nil {
				return fmt.Errorf("read --editor flag: %w", err)
			}
			requestedRef, err := cmd.Flags().GetString("ref")
			if err != nil {
				return fmt.Errorf("read --ref flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			sourceArg := args[0]
			prompter := buildTemplateApplyPrompter(cmd, interactive)
			var editor orbittemplate.Editor
			if editorMode {
				editor, err = orbittemplate.NewEnvironmentEditor()
				if err != nil {
					return fmt.Errorf("configure bindings editor: %w", err)
				}
			}
			localSource, err := templateSourceUsesLocalRevision(cmd, repo.Root, sourceArg)
			if err != nil {
				return err
			}

			if localSource {
				if strings.TrimSpace(requestedRef) != "" {
					return fmt.Errorf("--ref is only supported when applying from an external Git source")
				}

				previewInput := orbittemplate.TemplateApplyPreviewInput{
					RepoRoot:                repo.Root,
					SourceRef:               sourceArg,
					BindingsFilePath:        bindingsPath,
					OverwriteExisting:       overwriteExisting,
					AllowUnresolvedBindings: allowUnresolvedBindings,
					Interactive:             interactive,
					Prompter:                prompter,
					EditorMode:              editorMode,
					Editor:                  editor,
					Now:                     time.Now().UTC(),
				}

				if dryRun {
					preview, err := orbittemplate.BuildTemplateApplyPreview(cmd.Context(), previewInput)
					if err != nil {
						return fmt.Errorf("build template apply preview: %w", err)
					}
					return emitTemplateApplyPreview(cmd, preview, jsonOutput, overwriteExisting)
				}

				result, err := orbittemplate.ApplyLocalTemplate(cmd.Context(), orbittemplate.TemplateApplyInput{
					Preview: previewInput,
				})
				if err != nil {
					return fmt.Errorf("apply template branch: %w", err)
				}
				return emitTemplateApplyResult(cmd, result, jsonOutput)
			}

			previewInput := orbittemplate.RemoteTemplateApplyPreviewInput{
				RepoRoot:                repo.Root,
				RemoteURL:               sourceArg,
				RequestedRef:            requestedRef,
				BindingsFilePath:        bindingsPath,
				OverwriteExisting:       overwriteExisting,
				AllowUnresolvedBindings: allowUnresolvedBindings,
				Interactive:             interactive,
				Prompter:                prompter,
				EditorMode:              editorMode,
				Editor:                  editor,
				Now:                     time.Now().UTC(),
			}

			if dryRun {
				preview, err := orbittemplate.BuildRemoteTemplateApplyPreview(cmd.Context(), previewInput)
				if err != nil {
					return fmt.Errorf("build template apply preview: %w", err)
				}
				return emitTemplateApplyPreview(cmd, preview, jsonOutput, overwriteExisting)
			}

			result, err := orbittemplate.ApplyRemoteTemplate(cmd.Context(), orbittemplate.RemoteTemplateApplyInput{
				Preview: previewInput,
			})
			if err != nil {
				return fmt.Errorf("apply template branch: %w", err)
			}

			return emitTemplateApplyResult(cmd, result, jsonOutput)
		},
	}
	cmd.Hidden = true

	cmd.Flags().String("bindings", "", "Path to an explicit bindings YAML file")
	cmd.Flags().String("ref", "", "Select one template branch explicitly when applying from an external Git source")
	cmd.Flags().Bool("overwrite-existing", false, "Allow overwriting conflicting runtime files")
	cmd.Flags().Bool("allow-unresolved-bindings", false, "Compatibility no-op: unresolved required bindings are preserved by default")
	cmd.Flags().Bool("strict-bindings", false, "Fail when required bindings are unresolved instead of preserving placeholders")
	cmd.Flags().Bool("dry-run", false, "Preview template apply without writing files")
	cmd.Flags().Bool("interactive", false, "Prompt for missing bindings interactively")
	cmd.Flags().Bool("editor", false, "Open an editor-backed bindings skeleton for missing required values")
	addJSONFlag(cmd)

	return cmd
}

func templateApplyAllowUnresolvedBindingsFromFlags(cmd *cobra.Command) (bool, error) {
	allowUnresolvedBindings, err := cmd.Flags().GetBool("allow-unresolved-bindings")
	if err != nil {
		return false, fmt.Errorf("read --allow-unresolved-bindings flag: %w", err)
	}
	strictBindings, err := cmd.Flags().GetBool("strict-bindings")
	if err != nil {
		return false, fmt.Errorf("read --strict-bindings flag: %w", err)
	}
	if allowUnresolvedBindings && strictBindings {
		return false, fmt.Errorf("--strict-bindings cannot be used with --allow-unresolved-bindings")
	}
	return !strictBindings, nil
}

func emitTemplateApplyPreview(cmd *cobra.Command, preview orbittemplate.TemplateApplyPreview, jsonOutput bool, overwriteExisting bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), templateApplyPreviewPayload(preview, overwriteExisting))
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "legacy orbit template apply dry-run from %s\n", preview.Source.Ref); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "preferred_command: %s\n", legacyTemplateApplyPreferredCommand); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_ref: %s\n", preview.InstallRecord.Template.SourceRef); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_kind: %s\n", preview.InstallRecord.Template.SourceKind); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if strings.TrimSpace(preview.InstallRecord.Template.SourceRepo) != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_repo: %s\n", preview.InstallRecord.Template.SourceRepo); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "source_commit: %s\n", preview.Source.Commit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_id: %s\n", preview.Source.Manifest.Template.OrbitID); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "manifest:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "default_template: %t\n", preview.Source.Manifest.Template.DefaultTemplate); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created_from_branch: %s\n", preview.Source.Manifest.Template.CreatedFromBranch); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created_from_commit: %s\n", preview.Source.Manifest.Template.CreatedFromCommit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "variable_count: %d\n", len(preview.Source.Manifest.Variables)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "overwrite_existing: %t\n", overwriteExisting); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitTemplateApplyWarnings(cmd, preview.Warnings); err != nil {
		return err
	}

	if len(preview.ResolvedBindings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "bindings:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		names := make([]string, 0, len(preview.ResolvedBindings))
		for name := range preview.ResolvedBindings {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			resolved := preview.ResolvedBindings[name]
			namespace := ""
			if resolved.Namespace != "" {
				namespace = fmt.Sprintf(" (namespace: %s)", resolved.Namespace)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s <- %s%s\n", name, stringifyBindingSource(resolved.Source), namespace); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "files:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, path := range templateApplyPreviewPaths(preview) {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if len(preview.Conflicts) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "conflicts:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, conflict := range preview.Conflicts {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", conflict.Path, conflict.Message); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}

func emitTemplateApplyResult(cmd *cobra.Command, result orbittemplate.TemplateApplyResult, jsonOutput bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), templateApplyResultPayload(result))
	}
	if err := emitTemplateApplyWarnings(cmd, result.Preview.Warnings); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"legacy orbit template apply installed orbit %s from %s\npreferred_command: %s\nfiles: %d\n",
		result.Preview.Source.Manifest.Template.OrbitID,
		result.Preview.Source.Ref,
		legacyTemplateApplyPreferredCommand,
		len(result.WrittenPaths),
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	return nil
}

func emitTemplateApplyWarnings(cmd *cobra.Command, warnings []string) error {
	if len(warnings) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "warnings: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "warnings:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, warning := range warnings {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), warning); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func templateSourceUsesLocalRevision(cmd *cobra.Command, repoRoot string, source string) (bool, error) {
	exists, err := gitpkg.RevisionExists(cmd.Context(), repoRoot, source)
	if err != nil {
		return false, fmt.Errorf("check local template source %q: %w", source, err)
	}

	return exists, nil
}

func buildTemplateApplyPrompter(cmd *cobra.Command, interactive bool) orbittemplate.BindingPrompter {
	if !interactive {
		return nil
	}

	return orbittemplate.LineBindingPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
}

func templateApplyPreviewPayload(preview orbittemplate.TemplateApplyPreview, overwriteExisting bool) templateApplyPreviewJSON {
	return templateApplyPreviewJSON{
		DryRun:            true,
		OverwriteExisting: overwriteExisting,
		Source:            templateApplySourcePayload(preview),
		OrbitID:           preview.Source.Manifest.Template.OrbitID,
		Manifest:          templateApplyManifestPayload(preview),
		Bindings:          templateApplyBindingsPayload(preview.ResolvedBindings),
		Files:             templateApplyPreviewPaths(preview),
		Warnings:          append([]string(nil), preview.Warnings...),
		Conflicts:         preview.Conflicts,
	}
}

func templateApplyResultPayload(result orbittemplate.TemplateApplyResult) templateApplyResultJSON {
	return templateApplyResultJSON{
		DryRun:       false,
		Source:       templateApplySourcePayload(result.Preview),
		OrbitID:      result.Preview.Source.Manifest.Template.OrbitID,
		WrittenPaths: result.WrittenPaths,
		Warnings:     append([]string(nil), result.Preview.Warnings...),
	}
}

func templateApplySourcePayload(preview orbittemplate.TemplateApplyPreview) templateApplySourceJSON {
	return templateApplySourceJSON{
		Kind:   preview.InstallRecord.Template.SourceKind,
		Repo:   preview.InstallRecord.Template.SourceRepo,
		Ref:    preview.InstallRecord.Template.SourceRef,
		Commit: preview.Source.Commit,
	}
}

func templateApplyManifestPayload(preview orbittemplate.TemplateApplyPreview) templateApplyManifestJSON {
	return templateApplyManifestJSON{
		OrbitID:           preview.Source.Manifest.Template.OrbitID,
		DefaultTemplate:   preview.Source.Manifest.Template.DefaultTemplate,
		CreatedFromBranch: preview.Source.Manifest.Template.CreatedFromBranch,
		CreatedFromCommit: preview.Source.Manifest.Template.CreatedFromCommit,
		VariableCount:     len(preview.Source.Manifest.Variables),
	}
}

func templateApplyBindingsPayload(resolvedBindings map[string]bindings.ResolvedBinding) []templateApplyBindingJSON {
	names := make([]string, 0, len(resolvedBindings))
	for name := range resolvedBindings {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]templateApplyBindingJSON, 0, len(names))
	for _, name := range names {
		resolved := resolvedBindings[name]
		items = append(items, templateApplyBindingJSON{
			Name:      name,
			Source:    stringifyBindingSource(resolved.Source),
			Namespace: resolved.Namespace,
		})
	}

	return items
}

func templateApplyPreviewPaths(preview orbittemplate.TemplateApplyPreview) []string {
	paths := make([]string, 0, len(preview.RenderedFiles)+4)
	for _, file := range preview.RenderedFiles {
		paths = append(paths, file.Path)
	}
	if preview.RenderedSharedAgentsFile != nil {
		paths = append(paths, preview.RenderedSharedAgentsFile.Path)
	}
	paths = append(paths,
		fmt.Sprintf(".harness/orbits/%s.yaml", preview.Source.Manifest.Template.OrbitID),
		fmt.Sprintf(".harness/installs/%s.yaml", preview.Source.Manifest.Template.OrbitID),
		".harness/vars.yaml",
	)

	return paths
}

func stringifyBindingSource(source bindings.MergeSource) string {
	value := string(source)
	if value == "" {
		return "unknown"
	}

	return strings.TrimSpace(value)
}
