package orbit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestBuildFileInventorySnapshotForMemberSchema(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.GlobalConfig{
			Version:           1,
			SharedScope:       []string{"README.md"},
			ProjectionVisible: []string{"docs/process/**"},
			Behavior:          orbitpkg.DefaultGlobalConfig().Behavior,
		},
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
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
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		".markdownlint.yaml",
		"README.md",
		"docs/guide.md",
		"docs/process/flow.md",
	})
	require.NoError(t, err)

	snapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Date(2026, time.April, 5, 15, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 15, 0, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".markdownlint.yaml",
				MemberName:    "docs-rules",
				Role:          orbitpkg.PathRoleRule,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
			{
				Path:          ".orbit/orbits/docs.yaml",
				Role:          orbitpkg.PathRoleMeta,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
			{
				Path:          "README.md",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
			{
				Path:          "docs/guide.md",
				MemberName:    "docs-content",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
			{
				Path:          "docs/process/flow.md",
				MemberName:    "docs-process",
				Role:          orbitpkg.PathRoleProcess,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: true,
			},
		},
	}, snapshot)
}

func TestBuildFileInventorySnapshotForHostedMemberSchema(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.DefaultGlobalConfig(),
		Orbits: []orbitpkg.Definition{
			{
				ID:         "docs",
				Include:    []string{"docs/**"},
				SourcePath: "/tmp/repo/.harness/orbits/docs.yaml",
			},
		},
	}
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
				Key:  "docs-content",
				Role: orbitpkg.OrbitMemberSubject,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"docs/**"},
				},
			},
		},
		SourcePath: "/tmp/repo/.harness/orbits/docs.yaml",
	}

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	})
	require.NoError(t, err)

	snapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Date(2026, time.April, 5, 15, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 15, 30, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".harness/orbits/docs.yaml",
				Role:          orbitpkg.PathRoleMeta,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "docs/guide.md",
				MemberName:    "docs-content",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
		},
	}, snapshot)
}

func TestBuildFileInventorySnapshotForLegacySchema(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.GlobalConfig{
			Version:           1,
			SharedScope:       []string{"README.md"},
			ProjectionVisible: []string{"notes/**"},
			Behavior:          orbitpkg.DefaultGlobalConfig().Behavior,
		},
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
		},
	}
	spec := orbitpkg.OrbitSpecFromDefinition(orbitpkg.Definition{
		ID:         "docs",
		Include:    []string{"docs/**"},
		Exclude:    []string{"docs/archive/**"},
		SourcePath: "/tmp/repo/.orbit/orbits/docs.yaml",
	})

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
		"notes/todo.md",
	})
	require.NoError(t, err)

	snapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Date(2026, time.April, 5, 16, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 16, 0, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".orbit/orbits/docs.yaml",
				Role:          orbitpkg.PathRoleMeta,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: true,
			},
			{
				Path:          "README.md",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "docs/guide.md",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "notes/todo.md",
				Role:          orbitpkg.PathRoleProcess,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
		},
	}, snapshot)
}

func TestBuildFileInventorySnapshotIncludesCapabilityOverlayPaths(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.DefaultGlobalConfig(),
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
		},
	}
	spec := orbitpkg.OrbitSpec{
		ID: "docs",
		Meta: &orbitpkg.OrbitMeta{
			File:                ".orbit/orbits/docs.yaml",
			IncludeInProjection: true,
			IncludeInWrite:      true,
			IncludeInExport:     true,
		},
		Capabilities: &orbitpkg.OrbitCapabilities{
			Commands: &orbitpkg.OrbitCommandCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"orbit/commands/**/*.md"},
				},
			},
			Skills: &orbitpkg.OrbitSkillCapabilities{
				Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
					Paths: orbitpkg.OrbitMemberPaths{
						Include: []string{"orbit/skills/*"},
					},
				},
			},
		},
		Members: []orbitpkg.OrbitMember{
			{
				Key:  "docs-content",
				Role: orbitpkg.OrbitMemberSubject,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"docs/**"},
				},
			},
		},
		SourcePath: "/tmp/repo/.orbit/orbits/docs.yaml",
	}

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	})
	require.NoError(t, err)

	snapshot, err := orbitpkg.BuildFileInventorySnapshot(config, spec, plan, time.Date(2026, time.April, 5, 16, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, statepkg.FileInventorySnapshot{
		Orbit:       "docs",
		GeneratedAt: time.Date(2026, time.April, 5, 16, 30, 0, 0, time.UTC),
		Files: []statepkg.FileInventoryEntry{
			{
				Path:          ".orbit/orbits/docs.yaml",
				Role:          orbitpkg.PathRoleMeta,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "docs/guide.md",
				MemberName:    "docs-content",
				Role:          orbitpkg.PathRoleSubject,
				Projection:    true,
				OrbitWrite:    false,
				Export:        false,
				Orchestration: false,
			},
			{
				Path:          "orbit/commands/review.md",
				Role:          orbitpkg.PathRoleCapability,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "orbit/skills/docs-style/SKILL.md",
				Role:          orbitpkg.PathRoleCapability,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
			{
				Path:          "orbit/skills/docs-style/checklist.md",
				Role:          orbitpkg.PathRoleCapability,
				Projection:    true,
				OrbitWrite:    true,
				Export:        true,
				Orchestration: false,
			},
		},
	}, snapshot)
}
