package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestInstallReturnsWarningAndRollsBackGuidanceArtifactsWhenScopedGuidanceFails(t *testing.T) {
	repo := seedInstallGuidanceCommandRepo(t, []installGuidanceCommandTemplateSpec{{
		OrbitID:        "docs",
		AgentsTemplate: "You are the $project_name docs orbit.\n",
		Files: map[string]string{
			"docs/guide.md": "$project_name guide\n",
		},
	}})
	bindingsPath := writeInstallGuidanceCommandBindings(t, repo.Root)

	composeRuntimeGuidanceForInstall = func(_ context.Context, input harnesspkg.ComposeRuntimeGuidanceInput) (harnesspkg.ComposeRuntimeGuidanceResult, error) {
		require.Equal(t, repo.Root, input.RepoRoot)
		require.Equal(t, harnesspkg.GuidanceTargetAll, input.Target)
		require.Equal(t, []string{"docs"}, input.OrbitIDs)

		installRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
		require.NoError(t, err)
		require.Equal(t, "docs", installRecord.OrbitID)

		guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
		require.NoError(t, err)
		require.Equal(t, "Installed Orbit guide\n", string(guideData))

		require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "AGENTS.md"), []byte("mutated guidance\n"), 0o600))
		return harnesspkg.ComposeRuntimeGuidanceResult{}, fmt.Errorf("injected compose failure")
	}
	t.Cleanup(func() {
		composeRuntimeGuidanceForInstall = harnesspkg.ComposeRuntimeGuidance
	})

	stdout, stderr, err := executeInstallCommand(t, repo.Root, "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
		MemberCount  int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.NotContains(t, payload.WrittenPaths, "AGENTS.md")
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], "scoped guidance compose was rolled back")
	require.Contains(t, payload.Warnings[0], "hyard guide sync --target all")

	_, statErr := os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))
}

func TestInstallBatchReturnsWarningAndRollsBackGuidanceArtifactsWhenScopedGuidanceFails(t *testing.T) {
	repo := seedInstallGuidanceCommandRepo(t, []installGuidanceCommandTemplateSpec{
		{
			OrbitID:        "docs",
			AgentsTemplate: "You are the $project_name docs orbit.\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID:        "cmd",
			AgentsTemplate: "Use the $project_name cmd release flow.\n",
			Files: map[string]string{
				"cmd/README.md": "$project_name cmd guide\n",
			},
		},
	})
	bindingsPath := writeInstallGuidanceCommandBindings(t, repo.Root)

	composeRuntimeGuidanceForInstall = func(_ context.Context, input harnesspkg.ComposeRuntimeGuidanceInput) (harnesspkg.ComposeRuntimeGuidanceResult, error) {
		require.Equal(t, repo.Root, input.RepoRoot)
		require.Equal(t, harnesspkg.GuidanceTargetAll, input.Target)
		require.Equal(t, []string{"docs", "cmd"}, input.OrbitIDs)

		_, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
		require.NoError(t, err)
		_, err = harnesspkg.LoadInstallRecord(repo.Root, "cmd")
		require.NoError(t, err)

		guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
		require.NoError(t, err)
		require.Equal(t, "Installed Orbit guide\n", string(guideData))

		cmdData, err := os.ReadFile(filepath.Join(repo.Root, "cmd", "README.md"))
		require.NoError(t, err)
		require.Equal(t, "Installed Orbit cmd guide\n", string(cmdData))

		require.NoError(t, os.WriteFile(filepath.Join(repo.Root, "AGENTS.md"), []byte("mutated batch guidance\n"), 0o600))
		return harnesspkg.ComposeRuntimeGuidanceResult{}, fmt.Errorf("injected batch compose failure")
	}
	t.Cleanup(func() {
		composeRuntimeGuidanceForInstall = harnesspkg.ComposeRuntimeGuidance
	})

	stdout, stderr, err := executeInstallBatchCommand(t, repo.Root, "orbit-template/docs", "orbit-template/cmd", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitIDs     []string `json:"orbit_ids"`
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
		MemberCount  int      `json:"member_count"`
		Items        []struct {
			OrbitID      string   `json:"orbit_id"`
			WrittenPaths []string `json:"written_paths"`
			Warnings     []string `json:"warnings"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"docs", "cmd"}, payload.OrbitIDs)
	require.Equal(t, 2, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")
	require.Contains(t, payload.WrittenPaths, "cmd/README.md")
	require.NotContains(t, payload.WrittenPaths, "AGENTS.md")
	require.Len(t, payload.Warnings, 1)
	require.Contains(t, payload.Warnings[0], "scoped guidance compose was rolled back")
	require.Contains(t, payload.Warnings[0], "hyard guide sync --target all")
	require.Len(t, payload.Items, 2)
	for _, item := range payload.Items {
		require.Empty(t, item.Warnings)
		require.NotContains(t, item.WrittenPaths, "AGENTS.md")
	}

	_, statErr := os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	cmdData, err := os.ReadFile(filepath.Join(repo.Root, "cmd", "README.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit cmd guide\n", string(cmdData))
}

type installGuidanceCommandTemplateSpec struct {
	OrbitID        string
	AgentsTemplate string
	Files          map[string]string
}

func seedInstallGuidanceCommandRepo(t *testing.T, templates []installGuidanceCommandTemplateSpec) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	for _, template := range templates {
		repo.WriteFile(t, ".harness/vars.yaml", ""+
			"schema_version: 1\n"+
			"variables:\n"+
			"  project_name:\n"+
			"    value: Orbit\n"+
			"    description: Product title\n")

		spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(template.OrbitID)
		require.NoError(t, err)
		require.NotNil(t, spec.Meta)
		spec.Meta.AgentsTemplate = template.AgentsTemplate
		spec.Members = []orbitpkg.OrbitMember{{
			Key:  template.OrbitID + "-content",
			Role: orbitpkg.OrbitMemberSubject,
			Paths: orbitpkg.OrbitMemberPaths{
				Include: []string{template.OrbitID + "/**"},
			},
		}}
		require.NotNil(t, spec.Behavior)
		spec.Behavior.Scope.WriteRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberSubject}
		spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberSubject}
		spec.Behavior.Scope.OrchestrationRoles = []orbitpkg.OrbitMemberRole{orbitpkg.OrbitMemberMeta, orbitpkg.OrbitMemberRule, orbitpkg.OrbitMemberProcess, orbitpkg.OrbitMemberSubject}
		_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
		require.NoError(t, err)

		for path, content := range template.Files {
			repo.WriteFile(t, path, content)
		}
		repo.AddAndCommit(t, "seed "+template.OrbitID+" template source")

		_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
			Preview: orbittemplate.TemplateSavePreviewInput{
				RepoRoot:     repo.Root,
				OrbitID:      template.OrbitID,
				TargetBranch: "orbit-template/" + template.OrbitID,
				Now:          now,
			},
		})
		require.NoError(t, err)

		rmArgs := []string{"rm", "-f", filepath.Join(".harness", "orbits", template.OrbitID+".yaml"), ".harness/vars.yaml"}
		for path := range template.Files {
			rmArgs = append(rmArgs, path)
		}
		repo.Run(t, rmArgs...)
		repo.AddAndCommit(t, "clear "+template.OrbitID+" runtime content")
	}

	return repo
}

func writeInstallGuidanceCommandBindings(t *testing.T, repoRoot string) string {
	t.Helper()

	bindingsPath := filepath.Join(repoRoot, "install-guidance-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))
	return bindingsPath
}
