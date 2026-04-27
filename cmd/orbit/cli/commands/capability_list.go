package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type capabilityListOutput struct {
	RepoRoot             string                                   `json:"repo_root"`
	OrbitID              string                                   `json:"orbit"`
	File                 string                                   `json:"file"`
	Capabilities         orbitpkg.OrbitCapabilities               `json:"capabilities"`
	ResolvedCommands     []orbitpkg.ResolvedCommandCapability     `json:"resolved_commands,omitempty" yaml:"resolved_commands,omitempty"`
	ResolvedLocalSkills  []orbitpkg.ResolvedLocalSkillCapability  `json:"resolved_local_skills,omitempty" yaml:"resolved_local_skills,omitempty"`
	ResolvedRemoteSkills []orbitpkg.ResolvedRemoteSkillCapability `json:"resolved_remote_skills,omitempty" yaml:"resolved_remote_skills,omitempty"`
}

// NewCapabilityListCommand creates the orbit capability list command.
func NewCapabilityListCommand() *cobra.Command {
	var orbitID string
	var resolve bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List hosted orbit capabilities",
		Example: "" +
			"  orbit capability list --orbit execute\n" +
			"  orbit capability list --orbit execute --json\n",
		Args: cobra.NoArgs,
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

			output := capabilityListOutput{
				RepoRoot:     repo.Root,
				OrbitID:      definition.ID,
				File:         spec.SourcePath,
				Capabilities: capabilityListValue(spec.Capabilities),
			}
			if resolve {
				resolved, err := resolveCapabilitiesForDisplay(cmd.Context(), repo.Root, config, spec)
				if err != nil {
					return err
				}
				output.ResolvedCommands = resolved.Commands
				output.ResolvedLocalSkills = resolved.LocalSkills
				output.ResolvedRemoteSkills = resolved.RemoteSkills
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitJSON(cmd.OutOrStdout(), output)
			}

			data, err := yaml.Marshal(output)
			if err != nil {
				return fmt.Errorf("marshal capabilities output: %w", err)
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), string(data)); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().BoolVar(&resolve, "resolve", false, "Resolve commands and skills from canonical capability truth")

	return cmd
}

func capabilityListValue(capabilities *orbitpkg.OrbitCapabilities) orbitpkg.OrbitCapabilities {
	if capabilities == nil {
		return orbitpkg.OrbitCapabilities{}
	}

	output := orbitpkg.OrbitCapabilities{}
	if capabilities.Commands != nil {
		output.Commands = &orbitpkg.OrbitCommandCapabilityPaths{
			Paths: orbitpkg.OrbitMemberPaths{
				Include: append([]string(nil), capabilities.Commands.Paths.Include...),
				Exclude: append([]string(nil), capabilities.Commands.Paths.Exclude...),
			},
		}
	}
	if capabilities.Skills != nil {
		output.Skills = &orbitpkg.OrbitSkillCapabilities{}
		if capabilities.Skills.Local != nil {
			output.Skills.Local = &orbitpkg.OrbitLocalSkillCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: append([]string(nil), capabilities.Skills.Local.Paths.Include...),
					Exclude: append([]string(nil), capabilities.Skills.Local.Paths.Exclude...),
				},
			}
		}
		if capabilities.Skills.Remote != nil {
			output.Skills.Remote = &orbitpkg.OrbitRemoteSkillCapabilities{
				URIs:         append([]string(nil), capabilities.Skills.Remote.URIs...),
				Dependencies: append([]orbitpkg.OrbitRemoteSkillDependency(nil), capabilities.Skills.Remote.Dependencies...),
			}
		}
	}

	return output
}

func buildCapabilityOutput(
	kind orbitpkg.CapabilityKind,
	capabilityID string,
	capabilityPath string,
	capabilityDescription string,
) any {
	switch kind {
	case orbitpkg.CapabilityKindCommand:
		return map[string]string{
			"id":   capabilityID,
			"path": capabilityPath,
		}
	case orbitpkg.CapabilityKindSkill:
		return map[string]string{
			"id":   capabilityID,
			"path": capabilityPath,
		}
	default:
		return map[string]string{
			"id":          capabilityID,
			"path":        capabilityPath,
			"description": capabilityDescription,
		}
	}
}
