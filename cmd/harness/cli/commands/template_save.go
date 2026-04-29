package commands

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
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
	Path         string                      `json:"path"`
	Contributors []string                    `json:"contributors"`
	Ambiguities  []templateSaveAmbiguityJSON `json:"ambiguities"`
}

type templateSaveConflictJSON struct {
	Kind         string   `json:"kind"`
	Path         string   `json:"path,omitempty"`
	Variable     string   `json:"variable,omitempty"`
	Contributors []string `json:"contributors"`
	Message      string   `json:"message"`
}

type rootGuidanceJSON struct {
	Agents    bool `json:"agents"`
	Humans    bool `json:"humans"`
	Bootstrap bool `json:"bootstrap"`
}

type templateSaveManifestJSON struct {
	HarnessID         string           `json:"harness_id"`
	DefaultTemplate   bool             `json:"default_template"`
	CreatedFromBranch string           `json:"created_from_branch"`
	CreatedFromCommit string           `json:"created_from_commit"`
	CreatedAt         string           `json:"created_at"`
	RootGuidance      rootGuidanceJSON `json:"root_guidance"`
}

type templateSaveVariableJSON struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type templateSavePreviewJSON struct {
	DryRun          bool                              `json:"dry_run"`
	HarnessRoot     string                            `json:"harness_root"`
	HarnessID       string                            `json:"harness_id"`
	TargetBranch    string                            `json:"target_branch"`
	Files           []string                          `json:"files"`
	Warnings        []string                          `json:"warnings,omitempty"`
	MemberCount     int                               `json:"member_count"`
	DefaultTemplate bool                              `json:"default_template"`
	RootGuidance    rootGuidanceJSON                  `json:"root_guidance"`
	Replacements    []templateSaveFileReplacementJSON `json:"replacements"`
	Ambiguities     []templateSaveFileAmbiguityJSON   `json:"ambiguities"`
	Conflicts       []templateSaveConflictJSON        `json:"conflicts,omitempty"`
	Manifest        templateSaveManifestJSON          `json:"manifest"`
	Members         []string                          `json:"members"`
	Variables       []templateSaveVariableJSON        `json:"variables"`
}

type templateSaveFailureJSON struct {
	DryRun            bool                            `json:"dry_run"`
	Saved             bool                            `json:"saved"`
	Stage             string                          `json:"stage,omitempty"`
	Reason            string                          `json:"reason,omitempty"`
	HarnessRoot       string                          `json:"harness_root"`
	HarnessID         string                          `json:"harness_id,omitempty"`
	TargetBranch      string                          `json:"target_branch"`
	DefaultTemplate   bool                            `json:"default_template"`
	RootGuidance      rootGuidanceJSON                `json:"root_guidance"`
	OverwriteRequired bool                            `json:"overwrite_required,omitempty"`
	Ambiguities       []templateSaveFileAmbiguityJSON `json:"ambiguities,omitempty"`
	Conflicts         []templateSaveConflictJSON      `json:"conflicts,omitempty"`
	Message           string                          `json:"message"`
}

type templateSaveResultJSON struct {
	DryRun          bool             `json:"dry_run"`
	HarnessRoot     string           `json:"harness_root"`
	HarnessID       string           `json:"harness_id"`
	TargetBranch    string           `json:"target_branch"`
	Ref             string           `json:"ref"`
	Commit          string           `json:"commit"`
	Files           []string         `json:"files"`
	Warnings        []string         `json:"warnings,omitempty"`
	MemberCount     int              `json:"member_count"`
	DefaultTemplate bool             `json:"default_template"`
	RootGuidance    rootGuidanceJSON `json:"root_guidance"`
}

