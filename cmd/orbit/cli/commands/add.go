package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type addOutput struct {
	RepoRoot   string `json:"repo_root"`
	Definition any    `json:"definition,omitempty"`
	Orbit      any    `json:"orbit,omitempty"`
	Schema     string `json:"schema,omitempty"`
	File       string `json:"file"`
}

// NewAddCommand creates the orbit create command and keeps "add" as a compatibility alias.
func NewAddCommand() *cobra.Command {
	var memberSchema bool
	var orbitName string
	var orbitDescription string
	var withSpec bool

	cmd := &cobra.Command{
		Use:     "create <orbit-id>",
		Aliases: []string{"add"},
		Short:   "Create an orbit definition skeleton",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			filename, err := orbitpkg.HostedDefinitionPath(repo.Root, args[0])
			if err != nil {
				return fmt.Errorf("build orbit definition path: %w", err)
			}
			if _, err := os.Stat(filename); err == nil {
				return fmt.Errorf("orbit definition file %q already exists", filename)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat orbit definition: %w", err)
			}

			config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
			if err != nil {
				return err
			}

			orbitID := args[0]
			if _, found := config.OrbitByID(orbitID); found {
				return fmt.Errorf("orbit %q already exists", orbitID)
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}

			spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
			if err != nil {
				return fmt.Errorf("build orbit skeleton: %w", err)
			}
			if orbitName != "" {
				spec.Name = orbitName
			}
			if orbitDescription != "" {
				spec.Description = orbitDescription
			}
			if withSpec {
				spec, err = orbitpkg.AddSpecMember(spec)
				if err != nil {
					return fmt.Errorf("add spec member: %w", err)
				}
				specDocPath, err := orbitpkg.SpecDocPath(repo.Root, orbitID)
				if err != nil {
					return fmt.Errorf("build spec doc path: %w", err)
				}
				if _, err := os.Stat(specDocPath); err == nil {
					return fmt.Errorf("spec doc file %q already exists", specDocPath)
				} else if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("stat spec doc: %w", err)
				}
			}
			spec, err = orbitpkg.SeedDefaultCapabilityTruth(spec)
			if err != nil {
				return fmt.Errorf("seed default capability truth: %w", err)
			}

			filename, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}
			if withSpec {
				if _, err := orbitpkg.WriteSpecDoc(repo.Root, orbitID); err != nil {
					return fmt.Errorf("write spec doc: %w", err)
				}
			}

			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), addOutput{
					RepoRoot: repo.Root,
					Orbit:    spec,
					Schema:   "members",
					File:     filename,
				})
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created orbit %s at %s\n", orbitID, filename)
			if err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	addJSONFlag(cmd)
	cmd.Flags().BoolVar(&memberSchema, "member-schema", false, "Compatibility no-op: orbit create always creates a member-schema orbit skeleton")
	cmd.Flags().StringVar(&orbitName, "name", "", "Set the orbit display name when creating a member-schema orbit")
	cmd.Flags().StringVar(&orbitDescription, "description", "", "Set the orbit description when creating a member-schema orbit")
	cmd.Flags().BoolVar(&withSpec, "with-spec", false, "Create docs/<orbit-id>.md and add a rule member keyed as spec")

	return cmd
}
