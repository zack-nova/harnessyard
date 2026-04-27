package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildTemplateMemberCandidateBuildsFilesAndVariableSpecs(t *testing.T) {
	t.Parallel()

	for _, source := range []string{MemberSourceManual, MemberSourceInstallOrbit} {
		source := source
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			repo := seedTemplateCandidateRepo(t)

			candidate, err := BuildTemplateMemberCandidate(context.Background(), repo.Root, RuntimeMember{
				OrbitID: "docs",
				Source:  source,
				AddedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
			})
			require.NoError(t, err)

			require.Equal(t, "docs", candidate.OrbitID)
			require.Equal(t, []string{
				".harness/orbits/docs.yaml",
				"docs/guide.md",
			}, candidate.FilePaths())
			require.Equal(t, []orbittemplate.CandidateFile{
				{
					Path:    ".harness/orbits/docs.yaml",
					Content: []byte("package:\n    type: orbit\n    name: docs\ndescription: Docs orbit\ninclude:\n    - docs/**\n\n"),
					Mode:    gitpkg.FileModeRegular,
				},
				{
					Path:    "docs/guide.md",
					Content: []byte("$project_name guide\n"),
					Mode:    gitpkg.FileModeRegular,
				},
			}, candidate.Files)
			require.Equal(t, map[string]TemplateVariableSpec{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
			}, candidate.Variables)
			require.Equal(t, []orbittemplate.FileReplacementSummary{
				{
					Path: "docs/guide.md",
					Replacements: []orbittemplate.ReplacementSummary{
						{
							Variable: "project_name",
							Literal:  "Orbit",
							Count:    1,
						},
					},
				},
			}, candidate.ReplacementSummaries)
			require.Empty(t, candidate.Ambiguities)
		})
	}
}

func TestBuildTemplateMemberCandidateSkipsProjectionVisibleFiles(t *testing.T) {
	t.Parallel()

	repo := seedTemplateCandidateRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible:\n"+
		"  - README.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, "README.md", "Orbit readme\n")
	repo.AddAndCommit(t, "add projection-visible readme")

	candidate, err := BuildTemplateMemberCandidate(context.Background(), repo.Root, RuntimeMember{
		OrbitID: "docs",
		Source:  MemberSourceManual,
		AddedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, candidate.FilePaths())
}

func TestBuildTemplateMemberCandidateUsesExportSurfaceForProcessOverrides(t *testing.T) {
	t.Parallel()

	repo := seedTemplateCandidateRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"      exclude:\n"+
		"        - docs/process/**\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n"+
		"    scopes:\n"+
		"      export: true\n")
	repo.WriteFile(t, "docs/process/flow.md", "Exported process flow\n")
	repo.AddAndCommit(t, "enable export override for process member")

	candidate, err := BuildTemplateMemberCandidate(context.Background(), repo.Root, RuntimeMember{
		OrbitID: "docs",
		Source:  MemberSourceManual,
		AddedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/process/flow.md",
	}, candidate.FilePaths())
}

func TestBuildTemplateMemberCandidateIgnoresHostedCompanionVariables(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: $control_only docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/guide.md", "Plain guide\n")
	repo.AddAndCommit(t, "seed runtime repo with hosted companion variable")

	candidate, err := BuildTemplateMemberCandidate(context.Background(), repo.Root, RuntimeMember{
		OrbitID: "docs",
		Source:  MemberSourceManual,
		AddedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, candidate.FilePaths())
	require.Empty(t, candidate.Variables)
}

func TestBuildTemplateMemberCandidateFailsWhenDefinitionMissing(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "seed runtime repo without target definition")

	_, err := BuildTemplateMemberCandidate(context.Background(), repo.Root, RuntimeMember{
		OrbitID: "docs",
		Source:  MemberSourceManual,
		AddedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit "docs" not found`)
}

func seedTemplateCandidateRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.WriteFile(t, "AGENTS.md", "global Orbit guidance\n")
	repo.AddAndCommit(t, "seed runtime repo for harness template candidate")

	return repo
}
