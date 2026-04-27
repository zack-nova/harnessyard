package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestWriteTemplateBranchCreatesTargetBranchWithoutMutatingCurrentWorktree(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	result, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:       "orbit-template/docs",
		Message:      "save docs template",
		ManifestPath: ".harness/manifest.yaml",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: orbit_template\n" +
			"template:\n" +
			"  orbit_id: docs\n" +
			"  default_template: false\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-21T10:00:00Z\n" +
			"variables:\n" +
			"  project_name:\n" +
			"    required: true\n"),
		Files: []TemplateTreeFile{
			{
				Path:    ".orbit/orbits/docs.yaml",
				Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
			},
			{
				Path:    "docs/guide.md",
				Content: []byte("$project_name guide\n"),
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "refs/heads/orbit-template/docs", result.Ref)
	require.NotEmpty(t, result.Commit)

	require.Equal(t, currentBranch, strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD")))

	readmeData, err := os.ReadFile(filepath.Join(repo.Root, "README.md"))
	require.NoError(t, err)
	require.Equal(t, "runtime readme\n", string(readmeData))

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "template.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	files := nonEmptyLines(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs"))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
	}, files)

	guideData, err := ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))

	manifestData, err := ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template")
	require.Contains(t, string(manifestData), "orbit_id: docs")
}

func TestWriteTemplateBranchFailsClosedWhenTargetBranchExistsWithoutOverwrite(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")
	repo.Run(t, "branch", "orbit-template/docs")

	previousCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs"))

	_, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:       "orbit-template/docs",
		Message:      "save docs template",
		ManifestPath: ".harness/manifest.yaml",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: orbit_template\n" +
			"template:\n" +
			"  orbit_id: docs\n" +
			"  default_template: false\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-21T10:00:00Z\n" +
			"variables: {}\n"),
		Files: []TemplateTreeFile{
			{
				Path:    "docs/guide.md",
				Content: []byte("guide\n"),
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "target branch \"orbit-template/docs\" already exists")
	var existsErr *TemplateTargetBranchExistsError
	require.ErrorAs(t, err, &existsErr)
	require.Equal(t, "orbit-template/docs", existsErr.Branch)
	require.Equal(t, previousCommit, strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs")))
}

func TestWriteTemplateBranchAllowsExplicitOverwrite(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")
	repo.Run(t, "branch", "orbit-template/docs")

	previousCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs"))

	result, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:       "orbit-template/docs",
		Overwrite:    true,
		Message:      "save docs template",
		ManifestPath: ".harness/manifest.yaml",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: orbit_template\n" +
			"template:\n" +
			"  orbit_id: docs\n" +
			"  default_template: true\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-21T10:00:00Z\n" +
			"variables: {}\n"),
		Files: []TemplateTreeFile{
			{
				Path:    ".orbit/orbits/docs.yaml",
				Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
			},
			{
				Path:    "docs/guide.md",
				Content: []byte("templated\n"),
			},
		},
	})
	require.NoError(t, err)
	require.NotEqual(t, previousCommit, result.Commit)
	require.Equal(t, result.Commit, strings.TrimSpace(repo.Run(t, "rev-parse", "orbit-template/docs")))

	files := nonEmptyLines(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs"))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".orbit/orbits/docs.yaml",
		"docs/guide.md",
	}, files)
}

func TestWriteTemplateBranchRejectsCurrentBranchAsTarget(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	_, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:       currentBranch,
		Message:      "save docs template",
		ManifestPath: ".harness/manifest.yaml",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: orbit_template\n" +
			"template:\n" +
			"  orbit_id: docs\n" +
			"  default_template: false\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-21T10:00:00Z\n" +
			"variables: {}\n"),
		Files: []TemplateTreeFile{
			{
				Path:    "docs/guide.md",
				Content: []byte("guide\n"),
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "target branch must not be the current branch")
}

func TestWriteTemplateBranchRequiresExplicitManifestPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")

	_, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:  "orbit-template/docs",
		Message: "save docs template",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: orbit_template\n" +
			"template:\n" +
			"  orbit_id: docs\n" +
			"  default_template: false\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-21T10:00:00Z\n"),
		Files: []TemplateTreeFile{
			{
				Path:    "docs/guide.md",
				Content: []byte("guide\n"),
			},
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "template branch manifest path must not be empty")
}

func TestWriteTemplateBranchSupportsExplicitManifestPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "runtime readme\n")
	repo.AddAndCommit(t, "seed runtime repo")

	result, err := WriteTemplateBranch(context.Background(), repo.Root, WriteTemplateBranchInput{
		Branch:       "harness-template/workspace",
		Message:      "save harness template",
		ManifestPath: ".harness/template.yaml",
		Manifest: []byte("" +
			"schema_version: 1\n" +
			"kind: harness_template\n" +
			"template:\n" +
			"  harness_id: workspace\n" +
			"  default_template: false\n" +
			"  created_from_branch: main\n" +
			"  created_from_commit: abc123\n" +
			"  created_at: 2026-03-26T10:00:00Z\n" +
			"  includes_root_agents: false\n" +
			"members: []\n" +
			"variables: {}\n"),
		Files: []TemplateTreeFile{
			{
				Path:    ".orbit/orbits/docs.yaml",
				Content: []byte("id: docs\ninclude:\n  - docs/**\n"),
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Commit)

	files := nonEmptyLines(repo.Run(t, "ls-tree", "-r", "--name-only", "harness-template/workspace"))
	require.Equal(t, []string{
		".harness/template.yaml",
		".orbit/orbits/docs.yaml",
	}, files)

	_, err = ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".orbit/template.yaml")
	require.Error(t, err)

	manifestData, err := ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/template.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: harness_template")
	require.Contains(t, string(manifestData), "harness_id: workspace")
}

func nonEmptyLines(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}
