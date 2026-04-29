package orbittemplate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestBuildTemplateSavePreviewBuildsManifestAndTemplateTree(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
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
	repo.AddAndCommit(t, "seed runtime repo")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	headCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	now := time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC)

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:      repo.Root,
		OrbitID:       "docs",
		TargetBranch:  "orbit-template/docs",
		DefaultBranch: true,
		Now:           now,
	})
	require.NoError(t, err)

	require.Equal(t, repo.Root, preview.RepoRoot)
	require.Equal(t, "docs", preview.OrbitID)
	require.Equal(t, "orbit-template/docs", preview.TargetBranch)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, preview.FilePaths())
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
	}, preview.ReplacementSummaries)
	require.Empty(t, preview.Ambiguities)

	require.Equal(t, Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   true,
			CreatedFromBranch: currentBranch,
			CreatedFromCommit: headCommit,
			CreatedAt:         now,
		},
		Variables: map[string]VariableSpec{
			"project_name": {
				Description: "Product title",
				Required:    true,
			},
		},
	}, preview.Manifest)
}

func TestTemplateSavePreviewDoesNotExposeLegacyManifestData(t *testing.T) {
	t.Parallel()

	_, ok := reflect.TypeOf(TemplateSavePreview{}).FieldByName("ManifestData")
	require.False(t, ok)
}

func TestBuildTemplateSavePreviewCollectsAmbiguities(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit docs\n")
	repo.AddAndCommit(t, "seed runtime repo")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

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
	}, preview.Ambiguities)
}

func TestBuildTemplateSavePreviewIgnoresNonMarkdownVariableSyntax(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n"+
		"  - schema/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "schema/example.schema.json", "{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"title\": \"$project_name\"\n}\n")
	repo.AddAndCommit(t, "seed runtime repo")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Required: true,
		},
	}, preview.Manifest.Variables)
	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ninclude:\n  - docs/**\n  - schema/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "schema/example.schema.json",
			Content: []byte("{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n  \"title\": \"$project_name\"\n}\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.Files)
}

func TestBuildTemplateSavePreviewKeepsAgentsTemplateInCompanionSpecAndSkipsRootAgentsFile(t *testing.T) {
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
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "static guide\n")
	repo.AddAndCommit(t, "seed runtime repo with brief in hosted spec")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, []string{".harness/orbits/docs.yaml"}, preview.FilePaths())
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
	}, preview.Files)
	require.Empty(t, preview.ReplacementSummaries)
	require.Empty(t, preview.Warnings)
	require.Empty(t, preview.Manifest.SharedFiles)
	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
	}, preview.Manifest.Variables)
}

func TestBuildTemplateSavePreviewSkipsRuntimeGuidanceExportsAndWarns(t *testing.T) {
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
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  humans_template: |\n"+
		"    Run docs workflow for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"content:\n"+
		"  - name: guide-docs\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - AGENTS.md\n"+
		"        - HUMANS.md\n"+
		"        - README.md\n"+
		"behavior:\n"+
		"  scope:\n"+
		"    projection_roles: [meta, subject, rule, process]\n"+
		"    write_roles: [meta, rule]\n"+
		"    export_roles: [meta, rule]\n"+
		"    orchestration_roles: [meta, rule, process]\n"+
		"  orchestration:\n"+
		"    include_orbit_description: true\n"+
		"    materialize_agents_from_meta: true\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "AGENTS.md", "Docs orbit for Orbit\n")
	repo.WriteFile(t, "HUMANS.md", "Run docs workflow for Orbit\n")
	repo.WriteFile(t, "README.md", "Orbit readme\n")
	repo.AddAndCommit(t, "seed runtime repo with guidance artifacts in member export")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"README.md",
	}, preview.FilePaths())
	require.Equal(t, []string{
		"skip runtime guidance export paths for orbit \"docs\": AGENTS.md, HUMANS.md; template publishing uses meta.agents_template/meta.humans_template instead",
	}, preview.Warnings)
}

func TestBuildTemplateSavePreviewIncludesCapabilityAssetsAndKeepsCapabilitiesInCompanionSpec(t *testing.T) {
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
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Docs orbit for $project_name\n"+
		"  humans_template: |\n"+
		"    Run docs workflow for $project_name\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/**/*.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/*\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review $project_name docs.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use $project_name style guide.\n")
	repo.AddAndCommit(t, "seed runtime repo with capability assets")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
		"orbit/commands/review.md",
		"orbit/skills/docs-style/SKILL.md",
		"orbit/skills/docs-style/checklist.md",
	}, preview.FilePaths())
	require.Contains(t, string(preview.Files[0].Content), "capabilities:\n")
	require.Contains(t, string(preview.Files[0].Content), "commands:\n")
	require.Contains(t, string(preview.Files[0].Content), "- orbit/commands/**/*.md\n")
	require.Contains(t, string(preview.Files[0].Content), "skills:\n")
	require.Contains(t, string(preview.Files[0].Content), "- orbit/skills/*\n")
}

