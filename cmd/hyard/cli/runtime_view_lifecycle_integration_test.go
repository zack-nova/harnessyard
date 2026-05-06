package cli_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardInstallDefaultsRuntimeGuidanceToRunViewPresentation(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var installPayload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &installPayload))
	require.Empty(t, installPayload.Warnings)

	require.Equal(t, "Use docs runtime guidance.\n", readRepoFile(t, repo.Root, "AGENTS.md"))
	require.Equal(t, "Read the docs workflow.\n", readRepoFile(t, repo.Root, "HUMANS.md"))
	require.Equal(t, "Bootstrap the docs workflow.\n", readRepoFile(t, repo.Root, "BOOTSTRAP.md"))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var statusPayload struct {
		SelectedView       string `json:"selected_view"`
		SelectionPersisted bool   `json:"selection_persisted"`
		ActualPresentation struct {
			Mode                      string `json:"mode"`
			AuthoringScaffoldsPresent bool   `json:"authoring_scaffolds_present"`
			GuidanceMarkersPresent    bool   `json:"guidance_markers_present"`
			MemberHintsPresent        bool   `json:"member_hints_present"`
		} `json:"actual_presentation"`
		AllowedPublicationActions []string `json:"allowed_publication_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &statusPayload))

	require.Equal(t, "run", statusPayload.SelectedView)
	require.False(t, statusPayload.SelectionPersisted)
	require.Equal(t, "runtime_content", statusPayload.ActualPresentation.Mode)
	require.False(t, statusPayload.ActualPresentation.AuthoringScaffoldsPresent)
	require.False(t, statusPayload.ActualPresentation.GuidanceMarkersPresent)
	require.False(t, statusPayload.ActualPresentation.MemberHintsPresent)
	require.Equal(t, []string{"current_runtime_harness_package"}, statusPayload.AllowedPublicationActions)
}

func TestHyardCheckTreatsCleanedRunViewRootGuidanceAsPresentationState(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.OK)
	require.Empty(t, payload.Findings)
}

func TestHyardCheckReportsDuplicateMarkedRunViewRootGuidance(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	humansBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Read the docs workflow.\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "HUMANS.md", string(humansBlock)+string(humansBlock))

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.OK)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "root_guidance_invalid",
		Path:    "HUMANS.md",
		Message: `duplicate orbit block for "docs"`,
	})
}

func TestHyardCheckReportsMalformedMarkedRunViewAgentsGuidance(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	repo.WriteFile(t, "AGENTS.md", "<!-- orbit:begin workflow='docs' -->\nUse docs runtime guidance.\n")

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.OK)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "root_guidance_invalid",
		Path:    "AGENTS.md",
		Message: `malformed orbit block marker "<!-- orbit:begin workflow='docs' -->"`,
	})
}

func TestHyardCheckStillReportsNonGuidanceInstallBackedDrift(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewOrbitInstallRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "install", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)

	repo.WriteFile(t, "docs/guide.md", "# Locally changed guide\n")

	stdout, stderr, err = executeHyardCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OK       bool `json:"ok"`
		Findings []struct {
			Kind string `json:"kind"`
			Path string `json:"path"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.OK)
	require.Contains(t, payload.Findings, struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}{
		Kind: "runtime_file_drift",
		Path: "docs/guide.md",
	})
}

