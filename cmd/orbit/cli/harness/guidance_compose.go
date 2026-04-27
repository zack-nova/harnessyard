package harness

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const rootHumansPath = "HUMANS.md"
const rootBootstrapPath = "BOOTSTRAP.md"

// GuidanceTarget identifies one runtime guidance artifact target.
type GuidanceTarget = orbittemplate.GuidanceTarget

const (
	GuidanceTargetAgents    GuidanceTarget = orbittemplate.GuidanceTargetAgents
	GuidanceTargetHumans    GuidanceTarget = orbittemplate.GuidanceTargetHumans
	GuidanceTargetBootstrap GuidanceTarget = orbittemplate.GuidanceTargetBootstrap
	GuidanceTargetAll       GuidanceTarget = orbittemplate.GuidanceTargetAll
)

// ComposeRuntimeGuidanceInput describes one explicit runtime guidance compose request.
type ComposeRuntimeGuidanceInput struct {
	RepoRoot string
	Force    bool
	Target   GuidanceTarget
	OrbitIDs []string
}

// ComposeRuntimeAgentsInput describes one explicit root AGENTS compose request.
type ComposeRuntimeAgentsInput struct {
	RepoRoot string
	Force    bool
}

// ComposeRuntimeAgentsResult reports the runtime members considered during one compose.
type ComposeRuntimeAgentsResult struct {
	AgentsPath       string
	MemberCount      int
	ComposedOrbitIDs []string
	SkippedOrbitIDs  []string
	ChangedCount     int
	Forced           bool
}

// ComposeRuntimeGuidanceArtifactResult reports one target-specific materialized guidance artifact.
type ComposeRuntimeGuidanceArtifactResult struct {
	Target           GuidanceTarget
	Path             string
	ComposedOrbitIDs []string
	SkippedOrbitIDs  []string
	ChangedCount     int
}

// ComposeRuntimeGuidanceResult reports one runtime guidance compose request.
type ComposeRuntimeGuidanceResult struct {
	MemberCount int
	Target      GuidanceTarget
	Artifacts   []ComposeRuntimeGuidanceArtifactResult
	Forced      bool
}

// ComposeRuntimeHumansResult reports the runtime members considered during one compose.
type ComposeRuntimeHumansResult struct {
	HumansPath       string
	MemberCount      int
	ComposedOrbitIDs []string
	SkippedOrbitIDs  []string
	ChangedCount     int
	Forced           bool
}

type runtimeGuidanceTarget struct {
	target       GuidanceTarget
	relativePath string
	preflight    func(context.Context, string, string, string, bool) (bool, error)
	materialize  func(context.Context, string, string, bool) (bool, error)
}

