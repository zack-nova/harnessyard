package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type hyardAdoptSourcePayload struct {
	SchemaVersion      string                      `json:"schema_version"`
	RepoRoot           string                      `json:"repo_root"`
	Mode               string                      `json:"mode"`
	AdoptedOrbit       hyardAdoptCheckAdoptedOrbit `json:"adopted_orbit"`
	SourceManifestPath string                      `json:"source_manifest_path"`
	SourceBranch       string                      `json:"source_branch"`
	WrittenPaths       []string                    `json:"written_paths"`
	NextActions        []hyardAdoptCheckNextAction `json:"next_actions"`
}

func TestHyardAdoptSourceJSONDefaultsOrbitIDFromGitRepoRootAndWritesSourceTruth(t *testing.T) {
	t.Parallel()

	rawRepoRoot := newNamedGitRepoForHyardAdopt(t, "docs-source")
	repoRoot, err := filepath.EvalSymlinks(rawRepoRoot)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "content"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, "README.md"),
		[]byte("# Existing authored source\n"),
		0o600,
	))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed authored source")

	stdout, stderr, err := executeHyardCLI(t, filepath.Join(repoRoot, "content"), "adopt", "source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptSourcePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "1.0", payload.SchemaVersion)
	require.Equal(t, repoRoot, payload.RepoRoot)
	require.Equal(t, "source_write", payload.Mode)
	require.Equal(t, "docs-source", payload.AdoptedOrbit.ID)
	require.Equal(t, "repository_root_basename", payload.AdoptedOrbit.DerivedFrom)
	require.Equal(t, filepath.Join(repoRoot, ".harness", "manifest.yaml"), payload.SourceManifestPath)
	require.ElementsMatch(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/docs-source.yaml",
	}, payload.WrittenPaths)
	require.Contains(t, payload.NextActions, hyardAdoptCheckNextAction{
		Command: "hyard publish orbit",
		Reason:  "publish the adopted Orbit Package after review",
	})

	manifestFile, err := harnesspkg.LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, harnesspkg.ManifestKindSource, manifestFile.Kind)
	require.NotNil(t, manifestFile.Source)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs-source"}, manifestFile.Source.Package)
	require.Equal(t, payload.SourceBranch, manifestFile.Source.SourceBranch)

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repoRoot, "docs-source")
	require.NoError(t, err)
	require.Equal(t, "docs-source", spec.ID)
	require.NotNil(t, spec.Meta)
	require.Equal(t, ".harness/orbits/docs-source.yaml", spec.Meta.File)
	require.NoDirExists(t, filepath.Join(repoRoot, ".harness", "agents"))
}

func TestHyardAdoptSourceJSONFailsClosedForInvalidDefaultOrbitID(t *testing.T) {
	t.Parallel()

	rawRepoRoot := newNamedGitRepoForHyardAdopt(t, "Docs Source")
	repoRoot, err := filepath.EvalSymlinks(rawRepoRoot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, "README.md"),
		[]byte("# Existing authored source\n"),
		0o600,
	))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed authored source")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json")
	require.Error(t, err)
	require.ErrorContains(t, err, `Git repo root basename "Docs Source"`)
	require.ErrorContains(t, err, "pass --orbit <orbit-id>")
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.NoDirExists(t, filepath.Join(repoRoot, ".harness"))
}

func TestHyardAdoptSourceJSONUsesExplicitOrbitOverride(t *testing.T) {
	t.Parallel()

	rawRepoRoot := newNamedGitRepoForHyardAdopt(t, "Docs Source")
	repoRoot, err := filepath.EvalSymlinks(rawRepoRoot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(repoRoot, "README.md"),
		[]byte("# Existing authored source\n"),
		0o600,
	))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed authored source")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptSourcePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.AdoptedOrbit.ID)
	require.Equal(t, "flag", payload.AdoptedOrbit.DerivedFrom)

	manifestFile, err := harnesspkg.LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"}, manifestFile.Source.Package)
	require.FileExists(t, filepath.Join(repoRoot, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repoRoot, ".harness", "orbits", "Docs Source.yaml"))
}

func TestHyardAdoptSourceJSONTreatsRootGuidanceAsMetaTemplates(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "docs-source")
	agents := "# Agent guidance\n\nAuthor package guidance here.\n"
	humans := "# Human handoff\n\nRead the source notes.\n"
	bootstrap := "# Bootstrap\n\nPrepare the package workspace.\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "AGENTS.md"), []byte(agents), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "HUMANS.md"), []byte(humans), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "BOOTSTRAP.md"), []byte(bootstrap), 0o600))
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed root guidance")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptSourcePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, payload.WrittenPaths, "AGENTS.md")
	require.NotContains(t, payload.WrittenPaths, "HUMANS.md")
	require.NotContains(t, payload.WrittenPaths, "BOOTSTRAP.md")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repoRoot, "docs-source")
	require.NoError(t, err)
	require.Equal(t, agents, spec.Meta.AgentsTemplate)
	require.Equal(t, humans, spec.Meta.HumansTemplate)
	require.Equal(t, bootstrap, spec.Meta.BootstrapTemplate)
	require.Equal(t, agents, readRepoFile(t, repoRoot, "AGENTS.md"))
	require.Equal(t, humans, readRepoFile(t, repoRoot, "HUMANS.md"))
	require.Equal(t, bootstrap, readRepoFile(t, repoRoot, "BOOTSTRAP.md"))
	require.NotContains(t, readRepoFile(t, repoRoot, "AGENTS.md"), "<!-- orbit:begin")
}

