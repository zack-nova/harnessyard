package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessBootstrapCompleteOrbitRemovesRuntimeBootstrapSurfaceAndWritesState(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{
		includeOpsOrbit: true,
	})

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot            string   `json:"harness_root"`
		CompletedOrbits        []string `json:"completed_orbits"`
		AlreadyCompletedOrbits []string `json:"already_completed_orbits"`
		RemovedPaths           []string `json:"removed_paths"`
		RemovedBootstrapBlocks []string `json:"removed_bootstrap_blocks"`
		DeletedBootstrapFile   bool     `json:"deleted_bootstrap_file"`
		AutoLeftCurrentOrbit   bool     `json:"auto_left_current_orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, []string{"docs"}, payload.CompletedOrbits)
	require.Empty(t, payload.AlreadyCompletedOrbits)
	require.Contains(t, payload.RemovedPaths, "bootstrap/docs/setup.md")
	require.Equal(t, []string{"docs"}, payload.RemovedBootstrapBlocks)
	require.False(t, payload.DeletedBootstrapFile)
	require.False(t, payload.AutoLeftCurrentOrbit)

	_, err = os.Stat(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	bootstrapData, err := os.ReadFile(filepath.Join(repo.Root, "BOOTSTRAP.md"))
	require.NoError(t, err)
	require.NotContains(t, string(bootstrapData), `orbit_id="docs"`)
	require.Contains(t, string(bootstrapData), `orbit_id="ops"`)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	snapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.NotNil(t, snapshot.Bootstrap)
	require.True(t, snapshot.Bootstrap.Completed)
	require.False(t, snapshot.Bootstrap.CompletedAt.IsZero())
}

func TestHarnessBootstrapCompleteAllLeavesAlreadyCompletedOrbitsAsStableNoOp(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{
		includeOpsOrbit: true,
	})

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit:     "ops",
		UpdatedAt: time.Date(2026, time.April, 19, 10, 15, 0, 0, time.UTC),
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		CompletedOrbits        []string `json:"completed_orbits"`
		AlreadyCompletedOrbits []string `json:"already_completed_orbits"`
		RemovedPaths           []string `json:"removed_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"docs"}, payload.CompletedOrbits)
	require.Equal(t, []string{"ops"}, payload.AlreadyCompletedOrbits)
	require.Contains(t, payload.RemovedPaths, "bootstrap/docs/setup.md")
	require.NotContains(t, payload.RemovedPaths, "bootstrap/ops/setup.md")

	_, err = os.Stat(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "bootstrap", "ops", "setup.md"))
	require.NoError(t, err)

	docsSnapshot, err := store.ReadRuntimeStateSnapshot("docs")
	require.NoError(t, err)
	require.NotNil(t, docsSnapshot.Bootstrap)
	require.True(t, docsSnapshot.Bootstrap.Completed)

	opsSnapshot, err := store.ReadRuntimeStateSnapshot("ops")
	require.NoError(t, err)
	require.NotNil(t, opsSnapshot.Bootstrap)
	require.True(t, opsSnapshot.Bootstrap.Completed)
	require.Equal(t, time.Date(2026, time.April, 19, 10, 0, 0, 0, time.UTC), opsSnapshot.Bootstrap.CompletedAt)
}

func TestHarnessBootstrapCompleteAutoLeavesCurrentTargetOrbitProjection(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})
	var err error
	_, _, err = executeOrbitCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeOrbitCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		AutoLeftCurrentOrbit bool `json:"auto_left_current_orbit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.AutoLeftCurrentOrbit)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	_, err = store.ReadCurrentOrbit()
	require.ErrorIs(t, err, statepkg.ErrCurrentOrbitNotFound)
}

func TestHarnessBootstrapCompleteRejectsDirtyBootstrapSurface(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Locally edited docs bootstrap\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot complete bootstrap for orbit(s) docs with uncommitted changes`)
	require.ErrorContains(t, err, `bootstrap/docs/setup.md`)

	_, statErr := os.Stat(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md"))
	require.NoError(t, statErr)
}

