package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// EnsureMemberHintExportSync checks whether worktree member hints are drifted before save/publish.
// The first version stays fail-closed and requires an explicit `orbit member backfill` lane.
func EnsureMemberHintExportSync(ctx context.Context, repoRoot string, orbitID string, action string) error {
	revisionKind, err := loadCurrentRevisionManifestKind(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load %s: %w", branchManifestPath, err)
	}

	switch revisionKind {
	case "runtime", "source", "orbit_template":
	default:
		return nil
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load hosted orbit spec: %w", err)
	}

	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("list worktree files: %w", err)
	}

	inspection, err := orbitpkg.InspectMemberHints(repoRoot, spec, worktreeFiles)
	if err != nil {
		return fmt.Errorf("inspect member hints: %w", err)
	}
	if !inspection.DriftDetected {
		return nil
	}

	backfillCommand := fmt.Sprintf("orbit member backfill --orbit %s", orbitID)
	if inspection.BackfillAllowed {
		return fmt.Errorf(
			"current member hints for orbit %q are drifted; run `%s` before %s",
			orbitID,
			backfillCommand,
			action,
		)
	}

	diagnostics := summarizeMemberHintDiagnostics(inspection.Hints)
	if diagnostics == "" {
		return fmt.Errorf(
			"current member hints for orbit %q are not ready for %s; inspect with `orbit member detect --orbit %s --json`, resolve the reported hint diagnostics, then run `%s`",
			orbitID,
			action,
			orbitID,
			backfillCommand,
		)
	}

	return fmt.Errorf(
		"current member hints for orbit %q are not ready for %s: %s; inspect with `orbit member detect --orbit %s --json`, resolve the reported hint diagnostics, then run `%s`",
		orbitID,
		action,
		diagnostics,
		orbitID,
		backfillCommand,
	)
}

func summarizeMemberHintDiagnostics(hints []orbitpkg.DetectedMemberHint) string {
	parts := make([]string, 0, 3)
	for _, hint := range hints {
		for _, diagnostic := range hint.Diagnostics {
			location := strings.TrimSpace(hint.HintPath)
			if location == "" {
				location = strings.TrimSpace(hint.RootPath)
			}
			if location == "" {
				location = strings.TrimSpace(hint.ResolvedName)
			}
			if location != "" {
				parts = append(parts, fmt.Sprintf("%s: %s", location, diagnostic))
			} else {
				parts = append(parts, diagnostic)
			}
			if len(parts) == 3 {
				return strings.Join(parts, "; ")
			}
		}
	}

	return strings.Join(parts, "; ")
}
