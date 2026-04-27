package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templateSaveReplacementJSON struct {
	Variable string `json:"variable"`
	Literal  string `json:"literal"`
	Count    int    `json:"count"`
}

type templateSaveFileReplacementJSON struct {
	Path         string                        `json:"path"`
	Replacements []templateSaveReplacementJSON `json:"replacements"`
}

type templateSaveAmbiguityJSON struct {
	Literal   string   `json:"literal"`
	Variables []string `json:"variables"`
}

type templateSaveFileAmbiguityJSON struct {
	Path        string                      `json:"path"`
	Ambiguities []templateSaveAmbiguityJSON `json:"ambiguities"`
}

type templateSaveManifestJSON struct {
	OrbitID           string `json:"orbit_id"`
	DefaultTemplate   bool   `json:"default_template"`
	CreatedFromBranch string `json:"created_from_branch"`
	CreatedFromCommit string `json:"created_from_commit"`
	CreatedAt         string `json:"created_at"`
}

type templateSaveVariableJSON struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type templateSavePreviewJSON struct {
	DryRun       bool                              `json:"dry_run"`
	OrbitID      string                            `json:"orbit_id"`
	TargetBranch string                            `json:"target_branch"`
	Files        []string                          `json:"files"`
	Warnings     []string                          `json:"warnings,omitempty"`
	Replacements []templateSaveFileReplacementJSON `json:"replacements"`
	Ambiguities  []templateSaveFileAmbiguityJSON   `json:"ambiguities"`
	Manifest     templateSaveManifestJSON          `json:"manifest"`
	Variables    []templateSaveVariableJSON        `json:"variables"`
}

type templateSaveResultJSON struct {
	DryRun       bool     `json:"dry_run"`
	OrbitID      string   `json:"orbit_id"`
	TargetBranch string   `json:"target_branch"`
	Ref          string   `json:"ref"`
	Commit       string   `json:"commit"`
	Files        []string `json:"files"`
	Warnings     []string `json:"warnings,omitempty"`
}