// NewTemplateSaveCommand creates the harness template save command.
func NewTemplateSaveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "save",
		Short: "Save the current harness runtime as a harness template branch",
		Long: "Save the current harness runtime into a reusable harness template branch.\n" +
			"The command builds one merged template from the declared members, preserves root AGENTS.md\n" +
			"with whole-file semantics, and writes the target branch without switching branches.",
		Example: "" +
			"  harness template save --to harness-template/workspace\n" +
			"  harness template save --to harness-template/workspace --dry-run\n" +
			"  harness template save --to harness-template/workspace --edit-template\n" +
			"  harness template save --to harness-template/workspace --include-bootstrap\n" +
			"  harness template save --to harness-template/workspace --default --json\n",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return cobra.ExactArgs(0)(cmd, args)
			}

			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			targetBranch, err := cmd.Flags().GetString("to")
			if err != nil {
				return fmt.Errorf("read --to flag: %w", err)
			}
			if strings.TrimSpace(targetBranch) == "" {
				return fmt.Errorf("required flag %q not set", "to")
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
			includeBootstrap, err := cmd.Flags().GetBool("include-bootstrap")
			if err != nil {
				return fmt.Errorf("read --include-bootstrap flag: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
			}

			previewInput := harnesspkg.TemplateSavePreviewInput{
				RepoRoot:         resolved.Repo.Root,
				TargetBranch:     targetBranch,
				DefaultTemplate:  defaultTemplate,
				EditTemplate:     editTemplate,
				Now:              time.Now().UTC(),
				IncludeBootstrap: includeBootstrap,
			}
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
				preview, err := harnesspkg.BuildTemplateSavePreview(cmd.Context(), previewInput)
				if err != nil {
					if jsonOutput {
						payload, handled := templateSaveDryRunConflictPayload(resolved.Repo.Root, targetBranch, err)
						if handled {
							return emitJSON(cmd.OutOrStdout(), payload)
						}
					}
					return fmt.Errorf("build harness template save preview: %w", err)
				}
				if jsonOutput {
					if err := emitJSON(cmd.OutOrStdout(), templateSavePreviewPayload(resolved.Repo.Root, preview)); err != nil {
						return err
					}
					if err := stageProgress(progress, "template save complete"); err != nil {
						return err
					}
					return nil
				}
				if err := emitTemplateSavePreview(cmd, preview); err != nil {
					return err
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
			preview, err := harnesspkg.BuildTemplateSavePreview(cmd.Context(), previewInput)
			if err != nil {
				if jsonOutput {
					payload, handled := templateSaveFailurePayload(resolved.Repo.Root, targetBranch, nil, err)
					if handled {
						return emitJSON(cmd.OutOrStdout(), payload)
					}
				}
				return fmt.Errorf("build harness template save preview: %w", err)
			}
			if jsonOutput {
				if payload, handled := templateSaveFailurePayload(resolved.Repo.Root, targetBranch, &preview, nil); handled {
					return emitJSON(cmd.OutOrStdout(), payload)
				}
			}
			if err := stageProgress(progress, "writing template branch"); err != nil {
				return err
			}
			result, err := harnesspkg.WriteTemplateSavePreview(cmd.Context(), harnesspkg.TemplateSaveWriteInput{
				Preview:   preview,
				Overwrite: overwrite,
			})
			if err != nil {
				if jsonOutput {
					if payload, handled := templateSaveFailurePayload(resolved.Repo.Root, targetBranch, &preview, err); handled {
						return emitJSON(cmd.OutOrStdout(), payload)
					}
				}
				return fmt.Errorf("save harness template branch: %w", err)
			}
			if err := stageProgress(progress, "template save complete"); err != nil {
				return err
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), templateSaveResultPayload(resolved.Repo.Root, result))
			}

			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"saved harness template %s to branch %s\ncommit: %s\nfiles: %d\n",
				result.Preview.HarnessID,
				result.WriteResult.Branch,
				result.WriteResult.Commit,
				len(result.Preview.FilePaths()),
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", len(result.Preview.Manifest.Members)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "default_template: %t\n", result.Preview.Manifest.Template.DefaultTemplate); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if err := emitRootGuidanceText(cmd, result.Preview.Manifest.Template.RootGuidance); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if err := emitTemplateSaveWarnings(cmd, result.Preview.Warnings); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().String("to", "", "Target harness template branch name")
	cmd.Flags().Bool("dry-run", false, "Preview harness template save without writing a branch")
	cmd.Flags().Bool("edit-template", false, "Edit the generated harness template tree before saving")
	cmd.Flags().Bool("include-bootstrap", false, "Include bootstrap guidance and currently still-present completed bootstrap export files in the save preview/write set")
	cmd.Flags().Bool("overwrite", false, "Overwrite an existing target harness template branch")
	cmd.Flags().Bool("default", false, "Mark the saved harness template branch as the default template candidate")
	addPathFlag(cmd)
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

func emitTemplateSavePreview(cmd *cobra.Command, preview harnesspkg.TemplateSavePreview) error {
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"harness template save dry-run -> %s\n",
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
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", preview.HarnessID); err != nil {
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
	if err := emitRootGuidanceText(cmd, preview.Manifest.Template.RootGuidance); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	memberIDs := make([]string, 0, len(preview.Manifest.Members))
	for _, member := range preview.Manifest.Members {
		memberIDs = append(memberIDs, member.OrbitID)
	}
	sort.Strings(memberIDs)
	if len(memberIDs) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "members: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "members:"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, memberID := range memberIDs {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), memberID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	variableNames := sortedTemplateVariableNames(preview.Manifest.Variables)
	if len(variableNames) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "variables: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "variables:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
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

func emitRootGuidanceText(cmd *cobra.Command, rootGuidance harnesspkg.RootGuidanceMetadata) error {
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "root_guidance:"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agents: %t\n", rootGuidance.Agents); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "humans: %t\n", rootGuidance.Humans); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "bootstrap: %t\n", rootGuidance.Bootstrap); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	return nil
}

func templateSavePreviewPayload(repoRoot string, preview harnesspkg.TemplateSavePreview) templateSavePreviewJSON {
	memberIDs := make([]string, 0, len(preview.Manifest.Members))
	for _, member := range preview.Manifest.Members {
		memberIDs = append(memberIDs, member.OrbitID)
	}
	sort.Strings(memberIDs)

	return templateSavePreviewJSON{
		DryRun:          true,
		HarnessRoot:     repoRoot,
		HarnessID:       preview.HarnessID,
		TargetBranch:    preview.TargetBranch,
		Files:           preview.FilePaths(),
		Warnings:        append([]string(nil), preview.Warnings...),
		MemberCount:     len(preview.Manifest.Members),
		DefaultTemplate: preview.Manifest.Template.DefaultTemplate,
		RootGuidance:    rootGuidancePayload(preview.Manifest.Template.RootGuidance),
		Replacements:    templateSaveReplacementPayload(preview.ReplacementSummaries),
		Ambiguities:     templateSaveAmbiguityPayload(preview.Ambiguities, preview.AmbiguitySources),
		Manifest: templateSaveManifestJSON{
			HarnessID:         preview.HarnessID,
			DefaultTemplate:   preview.Manifest.Template.DefaultTemplate,
			CreatedFromBranch: preview.Manifest.Template.CreatedFromBranch,
			CreatedFromCommit: preview.Manifest.Template.CreatedFromCommit,
			CreatedAt:         preview.Manifest.Template.CreatedAt.UTC().Format(time.RFC3339),
			RootGuidance:      rootGuidancePayload(preview.Manifest.Template.RootGuidance),
		},
		Members:   memberIDs,
		Variables: templateSaveVariablePayload(preview.Manifest.Variables),
	}
}

func templateSaveDryRunConflictPayload(repoRoot string, targetBranch string, err error) (templateSavePreviewJSON, bool) {
	conflicts, handled := templateSaveConflicts(err)
	if !handled {
		return templateSavePreviewJSON{}, false
	}

	return templateSavePreviewJSON{
		DryRun:       true,
		HarnessRoot:  repoRoot,
		TargetBranch: targetBranch,
		Conflicts:    conflicts,
	}, true
}

func templateSaveFailurePayload(
	repoRoot string,
	targetBranch string,
	preview *harnesspkg.TemplateSavePreview,
	err error,
) (templateSaveFailureJSON, bool) {
	if preview != nil && len(preview.Ambiguities) > 0 {
		return templateSaveFailureJSON{
			DryRun:          false,
			Saved:           false,
			Stage:           "preview",
			Reason:          "replacement_ambiguity",
			HarnessRoot:     repoRoot,
			HarnessID:       preview.HarnessID,
			TargetBranch:    preview.TargetBranch,
			DefaultTemplate: preview.Manifest.Template.DefaultTemplate,
			RootGuidance:    rootGuidancePayload(preview.Manifest.Template.RootGuidance),
			Ambiguities:     templateSaveAmbiguityPayload(preview.Ambiguities, preview.AmbiguitySources),
			Message: fmt.Sprintf(
				"replacement ambiguity detected in %s; resolve the previewed ambiguities before saving",
				harnesspkg.FormatTemplateAmbiguitySources(preview.AmbiguitySources),
			),
		}, true
	}

	conflicts, handled := templateSaveConflicts(err)
	if !handled {
		var branchExists *gitpkg.TemplateTargetBranchExistsError
		if errors.As(err, &branchExists) {
			payload := templateSaveFailureJSON{
				DryRun:            false,
				Saved:             false,
				Stage:             "write",
				Reason:            "target_branch_exists",
				HarnessRoot:       repoRoot,
				TargetBranch:      targetBranch,
				OverwriteRequired: true,
				Message:           branchExists.Error(),
			}
			if preview != nil {
				payload.HarnessID = preview.HarnessID
				payload.TargetBranch = preview.TargetBranch
				payload.DefaultTemplate = preview.Manifest.Template.DefaultTemplate
				payload.RootGuidance = rootGuidancePayload(preview.Manifest.Template.RootGuidance)
			}
			return payload, true
		}
		return templateSaveFailureJSON{}, false
	}

	message := conflicts[0].Message
	return templateSaveFailureJSON{
		DryRun:       false,
		Saved:        false,
		Stage:        "preview",
		Reason:       conflicts[0].Kind,
		HarnessRoot:  repoRoot,
		TargetBranch: targetBranch,
		Conflicts:    conflicts,
		Message:      message,
	}, true
}

func templateSaveConflicts(err error) ([]templateSaveConflictJSON, bool) {
	var pathConflict *harnesspkg.TemplatePathConflictError
	if errors.As(err, &pathConflict) {
		return []templateSaveConflictJSON{{
			Kind:         "path_conflict",
			Path:         pathConflict.Path,
			Contributors: append([]string(nil), pathConflict.Members...),
			Message:      pathConflict.Error(),
		}}, true
	}

	var variableConflict *harnesspkg.TemplateVariableConflictError
	if errors.As(err, &variableConflict) {
		return []templateSaveConflictJSON{{
			Kind:         "variable_conflict",
			Variable:     variableConflict.Name,
			Contributors: append([]string(nil), variableConflict.Members...),
			Message:      variableConflict.Error(),
		}}, true
	}

	return nil, false
}

func templateSaveResultPayload(repoRoot string, result harnesspkg.TemplateSaveResult) templateSaveResultJSON {
	return templateSaveResultJSON{
		DryRun:          false,
		HarnessRoot:     repoRoot,
		HarnessID:       result.Preview.HarnessID,
		TargetBranch:    result.WriteResult.Branch,
		Ref:             result.WriteResult.Ref,
		Commit:          result.WriteResult.Commit,
		Files:           result.Preview.FilePaths(),
		Warnings:        append([]string(nil), result.Preview.Warnings...),
		MemberCount:     len(result.Preview.Manifest.Members),
		DefaultTemplate: result.Preview.Manifest.Template.DefaultTemplate,
		RootGuidance:    rootGuidancePayload(result.Preview.Manifest.Template.RootGuidance),
	}
}

func rootGuidancePayload(rootGuidance harnesspkg.RootGuidanceMetadata) rootGuidanceJSON {
	return rootGuidanceJSON{
		Agents:    rootGuidance.Agents,
		Humans:    rootGuidance.Humans,
		Bootstrap: rootGuidance.Bootstrap,
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

func templateSaveAmbiguityPayload(
	items []orbittemplate.FileReplacementAmbiguity,
	previewSources map[string][]string,
) []templateSaveFileAmbiguityJSON {
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
			Path:         item.Path,
			Contributors: append([]string(nil), previewContributorsForAmbiguity(item.Path, previewSources)...),
			Ambiguities:  ambiguities,
		})
	}
	return results
}

func previewContributorsForAmbiguity(path string, sources map[string][]string) []string {
	if len(sources) == 0 {
		return nil
	}

	return append([]string(nil), sources[path]...)
}

func templateSaveVariablePayload(variables map[string]harnesspkg.TemplateVariableSpec) []templateSaveVariableJSON {
	names := sortedTemplateVariableNames(variables)
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

func sortedTemplateVariableNames(variables map[string]harnesspkg.TemplateVariableSpec) []string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
