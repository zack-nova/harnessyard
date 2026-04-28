package packageops_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/packageops"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestRenameHostedOrbitPackageUpdatesSpecAndSourceManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  package:\n"+
		"    type: orbit\n"+
		"    name: docs\n"+
		"  source_branch: main\n")
	spec, err := orbit.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Name = "Docs Workflow"
	spec.Description = "Docs workflow package"
	spec.Members = []orbit.OrbitMember{
		{
			Name: "docs-content",
			Role: orbit.OrbitMemberRule,
			Paths: orbit.OrbitMemberPaths{
				Include: []string{"docs/**"},
			},
		},
	}
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	result, err := packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, "docs", "api")
	require.NoError(t, err)
	require.Equal(t, "docs", result.OldPackage)
	require.Equal(t, "api", result.NewPackage)
	require.Equal(t, ".harness/orbits/docs.yaml", result.OldDefinitionPath)
	require.Equal(t, ".harness/orbits/api.yaml", result.NewDefinitionPath)
	require.Equal(t, ".harness/manifest.yaml", result.ManifestPath)
	require.True(t, result.ManifestChanged)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	renamedSpec, err := orbit.LoadHostedOrbitSpec(context.Background(), repo.Root, "api")
	require.NoError(t, err)
	require.NotNil(t, renamedSpec.Package)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "api"}, *renamedSpec.Package)
	require.Equal(t, "api", renamedSpec.ID)
	require.Equal(t, "Docs Workflow", renamedSpec.Name)
	require.Equal(t, "Docs workflow package", renamedSpec.Description)
	require.NotNil(t, renamedSpec.Meta)
	require.Equal(t, ".harness/orbits/api.yaml", renamedSpec.Meta.File)
	require.Len(t, renamedSpec.Members, 1)
	require.Equal(t, "docs-content", renamedSpec.Members[0].Name)

	manifest, err := harness.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.NotNil(t, manifest.Source)
	require.Equal(t, "api", manifest.Source.OrbitID)
	require.Equal(t, ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "api"}, manifest.Source.Package)
}

func TestRenameHostedOrbitPackageUpdatesPackageOwnedPathSurfacesAndMovesFolders(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	const oldPackage = "development-commit-zh"
	const newPackage = "review-commit-debt"
	spec, err := orbit.DefaultHostedMemberSchemaSpec(oldPackage)
	require.NoError(t, err)
	spec.Members = []orbit.OrbitMember{
		{
			Name: "process",
			Role: orbit.OrbitMemberProcess,
			Paths: orbit.OrbitMemberPaths{
				Include: []string{"guides/development-commit-zh/**/*.md"},
				Exclude: []string{"guides/development-commit-zh/tmp/**"},
			},
		},
	}
	spec.Capabilities = &orbit.OrbitCapabilities{
		Commands: &orbit.OrbitCommandCapabilityPaths{
			Paths: orbit.OrbitMemberPaths{
				Include: []string{"commands/development-commit-zh/**/*.md"},
				Exclude: []string{"commands/development-commit-zh/drafts/**"},
			},
		},
		Skills: &orbit.OrbitSkillCapabilities{
			Local: &orbit.OrbitLocalSkillCapabilityPaths{
				Paths: orbit.OrbitMemberPaths{
					Include: []string{"skills/development-commit-zh/*"},
				},
			},
		},
	}
	spec.AgentAddons = &orbit.OrbitAgentAddons{
		Hooks: &orbit.OrbitAgentHookAddons{
			Entries: []orbit.OrbitAgentHookEntry{
				{
					ID: "review",
					Event: orbit.AgentAddonHookEvent{
						Kind: "tool.before",
					},
					Handler: orbit.AgentAddonHookHandler{
						Type: "command",
						Path: "hooks/development-commit-zh/review.sh",
					},
				},
			},
		},
	}
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "guides/development-commit-zh/process/guide.md", "Guide\n")
	repo.WriteFile(t, "commands/development-commit-zh/review/run.md", "Run\n")
	repo.WriteFile(t, "skills/development-commit-zh/review-commit-debt/SKILL.md", "---\nname: review-commit-debt\n---\n")
	repo.WriteFile(t, "hooks/development-commit-zh/review.sh", "#!/bin/sh\n")

	result, err := packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, oldPackage, newPackage)
	require.NoError(t, err)
	require.ElementsMatch(t, []packageops.RenamedPath{
		{OldPath: "commands/development-commit-zh", NewPath: "commands/review-commit-debt"},
		{OldPath: "guides/development-commit-zh", NewPath: "guides/review-commit-debt"},
		{OldPath: "hooks/development-commit-zh", NewPath: "hooks/review-commit-debt"},
		{OldPath: "skills/development-commit-zh", NewPath: "skills/review-commit-debt"},
	}, result.RenamedPaths)

	renamedSpec, err := orbit.LoadHostedOrbitSpec(context.Background(), repo.Root, newPackage)
	require.NoError(t, err)
	require.Equal(t, []string{"guides/review-commit-debt/**/*.md"}, renamedSpec.Members[0].Paths.Include)
	require.Equal(t, []string{"guides/review-commit-debt/tmp/**"}, renamedSpec.Members[0].Paths.Exclude)
	require.Equal(t, []string{"commands/review-commit-debt/**/*.md"}, renamedSpec.Capabilities.Commands.Paths.Include)
	require.Equal(t, []string{"commands/review-commit-debt/drafts/**"}, renamedSpec.Capabilities.Commands.Paths.Exclude)
	require.Equal(t, []string{"skills/review-commit-debt/*"}, renamedSpec.Capabilities.Skills.Local.Paths.Include)
	require.Equal(t, "hooks/review-commit-debt/review.sh", renamedSpec.AgentAddons.Hooks.Entries[0].Handler.Path)

	for _, path := range []string{
		"guides/review-commit-debt/process/guide.md",
		"commands/review-commit-debt/review/run.md",
		"skills/review-commit-debt/review-commit-debt/SKILL.md",
		"hooks/review-commit-debt/review.sh",
	} {
		_, err = os.Stat(filepath.Join(repo.Root, filepath.FromSlash(path)))
		require.NoError(t, err, path)
	}
	for _, path := range []string{
		"guides/development-commit-zh",
		"commands/development-commit-zh",
		"skills/development-commit-zh",
		"hooks/development-commit-zh",
	} {
		_, err = os.Stat(filepath.Join(repo.Root, filepath.FromSlash(path)))
		require.ErrorIs(t, err, os.ErrNotExist, path)
	}
}

