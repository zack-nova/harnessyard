package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

func TestTemplateSaveDryRunIgnoresCompletedBootstrapMembersAndWarns(t *testing.T) {
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
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"  - key: docs-bootstrap\n"+
		"    role: rule\n"+
		"    lane: bootstrap\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - bootstrap/docs/**\n")
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Docs bootstrap setup\n")
	repo.AddAndCommit(t, "add bootstrap lane to docs orbit")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 8, 0, 0, 0, time.UTC),
		},
	}))
	require.NoError(t, os.Remove(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md")))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files    []string `json:"files"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.NotContains(t, payload.Files, "bootstrap/docs/setup.md")
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], `skip bootstrap export paths for orbit "docs" because bootstrap is already completed in this runtime`)
	require.Contains(t, payload.Warnings[0], "bootstrap/docs/setup.md")
}

func TestTemplateSaveDryRunIncludesCompletedBootstrapMembersWhenExplicitlyRequested(t *testing.T) {
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
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"  - key: docs-bootstrap\n"+
		"    role: rule\n"+
		"    lane: bootstrap\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - bootstrap/docs/**\n")
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Docs bootstrap setup\n")
	repo.AddAndCommit(t, "add bootstrap lane to docs orbit")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 8, 0, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs", "--dry-run", "--include-completed-bootstrap", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files    []string `json:"files"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.Contains(t, payload.Files, "bootstrap/docs/setup.md")
	require.Empty(t, payload.Warnings)
}
