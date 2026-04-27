package orbittemplate

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildTemplateContentFiltersForbiddenPathsAndAddsCompanionDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Orbit docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/installs/docs.yaml", ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: template/docs\n"+
		"  template_commit: abc123\n"+
		"applied_at: 2026-03-21T10:30:00Z\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n")
	repo.AddAndCommit(t, "seed runtime repo")

	// A bogus cache file should not affect builder output.
	cachePath := filepath.Join(repo.GitDir(t), "orbit", "state", "resolved_scope", "docs.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(cachePath), 0o755))
	require.NoError(t, os.WriteFile(cachePath, []byte("broken-cache"), 0o600))

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			"docs/guide.md",
			".orbit/config.yaml",
			".harness/vars.yaml",
			".harness/installs/docs.yaml",
		},
		Bindings: map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ndescription: Orbit docs orbit\ninclude:\n  - docs/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
	require.Equal(t, []FileReplacementSummary{
		{
			Path: "docs/guide.md",
			Replacements: []ReplacementSummary{
				{
					Variable: "project_name",
					Literal:  "Orbit",
					Count:    1,
				},
			},
		},
	}, result.ReplacementSummaries)
	require.Empty(t, result.Ambiguities)
}

func TestBuildTemplateContentReadsHiddenTrackedFilesFromHEAD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/hidden.md", "Orbit hidden\n")
	repo.WriteFile(t, "README.md", "root\n")
	repo.AddAndCommit(t, "seed tracked files")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	_, err := os.Stat(filepath.Join(repo.Root, "docs", "hidden.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			"docs/hidden.md",
		},
		Bindings: map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/hidden.md",
			Content: []byte("$project_name hidden\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
}

func TestBuildTemplateContentMapsHostedRuntimeDefinitionToTemplateCompanion(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed hosted runtime repo")

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			".harness/orbits/docs.yaml",
			"docs/guide.md",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("package:\n    type: orbit\n    name: docs\ninclude:\n    - docs/**\n\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
}

func TestBuildTemplateContentRewritesHostedMemberSchemaMetaFileToTemplateCompanion(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed hosted member schema runtime repo")

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			".harness/orbits/docs.yaml",
			"docs/guide.md",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path: ".harness/orbits/docs.yaml",
			Content: []byte("" +
				"package:\n" +
				"    type: orbit\n" +
				"    name: docs\n" +
				"description: Docs orbit\n" +
				"meta:\n" +
				"    file: .harness/orbits/docs.yaml\n" +
				"    agents_template: |\n" +
				"        Docs orbit for $project_name\n" +
				"    include_in_projection: true\n" +
				"    include_in_write: true\n" +
				"    include_in_export: true\n" +
				"    include_description_in_orchestration: true\n" +
				"content:\n" +
				"    - name: docs-content\n" +
				"      role: subject\n" +
				"      paths:\n" +
				"        include:\n" +
				"            - docs/**\n\n"),
			Mode: gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
}

func TestBuildTemplateContentCopiesBinaryFilesWithoutReplacement(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - assets/**\n")
	binaryPath := filepath.Join(repo.Root, "assets", "logo.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0o755))
	require.NoError(t, os.WriteFile(binaryPath, []byte{0x00, 'O', 'r', 'b', 'i', 't'}, 0o600))
	repo.AddAndCommit(t, "seed binary asset")

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			"assets/logo.bin",
		},
		Bindings: map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ninclude:\n  - assets/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "assets/logo.bin",
			Content: []byte{0x00, 'O', 'r', 'b', 'i', 't'},
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
	require.Empty(t, result.ReplacementSummaries)
	require.Empty(t, result.Ambiguities)
}

func TestBuildTemplateContentCollectsAmbiguitiesWithoutMutatingText(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", "version: 1\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit docs\n")
	repo.AddAndCommit(t, "seed text file")

	result, err := BuildTemplateContent(context.Background(), BuildInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		UserScope: []string{
			"docs/guide.md",
		},
		Bindings: map[string]bindings.VariableBinding{
			"product_name": {
				Value: "Orbit",
			},
			"project_name": {
				Value: "Orbit",
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("Orbit docs\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, result.Files)
	require.Empty(t, result.ReplacementSummaries)
	require.Equal(t, []FileReplacementAmbiguity{
		{
			Path: "docs/guide.md",
			Ambiguities: []ReplacementAmbiguity{
				{
					Literal:   "Orbit",
					Variables: []string{"product_name", "project_name"},
				},
			},
		},
	}, result.Ambiguities)
}
