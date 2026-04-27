package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type orbitSkillDependencyJSON struct {
	URI      string `json:"uri"`
	Required bool   `json:"required"`
	Strength string `json:"strength"`
}

type orbitSkillMutationOutput struct {
	RepoRoot   string                   `json:"repo_root"`
	OrbitID    string                   `json:"orbit"`
	File       string                   `json:"file"`
	URI        string                   `json:"uri"`
	Required   bool                     `json:"required"`
	Dependency orbitSkillDependencyJSON `json:"dependency"`
}

type orbitSkillInspectOutput struct {
	RepoRoot             string                     `json:"repo_root"`
	OrbitID              string                     `json:"orbit"`
	File                 string                     `json:"file"`
	RemoteDependencies   []orbitSkillDependencyJSON `json:"remote_dependencies"`
	MigrationWarnings    []string                   `json:"migration_warnings,omitempty"`
	LocalSkillModeNotice string                     `json:"local_skill_mode_notice,omitempty"`
}

func newOrbitSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Link remote skills to hosted orbit authored truth",
		Long: "Link, unlink, and inspect remote skill dependencies through the hyard user surface.\n" +
			"Skill links write Orbit authored truth only; framework activation remains under `hyard agent`.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(
		newOrbitSkillLinkCommand(),
		newOrbitSkillUnlinkCommand(),
		newOrbitSkillInspectCommand(),
	)

	return cmd
}

func newOrbitSkillLinkCommand() *cobra.Command {
	var orbitID string
	var required bool

	cmd := &cobra.Command{
		Use:   "link <skill-uri>",
		Short: "Link one remote skill URI to an orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, definition, spec, err := loadHyardOrbitSkillSpec(cmd, orbitID)
			if err != nil {
				return err
			}

			spec, dependency, err := orbitpkg.LinkRemoteSkillDependency(spec, args[0], required)
			if err != nil {
				return fmt.Errorf("link remote skill dependency: %w", err)
			}
			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			output := orbitSkillMutationOutput{
				RepoRoot:   repo.Root,
				OrbitID:    definition.ID,
				File:       filename,
				URI:        dependency.URI,
				Required:   dependency.Required,
				Dependency: orbitSkillDependencyJSONFromDependency(dependency),
			}
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			strength := "recommended"
			if dependency.Required {
				strength = "required"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "linked %s skill %s to orbit %s at %s\n", strength, dependency.URI, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().BoolVar(&required, "required", false, "Mark the linked skill as required for agent activation")
	addHyardJSONFlag(cmd)

	return cmd
}

func newOrbitSkillUnlinkCommand() *cobra.Command {
	var orbitID string

	cmd := &cobra.Command{
		Use:   "unlink <skill-uri>",
		Short: "Unlink one remote skill URI from an orbit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, definition, spec, err := loadHyardOrbitSkillSpec(cmd, orbitID)
			if err != nil {
				return err
			}

			spec, dependency, err := orbitpkg.UnlinkRemoteSkillDependency(spec, args[0])
			if err != nil {
				return fmt.Errorf("unlink remote skill dependency: %w", err)
			}
			filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
			if err != nil {
				return fmt.Errorf("write orbit definition: %w", err)
			}

			output := orbitSkillMutationOutput{
				RepoRoot:   repo.Root,
				OrbitID:    definition.ID,
				File:       filename,
				URI:        dependency.URI,
				Required:   dependency.Required,
				Dependency: orbitSkillDependencyJSONFromDependency(dependency),
			}
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "unlinked skill %s from orbit %s at %s\n", dependency.URI, definition.ID, filename); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	addHyardJSONFlag(cmd)

	return cmd
}

func newOrbitSkillInspectCommand() *cobra.Command {
	var orbitID string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect remote skill links for an orbit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, err := loadHyardOrbitSkillSpec(cmd, orbitID)
			if err != nil {
				return err
			}

			resolved, err := orbitpkg.ResolveRemoteSkillCapabilities(spec)
			if err != nil {
				return fmt.Errorf("resolve remote skill dependencies: %w", err)
			}
			output := orbitSkillInspectOutput{
				RepoRoot:             repo.Root,
				OrbitID:              definition.ID,
				File:                 spec.SourcePath,
				RemoteDependencies:   make([]orbitSkillDependencyJSON, 0, len(resolved)),
				LocalSkillModeNotice: "local skills are treated as vendored / authoring compatibility assets; remote skill links are the recommended steady state",
			}
			for _, dependency := range resolved {
				output.RemoteDependencies = append(output.RemoteDependencies, orbitSkillDependencyJSONFromResolved(dependency))
			}
			if spec.Capabilities != nil &&
				spec.Capabilities.Skills != nil &&
				spec.Capabilities.Skills.Remote != nil &&
				len(spec.Capabilities.Skills.Remote.URIs) > 0 {
				output.MigrationWarnings = append(output.MigrationWarnings, "legacy capabilities.skills.remote.uris detected; migrate to capabilities.skills.remote.dependencies")
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s\n", output.OrbitID); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
			if len(output.RemoteDependencies) == 0 {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), "remote skills: none"); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}
			for _, dependency := range output.RemoteDependencies {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "remote skill: %s (%s)\n", dependency.URI, dependency.Strength); err != nil {
					return fmt.Errorf("write command output: %w", err)
				}
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	addHyardJSONFlag(cmd)

	return cmd
}

func loadHyardOrbitSkillSpec(cmd *cobra.Command, requestedOrbitID string) (gitpkg.Repo, orbitpkg.Definition, orbitpkg.OrbitSpec, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, err
	}
	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("discover git repository: %w", err)
	}

	config, err := orbitpkg.LoadHostedRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("load hosted repository config: %w", err)
	}
	orbitID := strings.TrimSpace(requestedOrbitID)
	if orbitID == "" {
		switch len(config.Orbits) {
		case 0:
			return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("orbit skill command requires --orbit because no hosted orbit definitions were found")
		case 1:
			orbitID = config.Orbits[0].ID
		default:
			return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("orbit skill command requires --orbit when multiple hosted orbit definitions exist")
		}
	}
	definition, ok := config.OrbitByID(orbitID)
	if !ok {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("orbit %q not found", orbitID)
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repo.Root, definition.ID)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("load orbit spec: %w", err)
	}
	spec, err = orbitpkg.EnsureHostedMemberSchema(spec)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, fmt.Errorf("upgrade hosted orbit spec: %w", err)
	}

	return repo, definition, spec, nil
}

func orbitSkillDependencyJSONFromDependency(dependency orbitpkg.OrbitRemoteSkillDependency) orbitSkillDependencyJSON {
	return orbitSkillDependencyJSON{
		URI:      dependency.URI,
		Required: dependency.Required,
		Strength: remoteSkillDependencyStrength(dependency.Required),
	}
}

func orbitSkillDependencyJSONFromResolved(dependency orbitpkg.ResolvedRemoteSkillCapability) orbitSkillDependencyJSON {
	return orbitSkillDependencyJSON{
		URI:      dependency.URI,
		Required: dependency.Required,
		Strength: remoteSkillDependencyStrength(dependency.Required),
	}
}

func remoteSkillDependencyStrength(required bool) string {
	if required {
		return "required"
	}

	return "recommended"
}

func addHyardJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func wantHyardJSON(cmd *cobra.Command) (bool, error) {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return false, fmt.Errorf("read json flag: %w", err)
	}

	return jsonOutput, nil
}
