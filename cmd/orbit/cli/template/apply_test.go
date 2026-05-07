package orbittemplate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func writeTestOrbitTemplateBranchManifest(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
}

func TestResolveLocalTemplateSourceLoadsManifestDefinitionAndUserFiles(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, sourceRef)
	require.NoError(t, err)
	require.Equal(t, sourceRef, source.Ref)
	require.Equal(t, strings.TrimSpace(repo.Run(t, "rev-parse", sourceRef)), source.Commit)
	require.Equal(t, "docs", source.Manifest.Template.OrbitID)
	require.Equal(t, "docs", source.Definition.ID)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
}

func TestResolveLocalTemplateSourceLoadsVariablesFromBranchManifestWithoutLegacyTemplateManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    description: Product title\n"+
		"    required: true\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed branch-manifest-only template branch")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
	}, source.Manifest.Variables)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
}

func TestResolveLocalTemplateSourceLoadsAgentsTemplateFromCompanionSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	writeTestOrbitTemplateBranchManifest(t, repo)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Project guidance for $project_name\n"+
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
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed template branch with structured brief")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
	require.NotNil(t, source.Spec.Meta)
	require.Equal(t, "Project guidance for $project_name\n", source.Spec.Meta.AgentsTemplate)
}

func TestResolveLocalTemplateSourceLoadsCapabilitiesAndCapabilityAssetsFromHostedCompanionSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	writeTestOrbitTemplateBranchManifest(t, repo)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Project guidance for $project_name\n"+
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
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review $project_name docs.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use $project_name style guide.\n")
	repo.AddAndCommit(t, "seed template branch with capabilities")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.NotNil(t, source.Spec.Meta)
	require.NotNil(t, source.Spec.Capabilities)
	require.NotNil(t, source.Spec.Capabilities.Commands)
	require.Equal(t, orbitpkg.OrbitMemberPaths{
		Include: []string{"orbit/commands/**/*.md"},
	}, source.Spec.Capabilities.Commands.Paths)
	require.NotNil(t, source.Spec.Capabilities.Skills)
	require.NotNil(t, source.Spec.Capabilities.Skills.Local)
	require.Equal(t, orbitpkg.OrbitMemberPaths{
		Include: []string{"orbit/skills/*"},
	}, source.Spec.Capabilities.Skills.Local.Paths)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "orbit/commands/review.md",
			Content: []byte("Review $project_name docs.\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "orbit/skills/docs-style/SKILL.md",
			Content: []byte("---\nname: docs-style\ndescription: Docs style references.\n---\n# Docs Style\n"),
			Mode:    gitpkg.FileModeRegular,
		},
		{
			Path:    "orbit/skills/docs-style/checklist.md",
			Content: []byte("Use $project_name style guide.\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
}

func TestResolveLocalTemplateSourceRejectsHostedCapabilitySkillRootMissingSkillMD(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	writeTestOrbitTemplateBranchManifest(t, repo)
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
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use $project_name style guide.\n")
	repo.AddAndCommit(t, "seed template branch with incomplete skill capability")

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, `local skill root "orbit/skills/docs-style": SKILL.md must exist and be tracked`)
}

func TestResolveLocalTemplateSourcePrefersHostedCompanionOverLegacyCompanion(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	writeTestOrbitTemplateBranchManifest(t, repo)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Hosted docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Hosted guidance for $project_name\n"+
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Legacy docs orbit\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    Legacy guidance for $project_name\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed dual-host template branch")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.NotNil(t, source.Spec.Meta)
	require.Equal(t, ".harness/orbits/docs.yaml", source.Spec.Meta.File)
	require.Equal(t, "Hosted guidance for $project_name\n", source.Spec.Meta.AgentsTemplate)
}

func TestResolveLocalTemplateSourceRejectsNonTemplateBranchManifest(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	repo.Run(t, "checkout", sourceRef)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"  updated_at: 2026-03-21T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "corrupt template branch manifest kind")
	repo.Run(t, "checkout", currentBranch)

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, sourceRef)
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
	require.ErrorContains(t, err, "orbit_template")
}

func TestResolveLocalTemplateSourceRejectsLegacyAgentsPayloadFile(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	writeTestOrbitTemplateBranchManifest(t, repo)
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "AGENTS.md", "project guidance\n")
	repo.AddAndCommit(t, "seed invalid template branch")

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.Error(t, err)
	require.ErrorContains(t, err, "contains unsupported legacy AGENTS.md payload")
}

