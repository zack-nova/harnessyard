package branchinfo

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestClassifyRevisionReturnsOrbitTemplateForValidManifestRegardlessOfBranchName(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: orbit_template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "add template branch contract")
	repo.Run(t, "branch", "-M", "not-a-template-name")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:         KindTemplate,
		TemplateKind: TemplateKindOrbit,
		Reason:       "valid .harness/manifest.yaml present with kind=orbit_template",
	}, result)

	resultByBranch, err := ClassifyRevision(context.Background(), repo.Root, "not-a-template-name")
	require.NoError(t, err)
	require.Equal(t, result, resultByBranch)
}

func TestClassifyRevisionReturnsHarnessTemplateForValidManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"root_guidance:\n"+
		"  agents: false\n"+
		"  humans: false\n"+
		"  bootstrap: false\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "template content\n")
	repo.AddAndCommit(t, "add harness template branch contract")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:         KindTemplate,
		TemplateKind: TemplateKindHarness,
		Reason:       "valid .harness/manifest.yaml present with kind=harness_template",
	}, result)
}

func TestClassifyRevisionReturnsRuntimeForValidRuntimeManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "runtime content\n")
	repo.AddAndCommit(t, "add runtime control plane")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:   KindRuntime,
		Reason: "valid .harness/manifest.yaml present with kind=runtime",
	}, result)
}

func TestClassifyRevisionReturnsSourceForValidSourceManifest(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"  orbit_id: docs\n"+
		"  source_branch: main\n")
	repo.WriteFile(t, ".orbit/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "source content\n")
	repo.AddAndCommit(t, "add source control plane")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:   KindSource,
		Reason: "valid .harness/manifest.yaml present with kind=source",
	}, result)
}

func TestClassifyRevisionFallsBackToPlainWhenManifestIsInvalid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: Docs\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, "README.md", "plain branch\n")
	repo.AddAndCommit(t, "add invalid manifest")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, KindPlain, result.Kind)
	require.Contains(t, result.Reason, "invalid .harness/manifest.yaml")
}

func TestClassifyRevisionFallsBackToPlainWhenManifestIsMissingEvenIfLegacyMarkersExist(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n")
	repo.WriteFile(t, ".harness/runtime.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_runtime\n"+
		"harness:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "seed legacy markers only")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:   KindPlain,
		Reason: "no valid .harness/manifest.yaml found",
	}, result)
}

func TestClassifyRevisionRecognizesZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.AddAndCommit(t, "add zero-member runtime control plane")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, KindRuntime, result.Kind)
	require.Equal(t, "valid .harness/manifest.yaml present with kind=runtime", result.Reason)
}

func TestClassifyRevisionPrefersManifestOverLegacyMarkers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".orbit/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: template\n"+
		"template:\n"+
		"  orbit_id: docs\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-21T10:00:00Z\n"+
		"variables: {}\n")
	repo.WriteFile(t, ".harness/template.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template\n"+
		"template:\n"+
		"  harness_id: workspace\n"+
		"  default_template: false\n"+
		"  created_from_branch: main\n"+
		"  created_from_commit: abc123\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  root_guidance:\n"+
		"    agents: false\n"+
		"    humans: false\n"+
		"    bootstrap: false\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"variables: {}\n")
	repo.AddAndCommit(t, "seed manifest and legacy markers")

	result, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)
	require.Equal(t, Classification{
		Kind:   KindRuntime,
		Reason: "valid .harness/manifest.yaml present with kind=runtime",
	}, result)
}

func TestClassifyRevisionMatchesCheckedOutAndUncheckedOutRevision(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members: []\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", "id: docs\ninclude:\n  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "runtime branch\n")
	repo.AddAndCommit(t, "add runtime branch")
	runtimeRevision := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	repo.Run(t, "branch", "runtime-view")
	repo.Run(t, "checkout", "-b", "plain-view")
	repo.Run(t, "rm", "-r", ".harness")
	repo.AddAndCommit(t, "remove manifest control plane")

	uncheckedOut, err := ClassifyRevision(context.Background(), repo.Root, "runtime-view")
	require.NoError(t, err)

	repo.Run(t, "checkout", "runtime-view")
	currentHead, err := ClassifyRevision(context.Background(), repo.Root, "HEAD")
	require.NoError(t, err)

	byRevision, err := ClassifyRevision(context.Background(), repo.Root, runtimeRevision)
	require.NoError(t, err)

	require.Equal(t, KindRuntime, uncheckedOut.Kind)
	require.Equal(t, uncheckedOut, currentHead)
	require.Equal(t, uncheckedOut, byRevision)
}
