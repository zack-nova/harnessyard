package orbit_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestResolveProjectionPlanBuildsRoleAwarePathsForMemberSchema(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global:                orbitpkg.DefaultGlobalConfig(),
		HasLegacyGlobalConfig: true,
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
			{ID: "cmd", Include: []string{"cmd/**"}},
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
		SourcePath: "docs.yaml",
	}

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		".orbit/orbits/cmd.yaml",
		".markdownlint.yaml",
		"docs/guide.md",
		"docs/process/review.md",
		"cmd/orbit/main.go",
	})
	require.NoError(t, err)

	require.Equal(t, []string{
		".harness/orbits/cmd.yaml",
		".harness/orbits/docs.yaml",
		".orbit/config.yaml",
	}, plan.ControlPaths)
	require.Equal(t, []string{".orbit/orbits/docs.yaml"}, plan.MetaPaths)
	require.Equal(t, []string{"docs/guide.md"}, plan.SubjectPaths)
	require.Equal(t, []string{".markdownlint.yaml"}, plan.RulePaths)
	require.Equal(t, []string{"docs/process/review.md"}, plan.ProcessPaths)
	require.Equal(t, []string{
		".markdownlint.yaml",
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
		"docs/process/review.md",
	}, plan.ProjectionPaths)
	require.Equal(t, []string{
		".markdownlint.yaml",
		".orbit/orbits/docs.yaml",
	}, plan.OrbitWritePaths)
	require.Equal(t, []string{
		".markdownlint.yaml",
		".orbit/orbits/docs.yaml",
	}, plan.ExportPaths)
	require.Equal(t, []string{
		".markdownlint.yaml",
		".orbit/orbits/docs.yaml",
		"docs/process/review.md",
	}, plan.OrchestrationPaths)
}

func TestResolveProjectionPlanAcceptsHostedMemberSchemaSpec(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.DefaultGlobalConfig(),
		Orbits: []orbitpkg.Definition{
			{
				ID:         "docs",
				Include:    []string{"docs/**"},
				SourcePath: ".harness/orbits/docs.yaml",
			},
		},
	}
	spec := orbitpkg.OrbitSpec{
		ID:          "docs",
		Description: "docs orbit",
		Meta: &orbitpkg.OrbitMeta{
			File:                              ".harness/orbits/docs.yaml",
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
				},
			},
			{
				Key:  "docs-rules",
				Role: orbitpkg.OrbitMemberRule,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{".markdownlint.yaml"},
				},
			},
		},
		SourcePath: ".harness/orbits/docs.yaml",
	}

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".harness/orbits/docs.yaml",
		".markdownlint.yaml",
		"docs/guide.md",
	})
	require.NoError(t, err)
	require.Equal(t, []string{".harness/orbits/docs.yaml"}, plan.MetaPaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		".markdownlint.yaml",
		"docs/guide.md",
	}, plan.ProjectionPaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		".markdownlint.yaml",
	}, plan.OrbitWritePaths)
}

func TestResolveProjectionPlanHonorsMemberScopeOverrides(t *testing.T) {
	t.Parallel()

	config := orbitpkg.RepositoryConfig{
		Global: orbitpkg.DefaultGlobalConfig(),
		Orbits: []orbitpkg.Definition{
			{ID: "docs", Include: []string{"docs/**"}},
		},
	}

	writeTrue := true
	exportTrue := true
	spec := orbitpkg.OrbitSpec{
		ID: "docs",
		Meta: &orbitpkg.OrbitMeta{
			File:                ".orbit/orbits/docs.yaml",
			IncludeInProjection: true,
			IncludeInWrite:      true,
			IncludeInExport:     true,
		},
		Members: []orbitpkg.OrbitMember{
			{
				Key:  "docs-process",
				Role: orbitpkg.OrbitMemberProcess,
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"docs/process/**"},
				},
				Scopes: &orbitpkg.OrbitMemberScopePatch{
					Write:  &writeTrue,
					Export: &exportTrue,
				},
			},
		},
		SourcePath: "docs.yaml",
	}

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		"docs/process/review.md",
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		"docs/process/review.md",
	}, plan.OrbitWritePaths)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		"docs/process/review.md",
	}, plan.ExportPaths)
}