func TestResolveLocalTemplateSourceIgnoresInvalidLegacyTemplateManifestWhenBranchManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	repo.WriteFile(t, ".orbit/template.yaml", "schema_version: nope\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed template branch with invalid legacy manifest")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Required: true,
		},
	}, source.Manifest.Variables)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
}

func TestResolveLocalTemplateSourceIgnoresLegacySharedFilesManifestEntryWhenBranchManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n"+
		"shared_files:\n"+
		"  - path: AGENTS.md\n"+
		"    kind: agents_fragment\n"+
		"    merge_mode: replace-block\n"+
		"    include_unmarked_content: true\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed legacy shared files template")

	source, err := ResolveLocalTemplateSource(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, map[string]VariableSpec{
		"project_name": {
			Required: true,
		},
	}, source.Manifest.Variables)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, source.Files)
}

func TestRenderTemplateFilesRequiresBindingsForReferencedVariables(t *testing.T) {
	t.Parallel()

	_, err := RenderTemplateFiles([]CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
		},
	}, map[string]string{})
	require.Error(t, err)
	require.ErrorContains(t, err, "missing binding")
	require.ErrorContains(t, err, "project_name")
}

func TestRenderTemplateFilesIgnoresNonMarkdownFiles(t *testing.T) {
	t.Parallel()

	rendered, err := RenderTemplateFiles([]CandidateFile{
		{
			Path:    "schema/example.schema.json",
			Content: []byte("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"title\":\"$project_name\"}\n"),
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
		},
	}, map[string]string{
		"project_name": "Orbit",
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "schema/example.schema.json",
			Content: []byte("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"title\":\"$project_name\"}\n"),
		},
		{
			Path:    "docs/guide.md",
			Content: []byte("Orbit guide\n"),
		},
	}, rendered)
}

func TestBuildTemplateApplyPreviewReusesRepoVarsAndCollectsWrites(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Local Orbit\n"+
		"    description: Local title\n")
	repo.AddAndCommit(t, "add repo vars")

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, sourceRef, preview.Source.Ref)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Local Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, map[string]bindings.ResolvedBinding{
		"project_name": {
			Value:       "Local Orbit",
			Description: "Product title",
			Required:    true,
			Source:      bindings.SourceRepoVars,
		},
	}, preview.ResolvedBindings)
	require.Nil(t, preview.VarsFile)
	require.Empty(t, preview.Conflicts)
	require.Equal(t, InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      sourceRef,
			TemplateCommit: strings.TrimSpace(repo.Run(t, "rev-parse", sourceRef)),
		},
		AppliedAt: time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
		Variables: &InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Local Orbit",
					Description: "Product title",
				},
			},
		},
	}, preview.InstallRecord)
}

func TestBuildTemplateApplyPreviewSnapshotsEmptyVariablesContract(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepoWithoutVariables(t)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, preview.InstallRecord.Variables)
	require.Equal(t, &InstallVariablesSnapshot{
		Declarations:    map[string]bindings.VariableDeclaration{},
		ResolvedAtApply: map[string]bindings.VariableBinding{},
	}, preview.InstallRecord.Variables)

	filename, err := WriteInstallRecord(repo.Root, preview.InstallRecord)
	require.NoError(t, err)
	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Contains(t, string(data), ""+
		"variables:\n"+
		"    declarations: {}\n"+
		"    resolved_at_apply: {}\n"+
		"    unresolved_at_apply: []\n"+
		"    observed_runtime_unresolved: []\n")
}

func TestBuildTemplateApplyPreviewPrefersScopedRepoVars(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Global Orbit\n"+
		"    description: Global title\n"+
		"scoped_variables:\n"+
		"  docs:\n"+
		"    variables:\n"+
		"      project_name:\n"+
		"        value: Docs Orbit\n"+
		"        description: Docs title\n")
	repo.AddAndCommit(t, "add scoped repo vars")

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Docs Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, bindings.SourceRepoVarsScoped, preview.ResolvedBindings["project_name"].Source)
	require.Equal(t, "docs", preview.ResolvedBindings["project_name"].Namespace)
	require.Nil(t, preview.VarsFile)
	require.Equal(t, map[string]string{"project_name": "docs"}, preview.InstallRecord.Variables.Namespaces)
}

