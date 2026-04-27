package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestSourceInitCreatesHostedDefinitionInPlainRepo(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeCLI(t, repo.Root, "source", "init", "--orbit", "docs", "--name", "Docs Orbit", "--description", "Docs source branch", "--json")
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

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: source\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
	require.Contains(t, string(manifestData), "source_branch: main\n")

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs source branch\n")
	require.Contains(t, string(definitionData), "meta:\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, repo.Root)
	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Docs source branch\n")
	require.NotContains(t, string(definitionData), "members:\n")
	require.NotContains(t, string(definitionData), "key:")

	validateStdout, validateStderr, err := executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, validateStderr)
	require.Contains(t, validateStdout, "docs")
}

func TestTemplateInitCreatesOrbitTemplateManifestInPlainRepo(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init", "--orbit", "docs", "--name", "Docs Orbit", "--description", "Docs template branch", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot      string `json:"repo_root"`
		ManifestPath  string `json:"manifest_path"`
		CurrentBranch string `json:"current_branch"`
		Package       struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), payload.ManifestPath)
	require.Equal(t, "main", payload.CurrentBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
	require.Contains(t, string(manifestData), "created_from_branch: main\n")
	require.Contains(t, string(manifestData), "created_from_commit:")

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs template branch\n")
	require.Contains(t, string(definitionData), "meta:\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, repo.Root)
	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Docs template branch\n")
	require.NotContains(t, string(definitionData), "members:\n")
	require.NotContains(t, string(definitionData), "key:")

	validateStdout, validateStderr, err := executeCLI(t, repo.Root, "validate")
	require.NoError(t, err)
	require.Empty(t, validateStderr)
	require.Contains(t, validateStdout, "docs")
}

func TestSourceInitDefaultsToOnlyHostedOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, _, err := executeCLI(t, repo.Root, "create", "docs", "--name", "Docs Orbit")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "source", "init", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SourceBranch string `json:"source_branch"`
		Package      struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	require.Equal(t, "main", payload.SourceBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: source\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
}

func TestTemplateInitDefaultsToOnlyHostedOrbitDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, _, err := executeCLI(t, repo.Root, "create", "docs", "--name", "Docs Orbit")
	require.NoError(t, err)

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "init", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		CurrentBranch string `json:"current_branch"`
		Package       struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"package"`
		Changed bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	require.Equal(t, "main", payload.CurrentBranch)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.True(t, payload.Changed)

	manifestData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
}

func TestAuthoringInitWithoutHostedOrbitStillRequiresExplicitOrbit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "source", args: []string{"source", "init"}},
		{name: "template", args: []string{"template", "init"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := testutil.NewRepo(t)
			repo.Run(t, "branch", "-m", "main")

			stdout, stderr, err := executeCLI(t, repo.Root, tc.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "authoring init requires --orbit when no hosted orbit definition is present")
		})
	}
}

func TestAuthoringInitWithoutOrbitFailsClosedForMultipleHostedDefinitions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "source", args: []string{"source", "init"}},
		{name: "template", args: []string{"template", "init"}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := testutil.NewRepo(t)
			repo.Run(t, "branch", "-m", "main")
			_, _, err := executeCLI(t, repo.Root, "create", "docs")
			require.NoError(t, err)
			_, _, err = executeCLI(t, repo.Root, "create", "api")
			require.NoError(t, err)

			stdout, stderr, err := executeCLI(t, repo.Root, tc.args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "authoring branch must contain exactly one hosted orbit definition")
		})
	}
}

func TestSourceInitWithSpecCreatesSpecMemberAndDoc(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, stderr, err := executeCLI(t, repo.Root, "source", "init", "--orbit", "docs", "--with-spec", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: spec\n")
	require.Contains(t, string(definitionData), "- docs/docs.md\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, repo.Root)

	specDocData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))
}

func TestTemplateInitWithSpecCreatesSpecMemberAndDoc(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, stderr, err := executeCLI(t, repo.Root, "template", "init", "--orbit", "docs", "--with-spec", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: spec\n")
	require.Contains(t, string(definitionData), "- docs/docs.md\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, repo.Root)

	specDocData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))
}

func TestSourceInitDoesNotRewriteExistingHostedOrbitToAddDefaultCapabilities(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Existing docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"behavior:\n"+
		"  scope:\n"+
		"    projection_roles:\n"+
		"      - meta\n"+
		"      - subject\n"+
		"      - rule\n"+
		"      - process\n"+
		"    write_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    export_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"    orchestration_roles:\n"+
		"      - meta\n"+
		"      - rule\n"+
		"      - process\n"+
		"  orchestration:\n"+
		"    include_orbit_description: true\n"+
		"    materialize_agents_from_meta: true\n")
	originalDefinition, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)

	_, stderr, err := executeCLI(t, repo.Root, "source", "init", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Equal(t, string(originalDefinition), string(definitionData))
	require.NotContains(t, string(definitionData), "capabilities:\n")
	for _, path := range []string{"AGENTS.md", "HUMANS.md", "BOOTSTRAP.md"} {
		_, statErr := os.Stat(filepath.Join(repo.Root, path))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	}
}
