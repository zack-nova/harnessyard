package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type capabilitySetPathsOutput struct {
	RepoRoot     string                     `json:"repo_root"`
	OrbitID      string                     `json:"orbit"`
	File         string                     `json:"file"`
	Capabilities orbitpkg.OrbitCapabilities `json:"capabilities"`
}

func newCapabilitySetCommandsPathsCommand() *cobra.Command {
	var orbitID string
	var include []string
	var exclude []string
	var clearAll bool

	cmd := &cobra.Command{
		Use:   "commands-paths",
		Short: "Set hosted orbit command capability paths",
		Example: "" +
			"  orbit capability set commands-paths --orbit execute --include 'commands/execute/**/*.md'\n" +
			"  orbit capability set commands-paths --orbit execute --include 'commands/execute/**/*.md' --exclude 'commands/execute/_drafts/**' --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, err := loadHostedOrbitSpecForAuthoring(cmd, orbitID)
			if err != nil {
				return err
			}

			spec, err = orbitpkg.SetCommandCapabilityPaths(spec, include, exclude, clearAll)
			if err != nil {
				return fmt.Errorf("set command capability paths: %w", err)
			}

			return writeCapabilityPathsResult(cmd, repo.Root, definition.ID, spec)
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringArrayVar(&include, "include", nil, "Repository-relative command capability include pattern")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Repository-relative command capability exclude pattern")
	cmd.Flags().BoolVar(&clearAll, "clear", false, "Clear command capability truth")

	return cmd
}

func newCapabilitySetSkillsLocalPathsCommand() *cobra.Command {
	var orbitID string
	var include []string
	var exclude []string
	var clearAll bool

	cmd := &cobra.Command{
		Use:   "skills-local-paths",
		Short: "Set hosted orbit local skill capability paths",
		Example: "" +
			"  orbit capability set skills-local-paths --orbit execute --include 'skills/execute/*'\n" +
			"  orbit capability set skills-local-paths --orbit execute --include 'skills/execute/*' --exclude 'skills/execute/_archive/*' --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, err := loadHostedOrbitSpecForAuthoring(cmd, orbitID)
			if err != nil {
				return err
			}

			spec, err = orbitpkg.SetLocalSkillCapabilityPaths(spec, include, exclude, clearAll)
			if err != nil {
				return fmt.Errorf("set local skill capability paths: %w", err)
			}

			return writeCapabilityPathsResult(cmd, repo.Root, definition.ID, spec)
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringArrayVar(&include, "include", nil, "Repository-relative local skill capability include pattern")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Repository-relative local skill capability exclude pattern")
	cmd.Flags().BoolVar(&clearAll, "clear", false, "Clear local skill capability truth")

	return cmd
}

func newCapabilitySetSkillsRemoteURIsCommand() *cobra.Command {
	var orbitID string
	var uris []string
	var clearAll bool

	cmd := &cobra.Command{
		Use:   "skills-remote-uris",
		Short: "Set hosted orbit remote skill capability URIs",
		Example: "" +
			"  orbit capability set skills-remote-uris --orbit execute --uri github://acme/frontend-remote-skill\n" +
			"  orbit capability set skills-remote-uris --orbit execute --uri github://acme/frontend-remote-skill --uri https://example.com/skills/research-playbook --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, definition, spec, err := loadHostedOrbitSpecForAuthoring(cmd, orbitID)
			if err != nil {
				return err
			}

			spec, err = orbitpkg.SetRemoteSkillCapabilityURIs(spec, uris, clearAll)
			if err != nil {
				return fmt.Errorf("set remote skill capability URIs: %w", err)
			}

			return writeCapabilityPathsResult(cmd, repo.Root, definition.ID, spec)
		},
	}

	addJSONFlag(cmd)
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Target hosted orbit id; optional in single-orbit source/orbit_template branches")
	cmd.Flags().StringArrayVar(&uris, "uri", nil, "Remote skill capability URI")
	cmd.Flags().BoolVar(&clearAll, "clear", false, "Clear remote skill capability truth")

	return cmd
}

func writeCapabilityPathsResult(cmd *cobra.Command, repoRoot string, orbitID string, spec orbitpkg.OrbitSpec) error {
	filename, err := orbitpkg.WriteHostedOrbitSpec(repoRoot, spec)
	if err != nil {
		return fmt.Errorf("write orbit definition: %w", err)
	}

	output := capabilitySetPathsOutput{
		RepoRoot:     repoRoot,
		OrbitID:      orbitID,
		File:         filename,
		Capabilities: capabilityListValue(spec.Capabilities),
	}

	jsonOutput, err := wantJSON(cmd)
	if err != nil {
		return err
	}
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), output)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "updated capability truth for orbit %s at %s\n", orbitID, filename); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}

	return nil
}
