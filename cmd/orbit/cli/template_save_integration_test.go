package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/branchinfo"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplateSaveCreatesTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
	require.Contains(t, stdout, "commit: ")
	require.Contains(t, stdout, "files: 3")

	require.Equal(t, currentBranch, strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD")))

	classification, err := branchinfo.ClassifyRevision(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, err)
	require.Equal(t, branchinfo.KindTemplate, classification.Kind)

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(branchManifestData), "default_template: false")
	require.Contains(t, string(branchManifestData), "variables:")
	require.Contains(t, string(branchManifestData), "project_name:")
	require.Contains(t, string(branchManifestData), "description: Product title")
}

func TestTemplateSaveCreatesTemplateBranchWithSharedAgentsPayload(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
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
	repo.AddAndCommit(t, "add structured brief to hosted orbit spec")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
	require.Contains(t, stdout, "files: 2")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
	}, files)

	agentsData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "file: .harness/orbits/docs.yaml")
	require.Contains(t, string(agentsData), "agents_template: |")
	require.Contains(t, string(agentsData), "Docs orbit for $project_name")

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.NotContains(t, string(branchManifestData), "shared_files:")
}

func TestTemplateSaveSkipsProjectionVisibleFilesAndAgents(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
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
	repo.WriteFile(t, "README.md", "Orbit readme\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared intro\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"Docs orbit for Orbit\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")
	repo.AddAndCommit(t, "add projection-visible shared files")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
	require.Contains(t, stdout, "files: 3")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)
}

func TestTemplateSaveMemberSchemaUsesExportPaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".markdownlint.yaml", "line-length = false\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "docs/process/flow.md", "Review flow\n")
	initializeDocsMemberSchemaOrbit(t, repo)
	writeTestRuntimeManifest(t, repo, "docs")
	repo.AddAndCommit(t, "seed member schema runtime repo")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
	require.Contains(t, stdout, "files: 3")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		".markdownlint.yaml",
	}, files)
}

func TestTemplateSaveDryRunDoesNotWriteBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "template save dry-run for orbit docs -> orbit-template/docs")
	require.Contains(t, stdout, "files:")
	require.Contains(t, stdout, ".harness/orbits/docs.yaml")
	require.Contains(t, stdout, "docs/guide.md")
	require.Contains(t, stdout, "ambiguities: none")
	require.Contains(t, stdout, "default_template: false")
	require.Contains(t, stdout, "created_from_branch: "+currentBranch)
	require.Contains(t, stdout, "created_from_commit: "+currentCommit)
	require.Contains(t, stdout, "created_at: ")
	require.Contains(t, stdout, "project_name [required] Product title")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestTemplateSaveAllowsMissingVarsFileForNoVariableTemplate(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, "")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
	require.Contains(t, stdout, "files: 3")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)
}

func TestTemplateSaveFailsOutsideRuntimeRevision(t *testing.T) {
	t.Parallel()

	repo := seedNonRuntimeTemplateSaveRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `template save requires a runtime revision; current revision kind is "plain"`)
}

