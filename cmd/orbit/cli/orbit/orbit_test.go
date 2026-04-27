package orbit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestLoadRepositoryConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/cmd.yaml", "id: cmd\ninclude:\n  - cmd/**\n")

	config, err := orbitpkg.LoadRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)

	require.Equal(t, orbitpkg.DefaultGlobalConfig(), config.Global)
	require.Len(t, config.Orbits, 2)
	require.Equal(t, "cmd", config.Orbits[0].ID)
	require.Equal(t, "docs", config.Orbits[1].ID)
}

func TestLoadHostedRepositoryConfigUsesHarnessHostedDefinitionsWithoutGlobalConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", "id: cmd\ninclude:\n  - cmd/**\n")

	config, err := orbitpkg.LoadHostedRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)

	require.Equal(t, orbitpkg.DefaultGlobalConfig(), config.Global)
	require.Len(t, config.Orbits, 2)
	require.Equal(t, "cmd", config.Orbits[0].ID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"), config.Orbits[0].SourcePath)
	require.Equal(t, "docs", config.Orbits[1].ID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), config.Orbits[1].SourcePath)
}

func TestLoadRuntimeRepositoryConfigUsesHostedDefinitionsWithoutGlobalConfig(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	config, err := orbitpkg.LoadRuntimeRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)

	require.Equal(t, orbitpkg.DefaultGlobalConfig(), config.Global)
	require.Len(t, config.Orbits, 1)
	require.Equal(t, "docs", config.Orbits[0].ID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), config.Orbits[0].SourcePath)
}

func TestLoadOrbitSpecAndProjectionPlanDoesNotInjectLegacyConfigIntoHostedControlPaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "docs/guide.md", "guide\n")

	config, err := orbitpkg.LoadHostedRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)

	trackedFiles := []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}
	loadedSpec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(context.Background(), repo.Root, config, "docs", trackedFiles)
	require.NoError(t, err)

	require.Equal(t, "docs", loadedSpec.ID)
	require.Equal(t, []string{".harness/orbits/docs.yaml"}, plan.ControlPaths)
}

func TestWriteHostedOrbitSpecCanonicalizesLegacyMemberKeysToNameOnly(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	spec := orbitpkg.OrbitSpec{
		ID: "docs",
		Meta: &orbitpkg.OrbitMeta{
			File:                ".harness/orbits/docs.yaml",
			IncludeInProjection: true,
			IncludeInWrite:      true,
			IncludeInExport:     true,
		},
		Members: []orbitpkg.OrbitMember{
			{
				Key:  "docs-rules",
				Role: orbitpkg.OrbitMemberRule,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{".markdownlint.yaml"},
				},
			},
		},
		SourcePath: filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"),
	}

	filename, err := orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.NotContains(t, string(data), "key:")
	require.Contains(t, string(data), "name: docs-rules\n")
}

func TestDefaultGlobalConfigUsesEmptySharedScopeAndProjectionVisible(t *testing.T) {
	t.Parallel()

	config := orbitpkg.DefaultGlobalConfig()
	require.Empty(t, config.SharedScope)
	require.Empty(t, config.ProjectionVisible)
}

func TestValidateRepositoryConfig(t *testing.T) {
	t.Parallel()

	config := orbitpkg.DefaultGlobalConfig()
	config.Behavior.SparseCheckoutMode = "cone"

	definitions := []orbitpkg.Definition{
		{ID: "docs", Include: []string{"docs/**"}, SourcePath: "docs.yaml"},
	}

	err := orbitpkg.ValidateRepositoryConfig(config, definitions)
	require.Error(t, err)

	config = orbitpkg.DefaultGlobalConfig()
	definitions = []orbitpkg.Definition{
		{ID: "docs", Include: []string{"docs/**"}, SourcePath: "docs.yaml"},
		{ID: "docs", Include: []string{"README.md"}, SourcePath: "duplicate.yaml"},
	}

	err = orbitpkg.ValidateRepositoryConfig(config, definitions)
	require.Error(t, err)
}

func TestValidateRepositoryConfigRejectsFilenameMismatch(t *testing.T) {
	t.Parallel()

	config := orbitpkg.DefaultGlobalConfig()
	definitions := []orbitpkg.Definition{
		{ID: "docs", Include: []string{"docs/**"}, SourcePath: "guide.yaml"},
	}

	err := orbitpkg.ValidateRepositoryConfig(config, definitions)
	require.Error(t, err)
	require.ErrorContains(t, err, "definition filename must match orbit id")
}

func TestValidateRepositoryConfigRejectsControlPlaneSharedScopePatterns(t *testing.T) {
	t.Parallel()

	definitions := []orbitpkg.Definition{
		{ID: "docs", Include: []string{"docs/**"}, SourcePath: "docs.yaml"},
	}

	for _, pattern := range []string{
		".orbit/**",
		".orbit/config.yaml",
		".orbit/orbits/cmd.yaml",
		".orbit/orbits/*.yaml",
		"**/.orbit/orbits/*.yaml",
	} {
		config := orbitpkg.DefaultGlobalConfig()
		config.SharedScope = []string{pattern}

		err := orbitpkg.ValidateRepositoryConfig(config, definitions)
		require.Error(t, err)
		require.ErrorContains(t, err, "must not match Orbit control-plane paths")
	}
}

func TestParseGlobalConfigDataReadsSharedScopeAndProjectionVisible(t *testing.T) {
	t.Parallel()

	config, err := orbitpkg.ParseGlobalConfigData([]byte("" +
		"version: 1\n" +
		"shared_scope:\n" +
		"  - LICENSE\n" +
		"projection_visible:\n" +
		"  - README.md\n" +
		"behavior:\n" +
		"  outside_changes_mode: warn\n" +
		"  block_switch_if_hidden_dirty: true\n" +
		"  commit_append_trailer: true\n" +
		"  sparse_checkout_mode: no-cone\n"))
	require.NoError(t, err)
	require.Equal(t, []string{"LICENSE"}, config.SharedScope)
	require.Equal(t, []string{"README.md"}, config.ProjectionVisible)
}

func TestParseGlobalConfigDataRejectsLegacyAlwaysVisibleField(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseGlobalConfigData([]byte("" +
		"version: 1\n" +
		"always_visible:\n" +
		"  - README.md\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "always_visible")
}

func TestParseGlobalConfigDataRejectsUnknownLegacyAlwaysVisibleAlongsideSharedScope(t *testing.T) {
	t.Parallel()

	_, err := orbitpkg.ParseGlobalConfigData([]byte("" +
		"version: 1\n" +
		"always_visible:\n" +
		"  - README.md\n" +
		"shared_scope:\n" +
		"  - LICENSE\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "always_visible")
}

func TestValidateRepositoryConfigRejectsControlPlaneSharedScopeAndProjectionVisiblePatterns(t *testing.T) {
	t.Parallel()

	definitions := []orbitpkg.Definition{
		{ID: "docs", Include: []string{"docs/**"}, SourcePath: "docs.yaml"},
	}

	for _, pattern := range []string{
		".orbit/**",
		".orbit/config.yaml",
		".orbit/template.yaml",
		".orbit/source.yaml",
		".orbit/orbits/*.yaml",
		".harness/**",
		".git/orbit/state/**",
	} {
		config := orbitpkg.DefaultGlobalConfig()
		config.SharedScope = []string{pattern}

		err := orbitpkg.ValidateRepositoryConfig(config, definitions)
		require.Error(t, err)
		require.ErrorContains(t, err, "must not match Orbit control-plane paths")

		config = orbitpkg.DefaultGlobalConfig()
		config.ProjectionVisible = []string{pattern}

		err = orbitpkg.ValidateRepositoryConfig(config, definitions)
		require.Error(t, err)
		require.ErrorContains(t, err, "must not match Orbit control-plane paths")
	}
}

func TestLoadRepositoryConfigPrefersVisibleWorktreeControlFiles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\nshared_scope:\n  - README.md\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ndescription: committed docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "add orbit control plane")

	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ndescription: modified docs\ninclude:\n  - docs/**\n")

	config, err := orbitpkg.LoadRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Len(t, config.Orbits, 1)
	require.Equal(t, "modified docs", config.Orbits[0].Description)
}

func TestLoadRepositoryConfigFallsBackToHEADWhenControlFilesAreHidden(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\nshared_scope:\n  - README.md\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/cmd.yaml", "id: cmd\ninclude:\n  - cmd/**\n")
	repo.WriteFile(t, "README.md", "hello\n")
	repo.AddAndCommit(t, "add orbit control plane")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	_, err := os.Stat(filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	config, err := orbitpkg.LoadRepositoryConfig(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Equal(t, []string{"README.md"}, config.Global.SharedScope)
	require.Len(t, config.Orbits, 2)
	require.Equal(t, "cmd", config.Orbits[0].ID)
	require.Equal(t, "docs", config.Orbits[1].ID)
}

func TestResolveScopeSetBuildsOwnedAndProjectionPaths(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.GlobalConfig{
			Version:           1,
			SharedScope:       []string{"README.md", "shared/owned.md"},
			ProjectionVisible: []string{"README.md", "notes/**", "shared/projection-only.md"},
			Behavior:          orbitpkg.DefaultGlobalConfig().Behavior,
		},
		HasLegacyGlobalConfig: true,
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
			{ID: "cmd", Include: []string{"cmd/**"}},
		},
	}
	definition := orbitpkg.Definition{
		ID:      "docs",
		Include: []string{"docs/**", "README.md"},
		Exclude: []string{"docs/archive/**"},
	}

	trackedFiles := []string{
		"cmd/orbit/main.go",
		"docs/archive/old.md",
		"README.md",
		"notes/todo.md",
		"shared/owned.md",
		"shared/projection-only.md",
		".orbit/config.yaml",
		`docs\guide.md`,
		"docs/tutorial.md",
		".orbit/orbits/docs.yaml",
		".orbit/orbits/cmd.yaml",
	}

	scopeSet, err := orbitpkg.ResolveScopeSet(config, definition, trackedFiles)
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/cmd.yaml",
		".harness/orbits/docs.yaml",
		".orbit/config.yaml",
	}, scopeSet.ControlReadPaths)
	require.Equal(t, []string{
		"README.md",
		"docs/guide.md",
		"docs/tutorial.md",
		"shared/owned.md",
	}, scopeSet.OwnedPaths)
	require.Equal(t, []string{
		"notes/todo.md",
		"shared/projection-only.md",
	}, scopeSet.ProjectionOnlyPaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
	}, scopeSet.CompanionPaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
		"docs/tutorial.md",
		"shared/owned.md",
	}, scopeSet.ScopedOperationPaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
		"docs/tutorial.md",
		"notes/todo.md",
		"shared/owned.md",
		"shared/projection-only.md",
	}, scopeSet.ProjectionPaths)
}

func TestPathMatchesOrbitIgnoresControlPlanePathsAndRespectsExcludePrecedence(t *testing.T) {
	t.Parallel()

	config := orbitpkg.GlobalConfig{
		Version:           1,
		SharedScope:       []string{"README.md", "shared/**"},
		ProjectionVisible: []string{"notes/**", "shared/projection-only.md"},
		Behavior:          orbitpkg.DefaultGlobalConfig().Behavior,
	}
	definition := orbitpkg.Definition{
		ID:      "docs",
		Include: []string{"docs/**", ".orbit/orbits/docs.yaml"},
		Exclude: []string{"README.md", "notes/**", "shared/projection-only.md"},
	}

	match, err := orbitpkg.PathMatchesOrbit(config, definition, ".orbit/config.yaml")
	require.NoError(t, err)
	require.False(t, match)

	match, err = orbitpkg.PathMatchesOrbit(config, definition, ".orbit/orbits/docs.yaml")
	require.NoError(t, err)
	require.False(t, match)

	match, err = orbitpkg.PathMatchesOrbit(config, definition, "README.md")
	require.NoError(t, err)
	require.False(t, match)

	match, err = orbitpkg.PathMatchesOrbit(config, definition, "shared/owned.md")
	require.NoError(t, err)
	require.True(t, match)

	match, err = orbitpkg.PathMatchesOrbit(config, definition, "notes/todo.md")
	require.NoError(t, err)
	require.False(t, match)
}

func TestDiscoverDefinitionsSkipsInvalidFiles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, ".orbit/orbits/bad.yaml", "id: bad\ninclude: [\n")
	repo.WriteFile(t, ".orbit/orbits/extra.yml", "id: extra\ninclude:\n  - extra/**\n")
	repo.WriteFile(t, ".orbit/orbits/invalid.yaml", "id: Docs\ninclude:\n  - docs/**\n")

	definitions, err := orbitpkg.DiscoverDefinitions(context.Background(), repo.Root)
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "docs", definitions[0].ID)
}

func TestSeedDefaultCapabilityTruthPreservesExistingAuthoredCapabilities(t *testing.T) {
	t.Parallel()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)

	spec.Capabilities = &orbitpkg.OrbitCapabilities{
		Commands: &orbitpkg.OrbitCommandCapabilityPaths{
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"commands/custom/**/*.md"},
			},
		},
		Skills: &orbitpkg.OrbitSkillCapabilities{
			Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"skills/custom/*"},
				},
			},
		},
	}

	seeded, err := orbitpkg.SeedDefaultCapabilityTruth(spec)
	require.NoError(t, err)
	require.Equal(t, []string{"commands/custom/**/*.md"}, seeded.Capabilities.Commands.Paths.Include)
	require.Equal(t, []string{"skills/custom/*"}, seeded.Capabilities.Skills.Local.Paths.Include)
}
