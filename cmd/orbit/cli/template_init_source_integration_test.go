package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestTemplateInitSourceCreatesSourceManifestFromCurrentBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init-source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot           string `json:"repo_root"`
		SourceManifestPath string `json:"source_manifest_path"`
		SourceBranch       string `json:"source_branch"`
		Package            struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), payload.SourceManifestPath)
	require.Equal(t, "main", payload.SourceBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	data, readErr := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"    package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"    source_branch: main\n", string(data))

	hostedDefinition, readErr := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, readErr)
	require.Equal(t, ""+
		"package:\n"+
		"    type: orbit\n"+
		"    name: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"    - docs/**\n"+
		"\n", string(hostedDefinition))

	_, statErr := os.Stat(filepath.Join(repo.Root, ".orbit", "orbits", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	_, statErr = os.Stat(filepath.Join(repo.Root, ".orbit", "source.yaml"))
	require.Error(t, statErr)
}

func TestTemplateInitSourceFailsOnDetachedHEAD(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)
	head := repo.Run(t, "rev-parse", "HEAD")
	repo.Run(t, "checkout", head[:len(head)-1])

	_, _, err := executeCLI(t, repo.Root, "template", "init-source")
	require.Error(t, err)
	require.ErrorContains(t, err, "detached HEAD")
}

func TestTemplateInitSourceFailsWhenMultipleOrbitDefinitionsExist(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "API\n")
	repo.AddAndCommit(t, "add second orbit")

	_, _, err := executeCLI(t, repo.Root, "template", "init-source")
	require.Error(t, err)
	require.ErrorContains(t, err, "exactly one orbit definition")
}

func TestTemplateInitSourcePrefersHostedOrbitDefinitionsOverLegacyCompanions(t *testing.T) {
	t.Parallel()

	repo := seedHostedTemplateAuthoringRepo(t)
	repo.WriteFile(t, ".orbit/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"include:\n"+
		"  - api/**\n")
	repo.WriteFile(t, "api/spec.md", "API\n")
	repo.AddAndCommit(t, "add legacy extra orbit")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init-source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Package struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	hostedDefinition, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(hostedDefinition), "meta:")
	require.Contains(t, string(hostedDefinition), "file: .harness/orbits/docs.yaml")
}

func TestTemplateInitSourceCreatesSourceManifestFromHostedOnlyOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := seedHostedOnlyTemplateAuthoringRepo(t)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init-source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Package struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "kind: source")
	require.Contains(t, string(data), "package:")
	require.Contains(t, string(data), "name: docs")
	require.NotContains(t, string(data), "orbit_id: docs")
}

func TestTemplateInitSourceReportsChangedWhenMigratingLegacyDefinitionOnExistingSourceBranch(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.AddAndCommit(t, "seed existing source manifest")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init-source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Changed)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTemplateInitSourceFailsWhenHarnessMetadataExists(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n")
	repo.AddAndCommit(t, "add harness metadata")

	_, _, err := executeCLI(t, repo.Root, "template", "init-source")
	require.Error(t, err)
	require.ErrorContains(t, err, ".harness/")
}

func TestTemplateInitSourceRemovesLegacyTemplateManifest(t *testing.T) {
	t.Parallel()

	repo := seedTemplateAuthoringRepo(t)
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-31T00:00:00Z\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "add template manifest")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init-source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Changed)

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "template.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	data, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(data), "kind: source")
	require.Contains(t, string(data), "package:")
	require.Contains(t, string(data), "name: docs")
	require.NotContains(t, string(data), "orbit_id: docs")
}

func seedTemplateAuthoringRepo(t *testing.T) *testutil.Repo {
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
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed template authoring repo")

	return repo
}

func seedHostedTemplateAuthoringRepo(t *testing.T) *testutil.Repo {
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
		"        - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed hosted template authoring repo")

	return repo
}

func seedHostedOnlyTemplateAuthoringRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
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
		"        - docs/**\n")
	repo.WriteFile(t, "README.md", "author docs\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed hosted-only template authoring repo")

	return repo
}