func TestBuildTemplateApplyPreviewWritesNamespacedBindingWhenRequested(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Docs Orbit\n"+
		"    description: Bound title\n"), 0o600))

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:           repo.Root,
		SourceRef:          sourceRef,
		BindingsFilePath:   bindingsPath,
		VariableNamespaces: map[string]string{"project_name": "docs"},
		Now:                time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	require.NotNil(t, preview.VarsFile)
	require.Empty(t, preview.VarsFile.Variables)
	require.Equal(t, bindings.VariableBinding{
		Value:       "Docs Orbit",
		Description: "Product title",
	}, preview.VarsFile.ScopedVariables["docs"].Variables["project_name"])
	require.Equal(t, "docs", preview.ResolvedBindings["project_name"].Namespace)
	require.Equal(t, map[string]string{"project_name": "docs"}, preview.InstallRecord.Variables.Namespaces)
}

func TestBuildTemplateApplyPreviewConflictsWhenBindingsWouldRewriteRepoVars(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Old Orbit\n"+
		"    description: Local title\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: New Orbit\n"), 0o600))

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:         repo.Root,
		SourceRef:        sourceRef,
		BindingsFilePath: bindingsPath,
		Now:              time.Date(2026, time.March, 21, 11, 2, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, preview.VarsFile)
	require.Contains(t, preview.Conflicts, ApplyConflict{
		Path:    ".harness/vars.yaml",
		Message: "target path already exists with different content",
	})
	require.Contains(t, preview.Conflicts, ApplyConflict{
		Path:    ".harness/vars.yaml",
		Message: "target path has uncommitted worktree status ??",
	})
}

func TestBuildTemplateApplyPreviewNamespacesIncompatibleInstallRecordVariableDeclaration(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Existing Orbit\n"+
		"    description: Product title\n")

	installPath := filepath.Join(repo.Root, ".harness", "installs", "cmd.yaml")
	_, err := WriteInstallRecordFile(installPath, InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "cmd",
		Template: Source{
			SourceKind:     InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/cmd",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.March, 21, 11, 1, 0, 0, time.UTC),
		Variables: &InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "CLI title",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Existing Orbit",
					Description: "CLI title",
				},
			},
		},
	})
	require.NoError(t, err)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 2, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, "docs", preview.ResolvedBindings["project_name"].Namespace)
	require.Equal(t, bindings.SourceRepoVars, preview.ResolvedBindings["project_name"].Source)
	require.Nil(t, preview.VarsFile)
	require.NotNil(t, preview.InstallRecord.Variables)
	require.Equal(t, map[string]string{"project_name": "docs"}, preview.InstallRecord.Variables.Namespaces)
}

func TestBuildTemplateApplyPreviewRendersSharedAgentsPayload(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepoWithSharedAgents(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Local Orbit\n"+
		"    description: Local title\n")

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotNil(t, preview.RenderedSharedAgentsFile)
	require.Equal(t, &CandidateFile{
		Path:    sharedFilePathAgents,
		Content: []byte("Docs orbit for Local Orbit\n"),
		Mode:    gitpkg.FileModeRegular,
	}, preview.RenderedSharedAgentsFile)
	require.Empty(t, preview.Warnings)
	require.Empty(t, preview.Conflicts)
}

func TestBuildTemplateApplyPreviewWarnsWhenRuntimeAgentsAlreadyHasCurrentOrbitBlock(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepoWithSharedAgents(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Local Orbit\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"old docs block\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n")
	repo.AddAndCommit(t, "seed clean runtime agents")

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		`runtime AGENTS.md already contains orbit block "docs"; apply will replace it in place`,
	}, preview.Warnings)
	require.Empty(t, preview.Conflicts)
}

func TestBuildTemplateApplyPreviewFailsWhenRuntimeAgentsIsMalformed(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepoWithSharedAgents(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Local Orbit\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"broken docs block\n")

	_, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "parse runtime AGENTS.md")
	require.ErrorContains(t, err, "unterminated orbit block")
}

func TestBuildTemplateApplyPreviewReusesUncommittedWorktreeVars(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Worktree Orbit\n"+
		"    description: Worktree title\n")

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:  repo.Root,
		SourceRef: sourceRef,
		Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Worktree Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, bindings.SourceRepoVars, preview.ResolvedBindings["project_name"].Source)
}

