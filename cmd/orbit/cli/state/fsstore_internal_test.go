package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteCurrentOrbitPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}
	require.NoError(t, store.WriteCurrentOrbit(original))

	currentPath := store.currentOrbitPath()
	originalBytes, err := os.ReadFile(currentPath)
	require.NoError(t, err)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteCurrentOrbit(CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC),
		SparseEnabled: false,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(currentPath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadCurrentOrbit()
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func TestWriteWarningsPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := WarningSnapshot{
		CurrentOrbit: "docs",
		Messages:     []string{"outside changes are present"},
		CreatedAt:    time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteWarnings(original))

	warningsPath := store.warningsPath()
	originalBytes, err := os.ReadFile(warningsPath)
	require.NoError(t, err)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteWarnings(WarningSnapshot{
		CurrentOrbit: "cmd",
		Messages:     []string{"new warning"},
		CreatedAt:    time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(warningsPath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func TestWriteStatusSnapshotPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := StatusSnapshot{
		CurrentOrbit: "docs",
		InScope: []PathChange{
			{Path: ".orbit/orbits/docs.yaml", Code: "M", Tracked: true, InScope: true},
			{Path: "docs/guide.md", Code: "M", Tracked: true, InScope: true},
		},
		OutOfScope: []PathChange{
			{Path: "src/main.go", Code: "M", Tracked: true, InScope: false},
		},
		HiddenDirtyRisk: []string{"src/main.go"},
		SafeToSwitch:    false,
		CommitWarnings:  []string{"outside changes are present"},
		CreatedAt:       time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteStatusSnapshot(original))

	statusPath := store.lastStatusPath()
	originalBytes, err := os.ReadFile(statusPath)
	require.NoError(t, err)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteStatusSnapshot(StatusSnapshot{
		CurrentOrbit: "docs",
		InScope: []PathChange{
			{Path: "docs/draft.md", Code: "??", Tracked: false, InScope: true},
		},
		OutOfScope: []PathChange{
			{Path: "scratch/outside.txt", Code: "??", Tracked: false, InScope: false},
		},
		HiddenDirtyRisk: []string{},
		SafeToSwitch:    true,
		CommitWarnings:  []string{},
		CreatedAt:       time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(statusPath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadStatusSnapshot()
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func TestWriteFileInventorySnapshotPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 13, 0, 0, 0, time.UTC),
		Files: []FileInventoryEntry{
			{Path: "docs/guide.md", MemberName: "docs-content", Role: "subject", Projection: true},
		},
	}
	require.NoError(t, store.WriteFileInventorySnapshot(original))

	inventoryPath := store.fileInventoryPath("docs")
	originalBytes, err := os.ReadFile(inventoryPath)
	require.NoError(t, err)
	require.Contains(t, string(originalBytes), `"member_name": "docs-content"`)
	require.NotContains(t, string(originalBytes), `"member_key"`)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteFileInventorySnapshot(FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 14, 0, 0, 0, time.UTC),
		Files: []FileInventoryEntry{
			{Path: ".markdownlint.yaml", MemberName: "docs-rules", Role: "rule", Projection: true, OrbitWrite: true},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(inventoryPath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadFileInventorySnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func TestWriteRuntimeStateSnapshotPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := RuntimeStateSnapshot{
		Orbit:     "docs",
		Running:   true,
		Phase:     "planning",
		EnteredAt: time.Date(2026, time.April, 5, 13, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.April, 5, 13, 5, 0, 0, time.UTC),
		PlanHash:  "abc123",
		Bootstrap: &RuntimeBootstrapState{
			Completed:   false,
			CompletedAt: time.Time{},
		},
	}
	require.NoError(t, store.WriteRuntimeStateSnapshot(original))

	runtimeStatePath := store.runtimeStatePath("docs")
	originalBytes, err := os.ReadFile(runtimeStatePath)
	require.NoError(t, err)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteRuntimeStateSnapshot(RuntimeStateSnapshot{
		Orbit:     "docs",
		Running:   false,
		Phase:     "idle",
		UpdatedAt: time.Date(2026, time.April, 5, 14, 0, 0, 0, time.UTC),
		PlanHash:  "def456",
		Bootstrap: &RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 5, 14, 0, 0, 0, time.UTC),
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(runtimeStatePath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func TestWriteGitStateSnapshotPreservesExistingStateWhenRenameFails(t *testing.T) {
	store := newInternalStore(t)

	original := GitStateSnapshot{
		Orbit: "docs",
		OrbitProjectionState: GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		UpdatedAt: time.Date(2026, time.April, 5, 13, 10, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteGitStateSnapshot(original))

	gitStatePath := store.gitStatePath("docs")
	originalBytes, err := os.ReadFile(gitStatePath)
	require.NoError(t, err)

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err = store.WriteGitStateSnapshot(GitStateSnapshot{
		Orbit: "docs",
		GlobalStageState: GitScopeState{
			Count: 1,
			Paths: []string{"src/main.go"},
		},
		UpdatedAt: time.Date(2026, time.April, 5, 14, 10, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	actualBytes, err := os.ReadFile(gitStatePath)
	require.NoError(t, err)
	require.Equal(t, originalBytes, actualBytes)

	loaded, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, original, loaded)

	assertNoStateTempFiles(t, store)
}

func newInternalStore(t *testing.T) FSStore {
	t.Helper()

	gitDir := filepath.Join(t.TempDir(), ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o750))

	store, err := NewFSStore(gitDir)
	require.NoError(t, err)
	require.NoError(t, store.Ensure())

	return store
}

func assertNoStateTempFiles(t *testing.T, store FSStore) {
	t.Helper()

	tempFiles, err := filepath.Glob(filepath.Join(store.StateDir, ".orbit-*"))
	require.NoError(t, err)
	require.Empty(t, tempFiles)
}
