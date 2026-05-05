package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	directoryPerm = 0o750
	filePerm      = 0o600
)

const (
	orbitStateDirName  = "orbit"
	stateDirName       = "state"
	orbitLedgerDirName = "orbits"
	// Keep the on-disk directory name stable while the codebase moves to projection-cache terminology.
	projectionCacheDirName = "resolved_scope"
	currentOrbitFileName   = "current_orbit.json"
	runtimeViewFileName    = "runtime_view_selection.json"
	lockFileName           = "orbit.lock"
)

var renameFile = os.Rename

// FSStore provides file-backed access to .git/orbit/state.
type FSStore struct {
	GitDir             string
	StateDir           string
	ProjectionCacheDir string
	OrbitLedgerDir     string
}

// NewFSStore builds the repository-local Orbit state store rooted at the git dir.
func NewFSStore(gitDir string) (FSStore, error) {
	if gitDir == "" {
		return FSStore{}, fmt.Errorf("git dir must not be empty")
	}

	cleanGitDir := filepath.Clean(gitDir)

	return FSStore{
		GitDir:             cleanGitDir,
		StateDir:           filepath.Join(cleanGitDir, orbitStateDirName, stateDirName),
		ProjectionCacheDir: filepath.Join(cleanGitDir, orbitStateDirName, stateDirName, projectionCacheDirName),
		OrbitLedgerDir:     filepath.Join(cleanGitDir, orbitStateDirName, stateDirName, orbitLedgerDirName),
	}, nil
}

// Ensure creates the required state directories.
func (store FSStore) Ensure() error {
	if err := os.MkdirAll(store.ProjectionCacheDir, directoryPerm); err != nil {
		return fmt.Errorf("create state directories: %w", err)
	}
	if err := os.MkdirAll(store.OrbitLedgerDir, directoryPerm); err != nil {
		return fmt.Errorf("create state directories: %w", err)
	}

	return nil
}

func (store FSStore) currentOrbitPath() string {
	return filepath.Join(store.StateDir, currentOrbitFileName)
}

func (store FSStore) runtimeViewSelectionPath() string {
	return filepath.Join(store.StateDir, runtimeViewFileName)
}

func (store FSStore) lockPath() string {
	return filepath.Join(store.StateDir, lockFileName)
}

func (store FSStore) projectionCachePath(orbitID string) string {
	return filepath.Join(store.ProjectionCacheDir, orbitID+".txt")
}

func (store FSStore) orbitLedgerPath(orbitID string, filename string) string {
	return filepath.Join(store.OrbitLedgerDir, orbitID, filename)
}

func (store FSStore) warningsPath() string {
	return filepath.Join(store.StateDir, warningsFileName)
}

func (store FSStore) lastStatusPath() string {
	return filepath.Join(store.StateDir, lastStatusFileName)
}

func (store FSStore) fileInventoryPath(orbitID string) string {
	return store.orbitLedgerPath(orbitID, fileInventoryFileName)
}

func (store FSStore) runtimeStatePath(orbitID string) string {
	return store.orbitLedgerPath(orbitID, runtimeStateFileName)
}

func (store FSStore) gitStatePath(orbitID string) string {
	return store.orbitLedgerPath(orbitID, gitStateFileName)
}

func atomicWriteFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), directoryPerm); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", filename, err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(filename), ".orbit-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", filename, err)
	}

	tempName := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempName)
		}
	}()

	if err := tempFile.Chmod(filePerm); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("set temp file permissions for %s: %w", filename, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file for %s: %w", filename, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file for %s: %w", filename, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", filename, err)
	}
	if err := renameFile(tempName, filename); err != nil {
		return fmt.Errorf("rename temp file for %s: %w", filename, err)
	}

	cleanupTemp = false

	return nil
}
