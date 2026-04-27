package orbittemplate

import (
	"context"
	"fmt"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// MaterializeInitialOrbitGuidance creates the first editable root guidance artifacts
// for a newly created authoring orbit. It materializes authored truth when present
// and falls back to empty blocks when the corresponding guidance truth is still empty.
func MaterializeInitialOrbitGuidance(ctx context.Context, repoRoot string, orbitID string) error {
	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		return fmt.Errorf("load hosted orbit spec: %w", err)
	}

	if _, err := MaterializeOrbitBrief(ctx, BriefMaterializeInput{
		RepoRoot:  repoRoot,
		OrbitID:   orbitID,
		SeedEmpty: !HasOrbitAgentsBody(spec),
	}); err != nil {
		return fmt.Errorf("materialize root AGENTS.md: %w", err)
	}
	if _, err := MaterializeOrbitHumans(ctx, HumansMaterializeInput{
		RepoRoot:  repoRoot,
		OrbitID:   orbitID,
		SeedEmpty: !hasExplicitOrbitHumansTemplate(spec),
	}); err != nil {
		return fmt.Errorf("materialize root HUMANS.md: %w", err)
	}
	if _, err := MaterializeOrbitBootstrap(ctx, BootstrapMaterializeInput{
		RepoRoot:  repoRoot,
		OrbitID:   orbitID,
		SeedEmpty: !hasExplicitOrbitBootstrapTemplate(spec),
	}); err != nil {
		return fmt.Errorf("materialize root BOOTSTRAP.md: %w", err)
	}

	return nil
}
