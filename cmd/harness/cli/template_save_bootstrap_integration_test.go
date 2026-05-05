package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessTemplateSaveDryRunIgnoresCompletedBootstrapMembersAndWarns(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapTemplateSaveRepo(t)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 9, 0, 0, 0, time.UTC),
		},
	}))
	require.NoError(t, os.Remove(filepath.Join(repo.Root, "bootstrap", "docs", "setup.md")))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files    []string `json:"files"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, ".harness/template_members/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.NotContains(t, payload.Files, "bootstrap/docs/setup.md")
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], `skip bootstrap export paths for orbit "docs" because bootstrap is already completed in this runtime`)
	require.Contains(t, payload.Warnings[0], "bootstrap/docs/setup.md")
}

func TestHarnessTemplateSaveDryRunIncludesCompletedBootstrapMembersWhenExplicitlyRequested(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapTemplateSaveRepo(t)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 9, 0, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run", "--include-bootstrap", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Files    []string `json:"files"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, ".harness/template_members/docs.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.Contains(t, payload.Files, "bootstrap/docs/setup.md")
	require.Empty(t, payload.Warnings)
}

func TestHarnessTemplatePublishIncludesCompletedBootstrapMembersWhenExplicitlyRequested(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBootstrapTemplateSaveRepo(t)
	repo.WriteFile(t, "BOOTSTRAP.md", "Direct bootstrap guidance edit\n")

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	require.NoError(t, store.WriteRuntimeStateSnapshot(statepkg.RuntimeStateSnapshot{
		Orbit: "docs",
		Bootstrap: &statepkg.RuntimeBootstrapState{
			Completed:   true,
			CompletedAt: time.Date(2026, time.April, 19, 9, 0, 0, 0, time.UTC),
		},
	}))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "publish", "--to", "harness-template/workspace", "--include-bootstrap", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	bootstrapMember, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", "bootstrap/docs/setup.md")
	require.NoError(t, err)
	require.Equal(t, "Docs bootstrap setup\n", string(bootstrapMember))

	rootBootstrap, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", "BOOTSTRAP.md")
	require.NoError(t, err)
	require.Equal(t, "Direct bootstrap guidance edit\n", string(rootBootstrap))
}

func seedHarnessBootstrapTemplateSaveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

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
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "bootstrap/docs/setup.md", "Docs bootstrap setup\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed harness bootstrap template save runtime")

	manifestFile, err := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, manifestFile.Members, 1)

	return repo
}