func TestHarnessBootstrapCompleteRejectsUntrackedBootstrapSurface(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{})
	repo.WriteFile(t, "bootstrap/docs/generated.md", "Untracked bootstrap artifact\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot complete bootstrap for orbit(s) docs with uncommitted changes`)
	require.ErrorContains(t, err, `bootstrap/docs/generated.md`)

	_, statErr := os.Stat(filepath.Join(repo.Root, "bootstrap", "docs", "generated.md"))
	require.NoError(t, statErr)
}

func TestHarnessBootstrapCompleteRejectsHiddenBootstrapPathsFromAnotherProjection(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapCompletionRepo(t, bootstrapCompletionSeedOptions{
		includeOpsOrbit: true,
	})
	_, _, err := executeOrbitCLI(t, repo.Root, "init")
	require.NoError(t, err)
	_, _, err = executeOrbitCLI(t, repo.Root, "enter", "ops")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bootstrap", "complete", "--orbit", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `leave the current orbit first`)
	require.ErrorContains(t, err, `bootstrap/docs/setup.md`)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	current, err := store.ReadCurrentOrbit()
	require.NoError(t, err)
	require.Equal(t, "ops", current.Orbit)
}

type bootstrapCompletionSeedOptions struct {
	includeOpsOrbit bool
}

func seedHarnessBootstrapCompletionRepo(t *testing.T, options bootstrapCompletionSeedOptions) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 19, 9, 0, 0, 0, time.UTC)

	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Docs bootstrap setup\n")

	docsSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	require.NotNil(t, docsSpec.Meta)
	docsSpec.Description = "Docs orbit"
	docsSpec.Meta.BootstrapTemplate = "Bootstrap the docs orbit.\n"
	docsSpec.Members = append(docsSpec.Members, orbitpkg.OrbitMember{
		Key:  "docs-bootstrap",
		Role: orbitpkg.OrbitMemberRule,
		Lane: orbitpkg.OrbitMemberLaneBootstrap,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"bootstrap/docs/**"},
		},
	})
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, docsSpec)
	require.NoError(t, err)

	members := []harnesspkg.ManifestMember{{
		OrbitID: "docs",
		Source:  harnesspkg.MemberSourceManual,
		AddedAt: now,
	}}

	var bootstrapData []byte
	bootstrapData, err = orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Bootstrap the docs orbit.\n"))
	require.NoError(t, err)

	if options.includeOpsOrbit {
		repo.WriteFile(t, "ops/runbook.md", "Ops runbook\n")
		repo.WriteFile(t, "bootstrap/ops/setup.md", "Ops bootstrap setup\n")

		opsSpec, err := orbitpkg.DefaultHostedMemberSchemaSpec("ops")
		require.NoError(t, err)
		require.NotNil(t, opsSpec.Meta)
		opsSpec.Description = "Ops orbit"
		opsSpec.Meta.BootstrapTemplate = "Bootstrap the ops orbit.\n"
		opsSpec.Members = append(opsSpec.Members, orbitpkg.OrbitMember{
			Key:  "ops-bootstrap",
			Role: orbitpkg.OrbitMemberRule,
			Lane: orbitpkg.OrbitMemberLaneBootstrap,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"bootstrap/ops/**"},
			},
		})
		_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, opsSpec)
		require.NoError(t, err)

		members = append(members, harnesspkg.ManifestMember{
			OrbitID: "ops",
			Source:  harnesspkg.MemberSourceManual,
			AddedAt: now,
		})

		opsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("ops", []byte("Bootstrap the ops orbit.\n"))
		require.NoError(t, err)
		bootstrapData = append(append(bootstrapData, '\n'), opsBlock...)
	}

	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: members,
	})
	require.NoError(t, err)

	repo.WriteFile(t, "BOOTSTRAP.md", string(bootstrapData))
	repo.AddAndCommit(t, "seed harness bootstrap completion repo")

	return repo
}
