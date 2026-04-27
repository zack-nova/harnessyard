package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const runtimeStateFileName = "runtime_state.json"

// ErrRuntimeStateSnapshotNotFound indicates no runtime state exists yet for the requested orbit.
var ErrRuntimeStateSnapshotNotFound = errors.New("runtime state snapshot not found")

// RuntimeBootstrapState stores bootstrap completion state in the orbit-local runtime ledger.
type RuntimeBootstrapState struct {
	Completed   bool      `json:"completed"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// RuntimeStateSnapshot stores one orbit-local runtime ledger snapshot.
type RuntimeStateSnapshot struct {
	Orbit     string                 `json:"orbit"`
	Running   bool                   `json:"running"`
	Phase     string                 `json:"phase"`
	EnteredAt time.Time              `json:"entered_at,omitempty"`
	UpdatedAt time.Time              `json:"updated_at"`
	PlanHash  string                 `json:"plan_hash,omitempty"`
	Bootstrap *RuntimeBootstrapState `json:"bootstrap,omitempty"`
}

// WriteRuntimeStateSnapshot writes runtime_state.json atomically.
func (store FSStore) WriteRuntimeStateSnapshot(snapshot RuntimeStateSnapshot) error {
	if err := ids.ValidateOrbitID(snapshot.Orbit); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runtime state snapshot: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.runtimeStatePath(snapshot.Orbit), data); err != nil {
		return fmt.Errorf("write runtime state snapshot: %w", err)
	}

	return nil
}

// ReadRuntimeStateSnapshot reads runtime_state.json for one orbit.
func (store FSStore) ReadRuntimeStateSnapshot(orbitID string) (RuntimeStateSnapshot, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return RuntimeStateSnapshot{}, fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := os.ReadFile(store.runtimeStatePath(orbitID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RuntimeStateSnapshot{}, ErrRuntimeStateSnapshotNotFound
		}
		return RuntimeStateSnapshot{}, fmt.Errorf("read runtime state snapshot: %w", err)
	}

	var snapshot RuntimeStateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return RuntimeStateSnapshot{}, fmt.Errorf("unmarshal runtime state snapshot: %w", err)
	}

	return snapshot, nil
}
