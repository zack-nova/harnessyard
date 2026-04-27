package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// GuidanceCommandOptions configures public wrappers around the legacy orbit
// guidance lane while preserving compatibility defaults for raw orbit commands.
type GuidanceCommandOptions struct {
	DefaultAllOrbitsWhenOrbitOmitted bool
}

type guidanceBackfillOutput struct {
	RepoRoot      string                     `json:"repo_root"`
	OrbitID       string                     `json:"orbit_id"`
	Target        string                     `json:"target"`
	ArtifactCount int                        `json:"artifact_count"`
	Artifacts     []guidanceBackfillArtifact `json:"artifacts"`
}

type guidanceBackfillAggregateOutput struct {
	RepoRoot      string                         `json:"repo_root"`
	Target        string                         `json:"target"`
	OrbitCount    int                            `json:"orbit_count"`
	ArtifactCount int                            `json:"artifact_count"`
	Orbits        []guidanceBackfillOrbitSummary `json:"orbits"`
}

type guidanceBackfillOrbitSummary struct {
	OrbitID       string                     `json:"orbit_id"`
	ArtifactCount int                        `json:"artifact_count"`
	Artifacts     []guidanceBackfillArtifact `json:"artifacts"`
}

type guidanceBackfillArtifact struct {
	Target         string                             `json:"target"`
	Status         string                             `json:"status"`
	DefinitionPath string                             `json:"definition_path"`
	UpdatedField   string                             `json:"updated_field"`
	Replacements   []orbittemplate.ReplacementSummary `json:"replacements,omitempty"`
}

type orbitGuidanceCheckOutput struct {
	RepoRoot      string                       `json:"repo_root"`
	OrbitID       string                       `json:"orbit_id"`
	Target        string                       `json:"target"`
	ArtifactCount int                          `json:"artifact_count"`
	Artifacts     []orbitGuidanceCheckArtifact `json:"artifacts"`
}

type orbitGuidanceAggregateCheckOutput struct {
	RepoRoot      string                          `json:"repo_root"`
	Target        string                          `json:"target"`
	OrbitCount    int                             `json:"orbit_count"`
	ArtifactCount int                             `json:"artifact_count"`
	Orbits        []orbitGuidanceCheckOrbitOutput `json:"orbits"`
}

type orbitGuidanceCheckOrbitOutput struct {
	OrbitID       string                       `json:"orbit_id"`
	ArtifactCount int                          `json:"artifact_count"`
	Artifacts     []orbitGuidanceCheckArtifact `json:"artifacts"`
}

type orbitGuidanceCheckArtifact struct {
	Target                   string `json:"target"`
	RevisionKind             string `json:"revision_kind"`
	State                    string `json:"state"`
	Reason                   string `json:"reason"`
	Path                     string `json:"path"`
	CompletionState          string `json:"completion_state,omitempty"`
	HasAuthoredTruth         bool   `json:"has_authored_truth"`
	HasRootArtifact          bool   `json:"has_root_artifact"`
	HasOrbitBlock            bool   `json:"has_orbit_block"`
	MaterializeAllowed       bool   `json:"materialize_allowed"`
	MaterializeRequiresForce bool   `json:"materialize_requires_force"`
	BackfillAllowed          bool   `json:"backfill_allowed"`
	SeedEmptyAllowed         bool   `json:"seed_empty_allowed"`
}

// NewGuidanceBackfillCommand creates the orbit guidance backfill command.
func NewGuidanceBackfillCommand() *cobra.Command {
	return NewGuidanceBackfillCommandWithOptions(GuidanceCommandOptions{})
}

// NewGuidanceBackfillCommandWithOptions creates the orbit guidance backfill
// command with optional public-wrapper behavior.
func NewGuidanceBackfillCommandWithOptions(options GuidanceCommandOptions) *cobra.Command {
	var requestedOrbitID string
	var target string
	var check bool

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill root AGENTS, HUMANS, or BOOTSTRAP blocks into hosted orbit guidance templates",
		Long: "Extract the current orbit block from a root guidance artifact, reverse-variableize it,\n" +
			"and write the result into the matching hosted meta.*_template field.\n" +
			"Supported revision kinds: runtime, source, orbit_template.",
		Example: "" +
			"  orbit guidance backfill --orbit docs --target all\n" +
			"  orbit guidance backfill --orbit docs --target humans --check\n" +
			"  orbit guidance backfill --orbit docs --target bootstrap --json\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := repoFromCommand(cmd)
			if err != nil {
				return err
			}

			resolvedTarget, targets, err := resolveOrbitGuidanceTargets(target)
			if err != nil {
				return err
			}

			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			if options.DefaultAllOrbitsWhenOrbitOmitted && strings.TrimSpace(requestedOrbitID) == "" {
				if check {
					orbits, err := inspectAllOrbitGuidanceTargets(cmd, repo.Root, resolvedTarget, targets, "backfill")
					if err != nil {
						return fmt.Errorf("inspect orbit guidance: %w", err)
					}
					return emitOrbitGuidanceAggregateCheck(cmd, repo.Root, resolvedTarget, orbits, jsonOutput)
				}

				orbits, err := backfillAllOrbitGuidanceTargets(cmd, repo.Root, resolvedTarget, targets)
				if err != nil {
					return err
				}
				return emitOrbitGuidanceAggregateBackfill(cmd, repo.Root, resolvedTarget, orbits, jsonOutput)
			}

			orbitID, err := resolveBriefOrbitID(cmd, repo, requestedOrbitID)
			if err != nil {
				return err
			}
			if check {
				statuses, err := inspectOrbitGuidanceTargets(cmd, repo.Root, orbitID, targets, "backfill")
				if err != nil {
					return fmt.Errorf("inspect orbit guidance: %w", err)
				}
				return emitOrbitGuidanceCheck(cmd, repo.Root, orbitID, resolvedTarget, statuses, jsonOutput)
			}

			artifacts, err := backfillOrbitGuidanceTargets(cmd, repo.Root, orbitID, resolvedTarget, targets)
			if err != nil {
				return err
			}
			return emitOrbitGuidanceBackfill(cmd, repo.Root, orbitID, resolvedTarget, artifacts, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&requestedOrbitID, "orbit", "", "Override the target orbit id instead of using the current orbit")
	if options.DefaultAllOrbitsWhenOrbitOmitted {
		cmd.Flags().Lookup("orbit").Usage = "Limit the operation to one orbit id; omitted processes all applicable orbits"
	}
	cmd.Flags().StringVar(&target, "target", string(orbittemplate.GuidanceTargetAll), "Target to backfill: agents, humans, bootstrap, or all")
	cmd.Flags().BoolVar(&check, "check", false, "Report the current guidance-lane state without modifying files")
	addJSONFlag(cmd)

	return cmd
}

