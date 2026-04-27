package view

import (
	"context"
	"fmt"
	"sort"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// StatusResult contains the latest classified status for the current orbit.
type StatusResult struct {
	CurrentOrbit    string                  `json:"current_orbit"`
	Snapshot        statepkg.StatusSnapshot `json:"snapshot"`
	WarningMessages []string                `json:"warnings,omitempty"`
}

const (
	statusSnapshotRefreshWarning = "failed to refresh status snapshot for orbit status: %v"
	statusLedgerRefreshWarning   = "failed to refresh runtime and git ledger for orbit status: %v"
	statusWarningSnapshotError   = "failed to refresh warning snapshot for orbit status: %v"
)

// Status classifies current worktree changes for the active orbit.
func Status(
	ctx context.Context,
	repo gitpkg.Repo,
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	current statepkg.CurrentOrbitState,
	definition orbitpkg.Definition,
) (StatusResult, error) {
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repo.Root)
	if err != nil {
		return StatusResult{}, fmt.Errorf("load tracked files: %w", err)
	}

	spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(ctx, repo.Root, config, definition.ID, trackedFiles)
	if err != nil {
		return StatusResult{}, fmt.Errorf("load current orbit runtime plan: %w", err)
	}
	if err := ValidateCurrentRuntimeLedgerPlan(store, current.Orbit, plan.PlanHash); err != nil {
		return StatusResult{}, err
	}
	if err := store.WriteProjectionCache(definition.ID, plan.ProjectionPaths); err != nil {
		return StatusResult{}, fmt.Errorf("write projection cache: %w", err)
	}
	inventorySnapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Now().UTC())
	if err != nil {
		return StatusResult{}, fmt.Errorf("build file inventory snapshot: %w", err)
	}
	if err := store.WriteFileInventorySnapshot(inventorySnapshot); err != nil {
		return StatusResult{}, fmt.Errorf("write file inventory snapshot: %w", err)
	}

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repo.Root)
	if err != nil {
		return StatusResult{}, fmt.Errorf("load worktree status: %w", err)
	}

	snapshot, err := ClassifyStatusForSpec(config, spec, current.Orbit, plan, statusEntries)
	if err != nil {
		return StatusResult{}, fmt.Errorf("classify status: %w", err)
	}
	snapshot.CreatedAt = time.Now().UTC()

	result := StatusResult{
		CurrentOrbit: current.Orbit,
		Snapshot:     snapshot,
	}

	if err := store.WriteStatusSnapshot(snapshot); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf(statusSnapshotRefreshWarning, err))
	}
	if err := WriteActiveRuntimeAndGitLedger(
		store,
		config,
		spec,
		plan,
		current.Orbit,
		"status",
		current.EnteredAt,
		snapshot.CreatedAt,
		statusEntries,
	); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf(statusLedgerRefreshWarning, err))
	}
	warningSnapshotMessages := append([]string{}, snapshot.CommitWarnings...)
	warningSnapshotMessages = append(warningSnapshotMessages, result.WarningMessages...)
	if err := WriteWarnings(store, current.Orbit, warningSnapshotMessages); err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf(statusWarningSnapshotError, err))
	}

	return result, nil
}

// ClassifyStatusForSpec applies role-aware scope rules for a resolved orbit spec.
func ClassifyStatusForSpec(
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	currentOrbit string,
	plan orbitpkg.ProjectionPlan,
	entries []gitpkg.StatusEntry,
) (statepkg.StatusSnapshot, error) {
	return classifyStatusForSpec(config, spec, currentOrbit, plan, entries, func(classification orbitpkg.PathClassification) bool {
		return classification.Projection
	})
}

// ClassifyOrbitWriteStatusForSpec applies orbit-write scope rules for commands
// that must act only on the role-aware write surface.
func ClassifyOrbitWriteStatusForSpec(
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	currentOrbit string,
	plan orbitpkg.ProjectionPlan,
	entries []gitpkg.StatusEntry,
) (statepkg.StatusSnapshot, error) {
	return classifyStatusForSpec(config, spec, currentOrbit, plan, entries, func(classification orbitpkg.PathClassification) bool {
		return classification.OrbitWrite
	})
}

