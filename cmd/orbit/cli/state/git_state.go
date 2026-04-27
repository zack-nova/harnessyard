package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const gitStateFileName = "git_state.json"

// ErrGitStateSnapshotNotFound indicates no git state exists yet for the requested orbit.
var ErrGitStateSnapshotNotFound = errors.New("git state snapshot not found")

// GitScopeState stores one minimal path-oriented Git observation bucket.
type GitScopeState struct {
	Count int      `json:"count"`
	Paths []string `json:"paths,omitempty"`
}

// GitStateSnapshot stores orbit-local and global Git observations side by side.
type GitStateSnapshot struct {
	Orbit                   string        `json:"orbit"`
	OrbitProjectionState    GitScopeState `json:"orbit_projection_state"`
	OrbitStageState         GitScopeState `json:"orbit_stage_state"`
	OrbitCommitState        GitScopeState `json:"orbit_commit_state"`
	OrbitExportState        GitScopeState `json:"orbit_export_state"`
	OrbitOrchestrationState GitScopeState `json:"orbit_orchestration_state"`
	GlobalStageState        GitScopeState `json:"global_stage_state"`
	GlobalCommitState       GitScopeState `json:"global_commit_state"`
	UpdatedAt               time.Time     `json:"updated_at"`
}

// WriteGitStateSnapshot writes git_state.json atomically.
func (store FSStore) WriteGitStateSnapshot(snapshot GitStateSnapshot) error {
	if err := ids.ValidateOrbitID(snapshot.Orbit); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}

	sortGitScopeState(&snapshot.OrbitProjectionState)
	sortGitScopeState(&snapshot.OrbitStageState)
	sortGitScopeState(&snapshot.OrbitCommitState)
	sortGitScopeState(&snapshot.OrbitExportState)
	sortGitScopeState(&snapshot.OrbitOrchestrationState)
	sortGitScopeState(&snapshot.GlobalStageState)
	sortGitScopeState(&snapshot.GlobalCommitState)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal git state snapshot: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.gitStatePath(snapshot.Orbit), data); err != nil {
		return fmt.Errorf("write git state snapshot: %w", err)
	}

	return nil
}

// ReadGitStateSnapshot reads git_state.json for one orbit.
func (store FSStore) ReadGitStateSnapshot(orbitID string) (GitStateSnapshot, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return GitStateSnapshot{}, fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := os.ReadFile(store.gitStatePath(orbitID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return GitStateSnapshot{}, ErrGitStateSnapshotNotFound
		}
		return GitStateSnapshot{}, fmt.Errorf("read git state snapshot: %w", err)
	}

	var snapshot GitStateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return GitStateSnapshot{}, fmt.Errorf("unmarshal git state snapshot: %w", err)
	}

	return snapshot, nil
}

func sortGitScopeState(state *GitScopeState) {
	sort.Strings(state.Paths)
}
