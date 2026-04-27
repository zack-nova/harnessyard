package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type templatePublishLocalJSON struct {
	Success bool   `json:"success"`
	Changed bool   `json:"changed"`
	Commit  string `json:"commit,omitempty"`
}

type templatePublishRemoteJSON struct {
	Attempted bool   `json:"attempted"`
	Success   bool   `json:"success"`
	Remote    string `json:"remote,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type templatePublishResultJSON struct {
	PackageName        string                    `json:"package_name"`
	PackageVersion     string                    `json:"package_version"`
	PackagePublishKind string                    `json:"package_publish_kind"`
	PackageCoordinate  string                    `json:"package_coordinate"`
	PackageLocatorKind string                    `json:"package_locator_kind,omitempty"`
	PackageLocator     string                    `json:"package_locator,omitempty"`
	HarnessID          string                    `json:"harness_id"`
	PublishRef         string                    `json:"publish_ref"`
	NextAction         string                    `json:"next_action"`
	NextRef            string                    `json:"next_ref"`
	Branch             string                    `json:"branch"`
	SourceBranch       string                    `json:"source_branch"`
	DefaultTemplate    bool                      `json:"default_template"`
	LocalPublish       templatePublishLocalJSON  `json:"local_publish"`
	RemotePush         templatePublishRemoteJSON `json:"remote_push"`
}

// NewTemplatePublishCommand creates the harness template publish command.
func NewTemplatePublishCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish the current runtime as a harness template branch",
		Long: "Publish the current runtime to a harness template branch.\n" +
			"This command is author-facing, keeps `harness template save` as the local export primitive,\n" +
			"and only consults remotes when `--push` is explicitly requested.",
		Example: "" +
			"  harness template publish --to harness-template/workspace\n" +
			"  harness template publish --to harness-template/workspace --default\n" +
			"  harness template publish --to harness-template/workspace --push --remote origin\n" +
			"  harness template publish --to harness-template/workspace --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			pushEnabled, err := cmd.Flags().GetBool("push")
			if err != nil {
				return fmt.Errorf("read --push flag: %w", err)
			}
			remoteName, err := cmd.Flags().GetString("remote")
			if err != nil {
				return fmt.Errorf("read --remote flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if remoteName != "" && !pushEnabled {
				return fmt.Errorf("--remote can only be used with --push")
			}

			result, err := harnesspkg.PublishTemplate(cmd.Context(), harnesspkg.TemplatePublishInput{
				RepoRoot:        resolved.Repo.Root,
				TargetBranch:    targetBranch,
				DefaultTemplate: defaultTemplate,
				Push:            pushEnabled,
				Remote:          remoteName,
			})
			if err != nil {
				var publishErr *harnesspkg.TemplatePublishError
				if errors.As(err, &publishErr) {
					if emitErr := emitTemplatePublishResult(cmd, publishErr.Result, jsonOutput); emitErr != nil {
						return emitErr
					}
					return fmt.Errorf("publish harness template branch: %w", publishErr.Err)
				}
				return fmt.Errorf("publish harness template branch: %w", err)
			}

			return emitTemplatePublishResult(cmd, result, jsonOutput)
		},
	}

	cmd.Flags().String("to", "", "Target harness template branch name")
	cmd.Flags().Bool("default", false, "Mark the published harness template branch as the default template candidate")
	cmd.Flags().Bool("push", false, "Push the published harness template branch after local publish succeeds")
	cmd.Flags().String("remote", "", "Remote name to use with --push; defaults to origin")
	addPathFlag(cmd)
	addTemplatePublishPackageMetadataFlags(cmd)
	addJSONFlag(cmd)

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

func templatePublishResultPayload(cmd *cobra.Command, result harnesspkg.TemplatePublishResult) (templatePublishResultJSON, error) {
	packageMetadata, err := readTemplatePublishPackageMetadata(cmd, defaultTemplatePublishPackageName(result))
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
		HarnessID:          result.Preview.HarnessID,
		PublishRef:         "refs/heads/" + result.Preview.PublishBranch,
		NextAction:         "clone",
		NextRef:            "refs/heads/" + result.Preview.PublishBranch,
		Branch:             result.Preview.PublishBranch,
		SourceBranch:       result.Preview.SourceBranch,
		DefaultTemplate:    result.Preview.DefaultTemplate,
		LocalPublish: templatePublishLocalJSON{
			Success: result.LocalSuccess,
			Changed: result.Changed,
		},
		RemotePush: templatePublishRemoteJSON{
			Attempted: result.RemotePush.Attempted,
			Success:   result.RemotePush.Success,
			Remote:    result.RemotePush.Remote,
			Reason:    result.RemotePush.Reason,
		},
	}
	if result.Changed {
		payload.LocalPublish.Commit = result.Commit
	}

	return payload, nil
}

func emitTemplatePublishResult(cmd *cobra.Command, result harnesspkg.TemplatePublishResult, jsonOutput bool) error {
	packageMetadata, err := readTemplatePublishPackageMetadata(cmd, defaultTemplatePublishPackageName(result))
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
		"harness_id: %s\npublish_ref: refs/heads/%s\nnext_action: clone\nnext_ref: refs/heads/%s\nsource_branch: %s\ndefault_template: %t\nlocal_publish.success: %t\nlocal_publish.changed: %t\n",
		result.Preview.HarnessID,
		result.Preview.PublishBranch,
		result.Preview.PublishBranch,
		result.Preview.SourceBranch,
		result.Preview.DefaultTemplate,
		result.LocalSuccess,
		result.Changed,
	); err != nil {
		return fmt.Errorf("write command output: %w", err)
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

	return nil
}

func defaultTemplatePublishPackageName(result harnesspkg.TemplatePublishResult) string {
	name := strings.TrimPrefix(result.Preview.PublishBranch, "harness-template/")
	if name != "" && name != result.Preview.PublishBranch {
		return name
	}
	if result.Preview.PublishBranch != "" {
		return result.Preview.PublishBranch
	}

	return result.Preview.HarnessID
}