func TestBuildTemplateApplyPreviewUsesInteractiveBindingsForMissingVariables(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:    repo.Root,
		SourceRef:   sourceRef,
		Interactive: true,
		Prompter: bindingPrompterFunc(func(_ context.Context, unresolved []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error) {
			require.Equal(t, []bindings.UnresolvedBinding{
				{
					Name:        "project_name",
					Description: "Product title",
					Required:    true,
				},
			}, unresolved)

			return map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Prompted Orbit",
					Description: "Prompted title",
				},
			}, nil
		}),
		Now: time.Date(2026, time.March, 21, 11, 30, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Prompted Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, bindings.SourceInteractive, preview.ResolvedBindings["project_name"].Source)
	require.Equal(t, "Product title", preview.ResolvedBindings["project_name"].Description)
}

func TestBuildTemplateApplyPreviewUsesEditorBindingsForMissingVariables(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:   repo.Root,
		SourceRef:  sourceRef,
		EditorMode: true,
		Editor: editorFunc(func(_ context.Context, filename string) error {
			return os.WriteFile(filename, []byte(""+
				"schema_version: 1\n"+
				"variables:\n"+
				"  project_name:\n"+
				"    value: Edited Orbit\n"+
				"    description: Edited title\n"), 0o600)
		}),
		Now: time.Date(2026, time.March, 21, 11, 35, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Edited Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, bindings.SourceEditor, preview.ResolvedBindings["project_name"].Source)
	require.Equal(t, "Product title", preview.ResolvedBindings["project_name"].Description)

	_, statErr := os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestBuildTemplateApplyPreviewEditorDoesNotOverrideExistingBindings(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: From File\n"+
		"    description: File title\n"), 0o600))
	editorCalled := false

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:         repo.Root,
		SourceRef:        sourceRef,
		BindingsFilePath: bindingsPath,
		EditorMode:       true,
		Editor: editorFunc(func(_ context.Context, filename string) error {
			editorCalled = true
			return os.WriteFile(filename, []byte(""+
				"schema_version: 1\n"+
				"variables:\n"+
				"  project_name:\n"+
				"    value: From Editor\n"), 0o600)
		}),
		Now: time.Date(2026, time.March, 21, 11, 40, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.False(t, editorCalled)
	require.Equal(t, bindings.SourceBindingsFile, preview.ResolvedBindings["project_name"].Source)
	require.Equal(t, "From File guide\n", string(preview.RenderedFiles[0].Content))
}

func TestBuildTemplateApplyPreviewFailsWhenEditorProducesInvalidYAML(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	_, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:   repo.Root,
		SourceRef:  sourceRef,
		EditorMode: true,
		Editor: editorFunc(func(_ context.Context, filename string) error {
			return os.WriteFile(filename, []byte("variables: ["), 0o600)
		}),
		Now: time.Date(2026, time.March, 21, 11, 42, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "parse edited bindings")
}

func TestBuildTemplateApplyPreviewFailsWhenEditorLeavesRequiredValueEmpty(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	_, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:   repo.Root,
		SourceRef:  sourceRef,
		EditorMode: true,
		Editor: editorFunc(func(_ context.Context, filename string) error {
			return os.WriteFile(filename, []byte(""+
				"schema_version: 1\n"+
				"variables:\n"+
				"  project_name:\n"+
				"    value: \"\"\n"+
				"    description: Product title\n"), 0o600)
		}),
		Now: time.Date(2026, time.March, 21, 11, 43, 0, 0, time.UTC),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "missing required bindings: project_name")
}

func TestBuildTemplateApplyPreviewDoesNotPromptWhenRepoVarsAlreadyResolveBindings(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Local Orbit\n"+
		"    description: Local title\n")
	promptCalled := false

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:    repo.Root,
		SourceRef:   sourceRef,
		Interactive: true,
		Prompter: bindingPrompterFunc(func(_ context.Context, _ []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error) {
			promptCalled = true
			return map[string]bindings.VariableBinding{}, context.Canceled
		}),
		Now: time.Date(2026, time.March, 21, 11, 45, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.False(t, promptCalled)
	require.Equal(t, bindings.SourceRepoVars, preview.ResolvedBindings["project_name"].Source)
}

