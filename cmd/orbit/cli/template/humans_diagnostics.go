package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// HumansLaneStatus captures one repo-local diagnostic view of the current orbit human-guidance lane.
type HumansLaneStatus struct {
	RepoRoot                 string
	OrbitID                  string
	RevisionKind             string
	State                    BriefLaneState
	HumansPath               string
	HasAuthoredTruth         bool
	HasRootHumans            bool
	HasOrbitBlock            bool
	MaterializeAllowed       bool
	MaterializeRequiresForce bool
	BackfillAllowed          bool
}

// InspectOrbitHumansLane reports the current human-guidance lane state for one orbit.
func InspectOrbitHumansLane(ctx context.Context, repoRoot string, orbitID string) (HumansLaneStatus, error) {
	return InspectOrbitHumansLaneForOperation(ctx, repoRoot, orbitID, "materialize")
}

// InspectOrbitHumansLaneForOperation reports the current human-guidance lane state for one orbit.
func InspectOrbitHumansLaneForOperation(ctx context.Context, repoRoot string, orbitID string, operation string) (HumansLaneStatus, error) {
	if repoRoot == "" {
		return HumansLaneStatus{}, fmt.Errorf("repo root must not be empty")
	}

	revisionKind, err := resolveBriefRevisionKind(repoRoot)
	if err != nil {
		return HumansLaneStatus{}, fmt.Errorf("load current revision manifest: %w", err)
	}
	if err := validateBriefRevisionKindAllowed(revisionKind, operation); err != nil {
		return HumansLaneStatus{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		return HumansLaneStatus{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitHumansPayload(spec, repoRoot, revisionKind)
	if err != nil {
		return HumansLaneStatus{}, err
	}

	status := HumansLaneStatus{
		RepoRoot:         repoRoot,
		OrbitID:          orbitID,
		RevisionKind:     revisionKind,
		HumansPath:       filepath.Join(repoRoot, filepath.FromSlash(runtimeHumansRepoPath)),
		HasAuthoredTruth: hasTruth,
	}

	data, err := os.ReadFile(status.HumansPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.State = missingTruthOrStructuredOnly(hasTruth)
			applyHumansLanePermissions(&status)
			return status, nil
		}
		return HumansLaneStatus{}, fmt.Errorf("read root HUMANS.md: %w", err)
	}
	status.HasRootHumans = true

	document, parseErr := ParseRuntimeAgentsDocument(data)
	if parseErr == nil {
		block, extractErr := extractRuntimeGuidanceBlock(document, orbitID, "root HUMANS.md")
		if extractErr == nil {
			status.HasOrbitBlock = true

			if !hasTruth {
				status.State = BriefLaneStateMissingTruth
				applyHumansLanePermissions(&status)
				return status, nil
			}

			if bytes.Equal(block, payload) {
				status.State = BriefLaneStateMaterializedInSync
			} else {
				status.State = BriefLaneStateMaterializedDrifted
			}
			applyHumansLanePermissions(&status)

			return status, nil
		}

		status.State = missingTruthOrStructuredOnly(hasTruth)
		applyHumansLanePermissions(&status)
		return status, nil
	}

	status.State = BriefLaneStateInvalidContainer
	applyHumansLanePermissions(&status)
	return status, nil
}

func applyHumansLanePermissions(status *HumansLaneStatus) {
	switch status.State {
	case BriefLaneStateStructuredOnly, BriefLaneStateMaterializedInSync:
		status.MaterializeAllowed = true
		if status.State == BriefLaneStateMaterializedInSync {
			status.BackfillAllowed = true
		}
	case BriefLaneStateMaterializedDrifted:
		status.MaterializeRequiresForce = true
		status.BackfillAllowed = true
	case BriefLaneStateMissingTruth:
		status.BackfillAllowed = status.HasOrbitBlock
	}
}
