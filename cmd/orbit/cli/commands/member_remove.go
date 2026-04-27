package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type memberRemoveOutput struct {
	RepoRoot string `json:"repo_root"`
	File     string `json:"file"`
	OrbitID  string `json:"orbit_id"`
	Name     string `json:"name"`
}

// NewMemberRemoveCommand creates the orbit member remove command.
func NewMemberRemoveCommand() *cobra.Command {
	var orbitID string
	var memberName string
	var legacyMemberKey string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove one hosted orbit member",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			memberIdentity, err := resolveMemberIdentityInput(memberName, legacyMemberKey)
			if err != nil {
				return err
			}

			index := -1
			for candidateIndex, member := range spec.Members {
				if member.Name == memberIdentity || member.Key == memberIdentity {
					index = candidateIndex
					break
				}
			}
			if index < 0 {
				return fmt.Errorf("member %q not found in orbit %q", memberIdentity, definition.ID)
			}

			spec.Members = append(spec.Members[:index], spec.Members[index+1:]...)

			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), memberRemoveOutput{
					RepoRoot: repo.Root,
					File:     filename,
					OrbitID:  definition.ID,
					Name:     memberIdentity,
				})
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "removed member %s from orbit %s at %s\n", memberIdentity, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringVar(&memberName, "name", "", "Member name to remove")
	cmd.Flags().StringVar(&legacyMemberKey, "key", "", "Deprecated legacy member key")

	return cmd
}