func resolveOrbitGuidanceTargets(raw string) (orbittemplate.GuidanceTarget, []orbittemplate.GuidanceTarget, error) {
	resolved, err := orbittemplate.NormalizeGuidanceTarget(orbittemplate.GuidanceTarget(strings.TrimSpace(raw)))
	if err != nil {
		return "", nil, fmt.Errorf("normalize guidance target: %w", err)
	}
	targets, err := orbittemplate.ExpandGuidanceTargets(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("expand guidance targets: %w", err)
	}

	return resolved, targets, nil
}

func guidanceOrbitIDsForAll(cmd *cobra.Command, repoRoot string) ([]string, error) {
	config, err := loadValidatedVisibleOrbitConfig(cmd.Context(), repoRoot)
	if err != nil {
		return nil, err
	}

	orbitIDs := make([]string, 0, len(config.Orbits))
	for _, definition := range config.Orbits {
		orbitIDs = append(orbitIDs, definition.ID)
	}
	sort.Strings(orbitIDs)
	if len(orbitIDs) == 0 {
		return nil, errors.New("no orbit guidance definitions found")
	}

	return orbitIDs, nil
}

const (
	guidanceMaterializeStatusRendered               = "rendered"
	guidanceMaterializeStatusUnchanged              = "unchanged"
	guidanceMaterializeStatusSeededEmpty            = "seeded_empty"
	guidanceMaterializeStatusSkippedExistingBlock   = "skipped_existing_block"
	guidanceMaterializeStatusSkippedNoAuthoredTruth = "skipped_no_authored_truth"
	guidanceMaterializeStatusSkippedBootstrapClosed = "skipped_bootstrap_closed"

	guidanceReasonAuthoredTruth      = "authored_truth"
	guidanceReasonExistingBlock      = "existing_block"
	guidanceReasonNoAuthoredTruth    = "no_authored_truth"
	guidanceReasonBootstrapCompleted = "bootstrap_completed"
	guidanceReasonDriftRequiresForce = "drift_requires_force"
	guidanceReasonInvalidContainer   = "invalid_container"
)

