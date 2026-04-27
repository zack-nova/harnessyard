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

// BootstrapLaneStatus captures one repo-local diagnostic view of the current orbit bootstrap-guidance lane.
type BootstrapLaneStatus struct {
	RepoRoot                 string
	OrbitID                  string
	RevisionKind             string
	State                    BriefLaneState
	CompletionState          BootstrapCompletionState
	BootstrapPath            string
	HasAuthoredTruth         bool
	HasRootBootstrap         bool
	HasOrbitBlock            bool
	MaterializeAllowed       bool
	MaterializeRequiresForce bool
	BackfillAllowed          bool
}

// InspectOrbitBootstrapLane reports the current bootstrap-guidance lane state for one orbit.
func InspectOrbitBootstrapLane(ctx context.Context, repoRoot string, orbitID string) (BootstrapLaneStatus, error) {
	return InspectOrbitBootstrapLaneForOperation(ctx, repoRoot, orbitID, "materialize")
}

// InspectOrbitBootstrapLaneForOperation reports the current bootstrap-guidance lane state for one orbit.
func InspectOrbitBootstrapLaneForOperation(ctx context.Context, repoRoot string, orbitID string, operation string) (BootstrapLaneStatus, error) {
	if repoRoot == "" {
		return BootstrapLaneStatus{}, fmt.Errorf("repo root must not be empty")
	}

	revisionKind, err := resolveBriefRevisionKind(repoRoot)
	if err != nil {
		return BootstrapLaneStatus{}, fmt.Errorf("load current revision manifest: %w", err)
	}
	if err := validateBriefRevisionKindAllowed(revisionKind, operation); err != nil {
		return BootstrapLaneStatus{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		return BootstrapLaneStatus{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}

	payload, hasTruth, err := materializedOrbitBootstrapPayload(spec, repoRoot, revisionKind)
	if err != nil {
		return BootstrapLaneStatus{}, err
	}

	status := BootstrapLaneStatus{
		RepoRoot:         repoRoot,
		OrbitID:          orbitID,
		RevisionKind:     revisionKind,
		BootstrapPath:    filepath.Join(repoRoot, filepath.FromSlash(runtimeBootstrapRepoPath)),
		HasAuthoredTruth: hasTruth,
	}
	bootstrapStatus, err := InspectBootstrapOrbitForRevision(ctx, repoRoot, "", orbitID, revisionKind)
	if err != nil {
		return BootstrapLaneStatus{}, err
	}
	status.CompletionState = bootstrapStatus.CompletionState

	data, err := os.ReadFile(status.BootstrapPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			status.State = missingTruthOrStructuredOnly(hasTruth)
			applyBootstrapLanePermissions(&status, bootstrapStatus)
			return status, nil
		}
		return BootstrapLaneStatus{}, fmt.Errorf("read root BOOTSTRAP.md: %w", err)
	}
	status.HasRootBootstrap = true

	document, parseErr := ParseRuntimeAgentsDocument(data)
	if parseErr == nil {
		block, extractErr := extractRuntimeGuidanceBlock(document, orbitID, "root BOOTSTRAP.md")
		if extractErr == nil {
			status.HasOrbitBlock = true

			if !hasTruth {
				status.State = BriefLaneStateMissingTruth
				applyBootstrapLanePermissions(&status, bootstrapStatus)
				return status, nil
			}

			if bytes.Equal(block, payload) {
				status.State = BriefLaneStateMaterializedInSync
			} else {
				status.State = BriefLaneStateMaterializedDrifted
			}
			applyBootstrapLanePermissions(&status, bootstrapStatus)
			return status, nil
		}

		status.State = missingTruthOrStructuredOnly(hasTruth)
		applyBootstrapLanePermissions(&status, bootstrapStatus)
		return status, nil
	}

	status.State = BriefLaneStateInvalidContainer
	applyBootstrapLanePermissions(&status, bootstrapStatus)
	return status, nil
}

func applyBootstrapLanePermissions(status *BootstrapLaneStatus, bootstrapStatus BootstrapOrbitStatus) {
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

	if PlanBootstrapGuidanceMaterialize(bootstrapStatus).Action != BootstrapActionAllow {
		status.MaterializeAllowed = false
		status.MaterializeRequiresForce = false
	}
	if PlanBootstrapGuidanceBackfill(bootstrapStatus, status.HasOrbitBlock).Action != BootstrapActionAllow {
		status.BackfillAllowed = false
	}
}
