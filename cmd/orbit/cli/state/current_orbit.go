package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ErrCurrentOrbitNotFound indicates no current orbit state file exists.
var ErrCurrentOrbitNotFound = errors.New("current orbit state not found")

// CurrentOrbitState captures the active orbit runtime state.
type CurrentOrbitState struct {
	Orbit         string    `json:"orbit"`
	EnteredAt     time.Time `json:"entered_at"`
	SparseEnabled bool      `json:"sparse_enabled"`
}

// WriteCurrentOrbit writes current_orbit.json atomically.
func (store FSStore) WriteCurrentOrbit(current CurrentOrbitState) error {
	if err := ids.ValidateOrbitID(current.Orbit); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal current orbit state: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.currentOrbitPath(), data); err != nil {
		return fmt.Errorf("write current orbit state: %w", err)
	}

	return nil
}

// ReadCurrentOrbit reads current_orbit.json.
func (store FSStore) ReadCurrentOrbit() (CurrentOrbitState, error) {
	data, err := os.ReadFile(store.currentOrbitPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CurrentOrbitState{}, ErrCurrentOrbitNotFound
		}
		return CurrentOrbitState{}, fmt.Errorf("read current orbit state: %w", err)
	}

	var current CurrentOrbitState
	if err := json.Unmarshal(data, &current); err != nil {
		return CurrentOrbitState{}, fmt.Errorf("unmarshal current orbit state: %w", err)
	}

	return current, nil
}

// ClearCurrentOrbit removes current_orbit.json if it exists.
func (store FSStore) ClearCurrentOrbit() error {
	if err := os.Remove(store.currentOrbitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove current orbit state: %w", err)
	}

	return nil
}
