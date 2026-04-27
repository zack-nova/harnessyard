package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type capabilitySetOutput struct {
	RepoRoot   string `json:"repo_root"`
	OrbitID    string `json:"orbit"`
	File       string `json:"file"`
	Kind       string `json:"kind"`
	Capability any    `json:"capability"`
}

func newCapabilitySetEntryCommand(kind string) *cobra.Command {
	var orbitID string
	var capabilityID string
	var capabilityPath string
	var capabilityDescription string

	cmd := &cobra.Command{
		Use:    kind,
		Short:  fmt.Sprintf("Compatibility path: set one hosted orbit %s capability", kind),
		Long:   fmt.Sprintf("Compatibility command that rewrites one %s asset into the v0.66 path-scoped capability truth.", kind),
		Hidden: true,
		Example: "" +
			"  orbit capability set command --orbit execute --id tdd-loop --path execute/commands/tdd-loop.md\n" +
			"  orbit capability set skill --orbit execute --id frontend-test-lab --path execute/skills/frontend-test-lab/SKILL.md --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, err := loadHostedOrbitSpecForAuthoring(cmd, orbitID)
			if err != nil {
				return err
			}

			parsedKind, err := orbitpkg.ParseCapabilityKind(kind)
			if err != nil {
				return fmt.Errorf("parse capability kind: %w", err)
			}

			spec, err = orbitpkg.SetCapability(spec, parsedKind, capabilityID, capabilityPath, capabilityDescription)
			if err != nil {
				return fmt.Errorf("set %s capability: %w", parsedKind, err)
			}

			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			capability := buildCapabilityOutput(parsedKind, capabilityID, capabilityPath, capabilityDescription)
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), capabilitySetOutput{
					RepoRoot:   repo.Root,
					OrbitID:    definition.ID,
					File:       filename,
					Kind:       string(parsedKind),
					Capability: capability,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "updated %s capability %s in orbit %s at %s\n", parsedKind, capabilityID, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&capabilityID, "id", "", "Capability id to update")
	cmd.Flags().StringVar(&capabilityPath, "path", "", "Repository-relative capability asset path")
	cmd.Flags().StringVar(&capabilityDescription, "description", "", "Optional capability description")
	mustMarkFlagRequired(cmd, "id")
	mustMarkFlagRequired(cmd, "path")

	return cmd
}
