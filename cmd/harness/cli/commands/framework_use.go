package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type frameworkUseOutput struct {
	HarnessRoot     string   `json:"harness_root"`
	HarnessID       string   `json:"harness_id"`
	Framework       string   `json:"framework"`
	Frameworks      []string `json:"frameworks,omitempty"`
	SelectionSource string   `json:"selection_source"`
	SelectionPath   string   `json:"selection_path"`
}

// NewFrameworkUseCommand creates the harness framework use command.
func NewFrameworkUseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <framework> [framework...]",
		Short: "Select the current machine's framework for this runtime",
		Long: "Select the current machine's framework for this runtime by writing the repo-local selection.json file.\n" +
			"This command does not compose guidance or apply framework side effects.",
		Example: "" +
			"  harness framework use claude\n" +
			"  harness framework use codex claude-code\n" +
			"  harness framework use codex --json\n",
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

			frameworkIDs := make([]string, 0, len(args))
			seenFrameworks := map[string]struct{}{}
			for _, arg := range args {
				adapter, ok := harnesspkg.LookupFrameworkAdapter(arg)
				if !ok {
					return fmt.Errorf("framework %q is not supported by this build", arg)
				}
				if _, ok := seenFrameworks[adapter.ID]; ok {
					continue
				}
				seenFrameworks[adapter.ID] = struct{}{}
				frameworkIDs = append(frameworkIDs, adapter.ID)
			}
			frameworkID := frameworkIDs[0]

			selection := harnesspkg.FrameworkSelection{
				SelectedFramework:  frameworkID,
				SelectedFrameworks: frameworkIDs,
				SelectionSource:    harnesspkg.FrameworkSelectionSourceExplicitLocal,
				UpdatedAt:          time.Now().UTC(),
			}
			if len(frameworkIDs) == 1 {
				selection.SelectedFrameworks = nil
			}
			selectionPath, err := harnesspkg.WriteFrameworkSelection(resolved.Repo.GitDir, selection)
			if err != nil {
				return fmt.Errorf("write framework selection: %w", err)
			}

			output := frameworkUseOutput{
				HarnessRoot:     resolved.Repo.Root,
				HarnessID:       resolved.Manifest.Runtime.ID,
				Framework:       frameworkID,
				Frameworks:      frameworkIDs,
				SelectionSource: string(selection.SelectionSource),
				SelectionPath:   selectionPath,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "selected framework %s for harness %s\n", strings.Join(frameworkIDs, ","), resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