func TestHyardCreateAndInitRuntimeDefaultToRunViewStatus(t *testing.T) {
	t.Parallel()

	createRoot := filepath.Join(t.TempDir(), "created-runtime")
	stdout, stderr, err := executeHyardCLI(t, filepath.Dir(createRoot), "create", "runtime", createRoot, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)
	requireHyardRunViewRuntimeContentStatus(t, createRoot)

	initRepo := testutil.NewRepo(t)
	stdout, stderr, err = executeHyardCLI(t, initRepo.Root, "init", "runtime", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.NotEmpty(t, stdout)
	requireHyardRunViewRuntimeContentStatus(t, initRepo.Root)
}

func TestHyardGuideSyncDefaultsRuntimeGuidanceToRunViewPresentation(t *testing.T) {
	t.Parallel()

	repo := seedHyardRunViewGuidanceRuntimeRepo(t)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "guide", "sync", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ArtifactCount int `json:"artifact_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 3, payload.ArtifactCount)

	require.Equal(t, "Use docs runtime guidance.\n", readRepoFile(t, repo.Root, "AGENTS.md"))
	require.Equal(t, "Read the docs workflow.\n", readRepoFile(t, repo.Root, "HUMANS.md"))
	require.Equal(t, "Bootstrap the docs workflow.\n", readRepoFile(t, repo.Root, "BOOTSTRAP.md"))
	requireHyardRunViewRuntimeContentStatus(t, repo.Root)
}

func TestHyardCloneDefaultsInstalledRuntimeToRunViewPresentation(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHyardRunViewHarnessTemplateSourceRepo(t)
	parentDir := filepath.Join(t.TempDir(), "clones")

	stdout, stderr, err := executeHyardCLI(
		t,
		sourceRepo.Root,
		"clone",
		sourceRepo.Root,
		"cloned-runtime",
		"--path",
		parentDir,
		"--ref",
		"harness-template/workspace",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "cloned-runtime", filepath.Base(payload.HarnessRoot))
	require.Equal(t, "Harness template runtime guidance.\n", readRepoFile(t, payload.HarnessRoot, "AGENTS.md"))
	requireHyardRunViewRuntimeContentStatus(t, payload.HarnessRoot)
}

func seedHyardRunViewOrbitInstallRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Use docs runtime guidance.\n"
	spec.Meta.HumansTemplate = "Read the docs workflow.\n"
	spec.Meta.BootstrapTemplate = "Bootstrap the docs workflow.\n"
	spec.Members = []orbitpkg.OrbitMember{{
		Key:  "guide",
		Role: orbitpkg.OrbitMemberSubject,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/**"},
		},
	}}
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.WriteRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberSubject,
	}
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberSubject,
	}
	spec.Behavior.Scope.OrchestrationRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberProcess,
		orbitpkg.OrbitMemberSubject,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	repo.WriteFile(t, "docs/guide.md", "# Guide\n")
	repo.AddAndCommit(t, "seed docs template source")

	_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
		Preview: orbittemplate.TemplateSavePreviewInput{
			RepoRoot:     repo.Root,
			OrbitID:      "docs",
			TargetBranch: "orbit-template/docs",
			Now:          now,
		},
	})
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", filepath.Join(".harness", "orbits", "docs.yaml"), filepath.Join("docs", "guide.md"))
	repo.AddAndCommit(t, "clear docs runtime content")

	return repo
}

func seedHyardRunViewGuidanceRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHyardRuntimeRepo(t)
	writeHyardRunViewGuidanceSpec(t, repo.Root)
	repo.AddAndCommit(t, "seed runtime guidance truth")

	return repo
}

func seedHyardRunViewHarnessTemplateSourceRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 5, 8, 0, 0, 0, time.UTC)
	_, err := harnesspkg.BootstrapRuntimeControlPlane(repo.Root, now)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "# Guide\n")
	repo.WriteFile(t, "AGENTS.md", "Harness template runtime guidance.\n")
	_, err = harnesspkg.AddManualMember(context.Background(), repo.Root, "docs", now)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness template source")

	err = executeHarnessCLIForHyardTest(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)

	return repo
}

func writeHyardRunViewGuidanceSpec(t *testing.T, repoRoot string) {
	t.Helper()

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	spec.Description = "Docs orbit"
	require.NotNil(t, spec.Meta)
	spec.Meta.AgentsTemplate = "Use docs runtime guidance.\n"
	spec.Meta.HumansTemplate = "Read the docs workflow.\n"
	spec.Meta.BootstrapTemplate = "Bootstrap the docs workflow.\n"
	spec.Members = []orbitpkg.OrbitMember{{
		Key:  "guide",
		Role: orbitpkg.OrbitMemberSubject,
		Paths: orbitpkg.OrbitMemberPaths{
			Include: []string{"docs/**"},
		},
	}}
	require.NotNil(t, spec.Behavior)
	spec.Behavior.Scope.WriteRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberSubject,
	}
	spec.Behavior.Scope.ExportRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberSubject,
	}
	spec.Behavior.Scope.OrchestrationRoles = []orbitpkg.OrbitMemberRole{
		orbitpkg.OrbitMemberMeta,
		orbitpkg.OrbitMemberRule,
		orbitpkg.OrbitMemberProcess,
		orbitpkg.OrbitMemberSubject,
	}
	_, err = orbitpkg.WriteHostedOrbitSpec(repoRoot, spec)
	require.NoError(t, err)
}

func requireHyardRunViewRuntimeContentStatus(t *testing.T, repoRoot string) {
	t.Helper()

	stdout, stderr, err := executeHyardCLI(t, repoRoot, "view", "status", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		SelectedView       string `json:"selected_view"`
		SelectionPersisted bool   `json:"selection_persisted"`
		ActualPresentation struct {
			Mode                      string `json:"mode"`
			AuthoringScaffoldsPresent bool   `json:"authoring_scaffolds_present"`
			GuidanceMarkersPresent    bool   `json:"guidance_markers_present"`
			MemberHintsPresent        bool   `json:"member_hints_present"`
		} `json:"actual_presentation"`
		AllowedPublicationActions []string `json:"allowed_publication_actions"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Equal(t, "run", payload.SelectedView)
	require.False(t, payload.SelectionPersisted)
	require.Equal(t, "runtime_content", payload.ActualPresentation.Mode)
	require.False(t, payload.ActualPresentation.AuthoringScaffoldsPresent)
	require.False(t, payload.ActualPresentation.GuidanceMarkersPresent)
	require.False(t, payload.ActualPresentation.MemberHintsPresent)
	require.Equal(t, []string{"current_runtime_harness_package"}, payload.AllowedPublicationActions)
}
