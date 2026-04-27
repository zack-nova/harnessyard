package orbit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

func TestResolveCommandCapabilitiesDerivesNamesAndRejectsCollisions(t *testing.T) {
	t.Parallel()

	spec := orbitpkg.OrbitSpec{
		ID: "execute",
		Capabilities: &orbitpkg.OrbitCapabilities{
			Commands: &orbitpkg.OrbitCommandCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"commands/execute/**/*.md"},
				},
			},
		},
	}

	_, err := orbitpkg.ResolveCommandCapabilities(spec, []string{
		"commands/execute/review.md",
		"commands/execute/frontend/review.md",
	}, []string{
		"commands/execute/review.md",
		"commands/execute/frontend/review.md",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `resolved command name "review" is declared by multiple files`)
}

func TestResolveCommandCapabilitiesRejectsNonMarkdownAssets(t *testing.T) {
	t.Parallel()

	spec := orbitpkg.OrbitSpec{
		ID: "execute",
		Capabilities: &orbitpkg.OrbitCapabilities{
			Commands: &orbitpkg.OrbitCommandCapabilityPaths{
				Paths: orbitpkg.OrbitMemberPaths{
					Include: []string{"commands/execute/*"},
				},
			},
		},
	}

	_, err := orbitpkg.ResolveCommandCapabilities(spec, []string{
		"commands/execute/review.txt",
	}, []string{
		"commands/execute/review.txt",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `command path "commands/execute/review.txt" must end with ".md"`)
}

func TestResolveLocalSkillCapabilitiesResolvesValidSkillRoots(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeCapabilityTestFile(t, repoRoot, "skills/execute/frontend-test-lab/SKILL.md", ""+
		"---\n"+
		"name: frontend-test-lab\n"+
		"description: Fast frontend validation workflow\n"+
		"---\n"+
		"# Frontend Test Lab\n")
	writeCapabilityTestFile(t, repoRoot, "skills/execute/frontend-test-lab/checklist.md", "ship it\n")

	spec := orbitpkg.OrbitSpec{
		ID: "execute",
		Capabilities: &orbitpkg.OrbitCapabilities{
			Skills: &orbitpkg.OrbitSkillCapabilities{
				Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
					Paths: orbitpkg.OrbitMemberPaths{
						Include: []string{"skills/execute/*"},
					},
				},
			},
		},
	}

	resolved, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, []string{
		"skills/execute/frontend-test-lab/SKILL.md",
		"skills/execute/frontend-test-lab/checklist.md",
	}, []string{
		"skills/execute/frontend-test-lab/SKILL.md",
		"skills/execute/frontend-test-lab/checklist.md",
	})
	require.NoError(t, err)
	require.Equal(t, []orbitpkg.ResolvedLocalSkillCapability{{
		Name:        "frontend-test-lab",
		RootPath:    "skills/execute/frontend-test-lab",
		SkillMDPath: "skills/execute/frontend-test-lab/SKILL.md",
	}}, resolved)
}

func TestResolveLocalSkillCapabilitiesRejectsMissingSkillFrontmatter(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeCapabilityTestFile(t, repoRoot, "skills/execute/frontend-test-lab/SKILL.md", "# Missing frontmatter\n")

	spec := orbitpkg.OrbitSpec{
		ID: "execute",
		Capabilities: &orbitpkg.OrbitCapabilities{
			Skills: &orbitpkg.OrbitSkillCapabilities{
				Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
					Paths: orbitpkg.OrbitMemberPaths{
						Include: []string{"skills/execute/*"},
					},
				},
			},
		},
	}

	_, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, []string{
		"skills/execute/frontend-test-lab/SKILL.md",
	}, []string{
		"skills/execute/frontend-test-lab/SKILL.md",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `local skill root "skills/execute/frontend-test-lab": SKILL.md must start with YAML frontmatter`)
}

func TestResolveRemoteSkillCapabilitiesAcceptsSupportedSchemes(t *testing.T) {
	t.Parallel()

	spec := orbitpkg.OrbitSpec{
		ID: "execute",
		Capabilities: &orbitpkg.OrbitCapabilities{
			Skills: &orbitpkg.OrbitSkillCapabilities{
				Remote: &orbitpkg.OrbitRemoteSkillCapabilities{
					URIs: []string{
						"github://acme/frontend-remote-skill",
						"https://example.com/skills/research-playbook",
					},
				},
			},
		},
	}

	resolved, err := orbitpkg.ResolveRemoteSkillCapabilities(spec)
	require.NoError(t, err)
	require.Equal(t, []orbitpkg.ResolvedRemoteSkillCapability{
		{URI: "github://acme/frontend-remote-skill"},
		{URI: "https://example.com/skills/research-playbook"},
	}, resolved)
}

func writeCapabilityTestFile(t *testing.T, repoRoot string, repoPath string, content string) {
	t.Helper()

	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}
