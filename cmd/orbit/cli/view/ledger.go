package view

import (
	"fmt"
	"sort"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// WriteActiveRuntimeAndGitLedger records the current active runtime phase plus
// role-aware Git observations for one orbit.
func WriteActiveRuntimeAndGitLedger(
	store statepkg.FSStore,
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	plan orbitpkg.ProjectionPlan,
	orbitID string,
	phase string,
	enteredAt time.Time,
	updatedAt time.Time,
	entries []gitpkg.StatusEntry,
) error {
	if err := store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     orbitID,
		Running:   true,
		Phase:     phase,
		EnteredAt: enteredAt,
		UpdatedAt: updatedAt,
		PlanHash:  plan.PlanHash,
	}); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	gitSnapshot, err := buildActiveGitStateSnapshot(config, spec, plan, orbitID, updatedAt, entries)
	if err != nil {
		return fmt.Errorf("build git state snapshot: %w", err)
	}
	if err := store.WriteGitStateSnapshot(gitSnapshot); err != nil {
		return fmt.Errorf("write git state snapshot: %w", err)
	}

	return nil
}

// WriteLeftRuntimeAndGitLedger records the inactive runtime phase plus global
// Git observations after leaving an orbit projection.
func WriteLeftRuntimeAndGitLedger(
	store statepkg.FSStore,
	orbitID string,
	enteredAt time.Time,
	planHash string,
	updatedAt time.Time,
	entries []gitpkg.StatusEntry,
) error {
	if orbitID == "" {
		return nil
	}

	if err := store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     orbitID,
		Running:   false,
		Phase:     "left",
		EnteredAt: enteredAt,
		UpdatedAt: updatedAt,
		PlanHash:  planHash,
	}); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	if err := store.WriteGitStateSnapshot(statepkg.GitStateSnapshot{
		Orbit:             orbitID,
		GlobalStageState:  gitScopeStateFromSet(collectPaths(entries, func(entry gitpkg.StatusEntry) bool { return entry.Staged })),
		GlobalCommitState: gitScopeStateFromSet(collectPaths(entries, func(_ gitpkg.StatusEntry) bool { return true })),
		UpdatedAt:         updatedAt,
	}); err != nil {
		return fmt.Errorf("write git state snapshot: %w", err)
	}

	return nil
}

func previousRuntimeLedgerState(store statepkg.FSStore, orbitID string) (time.Time, string) {
	if orbitID == "" {
		return time.Time{}, ""
	}

	snapshot, err := store.ReadRuntimeStateSnapshot(orbitID)
	if err != nil {
		return time.Time{}, ""
	}

	return snapshot.EnteredAt, snapshot.PlanHash
}

func buildActiveGitStateSnapshot(
	config orbitpkg.RepositoryConfig,
	spec orbitpkg.OrbitSpec,
	plan orbitpkg.ProjectionPlan,
	orbitID string,
	updatedAt time.Time,
	entries []gitpkg.StatusEntry,
) (statepkg.GitStateSnapshot, error) {
	orbitProjection := make(map[string]struct{}, len(entries))
	orbitStage := make(map[string]struct{}, len(entries))
	orbitCommit := make(map[string]struct{}, len(entries))
	orbitExport := make(map[string]struct{}, len(entries))
	orbitOrchestration := make(map[string]struct{}, len(entries))
	globalStage := make(map[string]struct{}, len(entries))
	globalCommit := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		globalCommit[entry.Path] = struct{}{}
		if entry.Staged {
			globalStage[entry.Path] = struct{}{}
		}

		classification, err := orbitpkg.ClassifyOrbitPath(config, spec, plan, entry.Path, entry.Tracked)
		if err != nil {
			return statepkg.GitStateSnapshot{}, fmt.Errorf("classify path %q: %w", entry.Path, err)
		}

		if classification.Projection {
			orbitProjection[entry.Path] = struct{}{}
		}
		if classification.OrbitWrite {
			orbitCommit[entry.Path] = struct{}{}
			if entry.Staged {
				orbitStage[entry.Path] = struct{}{}
			}
		}
		if classification.Export {
			orbitExport[entry.Path] = struct{}{}
		}
		if classification.Orchestration {
			orbitOrchestration[entry.Path] = struct{}{}
		}
	}

	return statepkg.GitStateSnapshot{
		Orbit:                   orbitID,
		OrbitProjectionState:    gitScopeStateFromSet(orbitProjection),
		OrbitStageState:         gitScopeStateFromSet(orbitStage),
		OrbitCommitState:        gitScopeStateFromSet(orbitCommit),
		OrbitExportState:        gitScopeStateFromSet(orbitExport),
		OrbitOrchestrationState: gitScopeStateFromSet(orbitOrchestration),
		GlobalStageState:        gitScopeStateFromSet(globalStage),
		GlobalCommitState:       gitScopeStateFromSet(globalCommit),
		UpdatedAt:               updatedAt,
	}, nil
}

func collectPaths(entries []gitpkg.StatusEntry, include func(gitpkg.StatusEntry) bool) map[string]struct{} {
	paths := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if !include(entry) {
			continue
		}
		paths[entry.Path] = struct{}{}
	}

	return paths
}

func gitScopeStateFromSet(paths map[string]struct{}) statepkg.GitScopeState {
	ordered := make([]string, 0, len(paths))
	for pathValue := range paths {
		ordered = append(ordered, pathValue)
	}

	sort.Strings(ordered)

	return statepkg.GitScopeState{
		Count: len(ordered),
		Paths: ordered,
	}
}
