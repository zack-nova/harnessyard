package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type agentConfigImportOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.AgentConfigImportResult
}

// NewAgentConfigImportCommand creates the harness agent config import command.
func NewAgentConfigImportCommand() *cobra.Command {
	var yes bool
	var replace bool
	var preserveNative bool
	cmd := &cobra.Command{
		Use:   "import <framework>",
		Short: "Import local native agent config into harness truth",
		Long: "Import local native agent config for one supported framework into harness agent truth.\n" +
			"By default this previews the project-over-global import without writing files.",
		Example: "" +
			"  harness agent config import codex\n" +
			"  harness agent config import codex --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			result, err := harnesspkg.ImportAgentConfig(cmd.Context(), harnesspkg.AgentConfigImportInput{
				RepoRoot:       resolved.Repo.Root,
				Framework:      args[0],
				Write:          yes,
				Replace:        replace,
				PreserveNative: preserveNative,
			})
			if err != nil {
				return fmt.Errorf("import agent config: %w", err)
			}

			output := agentConfigImportOutput{
				HarnessRoot:             resolved.Repo.Root,
				HarnessID:               resolved.Manifest.Runtime.ID,
				AgentConfigImportResult: result,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "framework: %s\n", output.Framework); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry_run: %t\n", output.DryRun); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			for _, entry := range output.Imported {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "import: %s source=%s\n", entry.Key, entry.Source); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, entry := range output.Skipped {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skip: %s source=%s reason=%s\n", entry.Key, entry.Source, entry.Reason); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, sidecar := range output.Sidecars {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "sidecar: %s source=%s reason=%s\n", sidecar.Path, sidecar.Source, sidecar.Reason); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, sidecar := range output.SkippedSidecars {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "skip_sidecar: %s source=%s reason=%s\n", sidecar.Path, sidecar.Source, sidecar.Reason); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, path := range output.WrittenPaths {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "write: %s\n", path); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)
	cmd.Flags().BoolVar(&yes, "yes", false, "Write imported harness agent truth instead of previewing")
	cmd.Flags().BoolVar(&replace, "replace", false, "Replace existing harness agent config keys with imported native values")
	cmd.Flags().BoolVar(&preserveNative, "preserve-native", false, "Preserve the selected native config source as an agent config sidecar")

	return cmd
}
