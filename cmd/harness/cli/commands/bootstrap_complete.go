package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type bootstrapCompleteOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.BootstrapCompleteResult
}

// NewBootstrapCompleteCommand creates the harness bootstrap complete command.
func NewBootstrapCompleteCommand() *cobra.Command {
	var orbitID string
	var completeAll bool

	cmd := &cobra.Command{
		Use:   "complete",
		Short: "Mark runtime bootstrap as completed and clean its runtime-only surface",
		Long: "Write bootstrap completion state into the repo-local runtime ledger, remove matching\n" +
			"BOOTSTRAP.md orbit blocks, and delete lane=bootstrap runtime files without changing authored truth.",
		Example: "" +
			"  harness bootstrap complete --orbit docs\n" +
			"  harness bootstrap complete --orbit docs --json\n" +
			"  harness bootstrap complete --all\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if completeAll && orbitID != "" {
				return fmt.Errorf("--orbit and --all cannot be used together")
			}
			if !completeAll && orbitID == "" {
				return fmt.Errorf("either --orbit or --all must be provided")
			}

			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			result, err := harnesspkg.CompleteRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapCompleteInput{
				OrbitID: orbitID,
				All:     completeAll,
				Now:     time.Now().UTC(),
			})
			if err != nil {
				return fmt.Errorf("complete runtime bootstrap: %w", err)
			}

			output := bootstrapCompleteOutput{
				HarnessRoot:             resolved.Repo.Root,
				HarnessID:               resolved.Manifest.Runtime.ID,
				BootstrapCompleteResult: result,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "completed bootstrap for %d orbit(s) in harness %s\n", len(result.CompletedOrbits), resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(result.AlreadyCompletedOrbits) > 0 {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "already completed: %s\n", strings.Join(result.AlreadyCompletedOrbits, ", ")); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Complete bootstrap for a specific runtime orbit")
	cmd.Flags().BoolVar(&completeAll, "all", false, "Complete bootstrap for all bootstrap-enabled runtime orbits")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
