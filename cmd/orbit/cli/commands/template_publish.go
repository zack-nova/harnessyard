package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type templatePublishLocalJSON struct {
	Success bool   `json:"success"`
	Changed bool   `json:"changed"`
	Commit  string `json:"commit,omitempty"`
}

type templatePublishRemoteJSON struct {
	Attempted          bool     `json:"attempted"`
	Success            bool     `json:"success"`
	Remote             string   `json:"remote,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	SourceBranchStatus string   `json:"source_branch_status,omitempty"`
	NextActions        []string `json:"next_actions,omitempty"`
}

type templatePublishResultJSON struct {
	PackageName        string                    `json:"package_name"`
	PackageVersion     string                    `json:"package_version"`
	PackagePublishKind string                    `json:"package_publish_kind"`
	PackageCoordinate  string                    `json:"package_coordinate"`
	PackageLocatorKind string                    `json:"package_locator_kind,omitempty"`
	PackageLocator     string                    `json:"package_locator,omitempty"`
	OrbitID            string                    `json:"orbit_id"`
	PublishRef         string                    `json:"publish_ref"`
	NextAction         string                    `json:"next_action"`
	NextRef            string                    `json:"next_ref"`
	Branch             string                    `json:"branch"`
	SourceBranch       string                    `json:"source_branch"`
	DefaultTemplate    bool                      `json:"default_template"`
	Warnings           []string                  `json:"warnings,omitempty"`
	LocalPublish       templatePublishLocalJSON  `json:"local_publish"`
	RemotePush         templatePublishRemoteJSON `json:"remote_push"`
}

// NewTemplatePublishCommand creates the orbit template publish command.
func NewTemplatePublishCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish one orbit from the current source or template revision",
		Long: "Publish one orbit from the current source or orbit_template revision by generating the installable payload for the current branch state.\n" +
			"This command is author-facing, requires a clean tracked worktree, and does not push by default.",
		Example: "" +
			"  orbit template publish\n" +
			"  orbit template publish --backfill-brief\n" +
			"  orbit template publish --allow-out-of-range-skills\n" +
			"  orbit template publish --aggregate-detected-skills\n" +
			"  orbit template publish --default\n" +
			"  orbit template publish --push --remote origin\n" +
			"  orbit template publish --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			orbitID, err := cmd.Flags().GetString("orbit")
			if err != nil {
				return fmt.Errorf("read --orbit flag: %w", err)
			}
			defaultTemplate, err := cmd.Flags().GetBool("default")
			if err != nil {
				return fmt.Errorf("read --default flag: %w", err)
			}
			pushEnabled, err := cmd.Flags().GetBool("push")
			if err != nil {
				return fmt.Errorf("read --push flag: %w", err)
			}
			remoteName, err := cmd.Flags().GetString("remote")
			if err != nil {
				return fmt.Errorf("read --remote flag: %w", err)
			}
			targetBranch, err := cmd.Flags().GetString("target-branch")
			if err != nil {
				return fmt.Errorf("read --target-branch flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progress, err := progressFromCommand(cmd)
			if err != nil {
				return err
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
			if remoteName != "" && !pushEnabled {
				return fmt.Errorf("--remote can only be used with --push")
			}
			defaultTemplateSet := cmd.Flags().Changed("default")
			confirmPrompter := buildTemplatePublishPrompter(cmd)
			skillDetectionPrompter := buildTemplateSkillDetectionPrompter(cmd)

			result, err := orbittemplate.PublishTemplate(cmd.Context(), orbittemplate.TemplatePublishInput{
				RepoRoot:                 repo.Root,
				OrbitID:                  orbitID,
				DefaultTemplate:          defaultTemplate,
				DefaultTemplateSet:       defaultTemplateSet,
				BackfillBrief:            backfillBrief,
				AggregateDetectedSkills:  aggregateDetectedSkills,
				AllowOutOfRangeSkills:    allowOutOfRangeSkills,
				ConfirmPrompter:          confirmPrompter,
				SkillDetectionPrompter:   skillDetectionPrompter,
				SourceBranchPushPrompter: buildTemplateSourceBranchPushPrompter(cmd, jsonOutput),
				Push:                     pushEnabled,
				Remote:                   remoteName,
				TargetBranch:             targetBranch,
				Progress:                 progress.Stage,
			})
			if err != nil {
				var publishErr *orbittemplate.PublishError
				if errors.As(err, &publishErr) {
					if emitErr := emitTemplatePublishResult(cmd, publishErr.Result, jsonOutput); emitErr != nil {
						return emitErr
					}
					return fmt.Errorf("publish template branch: %w", publishErr.Err)
				}
				return fmt.Errorf("publish template branch: %w", err)
			}

			return emitTemplatePublishResult(cmd, result, jsonOutput)
		},
	}

	cmd.Flags().String("orbit", "", "Explicit orbit id to verify against the single source orbit")
	cmd.Flags().Bool("default", false, "Mark the published template branch as the default template")
	cmd.Flags().Bool("backfill-brief", false, "Backfill a drifted root AGENTS orbit block into hosted brief truth before publishing")
	cmd.Flags().Bool("aggregate-detected-skills", false, "Move detected out-of-range local skills into the default skills/<orbit-id>/* location before publishing")
	cmd.Flags().Bool("allow-out-of-range-skills", false, "Publish even when valid local skills are outside capabilities.skills.local.paths, and emit a warning instead")
	cmd.Flags().Bool("push", false, "Push the published template branch after local publish succeeds")
	cmd.Flags().String("remote", "", "Remote name to use with --push; defaults to origin")
	cmd.Flags().String("target-branch", "", "Target template branch override for package locator publish")
	if err := cmd.Flags().MarkHidden("target-branch"); err != nil {
		panic(err)
	}
	addTemplatePublishPackageMetadataFlags(cmd)
	addJSONFlag(cmd)
	addProgressFlag(cmd)

	return cmd
}

type templatePublishPackageMetadata struct {
	Name        string
	Version     string
	PublishKind string
	Coordinate  string
	LocatorKind string
	Locator     string
}

func addTemplatePublishPackageMetadataFlags(cmd *cobra.Command) {
	cmd.Flags().String("package-name", "", "User-facing package name for publish output")
	cmd.Flags().String("package-version", "", "User-facing package version for publish output")
	cmd.Flags().String("package-publish-kind", "", "User-facing package publish kind for publish output")
	cmd.Flags().String("package-coordinate", "", "User-facing package coordinate for publish output")
	cmd.Flags().String("package-locator-kind", "", "User-facing package locator kind for publish output")
	cmd.Flags().String("package-locator", "", "User-facing package locator for publish output")
	for _, flagName := range []string{"package-name", "package-version", "package-publish-kind", "package-coordinate", "package-locator-kind", "package-locator"} {
		if err := cmd.Flags().MarkHidden(flagName); err != nil {
			panic(err)
		}
	}
}

func readTemplatePublishPackageMetadata(cmd *cobra.Command, defaultName string) (templatePublishPackageMetadata, error) {
	name, err := cmd.Flags().GetString("package-name")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-name flag: %w", err)
	}
	version, err := cmd.Flags().GetString("package-version")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-version flag: %w", err)
	}
	publishKind, err := cmd.Flags().GetString("package-publish-kind")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-publish-kind flag: %w", err)
	}
	coordinate, err := cmd.Flags().GetString("package-coordinate")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-coordinate flag: %w", err)
	}
	locatorKind, err := cmd.Flags().GetString("package-locator-kind")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-locator-kind flag: %w", err)
	}
	locator, err := cmd.Flags().GetString("package-locator")
	if err != nil {
		return templatePublishPackageMetadata{}, fmt.Errorf("read --package-locator flag: %w", err)
	}
	if name == "" {
		name = defaultName
	}
	if version == "" {
		version = "none"
	}
	if publishKind == "" {
		publishKind = "snapshot"
	}
	if coordinate == "" {
		coordinate = name
	}

	return templatePublishPackageMetadata{
		Name:        name,
		Version:     version,
		PublishKind: publishKind,
		Coordinate:  coordinate,
		LocatorKind: locatorKind,
		Locator:     locator,
	}, nil
}

func templatePublishResultPayload(cmd *cobra.Command, result orbittemplate.TemplatePublishResult) (templatePublishResultJSON, error) {
	packageMetadata, err := readTemplatePublishPackageMetadata(cmd, result.Preview.OrbitID)
	if err != nil {
		return templatePublishResultJSON{}, err
	}

	payload := templatePublishResultJSON{
		PackageName:        packageMetadata.Name,
		PackageVersion:     packageMetadata.Version,
		PackagePublishKind: packageMetadata.PublishKind,
		PackageCoordinate:  packageMetadata.Coordinate,
		PackageLocatorKind: packageMetadata.LocatorKind,
		PackageLocator:     packageMetadata.Locator,
		OrbitID:            result.Preview.OrbitID,
		PublishRef:         "refs/heads/" + result.Preview.PublishBranch,
		NextAction:         "install",
		NextRef:            "refs/heads/" + result.Preview.PublishBranch,
		Branch:             result.Preview.PublishBranch,
		SourceBranch:       result.Preview.SourceBranch,
		DefaultTemplate:    result.Preview.DefaultTemplate,
		Warnings:           append([]string(nil), result.Preview.SavePreview.Warnings...),
		LocalPublish: templatePublishLocalJSON{
			Success: result.LocalSuccess,
			Changed: result.Changed,
		},
		RemotePush: templatePublishRemoteJSON{
			Attempted:          result.RemotePush.Attempted,
			Success:            result.RemotePush.Success,
			Remote:             result.RemotePush.Remote,
			Reason:             result.RemotePush.Reason,
			SourceBranchStatus: string(result.RemotePush.SourceBranchStatus),
			NextActions:        append([]string(nil), result.RemotePush.NextActions...),
		},
	}
	if result.Changed {
		payload.LocalPublish.Commit = result.Commit
	}

	return payload, nil
}

func emitTemplatePublishResult(cmd *cobra.Command, result orbittemplate.TemplatePublishResult, jsonOutput bool) error {
	packageMetadata, err := readTemplatePublishPackageMetadata(cmd, result.Preview.OrbitID)
	if err != nil {
		return err
	}
	if jsonOutput {
		payload, err := templatePublishResultPayload(cmd, result)
		if err != nil {
			return err
		}
		return emitJSON(cmd.OutOrStdout(), payload)
	}

	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"package_name: %s\npackage_version: %s\npackage_publish_kind: %s\npackage_coordinate: %s\n",
		packageMetadata.Name,
		packageMetadata.Version,
		packageMetadata.PublishKind,
		packageMetadata.Coordinate,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if packageMetadata.LocatorKind != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_locator_kind: %s\n", packageMetadata.LocatorKind); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if packageMetadata.Locator != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_locator: %s\n", packageMetadata.Locator); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"orbit_id: %s\npublish_ref: refs/heads/%s\nnext_action: install\nnext_ref: refs/heads/%s\nsource_branch: %s\ndefault_template: %t\nlocal_publish.success: %t\nlocal_publish.changed: %t\n",
		result.Preview.OrbitID,
		result.Preview.PublishBranch,
		result.Preview.PublishBranch,
		result.Preview.SourceBranch,
		result.Preview.DefaultTemplate,
		result.LocalSuccess,
		result.Changed,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if err := emitTemplateSaveWarnings(cmd, result.Preview.SavePreview.Warnings); err != nil {
		return err
	}
	if result.Changed {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "local_publish.commit: %s\n", result.Commit); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		cmd.OutOrStdout(),
		"remote_push.attempted: %t\nremote_push.success: %t\n",
		result.RemotePush.Attempted,
		result.RemotePush.Success,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if result.RemotePush.Remote != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote_push.remote: %s\n", result.RemotePush.Remote); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if result.RemotePush.Reason != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote_push.reason: %s\n", result.RemotePush.Reason); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if result.RemotePush.SourceBranchStatus != "" {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote_push.source_branch_status: %s\n", result.RemotePush.SourceBranchStatus); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, action := range result.RemotePush.NextActions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote_push.next_action: %s\n", action); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func buildTemplatePublishPrompter(cmd *cobra.Command) orbittemplate.ConfirmPrompter {
	return orbittemplate.LineConfirmPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
}

func buildTemplateSourceBranchPushPrompter(cmd *cobra.Command, jsonOutput bool) orbittemplate.SourceBranchPushPrompter {
	interactive := templatePublishInteractiveFromContext(cmd.Context()) ||
		(templatePublishStreamIsTerminal(cmd.InOrStdin()) && templatePublishStreamIsTerminal(cmd.ErrOrStderr()))
	if jsonOutput || !interactive {
		return nil
	}
	return orbittemplate.LineSourceBranchPushPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
}