func classifyStatusForSpec(
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	currentOrbit string,
	plan orbitpkg.ProjectionPlan,
	entries []gitpkg.StatusEntry,
	isInScope func(orbitpkg.PathClassification) bool,
) (statepkg.StatusSnapshot, error) {
	snapshot := statepkg.StatusSnapshot{
		CurrentOrbit: currentOrbit,
		InScope:      make([]statepkg.PathChange, 0),
		OutOfScope:   make([]statepkg.PathChange, 0),
		SafeToSwitch: true,
	}

	for _, entry := range entries {
		classification, err := orbitpkg.ClassifyOrbitPath(config, spec, plan, entry.Path, entry.Tracked)
		if err != nil {
			return statepkg.StatusSnapshot{}, fmt.Errorf("classify path %q: %w", entry.Path, err)
		}

		change := statepkg.PathChange{
			Path:          entry.Path,
			Code:          entry.Code,
			Tracked:       entry.Tracked,
			InScope:       isInScope(classification),
			Role:          classification.Role,
			Projection:    classification.Projection,
			OrbitWrite:    classification.OrbitWrite,
			Export:        classification.Export,
			Orchestration: classification.Orchestration,
		}

		if change.InScope {
			snapshot.InScope = append(snapshot.InScope, change)
			continue
		}

		snapshot.OutOfScope = append(snapshot.OutOfScope, change)
		if entry.Tracked {
			snapshot.HiddenDirtyRisk = append(snapshot.HiddenDirtyRisk, entry.Path)
		}
	}

	finalizeStatusSnapshot(&snapshot)

	return snapshot, nil
}

// ClassifyStatus applies the shared in-scope / out-of-scope rules for an orbit snapshot.
func ClassifyStatus(
	config orbitpkg.GlobalConfig,
	definition orbitpkg.Definition,
	currentOrbit string,
	scope []string,
	entries []gitpkg.StatusEntry,
) (statepkg.StatusSnapshot, error) {
	scopeSet := makeScopeSet(scope)
	snapshot := statepkg.StatusSnapshot{
		CurrentOrbit: currentOrbit,
		InScope:      make([]statepkg.PathChange, 0),
		OutOfScope:   make([]statepkg.PathChange, 0),
		SafeToSwitch: true,
	}

	for _, entry := range entries {
		change := statepkg.PathChange{
			Path:    entry.Path,
			Code:    entry.Code,
			Tracked: entry.Tracked,
		}

		if entry.Tracked {
			_, change.InScope = scopeSet[entry.Path]
		} else {
			inScope, err := orbitpkg.PathMatchesOrbit(config, definition, entry.Path)
			if err != nil {
				return statepkg.StatusSnapshot{}, fmt.Errorf("classify untracked path %q: %w", entry.Path, err)
			}
			change.InScope = inScope
		}

		if change.InScope {
			snapshot.InScope = append(snapshot.InScope, change)
			continue
		}

		snapshot.OutOfScope = append(snapshot.OutOfScope, change)
		if entry.Tracked {
			snapshot.HiddenDirtyRisk = append(snapshot.HiddenDirtyRisk, entry.Path)
		}
	}

	if len(snapshot.HiddenDirtyRisk) > 0 {
		snapshot.SafeToSwitch = false
	}
	if len(snapshot.OutOfScope) > 0 {
		snapshot.CommitWarnings = append(
			snapshot.CommitWarnings,
			"outside changes are present; orbit commit will only include the current orbit scope",
		)
	}

	return snapshot, nil
}

func makeScopeSet(scope []string) map[string]struct{} {
	scopeSet := make(map[string]struct{}, len(scope))
	for _, pathValue := range scope {
		scopeSet[pathValue] = struct{}{}
	}

	return scopeSet
}

func finalizeStatusSnapshot(snapshot *statepkg.StatusSnapshot) {
	sort.Slice(snapshot.InScope, func(left, right int) bool {
		if snapshot.InScope[left].Path == snapshot.InScope[right].Path {
			return snapshot.InScope[left].Code < snapshot.InScope[right].Code
		}

		return snapshot.InScope[left].Path < snapshot.InScope[right].Path
	})
	sort.Slice(snapshot.OutOfScope, func(left, right int) bool {
		if snapshot.OutOfScope[left].Path == snapshot.OutOfScope[right].Path {
			return snapshot.OutOfScope[left].Code < snapshot.OutOfScope[right].Code
		}

		return snapshot.OutOfScope[left].Path < snapshot.OutOfScope[right].Path
	})
	sort.Strings(snapshot.HiddenDirtyRisk)

	if len(snapshot.HiddenDirtyRisk) > 0 {
		snapshot.SafeToSwitch = false
	}
	if len(snapshot.OutOfScope) > 0 {
		snapshot.CommitWarnings = append(
			snapshot.CommitWarnings,
			"outside changes are present; orbit commit will only include the current orbit scope",
		)
	}
}
