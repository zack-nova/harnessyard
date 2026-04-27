package commands

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type createOutput struct {
	HarnessRoot      string `json:"harness_root"`
	ManifestPath     string `json:"manifest_path"`
	OrbitsDir        string `json:"orbits_dir"`
	GitInitialized   bool   `json:"git_initialized"`
	ManifestCreated  bool   `json:"manifest_created"`
	OrbitsDirCreated bool   `json:"orbits_dir_created"`
}

// NewCreateCommand creates the harness create command.
func NewCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <path>",
		Short: "Create a new harness runtime repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := absolutePathFromArg(cmd, args[0])
			if err != nil {
				return err
			}
			if err := os.MkdirAll(targetPath, 0o750); err != nil {
				return fmt.Errorf("create target directory %s: %w", targetPath, err)
			}

			gitInitialized, err := gitpkg.EnsureRepoRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("ensure git repo root: %w", err)
			}

			repo, err := gitpkg.DiscoverRepo(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			if gitpkg.ComparablePath(repo.Root) != gitpkg.ComparablePath(targetPath) {
				return fmt.Errorf("expected harness root %s to be a git repo root, got %s", targetPath, repo.Root)
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

			output := createOutput{
				HarnessRoot:      repo.Root,
				ManifestPath:     bootstrap.ManifestPath,
				OrbitsDir:        bootstrap.OrbitsDir,
				GitInitialized:   gitInitialized,
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

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "created harness in %s\n", repo.Root); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
