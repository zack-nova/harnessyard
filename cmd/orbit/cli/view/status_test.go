package view

import (
	"testing"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestClassifyStatusIncludesInScopeUntracked(t *testing.T) {
	t.Parallel()

	definition := orbitpkg.Definition{
		ID:          "docs",
		Description: "docs orbit",
		Include:     []string{"docs/**"},
	}

	snapshot, err := ClassifyStatus(
		orbitpkg.DefaultGlobalConfig(),
		definition,
		"docs",
		[]string{"README.md", "docs/guide.md", "docs/old.md"},
		[]gitpkg.StatusEntry{
			{Path: "docs/guide.md", Code: "M", Tracked: true},
			{Path: "docs/new.md", Code: "??", Tracked: false},
			{Path: "src/main.go", Code: "M", Tracked: true},
			{Path: "scratch/outside.txt", Code: "??", Tracked: false},
		},
	)
	require.NoError(t, err)
	require.Equal(t, []statepkg.PathChange{
		{Path: "docs/guide.md", Code: "M", Tracked: true, InScope: true},
		{Path: "docs/new.md", Code: "??", Tracked: false, InScope: true},
	}, snapshot.InScope)
	require.Equal(t, []statepkg.PathChange{
		{Path: "src/main.go", Code: "M", Tracked: true, InScope: false},
		{Path: "scratch/outside.txt", Code: "??", Tracked: false, InScope: false},
	}, snapshot.OutOfScope)
	require.Equal(t, []string{"src/main.go"}, snapshot.HiddenDirtyRisk)
	require.False(t, snapshot.SafeToSwitch)
	require.Equal(t, []string{
		"outside changes are present; orbit commit will only include the current orbit scope",
	}, snapshot.CommitWarnings)
}

func TestClassifyStatusForSpecIncludesRoleAndScopeFlags(t *testing.T) {
	t.Parallel()

	global := orbitpkg.DefaultGlobalConfig()
	global.SharedScope = []string{"README.md"}
	global.ProjectionVisible = []string{"docs/process/**"}

	config := orbitpkg.RepositoryConfig{
		Global: global,
		Orbits: []orbitpkg.Definition{
			{
				ID:          "docs",
				Description: "docs orbit",
				Include:     []string{"docs/**"},
				SourcePath:  "/tmp/repo/.orbit/orbits/docs.yaml",
			},
		},
	}
	spec := orbitpkg.OrbitSpec{
		ID:          "docs",
		Description: "docs orbit",
		Meta: &orbitpkg.OrbitMeta{
			File:                              ".orbit/orbits/docs.yaml",
			IncludeInProjection:               true,
			IncludeInWrite:                    true,
			IncludeInExport:                   true,
			IncludeDescriptionInOrchestration: true,
		},
		Members: []orbitpkg.OrbitMember{
			{
				Key:  "docs-content",
				Role: orbitpkg.OrbitMemberSubject,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"docs/**"},
					Exclude: []string{"docs/process/**"},
				},
			},
			{
				Key:  "docs-rules",
				Role: orbitpkg.OrbitMemberRule,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{".markdownlint.yaml"},
				},
			},
			{
				Key:  "docs-process",
				Role: orbitpkg.OrbitMemberProcess,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"docs/process/**"},
				},
			},
		},
		SourcePath: "/tmp/repo/.orbit/orbits/docs.yaml",
	}
	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/orbits/docs.yaml",
		".markdownlint.yaml",
		"README.md",
		"docs/guide.md",
		"docs/process/flow.md",
		"src/main.go",
	})
	require.NoError(t, err)

	snapshot, err := ClassifyStatusForSpec(
		config,
		spec,
		"docs",
		plan,
		[]gitpkg.StatusEntry{
			{Path: ".orbit/orbits/docs.yaml", Code: "M", Tracked: true},
			{Path: ".markdownlint.yaml", Code: "M", Tracked: true},
			{Path: "docs/guide.md", Code: "M", Tracked: true},
			{Path: "docs/process/flow.md", Code: "M", Tracked: true},
			{Path: "README.md", Code: "??", Tracked: false},
			{Path: "docs/process/draft.md", Code: "??", Tracked: false},
			{Path: "scratch/outside.txt", Code: "??", Tracked: false},
			{Path: "src/main.go", Code: "M", Tracked: true},
		},
	)
	require.NoError(t, err)
	require.Equal(t, []statepkg.PathChange{
		{
			Path:          ".markdownlint.yaml",
			Code:          "M",
			Tracked:       true,
			InScope:       true,
			Role:          orbitpkg.PathRoleRule,
			Projection:    true,
			OrbitWrite:    true,
			Export:        true,
			Orchestration: true,
		},
		{
			Path:          ".orbit/orbits/docs.yaml",
			Code:          "M",
			Tracked:       true,
			InScope:       true,
			Role:          orbitpkg.PathRoleMeta,
			Projection:    true,
			OrbitWrite:    true,
			Export:        true,
			Orchestration: true,
		},
		{
			Path:          "README.md",
			Code:          "??",
			Tracked:       false,
			InScope:       true,
			Role:          orbitpkg.PathRoleSubject,
			Projection:    true,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: false,
		},
		{
			Path:          "docs/guide.md",
			Code:          "M",
			Tracked:       true,
			InScope:       true,
			Role:          orbitpkg.PathRoleSubject,
			Projection:    true,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: false,
		},
		{
			Path:          "docs/process/draft.md",
			Code:          "??",
			Tracked:       false,
			InScope:       true,
			Role:          orbitpkg.PathRoleProcess,
			Projection:    true,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: true,
		},
		{
			Path:          "docs/process/flow.md",
			Code:          "M",
			Tracked:       true,
			InScope:       true,
			Role:          orbitpkg.PathRoleProcess,
			Projection:    true,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: true,
		},
	}, snapshot.InScope)
	require.Equal(t, []statepkg.PathChange{
		{
			Path:          "scratch/outside.txt",
			Code:          "??",
			Tracked:       false,
			InScope:       false,
			Role:          orbitpkg.PathRoleOutside,
			Projection:    false,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: false,
		},
		{
			Path:          "src/main.go",
			Code:          "M",
			Tracked:       true,
			InScope:       false,
			Role:          orbitpkg.PathRoleOutside,
			Projection:    false,
			OrbitWrite:    false,
			Export:        false,
			Orchestration: false,
		},
	}, snapshot.OutOfScope)
	require.Equal(t, []string{"src/main.go"}, snapshot.HiddenDirtyRisk)
	require.False(t, snapshot.SafeToSwitch)
	require.Equal(t, []string{
		"outside changes are present; orbit commit will only include the current orbit scope",
	}, snapshot.CommitWarnings)
}
