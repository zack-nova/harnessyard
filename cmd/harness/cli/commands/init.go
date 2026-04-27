package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type initOutput struct {
	HarnessRoot      string `json:"harness_root"`
	ManifestPath     string `json:"manifest_path"`
	OrbitsDir        string `json:"orbits_dir"`
	ManifestCreated  bool   `json:"manifest_created"`
	OrbitsDirCreated bool   `json:"orbits_dir_created"`
}

// NewInitCommand creates the harness init command.
func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize harness runtime metadata in an existing Git repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromInitCommand(cmd, "harness create")
			if err != nil {
				return err
			}

			manifestPath := harnesspkg.ManifestPath(repo.Root)
			if _, err := os.Stat(manifestPath); err == nil {
				manifest, loadErr := harnesspkg.LoadManifestFile(repo.Root)
				if loadErr == nil {
					if manifest.Kind != harnesspkg.ManifestKindRuntime {
						return fmt.Errorf("harness already initialized with non-runtime manifest at %s", manifestPath)
					}
					return fmt.Errorf("harness runtime already initialized at %s", manifestPath)
				}
				return fmt.Errorf("load existing harness manifest: %w", loadErr)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat %s: %w", manifestPath, err)
			}

			bootstrap, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("bootstrap harness control plane: %w", err)
			}

			output := initOutput{
				HarnessRoot:      repo.Root,
				ManifestPath:     bootstrap.ManifestPath,
				OrbitsDir:        bootstrap.OrbitsDir,
				ManifestCreated:  bootstrap.ManifestCreated,
				OrbitsDirCreated: bootstrap.OrbitsDirCreated,
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "initialized harness in %s\n", repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}
