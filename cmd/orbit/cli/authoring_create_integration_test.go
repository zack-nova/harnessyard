package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestSourceCreateBootstrapsMissingDirectory(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	targetPath := filepath.Join(parentDir, "source-authoring")

	stdout, stderr, err := executeCLI(
		t,
		parentDir,
		"source",
		"create",
		targetPath,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs source branch",
		"--json",
	)
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
		GitInitialized bool `json:"git_initialized"`
		Changed        bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "publish_orbit_id")
	requireSamePath(t, targetPath, payload.RepoRoot)
	requireSamePath(t, filepath.Join(targetPath, ".harness", "manifest.yaml"), payload.SourceManifestPath)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.NotEmpty(t, payload.SourceBranch)
	require.True(t, payload.GitInitialized)
	require.True(t, payload.Changed)

	require.NoError(t, assertFileExists(filepath.Join(targetPath, ".git")))

	manifestData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: source\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
	require.Contains(t, string(manifestData), "source_branch: "+payload.SourceBranch+"\n")

	definitionData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs source branch\n")
	require.Contains(t, string(definitionData), "meta:\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, targetPath)
	agentsData, err := os.ReadFile(filepath.Join(targetPath, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Docs source branch\n")
	require.NotContains(t, string(definitionData), "members:\n")
	require.NotContains(t, string(definitionData), "key:")
}

func TestTemplateCreateBootstrapsExistingNonGitDirectory(t *testing.T) {
	t.Parallel()

	targetPath := t.TempDir()

	stdout, stderr, err := executeCLI(
		t,
		targetPath,
		"template",
		"create",
		targetPath,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs template branch",
		"--json",
	)
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
		GitInitialized bool `json:"git_initialized"`
		Changed        bool `json:"changed"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, stdout, "\"orbit_id\"")
	requireSamePath(t, targetPath, payload.RepoRoot)
	requireSamePath(t, filepath.Join(targetPath, ".harness", "manifest.yaml"), payload.ManifestPath)
	require.Equal(t, "orbit", payload.Package.Type)
	require.Equal(t, "docs", payload.Package.Name)
	require.NotEmpty(t, payload.CurrentBranch)
	require.True(t, payload.GitInitialized)
	require.True(t, payload.Changed)

	require.NoError(t, assertFileExists(filepath.Join(targetPath, ".git")))

	manifestData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: orbit_template\n")
	require.Contains(t, string(manifestData), "package:\n")
	require.Contains(t, string(manifestData), "type: orbit\n")
	require.Contains(t, string(manifestData), "name: docs\n")
	require.NotContains(t, string(manifestData), "orbit_id: docs\n")
	require.Contains(t, string(manifestData), "created_from_branch: "+payload.CurrentBranch+"\n")

	definitionData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(definitionData), "name: Docs Orbit\n")
	require.Contains(t, string(definitionData), "description: Docs template branch\n")
	require.Contains(t, string(definitionData), "meta:\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, targetPath)
	agentsData, err := os.ReadFile(filepath.Join(targetPath, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Docs template branch\n")
	require.NotContains(t, string(definitionData), "members:\n")
	require.NotContains(t, string(definitionData), "key:")
}

func TestSourceCreateReusesExistingPlainGitRepo(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"source",
		"create",
		repo.Root,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs source branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		GitInitialized bool   `json:"git_initialized"`
		SourceBranch   string `json:"source_branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.GitInitialized)
	require.Equal(t, "main", payload.SourceBranch)
}

func TestSourceCreateWithSpecBootstrapsSpecMemberAndDoc(t *testing.T) {
	t.Parallel()

	parentDir := t.TempDir()
	targetPath := filepath.Join(parentDir, "source-authoring")

	_, stderr, err := executeCLI(
		t,
		parentDir,
		"source",
		"create",
		targetPath,
		"--orbit", "docs",
		"--with-spec",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: spec\n")
	require.Contains(t, string(definitionData), "- docs/docs.md\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, targetPath)

	specDocData, err := os.ReadFile(filepath.Join(targetPath, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))
}

func TestTemplateCreateReusesExistingPlainGitRepo(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	stdout, stderr, err := executeCLI(
		t,
		repo.Root,
		"template",
		"create",
		repo.Root,
		"--orbit",
		"docs",
		"--name",
		"Docs Orbit",
		"--description",
		"Docs template branch",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		GitInitialized bool   `json:"git_initialized"`
		CurrentBranch  string `json:"current_branch"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.GitInitialized)
	require.Equal(t, "main", payload.CurrentBranch)
}

func TestTemplateCreateWithSpecBootstrapsSpecMemberAndDoc(t *testing.T) {
	t.Parallel()

	targetPath := t.TempDir()

	_, stderr, err := executeCLI(
		t,
		targetPath,
		"template",
		"create",
		targetPath,
		"--orbit", "docs",
		"--with-spec",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	definitionData, err := os.ReadFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(definitionData), "key:")
	require.Contains(t, string(definitionData), "name: spec\n")
	require.Contains(t, string(definitionData), "- docs/docs.md\n")
	requireContainsDefaultCapabilityTruth(t, string(definitionData))
	requireContainsSeedEmptyGuidanceArtifacts(t, targetPath)

	specDocData, err := os.ReadFile(filepath.Join(targetPath, "docs", "docs.md"))
	require.NoError(t, err)
	require.Equal(t, "# docs Spec\n", string(specDocData))
}

func TestAuthoringCreateWithoutOrbitRequiresExplicitIdentityForFirstOrbit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                 string
		command              []string
		precreateTarget      bool
		seedHostedDefinition bool
	}{
		{name: "source missing directory", command: []string{"source", "create"}},
		{name: "source non git directory with hosted orbit", command: []string{"source", "create"}, precreateTarget: true, seedHostedDefinition: true},
		{name: "template non git directory", command: []string{"template", "create"}, precreateTarget: true},
		{name: "template non git directory with hosted orbit", command: []string{"template", "create"}, precreateTarget: true, seedHostedDefinition: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parentDir := t.TempDir()
			targetPath := filepath.Join(parentDir, "authoring")
			if tc.precreateTarget {
				require.NoError(t, os.MkdirAll(targetPath, 0o755))
			}
			if tc.seedHostedDefinition {
				require.NoError(t, os.MkdirAll(filepath.Join(targetPath, ".harness", "orbits"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(targetPath, ".harness", "orbits", "docs.yaml"), []byte("id: docs\ninclude:\n  - docs/**\n"), 0o600))
			}

			args := append(append([]string{}, tc.command...), targetPath)
			stdout, stderr, err := executeCLI(t, parentDir, args...)
			require.Error(t, err)
			require.Empty(t, stdout)
			require.Empty(t, stderr)
			require.ErrorContains(t, err, "authoring create requires --orbit when target is not already a Git repository")
		})
	}
}

func TestSourceCreateFailsClosedOnIncompatibleManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  harness_id: main\n"+
		"  name: Main Harness\n"+
		"members: []\n")

	_, _, err := executeCLI(t, repo.Root, "source", "create", repo.Root, "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `source init requires a plain or source branch; current revision kind is "runtime"`)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestTemplateCreateFailsClosedOnIncompatibleManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"harness_template:\n"+
		"  harness_id: workspace\n"+
		"  name: Workspace\n")

	_, _, err := executeCLI(t, repo.Root, "template", "create", repo.Root, "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `template init requires a plain or orbit_template branch; current revision kind is "harness_template"`)

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestSourceInitReportsCreateGuidanceOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	_, _, err := executeCLI(
		t,
		workingDir,
		"source",
		"init",
		"--orbit",
		"research",
		"--name",
		"Research Orbit",
		"--description",
		"Gather source-backed notes",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "current directory is not a Git repository")
	require.ErrorContains(t, err, `orbit source create . --orbit research --name "Research Orbit" --description "Gather source-backed notes"`)
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
}

func TestTemplateInitReportsCreateGuidanceOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	_, _, err := executeCLI(
		t,
		workingDir,
		"template",
		"init",
		"--orbit",
		"research",
		"--name",
		"Research Orbit",
		"--description",
		"Installable research orbit",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "current directory is not a Git repository")
	require.ErrorContains(t, err, `orbit template create . --orbit research --name "Research Orbit" --description "Installable research orbit"`)
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
}

func TestTemplateInitSourceReportsSourceCreateGuidanceOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	_, _, err := executeCLI(
		t,
		workingDir,
		"template",
		"init-source",
		"--orbit",
		"research",
		"--name",
		"Research Orbit",
		"--description",
		"Research notes",
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "current directory is not a Git repository")
	require.ErrorContains(t, err, `orbit source create . --orbit research --name "Research Orbit" --description "Research notes"`)
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
}

func assertFileExists(filename string) error {
	_, err := os.Stat(filename)
	return err
}

func requireSamePath(t *testing.T, expected string, actual string) {
	t.Helper()

	require.Equal(t, filepath.Clean(mustEvalPath(expected)), filepath.Clean(mustEvalPath(actual)))
}

func mustEvalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}

	return path
}
