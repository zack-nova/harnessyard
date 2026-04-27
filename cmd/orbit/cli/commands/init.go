package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

type initOutput struct {
	RepoRoot      string `json:"repo_root"`
	ConfigPath    string `json:"config_path"`
	OrbitsDir     string `json:"orbits_dir"`
	StateDir      string `json:"state_dir"`
	ConfigCreated bool   `json:"config_created"`
	OrbitsCreated bool   `json:"orbits_created"`
	Deprecated    bool   `json:"deprecated"`
	MigrationHint string `json:"migration_hint"`
}

// NewInitCommand creates the orbit init command.
func NewInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize Orbit config and runtime state",
		Long: "Initialize the legacy Orbit compatibility config and local runtime state.\n" +
			"This command is deprecated for new harness-centric repositories; use `harness init`\n" +
			"to bootstrap the formal runtime control plane.",
		Example: "" +
			"  harness init\n" +
			"  orbit init --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromRuntimeInitCommand(cmd, "harness create")
			if err != nil {
				return err
			}

			initResult, err := orbitpkg.EnsureInitialized(repo.Root)
			if err != nil {
				return fmt.Errorf("initialize compatibility orbit config: %w", err)
			}

			store, err := statepkg.NewFSStore(repo.GitDir)
			if err != nil {
				return fmt.Errorf("create state store: %w", err)
			}
			if err := store.Ensure(); err != nil {
				return fmt.Errorf("initialize state store: %w", err)
			}

			output := initOutput{
				RepoRoot:      repo.Root,
				ConfigPath:    initResult.ConfigPath,
				OrbitsDir:     initResult.OrbitsDir,
				StateDir:      store.StateDir,
				ConfigCreated: initResult.ConfigCreated,
				OrbitsCreated: initResult.OrbitsDirCreated,
				Deprecated:    true,
				MigrationHint: "orbit init is deprecated; use harness init",
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"initialized orbit in %s\nmigration_hint: %s\n",
				repo.Root,
				output.MigrationHint,
			)
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