func TestBuildTemplateApplyPreviewDoesNotPromptWhenBindingsFileAlreadyResolveBindings(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: File Orbit\n"+
		"    description: File title\n"), 0o600))
	promptCalled := false

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:         repo.Root,
		SourceRef:        sourceRef,
		BindingsFilePath: bindingsPath,
		Interactive:      true,
		Prompter: bindingPrompterFunc(func(_ context.Context, _ []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error) {
			promptCalled = true
			return map[string]bindings.VariableBinding{}, context.Canceled
		}),
		Now: time.Date(2026, time.March, 21, 11, 50, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.False(t, promptCalled)
	require.Equal(t, bindings.SourceBindingsFile, preview.ResolvedBindings["project_name"].Source)
}

func TestApplyLocalTemplateMergesSharedAgentsBlockInPlace(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepoWithSharedAgents(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin workflow=\"api\" -->\n"+
		"api only\n"+
		"<!-- orbit:end workflow=\"api\" -->\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"old docs block\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n"+
		"tail guidance\n")
	repo.AddAndCommit(t, "seed runtime agents for merge")

	result, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:  repo.Root,
			SourceRef: sourceRef,
			Now:       time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		`runtime AGENTS.md already contains orbit block "docs"; apply will replace it in place`,
	}, result.Preview.Warnings)
	require.Contains(t, result.WrittenPaths, "AGENTS.md")

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"shared intro\n"+
		"<!-- orbit:begin workflow=\"api\" -->\n"+
		"api only\n"+
		"<!-- orbit:end workflow=\"api\" -->\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"Docs orbit for Applied Orbit\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n"+
		"tail guidance\n", string(agentsData))
}

func TestApplyLocalTemplateFailsClosedOnMissingVariable(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	_, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:  repo.Root,
			SourceRef: sourceRef,
			Now:       time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "missing required bindings")
	require.ErrorContains(t, err, "project_name")
}

func TestBuildTemplateApplyPreviewAllowsUnresolvedBindingsWhenRequested(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:                repo.Root,
		SourceRef:               sourceRef,
		AllowUnresolvedBindings: true,
		Now:                     time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("$project_name guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Empty(t, preview.ResolvedBindings)
	require.Equal(t, []string{
		"install kept template variables unresolved: project_name",
	}, preview.Warnings)
	require.NotNil(t, preview.InstallRecord.Variables)
	require.Equal(t, map[string]bindings.VariableDeclaration{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
	}, preview.InstallRecord.Variables.Declarations)
	require.Empty(t, preview.InstallRecord.Variables.ResolvedAtApply)
	require.Equal(t, []string{"project_name"}, preview.InstallRecord.Variables.UnresolvedAtApply)
	require.Nil(t, preview.VarsFile)
}

func TestApplyLocalTemplateWritesRuntimeFilesDefinitionInstallRecordAndVars(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"+
		"    description: Bound title\n"), 0o600))

	result, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Preview.Conflicts)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Applied Orbit guide\n", string(guideData))

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "id: docs")

	installRecord, err := loadRuntimeInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, sourceRef, installRecord.Template.SourceRef)
	require.Equal(t, InstallSourceKindLocalBranch, installRecord.Template.SourceKind)

	varsFile, err := loadRuntimeVarsFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, "Applied Orbit", varsFile.Variables["project_name"].Value)
	require.Equal(t, "Product title", varsFile.Variables["project_name"].Description)

	_, err = os.Stat(filepath.Join(repo.GitDir(t), "orbit", "state", "current_orbit.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestApplyLocalTemplateRequiresOverwriteForConflictingPaths(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, "docs/guide.md", "conflicting runtime content\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:         repo.Root,
		SourceRef:        sourceRef,
		BindingsFilePath: bindingsPath,
		Now:              time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.NotEmpty(t, preview.Conflicts)

	_, err = ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
		},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "conflicts detected")

	result, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:          repo.Root,
			SourceRef:         sourceRef,
			BindingsFilePath:  bindingsPath,
			OverwriteExisting: true,
			Now:               time.Date(2026, time.March, 21, 11, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.WrittenPaths)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Applied Orbit guide\n", string(guideData))
}