func materializeOrbitGuidanceTargets(
	cmd *cobra.Command,
	repoRoot string,
	orbitID string,
	resolvedTarget orbittemplate.GuidanceTarget,
	targets []orbittemplate.GuidanceTarget,
	statuses []orbitGuidanceCheckArtifact,
	force bool,
	seedEmpty bool,
	strict bool,
) ([]guidanceMaterializeArtifact, error) {
	if err := preflightMaterializeOrbitGuidanceTargets(orbitID, resolvedTarget, targets, statuses, force, seedEmpty, strict); err != nil {
		return nil, err
	}

	artifacts := make([]guidanceMaterializeArtifact, 0, len(targets))
	for index, target := range targets {
		status := statuses[index]
		if isSeedEmptyExistingBlock(status, seedEmpty) {
			artifacts = append(artifacts, skippedSeedEmptyExistingBlockArtifact(status, force))
			continue
		}
		if !shouldRenderGuidanceTarget(status, force, seedEmpty) {
			artifacts = append(artifacts, skippedGuidanceMaterializeArtifact(status, force))
			continue
		}
		artifact, err := materializeOrbitGuidanceTarget(cmd, repoRoot, orbitID, target, force, seedEmpty)
		if err != nil {
			return nil, err
		}
		artifact.Status = renderedGuidanceMaterializeStatus(artifact, status, seedEmpty)
		artifact.Reason = status.Reason
		artifact.SeedEmptyAllowed = status.SeedEmptyAllowed
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

func preflightMaterializeOrbitGuidanceTargets(
	orbitID string,
	resolvedTarget orbittemplate.GuidanceTarget,
	targets []orbittemplate.GuidanceTarget,
	statuses []orbitGuidanceCheckArtifact,
	force bool,
	seedEmpty bool,
	strict bool,
) error {
	if len(statuses) != len(targets) {
		return fmt.Errorf("materialize orbit guidance: expected %d target statuses, got %d", len(targets), len(statuses))
	}

	strictMode := strict || resolvedTarget != orbittemplate.GuidanceTargetAll
	for _, status := range statuses {
		if isSeedEmptyExistingBlock(status, seedEmpty) {
			continue
		}
		if shouldRenderGuidanceTarget(status, force, seedEmpty) {
			continue
		}
		if !strictMode && isSkippableDefaultGuidanceTarget(status) {
			continue
		}
		return guidanceMaterializeBlockedError(orbitID, status)
	}

	return nil
}

func shouldRenderGuidanceTarget(status orbitGuidanceCheckArtifact, force bool, seedEmpty bool) bool {
	if isSeedEmptyExistingBlock(status, seedEmpty) {
		return false
	}
	if status.MaterializeAllowed {
		return true
	}
	if force && status.MaterializeRequiresForce {
		return true
	}
	return seedEmpty && status.SeedEmptyAllowed
}

func isSeedEmptyExistingBlock(status orbitGuidanceCheckArtifact, seedEmpty bool) bool {
	return seedEmpty && status.HasOrbitBlock
}

func isSkippableDefaultGuidanceTarget(status orbitGuidanceCheckArtifact) bool {
	switch status.Reason {
	case guidanceReasonNoAuthoredTruth, guidanceReasonBootstrapCompleted:
		return true
	default:
		return false
	}
}

func skippedSeedEmptyExistingBlockArtifact(status orbitGuidanceCheckArtifact, force bool) guidanceMaterializeArtifact {
	return guidanceMaterializeArtifact{
		Target:           status.Target,
		Status:           guidanceMaterializeStatusSkippedExistingBlock,
		Reason:           guidanceReasonExistingBlock,
		Path:             status.Path,
		Changed:          false,
		Forced:           force,
		SeedEmptyAllowed: status.SeedEmptyAllowed,
	}
}

func skippedGuidanceMaterializeArtifact(status orbitGuidanceCheckArtifact, force bool) guidanceMaterializeArtifact {
	artifactStatus := guidanceMaterializeStatusSkippedNoAuthoredTruth
	if status.Reason == guidanceReasonBootstrapCompleted {
		artifactStatus = guidanceMaterializeStatusSkippedBootstrapClosed
	}
	return guidanceMaterializeArtifact{
		Target:           status.Target,
		Status:           artifactStatus,
		Reason:           status.Reason,
		Path:             status.Path,
		Changed:          false,
		Forced:           force,
		SeedEmptyAllowed: status.SeedEmptyAllowed,
	}
}

func renderedGuidanceMaterializeStatus(artifact guidanceMaterializeArtifact, status orbitGuidanceCheckArtifact, seedEmpty bool) string {
	if seedEmpty && status.SeedEmptyAllowed {
		return guidanceMaterializeStatusSeededEmpty
	}
	if artifact.Changed {
		return guidanceMaterializeStatusRendered
	}
	return guidanceMaterializeStatusUnchanged
}

func guidanceMaterializeBlockedError(orbitID string, status orbitGuidanceCheckArtifact) error {
	switch status.Reason {
	case guidanceReasonNoAuthoredTruth:
		return fmt.Errorf(
			"orbit %q does not have authored %s guidance; rerun with --seed-empty to create an editable empty block",
			orbitID,
			guidanceTargetDescription(status.Target),
		)
	case guidanceReasonBootstrapCompleted:
		return fmt.Errorf("bootstrap guidance for orbit %q is closed because bootstrap is already completed in this runtime", orbitID)
	case guidanceReasonDriftRequiresForce:
		return fmt.Errorf(
			"%s already has local edits in orbit block %q; render will not append a duplicate block. Run `hyard guide save --orbit %s --target %s` to keep those edits, or rerun `hyard guide render --orbit %s --target %s --force` to discard and re-render that block",
			guidanceTargetPathLabel(status),
			orbitID,
			orbitID,
			status.Target,
			orbitID,
			status.Target,
		)
	case guidanceReasonInvalidContainer:
		return fmt.Errorf("%s is invalid; resolve the guidance container before rendering", guidanceTargetPathLabel(status))
	default:
		return fmt.Errorf("orbit %q guidance target %q cannot be rendered in state %q", orbitID, status.Target, status.State)
	}
}

func guidanceTargetDescription(target string) string {
	switch target {
	case string(orbittemplate.GuidanceTargetAgents):
		return "agent"
	case string(orbittemplate.GuidanceTargetHumans):
		return "human"
	case string(orbittemplate.GuidanceTargetBootstrap):
		return "bootstrap"
	default:
		return target
	}
}

func guidanceTargetPathLabel(status orbitGuidanceCheckArtifact) string {
	switch status.Target {
	case string(orbittemplate.GuidanceTargetAgents):
		return "root AGENTS.md"
	case string(orbittemplate.GuidanceTargetHumans):
		return "root HUMANS.md"
	case string(orbittemplate.GuidanceTargetBootstrap):
		return "root BOOTSTRAP.md"
	default:
		if status.Path != "" {
			return status.Path
		}
		return status.Target
	}
}

func materializeOrbitGuidanceTarget(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget, force bool, seedEmpty bool) (guidanceMaterializeArtifact, error) {
	switch target {
	case orbittemplate.GuidanceTargetAgents:
		result, err := orbittemplate.MaterializeOrbitBrief(cmd.Context(), orbittemplate.BriefMaterializeInput{
			RepoRoot:  repoRoot,
			OrbitID:   orbitID,
			Force:     force,
			SeedEmpty: seedEmpty,
		})
		if err != nil {
			return guidanceMaterializeArtifact{}, fmt.Errorf("materialize orbit guidance: %w", err)
		}
		return guidanceMaterializeArtifact{Target: string(target), Path: result.AgentsPath, Changed: result.Changed, Forced: result.Forced}, nil
	case orbittemplate.GuidanceTargetHumans:
		result, err := orbittemplate.MaterializeOrbitHumans(cmd.Context(), orbittemplate.HumansMaterializeInput{
			RepoRoot:  repoRoot,
			OrbitID:   orbitID,
			Force:     force,
			SeedEmpty: seedEmpty,
		})
		if err != nil {
			return guidanceMaterializeArtifact{}, fmt.Errorf("materialize orbit guidance: %w", err)
		}
		return guidanceMaterializeArtifact{Target: string(target), Path: result.HumansPath, Changed: result.Changed, Forced: result.Forced}, nil
	case orbittemplate.GuidanceTargetBootstrap:
		result, err := orbittemplate.MaterializeOrbitBootstrap(cmd.Context(), orbittemplate.BootstrapMaterializeInput{
			RepoRoot:  repoRoot,
			OrbitID:   orbitID,
			Force:     force,
			SeedEmpty: seedEmpty,
		})
		if err != nil {
			return guidanceMaterializeArtifact{}, fmt.Errorf("materialize orbit guidance: %w", err)
		}
		return guidanceMaterializeArtifact{Target: string(target), Path: result.BootstrapPath, Changed: result.Changed, Forced: result.Forced}, nil
	default:
		return guidanceMaterializeArtifact{}, fmt.Errorf("unsupported guidance target %q", target)
	}
}

func backfillOrbitGuidanceTargets(cmd *cobra.Command, repoRoot string, orbitID string, resolvedTarget orbittemplate.GuidanceTarget, targets []orbittemplate.GuidanceTarget) ([]guidanceBackfillArtifact, error) {
	artifacts := make([]guidanceBackfillArtifact, 0, len(targets))
	allowMissingBootstrapSkip := resolvedTarget == orbittemplate.GuidanceTargetAll
	for _, target := range targets {
		if resolvedTarget == orbittemplate.GuidanceTargetAll {
			status, err := inspectOrbitGuidanceTarget(cmd, repoRoot, orbitID, target, "backfill")
			if err != nil {
				return nil, fmt.Errorf("inspect orbit guidance: %w", err)
			}
			if !status.HasOrbitBlock {
				if status.HasAuthoredTruth && status.HasRootArtifact {
					return nil, guidanceBackfillBlockedError(orbitID, status)
				}
				artifact, err := skippedMissingBlockBackfillArtifact(repoRoot, orbitID, target)
				if err != nil {
					return nil, err
				}
				artifacts = append(artifacts, artifact)
				continue
			}
		}
		artifact, err := backfillOrbitGuidanceTarget(cmd, repoRoot, orbitID, target)
		if err != nil {
			if allowMissingBootstrapSkip && isMissingBootstrapRootArtifact(target, err) {
				artifact, artifactErr := skippedMissingBlockBackfillArtifact(repoRoot, orbitID, target)
				if artifactErr != nil {
					return nil, artifactErr
				}
				artifacts = append(artifacts, artifact)
				continue
			}
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

func skippedMissingBlockBackfillArtifact(repoRoot string, orbitID string, target orbittemplate.GuidanceTarget) (guidanceBackfillArtifact, error) {
	definitionPath, err := orbitpkg.HostedDefinitionPath(repoRoot, orbitID)
	if err != nil {
		return guidanceBackfillArtifact{}, fmt.Errorf("build hosted orbit spec path: %w", err)
	}

	return guidanceBackfillArtifact{
		Target:         string(target),
		Status:         string(orbittemplate.GuidanceBackfillStatusSkipped),
		DefinitionPath: definitionPath,
		UpdatedField:   guidanceUpdatedFieldForTarget(target),
	}, nil
}

func guidanceUpdatedFieldForTarget(target orbittemplate.GuidanceTarget) string {
	switch target {
	case orbittemplate.GuidanceTargetAgents:
		return "meta.agents_template"
	case orbittemplate.GuidanceTargetHumans:
		return "meta.humans_template"
	case orbittemplate.GuidanceTargetBootstrap:
		return "meta.bootstrap_template"
	default:
		return ""
	}
}

type aggregateBackfillRequest struct {
	OrbitID string
	Targets []orbittemplate.GuidanceTarget
}

func backfillAllOrbitGuidanceTargets(
	cmd *cobra.Command,
	repoRoot string,
	resolvedTarget orbittemplate.GuidanceTarget,
	targets []orbittemplate.GuidanceTarget,
) ([]guidanceBackfillOrbitSummary, error) {
	requests, err := discoverAggregateBackfillRequests(cmd, repoRoot, targets)
	if err != nil {
		return nil, err
	}

	for _, request := range requests {
		statuses, err := inspectOrbitGuidanceTargets(cmd, repoRoot, request.OrbitID, request.Targets, "backfill")
		if err != nil {
			return nil, fmt.Errorf("inspect orbit %q guidance: %w", request.OrbitID, err)
		}
		for _, status := range statuses {
			if status.BackfillAllowed {
				continue
			}
			return nil, guidanceBackfillBlockedError(request.OrbitID, status)
		}
	}

	orbits := make([]guidanceBackfillOrbitSummary, 0, len(requests))
	for _, request := range requests {
		artifacts, err := backfillOrbitGuidanceTargets(cmd, repoRoot, request.OrbitID, resolvedTarget, request.Targets)
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, guidanceBackfillOrbitSummary{
			OrbitID:       request.OrbitID,
			ArtifactCount: len(artifacts),
			Artifacts:     artifacts,
		})
	}

	return orbits, nil
}

func discoverAggregateBackfillRequests(
	cmd *cobra.Command,
	repoRoot string,
	targets []orbittemplate.GuidanceTarget,
) ([]aggregateBackfillRequest, error) {
	orbitIDs, err := guidanceOrbitIDsForAll(cmd, repoRoot)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		allowed[orbitID] = struct{}{}
	}

	targetsByOrbit := make(map[string]map[orbittemplate.GuidanceTarget]struct{}, len(orbitIDs))
	for _, target := range targets {
		path, label, err := guidanceRootArtifactPath(repoRoot, target)
		if err != nil {
			return nil, err
		}
		//nolint:gosec // path is one fixed repo-root guidance artifact selected by target.
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", label, err)
		}
		document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", label, err)
		}
		for _, segment := range document.Segments {
			if segment.Kind != orbittemplate.AgentsRuntimeSegmentBlock {
				continue
			}
			if _, ok := allowed[segment.OrbitID]; !ok {
				return nil, fmt.Errorf("%s contains orbit block %q, but that orbit is not in the current guidance scope", label, segment.OrbitID)
			}
			if targetsByOrbit[segment.OrbitID] == nil {
				targetsByOrbit[segment.OrbitID] = make(map[orbittemplate.GuidanceTarget]struct{})
			}
			targetsByOrbit[segment.OrbitID][target] = struct{}{}
		}
	}

	requests := make([]aggregateBackfillRequest, 0, len(targetsByOrbit))
	for _, orbitID := range orbitIDs {
		targetSet := targetsByOrbit[orbitID]
		if len(targetSet) == 0 {
			continue
		}
		requestTargets := make([]orbittemplate.GuidanceTarget, 0, len(targetSet))
		for _, target := range targets {
			if _, ok := targetSet[target]; ok {
				requestTargets = append(requestTargets, target)
			}
		}
		requests = append(requests, aggregateBackfillRequest{OrbitID: orbitID, Targets: requestTargets})
	}

	return requests, nil
}

func guidanceRootArtifactPath(repoRoot string, target orbittemplate.GuidanceTarget) (string, string, error) {
	switch target {
	case orbittemplate.GuidanceTargetAgents:
		return filepath.Join(repoRoot, "AGENTS.md"), "root AGENTS.md", nil
	case orbittemplate.GuidanceTargetHumans:
		return filepath.Join(repoRoot, "HUMANS.md"), "root HUMANS.md", nil
	case orbittemplate.GuidanceTargetBootstrap:
		return filepath.Join(repoRoot, "BOOTSTRAP.md"), "root BOOTSTRAP.md", nil
	default:
		return "", "", fmt.Errorf("unsupported guidance target %q", target)
	}
}

func guidanceBackfillBlockedError(orbitID string, status orbitGuidanceCheckArtifact) error {
	if status.Target == string(orbittemplate.GuidanceTargetBootstrap) &&
		status.CompletionState == string(orbittemplate.BootstrapCompletionStateCompleted) {
		return fmt.Errorf("bootstrap guidance for orbit %q is closed because bootstrap is already completed in this runtime", orbitID)
	}
	if !status.HasRootArtifact {
		return fmt.Errorf("%s is missing", guidanceTargetPathLabel(status))
	}
	if !status.HasOrbitBlock {
		return fmt.Errorf("%s does not contain orbit block %q", guidanceTargetPathLabel(status), orbitID)
	}
	return fmt.Errorf("orbit %q guidance target %q cannot be written back in state %q", orbitID, status.Target, status.State)
}

func backfillOrbitGuidanceTarget(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget) (guidanceBackfillArtifact, error) {
	switch target {
	case orbittemplate.GuidanceTargetAgents:
		result, err := orbittemplate.BackfillOrbitBrief(cmd.Context(), orbittemplate.BriefBackfillInput{RepoRoot: repoRoot, OrbitID: orbitID})
		if err != nil {
			return guidanceBackfillArtifact{}, fmt.Errorf("backfill orbit guidance: %w", err)
		}
		return guidanceBackfillArtifact{Target: string(target), Status: string(result.Status), DefinitionPath: result.DefinitionPath, UpdatedField: "meta.agents_template", Replacements: result.Replacements}, nil
	case orbittemplate.GuidanceTargetHumans:
		result, err := orbittemplate.BackfillOrbitHumans(cmd.Context(), orbittemplate.HumansBackfillInput{RepoRoot: repoRoot, OrbitID: orbitID})
		if err != nil {
			return guidanceBackfillArtifact{}, fmt.Errorf("backfill orbit guidance: %w", err)
		}
		return guidanceBackfillArtifact{Target: string(target), Status: string(result.Status), DefinitionPath: result.DefinitionPath, UpdatedField: "meta.humans_template", Replacements: result.Replacements}, nil
	case orbittemplate.GuidanceTargetBootstrap:
		result, err := orbittemplate.BackfillOrbitBootstrap(cmd.Context(), orbittemplate.BootstrapBackfillInput{RepoRoot: repoRoot, OrbitID: orbitID})
		if err != nil {
			return guidanceBackfillArtifact{}, fmt.Errorf("backfill orbit guidance: %w", err)
		}
		return guidanceBackfillArtifact{Target: string(target), Status: string(result.Status), DefinitionPath: result.DefinitionPath, UpdatedField: "meta.bootstrap_template", Replacements: result.Replacements}, nil
	default:
		return guidanceBackfillArtifact{}, fmt.Errorf("unsupported guidance target %q", target)
	}
}

func isMissingBootstrapRootArtifact(target orbittemplate.GuidanceTarget, err error) bool {
	return target == orbittemplate.GuidanceTargetBootstrap && errors.Is(err, orbittemplate.ErrGuidanceRootArtifactMissing)
}

func inspectAllOrbitGuidanceTargets(
	cmd *cobra.Command,
	repoRoot string,
	_ orbittemplate.GuidanceTarget,
	targets []orbittemplate.GuidanceTarget,
	operation string,
) ([]orbitGuidanceCheckOrbitOutput, error) {
	if operation == "backfill" {
		requests, err := discoverAggregateBackfillRequests(cmd, repoRoot, targets)
		if err != nil {
			return nil, err
		}
		orbits := make([]orbitGuidanceCheckOrbitOutput, 0, len(requests))
		for _, request := range requests {
			statuses, err := inspectOrbitGuidanceTargets(cmd, repoRoot, request.OrbitID, request.Targets, operation)
			if err != nil {
				return nil, err
			}
			orbits = append(orbits, orbitGuidanceCheckOrbitOutput{
				OrbitID:       request.OrbitID,
				ArtifactCount: len(statuses),
				Artifacts:     statuses,
			})
		}
		return orbits, nil
	}

	orbitIDs, err := guidanceOrbitIDsForAll(cmd, repoRoot)
	if err != nil {
		return nil, err
	}
	orbits := make([]orbitGuidanceCheckOrbitOutput, 0, len(orbitIDs))
	for _, orbitID := range orbitIDs {
		statuses, err := inspectOrbitGuidanceTargets(cmd, repoRoot, orbitID, targets, operation)
		if err != nil {
			return nil, err
		}
		orbits = append(orbits, orbitGuidanceCheckOrbitOutput{
			OrbitID:       orbitID,
			ArtifactCount: len(statuses),
			Artifacts:     statuses,
		})
	}
	return orbits, nil
}

func inspectOrbitGuidanceTargets(cmd *cobra.Command, repoRoot string, orbitID string, targets []orbittemplate.GuidanceTarget, operation string) ([]orbitGuidanceCheckArtifact, error) {
	statuses := make([]orbitGuidanceCheckArtifact, 0, len(targets))
	for _, target := range targets {
		status, err := inspectOrbitGuidanceTarget(cmd, repoRoot, orbitID, target, operation)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

func inspectOrbitGuidanceTarget(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget, operation string) (orbitGuidanceCheckArtifact, error) {
	switch target {
	case orbittemplate.GuidanceTargetAgents:
		status, err := orbittemplate.InspectOrbitBriefLaneForOperation(cmd.Context(), repoRoot, orbitID, operation)
		if err != nil {
			return orbitGuidanceCheckArtifact{}, fmt.Errorf("inspect orbit brief lane: %w", err)
		}
		return completeGuidanceCheckArtifactForTarget(cmd, repoRoot, orbitID, target, orbitGuidanceCheckArtifact{
			Target:                   string(target),
			RevisionKind:             status.RevisionKind,
			State:                    string(status.State),
			Path:                     status.AgentsPath,
			HasAuthoredTruth:         status.HasAuthoredTruth,
			HasRootArtifact:          status.HasRootAgents,
			HasOrbitBlock:            status.HasOrbitBlock,
			MaterializeAllowed:       status.MaterializeAllowed,
			MaterializeRequiresForce: status.MaterializeRequiresForce,
			BackfillAllowed:          status.BackfillAllowed,
		})
	case orbittemplate.GuidanceTargetHumans:
		status, err := orbittemplate.InspectOrbitHumansLaneForOperation(cmd.Context(), repoRoot, orbitID, operation)
		if err != nil {
			return orbitGuidanceCheckArtifact{}, fmt.Errorf("inspect orbit humans lane: %w", err)
		}
		return completeGuidanceCheckArtifactForTarget(cmd, repoRoot, orbitID, target, orbitGuidanceCheckArtifact{
			Target:                   string(target),
			RevisionKind:             status.RevisionKind,
			State:                    string(status.State),
			Path:                     status.HumansPath,
			HasAuthoredTruth:         status.HasAuthoredTruth,
			HasRootArtifact:          status.HasRootHumans,
			HasOrbitBlock:            status.HasOrbitBlock,
			MaterializeAllowed:       status.MaterializeAllowed,
			MaterializeRequiresForce: status.MaterializeRequiresForce,
			BackfillAllowed:          status.BackfillAllowed,
		})
	case orbittemplate.GuidanceTargetBootstrap:
		status, err := orbittemplate.InspectOrbitBootstrapLaneForOperation(cmd.Context(), repoRoot, orbitID, operation)
		if err != nil {
			return orbitGuidanceCheckArtifact{}, fmt.Errorf("inspect orbit bootstrap lane: %w", err)
		}
		return completeGuidanceCheckArtifactForTarget(cmd, repoRoot, orbitID, target, orbitGuidanceCheckArtifact{
			Target:                   string(target),
			RevisionKind:             status.RevisionKind,
			State:                    string(status.State),
			CompletionState:          string(status.CompletionState),
			Path:                     status.BootstrapPath,
			HasAuthoredTruth:         status.HasAuthoredTruth,
			HasRootArtifact:          status.HasRootBootstrap,
			HasOrbitBlock:            status.HasOrbitBlock,
			MaterializeAllowed:       status.MaterializeAllowed,
			MaterializeRequiresForce: status.MaterializeRequiresForce,
			BackfillAllowed:          status.BackfillAllowed,
		})
	default:
		return orbitGuidanceCheckArtifact{}, fmt.Errorf("unsupported guidance target %q", target)
	}
}

func completeGuidanceCheckArtifactForTarget(
	cmd *cobra.Command,
	repoRoot string,
	orbitID string,
	target orbittemplate.GuidanceTarget,
	artifact orbitGuidanceCheckArtifact,
) (orbitGuidanceCheckArtifact, error) {
	completed := completeGuidanceCheckArtifact(artifact)
	spec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), repoRoot, orbitID)
	if err != nil {
		return orbitGuidanceCheckArtifact{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	hasExplicitTemplate, err := orbittemplate.HasExplicitGuidanceTemplate(spec, target)
	if err != nil {
		return orbitGuidanceCheckArtifact{}, fmt.Errorf("inspect explicit guidance template for target %q: %w", target, err)
	}
	completed.SeedEmptyAllowed = guidanceSeedEmptyAllowed(completed, hasExplicitTemplate)
	return completed, nil
}

func completeGuidanceCheckArtifact(artifact orbitGuidanceCheckArtifact) orbitGuidanceCheckArtifact {
	artifact.Reason = guidanceCheckReason(artifact)
	return artifact
}

func guidanceCheckReason(artifact orbitGuidanceCheckArtifact) string {
	if artifact.Target == string(orbittemplate.GuidanceTargetBootstrap) &&
		artifact.CompletionState == string(orbittemplate.BootstrapCompletionStateCompleted) {
		return guidanceReasonBootstrapCompleted
	}
	if artifact.MaterializeRequiresForce {
		return guidanceReasonDriftRequiresForce
	}
	switch artifact.State {
	case string(orbittemplate.BriefLaneStateInvalidContainer):
		return guidanceReasonInvalidContainer
	}
	if !artifact.HasAuthoredTruth {
		return guidanceReasonNoAuthoredTruth
	}
	return guidanceReasonAuthoredTruth
}

func guidanceSeedEmptyAllowed(artifact orbitGuidanceCheckArtifact, hasExplicitTemplate bool) bool {
	if hasExplicitTemplate || artifact.HasOrbitBlock {
		return false
	}
	switch artifact.Reason {
	case guidanceReasonInvalidContainer, guidanceReasonBootstrapCompleted:
		return false
	default:
		return true
	}
}

func emitOrbitGuidanceMaterialize(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget, artifacts []guidanceMaterializeArtifact, jsonOutput bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), guidanceMaterializeOutput{
			RepoRoot:      repoRoot,
			OrbitID:       orbitID,
			Target:        string(target),
			ArtifactCount: len(artifacts),
			Artifacts:     artifacts,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "processed orbit guidance %s for target %s\n", orbitID, target); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact_count: %d\n", len(artifacts)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, artifact := range artifacts {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"artifact: %s status=%s reason=%s path=%s changed=%t\n",
			artifact.Target,
			artifact.Status,
			artifact.Reason,
			artifact.Path,
			artifact.Changed,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitOrbitGuidanceAggregateMaterialize(cmd *cobra.Command, repoRoot string, target orbittemplate.GuidanceTarget, orbits []guidanceMaterializeOrbitSummary, jsonOutput bool) error {
	artifactCount := countGuidanceMaterializeArtifacts(orbits)
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), guidanceMaterializeAggregateOutput{
			RepoRoot:      repoRoot,
			Target:        string(target),
			OrbitCount:    len(orbits),
			ArtifactCount: artifactCount,
			Orbits:        orbits,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "processed orbit guidance for %d orbits target %s\n", len(orbits), target); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact_count: %d\n", artifactCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, orbit := range orbits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s artifact_count=%d\n", orbit.OrbitID, orbit.ArtifactCount); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, artifact := range orbit.Artifacts {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"artifact: %s target=%s status=%s reason=%s path=%s changed=%t\n",
				orbit.OrbitID,
				artifact.Target,
				artifact.Status,
				artifact.Reason,
				artifact.Path,
				artifact.Changed,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}

func emitOrbitGuidanceBackfill(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget, artifacts []guidanceBackfillArtifact, jsonOutput bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), guidanceBackfillOutput{
			RepoRoot:      repoRoot,
			OrbitID:       orbitID,
			Target:        string(target),
			ArtifactCount: len(artifacts),
			Artifacts:     artifacts,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "backfilled orbit guidance %s for target %s\n", orbitID, target); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact_count: %d\n", len(artifacts)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, artifact := range artifacts {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact: %s status=%s updated_field=%s path=%s\n", artifact.Target, artifact.Status, artifact.UpdatedField, artifact.DefinitionPath); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitOrbitGuidanceAggregateBackfill(cmd *cobra.Command, repoRoot string, target orbittemplate.GuidanceTarget, orbits []guidanceBackfillOrbitSummary, jsonOutput bool) error {
	artifactCount := countGuidanceBackfillArtifacts(orbits)
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), guidanceBackfillAggregateOutput{
			RepoRoot:      repoRoot,
			Target:        string(target),
			OrbitCount:    len(orbits),
			ArtifactCount: artifactCount,
			Orbits:        orbits,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "processed orbit guidance writeback for %d orbits target %s\n", len(orbits), target); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "artifact_count: %d\n", artifactCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, orbit := range orbits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit: %s artifact_count=%d\n", orbit.OrbitID, orbit.ArtifactCount); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, artifact := range orbit.Artifacts {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"artifact: %s target=%s status=%s updated_field=%s path=%s\n",
				orbit.OrbitID,
				artifact.Target,
				artifact.Status,
				artifact.UpdatedField,
				artifact.DefinitionPath,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}