// ComposeRuntimeGuidance materializes authored runtime guidance artifacts for the requested target.
func ComposeRuntimeGuidance(ctx context.Context, input ComposeRuntimeGuidanceInput) (ComposeRuntimeGuidanceResult, error) {
	if input.RepoRoot == "" {
		return ComposeRuntimeGuidanceResult{}, fmt.Errorf("repo root must not be empty")
	}

	target, err := normalizeGuidanceTarget(input.Target)
	if err != nil {
		return ComposeRuntimeGuidanceResult{}, err
	}

	runtimeFile, err := LoadRuntimeFile(input.RepoRoot)
	if err != nil {
		return ComposeRuntimeGuidanceResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	members, err := selectRuntimeGuidanceMembers(runtimeFile.Members, input.OrbitIDs)
	if err != nil {
		return ComposeRuntimeGuidanceResult{}, err
	}

	targets, err := guidanceTargetsForComposeTarget(target)
	if err != nil {
		return ComposeRuntimeGuidanceResult{}, err
	}
	repo, err := gitpkg.DiscoverRepo(ctx, input.RepoRoot)
	if err != nil {
		return ComposeRuntimeGuidanceResult{}, fmt.Errorf("discover repository git dir: %w", err)
	}

	result := ComposeRuntimeGuidanceResult{
		MemberCount: len(members),
		Target:      target,
		Artifacts:   make([]ComposeRuntimeGuidanceArtifactResult, 0, len(targets)),
		Forced:      input.Force,
	}

	type preparedTarget struct {
		target   runtimeGuidanceTarget
		result   ComposeRuntimeGuidanceArtifactResult
		orbitIDs []string
	}
	prepared := make([]preparedTarget, 0, len(targets))
	for _, target := range targets {
		preparedResult, orbitIDs, err := prepareRuntimeGuidanceTarget(ctx, input.RepoRoot, repo.GitDir, members, input.Force, target)
		if err != nil {
			return ComposeRuntimeGuidanceResult{}, err
		}
		prepared = append(prepared, preparedTarget{
			target:   target,
			result:   preparedResult,
			orbitIDs: orbitIDs,
		})
	}

	for _, target := range prepared {
		artifactResult := target.result
		for _, orbitID := range target.orbitIDs {
			changed, err := target.target.materialize(ctx, input.RepoRoot, orbitID, input.Force)
			if err != nil {
				return ComposeRuntimeGuidanceResult{}, fmt.Errorf("compose orbit %q into root %s: %w", orbitID, strings.TrimSuffix(target.target.relativePath, ".md")+".md", err)
			}
			artifactResult.ComposedOrbitIDs = append(artifactResult.ComposedOrbitIDs, orbitID)
			if changed {
				artifactResult.ChangedCount++
			}
		}
		result.Artifacts = append(result.Artifacts, artifactResult)
	}

	return result, nil
}

func selectRuntimeGuidanceMembers(members []RuntimeMember, orbitIDs []string) ([]RuntimeMember, error) {
	if len(orbitIDs) == 0 {
		return append([]RuntimeMember(nil), members...), nil
	}

	wanted := make(map[string]struct{}, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		trimmed := strings.TrimSpace(orbitID)
		if err := ids.ValidateOrbitID(trimmed); err != nil {
			return nil, fmt.Errorf("validate orbit_id %q: %w", orbitID, err)
		}
		wanted[trimmed] = struct{}{}
	}

	selected := make([]RuntimeMember, 0, len(wanted))
	seen := make(map[string]struct{}, len(wanted))
	for _, member := range members {
		if _, ok := wanted[member.OrbitID]; !ok {
			continue
		}
		selected = append(selected, member)
		seen[member.OrbitID] = struct{}{}
	}

	for orbitID := range wanted {
		if _, ok := seen[orbitID]; ok {
			continue
		}
		return nil, fmt.Errorf("orbit_id %q is not a current runtime member", orbitID)
	}

	return selected, nil
}

// ComposeRuntimeAgents materializes authored orbit briefs for all current runtime members that
// define a brief, preserving unrelated prose and non-target blocks in root AGENTS.md.
func ComposeRuntimeAgents(ctx context.Context, input ComposeRuntimeAgentsInput) (ComposeRuntimeAgentsResult, error) {
	result, err := ComposeRuntimeGuidance(ctx, ComposeRuntimeGuidanceInput{
		RepoRoot: input.RepoRoot,
		Force:    input.Force,
		Target:   GuidanceTargetAgents,
	})
	if err != nil {
		return ComposeRuntimeAgentsResult{}, err
	}
	if len(result.Artifacts) != 1 {
		return ComposeRuntimeAgentsResult{}, fmt.Errorf("compose runtime AGENTS: expected exactly one agent artifact, got %d", len(result.Artifacts))
	}
	artifact := result.Artifacts[0]

	return ComposeRuntimeAgentsResult{
		AgentsPath:       artifact.Path,
		MemberCount:      result.MemberCount,
		ComposedOrbitIDs: append([]string(nil), artifact.ComposedOrbitIDs...),
		SkippedOrbitIDs:  append([]string(nil), artifact.SkippedOrbitIDs...),
		ChangedCount:     artifact.ChangedCount,
		Forced:           result.Forced,
	}, nil
}

// ComposeRuntimeHumans materializes authored orbit human guidance for all current runtime members.
func ComposeRuntimeHumans(ctx context.Context, input ComposeRuntimeAgentsInput) (ComposeRuntimeHumansResult, error) {
	result, err := ComposeRuntimeGuidance(ctx, ComposeRuntimeGuidanceInput{
		RepoRoot: input.RepoRoot,
		Force:    input.Force,
		Target:   GuidanceTargetHumans,
	})
	if err != nil {
		return ComposeRuntimeHumansResult{}, err
	}
	if len(result.Artifacts) != 1 {
		return ComposeRuntimeHumansResult{}, fmt.Errorf("compose runtime HUMANS: expected exactly one human artifact, got %d", len(result.Artifacts))
	}
	artifact := result.Artifacts[0]

	return ComposeRuntimeHumansResult{
		HumansPath:       artifact.Path,
		MemberCount:      result.MemberCount,
		ComposedOrbitIDs: append([]string(nil), artifact.ComposedOrbitIDs...),
		SkippedOrbitIDs:  append([]string(nil), artifact.SkippedOrbitIDs...),
		ChangedCount:     artifact.ChangedCount,
		Forced:           result.Forced,
	}, nil
}

func normalizeGuidanceTarget(target GuidanceTarget) (GuidanceTarget, error) {
	resolved, err := orbittemplate.NormalizeGuidanceTarget(target)
	if err != nil {
		return "", fmt.Errorf("normalize guidance target: %w", err)
	}

	return resolved, nil
}

func guidanceTargetsForComposeTarget(target GuidanceTarget) ([]runtimeGuidanceTarget, error) {
	switch target {
	case GuidanceTargetAll:
		return []runtimeGuidanceTarget{agentGuidanceTarget(), humanGuidanceTarget(), bootstrapGuidanceTarget()}, nil
	case GuidanceTargetAgents:
		return []runtimeGuidanceTarget{agentGuidanceTarget()}, nil
	case GuidanceTargetHumans:
		return []runtimeGuidanceTarget{humanGuidanceTarget()}, nil
	case GuidanceTargetBootstrap:
		return []runtimeGuidanceTarget{bootstrapGuidanceTarget()}, nil
	default:
		return nil, fmt.Errorf("unsupported guidance target %q", target)
	}
}

func prepareRuntimeGuidanceTarget(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	members []RuntimeMember,
	force bool,
	target runtimeGuidanceTarget,
) (ComposeRuntimeGuidanceArtifactResult, []string, error) {
	result := ComposeRuntimeGuidanceArtifactResult{
		Target: target.target,
		Path:   filepath.Join(repoRoot, target.relativePath),
	}

	orbitIDs := make([]string, 0, len(members))
	for _, member := range members {
		skip, err := target.preflight(ctx, repoRoot, gitDir, member.OrbitID, force)
		if err != nil {
			return ComposeRuntimeGuidanceArtifactResult{}, nil, err
		}
		if skip {
			result.SkippedOrbitIDs = append(result.SkippedOrbitIDs, member.OrbitID)
			continue
		}
		orbitIDs = append(orbitIDs, member.OrbitID)
	}

	return result, orbitIDs, nil
}

func agentGuidanceTarget() runtimeGuidanceTarget {
	return runtimeGuidanceTarget{
		target:       GuidanceTargetAgents,
		relativePath: rootAgentsPath,
		preflight: func(ctx context.Context, repoRoot string, _ string, orbitID string, force bool) (bool, error) {
			spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
			if err != nil {
				return false, fmt.Errorf("load hosted orbit %q: %w", orbitID, err)
			}
			if !orbittemplate.HasOrbitAgentsBody(spec) {
				return true, nil
			}
			status, err := orbittemplate.InspectOrbitBriefLane(ctx, repoRoot, orbitID)
			if err != nil {
				return false, fmt.Errorf("inspect orbit %q brief lane: %w", orbitID, err)
			}
			if status.MaterializeRequiresForce && !force {
				return false, fmt.Errorf(
					"compose orbit %q into root AGENTS.md: root AGENTS.md already contains drifted orbit block %q; run `orbit brief backfill --orbit %s` or rerun with --force to overwrite it",
					orbitID,
					orbitID,
					orbitID,
				)
			}
			return false, nil
		},
		materialize: func(ctx context.Context, repoRoot string, orbitID string, force bool) (bool, error) {
			materializeResult, err := orbittemplate.MaterializeOrbitBrief(ctx, orbittemplate.BriefMaterializeInput{
				RepoRoot: repoRoot,
				OrbitID:  orbitID,
				Force:    force,
			})
			if err != nil {
				return false, fmt.Errorf("materialize orbit brief: %w", err)
			}
			return materializeResult.Changed, nil
		},
	}
}

func humanGuidanceTarget() runtimeGuidanceTarget {
	return runtimeGuidanceTarget{
		target:       GuidanceTargetHumans,
		relativePath: rootHumansPath,
		preflight: func(ctx context.Context, repoRoot string, _ string, orbitID string, force bool) (bool, error) {
			spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
			if err != nil {
				return false, fmt.Errorf("load hosted orbit %q: %w", orbitID, err)
			}
			if spec.Meta == nil || strings.TrimSpace(spec.Meta.HumansTemplate) == "" {
				return true, nil
			}
			status, err := orbittemplate.InspectOrbitHumansLane(ctx, repoRoot, orbitID)
			if err != nil {
				return false, fmt.Errorf("inspect orbit %q HUMANS lane: %w", orbitID, err)
			}
			if status.MaterializeRequiresForce && !force {
				return false, fmt.Errorf(
					"compose orbit %q into root HUMANS.md: root HUMANS.md already contains drifted orbit block %q; rerun with --force to overwrite it",
					orbitID,
					orbitID,
				)
			}
			return false, nil
		},
		materialize: func(ctx context.Context, repoRoot string, orbitID string, force bool) (bool, error) {
			materializeResult, err := orbittemplate.MaterializeOrbitHumans(ctx, orbittemplate.HumansMaterializeInput{
				RepoRoot: repoRoot,
				OrbitID:  orbitID,
				Force:    force,
			})
			if err != nil {
				return false, fmt.Errorf("materialize orbit humans: %w", err)
			}
			return materializeResult.Changed, nil
		},
	}
}

func bootstrapGuidanceTarget() runtimeGuidanceTarget {
	return runtimeGuidanceTarget{
		target:       GuidanceTargetBootstrap,
		relativePath: rootBootstrapPath,
		preflight: func(ctx context.Context, repoRoot string, gitDir string, orbitID string, force bool) (bool, error) {
			bootstrapStatus, err := orbittemplate.InspectBootstrapOrbit(ctx, repoRoot, gitDir, orbitID)
			if err != nil {
				return false, fmt.Errorf("inspect bootstrap state for orbit %q: %w", orbitID, err)
			}
			if orbittemplate.PlanBootstrapCompose(bootstrapStatus).Action == orbittemplate.BootstrapActionSkip {
				return true, nil
			}
			status, err := orbittemplate.InspectOrbitBootstrapLane(ctx, repoRoot, orbitID)
			if err != nil {
				return false, fmt.Errorf("inspect orbit %q BOOTSTRAP lane: %w", orbitID, err)
			}
			if status.MaterializeRequiresForce && !force {
				return false, fmt.Errorf(
					"compose orbit %q into root BOOTSTRAP.md: root BOOTSTRAP.md already contains drifted orbit block %q; rerun with --force to overwrite it",
					orbitID,
					orbitID,
				)
			}
			return false, nil
		},
		materialize: func(ctx context.Context, repoRoot string, orbitID string, force bool) (bool, error) {
			materializeResult, err := orbittemplate.MaterializeOrbitBootstrap(ctx, orbittemplate.BootstrapMaterializeInput{
				RepoRoot: repoRoot,
				OrbitID:  orbitID,
				Force:    force,
			})
			if err != nil {
				return false, fmt.Errorf("materialize orbit bootstrap guidance: %w", err)
			}
			return materializeResult.Changed, nil
		},
	}
}
