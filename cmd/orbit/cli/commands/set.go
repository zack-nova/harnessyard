package commands

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type setOutput struct {
	RepoRoot string `json:"repo_root"`
	File     string `json:"file"`
	Orbit    any    `json:"orbit"`
}

// NewSetCommand creates the orbit set command.
func NewSetCommand() *cobra.Command {
	var orbitID string
	var orbitName string
	var orbitDescription string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set top-level hosted orbit fields",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("name") && !cmd.Flags().Changed("description") {
				return errors.New("set requires at least one of --name or --description")
			}

			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			resolvedOrbitID, err := resolveAuthoredTruthOrbitID(cmd, repo, orbitID)
			if err != nil {
				return err
			}

			config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}
			definition, err := definitionByID(config, resolvedOrbitID)
			if err != nil {
				return err
			}

			spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
			if err != nil {
				return fmt.Errorf("load orbit spec: %w", err)
			}
			spec, err = orbitpkg.EnsureHostedMemberSchema(spec)
			if err != nil {
				return fmt.Errorf("upgrade hosted orbit spec: %w", err)
			}

			if cmd.Flags().Changed("name") {
				spec.Name = orbitName
			}
			if cmd.Flags().Changed("description") {
				spec.Description = orbitDescription
			}

			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), setOutput{
					RepoRoot: repo.Root,
					File:     filename,
					Orbit:    spec,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "updated orbit %s at %s\n", definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&orbitName, "name", "", "Set the orbit display name")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Set the orbit description")

	return cmd
}
