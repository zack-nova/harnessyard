package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// BriefExportSyncResult reports one optional brief backfill performed as part of a higher-level export command.
type BriefExportSyncResult struct {
	Backfilled     bool
	DefinitionPath string
	Warning        string
}

// EnsureBriefExportSync checks whether the current orbit brief is drifted before save/publish.
// When allowBackfill is true, it backfills the drifted brief into hosted truth before continuing.
func EnsureBriefExportSync(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	action string,
	allowBackfill bool,
) (BriefExportSyncResult, error) {
	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return BriefExportSyncResult{}, nil
		}
		return BriefExportSyncResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	if !spec.HasMemberSchema() || spec.Meta == nil {
		return BriefExportSyncResult{}, nil
	}

	status, err := InspectOrbitBriefLaneForOperation(ctx, repoRoot, orbitID, "backfill")
	if err != nil {
		return BriefExportSyncResult{}, fmt.Errorf("inspect orbit brief: %w", err)
	}
	if status.State != BriefLaneStateMaterializedDrifted {
		return BriefExportSyncResult{}, nil
	}
	if !allowBackfill {
		return BriefExportSyncResult{}, fmt.Errorf(
			"current root AGENTS.md contains a drifted orbit block %q; run `orbit brief backfill --orbit %s` first or rerun with --backfill-brief before %s",
			orbitID,
			orbitID,
			action,
		)
	}

	result, err := BackfillOrbitBrief(ctx, BriefBackfillInput{
		RepoRoot: repoRoot,
		OrbitID:  orbitID,
	})
	if err != nil {
		return BriefExportSyncResult{}, fmt.Errorf("backfill orbit brief: %w", err)
	}

	return BriefExportSyncResult{
		Backfilled:     true,
		DefinitionPath: result.DefinitionPath,
		Warning:        formatBriefExportSyncWarning(result.Status, orbitID, result.DefinitionPath),
	}, nil
}

func formatBriefExportSyncWarning(status GuidanceBackfillStatus, orbitID string, definitionPath string) string {
	switch status {
	case GuidanceBackfillStatusRemoved:
		return fmt.Sprintf("auto-removed orbit brief %s from %s", orbitID, definitionPath)
	case GuidanceBackfillStatusSkipped:
		return fmt.Sprintf("orbit brief %s already matched hosted truth at %s", orbitID, definitionPath)
	default:
		return fmt.Sprintf("auto-backfilled orbit brief %s into %s", orbitID, definitionPath)
	}
}
