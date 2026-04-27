package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type capabilityRemoveOutput struct {
	RepoRoot string `json:"repo_root"`
	OrbitID  string `json:"orbit"`
	File     string `json:"file"`
	Kind     string `json:"kind"`
	ID       string `json:"id"`
}

// NewCapabilityRemoveCommand creates the orbit capability remove command.
func NewCapabilityRemoveCommand() *cobra.Command {
	var orbitID string
	var capabilityID string

	cmd := &cobra.Command{
		Use:    "remove <command|skill>",
		Short:  "Remove one hosted orbit capability",
		Hidden: true,
		Example: "" +
			"  orbit capability remove command --orbit execute --id tdd-loop\n" +
			"  orbit capability remove skill --orbit execute --id frontend-test-lab --json\n",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			kind, err := orbitpkg.ParseCapabilityKind(args[0])
			if err != nil {
				return fmt.Errorf("parse capability kind: %w", err)
			}

			spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
			if err != nil {
				return fmt.Errorf("load orbit spec: %w", err)
			}
			spec, err = orbitpkg.EnsureHostedMemberSchema(spec)
			if err != nil {
				return fmt.Errorf("upgrade hosted orbit spec: %w", err)
			}
			spec, err = orbitpkg.RemoveCapability(spec, kind, capabilityID)
			if err != nil {
				return fmt.Errorf("remove capability: %w", err)
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
				return emitJSON(cmd.OutOrStdout(), capabilityRemoveOutput{
					RepoRoot: repo.Root,
					OrbitID:  definition.ID,
					File:     filename,
					Kind:     string(kind),
					ID:       capabilityID,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed %s capability %s from orbit %s at %s\n", kind, capabilityID, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&capabilityID, "id", "", "Capability id to remove")
	mustMarkFlagRequired(cmd, "id")

	return cmd
}
