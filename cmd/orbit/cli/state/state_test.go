package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestCurrentOrbitStateRoundTrip(t *testing.T) {
	t.Parallel()

	store := newStore(t)
	current := statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}

	require.NoError(t, store.WriteCurrentOrbit(current))

	loaded, err := store.ReadCurrentOrbit()
	require.NoError(t, err)
	require.Equal(t, current, loaded)

	require.NoError(t, store.ClearCurrentOrbit())

	_, err = store.ReadCurrentOrbit()
	require.ErrorIs(t, err, statepkg.ErrCurrentOrbitNotFound)
}

func TestProjectionCacheRoundTrip(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	require.NoError(t, store.WriteProjectionCache("docs", []string{
		"docs/guide.md",
		`docs\guide.md`,
		"README.md",
	}))

	loaded, err := store.ReadProjectionCache("docs")
	require.NoError(t, err)
	require.Equal(t, []string{
		"README.md",
		"docs/guide.md",
	}, loaded)
}

func TestProjectionCacheRejectsInvalidOrbitID(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	err := store.WriteProjectionCache("Docs", []string{"README.md"})
	require.Error(t, err)
}

func TestAcquireLock(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	lock, err := store.AcquireLock()
	require.NoError(t, err)

	secondLock, err := store.AcquireLock()
	require.ErrorIs(t, err, statepkg.ErrLockHeld)
	require.Nil(t, secondLock)

	require.NoError(t, lock.Release())

	thirdLock, err := store.AcquireLock()
	require.NoError(t, err)
	require.NoError(t, thirdLock.Release())
}

func TestWarningsRoundTripOverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	first := statepkg.WarningSnapshot{
		CurrentOrbit: "docs",
		Messages:     []string{"outside changes are present"},
		CreatedAt:    time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteWarnings(first))

	loaded, err := store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, first, loaded)

	second := statepkg.WarningSnapshot{
		CurrentOrbit: "cmd",
		Messages:     []string{"warning one", "warning two"},
		CreatedAt:    time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteWarnings(second))

	loaded, err = store.ReadWarnings()
	require.NoError(t, err)
	require.Equal(t, second, loaded)
}

func TestStatusSnapshotRoundTripOverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	first := statepkg.StatusSnapshot{
		CurrentOrbit: "docs",
		InScope: []statepkg.PathChange{
			{Path: "docs/guide.md", Code: "M", Tracked: true, InScope: true},
			{Path: ".orbit/orbits/docs.yaml", Code: "M", Tracked: true, InScope: true},
		},
		OutOfScope: []statepkg.PathChange{
			{Path: "src/main.go", Code: "M", Tracked: true, InScope: false},
		},
		HiddenDirtyRisk: []string{"src/main.go"},
		SafeToSwitch:    false,
		CommitWarnings:  []string{"outside changes are present"},
		CreatedAt:       time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteStatusSnapshot(first))

	loaded, err := store.ReadStatusSnapshot()
	require.NoError(t, err)
	require.Equal(t, first, loaded)

	second := statepkg.StatusSnapshot{
		CurrentOrbit: "docs",
		InScope: []statepkg.PathChange{
			{Path: "docs/draft.md", Code: "??", Tracked: false, InScope: true},
		},
		OutOfScope: []statepkg.PathChange{
			{Path: "scratch/outside.txt", Code: "??", Tracked: false, InScope: false},
		},
		HiddenDirtyRisk: []string{},
		SafeToSwitch:    true,
		CommitWarnings:  []string{},
		CreatedAt:       time.Date(2026, time.March, 21, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteStatusSnapshot(second))

	loaded, err = store.ReadStatusSnapshot()
	require.NoError(t, err)
	require.Equal(t, second, loaded)
}

func TestFileInventorySnapshotRoundTripOverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	first := statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          "docs/guide.md",
				MemberName:    "docs-content",
				Role:          "subject",
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
			{
				Path:          ".orbit/orbits/docs.yaml",
				Role:          "meta",
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
		},
	}
	require.NoError(t, store.WriteFileInventorySnapshot(first))

	loaded, err := store.ReadFileInventorySnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".orbit/orbits/docs.yaml",
				Role:          "meta",
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
			{
				Path:          "docs/guide.md",
				MemberName:    "docs-content",
				Role:          "subject",
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
		},
	}, loaded)

	second := statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".markdownlint.yaml",
				MemberName:    "docs-rules",
				Role:          "rule",
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
		},
	}
	require.NoError(t, store.WriteFileInventorySnapshot(second))

	loaded, err = store.ReadFileInventorySnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, second, loaded)
}

func TestRuntimeStateSnapshotRoundTripOverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	first := statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		Running:   true,
		Phase:     "planning",
		EnteredAt: time.Date(2026, time.April, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.April, 5, 10, 5, 0, 0, time.UTC),
		PlanHash:  "abc123",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   false,
			CompletedAt: time.Time{},
		},
	}
	require.NoError(t, store.WriteRuntimeStateSnapshot(first))

	loaded, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, first, loaded)

	second := statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		Running:   false,
		Phase:     "idle",
		UpdatedAt: time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		PlanHash:  "def456",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 5, 11, 0, 0, 0, time.UTC),
		},
	}
	require.NoError(t, store.WriteRuntimeStateSnapshot(second))

	loaded, err = store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, second, loaded)
}