func TestTemplateSaveUsesZeroCommitProvenanceWithoutCommittedHead(t *testing.T) {
	t.Parallel()

	repo := seedUncommittedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool   `json:"dry_run"`
		OrbitID      string `json:"orbit_id"`
		TargetBranch string `json:"target_branch"`
		Ref          string `json:"ref"`
		Commit       string `json:"commit"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.TargetBranch)
	require.Equal(t, "refs/heads/orbit-template/docs", payload.Ref)
	require.NotEmpty(t, payload.Commit)

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(branchManifestData), "created_from_branch: main")
	require.Contains(t, string(branchManifestData), "created_from_commit: \""+orbittemplate.ZeroGitCommitID+"\"")
}

func TestTemplateSaveFailsClosedOnDetachedHeadRuntimeRevision(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", "--detach", currentCommit)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "template save requires a current branch; detached HEAD is not supported")
}

func TestTemplateSaveFailsClosedOnDriftedMemberHints(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveMemberHintSpec(t, repo, nil)
	repo.AddAndCommit(t, "write member hint spec", ".harness/orbits/docs.yaml")
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-rules\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Style\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "orbit member backfill --orbit docs")
	require.ErrorContains(t, err, "before saving")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplateSaveRejectsUntrackedMemberPayloadReferencedByTruth(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveMemberHintSpec(t, repo, []orbitpkg.OrbitMember{
		{
			Name: "docs-new",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/new.md"},
			},
		},
	})
	repo.AddAndCommit(t, "commit backfilled member truth", ".harness/orbits/docs.yaml")
	repo.WriteFile(t, "docs/new.md", "# New\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `member export path "docs/new.md" is missing from the template payload`)
	require.ErrorContains(t, err, `git add docs/new.md`)

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplateSaveAllowsInSyncMemberHints(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveMemberHintSpec(t, repo, []orbitpkg.OrbitMember{
		{
			Name: "docs-rules",
			Role: orbitpkg.OrbitMemberRule,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{"docs/rules/style.md"},
			},
		},
	})
	repo.WriteFile(t, "docs/rules/style.md", ""+
		"---\n"+
		"orbit_member:\n"+
		"  name: docs-rules\n"+
		"  role: rule\n"+
		"---\n"+
		"\n"+
		"# Style\n")
	repo.AddAndCommit(t, "add in-sync member hint before template save")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")
}

func TestTemplateSaveDryRunWarnsWhenRuntimeAgentsLacksCurrentOrbitMarker(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope:\n"+
		"  - AGENTS.md\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"shared Orbit guidance\n"+
		"<!-- orbit:begin orbit_id=\"api\" -->\n"+
		"api only\n"+
		"<!-- orbit:end orbit_id=\"api\" -->\n")
	repo.AddAndCommit(t, "add runtime agents without docs block")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "warnings: none")
	require.NotContains(t, stdout, `runtime AGENTS.md does not contain orbit block "docs"; saving unmarked content only`)
	require.NotContains(t, stdout, "AGENTS.md")
}

func TestTemplateSaveDryRunSupportsJSONOutput(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		OrbitID      string   `json:"orbit_id"`
		TargetBranch string   `json:"target_branch"`
		Files        []string `json:"files"`
		Manifest     struct {
			OrbitID           string `json:"orbit_id"`
			DefaultTemplate   bool   `json:"default_template"`
			CreatedFromBranch string `json:"created_from_branch"`
			CreatedFromCommit string `json:"created_from_commit"`
			CreatedAt         string `json:"created_at"`
		} `json:"manifest"`
		Replacements []struct {
			Path         string `json:"path"`
			Replacements []struct {
				Variable string `json:"variable"`
				Literal  string `json:"literal"`
				Count    int    `json:"count"`
			} `json:"replacements"`
		} `json:"replacements"`
		Variables []struct {
			Name        string `json:"name"`
			Required    bool   `json:"required"`
			Description string `json:"description"`
		} `json:"variables"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.TargetBranch)
	require.Equal(t, "docs", payload.Manifest.OrbitID)
	require.False(t, payload.Manifest.DefaultTemplate)
	require.Equal(t, currentBranch, payload.Manifest.CreatedFromBranch)
	require.Equal(t, currentCommit, payload.Manifest.CreatedFromCommit)
	require.NotEmpty(t, payload.Manifest.CreatedAt)
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.Len(t, payload.Replacements, 1)
	require.Equal(t, "docs/guide.md", payload.Replacements[0].Path)
	require.Len(t, payload.Replacements[0].Replacements, 1)
	require.Equal(t, "project_name", payload.Replacements[0].Replacements[0].Variable)
	require.Equal(t, "Orbit", payload.Replacements[0].Replacements[0].Literal)
	require.Equal(t, 1, payload.Replacements[0].Replacements[0].Count)
	require.Len(t, payload.Variables, 1)
	require.Equal(t, "project_name", payload.Variables[0].Name)
	require.True(t, payload.Variables[0].Required)
	require.Equal(t, "Product title", payload.Variables[0].Description)
}

