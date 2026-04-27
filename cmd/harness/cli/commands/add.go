package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type addOutput struct {
	HarnessRoot  string `json:"harness_root"`
	OrbitID      string `json:"orbit_id"`
	Source       string `json:"source"`
	ManifestPath string `json:"manifest_path"`
	MemberCount  int    `json:"member_count"`
}

// NewAddCommand creates the harness add command.
func NewAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <orbit-id>",
		Short: "Add one orbit definition to the harness runtime as a manual member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			result, err := harnesspkg.AddManualMember(cmd.Context(), resolved.Repo.Root, args[0], time.Now().UTC())
			if err != nil {
				return fmt.Errorf("add manual harness member: %w", err)
			}

			output := addOutput{
				HarnessRoot:  resolved.Repo.Root,
				OrbitID:      args[0],
				Source:       harnesspkg.MemberSourceManual,
				ManifestPath: result.ManifestPath,
				MemberCount:  len(result.Runtime.Members),
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "added orbit %s to harness %s\n", args[0], resolved.Repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
