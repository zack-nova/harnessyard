package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkListOutput struct {
	HarnessRoot            string                                      `json:"harness_root"`
	HarnessID              string                                      `json:"harness_id"`
	SupportedFrameworks    []string                                    `json:"supported_frameworks"`
	RecommendedFramework   string                                      `json:"recommended_framework,omitempty"`
	ResolvedFramework      string                                      `json:"resolved_framework,omitempty"`
	ResolutionSource       string                                      `json:"resolution_source"`
	PackageRecommendations []harnesspkg.FrameworkPackageRecommendation `json:"package_recommendations,omitempty"`
	Warnings               []string                                    `json:"warnings,omitempty"`
}

// NewFrameworkListCommand creates the harness framework list command.
func NewFrameworkListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List supported, recommended, and currently resolved frameworks",
		Long: "List the supported framework adapters in the current build, the runtime's recommended framework,\n" +
			"and the framework currently resolved on this machine.",
		Example: "" +
			"  harness framework list\n" +
			"  harness framework list --json\n",
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

			resolution, err := harnesspkg.ResolveFramework(cmd.Context(), harnesspkg.FrameworkResolutionInput{
				RepoRoot: resolved.Repo.Root,
				GitDir:   resolved.Repo.GitDir,
			})
			if err != nil {
				return fmt.Errorf("resolve framework: %w", err)
			}

			output := frameworkListOutput{
				HarnessRoot:            resolved.Repo.Root,
				HarnessID:              resolved.Manifest.Runtime.ID,
				SupportedFrameworks:    append([]string(nil), resolution.SupportedFrameworks...),
				RecommendedFramework:   resolution.RecommendedFramework,
				ResolvedFramework:      resolution.Framework,
				ResolutionSource:       string(resolution.Source),
				PackageRecommendations: append([]harnesspkg.FrameworkPackageRecommendation(nil), resolution.PackageRecommendations...),
				Warnings:               append([]string(nil), resolution.Warnings...),
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_root: %s\n", output.HarnessRoot); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "harness_id: %s\n", output.HarnessID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "supported_frameworks: %s\n", strings.Join(output.SupportedFrameworks, ",")); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if output.RecommendedFramework == "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "recommended_framework: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "recommended_framework: %s\n", output.RecommendedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if output.ResolvedFramework == "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "resolved_framework: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved_framework: %s\n", output.ResolvedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "resolution_source: %s\n", output.ResolutionSource); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, recommendation := range output.PackageRecommendations {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "package_recommendation: %s=%s\n", recommendation.HarnessID, recommendation.RecommendedFramework); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			if len(output.Warnings) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "warnings: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			} else {
				for _, warning := range output.Warnings {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning); err != nil {
						return fmt.Errorf("write command output: %w", err)
					}
				}
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