func TestTemplateSaveDryRunJSONIncludesAmbiguitySummary(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Ambiguities []struct {
			Path        string `json:"path"`
			Ambiguities []struct {
				Literal   string   `json:"literal"`
				Variables []string `json:"variables"`
			} `json:"ambiguities"`
		} `json:"ambiguities"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Len(t, payload.Ambiguities, 1)
	require.Equal(t, "docs/guide.md", payload.Ambiguities[0].Path)
	require.Len(t, payload.Ambiguities[0].Ambiguities, 1)
	require.Equal(t, "Orbit", payload.Ambiguities[0].Ambiguities[0].Literal)
	require.ElementsMatch(t, []string{"product_name", "project_name"}, payload.Ambiguities[0].Ambiguities[0].Variables)
}

func TestTemplateSaveSupportsJSONOutput(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		OrbitID      string   `json:"orbit_id"`
		TargetBranch string   `json:"target_branch"`
		Ref          string   `json:"ref"`
		Commit       string   `json:"commit"`
		Files        []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "orbit-template/docs", payload.TargetBranch)
	require.Equal(t, "refs/heads/orbit-template/docs", payload.Ref)
	require.NotEmpty(t, payload.Commit)
	require.Contains(t, payload.Files, ".harness/manifest.yaml")
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
}

func TestTemplateSaveDefaultFlagOnlyAffectsManifestMetadata(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--default")
	require.NoError(t, err)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "default_template: true")
}

func TestTemplateSaveFailsWhenTargetBranchExistsWithoutOverwrite(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.Run(t, "branch", "orbit-template/docs")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "already exists")
}

func TestTemplateSaveDryRunFailsClosedOnAmbiguity(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"  project_name:\n"+
		"    value: Orbit\n")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "template save dry-run for orbit docs -> orbit-template/docs")
	require.Contains(t, stdout, "ambiguities:")
	require.Contains(t, stdout, "docs/guide.md: Orbit -> product_name, project_name")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplateSaveUsesInstallRecordSourceRefAsDefaultWritebackTarget(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveInstallRecord(t, repo)
	writeTemplateSaveRuntimeMemberSource(t, repo, "docs", harness.ManifestMemberSourceInstallOrbit)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"docs/guide.md",
	}, files)

	manifestFile, err := harness.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, manifestFile.Members, 1)
	require.Equal(t, harness.ManifestMemberSourceInstallOrbit, manifestFile.Members[0].Source)

	installRecord, err := harness.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", installRecord.Template.SourceRef)
}

func TestTemplateSaveRequiresExplicitTargetForManualRuntimeMembersEvenWithInstallRecord(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveInstallRecord(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `template save can omit --to only for install_orbit members; runtime member "docs" uses source "manual"`)
}

func TestTemplateSaveRequiresExplicitTargetForBundleInstalledRuntimeMembers(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTestRuntimeManifestMembers(t, repo, harness.ManifestMember{
		OrbitID:        "docs",
		Source:         harness.ManifestMemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 9, 11, 5, 0, 0, time.UTC),
	})
	repo.Run(t, "add", ".harness/manifest.yaml")
	repo.Run(t, "commit", "-m", "mark runtime member as bundle-installed")
	writeTemplateSaveInstallRecord(t, repo)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `template save can omit --to only for install_orbit members; runtime member "docs" uses source "install_bundle"`)
}

func TestTemplateSaveDryRunUsesInstallRecordSourceRefAsDefaultWritebackTarget(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	writeTemplateSaveInstallRecord(t, repo)
	writeTemplateSaveRuntimeMemberSource(t, repo, "docs", harness.ManifestMemberSourceInstallOrbit)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "template save dry-run for orbit docs -> orbit-template/docs")

	exists, existsErr := gitpkg.LocalBranchExists(context.Background(), repo.Root, "orbit-template/docs")
	require.NoError(t, existsErr)
	require.False(t, exists)
}

func TestTemplateSaveReadsHiddenTrackedFilesFromHEAD(t *testing.T) {
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
	writeTestRuntimeManifest(t, repo, "docs")
	repo.WriteFile(t, "README.md", "root\n")
	repo.WriteFile(t, "docs/hidden.md", "Orbit hidden\n")
	repo.AddAndCommit(t, "seed runtime repo")

	repo.Run(t, "sparse-checkout", "init", "--no-cone")
	repo.Run(t, "sparse-checkout", "set", "README.md")

	_, err := os.Stat(filepath.Join(repo.Root, "docs", "hidden.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, _, err = executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)

	hiddenData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/hidden.md")
	require.NoError(t, err)
	require.Equal(t, "$project_name hidden\n", string(hiddenData))
}

func TestTemplateSaveEditTemplateWritesEditedTemplateWithoutMutatingRuntimeWorktree(t *testing.T) {
	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  service_url:\n"+
		"    value: http://localhost:3000\n"+
		"    description: Service URL\n")

	editorScript := filepath.Join(repo.Root, "edit-template.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"printf '%s\\n' '$project_name guide at $service_url' > \"$1/docs/guide.md\"\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--edit-template")
	require.NoError(t, err)

	runtimeData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Orbit guide\n", string(runtimeData))

	templateData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, "$project_name guide at $service_url\n", string(templateData))

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "project_name")
	require.Contains(t, string(manifestData), "service_url")
}

func TestTemplateSaveEditTemplatePreservesExecutableFileMode(t *testing.T) {
	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/build.sh", "#!/bin/sh\necho orbit\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "docs", "build.sh"), 0o755))
	repo.AddAndCommit(t, "add executable runtime file")

	editorScript := filepath.Join(repo.Root, "edit-template.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"printf '%s\\n' '#!/bin/sh' 'echo edited orbit' > \"$1/docs/build.sh\"\n"+
		"chmod 755 \"$1/docs/build.sh\"\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--edit-template")
	require.NoError(t, err)

	mode := strings.Fields(repo.Run(t, "ls-tree", "orbit-template/docs", "docs/build.sh"))[0]
	require.Equal(t, gitpkg.FileModeExecutable, mode)
}

func TestTemplateSaveEditTemplateSupportsQuotedEditorCommandWithSpacedPath(t *testing.T) {
	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	editorDir := filepath.Join(repo.Root, "tools with spaces")
	require.NoError(t, os.MkdirAll(editorDir, 0o755))
	editorScript := filepath.Join(editorDir, "edit template.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"if [ \"$1\" != \"--mode\" ] || [ \"$2\" != \"template edit\" ]; then\n"+
		"  exit 17\n"+
		"fi\n"+
		"printf '%s\\n' '$project_name hardening guide' > \"$3/docs/guide.md\"\n"), 0o755))
	t.Setenv("EDITOR", "\""+editorScript+"\" --mode \"template edit\"")

	_, _, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--edit-template")
	require.NoError(t, err)

	templateData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, "$project_name hardening guide\n", string(templateData))
}

func TestTemplateSaveFailsClosedOnOutOfRangeLocalSkillsWithoutFlags(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveOutOfRangeSkillRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "extras/research-kit")
	require.ErrorContains(t, err, "--aggregate-detected-skills")
	require.ErrorContains(t, err, "--allow-out-of-range-skills")
}

func TestTemplateSaveFailsOnCapabilityOwnedMemberOverlap(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveRepo(t, ""+
		"schema_version: 1\n"+
		"variables: {}\n")
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
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/docs/**/*.md\n"+
		"members:\n"+
		"  - name: docs-content\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - commands/docs/**\n")
	repo.AddAndCommit(t, "seed invalid capability overlap")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `members[0].paths.include[1] overlaps capability-owned commands path "commands/docs/**/*.md"`)
}

func TestTemplateSaveAllowOutOfRangeLocalSkillsWarnsAndKeepsDetectedSkillInTemplatePayload(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveOutOfRangeSkillRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--allow-out-of-range-skills")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "warnings:")
	require.Contains(t, stdout, "extras/research-kit")
	require.Contains(t, stdout, "skills/docs/*")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"declared-skills/docs-style/SKILL.md",
		"declared-skills/docs-style/checklist.md",
		"docs/guide.md",
		"extras/research-kit/SKILL.md",
		"extras/research-kit/playbook.md",
	}, files)

	specData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	require.NotContains(t, string(specData), "skills/docs/*")
}

func TestTemplateSaveAggregateDetectedSkillsMovesRootUpdatesSpecAndSaves(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSaveOutOfRangeSkillRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--aggregate-detected-skills")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved template orbit docs to branch orbit-template/docs")

	require.NoFileExists(t, filepath.Join(repo.Root, "extras", "research-kit", "SKILL.md"))
	require.FileExists(t, filepath.Join(repo.Root, "skills", "docs", "research-kit", "SKILL.md"))

	specData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(specData), "- declared-skills/*\n")
	require.Contains(t, string(specData), "- skills/docs/*\n")

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "orbit-template/docs")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs.yaml",
		"declared-skills/docs-style/SKILL.md",
		"declared-skills/docs-style/checklist.md",
		"docs/guide.md",
		"skills/docs/research-kit/SKILL.md",
		"skills/docs/research-kit/playbook.md",
	}, files)
}

func seedTemplateSaveRepo(t *testing.T, varsYAML string) *testutil.Repo {
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
	writeTestRuntimeManifest(t, repo, "docs")
	if strings.TrimSpace(varsYAML) != "" {
		repo.WriteFile(t, ".harness/vars.yaml", varsYAML)
	}
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "notes/todo.md", "outside\n")
	repo.AddAndCommit(t, "seed runtime repo")

	return repo
}

func seedUncommittedTemplateSaveRepo(t *testing.T, varsYAML string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
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
	writeTestRuntimeManifest(t, repo, "docs")
	if strings.TrimSpace(varsYAML) != "" {
		repo.WriteFile(t, ".harness/vars.yaml", varsYAML)
	}
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "notes/todo.md", "outside\n")
	repo.Run(t, "add", "-A")

	return repo
}

func writeTemplateSaveMemberHintSpec(t *testing.T, repo *testutil.Repo, members []orbitpkg.OrbitMember) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	spec.Members = append([]orbitpkg.OrbitMember(nil), members...)

	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
}

func seedTemplateSaveOutOfRangeSkillRepo(t *testing.T) *testutil.Repo {
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
		"          - declared-skills/*\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - extras/**\n")
	writeTestRuntimeManifest(t, repo, "docs")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {}\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "declared-skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "declared-skills/docs-style/checklist.md", "Use docs style guide.\n")
	repo.WriteFile(t, "extras/research-kit/SKILL.md", ""+
		"---\n"+
		"name: research-kit\n"+
		"description: Research kit references.\n"+
		"---\n"+
		"# Research Kit\n")
	repo.WriteFile(t, "extras/research-kit/playbook.md", "Use research kit.\n")
	repo.AddAndCommit(t, "seed runtime repo with out-of-range skill")

	return repo
}

func writeTemplateSaveInstallRecord(t *testing.T, repo *testutil.Repo) {
	t.Helper()

	_, err := harness.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD")),
		},
		AppliedAt: time.Date(2026, time.April, 8, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	repo.Run(t, "add", ".harness/installs")
	repo.Run(t, "commit", "-m", "add runtime install record")
}

func writeTemplateSaveRuntimeMemberSource(t *testing.T, repo *testutil.Repo, orbitID string, source string) {
	t.Helper()

	writeTestRuntimeManifestMembers(t, repo, harness.ManifestMember{
		OrbitID: orbitID,
		Source:  source,
		AddedAt: time.Date(2026, time.April, 9, 11, 5, 0, 0, time.UTC),
	})
	repo.Run(t, "add", ".harness/manifest.yaml")
	repo.Run(t, "commit", "-m", "update runtime member source")
}

func seedNonRuntimeTemplateSaveRepo(t *testing.T) *testutil.Repo {
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
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed non-runtime repo")

	return repo
}

func writeTestRuntimeManifest(t *testing.T, repo *testutil.Repo, orbitIDs ...string) {
	t.Helper()

	manifestFile, err := harness.DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.April, 9, 11, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	manifestFile.Members = make([]harness.ManifestMember, 0, len(orbitIDs))
	for index, orbitID := range orbitIDs {
		manifestFile.Members = append(manifestFile.Members, harness.ManifestMember{
			OrbitID: orbitID,
			Source:  harness.ManifestMemberSourceManual,
			AddedAt: time.Date(2026, time.April, 9, 11, 5+index, 0, 0, time.UTC),
		})
	}
	writeTestRuntimeManifestMembers(t, repo, manifestFile.Members...)
}

func writeTestRuntimeManifestMembers(t *testing.T, repo *testutil.Repo, members ...harness.ManifestMember) {
	t.Helper()

	manifestFile, err := harness.DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.April, 9, 11, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	manifestFile.Members = append([]harness.ManifestMember(nil), members...)
	_, err = harness.WriteManifestFile(repo.Root, manifestFile)
	require.NoError(t, err)
}

func splitLines(value string) []string {
	if value == "" {
		return nil
	}

	return strings.Split(value, "\n")
}