func TestBuildTemplateApplyPreviewPreservesConflictSummaryWhenOverwriteExisting(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.WriteFile(t, "docs/guide.md", "conflicting runtime content\n")

	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:          repo.Root,
		SourceRef:         sourceRef,
		BindingsFilePath:  bindingsPath,
		OverwriteExisting: true,
		Now:               time.Date(2026, time.March, 21, 11, 5, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Contains(t, preview.Conflicts, ApplyConflict{
		Path:    "docs/guide.md",
		Message: "target path already exists with different content",
	})
}

func TestBuildRemoteTemplateApplyPreviewUsesRemoteInstallMetadata(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Orbit\n"+
		"    description: Bound title\n"), 0o600))

	preview, err := BuildRemoteTemplateApplyPreview(context.Background(), RemoteTemplateApplyPreviewInput{
		RepoRoot:         runtimeRepo.Root,
		RemoteURL:        remoteURL,
		RequestedRef:     sourceRef,
		BindingsFilePath: bindingsPath,
		Now:              time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Equal(t, sourceRef, preview.Source.Ref)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Remote Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, preview.RenderedFiles)
	require.Equal(t, InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: Source{
			SourceKind:     InstallSourceKindExternalGit,
			SourceRepo:     remoteURL,
			SourceRef:      sourceRef,
			TemplateCommit: strings.TrimSpace(sourceRepo.Run(t, "rev-parse", sourceRef)),
		},
		AppliedAt: time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC),
		Variables: &InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "Product title",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Remote Orbit",
					Description: "Product title",
				},
			},
		},
	}, preview.InstallRecord)
}

