package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

const lastStatusFileName = "last_status.json"

// ErrStatusSnapshotNotFound indicates no status snapshot exists yet.
var ErrStatusSnapshotNotFound = errors.New("status snapshot not found")

// PathChange captures a classified worktree path change.
type PathChange struct {
	Path          string `json:"path"`
	Code          string `json:"code"`
	Tracked       bool   `json:"tracked"`
	InScope       bool   `json:"in_scope"`
	Role          string `json:"role"`
	Projection    bool   `json:"projection"`
	OrbitWrite    bool   `json:"orbit_write"`
	Export        bool   `json:"export"`
	Orchestration bool   `json:"orchestration"`
}

// StatusSnapshot stores the most recent orbit status classification.
type StatusSnapshot struct {
	CurrentOrbit    string       `json:"current_orbit"`
	InScope         []PathChange `json:"in_scope"`
	OutOfScope      []PathChange `json:"out_of_scope"`
	HiddenDirtyRisk []string     `json:"hidden_dirty_risk"`
	SafeToSwitch    bool         `json:"safe_to_switch"`
	CommitWarnings  []string     `json:"commit_warnings"`
	CreatedAt       time.Time    `json:"created_at"`
}

// WriteStatusSnapshot writes last_status.json atomically.
func (store FSStore) WriteStatusSnapshot(snapshot StatusSnapshot) error {
	sortPathChanges(snapshot.InScope)
	sortPathChanges(snapshot.OutOfScope)
	sort.Strings(snapshot.HiddenDirtyRisk)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status snapshot: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.lastStatusPath(), data); err != nil {
		return fmt.Errorf("write status snapshot: %w", err)
	}

	return nil
}

// ReadStatusSnapshot reads the most recent status snapshot.
func (store FSStore) ReadStatusSnapshot() (StatusSnapshot, error) {
	data, err := os.ReadFile(store.lastStatusPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StatusSnapshot{}, ErrStatusSnapshotNotFound
		}
		return StatusSnapshot{}, fmt.Errorf("read status snapshot: %w", err)
	}

	var snapshot StatusSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return StatusSnapshot{}, fmt.Errorf("unmarshal status snapshot: %w", err)
	}

	return snapshot, nil
}

func sortPathChanges(changes []PathChange) {
	sort.Slice(changes, func(left, right int) bool {
		if changes[left].Path == changes[right].Path {
			return changes[left].Code < changes[right].Code
		}

		return changes[left].Path < changes[right].Path
	})
}
