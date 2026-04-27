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

const fileInventoryFileName = "file_inventory.json"

// ErrFileInventorySnapshotNotFound indicates no file inventory exists yet for the requested orbit.
var ErrFileInventorySnapshotNotFound = errors.New("file inventory snapshot not found")

// FileInventoryEntry stores one role-aware file description for the local ledger.
type FileInventoryEntry struct {
	Path          string `json:"path"`
	MemberName    string `json:"member_name,omitempty"`
	Role          string `json:"role"`
	Projection    bool   `json:"projection"`
	OrbitWrite    bool   `json:"orbit_write"`
	Export        bool   `json:"export"`
	Orchestration bool   `json:"orchestration"`
}

// FileInventorySnapshot stores the current orbit-local file inventory.
type FileInventorySnapshot struct {
	Orbit       string               `json:"orbit"`
	GeneratedAt time.Time            `json:"generated_at"`
	Files       []FileInventoryEntry `json:"files"`
}

// WriteFileInventorySnapshot writes file_inventory.json atomically.
func (store FSStore) WriteFileInventorySnapshot(snapshot FileInventorySnapshot) error {
	if err := ids.ValidateOrbitID(snapshot.Orbit); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}

	sortFileInventoryEntries(snapshot.Files)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal file inventory snapshot: %w", err)
	}

	data = append(data, '\n')

	if err := atomicWriteFile(store.fileInventoryPath(snapshot.Orbit), data); err != nil {
		return fmt.Errorf("write file inventory snapshot: %w", err)
	}

	return nil
}

// ReadFileInventorySnapshot reads file_inventory.json for one orbit.
func (store FSStore) ReadFileInventorySnapshot(orbitID string) (FileInventorySnapshot, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return FileInventorySnapshot{}, fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := os.ReadFile(store.fileInventoryPath(orbitID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileInventorySnapshot{}, ErrFileInventorySnapshotNotFound
		}
		return FileInventorySnapshot{}, fmt.Errorf("read file inventory snapshot: %w", err)
	}

	var snapshot FileInventorySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return FileInventorySnapshot{}, fmt.Errorf("unmarshal file inventory snapshot: %w", err)
	}

	return snapshot, nil
}

func sortFileInventoryEntries(entries []FileInventoryEntry) {
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Path == entries[right].Path {
			if entries[left].Role == entries[right].Role {
				return entries[left].MemberName < entries[right].MemberName
			}

			return entries[left].Role < entries[right].Role
		}

		return entries[left].Path < entries[right].Path
	})
}