// NewTemplateSaveCommand creates the orbit template save command.
func NewTemplateSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save <orbit-id>",
		Short: "Save one orbit as a template branch",
		Long: "Save one runtime orbit into a reusable template branch.\n" +
			"The command builds the template from the current repository state, does not switch branches,\n" +
			"and can preview the full write set with --dry-run before writing the target branch.\n" +
			"If --to is omitted, the command reuses the installed template source_ref for install_orbit members from .harness/installs/<orbit-id>.yaml.",
		Example: "" +
			"  orbit template save docs --dry-run\n" +
			"  orbit template save docs --to orbit-template/docs --dry-run\n" +
			"  orbit template save docs --backfill-brief --to orbit-template/docs\n" +
			"  orbit template save docs --to orbit-template/docs --allow-out-of-range-skills\n" +
			"  orbit template save docs --to orbit-template/docs --aggregate-detected-skills\n" +
			"  orbit template save docs --to orbit-template/docs --edit-template\n" +
			"  orbit template save docs --to orbit-template/docs --include-completed-bootstrap\n" +
			"  orbit template save docs --to orbit-template/docs --default --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}
			if err := ensureTemplateSaveRuntimeRevision(cmd.Context(), repo.Root); err != nil {
				return err
			}

			targetBranch, err := cmd.Flags().GetString("to")
			if err != nil {
				return fmt.Errorf("read --to flag: %w", err)
			}
			targetBranch, err = resolveTemplateSaveTargetBranch(repo.Root, args[0], targetBranch)
			if err != nil {
				return err
			}

			defaultTemplate, err := cmd.Flags().GetBool("default")
			if err != nil {
				return fmt.Errorf("read --default flag: %w", err)
			}
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read --dry-run flag: %w", err)
			}
			overwrite, err := cmd.Flags().GetBool("overwrite")
			if err != nil {
				return fmt.Errorf("read --overwrite flag: %w", err)
			}
			editTemplate, err := cmd.Flags().GetBool("edit-template")
			if err != nil {
				return fmt.Errorf("read --edit-template flag: %w", err)
			}
			includeCompletedBootstrap, err := cmd.Flags().GetBool("include-completed-bootstrap")
			if err != nil {
				return fmt.Errorf("read --include-completed-bootstrap flag: %w", err)
			}
			backfillBrief, err := cmd.Flags().GetBool("backfill-brief")
			if err != nil {
				return fmt.Errorf("read --backfill-brief flag: %w", err)
			}
			aggregateDetectedSkills, err := cmd.Flags().GetBool("aggregate-detected-skills")
			if err != nil {
				return fmt.Errorf("read --aggregate-detected-skills flag: %w", err)
			}
			allowOutOfRangeSkills, err := cmd.Flags().GetBool("allow-out-of-range-skills")
			if err != nil {
				return fmt.Errorf("read --allow-out-of-range-skills flag: %w", err)
			}
			if dryRun && backfillBrief {
				return fmt.Errorf(
					"--dry-run cannot be combined with --backfill-brief; run `orbit brief backfill --orbit %s` first or rerun without --dry-run",
					args[0],
				)
			}
			if dryRun && aggregateDetectedSkills {
				return fmt.Errorf("--dry-run cannot be combined with --aggregate-detected-skills")
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}
			if err := orbittemplate.EnsureMemberHintExportSync(cmd.Context(), repo.Root, args[0], "saving"); err != nil {
				return fmt.Errorf("ensure member hint export sync: %w", err)
			}
			briefSync, err := orbittemplate.EnsureBriefExportSync(cmd.Context(), repo.Root, args[0], "saving", backfillBrief)
			if err != nil {
				return fmt.Errorf("ensure brief export sync: %w", err)
			}

			previewInput := orbittemplate.TemplateSavePreviewInput{
				RepoRoot:                  repo.Root,
				OrbitID:                   args[0],
				TargetBranch:              targetBranch,
				DefaultBranch:             defaultTemplate,
				Now:                       time.Now().UTC(),
				EditTemplate:              editTemplate,
				IncludeCompletedBootstrap: includeCompletedBootstrap,
			}
			if briefSync.Warning != "" {
				previewInput.Warnings = append(previewInput.Warnings, briefSync.Warning)
			}
			skillDetectionResult, err := orbittemplate.RunTemplateLocalSkillDetection(cmd.Context(), orbittemplate.TemplateLocalSkillDetectionInput{
				RepoRoot:                repo.Root,
				OrbitID:                 args[0],
				AggregateDetectedSkills: aggregateDetectedSkills,
				AllowOutOfRangeSkills:   allowOutOfRangeSkills,
				ConfirmPrompter:         buildTemplateSkillDetectionPrompter(cmd),
			})
			if err != nil {
				return fmt.Errorf("detect local skill candidates before save: %w", err)
			}
			previewInput.Warnings = append(previewInput.Warnings, skillDetectionResult.Warnings...)
			if editTemplate {
				editor, err := orbittemplate.NewEnvironmentEditor()
				if err != nil {
					return fmt.Errorf("configure template editor: %w", err)
				}
				previewInput.Editor = editor
			}

			if dryRun {
				if err := stageProgress(progress, "building template preview"); err != nil {
					return err
				}
				preview, err := orbittemplate.BuildTemplateSavePreview(cmd.Context(), previewInput)
				if err != nil {
					return fmt.Errorf("build template save preview: %w", err)
				}
				if jsonOutput {
					if err := emitJSON(cmd.OutOrStdout(), templateSavePreviewPayload(preview)); err != nil {
						return err
					}
				} else {
					if err := emitTemplateSavePreview(cmd, preview); err != nil {
						return err
					}
				}
				if len(preview.Ambiguities) > 0 {
					return fmt.Errorf("replacement ambiguity detected; resolve the previewed ambiguities before saving")
				}
				if err := stageProgress(progress, "template save complete"); err != nil {
					return err
				}

				return nil
			}

			if err := stageProgress(progress, "building template preview"); err != nil {
				return err
			}
			preview, err := orbittemplate.BuildTemplateSavePreview(cmd.Context(), previewInput)
			if err != nil {
				return fmt.Errorf("build template save preview: %w", err)
			}
			if err := stageProgress(progress, "writing template branch"); err != nil {
				return err
			}
			result, err := orbittemplate.WriteTemplateSavePreview(cmd.Context(), orbittemplate.TemplateSaveWriteInput{
				Preview:   preview,
				Overwrite: overwrite,
			})
			if err != nil {
				return fmt.Errorf("save template branch: %w", err)
			}
			if err := stageProgress(progress, "template save complete"); err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), templateSaveResultPayload(result))
			}
			if err := emitTemplateSaveWarnings(cmd, result.Preview.Warnings); err != nil {
				return err
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"saved template orbit %s to branch %s\ncommit: %s\nfiles: %d\n",
				result.Preview.OrbitID,
				result.WriteResult.Branch,
				result.WriteResult.Commit,
				len(result.Preview.Files)+1,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().String("to", "", "Target template branch name")
	cmd.Flags().Bool("dry-run", false, "Preview template save without writing a branch")
	cmd.Flags().Bool("overwrite", false, "Overwrite an existing target template branch")
	cmd.Flags().Bool("default", false, "Mark the saved template branch as the default template")
	cmd.Flags().Bool("edit-template", false, "Edit the generated template tree before saving")
	cmd.Flags().Bool("include-completed-bootstrap", false, "Include currently still-present completed bootstrap export files in the save preview/write set")
	cmd.Flags().Bool("backfill-brief", false, "Backfill a drifted root AGENTS orbit block into hosted brief truth before saving")
	cmd.Flags().Bool("aggregate-detected-skills", false, "Move detected out-of-range local skills into the default skills/<orbit-id>/* location before saving")
	cmd.Flags().Bool("allow-out-of-range-skills", false, "Save even when valid local skills are outside capabilities.skills.local.paths, and emit a warning instead")
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

func ensureTemplateSaveRuntimeRevision(ctx context.Context, repoRoot string) error {
	state, err := orbittemplate.LoadCurrentRepoState(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load current repo state: %w", err)
	}
	if state.Kind != "runtime" {
		return fmt.Errorf("template save requires a runtime revision; current revision kind is %q", state.Kind)
	}

	return nil
}

func resolveTemplateSaveTargetBranch(repoRoot string, orbitID string, explicitTarget string) (string, error) {
	if targetBranch := strings.TrimSpace(explicitTarget); targetBranch != "" {
		return targetBranch, nil
	}

	manifestFile, err := harness.LoadManifestFile(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load runtime manifest: %w", err)
	}
	memberSource, err := runtimeMemberSource(manifestFile, orbitID)
	if err != nil {
		return "", err
	}
	if memberSource != harness.ManifestMemberSourceInstallOrbit {
		return "", fmt.Errorf(
			"template save can omit --to only for install_orbit members; runtime member %q uses source %q",
			orbitID,
			memberSource,
		)
	}

	record, err := harness.LoadInstallRecord(repoRoot, orbitID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errorsNewRequiredFlag("to")
		}

		return "", fmt.Errorf("load runtime install record: %w", err)
	}

	targetBranch := strings.TrimSpace(record.Template.SourceRef)
	if targetBranch == "" {
		return "", fmt.Errorf("load runtime install record: template.source_ref must not be empty")
	}

	return targetBranch, nil
}