func TestGitStateSnapshotRoundTripOverwritesPreviousSnapshot(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	first := statepkg.GitStateSnapshot{
		Orbit: "docs",
		OrbitProjectionState: statepkg.GitScopeState{
			Count: 2,
			Paths: []string{"docs/guide.md", ".orbit/orbits/docs.yaml"},
		},
		OrbitStageState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitCommitState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitExportState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitOrchestrationState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{".orbit/orbits/docs.yaml"},
		},
		GlobalStageState: statepkg.GitScopeState{
			Count: 3,
			Paths: []string{".orbit/orbits/docs.yaml", "docs/guide.md", "src/main.go"},
		},
		GlobalCommitState: statepkg.GitScopeState{
			Count: 2,
			Paths: []string{"docs/guide.md", "src/main.go"},
		},
		UpdatedAt: time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteGitStateSnapshot(first))

	loaded, err := store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, statepkg.GitStateSnapshot{
		Orbit: "docs",
		OrbitProjectionState: statepkg.GitScopeState{
			Count: 2,
			Paths: []string{".orbit/orbits/docs.yaml", "docs/guide.md"},
		},
		OrbitStageState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitCommitState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitExportState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{"docs/guide.md"},
		},
		OrbitOrchestrationState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{".orbit/orbits/docs.yaml"},
		},
		GlobalStageState: statepkg.GitScopeState{
			Count: 3,
			Paths: []string{".orbit/orbits/docs.yaml", "docs/guide.md", "src/main.go"},
		},
		GlobalCommitState: statepkg.GitScopeState{
			Count: 2,
			Paths: []string{"docs/guide.md", "src/main.go"},
		},
		UpdatedAt: time.Date(2026, time.April, 5, 10, 10, 0, 0, time.UTC),
	}, loaded)

	second := statepkg.GitStateSnapshot{
		Orbit: "docs",
		OrbitProjectionState: statepkg.GitScopeState{
			Count: 1,
			Paths: []string{".markdownlint.yaml"},
		},
		UpdatedAt: time.Date(2026, time.April, 5, 11, 10, 0, 0, time.UTC),
	}
	require.NoError(t, store.WriteGitStateSnapshot(second))

	loaded, err = store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
	require.Equal(t, second, loaded)
}

func TestOrbitLedgerSnapshotsCoexistWithExistingStateFiles(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	require.NoError(t, store.WriteCurrentOrbit(statepkg.CurrentOrbitState{
		Orbit:         "docs",
		EnteredAt:     time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC),
		SparseEnabled: true,
	}))
	require.NoError(t, store.WriteProjectionCache("docs", []string{"docs/guide.md"}))
	require.NoError(t, store.WriteWarnings(statepkg.WarningSnapshot{
		CurrentOrbit: "docs",
		Messages:     []string{"warning"},
		CreatedAt:    time.Date(2026, time.April, 5, 12, 1, 0, 0, time.UTC),
	}))
	require.NoError(t, store.WriteStatusSnapshot(statepkg.StatusSnapshot{
		CurrentOrbit: "docs",
		CreatedAt:    time.Date(2026, time.April, 5, 12, 2, 0, 0, time.UTC),
	}))
	require.NoError(t, store.WriteFileInventorySnapshot(statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 12, 3, 0, 0, time.UTC),
	}))
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 5, 12, 4, 0, 0, time.UTC),
	}))
	require.NoError(t, store.WriteGitStateSnapshot(statepkg.GitStateSnapshot{
		Orbit:     "docs",
		UpdatedAt: time.Date(2026, time.April, 5, 12, 5, 0, 0, time.UTC),
	}))

	_, err := store.ReadCurrentOrbit()
	require.NoError(t, err)
	_, err = store.ReadProjectionCache("docs")
	require.NoError(t, err)
	_, err = store.ReadWarnings()
	require.NoError(t, err)
	_, err = store.ReadStatusSnapshot()
	require.NoError(t, err)
	_, err = store.ReadFileInventorySnapshot("docs")
	require.NoError(t, err)
	_, err = store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	_, err = store.ReadGitStateSnapshot("docs")
	require.NoError(t, err)
}

func newStore(t *testing.T) statepkg.FSStore {
	t.Helper()

	gitDir := filepath.Join(t.TempDir(), ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o750))

	store, err := statepkg.NewFSStore(gitDir)
	require.NoError(t, err)
	require.NoError(t, store.Ensure())

	return store
}

func TestReadMissingProjectionCache(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	_, err := store.ReadProjectionCache("docs")
	require.ErrorIs(t, err, statepkg.ErrProjectionCacheNotFound)
}

func TestReadMissingOrbitLedgerSnapshots(t *testing.T) {
	t.Parallel()

	store := newStore(t)

	_, err := store.ReadFileInventorySnapshot("docs")
	require.ErrorIs(t, err, statepkg.ErrFileInventorySnapshotNotFound)

	_, err = store.ReadRuntimeStateSnapshot("docs")
	require.ErrorIs(t, err, statepkg.ErrRuntimeStateSnapshotNotFound)

	_, err = store.ReadGitStateSnapshot("docs")
	require.ErrorIs(t, err, statepkg.ErrGitStateSnapshotNotFound)
}