func TestResolveProjectionPlanInjectsCapabilityOverlayIntoProjectionWriteAndExport(t *testing.T) {
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
		SourcePath: "docs.yaml",
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
	require.Equal(t, []string{
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	}, plan.CapabilityPaths)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	}, plan.ProjectionPaths)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	}, plan.OrbitWritePaths)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	}, plan.ExportPaths)
	require.Empty(t, plan.OrchestrationPaths)
}

func TestResolveProjectionPlanBuildsLegacyCompatibilityPaths(t *testing.T) {
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
			{ID: "cmd", Include: []string{"cmd/**"}},
		},
	}
	spec := orbitpkg.OrbitSpecFromDefinition(orbitpkg.Definition{
		ID:         "docs",
		Include:    []string{"docs/**"},
		Exclude:    []string{"docs/archive/**"},
		SourcePath: "docs.yaml",
	})

	plan, err := orbitpkg.ResolveProjectionPlan(config, spec, []string{
		".orbit/config.yaml",
		".orbit/orbits/docs.yaml",
		".orbit/orbits/cmd.yaml",
		"README.md",
		"docs/archive/old.md",
		"docs/guide.md",
		"notes/todo.md",
	})
	require.NoError(t, err)

	require.Equal(t, []string{".harness/orbits/docs.yaml"}, plan.MetaPaths)
	require.Equal(t, []string{"README.md", "docs/guide.md"}, plan.SubjectPaths)
	require.Equal(t, []string{"notes/todo.md"}, plan.ProcessPaths)
	require.Empty(t, plan.RulePaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
	}, plan.OrbitWritePaths)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
		"docs/guide.md",
		"notes/todo.md",
	}, plan.ProjectionPaths)
}

func TestScopeSetFromProjectionPlanUsesCompatibilityMapping(t *testing.T) {
	t.Parallel()

	plan := orbitpkg.ProjectionPlan{
		ControlPaths:       []string{".orbit/config.yaml", ".orbit/orbits/docs.yaml"},
		MetaPaths:          []string{".orbit/orbits/docs.yaml"},
		SubjectPaths:       []string{"docs/guide.md"},
		RulePaths:          []string{".markdownlint.yaml"},
		ProcessPaths:       []string{"docs/process/review.md"},
		ProjectionPaths:    []string{".orbit/orbits/docs.yaml", ".markdownlint.yaml", "docs/guide.md", "docs/process/review.md"},
		OrbitWritePaths:    []string{".orbit/orbits/docs.yaml", ".markdownlint.yaml"},
		ExportPaths:        []string{".orbit/orbits/docs.yaml", ".markdownlint.yaml"},
		OrchestrationPaths: []string{".orbit/orbits/docs.yaml", ".markdownlint.yaml", "docs/process/review.md"},
	}

	scopeSet := orbitpkg.ScopeSetFromProjectionPlan(plan)
	require.Equal(t, []string{".orbit/config.yaml", ".orbit/orbits/docs.yaml"}, scopeSet.ControlReadPaths)
	require.Equal(t, []string{".markdownlint.yaml", "docs/guide.md"}, scopeSet.OwnedPaths)
	require.Equal(t, []string{"docs/process/review.md"}, scopeSet.ProjectionOnlyPaths)
	require.Equal(t, []string{".orbit/orbits/docs.yaml"}, scopeSet.CompanionPaths)
	require.Equal(t, []string{
		".markdownlint.yaml",
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
	}, scopeSet.ScopedOperationPaths)
	require.Equal(t, []string{
		".orbit/orbits/docs.yaml",
		".markdownlint.yaml",
		"docs/guide.md",
		"docs/process/review.md",
	}, scopeSet.ProjectionPaths)
}
