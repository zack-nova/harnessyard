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

// BriefLaneState is one stable brief-lane diagnostic state.
type BriefLaneState string

const (
	BriefLaneStateStructuredOnly      BriefLaneState = "structured_only"
	BriefLaneStateMaterializedInSync  BriefLaneState = "materialized_in_sync"
	BriefLaneStateMaterializedDrifted BriefLaneState = "materialized_drifted"
	BriefLaneStateInvalidContainer    BriefLaneState = "invalid_container"
	BriefLaneStateMissingTruth        BriefLaneState = "missing_truth"
)

// BriefLaneStatus captures one repo-local diagnostic view of the current orbit brief lane.
type BriefLaneStatus struct {
	RepoRoot                 string
	OrbitID                  string
	RevisionKind             string
	State                    BriefLaneState
	AgentsPath               string
	HasAuthoredTruth         bool
	HasRootAgents            bool
	HasOrbitBlock            bool
	MaterializeAllowed       bool
	MaterializeRequiresForce bool
	BackfillAllowed          bool
}

// InspectOrbitBriefLane reports the current brief-lane state for one orbit using materialize-side revision semantics.
func InspectOrbitBriefLane(ctx context.Context, repoRoot string, orbitID string) (BriefLaneStatus, error) {
	return InspectOrbitBriefLaneForOperation(ctx, repoRoot, orbitID, "materialize")
}

// InspectOrbitBriefLaneForOperation reports the current brief-lane state for one orbit using one command's revision semantics.
func InspectOrbitBriefLaneForOperation(ctx context.Context, repoRoot string, orbitID string, operation string) (BriefLaneStatus, error) {
	if repoRoot == "" {
		return BriefLaneStatus{}, fmt.Errorf("repo root must not be empty")
	}

	revisionKind, err := resolveBriefRevisionKind(repoRoot)
	if err != nil {
		return BriefLaneStatus{}, fmt.Errorf("load current revision manifest: %w", err)
	}
	if err := validateBriefRevisionKindAllowed(revisionKind, operation); err != nil {
		return BriefLaneStatus{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		return BriefLaneStatus{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitBriefPayload(spec, repoRoot, revisionKind)
	if err != nil {
		return BriefLaneStatus{}, err
	}

	status := BriefLaneStatus{
		RepoRoot:         repoRoot,
		OrbitID:          orbitID,
		RevisionKind:     revisionKind,
		AgentsPath:       filepath.Join(repoRoot, filepath.FromSlash(runtimeAgentsRepoPath)),
		HasAuthoredTruth: hasTruth,
	}

	data, err := os.ReadFile(status.AgentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.State = missingTruthOrStructuredOnly(hasTruth)
			applyBriefLanePermissions(&status)
			return status, nil
		}
		return BriefLaneStatus{}, fmt.Errorf("read root AGENTS.md: %w", err)
	}
	status.HasRootAgents = true

	document, parseErr := ParseRuntimeAgentsDocument(data)
	if parseErr == nil {
		block, extractErr := extractRuntimeAgentsBlock(document, orbitID)
		if extractErr == nil {
			status.HasOrbitBlock = true

			if !hasTruth {
				status.State = BriefLaneStateMissingTruth
				applyBriefLanePermissions(&status)
				return status, nil
			}

			if bytes.Equal(block, payload) {
				status.State = BriefLaneStateMaterializedInSync
			} else {
				status.State = BriefLaneStateMaterializedDrifted
			}
			applyBriefLanePermissions(&status)

			return status, nil
		}
		if hasTruth && runtimeAgentsDocumentContainsRunViewPayload(document, data, payload) {
			status.State = BriefLaneStateMaterializedInSync
			applyBriefLanePermissions(&status)
			return status, nil
		}

		status.State = missingTruthOrStructuredOnly(hasTruth)
		applyBriefLanePermissions(&status)
		return status, nil
	}

	status.State = BriefLaneStateInvalidContainer
	applyBriefLanePermissions(&status)
	return status, nil
}

func missingTruthOrStructuredOnly(hasTruth bool) BriefLaneState {
	if hasTruth {
		return BriefLaneStateStructuredOnly
	}
	return BriefLaneStateMissingTruth
}

func applyBriefLanePermissions(status *BriefLaneStatus) {
	switch status.State {
	case BriefLaneStateStructuredOnly:
		status.MaterializeAllowed = true
	case BriefLaneStateMaterializedInSync:
		status.MaterializeAllowed = true
		status.BackfillAllowed = true
	case BriefLaneStateMaterializedDrifted:
		status.MaterializeRequiresForce = true
		status.BackfillAllowed = true
	case BriefLaneStateInvalidContainer:
		return
	case BriefLaneStateMissingTruth:
		status.BackfillAllowed = status.HasOrbitBlock
	}
}
