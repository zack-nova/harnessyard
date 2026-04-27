package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type bootstrapReopenOutput struct {
	HarnessRoot string `json:"harness_root"`
	HarnessID   string `json:"harness_id"`
	harnesspkg.BootstrapReopenResult
}

// NewBootstrapReopenCommand creates the harness bootstrap reopen command.
func NewBootstrapReopenCommand() *cobra.Command {
	var orbitID string
	var reopenAll bool
	var restoreSurface bool

	cmd := &cobra.Command{
		Use:   "reopen",
		Short: "Reopen runtime bootstrap back to pending",
		Long: "Clear bootstrap completion state for one or more runtime orbits and return them to pending.\n" +
			"By default this only\n" +
			"reopens the runtime ledger state; use --restore-surface to explicitly restore BOOTSTRAP.md\n" +
			"blocks and lane=bootstrap runtime files when a stable restore source exists.",
		Example: "" +
			"  harness bootstrap reopen --orbit docs\n" +
			"  harness bootstrap reopen --orbit docs --restore-surface\n" +
			"  harness bootstrap reopen --all --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if reopenAll && orbitID != "" {
				return fmt.Errorf("--orbit and --all cannot be used together")
			}
			if !reopenAll && orbitID == "" {
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

			result, err := harnesspkg.ReopenRuntimeBootstrap(cmd.Context(), resolved.Repo, harnesspkg.BootstrapReopenInput{
				OrbitID:        orbitID,
				All:            reopenAll,
				RestoreSurface: restoreSurface,
				Now:            time.Now().UTC(),
			})
			if err != nil {
				return fmt.Errorf("reopen runtime bootstrap: %w", err)
			}

			output := bootstrapReopenOutput{
				HarnessRoot:           resolved.Repo.Root,
				HarnessID:             resolved.Manifest.Runtime.ID,
				BootstrapReopenResult: result,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reopened bootstrap for %d orbit(s) in harness %s\n", len(result.ReopenedOrbits), resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(result.AlreadyPendingOrbits) > 0 {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "already pending: %s\n", strings.Join(result.AlreadyPendingOrbits, ", ")); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&orbitID, "orbit", "", "Reopen bootstrap for a specific runtime orbit")
	cmd.Flags().BoolVar(&reopenAll, "all", false, "Reopen bootstrap for all bootstrap-enabled runtime orbits")
	cmd.Flags().BoolVar(&restoreSurface, "restore-surface", false, "Also restore BOOTSTRAP.md blocks and lane=bootstrap runtime files when a stable source exists")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
