package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type resolvedCapabilityOutput struct {
	Commands     []orbitpkg.ResolvedCommandCapability     `json:"commands,omitempty" yaml:"commands,omitempty"`
	LocalSkills  []orbitpkg.ResolvedLocalSkillCapability  `json:"local_skills,omitempty" yaml:"local_skills,omitempty"`
	RemoteSkills []orbitpkg.ResolvedRemoteSkillCapability `json:"remote_skills,omitempty" yaml:"remote_skills,omitempty"`
}

func hasAuthoredCapabilities(spec orbitpkg.OrbitSpec) bool {
	if spec.Capabilities == nil {
		return false
	}
	if spec.Capabilities.Commands != nil {
		if len(spec.Capabilities.Commands.Paths.Include) > 0 || len(spec.Capabilities.Commands.Paths.Exclude) > 0 {
			return true
		}
	}
	if spec.Capabilities.Skills == nil {
		return false
	}
	if spec.Capabilities.Skills.Local != nil {
		if len(spec.Capabilities.Skills.Local.Paths.Include) > 0 || len(spec.Capabilities.Skills.Local.Paths.Exclude) > 0 {
			return true
		}
	}
	if spec.Capabilities.Skills.Remote != nil &&
		(len(spec.Capabilities.Skills.Remote.URIs) > 0 || len(spec.Capabilities.Skills.Remote.Dependencies) > 0) {
		return true
	}

	return false
}

func loadHostedOrbitSpecForAuthoring(cmd *cobra.Command, orbitID string) (gitpkg.Repo, orbitpkg.Definition, orbitpkg.OrbitSpec, error) {
	repo, err := repoFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, err
	}
	resolvedOrbitID, err := resolveAuthoredTruthOrbitID(cmd, repo, orbitID)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, err
	}

	config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, err
	}
	definition, err := definitionByID(config, resolvedOrbitID)
	if err != nil {
		return gitpkg.Repo{}, orbitpkg.Definition{}, orbitpkg.OrbitSpec{}, err
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

func resolveCapabilitiesForDisplay(
	ctx context.Context,
	repoRoot string,
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
) (resolvedCapabilityOutput, error) {
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return resolvedCapabilityOutput{}, fmt.Errorf("load tracked files: %w", err)
	}
	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, trackedFiles)
	if err != nil {
		return resolvedCapabilityOutput{}, fmt.Errorf("resolve projection plan: %w", err)
	}

	commands, err := orbitpkg.ResolveCommandCapabilities(spec, trackedFiles, plan.ExportPaths)
	if err != nil {
		return resolvedCapabilityOutput{}, fmt.Errorf("resolve command capabilities: %w", err)
	}
	localSkills, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, plan.ExportPaths)
	if err != nil {
		return resolvedCapabilityOutput{}, fmt.Errorf("resolve local skill capabilities: %w", err)
	}
	remoteSkills, err := orbitpkg.ResolveRemoteSkillCapabilities(spec)
	if err != nil {
		return resolvedCapabilityOutput{}, fmt.Errorf("resolve remote skill capabilities: %w", err)
	}

	return resolvedCapabilityOutput{
		Commands:     commands,
		LocalSkills:  localSkills,
		RemoteSkills: remoteSkills,
	}, nil
}