func TestRenameHostedOrbitPackageUpdatesLegacyPathListsAndMovesFiles(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	spec := orbit.OrbitSpec{
		Package: &ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs-workflow"},
		Include: []string{
			"assets/docs-workflow/docs-workflow.md",
			"docs/docs-workflow.md",
			"guides/docs-workflow/**",
		},
		Exclude: []string{"guides/docs-workflow/tmp/**"},
	}
	_, err := orbit.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "assets/docs-workflow/docs-workflow.md", "Nested\n")
	repo.WriteFile(t, "docs/docs-workflow.md", "Spec\n")
	repo.WriteFile(t, "guides/docs-workflow/guide.md", "Guide\n")

	result, err := packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, "docs-workflow", "api-workflow")
	require.NoError(t, err)
	require.ElementsMatch(t, []packageops.RenamedPath{
		{OldPath: "assets/docs-workflow", NewPath: "assets/api-workflow"},
		{OldPath: "assets/api-workflow/docs-workflow.md", NewPath: "assets/api-workflow/api-workflow.md"},
		{OldPath: "docs/docs-workflow.md", NewPath: "docs/api-workflow.md"},
		{OldPath: "guides/docs-workflow", NewPath: "guides/api-workflow"},
	}, result.RenamedPaths)

	renamedSpec, err := orbit.LoadHostedOrbitSpec(context.Background(), repo.Root, "api-workflow")
	require.NoError(t, err)
	require.Equal(t, []string{"assets/api-workflow/api-workflow.md", "docs/api-workflow.md", "guides/api-workflow/**"}, renamedSpec.Include)
	require.Equal(t, []string{"guides/api-workflow/tmp/**"}, renamedSpec.Exclude)
	_, err = os.Stat(filepath.Join(repo.Root, "assets", "api-workflow", "api-workflow.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, "assets", "api-workflow", "docs-workflow.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "docs", "api-workflow.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, "guides", "api-workflow", "guide.md"))
	require.NoError(t, err)
}

func TestRenameHostedOrbitPackageRejectsDestinationCollision(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	docsSpec, err := orbit.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, docsSpec)
	require.NoError(t, err)
	apiSpec, err := orbit.DefaultHostedMemberSchemaSpec("api")
	require.NoError(t, err)
	_, err = orbit.WriteHostedOrbitSpec(repo.Root, apiSpec)
	require.NoError(t, err)

	_, err = packageops.RenameHostedOrbitPackage(context.Background(), repo.Root, "docs", "api")
	require.Error(t, err)
	require.ErrorContains(t, err, `orbit package "api" already exists`)
}