func TestApplyRemoteTemplateWritesRuntimeFilesDefinitionInstallRecordAndVars(t *testing.T) {
	t.Parallel()

	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"+
		"    description: Bound title\n"), 0o600))

	result, err := ApplyRemoteTemplate(context.Background(), RemoteTemplateApplyInput{
		Preview: RemoteTemplateApplyPreviewInput{
			RepoRoot:         runtimeRepo.Root,
			RemoteURL:        remoteURL,
			RequestedRef:     sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 15, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	require.Empty(t, result.Preview.Conflicts)

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Applied Remote Orbit guide\n", string(guideData))

	installRecord, err := loadRuntimeInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, InstallSourceKindExternalGit, installRecord.Template.SourceKind)
	require.Equal(t, remoteURL, installRecord.Template.SourceRepo)
	require.Equal(t, sourceRef, installRecord.Template.SourceRef)
	require.Equal(t, strings.TrimSpace(sourceRepo.Run(t, "rev-parse", sourceRef)), installRecord.Template.TemplateCommit)

	_, err = os.Stat(filepath.Join(runtimeRepo.GitDir(t), "orbit", "state", "current_orbit.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReplayInstalledTemplateUsesRecordedTemplateCommitInsteadOfCurrentBranchHead(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	_, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 30, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before replay test branch update")

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", sourceRef)
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	repo.Run(t, "checkout", runtimeBranch)

	record, err := loadRuntimeInstallRecord(repo.Root, "docs")
	require.NoError(t, err)

	replay, err := ReplayInstalledTemplate(context.Background(), InstalledTemplateReplayInput{
		RepoRoot: repo.Root,
		Record:   record,
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Applied Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, replay.RenderedFiles)
	require.Equal(t, record.Template.TemplateCommit, replay.Source.Commit)
}

func TestReplayInstalledRemoteTemplateUsesRecordedTemplateCommitAfterRemoteForcePush(t *testing.T) {
	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"), 0o600))

	_, err := ApplyRemoteTemplate(context.Background(), RemoteTemplateApplyInput{
		Preview: RemoteTemplateApplyPreviewInput{
			RepoRoot:         runtimeRepo.Root,
			RemoteURL:        remoteURL,
			RequestedRef:     sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 20, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	sourceRepo.Run(t, "checkout", "--orphan", "rewritten-template")
	sourceRepo.Run(t, "rm", "-rf", ".")
	sourceRepo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: rewritten123\n"+
		"  created_at: 2026-03-21T12:30:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
	sourceRepo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: rewritten123\n"+
		"  created_at: 2026-03-21T12:30:00Z\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Rewritten docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "docs/reference.md", "$project_name rewritten reference\n")
	sourceRepo.AddAndCommit(t, "rewrite remote template branch history")
	sourceRepo.Run(t, "push", "--force", remoteURL, "rewritten-template:"+sourceRef)

	record, err := loadRuntimeInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)

	replay, err := ReplayInstalledTemplate(context.Background(), InstalledTemplateReplayInput{
		RepoRoot: runtimeRepo.Root,
		Record:   record,
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Applied Remote Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, replay.RenderedFiles)
	require.Equal(t, record.Template.TemplateCommit, replay.Source.Commit)
}

func TestReplayInstalledRemoteTemplateFetchesRecordedCommitPin(t *testing.T) {
	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"), 0o600))

	_, err := ApplyRemoteTemplate(context.Background(), RemoteTemplateApplyInput{
		Preview: RemoteTemplateApplyPreviewInput{
			RepoRoot:         runtimeRepo.Root,
			RemoteURL:        remoteURL,
			RequestedRef:     sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 21, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	record, err := loadRuntimeInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)

	commands := installGitCommandLogger(t)

	_, err = ReplayInstalledTemplate(context.Background(), InstalledTemplateReplayInput{
		RepoRoot: runtimeRepo.Root,
		Record:   record,
	})
	require.NoError(t, err)

	fetchCommands := make([]string, 0)
	for _, command := range commands() {
		if loggedGitSubcommand(command) == "fetch" {
			fetchCommands = append(fetchCommands, command)
		}
	}
	require.NotEmpty(t, fetchCommands)
	require.Contains(t, strings.Join(fetchCommands, "\n"), record.Template.TemplateCommit)
}

func TestReplayInstalledRemoteTemplateFallsBackToRecordedRefWhenCommitFetchFails(t *testing.T) {
	sourceRepo, sourceRef := seedLocalTemplateApplyRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedRemoteApplyRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Remote Orbit\n"), 0o600))

	_, err := ApplyRemoteTemplate(context.Background(), RemoteTemplateApplyInput{
		Preview: RemoteTemplateApplyPreviewInput{
			RepoRoot:         runtimeRepo.Root,
			RemoteURL:        remoteURL,
			RequestedRef:     sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 22, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	record, err := loadRuntimeInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)

	commands := installGitCommandLoggerWithFetchFailure(t, record.Template.TemplateCommit)

	replay, err := ReplayInstalledTemplate(context.Background(), InstalledTemplateReplayInput{
		RepoRoot: runtimeRepo.Root,
		Record:   record,
	})
	require.NoError(t, err)
	require.Equal(t, []CandidateFile{
		{
			Path:    "docs/guide.md",
			Content: []byte("Applied Remote Orbit guide\n"),
			Mode:    gitpkg.FileModeRegular,
		},
	}, replay.RenderedFiles)

	fetchCommands := make([]string, 0)
	for _, command := range commands() {
		if loggedGitSubcommand(command) == "fetch" {
			fetchCommands = append(fetchCommands, command)
		}
	}
	require.Len(t, fetchCommands, 2)
	require.Contains(t, fetchCommands[0], record.Template.TemplateCommit)
	require.Contains(t, fetchCommands[1], "refs/heads/"+sourceRef)
}

func TestBuildInstallOwnedCleanupPlanFailsClosedWithoutVariablesSnapshot(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	_, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 32, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before overwrite replay test")

	record, err := loadRuntimeInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	record.Variables = nil
	recordPath, err := runtimeInstallRecordPath(repo.Root, "docs")
	require.NoError(t, err)
	_, err = WriteInstallRecordFile(recordPath, record)
	require.NoError(t, err)
	repo.AddAndCommit(t, "remove install variable snapshot before overwrite replay")

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", sourceRef)
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	repo.Run(t, "checkout", runtimeBranch)

	preview, err := BuildTemplateApplyPreview(context.Background(), TemplateApplyPreviewInput{
		RepoRoot:          repo.Root,
		SourceRef:         sourceRef,
		BindingsFilePath:  bindingsPath,
		OverwriteExisting: true,
		Now:               time.Date(2026, time.March, 21, 12, 45, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	record, err = loadRuntimeInstallRecord(repo.Root, "docs")
	require.NoError(t, err)

	_, err = BuildInstallOwnedCleanupPlan(context.Background(), repo.Root, record, preview)
	require.Error(t, err)
	require.ErrorContains(t, err, "variables snapshot")
	require.ErrorContains(t, err, "overwrite replay")
}

func TestAnalyzeInstalledTemplateDriftReportsDefinitionAndRuntimeFileDrift(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	_, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 35, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Drifted docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Locally drifted guide\n")

	findings, err := AnalyzeInstalledTemplateDrift(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, findings, InstallDriftFinding{
		Kind: DriftKindDefinition,
		Path: ".harness/orbits/docs.yaml",
	})
	require.Contains(t, findings, InstallDriftFinding{
		Kind: DriftKindRuntimeFile,
		Path: "docs/guide.md",
	})
}

func TestAnalyzeInstalledTemplateDriftReportsProvenanceUnresolvable(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	bindingsPath := filepath.Join(repo.Root, "apply-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Applied Orbit\n"), 0o600))

	_, err := ApplyLocalTemplate(context.Background(), TemplateApplyInput{
		Preview: TemplateApplyPreviewInput{
			RepoRoot:         repo.Root,
			SourceRef:        sourceRef,
			BindingsFilePath: bindingsPath,
			Now:              time.Date(2026, time.March, 21, 12, 40, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	installRecordPath, err := runtimeInstallRecordPath(repo.Root, "docs")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(installRecordPath, []byte(""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"+
		"applied_at: 2026-03-21T12:40:00Z\n"), 0o600))

	findings, err := AnalyzeInstalledTemplateDrift(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, findings, InstallDriftFinding{
		Kind: DriftKindProvenanceUnresolvable,
		Path: ".harness/installs/docs.yaml",
	})
}

func TestResolveLocalTemplateSourceRejectsUnexpectedOrbitControlFiles(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.Run(t, "checkout", sourceRef)
	repo.WriteFile(t, ".orbit/orbits/extra.yaml", ""+
		"id: extra\n"+
		"description: Extra orbit\n"+
		"include:\n"+
		"  - extra/**\n")
	repo.AddAndCommit(t, "add unexpected template control file")

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, sourceRef)
	require.Error(t, err)
	require.ErrorContains(t, err, "forbidden path .orbit/orbits/extra.yaml")
}

func TestResolveLocalTemplateSourceRejectsHarnessMetadataPaths(t *testing.T) {
	t.Parallel()

	repo, sourceRef := seedLocalTemplateApplyRepo(t)
	repo.Run(t, "checkout", sourceRef)
	repo.WriteFile(t, ".harness/runtime.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_runtime\n"+
		"harness:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-26T10:00:00Z\n"+
		"  updated_at: 2026-03-26T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "add forbidden harness metadata")

	_, err := ResolveLocalTemplateSource(context.Background(), repo.Root, sourceRef)
	require.Error(t, err)
	require.ErrorContains(t, err, "forbidden path .harness/runtime.yaml")
}

func seedLocalTemplateApplyRepo(t *testing.T) (*testutil.Repo, string) {
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
	repo.AddAndCommit(t, "seed runtime repo")

	_, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", ".harness/vars.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, err)
	require.True(t, exists)

	return repo, "orbit-template/docs"
}

func seedLocalTemplateApplyRepoWithoutVariables(t *testing.T) (*testutil.Repo, string) {
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
	repo.WriteFile(t, "docs/guide.md", "Static guide\n")
	repo.AddAndCommit(t, "seed zero-variable runtime repo")

	_, err := SaveTemplateBranch(context.Background(), TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          time.Date(2026, time.March, 21, 10, 0, 0, 0, time.UTC),
		},
	})
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear zero-variable runtime branch")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, err)
	require.True(t, exists)

	return repo, "orbit-template/docs"
}

func seedLocalTemplateApplyRepoWithSharedAgents(t *testing.T) (*testutil.Repo, string) {
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
	repo.WriteFile(t, "docs/guide.md", "runtime guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	repo.Run(t, "checkout", "-b", "orbit-template/docs")
	repo.Run(t, "rm", "-rf", ".")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    required: true\n")
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
	repo.WriteFile(t, "docs/guide.md", "$project_name guide\n")
	repo.AddAndCommit(t, "seed template branch with structured brief")
	repo.Run(t, "checkout", currentBranch)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	return repo, "orbit-template/docs"
}

func seedRemoteApplyRuntimeRepo(t *testing.T) *testutil.Repo {
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
	repo.WriteFile(t, "README.md", "runtime repo\n")
	repo.AddAndCommit(t, "seed runtime repo")

	return repo
}

type bindingPrompterFunc func(ctx context.Context, unresolved []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error)

func (fn bindingPrompterFunc) PromptBindings(ctx context.Context, unresolved []bindings.UnresolvedBinding) (map[string]bindings.VariableBinding, error) {
	return fn(ctx, unresolved)
}