func emitOrbitGuidanceCheck(cmd *cobra.Command, repoRoot string, orbitID string, target orbittemplate.GuidanceTarget, artifacts []orbitGuidanceCheckArtifact, jsonOutput bool) error {
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), orbitGuidanceCheckOutput{
			RepoRoot:      repoRoot,
			OrbitID:       orbitID,
			Target:        string(target),
			ArtifactCount: len(artifacts),
			Artifacts:     artifacts,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_id: %s\ntarget: %s\nartifact_count: %d\n", orbitID, target, len(artifacts)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, artifact := range artifacts {
		if _, err := fmt.Fprintf(
			cmd.OutOrStdout(),
			"artifact: %s path=%s revision_kind=%s state=%s reason=%s seed_empty.allowed=%t has_authored_truth=%t has_root_artifact=%t has_orbit_block=%t materialize.allowed=%t materialize.requires_force=%t backfill.allowed=%t\n",
			artifact.Target,
			artifact.Path,
			artifact.RevisionKind,
			artifact.State,
			artifact.Reason,
			artifact.SeedEmptyAllowed,
			artifact.HasAuthoredTruth,
			artifact.HasRootArtifact,
			artifact.HasOrbitBlock,
			artifact.MaterializeAllowed,
			artifact.MaterializeRequiresForce,
			artifact.BackfillAllowed,
		); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}