func TestHyardAdoptSourceJSONBackfillsNestedMemberHintsAndPreservesMarkdownMetadata(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "docs-source")
	repoWriteFileForHyardAdoptSource(t, repoRoot, "docs/process/review.md", ""+
		"---\n"+
		"title: Review Flow\n"+
		"orbit_member:\n"+
		"  name: review\n"+
		"  description: Documentation review workflow\n"+
		"  role: process\n"+
		"---\n"+
		"\n"+
		"# Review\n")
	repoWriteFileForHyardAdoptSource(t, repoRoot, "docs/guides/.orbit-member.yaml", ""+
		"orbit_member:\n"+
		"  description: Guide collection\n")
	repoWriteFileForHyardAdoptSource(t, repoRoot, "docs/guides/intro.md", "# Intro\n")
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed member hints")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptSourcePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.WrittenPaths, "docs/process/review.md")
	require.Contains(t, payload.WrittenPaths, "docs/guides/.orbit-member.yaml")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repoRoot, "docs-source")
	require.NoError(t, err)
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name:        "review",
		Description: "Documentation review workflow",
		Role:        orbitpkg.OrbitMemberProcess,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/process/review.md"},
		},
	})
	require.Contains(t, spec.Members, orbitpkg.OrbitMember{
		Name:        "guides",
		Description: "Guide collection",
		Role:        orbitpkg.OrbitMemberProcess,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/guides/**"},
		},
	})
	require.Equal(t, ""+
		"---\n"+
		"title: Review Flow\n"+
		"---\n"+
		"\n"+
		"# Review\n",
		readRepoFile(t, repoRoot, "docs/process/review.md"),
	)
	require.NoFileExists(t, filepath.Join(repoRoot, "docs", "guides", ".orbit-member.yaml"))
}

func TestHyardAdoptSourceJSONDoesNotWriteRuntimeAdoptionTruth(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "docs-source")
	repoWriteFileForHyardAdoptSource(t, repoRoot, "AGENTS.md", "# Agent guidance\n")
	repoWriteFileForHyardAdoptSource(t, repoRoot, ".codex/config.toml", "model = \"gpt-5.4\"\n")
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed codex source footprint")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload hyardAdoptSourcePayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, payload.WrittenPaths, ".harness/agents/config.yaml")
	require.NotContains(t, payload.WrittenPaths, ".harness/agents/manifest.yaml")

	manifestFile, err := harnesspkg.LoadManifestFile(repoRoot)
	require.NoError(t, err)
	require.Equal(t, harnesspkg.ManifestKindSource, manifestFile.Kind)
	require.Nil(t, manifestFile.Runtime)
	require.Empty(t, manifestFile.Members)
	require.NoDirExists(t, filepath.Join(repoRoot, ".harness", "agents"))
	require.NoFileExists(t, filepath.Join(repoRoot, ".harness", "vars.yaml"))
	require.NoFileExists(t, filepath.Join(repoRoot, ".harness", "template.yaml"))
	require.NotContains(t, readRepoFile(t, repoRoot, "AGENTS.md"), "<!-- orbit:begin")
}

func TestHyardAdoptSourceJSONRefusesInvalidMemberHintsBeforeWriting(t *testing.T) {
	t.Parallel()

	repoRoot := newNamedGitRepoForHyardAdopt(t, "docs-source")
	repoWriteFileForHyardAdoptSource(t, repoRoot, "docs/process/review.md", ""+
		"---\n"+
		"orbit_member: review\n"+
		"---\n"+
		"\n"+
		"# Review\n")
	runGitForHyardAdopt(t, repoRoot, "add", "-A")
	runGitForHyardAdopt(t, repoRoot, "commit", "-m", "seed invalid member hint")

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "adopt", "source", "--json")
	require.Error(t, err)
	require.ErrorContains(t, err, "source member hints are not ready")
	require.ErrorContains(t, err, "orbit_member must be a mapping")
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.NoDirExists(t, filepath.Join(repoRoot, ".harness"))
}

func repoWriteFileForHyardAdoptSource(t *testing.T, repoRoot string, relativePath string, contents string) {
	t.Helper()

	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	require.NoError(t, os.MkdirAll(filepath.Dir(absolutePath), 0o750))
	require.NoError(t, os.WriteFile(absolutePath, []byte(contents), 0o600))
}
