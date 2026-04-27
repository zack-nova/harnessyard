package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type capabilityAddOutput struct {
	RepoRoot   string `json:"repo_root"`
	OrbitID    string `json:"orbit"`
	File       string `json:"file"`
	Kind       string `json:"kind"`
	Capability any    `json:"capability"`
}

// NewCapabilityAddCommand creates the orbit capability add command.
func NewCapabilityAddCommand() *cobra.Command {
	var orbitID string
	var capabilityID string
	var capabilityPath string
	var capabilityDescription string

	cmd := &cobra.Command{
		Use:    "add <command|skill>",
		Short:  "Add one hosted orbit capability",
		Long:   "Add one hosted capability entry under capabilities.commands or capabilities.skills.",
		Hidden: true,
		Example: "" +
			"  orbit capability add command --orbit execute --id tdd-loop --path execute/commands/tdd-loop.md\n" +
			"  orbit capability add skill --orbit execute --id frontend-test-lab --path execute/skills/frontend-test-lab/SKILL.md --json\n",
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
			spec, err = orbitpkg.AddCapability(spec, kind, capabilityID, capabilityPath, capabilityDescription)
			if err != nil {
				return fmt.Errorf("add capability: %w", err)
			}

			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			capability := buildCapabilityOutput(kind, capabilityID, capabilityPath, capabilityDescription)
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), capabilityAddOutput{
					RepoRoot:   repo.Root,
					OrbitID:    definition.ID,
					File:       filename,
					Kind:       string(kind),
					Capability: capability,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "added %s capability %s to orbit %s at %s\n", kind, capabilityID, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&capabilityID, "id", "", "Unique capability id")
	cmd.Flags().StringVar(&capabilityPath, "path", "", "Repository-relative capability asset path")
	cmd.Flags().StringVar(&capabilityDescription, "description", "", "Optional capability description")
	mustMarkFlagRequired(cmd, "id")
	mustMarkFlagRequired(cmd, "path")

	return cmd
}
