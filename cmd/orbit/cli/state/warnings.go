package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const warningsFileName = "warnings.json"

// ErrWarningsNotFound indicates no warning snapshot exists yet.
var ErrWarningsNotFound = errors.New("warning snapshot not found")

// WarningSnapshot stores the latest warning summary for a repo-local Orbit command.
type WarningSnapshot struct {
	CurrentOrbit string    `json:"current_orbit"`
	Messages     []string  `json:"messages"`
	CreatedAt    time.Time `json:"created_at"`
}

// WriteWarnings writes warnings.json atomically.
func (store FSStore) WriteWarnings(snapshot WarningSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal warning snapshot: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.warningsPath(), data); err != nil {
		return fmt.Errorf("write warning snapshot: %w", err)
	}

	return nil
}

// ReadWarnings reads the latest warning snapshot.
func (store FSStore) ReadWarnings() (WarningSnapshot, error) {
	data, err := os.ReadFile(store.warningsPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WarningSnapshot{}, ErrWarningsNotFound
		}
		return WarningSnapshot{}, fmt.Errorf("read warning snapshot: %w", err)
	}

	var snapshot WarningSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return WarningSnapshot{}, fmt.Errorf("unmarshal warning snapshot: %w", err)
	}

	return snapshot, nil
}
