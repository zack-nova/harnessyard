package orbittemplate

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestRunTemplateLocalSkillDetectionRejectsOutOfRangeValidSkillsWithoutFlags(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	_, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "extras/research-kit")
	require.ErrorContains(t, err, "--aggregate-detected-skills")
	require.ErrorContains(t, err, "--allow-out-of-range-skills")
}

func TestRunTemplateLocalSkillDetectionAllowsOutOfRangeValidSkillsWithWarning(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	result, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot:              repo.Root,
		OrbitID:               "docs",
		AllowOutOfRangeSkills: true,
	})
	require.NoError(t, err)
	require.False(t, result.Aggregated)
	require.Equal(t, []string{
		`detected valid local skills outside capabilities.skills.local.paths: extras/research-kit; these skills will not take effect unless you expand capabilities.skills.local.paths or move them under skills/docs/*`,
	}, result.Warnings)

	require.FileExists(t, repo.Root+"/extras/research-kit/SKILL.md")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.NotContains(t, spec.Capabilities.Skills.Local.Paths.Include, "skills/docs/*")
}

func TestRunTemplateLocalSkillDetectionAggregatesDetectedSkillsAndUpdatesCapabilityPaths(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	result, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot:                repo.Root,
		OrbitID:                 "docs",
		AggregateDetectedSkills: true,
	})
	require.NoError(t, err)
	require.True(t, result.Aggregated)
	require.Empty(t, result.Warnings)

	require.NoFileExists(t, repo.Root+"/extras/research-kit/SKILL.md")
	require.FileExists(t, repo.Root+"/skills/docs/research-kit/SKILL.md")
	require.FileExists(t, repo.Root+"/skills/docs/research-kit/playbook.md")

	spec, err := orbitpkg.LoadHostedOrbitSpec(context.Background(), repo.Root, "docs")
	require.NoError(t, err)
	require.Contains(t, spec.Capabilities.Skills.Local.Paths.Include, "declared-skills/*")
	require.Contains(t, spec.Capabilities.Skills.Local.Paths.Include, "skills/docs/*")
}

func TestRunTemplateLocalSkillDetectionUsesInteractiveAggregationChoice(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, validOutOfRangeSkillFrontmatter())

	result, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
		ConfirmPrompter: confirmPrompterFunc(func(_ context.Context, prompt string) (bool, error) {
			require.Contains(t, prompt, "extras/research-kit")
			require.Contains(t, prompt, "skills/docs/research-kit")
			return true, nil
		}),
	})
	require.NoError(t, err)
	require.True(t, result.Aggregated)
	require.FileExists(t, repo.Root+"/skills/docs/research-kit/SKILL.md")
}

func TestRunTemplateLocalSkillDetectionRejectsAggregationWhenExistingExcludesStillBlockDefaultSkillPath(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepoWithCapabilityPaths(
		t,
		validOutOfRangeSkillFrontmatter(),
		[]string{"declared-skills/*"},
		[]string{"skills/docs/**"},
	)

	_, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot:                repo.Root,
		OrbitID:                 "docs",
		AggregateDetectedSkills: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `skills/docs/research-kit`)
	require.ErrorContains(t, err, "capabilities.skills.local.paths")
	require.FileExists(t, repo.Root+"/extras/research-kit/SKILL.md")
	require.NoFileExists(t, repo.Root+"/skills/docs/research-kit/SKILL.md")
}

func TestRunTemplateLocalSkillDetectionIgnoresInvalidOutOfRangeSkillRoots(t *testing.T) {
	t.Parallel()

	repo := seedTemplateSkillDetectionSourceRepo(t, invalidOutOfRangeSkillFrontmatter())

	result, err := RunTemplateLocalSkillDetection(context.Background(), TemplateLocalSkillDetectionInput{
		RepoRoot: repo.Root,
		OrbitID:  "docs",
	})
	require.NoError(t, err)
	require.False(t, result.Aggregated)
	require.Empty(t, result.Warnings)
	require.Empty(t, result.Detected)
	require.FileExists(t, repo.Root+"/extras/research-kit/SKILL.md")
}

func seedTemplateSkillDetectionSourceRepo(t *testing.T, outOfRangeSkillMD string) *testutil.Repo {
	t.Helper()

	return seedTemplateSkillDetectionSourceRepoWithCapabilityPaths(
		t,
		outOfRangeSkillMD,
		[]string{"declared-skills/*"},
		nil,
	)
}

func seedTemplateSkillDetectionSourceRepoWithCapabilityPaths(
	t *testing.T,
	outOfRangeSkillMD string,
	includes []string,
	excludes []string,
) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")
	_, err := WriteSourceManifest(repo.Root, SourceManifest{
		SchemaVersion: sourceSchemaVersion,
		Kind:          SourceKind,
		SourceBranch:  "main",
		Publish: &SourcePublishConfig{
			OrbitID: "docs",
		},
	})
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
		"capabilities:\n"+
		"  skills:\n"+
		"    local:\n"+
		renderSkillCapabilityPathsYAML(includes, excludes)+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"        - extras/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "declared-skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "declared-skills/docs-style/checklist.md", "Use docs style guide.\n")
	repo.WriteFile(t, "extras/research-kit/SKILL.md", outOfRangeSkillMD)
	repo.WriteFile(t, "extras/research-kit/playbook.md", "Use research kit.\n")
	repo.AddAndCommit(t, "seed template skill detection source repo")

	return repo
}

func renderSkillCapabilityPathsYAML(includes []string, excludes []string) string {
	var builder strings.Builder
	builder.WriteString("      paths:\n")
	builder.WriteString("        include:\n")
	for _, include := range includes {
		builder.WriteString("          - " + include + "\n")
	}
	if len(excludes) > 0 {
		builder.WriteString("        exclude:\n")
		for _, exclude := range excludes {
			builder.WriteString("          - " + exclude + "\n")
		}
	}
	return builder.String()
}

func validOutOfRangeSkillFrontmatter() string {
	return "" +
		"---\n" +
		"name: research-kit\n" +
		"description: Research kit references.\n" +
		"---\n" +
		"# Research Kit\n"
}

func invalidOutOfRangeSkillFrontmatter() string {
	return "" +
		"---\n" +
		"name: research-kit\n" +
		"---\n" +
		"# Research Kit\n"
}

type confirmPrompterFunc func(ctx context.Context, prompt string) (bool, error)

func (fn confirmPrompterFunc) Confirm(ctx context.Context, prompt string) (bool, error) {
	return fn(ctx, prompt)
}