func runtimeMemberSource(manifestFile harness.ManifestFile, orbitID string) (string, error) {
	for _, member := range manifestFile.Members {
		if member.OrbitID == orbitID {
			return member.Source, nil
		}
	}

	return "", fmt.Errorf("load runtime manifest: orbit %q is not declared in members", orbitID)
}

func emitTemplateSavePreview(cmd *cobra.Command, preview orbittemplate.TemplateSavePreview) error {
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"template save dry-run for orbit %s -> %s\n",
		preview.OrbitID,
		preview.TargetBranch,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitTemplateSaveWarnings(cmd, preview.Warnings); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "files:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, path := range preview.FilePaths() {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	if len(preview.ReplacementSummaries) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "replacements: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "replacements:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, summary := range preview.ReplacementSummaries {
			for _, replacement := range summary.Replacements {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s: %s <- %s (%d)\n",
					summary.Path,
					replacement.Variable,
					replacement.Literal,
					replacement.Count,
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
		}
	}

	if len(preview.Ambiguities) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "ambiguities: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "ambiguities:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, ambiguity := range preview.Ambiguities {
			for _, item := range ambiguity.Ambiguities {
				if _, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"%s: %s -> %s\n",
					ambiguity.Path,
					item.Literal,
					strings.Join(item.Variables, ", "),
				); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
		}
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "manifest:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_id: %s\n", preview.Manifest.Template.OrbitID); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "default_template: %t\n", preview.Manifest.Template.DefaultTemplate); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created_from_branch: %s\n", preview.Manifest.Template.CreatedFromBranch); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created_from_commit: %s\n", preview.Manifest.Template.CreatedFromCommit); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created_at: %s\n", preview.Manifest.Template.CreatedAt.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	variableNames := contractutil.SortedKeys(preview.Manifest.Variables)
	if len(variableNames) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "variables: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "variables:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	sort.Strings(variableNames)
	for _, name := range variableNames {
		spec := preview.Manifest.Variables[name]
		line := fmt.Sprintf("%s [required]", name)
		if !spec.Required {
			line = fmt.Sprintf("%s [optional]", name)
		}
		if spec.Description != "" {
			line += " " + spec.Description
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitTemplateSaveWarnings(cmd *cobra.Command, warnings []string) error {
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

func buildTemplateSkillDetectionPrompter(cmd *cobra.Command) orbittemplate.ConfirmPrompter {
	if !templateSkillDetectionInteractive(cmd.InOrStdin(), cmd.ErrOrStderr()) {
		return nil
	}
	return orbittemplate.LineConfirmPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
}

func templateSkillDetectionInteractive(reader io.Reader, writer io.Writer) bool {
	return streamIsTerminal(reader) && streamIsTerminal(writer)
}

func streamIsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

func templateSavePreviewPayload(preview orbittemplate.TemplateSavePreview) templateSavePreviewJSON {
	return templateSavePreviewJSON{
		DryRun:       true,
		OrbitID:      preview.OrbitID,
		TargetBranch: preview.TargetBranch,
		Files:        preview.FilePaths(),
		Warnings:     append([]string(nil), preview.Warnings...),
		Replacements: templateSaveReplacementPayload(preview.ReplacementSummaries),
		Ambiguities:  templateSaveAmbiguityPayload(preview.Ambiguities),
		Manifest: templateSaveManifestJSON{
			OrbitID:           preview.Manifest.Template.OrbitID,
			DefaultTemplate:   preview.Manifest.Template.DefaultTemplate,
			CreatedFromBranch: preview.Manifest.Template.CreatedFromBranch,
			CreatedFromCommit: preview.Manifest.Template.CreatedFromCommit,
			CreatedAt:         preview.Manifest.Template.CreatedAt.UTC().Format(time.RFC3339),
		},
		Variables: templateSaveVariablePayload(preview.Manifest.Variables),
	}
}

func templateSaveResultPayload(result orbittemplate.TemplateSaveResult) templateSaveResultJSON {
	files := append(previewFilePaths(result.Preview), harness.ManifestRepoPath())
	sort.Strings(files)

	return templateSaveResultJSON{
		DryRun:       false,
		OrbitID:      result.Preview.OrbitID,
		TargetBranch: result.WriteResult.Branch,
		Ref:          result.WriteResult.Ref,
		Commit:       result.WriteResult.Commit,
		Files:        files,
		Warnings:     append([]string(nil), result.Preview.Warnings...),
	}
}

func templateSaveReplacementPayload(items []orbittemplate.FileReplacementSummary) []templateSaveFileReplacementJSON {
	results := make([]templateSaveFileReplacementJSON, 0, len(items))
	for _, item := range items {
		replacements := make([]templateSaveReplacementJSON, 0, len(item.Replacements))
		for _, replacement := range item.Replacements {
			replacements = append(replacements, templateSaveReplacementJSON{
				Variable: replacement.Variable,
				Literal:  replacement.Literal,
				Count:    replacement.Count,
			})
		}
		results = append(results, templateSaveFileReplacementJSON{
			Path:         item.Path,
			Replacements: replacements,
		})
	}
	return results
}

func templateSaveAmbiguityPayload(items []orbittemplate.FileReplacementAmbiguity) []templateSaveFileAmbiguityJSON {
	results := make([]templateSaveFileAmbiguityJSON, 0, len(items))
	for _, item := range items {
		ambiguities := make([]templateSaveAmbiguityJSON, 0, len(item.Ambiguities))
		for _, ambiguity := range item.Ambiguities {
			ambiguities = append(ambiguities, templateSaveAmbiguityJSON{
				Literal:   ambiguity.Literal,
				Variables: ambiguity.Variables,
			})
		}
		results = append(results, templateSaveFileAmbiguityJSON{
			Path:        item.Path,
			Ambiguities: ambiguities,
		})
	}
	return results
}

func templateSaveVariablePayload(variables map[string]orbittemplate.VariableSpec) []templateSaveVariableJSON {
	names := contractutil.SortedKeys(variables)
	results := make([]templateSaveVariableJSON, 0, len(names))
	for _, name := range names {
		spec := variables[name]
		results = append(results, templateSaveVariableJSON{
			Name:        name,
			Required:    spec.Required,
			Description: spec.Description,
		})
	}
	return results
}

func previewFilePaths(preview orbittemplate.TemplateSavePreview) []string {
	return append([]string(nil), preview.FilePaths()...)
}