func emitOrbitGuidanceAggregateCheck(cmd *cobra.Command, repoRoot string, target orbittemplate.GuidanceTarget, orbits []orbitGuidanceCheckOrbitOutput, jsonOutput bool) error {
	artifactCount := countGuidanceCheckArtifacts(orbits)
	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), orbitGuidanceAggregateCheckOutput{
			RepoRoot:      repoRoot,
			Target:        string(target),
			OrbitCount:    len(orbits),
			ArtifactCount: artifactCount,
			Orbits:        orbits,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "target: %s\norbit_count: %d\nartifact_count: %d\n", target, len(orbits), artifactCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, orbit := range orbits {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_id: %s artifact_count=%d\n", orbit.OrbitID, orbit.ArtifactCount); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		for _, artifact := range orbit.Artifacts {
			if _, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"artifact: %s target=%s path=%s revision_kind=%s state=%s reason=%s seed_empty.allowed=%t has_authored_truth=%t has_root_artifact=%t has_orbit_block=%t materialize.allowed=%t materialize.requires_force=%t backfill.allowed=%t\n",
				orbit.OrbitID,
				artifact.Target,
				artifact.Path,
				artifact.RevisionKind,
				artifact.State,
				artifact.Reason,
				artifact.SeedEmptyAllowed,
				artifact.HasAuthoredTruth,
				artifact.HasRootArtifact,
				artifact.HasOrbitBlock,
				artifact.MaterializeAllowed,
				artifact.MaterializeRequiresForce,
				artifact.BackfillAllowed,
			); err != nil {
				return fmt.Errorf("write command output: %w", err)
			}
		}
	}

	return nil
}

func countGuidanceMaterializeArtifacts(orbits []guidanceMaterializeOrbitSummary) int {
	total := 0
	for _, orbit := range orbits {
		total += len(orbit.Artifacts)
	}
	return total
}

func countGuidanceBackfillArtifacts(orbits []guidanceBackfillOrbitSummary) int {
	total := 0
	for _, orbit := range orbits {
		total += len(orbit.Artifacts)
	}
	return total
}

func countGuidanceCheckArtifacts(orbits []orbitGuidanceCheckOrbitOutput) int {
	total := 0
	for _, orbit := range orbits {
		total += len(orbit.Artifacts)
	}
	return total
}