func TestBuildTemplateSavePreviewRejectsHostedCapabilitySkillRootMissingSkillMD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/*\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use Orbit style guide.\n")
	repo.AddAndCommit(t, "seed runtime repo with incomplete skill capability")

	_, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `local skill root "orbit/skills/docs-style": SKILL.md must exist and be tracked`)
}

func TestBuildTemplateSavePreviewSkipsEmptySharedAgentsPayload(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - AGENTS.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin orbit_id=\"api\" -->\n"+
		"api only\n"+
		"<!-- orbit:end orbit_id=\"api\" -->\n")
	repo.WriteFile(t, "docs/guide.md", "static guide\n")
	repo.AddAndCommit(t, "seed runtime repo with empty docs payload")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Nil(t, preview.Warnings)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, preview.FilePaths())
	require.Empty(t, preview.Manifest.SharedFiles)
}

func TestBuildTemplateSavePreviewSkipsProjectionVisibleFilesAndAgents(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible:\n"+
		"  - README.md\n"+
		"  - AGENTS.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
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
	repo.WriteFile(t, "README.md", "Orbit readme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs orbit for Orbit\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo with projection-visible shared files")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, preview.FilePaths())
	require.Empty(t, preview.Warnings)
	require.Empty(t, preview.Manifest.SharedFiles)
}

func TestBuildTemplateSavePreviewSkipsProjectionVisibleFilesFromManifestVariables(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
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
	repo.WriteFile(t, "README.md", "$project_name readme\n")
	repo.WriteFile(t, "docs/guide.md", "static guide\n")
	repo.AddAndCommit(t, "seed runtime repo with projection-visible variable file")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, map[string]VariableSpec{}, preview.Manifest.Variables)
}

func TestSaveTemplateBranchFailsClosedOnAmbiguity(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit docs\n")
	repo.AddAndCommit(t, "seed runtime repo")

	_, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "replacement ambiguity")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestSaveTemplateBranchWritesTemplateBranch(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	result, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:      repo.Root,
			OrbitID:       "docs",
			TargetBranch:  "orbit-template/docs",
			DefaultBranch: true,
			Now:           time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", result.WriteResult.Branch)
	require.NotEmpty(t, result.WriteResult.Commit)

	files := strings.Split(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")), "\n")
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)
}

func TestSaveTemplateBranchPreservesExecutableFileMode(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/build.sh", "#!/bin/sh\necho orbit\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "docs", "build.sh"), 0o755))
	repo.AddAndCommit(t, "seed runtime repo with executable")

	_, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	mode := strings.Fields(repo.Run(t, "ls-tree", "orbit-template/docs", "docs/build.sh"))[0]
	require.Equal(t, "100755", mode)
}

func TestBuildTemplateSavePreviewEditTemplateRegeneratesManifestWithoutMutatingRuntimeWorktree(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  service_url:\n"+
		"    value: http://localhost:3000\n"+
		"    description: Service URL\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		EditTemplate: true,
		Editor: editorFunc(func(_ context.Context, dir string) error {
			return os.WriteFile(
				filepath.Join(dir, "docs", "guide.md"),
				[]byte("$project_name guide at $service_url\n"),
				0o600,
			)
		}),
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    ".harness/orbits/docs.yaml",
			Content: []byte("id: docs\ndescription: Docs orbit\ninclude:\n  - docs/**\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide at $service_url\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.Files)
	require.Equal(t, Manifest{
		SchemaVersion: 1,
		Kind:          TemplateKind,
		Template: Metadata{
			OrbitID:           "docs",
			DefaultTemplate:   false,
			CreatedFromBranch: strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD")),
			CreatedFromCommit: strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")),
			CreatedAt:         time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
		Variables: map[string]VariableSpec{
			"project_name": {
				Description: "Product title",
				Required:    true,
			},
			"service_url": {
				Description: "Service URL",
				Required:    true,
			},
		},
	}, preview.Manifest)

	runtimeData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Orbit guide\n", string(runtimeData))
}

func TestBuildTemplateSavePreviewEditTemplateAllowsEditingSharedAgentsPayloadWithoutMutatingRuntimeWorktree(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  service_url:\n"+
		"    value: https://orbit.example\n"+
		"    description: Service URL\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs orbit for Orbit\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.WriteFile(t, "docs/guide.md", "static guide\n")
	repo.AddAndCommit(t, "seed runtime repo with shared agents")

	preview, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		EditTemplate: true,
		Editor: editorFunc(func(_ context.Context, dir string) error {
			return os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Edited $project_name guidance at $service_url\n"), 0o600)
		}),
	})
	require.NoError(t, err)

	require.Contains(t, preview.FilePaths(), "AGENTS.md")
	require.Equal(t, []byte("Edited $project_name guidance at $service_url\n"), preview.Files[1].Content)
	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
		"service_url": {
			Description: "Service URL",
			Required:    true,
		},
	}, preview.Manifest.Variables)

	runtimeData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs orbit for Orbit\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n", string(runtimeData))
}

func TestBuildTemplateSavePreviewEditTemplateFailsWhenDefinitionIsRemoved(t *testing.T) {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	_, err := BuildTemplateSavePreview(context.Background(), TemplateSavePreviewInput{
		RepoRoot:     repo.Root,
		OrbitID:      "docs",
		TargetBranch: "orbit-template/docs",
		Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		EditTemplate: true,
		Editor: editorFunc(func(_ context.Context, dir string) error {
			return os.Remove(filepath.Join(dir, ".harness", "orbits", "docs.yaml"))
		}),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "edited template must keep companion definition")
}

type editorFunc func(ctx context.Context, dir string) error

func (fn editorFunc) Edit(ctx context.Context, dir string) error {
	return fn(ctx, dir)
}

func (fn editorFunc) String() string {
	return fmt.Sprintf("editorFunc(%p)", fn)
}
