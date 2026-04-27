package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type listedOrbit struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	SourcePath  string `json:"source_path"`
}

type listOutput struct {
	RepoRoot string        `json:"repo_root"`
	Orbits   []listedOrbit `json:"orbits"`
}

// NewListCommand creates the orbit list command.
func NewListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured orbits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			runtimeManifest, err := runtimeManifestPresent(repo.Root)
			if err != nil {
				return err
			}

			var definitions []orbitpkg.Definition
			if runtimeManifest {
				config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
				if err != nil {
					return err
				}
				definitions = config.Orbits
			} else {
				definitions, err = orbitpkg.DiscoverHostedDefinitions(cmd.Context(), repo.Root)
				if err != nil {
					return fmt.Errorf("discover orbit definitions: %w", err)
				}
			}

			output := listOutput{
				RepoRoot: repo.Root,
				Orbits:   make([]listedOrbit, 0, len(definitions)),
			}

			for _, definition := range definitions {
				output.Orbits = append(output.Orbits, listedOrbit{
					ID:          definition.ID,
					Description: definition.Description,
					SourcePath:  definition.SourcePath,
				})
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			if len(output.Orbits) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "no orbits configured")
				if err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
				return nil
			}

			for _, orbitItem := range output.Orbits {
				if orbitItem.Description == "" {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), orbitItem.ID)
				} else {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", orbitItem.ID, orbitItem.Description)
				}
				if err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	addJSONFlag(cmd)

	return cmd
}
