package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harnesscli "github.com/zack-nova/harnessyard/cmd/harness/cli"
	harnesscommands "github.com/zack-nova/harnessyard/cmd/harness/cli/commands"
	orbitcli "github.com/zack-nova/harnessyard/cmd/orbit/cli"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHarnessInitCreatesManifestAndHostedOrbitsDir(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "initialized harness in "+repo.Root+"\n", stdout)

	manifestFile, err := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, harnesspkg.ManifestKindRuntime, manifestFile.Kind)
	require.Equal(t, []harnesspkg.ManifestMember{}, manifestFile.Members)

	orbitSpecsDirInfo, err := os.Stat(filepath.Join(repo.Root, ".harness", "orbits"))
	require.NoError(t, err)
	require.True(t, orbitSpecsDirInfo.IsDir())

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "runtime.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(repo.Root, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessCreateInitializesNewRepository(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	resolvedTargetPath, err := filepath.EvalSymlinks(baseDir)
	require.NoError(t, err)
	resolvedTargetPath = filepath.Join(resolvedTargetPath, "Project A")

	stdout, stderr, err := executeHarnessCLI(t, baseDir, "create", "Project A", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot    string `json:"harness_root"`
		ManifestPath   string `json:"manifest_path"`
		OrbitsDir      string `json:"orbits_dir"`
		GitInitialized bool   `json:"git_initialized"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, resolvedTargetPath, payload.HarnessRoot)
	require.Equal(t, filepath.Join(resolvedTargetPath, ".harness", "manifest.yaml"), payload.ManifestPath)
	require.Equal(t, filepath.Join(resolvedTargetPath, ".harness", "orbits"), payload.OrbitsDir)
	require.True(t, payload.GitInitialized)

	_, err = os.Stat(filepath.Join(resolvedTargetPath, ".git"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(resolvedTargetPath, ".harness", "runtime.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInitReportsCreateGuidanceOutsideGitRepository(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()

	_, stderr, err := executeHarnessCLI(t, workingDir, "init")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "current directory is not a Git repository")
	require.ErrorContains(t, err, "to start a new harness runtime repo here, run:")
	require.ErrorContains(t, err, "harness create .")
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
	require.NotContains(t, err.Error(), "discover git repository")
}

func TestHarnessInitPathReportsCreateGuidanceForRequestedPath(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "demo-runtime"), 0o755))

	_, stderr, err := executeHarnessCLI(t, workingDir, "init", "--path", "demo-runtime")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `path "demo-runtime" is not a Git repository`)
	require.ErrorContains(t, err, "to start a new harness runtime repo there, run:")
	require.ErrorContains(t, err, "harness create demo-runtime")
	require.NotContains(t, err.Error(), "git rev-parse --show-toplevel")
	require.NotContains(t, err.Error(), "discover git repository")
}

func TestHarnessRootPrintsResolvedRootFromSubdirectory(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	subdir := filepath.Join(repo.Root, "nested", "path")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	stdout, stderr, err := executeHarnessCLI(t, subdir, "root")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, repo.Root+"\n", stdout)
}

func TestHarnessInspectReportsZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot          string   `json:"harness_root"`
		HarnessID            string   `json:"harness_id"`
		MemberCount          int      `json:"member_count"`
		Members              []string `json:"members"`
		VarsCount            int      `json:"vars_count"`
		InstallCount         int      `json:"install_count"`
		DetachedInstallCount int      `json:"detached_install_count"`
		InvalidInstallCount  int      `json:"invalid_install_count"`
		BundleCount          int      `json:"bundle_count"`
		CurrentProjection    string   `json:"current_projection"`
		Readiness            struct {
			Status  string `json:"status"`
			Summary struct {
				OrbitCount          int `json:"orbit_count"`
				BlockingReasonCount int `json:"blocking_reason_count"`
				AdvisoryReasonCount int `json:"advisory_reason_count"`
			} `json:"summary"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, 0, payload.MemberCount)
	require.Empty(t, payload.Members)
	require.Equal(t, 0, payload.VarsCount)
	require.Equal(t, 0, payload.InstallCount)
	require.Equal(t, 0, payload.DetachedInstallCount)
	require.Equal(t, 0, payload.InvalidInstallCount)
	require.Equal(t, 0, payload.BundleCount)
	require.Empty(t, payload.CurrentProjection)
	require.Equal(t, "ready", payload.Readiness.Status)
	require.Equal(t, 0, payload.Readiness.Summary.OrbitCount)
	require.Equal(t, 0, payload.Readiness.Summary.BlockingReasonCount)
	require.Equal(t, 0, payload.Readiness.Summary.AdvisoryReasonCount)
}

func TestHarnessInspectTextOutputForZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "inspect")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"harness_root: "+repo.Root+"\n"+
		"harness_id: "+harnesspkg.DefaultHarnessIDForPath(repo.Root)+"\n"+
		"harness_name: "+filepath.Base(repo.Root)+"\n"+
		"member_count: 0\n"+
		"members: none\n"+
		"vars_count: 0\n"+
		"install_count: 0\n"+
		"detached_install_count: 0\n"+
		"invalid_install_count: 0\n"+
		"bundle_count: 0\n"+
		"current_projection: none\n"+
		"readiness_status: ready\n"+
		"readiness_orbit_count: 0\n"+
		"readiness_blocking_reason_count: 0\n"+
		"readiness_advisory_reason_count: 0\n", stdout)
}

func TestHarnessInspectReportsBundleCount(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:      1,
		HarnessID:          "workspace",
		Template:           orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/workspace", TemplateCommit: "abc123"},
		MemberIDs:          []string{"docs"},
		AppliedAt:          time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		InstallCount int `json:"install_count"`
		BundleCount  int `json:"bundle_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 0, payload.InstallCount)
	require.Equal(t, 1, payload.BundleCount)
}

func TestHarnessInspectSeparatesDetachedInstallCount(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 18, 30, 0, 0, time.UTC),
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		InstallCount         int `json:"install_count"`
		DetachedInstallCount int `json:"detached_install_count"`
		Readiness            struct {
			Status  string `json:"status"`
			Summary struct {
				OrbitCount int `json:"orbit_count"`
			} `json:"summary"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 0, payload.InstallCount)
	require.Equal(t, 1, payload.DetachedInstallCount)
	require.Equal(t, "ready", payload.Readiness.Status)
	require.Zero(t, payload.Readiness.Summary.OrbitCount)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "inspect")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "install_count: 0\n")
	require.Contains(t, stdout, "detached_install_count: 1\n")
	require.Contains(t, stdout, "readiness_status: ready\n")
}

func TestHarnessInspectSeparatesInvalidInstallCount(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	brokenPath, err := harnesspkg.InstallRecordPath(repo.Root, "broken")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(brokenPath), 0o755))
	require.NoError(t, os.WriteFile(brokenPath, []byte("schema_version: [\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		InstallCount         int `json:"install_count"`
		DetachedInstallCount int `json:"detached_install_count"`
		InvalidInstallCount  int `json:"invalid_install_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 0, payload.InstallCount)
	require.Equal(t, 0, payload.DetachedInstallCount)
	require.Equal(t, 1, payload.InvalidInstallCount)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "inspect")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "install_count: 0\n")
	require.Contains(t, stdout, "detached_install_count: 0\n")
	require.Contains(t, stdout, "invalid_install_count: 1\n")
}

func TestOrbitBranchInspectSeparatesDetachedAndInvalidInstallCounts(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	manifest, err := harnesspkg.DefaultRuntimeManifestFile(repo.Root, time.Date(2026, time.April, 16, 22, 20, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, manifest)
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 22, 25, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "api",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/api",
			TemplateCommit: "def456",
		},
		AppliedAt: time.Date(2026, time.April, 16, 22, 30, 0, 0, time.UTC),
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)
	repo.WriteFile(t, filepath.Join(".harness", "installs", "broken.yaml"), "schema_version: [\n")
	repo.AddAndCommit(t, "seed branch inspect install provenance")

	stdout, stderr, err := executeOrbitCLI(t, repo.Root, "branch", "inspect", "HEAD", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Inspection struct {
			InstallCount         int      `json:"install_count"`
			InstallIDs           []string `json:"install_ids"`
			DetachedInstallCount int      `json:"detached_install_count"`
			DetachedInstallIDs   []string `json:"detached_install_ids"`
			InvalidInstallCount  int      `json:"invalid_install_count"`
			InvalidInstallIDs    []string `json:"invalid_install_ids"`
		} `json:"inspection"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.Inspection.InstallCount)
	require.Equal(t, []string{"docs"}, payload.Inspection.InstallIDs)
	require.Equal(t, 1, payload.Inspection.DetachedInstallCount)
	require.Equal(t, []string{"api"}, payload.Inspection.DetachedInstallIDs)
	require.Equal(t, 1, payload.Inspection.InvalidInstallCount)
	require.Equal(t, []string{"broken"}, payload.Inspection.InvalidInstallIDs)

	stdout, stderr, err = executeOrbitCLI(t, repo.Root, "branch", "inspect", "HEAD")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "install_count: 1\n")
	require.Contains(t, stdout, "install_ids: [docs]\n")
	require.Contains(t, stdout, "detached_install_count: 1\n")
	require.Contains(t, stdout, "detached_install_ids: [api]\n")
	require.Contains(t, stdout, "invalid_install_count: 1\n")
	require.Contains(t, stdout, "invalid_install_ids: [broken]\n")
}

func TestHarnessFrameworkListReportsSupportedRecommendedAndResolvedFrameworks(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "list", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot          string   `json:"harness_root"`
		HarnessID            string   `json:"harness_id"`
		SupportedFrameworks  []string `json:"supported_frameworks"`
		RecommendedFramework string   `json:"recommended_framework"`
		ResolvedFramework    string   `json:"resolved_framework"`
		ResolutionSource     string   `json:"resolution_source"`
		Warnings             []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, []string{"claudecode", "codex", "openclaw"}, payload.SupportedFrameworks)
	require.Equal(t, "claudecode", payload.RecommendedFramework)
	require.Equal(t, "claudecode", payload.ResolvedFramework)
	require.Equal(t, "recommended_default", payload.ResolutionSource)
	require.Empty(t, payload.Warnings)
}

func TestHarnessFrameworkUseWritesSelectionAndInspectPrefersExplicitLocalFramework(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "use", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var usePayload struct {
		HarnessRoot     string `json:"harness_root"`
		HarnessID       string `json:"harness_id"`
		Framework       string `json:"framework"`
		SelectionSource string `json:"selection_source"`
		SelectionPath   string `json:"selection_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &usePayload))
	require.Equal(t, repo.Root, usePayload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), usePayload.HarnessID)
	require.Equal(t, "codex", usePayload.Framework)
	require.Equal(t, "explicit_local", usePayload.SelectionSource)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "selection.json"), usePayload.SelectionPath)

	selection, err := harnesspkg.LoadFrameworkSelection(filepath.Join(repo.Root, ".git"))
	require.NoError(t, err)
	require.Equal(t, "codex", selection.SelectedFramework)
	require.Equal(t, harnesspkg.FrameworkSelectionSourceExplicitLocal, selection.SelectionSource)
	require.False(t, selection.UpdatedAt.IsZero())

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var inspectPayload struct {
		ResolvedFramework string `json:"resolved_framework"`
		ResolutionSource  string `json:"resolution_source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &inspectPayload))
	require.Equal(t, "codex", inspectPayload.ResolvedFramework)
	require.Equal(t, "explicit_local", inspectPayload.ResolutionSource)
}

func TestHarnessFrameworkRecommendSetAndShowDoNotMutateLocalSelection(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "recommend", "set", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var setPayload struct {
		HarnessRoot          string `json:"harness_root"`
		HarnessID            string `json:"harness_id"`
		FrameworksPath       string `json:"frameworks_path"`
		RecommendedFramework string `json:"recommended_framework"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &setPayload))
	require.Equal(t, repo.Root, setPayload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), setPayload.HarnessID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"), setPayload.FrameworksPath)
	require.Equal(t, "codex", setPayload.RecommendedFramework)

	frameworksData, err := os.ReadFile(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(frameworksData), "recommended_framework: codex\n")
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "frameworks.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "recommend", "show", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var showPayload struct {
		RecommendedFramework string `json:"recommended_framework"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &showPayload))
	require.Equal(t, "codex", showPayload.RecommendedFramework)

	_, _, err = executeHarnessCLI(t, repo.Root, "framework", "use", "claude", "--json")
	require.NoError(t, err)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var inspectPayload struct {
		RecommendedFramework string `json:"recommended_framework"`
		ResolvedFramework    string `json:"resolved_framework"`
		ResolutionSource     string `json:"resolution_source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &inspectPayload))
	require.Equal(t, "codex", inspectPayload.RecommendedFramework)
	require.Equal(t, "claudecode", inspectPayload.ResolvedFramework)
	require.Equal(t, "explicit_local", inspectPayload.ResolutionSource)
}

func TestHarnessFrameworkInspectSummarizesRuntimeGuidanceAndCapabilities(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot          string   `json:"harness_root"`
		HarnessID            string   `json:"harness_id"`
		RecommendedFramework string   `json:"recommended_framework"`
		ResolvedFramework    string   `json:"resolved_framework"`
		ResolutionSource     string   `json:"resolution_source"`
		OrbitCount           int      `json:"orbit_count"`
		CommandCount         int      `json:"command_count"`
		SkillCount           int      `json:"skill_count"`
		HasAgentGuidance     bool     `json:"has_agent_guidance"`
		HasHumanGuidance     bool     `json:"has_human_guidance"`
		OrbitIDs             []string `json:"orbit_ids"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "claudecode", payload.RecommendedFramework)
	require.Equal(t, "claudecode", payload.ResolvedFramework)
	require.Equal(t, "recommended_default", payload.ResolutionSource)
	require.Equal(t, 1, payload.OrbitCount)
	require.Equal(t, 1, payload.CommandCount)
	require.Equal(t, 1, payload.SkillCount)
	require.True(t, payload.HasAgentGuidance)
	require.True(t, payload.HasHumanGuidance)
	require.Equal(t, []string{"docs"}, payload.OrbitIDs)
}

func TestHarnessAgentRecommendSetAndUseWriteAgentHosts(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "recommend", "set", "codex", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var recommendPayload struct {
		FrameworksPath       string `json:"frameworks_path"`
		RecommendedFramework string `json:"recommended_framework"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &recommendPayload))
	require.Equal(t, filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"), recommendPayload.FrameworksPath)
	require.Equal(t, "codex", recommendPayload.RecommendedFramework)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "frameworks.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "agent", "use", "claude", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var usePayload struct {
		Framework     string `json:"framework"`
		SelectionPath string `json:"selection_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &usePayload))
	require.Equal(t, "claudecode", usePayload.Framework)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "selection.json"), usePayload.SelectionPath)

	selection, err := harnesspkg.LoadFrameworkSelection(filepath.Join(repo.Root, ".git"))
	require.NoError(t, err)
	require.Equal(t, "claudecode", selection.SelectedFramework)
}

func TestHarnessAgentDeriveWritesRuntimeTruthFromInstalledBundleSnapshot(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	sourceRepo.WriteFile(t, harnesspkg.FrameworksRepoPath(), ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	sourceRepo.WriteFile(t, harnesspkg.AgentConfigRepoPath(), ""+
		"schema_version: 1\n")
	sourceRepo.WriteFile(t, harnesspkg.AgentOverlayRepoPath("claude"), ""+
		"schema_version: 1\n"+
		"mode: raw_passthrough\n"+
		"raw:\n"+
		"  profile: strict\n")
	sourceRepo.AddAndCommit(t, "add agent package truth to source runtime")

	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)

	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root))
	require.NoError(t, err)
	require.NotNil(t, bundleRecord.AgentConfig)
	require.Equal(t, 1, bundleRecord.AgentConfig.SchemaVersion)
	require.Equal(t, map[string]string{
		"claude": "" +
			"schema_version: 1\n" +
			"mode: raw_passthrough\n" +
			"raw:\n" +
			"  profile: strict\n",
	}, bundleRecord.AgentOverlays)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "agent", "derive", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot          string   `json:"harness_root"`
		HarnessID            string   `json:"harness_id"`
		RecommendedFramework string   `json:"recommended_framework,omitempty"`
		WrittenPaths         []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(runtimeRepo.Root), payload.HarnessID)
	require.Equal(t, "claude", payload.RecommendedFramework)
	require.Contains(t, payload.WrittenPaths, ".harness/agents/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/agents/agent.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/agents/overlays/claude.yaml")

	manifestData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "agents", "manifest.yaml"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n", string(manifestData))

	agentData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "agents", "agent.yaml"))
	require.NoError(t, err)
	require.Equal(t, "schema_version: 1\n", string(agentData))

	overlayData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "agents", "overlays", "claude.yaml"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"mode: raw_passthrough\n"+
		"raw:\n"+
		"  profile: strict\n", string(overlayData))
}

func TestHarnessAgentDeriveFailsClosedOnOverlayConflict(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = []harnesspkg.RuntimeMember{
		{
			OrbitID:        "docs",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: "docs_stack",
			AddedAt:        time.Date(2026, time.April, 23, 15, 0, 0, 0, time.UTC),
		},
		{
			OrbitID:        "ops",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: "ops_stack",
			AddedAt:        time.Date(2026, time.April, 23, 15, 1, 0, 0, time.UTC),
		},
	}
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "docs_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/docs-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"docs"},
		AppliedAt:            time.Date(2026, time.April, 23, 15, 0, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"docs/guide.md"},
		AgentConfig:          &harnesspkg.AgentConfigFile{SchemaVersion: 1},
		AgentOverlays: map[string]string{
			"claude": "" +
				"schema_version: 1\n" +
				"mode: raw_passthrough\n" +
				"raw:\n" +
				"  profile: strict\n",
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "ops_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRef: "harness-template/ops-stack", TemplateCommit: "bbb222"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"ops"},
		AppliedAt:            time.Date(2026, time.April, 23, 15, 1, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"ops/runbook.md"},
		AgentConfig:          &harnesspkg.AgentConfigFile{SchemaVersion: 1},
		AgentOverlays: map[string]string{
			"claude": "" +
				"schema_version: 1\n" +
				"mode: raw_passthrough\n" +
				"raw:\n" +
				"  profile: relaxed\n",
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "derive", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `overlay`)
	require.ErrorContains(t, err, `claude`)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "agents", "manifest.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "agents", "agent.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "agents", "overlays", "claude.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessFrameworkInspectReportsPackageRecommendationConflict(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkConflictRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RecommendedFramework   string `json:"recommended_framework"`
		ResolvedFramework      string `json:"resolved_framework"`
		ResolutionSource       string `json:"resolution_source"`
		PackageRecommendations []struct {
			HarnessID            string `json:"harness_id"`
			RecommendedFramework string `json:"recommended_framework"`
		} `json:"package_recommendations"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.RecommendedFramework)
	require.Empty(t, payload.ResolvedFramework)
	require.Equal(t, "unresolved_conflict", payload.ResolutionSource)
	require.Equal(t, []struct {
		HarnessID            string `json:"harness_id"`
		RecommendedFramework string `json:"recommended_framework"`
	}{
		{HarnessID: "docs_stack", RecommendedFramework: "claudecode"},
		{HarnessID: "ops_stack", RecommendedFramework: "codex"},
	}, payload.PackageRecommendations)
	require.Contains(t, payload.Warnings, `conflicting package framework recommendations detected: docs_stack=claudecode, ops_stack=codex`)
}

func TestHarnessFrameworkCommandsFailClosedOnPackageRecommendationConflict(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkConflictRepo(t)

	for _, command := range [][]string{
		{"framework", "plan", "--json"},
		{"framework", "apply", "--json"},
		{"framework", "check", "--json"},
		{"framework", "remove", "--json"},
	} {
		_, stderr, err := executeHarnessCLI(t, repo.Root, command...)
		require.Error(t, err)
		require.Empty(t, stderr)
		require.ErrorContains(t, err, "unresolved_conflict")
	}
}

func TestHarnessFrameworkInspectSummarizesRemoteSkillCapabilities(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRemoteSkillRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "inspect", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		CommandCount     int `json:"command_count"`
		SkillCount       int `json:"skill_count"`
		RemoteSkillCount int `json:"remote_skill_count"`
		RemoteSkills     []struct {
			OrbitID string `json:"orbit_id"`
			URI     string `json:"uri"`
		} `json:"remote_skills"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.CommandCount)
	require.Equal(t, 1, payload.SkillCount)
	require.Equal(t, 1, payload.RemoteSkillCount)
	require.Contains(t, payload.RemoteSkills, struct {
		OrbitID string `json:"orbit_id"`
		URI     string `json:"uri"`
	}{
		OrbitID: "docs",
		URI:     "https://example.com/skills/docs-style",
	})
}

func TestHarnessFrameworkPlanSeparatesDesiredProjectAndGlobalOutputs(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot      string `json:"harness_root"`
		HarnessID        string `json:"harness_id"`
		Framework        string `json:"framework"`
		ResolutionSource string `json:"resolution_source"`
		DesiredTruth     struct {
			RecommendedFramework string `json:"recommended_framework"`
			OrbitCount           int    `json:"orbit_count"`
			CommandCount         int    `json:"command_count"`
			SkillCount           int    `json:"skill_count"`
			HasAgentGuidance     bool   `json:"has_agent_guidance"`
			HasHumanGuidance     bool   `json:"has_human_guidance"`
		} `json:"desired_truth"`
		ProjectOutputs []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
			Kind   string `json:"kind"`
		} `json:"project_outputs"`
		GlobalOutputs []struct {
			Path   string `json:"path"`
			Action string `json:"action"`
			Kind   string `json:"kind"`
		} `json:"global_outputs"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "claudecode", payload.Framework)
	require.Equal(t, "recommended_default", payload.ResolutionSource)
	require.Equal(t, "claudecode", payload.DesiredTruth.RecommendedFramework)
	require.Equal(t, 1, payload.DesiredTruth.OrbitCount)
	require.Equal(t, 1, payload.DesiredTruth.CommandCount)
	require.Equal(t, 1, payload.DesiredTruth.SkillCount)
	require.True(t, payload.DesiredTruth.HasAgentGuidance)
	require.True(t, payload.DesiredTruth.HasHumanGuidance)
	require.Contains(t, payload.ProjectOutputs, struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Kind   string `json:"kind"`
	}{Path: "AGENTS.md", Action: "create", Kind: "guidance"})
	require.Contains(t, payload.ProjectOutputs, struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Kind   string `json:"kind"`
	}{Path: "HUMANS.md", Action: "create", Kind: "guidance"})
	require.Contains(t, payload.ProjectOutputs, struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Kind   string `json:"kind"`
	}{Path: "CLAUDE.md", Action: "symlink", Kind: "framework_alias"})
	require.Contains(t, payload.GlobalOutputs, struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Kind   string `json:"kind"`
	}{Path: "~/.claude/commands/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__review.md", Action: "symlink", Kind: "command"})
	require.Contains(t, payload.GlobalOutputs, struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Kind   string `json:"kind"`
	}{Path: "~/.claude/skills/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__docs-style", Action: "symlink", Kind: "skill"})
	require.Contains(t, payload.Warnings, "framework claudecode requires global command registration")
	require.Contains(t, payload.Warnings, "framework claudecode requires global skill registration")
}

func TestHarnessFrameworkPlanExposesAgentActivationRouteGroups(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RecommendedProjectOutputs []struct {
			Artifact       string   `json:"artifact"`
			ArtifactType   string   `json:"artifact_type"`
			Route          string   `json:"route"`
			Recommended    bool     `json:"recommended"`
			Source         string   `json:"source"`
			Path           string   `json:"path"`
			Action         string   `json:"action"`
			Kind           string   `json:"kind"`
			Mode           string   `json:"mode"`
			EffectiveScope string   `json:"effective_scope"`
			Invocation     []string `json:"invocation"`
		} `json:"recommended_project_outputs"`
		OptionalGlobalOutputs []struct {
			Artifact       string `json:"artifact"`
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			Recommended    bool   `json:"recommended"`
			Source         string `json:"source"`
			Path           string `json:"path"`
			Action         string `json:"action"`
			Kind           string `json:"kind"`
			Mode           string `json:"mode"`
			EffectiveScope string `json:"effective_scope"`
		} `json:"optional_global_outputs"`
		RecommendedHybridOutputs []struct {
			Artifact string `json:"artifact"`
		} `json:"recommended_hybrid_outputs"`
		CompatibilityOutputs []struct {
			Artifact string `json:"artifact"`
		} `json:"compatibility_outputs"`
		HookPreview []struct {
			Artifact string `json:"artifact"`
		} `json:"hook_preview"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	require.Contains(t, payload.RecommendedProjectOutputs, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Recommended    bool     `json:"recommended"`
		Source         string   `json:"source"`
		Path           string   `json:"path"`
		Action         string   `json:"action"`
		Kind           string   `json:"kind"`
		Mode           string   `json:"mode"`
		EffectiveScope string   `json:"effective_scope"`
		Invocation     []string `json:"invocation"`
	}{
		Artifact:       "review",
		ArtifactType:   "prompt-command",
		Route:          "project_skill",
		Recommended:    true,
		Source:         "orbit/commands/review.md",
		Path:           ".claude/skills/review",
		Action:         "symlink",
		Kind:           "command_as_skill",
		Mode:           "symlink",
		EffectiveScope: "project",
		Invocation:     []string{"/review"},
	})
	require.Contains(t, payload.RecommendedProjectOutputs, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Recommended    bool     `json:"recommended"`
		Source         string   `json:"source"`
		Path           string   `json:"path"`
		Action         string   `json:"action"`
		Kind           string   `json:"kind"`
		Mode           string   `json:"mode"`
		EffectiveScope string   `json:"effective_scope"`
		Invocation     []string `json:"invocation"`
	}{
		Artifact:       "docs-style",
		ArtifactType:   "local-skill",
		Route:          "project_skill",
		Recommended:    true,
		Source:         "orbit/skills/docs-style",
		Path:           ".claude/skills/docs-style",
		Action:         "symlink",
		Kind:           "skill",
		Mode:           "symlink",
		EffectiveScope: "project",
		Invocation:     []string{"/docs-style"},
	})
	require.Contains(t, payload.OptionalGlobalOutputs, struct {
		Artifact       string `json:"artifact"`
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Recommended    bool   `json:"recommended"`
		Source         string `json:"source"`
		Path           string `json:"path"`
		Action         string `json:"action"`
		Kind           string `json:"kind"`
		Mode           string `json:"mode"`
		EffectiveScope string `json:"effective_scope"`
	}{
		Artifact:       "review",
		ArtifactType:   "prompt-command",
		Route:          "global_registration",
		Recommended:    false,
		Source:         "orbit/commands/review.md",
		Path:           "~/.claude/skills/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__review",
		Action:         "symlink",
		Kind:           "command_as_skill",
		Mode:           "symlink",
		EffectiveScope: "global",
	})
	require.Empty(t, payload.RecommendedHybridOutputs)
	require.Empty(t, payload.CompatibilityOutputs)
	require.Empty(t, payload.HookPreview)
}

func TestHarnessFrameworkPlanAndApplyFailClosedOnCapabilityCollisions(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkCollisionRepo(t)

	_, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `resolved command name "review" is declared by multiple orbits`)

	_, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `resolved command name "review" is declared by multiple orbits`)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		FindingCount int `json:"finding_count"`
		Findings     []struct {
			Kind string `json:"kind"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.GreaterOrEqual(t, payload.FindingCount, 3)
	kinds := make([]string, 0, len(payload.Findings))
	for _, finding := range payload.Findings {
		kinds = append(kinds, finding.Kind)
	}
	require.Contains(t, kinds, "activation_missing")
	require.Contains(t, kinds, "command_name_collision")
	require.Contains(t, kinds, "skill_name_collision")
}

func TestHarnessFrameworkPlanAndApplyFailClosedOnUnsupportedRemoteSkills(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRequiredRemoteSkillRepo(t)

	_, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `framework "claudecode" does not support remote skill URI`)

	_, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `framework "claudecode" does not support remote skill URI`)
}

func TestHarnessFrameworkPlanAllowsRecommendedUnsupportedRemoteSkills(t *testing.T) {
	repo := seedHarnessFrameworkRemoteSkillRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "plan", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RemoteSkills []struct {
			OrbitID  string `json:"orbit_id"`
			URI      string `json:"uri"`
			Required bool   `json:"required"`
		} `json:"remote_skills"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.RemoteSkills, struct {
		OrbitID  string `json:"orbit_id"`
		URI      string `json:"uri"`
		Required bool   `json:"required"`
	}{
		OrbitID:  "docs",
		URI:      "https://example.com/skills/docs-style",
		Required: false,
	})

	_, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)
}

func TestHarnessGuidanceComposeUsesTargetContractInJSON(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "guidance", "compose", "--target", "all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot   string `json:"harness_root"`
		Target        string `json:"target"`
		MemberCount   int    `json:"member_count"`
		ArtifactCount int    `json:"artifact_count"`
		Artifacts     []struct {
			Target string `json:"target"`
			Path   string `json:"path"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "all", payload.Target)
	require.Equal(t, 1, payload.MemberCount)
	require.Equal(t, 3, payload.ArtifactCount)

	targets := make([]string, 0, len(payload.Artifacts))
	for _, artifact := range payload.Artifacts {
		targets = append(targets, artifact.Target)
	}
	require.ElementsMatch(t, []string{"agents", "humans", "bootstrap"}, targets)
}

func TestHarnessGuidanceComposeAcceptsEquivalentLegacyAudienceAlias(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "guidance", "compose", "--target", "agents", "--audience", "agent", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: --audience is deprecated; use --target")

	var payload struct {
		Target string `json:"target"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "agents", payload.Target)
}

func TestHarnessFrameworkApplyYesUsesProjectRoutesWithoutGlobalWrites(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot        string `json:"harness_root"`
		HarnessID          string `json:"harness_id"`
		Framework          string `json:"framework"`
		ResolutionSource   string `json:"resolution_source"`
		Status             string `json:"status"`
		ActivationPath     string `json:"activation_path"`
		ProjectOutputCount int    `json:"project_output_count"`
		GlobalOutputCount  int    `json:"global_output_count"`
		ArtifactResults    []struct {
			Artifact       string   `json:"artifact"`
			ArtifactType   string   `json:"artifact_type"`
			Route          string   `json:"route"`
			Mode           string   `json:"mode"`
			EffectiveScope string   `json:"effective_scope"`
			Path           string   `json:"path"`
			Status         string   `json:"status"`
			Invocation     []string `json:"invocation"`
		} `json:"artifact_results"`
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "claudecode", payload.Framework)
	require.Equal(t, "recommended_default", payload.ResolutionSource)
	require.Equal(t, "ok", payload.Status)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "activations", "claudecode.json"), payload.ActivationPath)
	require.Equal(t, 3, payload.ProjectOutputCount)
	require.Zero(t, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Mode           string   `json:"mode"`
		EffectiveScope string   `json:"effective_scope"`
		Path           string   `json:"path"`
		Status         string   `json:"status"`
		Invocation     []string `json:"invocation"`
	}{
		Artifact:       "review",
		ArtifactType:   "prompt-command",
		Route:          "project_skill",
		Mode:           "symlink",
		EffectiveScope: "project",
		Path:           ".claude/skills/review",
		Status:         "project_applied",
		Invocation:     []string{"/review"},
	})
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Mode           string   `json:"mode"`
		EffectiveScope string   `json:"effective_scope"`
		Path           string   `json:"path"`
		Status         string   `json:"status"`
		Invocation     []string `json:"invocation"`
	}{
		Artifact:       "docs-style",
		ArtifactType:   "local-skill",
		Route:          "project_skill",
		Mode:           "symlink",
		EffectiveScope: "project",
		Path:           ".claude/skills/docs-style",
		Status:         "project_applied",
		Invocation:     []string{"/docs-style"},
	})
	require.Empty(t, payload.Warnings)

	agentsBody, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsBody), "You are the docs orbit.")

	humansBody, err := os.ReadFile(filepath.Join(repo.Root, "HUMANS.md"))
	require.NoError(t, err)
	require.Contains(t, string(humansBody), "Run the docs workflow.")

	claudeAliasPath := filepath.Join(repo.Root, "CLAUDE.md")
	aliasTarget, err := os.Readlink(claudeAliasPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "AGENTS.md"), aliasTarget)

	projectCommandPath := filepath.Join(repo.Root, ".claude", "skills", "review")
	commandTarget, err := os.Readlink(projectCommandPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "claudecode", "commands", "docs", "review"), commandTarget)

	projectSkillPath := filepath.Join(repo.Root, ".claude", "skills", "docs-style")
	skillTarget, err := os.Readlink(projectSkillPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "orbit", "skills", "docs-style"), skillTarget)
	require.NoFileExists(t, filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review"))
	require.NoFileExists(t, filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__docs-style"))

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "claudecode")
	require.NoError(t, err)
	require.Equal(t, "claudecode", activation.Framework)
	require.Equal(t, repo.Root, activation.RepoRoot)
	require.Equal(t, "recommended_default", string(activation.ResolutionSource))
	require.Len(t, activation.ProjectOutputs, 3)
	require.Empty(t, activation.GlobalOutputs)
	require.Contains(t, activation.ProjectOutputs, harnesspkg.FrameworkActivationOutput{
		Path:           ".claude/skills/review",
		AbsolutePath:   filepath.Join(repo.Root, ".claude", "skills", "review"),
		Kind:           "command_as_skill",
		Action:         "symlink",
		Target:         filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "claudecode", "commands", "docs", "review"),
		OrbitID:        "docs",
		Artifact:       "review",
		ArtifactType:   "prompt-command",
		Source:         "orbit/commands/review.md",
		Route:          "project_skill",
		Mode:           "symlink",
		EffectiveScope: "project",
		Invocation:     []string{"/review"},
	})
	require.NotEmpty(t, activation.GuidanceHash)
	require.NotEmpty(t, activation.CapabilitiesHash)
	require.NotEmpty(t, activation.SelectionHash)
}

func TestHarnessFrameworkApplyCompilesCommandsIntoProjectSkills(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", strings.ReplaceAll(mustReadString(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")), "- orbit/commands/review.md", "- orbit/commands/*.md"))
	repo.WriteFile(t, "orbit/commands/review.md", ""+
		"---\n"+
		"name: review\n"+
		"description: Review current docs diff.\n"+
		"---\n"+
		"# Review\n\n"+
		"Review docs work.\n")
	repo.WriteFile(t, "orbit/commands/outline.md", ""+
		"Outline release notes and note risks.\n\n"+
		"Use concise bullets.\n")
	repo.AddAndCommit(t, "seed command renderer scenarios")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ArtifactResults []struct {
			Artifact string `json:"artifact"`
			Path     string `json:"path"`
			Target   string `json:"target"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Path     string `json:"path"`
		Target   string `json:"target"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Path:     ".claude/skills/review",
		Target:   filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "claudecode", "commands", "docs", "review"),
		Status:   "project_applied",
	})

	reviewTarget, err := os.Readlink(filepath.Join(repo.Root, ".claude", "skills", "review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "claudecode", "commands", "docs", "review"), reviewTarget)
	reviewSkill := mustReadString(t, filepath.Join(reviewTarget, "SKILL.md"))
	require.Contains(t, reviewSkill, "name: review\n")
	require.Contains(t, reviewSkill, "description: Review current docs diff.\n")
	require.Contains(t, reviewSkill, "# Review")
	require.Contains(t, reviewSkill, "Review docs work.")

	outlineTarget, err := os.Readlink(filepath.Join(repo.Root, ".claude", "skills", "outline"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "claudecode", "commands", "docs", "outline"), outlineTarget)
	outlineSkill := mustReadString(t, filepath.Join(outlineTarget, "SKILL.md"))
	require.Contains(t, outlineSkill, "name: outline\n")
	require.Contains(t, outlineSkill, "description: Outline release notes and note risks.\n")
	require.Contains(t, outlineSkill, "Use concise bullets.")
}

func TestHarnessFrameworkApplyCodexDefaultsToProjectSkillsAndGlobalFlagUsesPrompts(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var projectPayload struct {
		ArtifactResults []struct {
			Artifact string `json:"artifact"`
			Path     string `json:"path"`
			Target   string `json:"target"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &projectPayload))
	require.Contains(t, projectPayload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Path     string `json:"path"`
		Target   string `json:"target"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Path:     ".codex/skills/review",
		Target:   filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "codex", "commands", "docs", "review"),
		Status:   "project_applied",
	})
	codexProjectTarget, err := os.Readlink(filepath.Join(repo.Root, ".codex", "skills", "review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "codex", "commands", "docs", "review"), codexProjectTarget)
	require.NoFileExists(t, filepath.Join(homeDir, ".codex", "prompts", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review.md"))

	_, _, err = executeHarnessCLI(t, repo.Root, "framework", "remove", "--json")
	require.NoError(t, err)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "framework", "apply", "--global", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var globalPayload struct {
		ArtifactResults []struct {
			Artifact string `json:"artifact"`
			Path     string `json:"path"`
			Target   string `json:"target"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &globalPayload))
	require.Contains(t, globalPayload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Path     string `json:"path"`
		Target   string `json:"target"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Path:     "~/.codex/prompts/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__review.md",
		Target:   filepath.Join(repo.Root, "orbit", "commands", "review.md"),
		Status:   "global_applied",
	})
	codexPromptTarget, err := os.Readlink(filepath.Join(homeDir, ".codex", "prompts", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review.md"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "orbit", "commands", "review.md"), codexPromptTarget)
}

func TestHarnessFrameworkApplyOpenClawUsesWorkspaceProjectSkill(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: openclaw\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ArtifactResults []struct {
			Artifact       string `json:"artifact"`
			Path           string `json:"path"`
			Target         string `json:"target"`
			EffectiveScope string `json:"effective_scope"`
			Status         string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string `json:"artifact"`
		Path           string `json:"path"`
		Target         string `json:"target"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		Artifact:       "review",
		Path:           "skills/review",
		Target:         filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "openclaw", "commands", "docs", "review"),
		EffectiveScope: "project_workspace",
		Status:         "project_applied",
	})
	openClawTarget, err := os.Readlink(filepath.Join(repo.Root, "skills", "review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "openclaw", "commands", "docs", "review"), openClawTarget)
}

func TestHarnessFrameworkApplyAllowsGlobalFallbackForFailedProjectCommand(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".claude", "skills"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".claude", "skills", "review"), []byte("user owned\n"), 0o644))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--allow-global-fallback", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status             string `json:"status"`
		ProjectOutputCount int    `json:"project_output_count"`
		GlobalOutputCount  int    `json:"global_output_count"`
		ArtifactResults    []struct {
			Artifact string `json:"artifact"`
			Route    string `json:"route"`
			Path     string `json:"path"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Equal(t, 2, payload.ProjectOutputCount)
	require.Equal(t, 1, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Route:    "project_skill",
		Path:     ".claude/skills/review",
		Status:   "project_failed",
	})
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Route:    "global_registration",
		Path:     "~/.claude/skills/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__review",
		Status:   "global_applied",
	})
	require.Equal(t, "user owned\n", mustReadString(t, filepath.Join(repo.Root, ".claude", "skills", "review")))
	globalTarget, err := os.Readlink(filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "orbit", "commands", "review.md"), globalTarget)
}

func TestHarnessFrameworkApplyProjectOnlyReportsCompatibilityPendingForFailedCommand(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".claude", "skills"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".claude", "skills", "review"), []byte("user owned\n"), 0o644))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--project-only", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status             string `json:"status"`
		ProjectOutputCount int    `json:"project_output_count"`
		GlobalOutputCount  int    `json:"global_output_count"`
		ArtifactResults    []struct {
			Artifact string `json:"artifact"`
			Route    string `json:"route"`
			Path     string `json:"path"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "partial", payload.Status)
	require.Equal(t, 2, payload.ProjectOutputCount)
	require.Zero(t, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Route:    "project_skill",
		Path:     ".claude/skills/review",
		Status:   "project_failed",
	})
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "review",
		Route:    "project_compatibility",
		Path:     ".claude/skills/review",
		Status:   "compatibility_pending",
	})
	require.Equal(t, "user owned\n", mustReadString(t, filepath.Join(repo.Root, ".claude", "skills", "review")))
}

func TestHarnessFrameworkApplyCodexConfigUsesConfigYamlAndSidecar(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: gpt-5.4\n"+
		"  sandbox_mode: workspace-write\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", ""+
		"approval_policy = \"on-request\"\n"+
		"[features]\n"+
		"web_search = true\n")
	repo.AddAndCommit(t, "add codex agent config")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status          string `json:"status"`
		ArtifactResults []struct {
			Artifact       string   `json:"artifact"`
			ArtifactType   string   `json:"artifact_type"`
			Route          string   `json:"route"`
			Mode           string   `json:"mode"`
			SourceFiles    []string `json:"source_files"`
			Path           string   `json:"path"`
			EffectiveScope string   `json:"effective_scope"`
			Status         string   `json:"status"`
			GeneratedKeys  []string `json:"generated_keys"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Mode           string   `json:"mode"`
		SourceFiles    []string `json:"source_files"`
		Path           string   `json:"path"`
		EffectiveScope string   `json:"effective_scope"`
		Status         string   `json:"status"`
		GeneratedKeys  []string `json:"generated_keys"`
	}{
		Artifact:       "codex-config",
		ArtifactType:   "agent-config",
		Route:          "project_config",
		Mode:           "merge-config",
		SourceFiles:    []string{".harness/agents/config.yaml", ".harness/agents/codex.config.toml"},
		Path:           ".codex/config.toml",
		EffectiveScope: "project",
		Status:         "project_applied",
		GeneratedKeys:  []string{"approval_policy", "features.web_search", "model", "sandbox_mode"},
	})

	codexConfig := mustReadString(t, filepath.Join(repo.Root, ".codex", "config.toml"))
	require.Contains(t, codexConfig, "model = \"gpt-5.4\"\n")
	require.Contains(t, codexConfig, "sandbox_mode = \"workspace-write\"\n")
	require.Contains(t, codexConfig, "approval_policy = \"on-request\"\n")
	require.Contains(t, codexConfig, "[features]\n")
	require.Contains(t, codexConfig, "web_search = true\n")
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "config.toml", "SKILL.md"))

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.NoError(t, err)
	require.Contains(t, activation.ProjectOutputs, harnesspkg.FrameworkActivationOutput{
		Path:           ".codex/config.toml",
		AbsolutePath:   filepath.Join(repo.Root, ".codex", "config.toml"),
		Kind:           "config",
		Action:         "merge-config",
		Target:         filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "codex", "config", "project.toml"),
		Artifact:       "codex-config",
		ArtifactType:   "agent-config",
		Source:         ".harness/agents/config.yaml",
		SourceFiles:    []string{".harness/agents/config.yaml", ".harness/agents/codex.config.toml"},
		Sidecar:        ".harness/agents/codex.config.toml",
		Route:          "project_config",
		Mode:           "merge-config",
		EffectiveScope: "project",
		GeneratedKeys:  []string{"approval_policy", "features.web_search", "model", "sandbox_mode"},
		PatchOwnedKeys: []string{"approval_policy", "features.web_search", "model", "sandbox_mode"},
	})
}

func TestHarnessFrameworkApplyClaudeConfigMergesJsonSidecar(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  claudeCode:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: claude-sonnet-4-5\n")
	repo.WriteFile(t, ".harness/agents/claude-code.settings.json", "{\n  \"includeCoAuthoredBy\": false\n}\n")
	repo.AddAndCommit(t, "add claude agent config")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ArtifactResults []struct {
			Artifact string   `json:"artifact"`
			Path     string   `json:"path"`
			Status   string   `json:"status"`
			Keys     []string `json:"generated_keys"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string   `json:"artifact"`
		Path     string   `json:"path"`
		Status   string   `json:"status"`
		Keys     []string `json:"generated_keys"`
	}{
		Artifact: "claudecode-config",
		Path:     ".claude/settings.json",
		Status:   "project_applied",
		Keys:     []string{"includeCoAuthoredBy", "model"},
	})

	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadString(t, filepath.Join(repo.Root, ".claude", "settings.json"))), &settings))
	require.Equal(t, "claude-sonnet-4-5", settings["model"])
	require.Equal(t, false, settings["includeCoAuthoredBy"])
}

func TestHarnessAgentUseAndApplyMultipleAgentsAdditively(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"  claudecode:\n"+
		"    enabled: true\n"+
		"    scope: project\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "sandbox_mode = \"workspace-write\"\n")
	repo.WriteFile(t, ".harness/agents/claude-code.settings.json", "{\n  \"includeCoAuthoredBy\": false\n}\n")
	repo.AddAndCommit(t, "add multi-agent config")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "claude-code", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var usePayload struct {
		Frameworks []string `json:"frameworks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &usePayload))
	require.Equal(t, []string{"codex", "claudecode"}, usePayload.Frameworks)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "agent", "apply", "--yes", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var applyPayload struct {
		Frameworks []string `json:"frameworks"`
		Results    []struct {
			Framework string `json:"framework"`
			Status    string `json:"status"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &applyPayload))
	require.Equal(t, []string{"codex", "claudecode"}, applyPayload.Frameworks)
	require.Contains(t, applyPayload.Results, struct {
		Framework string `json:"framework"`
		Status    string `json:"status"`
	}{Framework: "codex", Status: "ok"})
	require.Contains(t, applyPayload.Results, struct {
		Framework string `json:"framework"`
		Status    string `json:"status"`
	}{Framework: "claudecode", Status: "ok"})

	require.Contains(t, mustReadString(t, filepath.Join(repo.Root, ".codex", "config.toml")), "sandbox_mode = \"workspace-write\"\n")
	var settings map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadString(t, filepath.Join(repo.Root, ".claude", "settings.json"))), &settings))
	require.Equal(t, false, settings["includeCoAuthoredBy"])
	_, err = harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "codex")
	require.NoError(t, err)
	_, err = harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "claudecode")
	require.NoError(t, err)
}

func TestHarnessAgentApplyMultipleAgentsPromptsWithFrameworkOverrides(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"  claudecode:\n"+
		"    enabled: true\n"+
		"    scope: project\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "sandbox_mode = \"workspace-write\"\n")
	repo.WriteFile(t, ".harness/agents/claude-code.settings.json", "{\n  \"includeCoAuthoredBy\": false\n}\n")
	repo.AddAndCommit(t, "add multi-agent config")

	_, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "claude-code", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err := executeHarnessCLIWithInput(t, repo.Root, "\n", "agent", "apply")
	require.NoError(t, err)
	require.Contains(t, stderr, "Apply command and skill artifacts as project skills? [Y/n] ")
	require.Contains(t, stdout, "framework: codex status=ok\n")
	require.Contains(t, stdout, "framework: claudecode status=ok\n")
}

func TestHarnessAgentApplyMultipleAgentsHooksPreviewUsesFrameworkOverrides(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"  claudecode:\n"+
		"    enabled: true\n"+
		"    scope: project\n")
	repo.AddAndCommit(t, "add multi-agent config")

	_, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "claude-code", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Hook activation may execute project scripts when the agent runs.")
	require.Contains(t, stdout, `"frameworks": [`)
	require.Contains(t, stdout, `"claudecode"`)
}

func TestHarnessAgentConfigClearTargetKeepsSidecarAndWarnsForGlobalTarget(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: global\n"+
		"  claudecode:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  approval_mode: ask\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "model = \"gpt-5.4\"\n")
	repo.WriteFile(t, ".harness/agents/claude-code.settings.json", "{\n  \"model\": \"claude-sonnet-4-5\"\n}\n")
	repo.AddAndCommit(t, "add agent config truth")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "config", "clear", "--target", "codex", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: clearing global agent config target codex")

	var payload struct {
		ConfigPath      string   `json:"config_path"`
		ClearedTargets  []string `json:"cleared_targets"`
		RemovedSidecars []string `json:"removed_sidecars"`
		Warnings        []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, filepath.Join(repo.Root, ".harness", "agents", "config.yaml"), payload.ConfigPath)
	require.Equal(t, []string{"codex"}, payload.ClearedTargets)
	require.Empty(t, payload.RemovedSidecars)
	require.Contains(t, payload.Warnings, "clearing global agent config target codex")

	config := mustReadString(t, filepath.Join(repo.Root, ".harness", "agents", "config.yaml"))
	require.NotContains(t, config, "  codex:")
	require.Contains(t, config, "  claudecode:")
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "agents", "claude-code.settings.json"))
}

func TestHarnessAgentConfigClearTargetRemovesSidecarWhenRequested(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"  claudecode:\n"+
		"    enabled: true\n"+
		"    scope: project\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "model = \"gpt-5.4\"\n")
	repo.WriteFile(t, ".harness/agents/claude-code.settings.json", "{\n  \"model\": \"claude-sonnet-4-5\"\n}\n")
	repo.AddAndCommit(t, "add agent sidecars")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "config", "clear", "--target", "codex", "--sidecars", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ClearedTargets  []string `json:"cleared_targets"`
		RemovedSidecars []string `json:"removed_sidecars"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{"codex"}, payload.ClearedTargets)
	require.Equal(t, []string{".harness/agents/codex.config.toml"}, payload.RemovedSidecars)
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "agents", "codex.config.toml"))
	require.FileExists(t, filepath.Join(repo.Root, ".harness", "agents", "claude-code.settings.json"))
}

func TestHarnessAgentApplyGlobalConfigWarns(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: global\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "sandbox_mode = \"workspace-write\"\n")
	repo.AddAndCommit(t, "add global codex config")
	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--global", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: applying global agent config for codex: ~/.codex/config.toml")

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Warnings, "applying global agent config for codex: ~/.codex/config.toml")
}

func TestHarnessAgentRemoveGlobalConfigWarns(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: global\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "sandbox_mode = \"workspace-write\"\n")
	repo.AddAndCommit(t, "add global codex config")
	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "use", "codex", "--json")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "agent", "apply", "--global", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "remove", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: removing global agent config for codex: ~/.codex/config.toml")

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Warnings, "removing global agent config for codex: ~/.codex/config.toml")
}

func TestHarnessFrameworkApplyConfigSidecarCannotOverrideUnifiedTruth(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"config:\n"+
		"  model: gpt-5.4\n")
	repo.WriteFile(t, ".harness/agents/codex.config.toml", "model = \"gpt-4.1\"\n")
	repo.AddAndCommit(t, "add conflicting codex sidecar")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--yes", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `sidecar ".harness/agents/codex.config.toml" cannot override unified config key "model"`)
	require.NoFileExists(t, filepath.Join(repo.Root, ".codex", "config.toml"))
}

func TestHarnessFrameworkApplyProjectOnlyReportsCompatibilityForUnmanagedConfigConflict(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"config:\n"+
		"  model: gpt-5.4\n")
	repo.WriteFile(t, ".codex/config.toml", "model = \"gpt-4.1\"\nunmanaged = true\n")
	repo.AddAndCommit(t, "add unmanaged codex config conflict")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--project-only", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status          string `json:"status"`
		ArtifactResults []struct {
			Artifact string `json:"artifact"`
			Route    string `json:"route"`
			Path     string `json:"path"`
			Status   string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "partial", payload.Status)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "codex-config",
		Route:    "project_config",
		Path:     ".codex/config.toml",
		Status:   "project_failed",
	})
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact string `json:"artifact"`
		Route    string `json:"route"`
		Path     string `json:"path"`
		Status   string `json:"status"`
	}{
		Artifact: "codex-config",
		Route:    "project_compatibility",
		Path:     ".codex/config.toml",
		Status:   "compatibility_pending",
	})
	require.Equal(t, "model = \"gpt-4.1\"\nunmanaged = true\n", mustReadString(t, filepath.Join(repo.Root, ".codex", "config.toml")))
}

func TestHarnessFrameworkApplyOpenClawGlobalConfigPatchesIsolatedHome(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: openclaw\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  openclaw:\n"+
		"    enabled: true\n"+
		"    scope: global\n"+
		"config:\n"+
		"  model: gpt-5.4\n")
	repo.WriteFile(t, ".harness/agents/openclaw.openclaw.json5", "{\n  // vendor extension\n  \"workspaceMode\": \"trusted\",\n}\n")
	repo.AddAndCommit(t, "add openclaw agent config")
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".openclaw"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".openclaw", "openclaw.json"), []byte("{\n  \"keep\": true\n}\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--global", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: applying global agent config for openclaw: ~/.openclaw/openclaw.json")

	var payload struct {
		Status            string `json:"status"`
		GlobalOutputCount int    `json:"global_output_count"`
		ArtifactResults   []struct {
			Artifact       string   `json:"artifact"`
			ArtifactType   string   `json:"artifact_type"`
			Route          string   `json:"route"`
			Mode           string   `json:"mode"`
			Path           string   `json:"path"`
			EffectiveScope string   `json:"effective_scope"`
			Status         string   `json:"status"`
			PatchOwnedKeys []string `json:"patch_owned_keys"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Equal(t, 3, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string   `json:"artifact"`
		ArtifactType   string   `json:"artifact_type"`
		Route          string   `json:"route"`
		Mode           string   `json:"mode"`
		Path           string   `json:"path"`
		EffectiveScope string   `json:"effective_scope"`
		Status         string   `json:"status"`
		PatchOwnedKeys []string `json:"patch_owned_keys"`
	}{
		Artifact:       "openclaw-config",
		ArtifactType:   "agent-config",
		Route:          "global_config",
		Mode:           "patch-global-config",
		Path:           "~/.openclaw/openclaw.json",
		EffectiveScope: "global",
		Status:         "global_applied",
		PatchOwnedKeys: []string{"model", "workspaceMode"},
	})

	var openclawConfig map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadString(t, filepath.Join(homeDir, ".openclaw", "openclaw.json"))), &openclawConfig))
	require.Equal(t, true, openclawConfig["keep"])
	require.Equal(t, "gpt-5.4", openclawConfig["model"])
	require.Equal(t, "trusted", openclawConfig["workspaceMode"])
}

func TestHarnessAgentApplyHooksCodexWritesPreviewedProjectHooks(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: codex\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  codex:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  unsupported_behavior: skip\n"+
		"  entries:\n"+
		"    - id: block-dangerous-shell\n"+
		"      description: Block dangerous shell commands.\n"+
		"      event:\n"+
		"        kind: tool.before\n"+
		"      match:\n"+
		"        tools: [shell]\n"+
		"        command_patterns: [\"rm -rf *\"]\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/block-dangerous-shell/run.sh\n"+
		"      targets:\n"+
		"        codex: true\n")
	repo.WriteFile(t, "hooks/block-dangerous-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-dangerous-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "add codex hooks truth")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Hook activation may execute project scripts when the agent runs.")
	require.Contains(t, stderr, "Apply hook activation? accepted by --yes")

	var payload struct {
		Status          string `json:"status"`
		ArtifactResults []struct {
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			Mode           string `json:"mode"`
			Path           string `json:"path"`
			EffectiveScope string `json:"effective_scope"`
			Status         string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Contains(t, payload.ArtifactResults, struct {
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Mode           string `json:"mode"`
		Path           string `json:"path"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		ArtifactType:   "hook-config",
		Route:          "project_hooks",
		Mode:           "merge-config",
		Path:           ".codex/config.toml",
		EffectiveScope: "project",
		Status:         "project_applied",
	})
	require.Contains(t, payload.ArtifactResults, struct {
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Mode           string `json:"mode"`
		Path           string `json:"path"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		ArtifactType:   "hook-config",
		Route:          "project_hooks",
		Mode:           "generate",
		Path:           ".codex/hooks.json",
		EffectiveScope: "project",
		Status:         "project_applied",
	})
	require.Contains(t, mustReadString(t, filepath.Join(repo.Root, ".codex", "config.toml")), "codex_hooks = true\n")
	codexHooks := mustReadString(t, filepath.Join(repo.Root, ".codex", "hooks.json"))
	require.Contains(t, codexHooks, `"PreToolUse"`)
	require.Contains(t, codexHooks, "hyard hooks run")
	require.Contains(t, codexHooks, "--target codex")
	require.Contains(t, codexHooks, "--hook block-dangerous-shell")
}

func TestHarnessAgentApplyHooksClaudeMergesCommandHooks(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	t.Setenv("HOME", t.TempDir())
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  claudeCode:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  entries:\n"+
		"    - id: capture-prompt\n"+
		"      event:\n"+
		"        kind: prompt.before_submit\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/capture-prompt/run.sh\n"+
		"      targets:\n"+
		"        claudeCode: true\n")
	repo.WriteFile(t, "hooks/capture-prompt/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "capture-prompt", "run.sh"), 0o755))
	repo.AddAndCommit(t, "add claude hooks truth")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "Will write:")

	var payload struct {
		Status          string `json:"status"`
		ArtifactResults []struct {
			ArtifactType string `json:"artifact_type"`
			Route        string `json:"route"`
			Mode         string `json:"mode"`
			Path         string `json:"path"`
			Status       string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Contains(t, payload.ArtifactResults, struct {
		ArtifactType string `json:"artifact_type"`
		Route        string `json:"route"`
		Mode         string `json:"mode"`
		Path         string `json:"path"`
		Status       string `json:"status"`
	}{
		ArtifactType: "hook-config",
		Route:        "project_hooks",
		Mode:         "merge-config",
		Path:         ".claude/settings.json",
		Status:       "project_applied",
	})

	settings := mustReadString(t, filepath.Join(repo.Root, ".claude", "settings.json"))
	require.Contains(t, settings, `"hooks"`)
	require.Contains(t, settings, `"UserPromptSubmit"`)
	require.Contains(t, settings, `"type": "command"`)
	require.Contains(t, settings, "hyard hooks run")
	require.Contains(t, settings, "--target claude")
	require.Contains(t, settings, "--hook capture-prompt")
}

func TestHarnessAgentApplyHooksOpenClawUsesHybridAndSkipsUnsupportedToolEvent(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: openclaw\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  openclaw:\n"+
		"    enabled: true\n"+
		"    scope: hybrid\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  unsupported_behavior: skip\n"+
		"  entries:\n"+
		"    - id: compact-policy\n"+
		"      description: Compact transcript policy.\n"+
		"      event:\n"+
		"        kind: compact.before\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/compact-policy/run.sh\n"+
		"      targets:\n"+
		"        openclaw: true\n"+
		"    - id: block-shell\n"+
		"      event:\n"+
		"        kind: tool.before\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/block-shell/run.sh\n"+
		"      targets:\n"+
		"        openclaw: true\n")
	repo.WriteFile(t, "hooks/compact-policy/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.WriteFile(t, "hooks/block-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "compact-policy", "run.sh"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "add openclaw hooks truth")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "OpenClaw hook activation requires:")
	require.Contains(t, stderr, "~/.openclaw/openclaw.json")

	var payload struct {
		Status            string `json:"status"`
		GlobalOutputCount int    `json:"global_output_count"`
		ArtifactResults   []struct {
			Artifact       string `json:"artifact"`
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			Mode           string `json:"mode"`
			Path           string `json:"path"`
			EffectiveScope string `json:"effective_scope"`
			Status         string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "partial", payload.Status)
	require.Equal(t, 1, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string `json:"artifact"`
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Mode           string `json:"mode"`
		Path           string `json:"path"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		Artifact:       "openclaw-hooks",
		ArtifactType:   "hook-config",
		Route:          "hybrid_hook_activation",
		Mode:           "patch-global-config",
		Path:           "~/.openclaw/openclaw.json",
		EffectiveScope: "hybrid",
		Status:         "global_applied",
	})
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string `json:"artifact"`
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		Mode           string `json:"mode"`
		Path           string `json:"path"`
		EffectiveScope string `json:"effective_scope"`
		Status         string `json:"status"`
	}{
		Artifact:       "block-shell",
		ArtifactType:   "hook-config",
		Route:          "unsupported_event",
		Mode:           "skip",
		Path:           "",
		EffectiveScope: "hybrid",
		Status:         "unsupported_skipped",
	})
	require.FileExists(t, filepath.Join(repo.Root, "hooks", "compact-policy", "HOOK.md"))
	handler := mustReadString(t, filepath.Join(repo.Root, "hooks", "compact-policy", "handler.ts"))
	require.Contains(t, handler, "hyard hooks run")
	require.Contains(t, handler, "--target openclaw")
	require.Contains(t, handler, "--hook compact-policy")
	require.NoFileExists(t, filepath.Join(repo.Root, "hooks", "block-shell", "HOOK.md"))

	openClawConfig := mustReadString(t, filepath.Join(homeDir, ".openclaw", "openclaw.json"))
	require.Contains(t, openClawConfig, `"enabled": true`)
	require.Contains(t, openClawConfig, `"session:compact:before"`)
	require.Contains(t, openClawConfig, `"hooks/compact-policy/handler.ts"`)
}

func TestHarnessFrameworkApplyGlobalFlagUsesGlobalRoutes(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--global", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status             string `json:"status"`
		ProjectOutputCount int    `json:"project_output_count"`
		GlobalOutputCount  int    `json:"global_output_count"`
		ArtifactResults    []struct {
			Artifact       string `json:"artifact"`
			ArtifactType   string `json:"artifact_type"`
			Route          string `json:"route"`
			EffectiveScope string `json:"effective_scope"`
			Path           string `json:"path"`
			Status         string `json:"status"`
		} `json:"artifact_results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "ok", payload.Status)
	require.Equal(t, 1, payload.ProjectOutputCount)
	require.Equal(t, 2, payload.GlobalOutputCount)
	require.Contains(t, payload.ArtifactResults, struct {
		Artifact       string `json:"artifact"`
		ArtifactType   string `json:"artifact_type"`
		Route          string `json:"route"`
		EffectiveScope string `json:"effective_scope"`
		Path           string `json:"path"`
		Status         string `json:"status"`
	}{
		Artifact:       "review",
		ArtifactType:   "prompt-command",
		Route:          "global_registration",
		EffectiveScope: "global",
		Path:           "~/.claude/skills/" + harnesspkg.DefaultHarnessIDForPath(repo.Root) + "__docs__review",
		Status:         "global_applied",
	})

	globalCommandPath := filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review")
	commandTarget, err := os.Readlink(globalCommandPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "orbit", "commands", "review.md"), commandTarget)

	globalSkillPath := filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__docs-style")
	skillTarget, err := os.Readlink(globalSkillPath)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repo.Root, "orbit", "skills", "docs-style"), skillTarget)
	require.NoFileExists(t, filepath.Join(repo.Root, ".claude", "skills", "review"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".claude", "skills", "docs-style"))
}

func TestHarnessFrameworkCheckReportsHealthyActivationAndExecutablePresence(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("HOME", homeDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot   string   `json:"harness_root"`
		HarnessID     string   `json:"harness_id"`
		Framework     string   `json:"framework"`
		Configured    bool     `json:"configured"`
		Stale         bool     `json:"stale"`
		OK            bool     `json:"ok"`
		FindingCount  int      `json:"finding_count"`
		ActivationIDs []string `json:"activation_ids"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "claudecode", payload.Framework)
	require.True(t, payload.Configured)
	require.False(t, payload.Stale)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
	require.Equal(t, []string{"claudecode"}, payload.ActivationIDs)
}

func TestHarnessFrameworkCheckDetectsStaleActivationWhenRuntimeAgentTruthChanges(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("HOME", homeDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)

	activation, err := harnesspkg.LoadFrameworkActivation(filepath.Join(repo.Root, ".git"), "claudecode")
	require.NoError(t, err)
	require.NotEmpty(t, activation.RuntimeAgentTruthHash)

	_, err = harnesspkg.WriteAgentConfigFile(repo.Root, harnesspkg.AgentConfigFile{
		SchemaVersion: 1,
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Framework    string `json:"framework"`
		Configured   bool   `json:"configured"`
		Stale        bool   `json:"stale"`
		OK           bool   `json:"ok"`
		FindingCount int    `json:"finding_count"`
		Findings     []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "claudecode", payload.Framework)
	require.True(t, payload.Configured)
	require.True(t, payload.Stale)
	require.False(t, payload.OK)
	require.GreaterOrEqual(t, payload.FindingCount, 1)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{Kind: "activation_stale", Path: "", Message: "framework activation is stale relative to current runtime truth"})
}

func TestHarnessAgentCheckReportsCompiledSkillAndConfigDrift(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  claudeCode:\n"+
		"    enabled: true\n"+
		"    scope: project\n"+
		"config:\n"+
		"  model: claude-sonnet-4-5\n")
	repo.AddAndCommit(t, "add claude config truth")

	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--yes", "--json")
	require.NoError(t, err)

	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work with a new checklist.\n")
	repo.WriteFile(t, ".claude/settings.json", "{\n  \"model\": \"manual-model\"\n}\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Framework    string `json:"framework"`
		Configured   bool   `json:"configured"`
		OK           bool   `json:"ok"`
		FindingCount int    `json:"finding_count"`
		Findings     []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "claudecode", payload.Framework)
	require.True(t, payload.Configured)
	require.False(t, payload.OK)
	require.GreaterOrEqual(t, payload.FindingCount, 2)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "compiled_skill_stale",
		Path:    ".claude/skills/review",
		Message: "compiled command-as-skill cache is stale relative to source",
	})
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "config_output_stale",
		Path:    ".claude/settings.json",
		Message: "framework-managed native config keys differ from the compiled activation output",
	})
}

func TestHarnessAgentCheckReportsHookHandlerAndUnsupportedEventFindings(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: openclaw\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  openclaw:\n"+
		"    enabled: true\n"+
		"    scope: hybrid\n"+
		"hooks:\n"+
		"  enabled: true\n"+
		"  unsupported_behavior: skip\n"+
		"  entries:\n"+
		"    - id: compact-policy\n"+
		"      event:\n"+
		"        kind: compact.before\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/compact-policy/run.sh\n"+
		"      targets:\n"+
		"        openclaw: true\n"+
		"    - id: block-shell\n"+
		"      event:\n"+
		"        kind: tool.before\n"+
		"      handler:\n"+
		"        type: command\n"+
		"        path: hooks/block-shell/run.sh\n"+
		"      targets:\n"+
		"        openclaw: true\n")
	repo.WriteFile(t, "hooks/compact-policy/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	repo.WriteFile(t, "hooks/block-shell/run.sh", "#!/bin/sh\nprintf '{\"decision\":\"allow\"}\\n'\n")
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "compact-policy", "run.sh"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "block-shell", "run.sh"), 0o755))
	repo.AddAndCommit(t, "add openclaw hook truth")

	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--hooks", "--yes", "--json")
	require.NoError(t, err)
	require.NoError(t, os.Chmod(filepath.Join(repo.Root, "hooks", "compact-policy", "run.sh"), 0o644))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Configured   bool `json:"configured"`
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
		Findings     []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Configured)
	require.False(t, payload.OK)
	require.GreaterOrEqual(t, payload.FindingCount, 2)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "hook_handler_not_executable",
		Path:    "hooks/compact-policy/run.sh",
		Message: "hook handler is not executable",
	})
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "hook_event_unsupported",
		Path:    "block-shell",
		Message: "hook event is not supported by the resolved framework",
	})
}

func TestHarnessFrameworkCheckDetectsStaleActivationAndMissingExecutable(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	_, _, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), []byte(""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the docs orbit.\n"+
		"  humans_template: |\n"+
		"    Run the docs workflow.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/*.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/docs-style\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"), 0o644))
	repo.WriteFile(t, "orbit/commands/outline.md", "Outline docs work.\n")
	repo.AddAndCommit(t, "expand docs framework capabilities")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	binDir := filepath.Join(t.TempDir(), "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.Symlink(realGit, filepath.Join(binDir, "git")))
	t.Setenv("PATH", binDir)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Framework    string `json:"framework"`
		Configured   bool   `json:"configured"`
		Stale        bool   `json:"stale"`
		OK           bool   `json:"ok"`
		FindingCount int    `json:"finding_count"`
		Findings     []struct {
			Kind    string `json:"kind"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "claudecode", payload.Framework)
	require.True(t, payload.Configured)
	require.True(t, payload.Stale)
	require.False(t, payload.OK)
	require.GreaterOrEqual(t, payload.FindingCount, 2)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{Kind: "activation_stale", Path: "", Message: "framework activation is stale relative to current runtime truth"})
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{Kind: "executable_missing", Path: "claude", Message: "required framework executable is not available on PATH"})
}

func TestHarnessFrameworkCheckReportsCapabilityFindingsBeforeActivation(t *testing.T) {
	t.Parallel()

	repo := seedHarnessFrameworkInvalidSkillRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Configured   bool `json:"configured"`
		Stale        bool `json:"stale"`
		OK           bool `json:"ok"`
		FindingCount int  `json:"finding_count"`
		Findings     []struct {
			Kind    string `json:"kind"`
			OrbitID string `json:"orbit_id"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.Configured)
	require.False(t, payload.Stale)
	require.False(t, payload.OK)
	require.GreaterOrEqual(t, payload.FindingCount, 3)
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "activation_missing",
		OrbitID: "",
		Path:    "",
		Message: "framework activation ledger is missing for the resolved framework",
	})
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "skill_missing_description",
		OrbitID: "docs",
		Path:    "orbit/skills/docs-style/SKILL.md",
		Message: "local skill root \"orbit/skills/docs-style\" must define a non-empty description in SKILL.md frontmatter",
	})
	require.Contains(t, payload.Findings, struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{
		Kind:    "skill_remote_uri_unsupported",
		OrbitID: "docs",
		Path:    "https://example.com/skills/docs-style",
		Message: "framework \"claudecode\" does not support remote skill URI \"https://example.com/skills/docs-style\"",
	})
}

func TestHarnessFrameworkRemovePreservesUserOwnedAliasAndRemovesOwnedOutputs(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	_, _, err := executeHarnessCLI(t, repo.Root, "framework", "apply", "--json")
	require.NoError(t, err)

	claudeAliasPath := filepath.Join(repo.Root, "CLAUDE.md")
	require.NoError(t, os.Remove(claudeAliasPath))
	require.NoError(t, os.WriteFile(claudeAliasPath, []byte("manual claude entry\n"), 0o644))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "framework", "remove", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot            string   `json:"harness_root"`
		HarnessID              string   `json:"harness_id"`
		RemovedActivationCount int      `json:"removed_activation_count"`
		RemovedOutputCount     int      `json:"removed_output_count"`
		SkippedOutputCount     int      `json:"skipped_output_count"`
		Warnings               []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, 1, payload.RemovedActivationCount)
	require.Equal(t, 2, payload.RemovedOutputCount)
	require.Equal(t, 1, payload.SkippedOutputCount)
	require.Contains(t, payload.Warnings, "skip removing project output CLAUDE.md because it is no longer owned by this runtime activation")

	aliasBody, err := os.ReadFile(claudeAliasPath)
	require.NoError(t, err)
	require.Equal(t, "manual claude entry\n", string(aliasBody))

	_, err = os.Lstat(filepath.Join(homeDir, ".claude", "commands", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__review.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(homeDir, ".claude", "skills", harnesspkg.DefaultHarnessIDForPath(repo.Root)+"__docs__docs-style"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".git", "orbit", "state", "frameworks", "activations", "claude.json"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessAgentRemovePrunesGlobalConfigPatchKeysAndPreservesUserSettings(t *testing.T) {
	repo := seedHarnessFrameworkRepo(t)
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: openclaw\n")
	repo.WriteFile(t, ".harness/agents/config.yaml", ""+
		"version: 1\n"+
		"targets:\n"+
		"  openclaw:\n"+
		"    enabled: true\n"+
		"    scope: global\n"+
		"config:\n"+
		"  nested:\n"+
		"    enabled: true\n")
	repo.AddAndCommit(t, "add nested openclaw config truth")
	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".openclaw"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".openclaw", "openclaw.json"), []byte("{\n  \"keep\": true\n}\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "agent", "apply", "--global", "--json")
	require.NoError(t, err)

	var applied map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadString(t, filepath.Join(homeDir, ".openclaw", "openclaw.json"))), &applied))
	require.Equal(t, true, applied["keep"])
	require.Equal(t, map[string]any{"enabled": true}, applied["nested"])

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agent", "remove", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "warning: removing global agent config for openclaw: ~/.openclaw/openclaw.json")
	require.Contains(t, stdout, `"removed_activation_count": 1`)

	var remaining map[string]any
	require.NoError(t, json.Unmarshal([]byte(mustReadString(t, filepath.Join(homeDir, ".openclaw", "openclaw.json"))), &remaining))
	require.Equal(t, map[string]any{"keep": true}, remaining)
	require.NoDirExists(t, filepath.Join(repo.Root, ".git", "orbit", "state", "agents", "compiled", "openclaw"))
}

func TestHarnessCheckSucceedsForZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string `json:"harness_root"`
		HarnessID    string `json:"harness_id"`
		OK           bool   `json:"ok"`
		FindingCount int    `json:"finding_count"`
		Readiness    struct {
			Status  string `json:"status"`
			Summary struct {
				OrbitCount int `json:"orbit_count"`
			} `json:"summary"`
		} `json:"readiness"`
		Findings []struct {
			Kind    string `json:"kind"`
			OrbitID string `json:"orbit_id"`
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
	require.Empty(t, payload.Findings)
	require.Equal(t, "ready", payload.Readiness.Status)
	require.Equal(t, 0, payload.Readiness.Summary.OrbitCount)
}

func TestHarnessCheckTextOutputForZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"harness_root: "+repo.Root+"\n"+
		"harness_id: "+harnesspkg.DefaultHarnessIDForPath(repo.Root)+"\n"+
		"readiness_status: ready\n"+
		"readiness_orbit_count: 0\n"+
		"readiness_blocking_reason_count: 0\n"+
		"readiness_advisory_reason_count: 0\n"+
		"ok: true\n"+
		"finding_count: 0\n"+
		"findings: none\n", stdout)
}

func TestHarnessReadyReportsReadyForZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "ready", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot string `json:"harness_root"`
		HarnessID   string `json:"harness_id"`
		Status      string `json:"status"`
		Runtime     struct {
			Status string `json:"status"`
		} `json:"runtime"`
		Agent struct {
			Status           string `json:"status"`
			Required         bool   `json:"required"`
			ActivationStatus string `json:"activation_status"`
		} `json:"agent"`
		Summary struct {
			OrbitCount int `json:"orbit_count"`
		} `json:"summary"`
		RuntimeReasons []struct {
			Code string `json:"code"`
		} `json:"runtime_reasons"`
		OrbitReports []struct {
			OrbitID string `json:"orbit_id"`
		} `json:"orbit_reports"`
		NextSteps []struct {
			Command string `json:"command"`
		} `json:"next_steps"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "ready", payload.Status)
	require.Equal(t, "ready", payload.Runtime.Status)
	require.Equal(t, "ready", payload.Agent.Status)
	require.False(t, payload.Agent.Required)
	require.Equal(t, "not_required", payload.Agent.ActivationStatus)
	require.Equal(t, 0, payload.Summary.OrbitCount)
	require.Empty(t, payload.RuntimeReasons)
	require.Empty(t, payload.OrbitReports)
	require.Empty(t, payload.NextSteps)
}

func TestHarnessReadyTextOutputForZeroMemberRuntime(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "ready")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"harness_root: "+repo.Root+"\n"+
		"harness_id: "+harnesspkg.DefaultHarnessIDForPath(repo.Root)+"\n"+
		"status: ready\n"+
		"runtime_status: ready\n"+
		"agent_status: ready\n"+
		"agent_activation: not_required\n"+
		"orbit_count: 0\n"+
		"ready_orbit_count: 0\n"+
		"usable_orbit_count: 0\n"+
		"broken_orbit_count: 0\n", stdout)
	require.NotContains(t, stdout, "runtime_reasons:")
	require.NotContains(t, stdout, "next_steps:")
}

func TestHarnessReadyTextOutputForManualOrbitWithoutAgentsTemplate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "ready")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"harness_root: "+repo.Root+"\n"+
		"harness_id: "+harnesspkg.DefaultHarnessIDForPath(repo.Root)+"\n"+
		"status: usable\n"+
		"runtime_status: usable\n"+
		"agent_status: ready\n"+
		"agent_activation: not_required\n"+
		"orbit_count: 1\n"+
		"ready_orbit_count: 0\n"+
		"usable_orbit_count: 1\n"+
		"broken_orbit_count: 0\n"+
		"warning: root AGENTS.md has not been composed for this orbit (orbit_ids=docs, code=agents_not_composed)\n"+
		"orbit: docs source=manual status=usable\n"+
		"orbit_warning: docs root AGENTS.md has not been composed for this orbit (code=agents_not_composed)\n"+
		"suggested_command: hyard guide sync --target agents (refresh root AGENTS orchestration)\n", stdout)
	require.NotContains(t, stdout, "runtime_reason:")
	require.NotContains(t, stdout, "reasons=")
	require.NotContains(t, stdout, "next_step:")
	require.NotContains(t, stdout, "next_steps:")
}

func TestHarnessCheckIgnoresLegacyRuntimeFileWhenManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/runtime.yaml", ""+
		"schema_version: nope\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
	require.Empty(t, payload.Findings)
}

func TestHarnessCheckReportsManifestSchemaInvalidForDuplicateMembers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: runtime\n"+
		"runtime:\n"+
		"  id: workspace\n"+
		"  created_at: 2026-03-25T10:00:00Z\n"+
		"  updated_at: 2026-03-25T10:00:00Z\n"+
		"members:\n"+
		"  - orbit_id: docs\n"+
		"    source: manual\n"+
		"    added_at: 2026-03-25T10:00:00Z\n"+
		"  - orbit_id: docs\n"+
		"    source: manual\n"+
		"    added_at: 2026-03-25T10:05:00Z\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	require.Len(t, payload.Findings, 1)
	require.Equal(t, "manifest_schema_invalid", payload.Findings[0].Kind)
	require.Equal(t, ".harness/manifest.yaml", payload.Findings[0].Path)
	require.Contains(t, payload.Findings[0].Message, "packages[1].package.name must be unique")
}

func TestHarnessCheckReportsMissingDefinitionForRuntimeMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID: "docs",
		Source:  harnesspkg.MemberSourceManual,
		AddedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
	})
	runtimeFile.Harness.UpdatedAt = time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "missing_definition")
	require.Equal(t, "docs", finding.OrbitID)
	require.Equal(t, ".harness/orbits/docs.yaml", finding.Path)
	require.Contains(t, finding.Message, "definition")
}

func TestHarnessCheckReportsMissingBundleRecordForBundleBackedMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "bundle_member_mismatch")
	require.Equal(t, "docs", finding.OrbitID)
	require.Contains(t, finding.Message, "bundle-backed member")
}

func TestHarnessCheckSucceedsForBundleBackedMemberWithMatchingBundleRecord(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:      1,
		HarnessID:          "workspace",
		Template:           orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/workspace", TemplateCommit: "abc123"},
		MemberIDs:          []string{"docs"},
		AppliedAt:          time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
}

func TestHarnessCheckReportsBundleOwnerMismatchForBundleBackedMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:      1,
		HarnessID:          "other_bundle",
		Template:           orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/other-bundle", TemplateCommit: "abc123"},
		MemberIDs:          []string{"docs"},
		AppliedAt:          time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "bundle_member_mismatch")
	require.Equal(t, "docs", finding.OrbitID)
	require.Contains(t, finding.Message, `owner_harness_id "workspace"`)
	require.Contains(t, finding.Message, "matching bundle record")
}

func TestHarnessCheckReportsOrphanedBundleRecordWhenRuntimeOwnerDoesNotMatch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:      1,
		HarnessID:          "other_bundle",
		Template:           orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/other-bundle", TemplateCommit: "abc123"},
		MemberIDs:          []string{"docs"},
		AppliedAt:          time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"docs/guide.md"},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFindingForOrbit(t, payload, "bundle_member_mismatch", "other_bundle")
	require.Contains(t, finding.Message, "no matching bundle-backed members")
}

func TestHarnessCheckReportsInstallMemberMismatchWithoutInstallRecord(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID: "docs",
		Source:  harnesspkg.MemberSourceInstallOrbit,
		AddedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
	})
	runtimeFile.Harness.UpdatedAt = time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "install_member_mismatch")
	require.Equal(t, "docs", finding.OrbitID)
	require.Equal(t, ".harness/installs/docs.yaml", finding.Path)
	require.Contains(t, finding.Message, "missing install record")
}

func TestHarnessCheckIgnoresDetachedInstallRecordWithoutMatchingMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Status:        orbittemplate.InstallRecordStatusDetached,
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "deadbeef",
		},
		AppliedAt: time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
}

func TestHarnessCheckReportsInstallPathMismatch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/installs/wrong.yaml", ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: deadbeef\n"+
		"applied_at: 2026-03-21T12:00:00Z\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "install_path_mismatch")
	require.Equal(t, "docs", finding.OrbitID)
	require.Equal(t, ".harness/installs/wrong.yaml", finding.Path)
	require.Contains(t, finding.Message, "path")
}

func TestHarnessCheckReportsInstallRecordInvalidSeparatelyFromPathMismatch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/installs/broken.yaml", ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_ref: [broken\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "install_record_invalid")
	require.Empty(t, finding.OrbitID)
	require.Equal(t, ".harness/installs/broken.yaml", finding.Path)
	require.Contains(t, finding.Message, "invalid")

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "finding: install_record_invalid orbit_id=- path=.harness/installs/broken.yaml")
}

func TestHarnessCheckReportsDefinitionAndRuntimeFileDrift(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Drifted docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Locally drifted guide\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	definitionFinding := requireHarnessCheckFinding(t, payload, "definition_drift")
	require.Equal(t, "docs", definitionFinding.OrbitID)
	require.Equal(t, ".harness/orbits/docs.yaml", definitionFinding.Path)
	runtimeFinding := requireHarnessCheckFinding(t, payload, "runtime_file_drift")
	require.Equal(t, "docs", runtimeFinding.OrbitID)
	require.Equal(t, "docs/guide.md", runtimeFinding.Path)
}

func TestHarnessCheckReportsProvenanceUnresolvable(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/installs/docs.yaml", ""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"+
		"applied_at: 2026-03-21T12:40:00Z\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.False(t, payload.OK)
	finding := requireHarnessCheckFinding(t, payload, "provenance_unresolvable")
	require.Equal(t, "docs", finding.OrbitID)
	require.Equal(t, ".harness/installs/docs.yaml", finding.Path)
}

func TestHarnessCheckTreatsUnresolvedBindingsAsWarningOnly(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	payload := decodeHarnessCheckPayload(t, stdout)
	require.True(t, payload.OK)
	require.Zero(t, payload.FindingCount)
	require.Empty(t, payload.Findings)
	require.NotNil(t, payload.BindingsSummary)
	require.Equal(t, 1, payload.BindingsSummary.UnresolvedInstallCount)
	require.Equal(t, 1, payload.BindingsSummary.UnresolvedVariableCount)
	require.Equal(t, []string{"docs"}, payload.BindingsSummary.OrbitIDs)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.Contains(t, readinessCodesFromPayload(payload.Readiness.RuntimeReasons), "unresolved_required_bindings")
	for _, existing := range payload.Findings {
		require.NotEqual(t, "provenance_unresolvable", existing.Kind)
	}
}

func TestHarnessCheckTextOutputAggregatesUnresolvedBindings(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "check")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "ok: true\n")
	require.Contains(t, stdout, "finding_count: 0\n")
	require.Contains(t, stdout, "unresolved_bindings: installs=1 variables=1 orbit_ids=docs\n")
	require.Contains(t, stdout, "findings: none\n")
}

func TestHarnessBindingsMissingReportsCurrentVarsGapsAcrossInstallBackedOrbits(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "You are the $project_name docs orbit.\n",
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
			AgentsTemplate: "Use $binary_name for $project_name releases.\n",
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--allow-unresolved-bindings",
		"--json",
	)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Filled Orbit\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "missing", "--all", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot     string   `json:"harness_root"`
		OrbitCount      int      `json:"orbit_count"`
		OrbitIDs        []string `json:"orbit_ids"`
		VariableCount   int      `json:"variable_count"`
		MissingCount    int      `json:"missing_count"`
		ReadinessReason string   `json:"readiness_reason"`
		Orbits          []struct {
			OrbitID       string `json:"orbit_id"`
			DeclaredCount int    `json:"declared_count"`
			MissingCount  int    `json:"missing_count"`
			Variables     []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Required    bool   `json:"required"`
				HasValue    bool   `json:"has_value"`
				Missing     bool   `json:"missing"`
			} `json:"variables"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, 2, payload.OrbitCount)
	require.ElementsMatch(t, []string{"docs", "cmd"}, payload.OrbitIDs)
	require.Equal(t, 3, payload.VariableCount)
	require.Equal(t, 1, payload.MissingCount)
	require.Equal(t, "unresolved_required_bindings", payload.ReadinessReason)
	require.Len(t, payload.Orbits, 2)

	orbitByID := make(map[string]struct {
		OrbitID       string `json:"orbit_id"`
		DeclaredCount int    `json:"declared_count"`
		MissingCount  int    `json:"missing_count"`
		Variables     []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Required    bool   `json:"required"`
			HasValue    bool   `json:"has_value"`
			Missing     bool   `json:"missing"`
		} `json:"variables"`
	}, len(payload.Orbits))
	for _, orbit := range payload.Orbits {
		orbitByID[orbit.OrbitID] = orbit
	}

	require.Equal(t, 1, orbitByID["docs"].DeclaredCount)
	require.Zero(t, orbitByID["docs"].MissingCount)
	require.Len(t, orbitByID["docs"].Variables, 1)
	require.Equal(t, "project_name", orbitByID["docs"].Variables[0].Name)
	require.True(t, orbitByID["docs"].Variables[0].HasValue)
	require.False(t, orbitByID["docs"].Variables[0].Missing)

	require.Equal(t, 2, orbitByID["cmd"].DeclaredCount)
	require.Equal(t, 1, orbitByID["cmd"].MissingCount)
	require.Len(t, orbitByID["cmd"].Variables, 2)
	require.Equal(t, "binary_name", orbitByID["cmd"].Variables[0].Name)
	require.Equal(t, "CLI binary", orbitByID["cmd"].Variables[0].Description)
	require.True(t, orbitByID["cmd"].Variables[0].Required)
	require.False(t, orbitByID["cmd"].Variables[0].HasValue)
	require.True(t, orbitByID["cmd"].Variables[0].Missing)
	require.Equal(t, "project_name", orbitByID["cmd"].Variables[1].Name)
	require.True(t, orbitByID["cmd"].Variables[1].HasValue)
	require.False(t, orbitByID["cmd"].Variables[1].Missing)
}

func TestHarnessBindingsMissingTextOutputMapsToReadinessReason(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
	})

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--allow-unresolved-bindings")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "missing", "--all")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "readiness_reason: unresolved_required_bindings\n")
}

func TestHarnessBindingsMissingRejectsDetachedOrbit(t *testing.T) {
	t.Parallel()

	repo := seedDetachedBindingsRepo(t)

	_, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "missing", "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `inspect missing bindings: orbit "docs" is detached`)
	require.Empty(t, stderr)
}

func TestHarnessBindingsScanRuntimeReportsObservedPlaceholdersAndWritesInstallSnapshot(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Follow $project_name docs workflow\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsBlock))

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"scan-runtime",
		"--orbit",
		"docs",
		"--write-install",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	var payload struct {
		HarnessRoot      string   `json:"harness_root"`
		OrbitCount       int      `json:"orbit_count"`
		OrbitIDs         []string `json:"orbit_ids"`
		PlaceholderCount int      `json:"placeholder_count"`
		WroteInstall     bool     `json:"wrote_install"`
		ReadinessReason  string   `json:"readiness_reason"`
		Orbits           []struct {
			OrbitID                   string   `json:"orbit_id"`
			PathCount                 int      `json:"path_count"`
			PlaceholderCount          int      `json:"placeholder_count"`
			ObservedRuntimeUnresolved []string `json:"observed_runtime_unresolved"`
			WroteInstall              bool     `json:"wrote_install"`
			Paths                     []struct {
				Path      string   `json:"path"`
				Variables []string `json:"variables"`
			} `json:"paths"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, 1, payload.OrbitCount)
	require.Equal(t, []string{"docs"}, payload.OrbitIDs)
	require.Equal(t, 2, payload.PlaceholderCount)
	require.True(t, payload.WroteInstall)
	require.Equal(t, "runtime_placeholders_observed", payload.ReadinessReason)
	require.Len(t, payload.Orbits, 1)
	require.Equal(t, "docs", payload.Orbits[0].OrbitID)
	require.Equal(t, 2, payload.Orbits[0].PathCount)
	require.Equal(t, 2, payload.Orbits[0].PlaceholderCount)
	require.Equal(t, []string{"project_name"}, payload.Orbits[0].ObservedRuntimeUnresolved)
	require.True(t, payload.Orbits[0].WroteInstall)
	require.Len(t, payload.Orbits[0].Paths, 2)
	require.Equal(t, "AGENTS.md", payload.Orbits[0].Paths[0].Path)
	require.Equal(t, []string{"project_name"}, payload.Orbits[0].Paths[0].Variables)
	require.Equal(t, "docs/guide.md", payload.Orbits[0].Paths[1].Path)
	require.Equal(t, []string{"project_name"}, payload.Orbits[0].Paths[1].Variables)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Variables)
	require.Equal(t, []string{"project_name"}, record.Variables.ObservedRuntimeUnresolved)
}

func TestHarnessBindingsScanRuntimeTextOutputMapsToReadinessReason(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--allow-unresolved-bindings")
	require.NoError(t, err)

	agentsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Follow $project_name docs workflow\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(agentsBlock))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "scan-runtime", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "readiness_reason: runtime_placeholders_observed\n")
}

func TestHarnessBindingsScanRuntimeRejectsDetachedOrbit(t *testing.T) {
	t.Parallel()

	repo := seedDetachedBindingsRepo(t)

	_, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "scan-runtime", "--orbit", "docs")
	require.Error(t, err)
	require.ErrorContains(t, err, `scan runtime bindings: orbit "docs" is detached`)
	require.Empty(t, stderr)
}

func TestHarnessBindingsApplyDryRunJSONUsesCurrentVarsWithoutMutatingRuntime(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Filled Orbit\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		ChangedCount int      `json:"changed_count"`
		ChangedPaths []string `json:"changed_paths"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 3, payload.ChangedCount)
	require.Contains(t, payload.ChangedPaths, "docs/guide.md")
	require.Contains(t, payload.ChangedPaths, "AGENTS.md")
	require.Contains(t, payload.ChangedPaths, ".harness/installs/docs.yaml")
	require.Empty(t, payload.Warnings)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Follow $project_name docs workflow\n")

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Variables)
	require.Equal(t, []string{"project_name"}, record.Variables.UnresolvedAtApply)
}

func TestHarnessBindingsApplyWritesRenderedRuntimeAndRefreshesInstallSnapshot(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	beforeRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	beforeRecord.AppliedAt = time.Date(2026, time.March, 21, 12, 0, 0, 0, time.UTC)
	_, err = orbittemplate.WriteInstallRecord(repo.Root, beforeRecord)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Filled Orbit\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Empty(t, payload.Warnings)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Filled Orbit guide\n", string(guideData))

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "Follow Filled Orbit docs workflow\n")
	require.NotContains(t, string(agentsData), "$project_name")

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Variables)
	require.True(t, record.AppliedAt.After(beforeRecord.AppliedAt))
	require.Equal(t, map[string]bindings.VariableBinding{
		"project_name": {
			Value:       "Filled Orbit",
			Description: "Product title",
		},
	}, record.Variables.ResolvedAtApply)
	require.Empty(t, record.Variables.UnresolvedAtApply)
	require.Empty(t, record.Variables.ObservedRuntimeUnresolved)

	checkStdout, checkStderr, err := executeHarnessCLI(t, repo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, checkStderr)
	checkPayload := decodeHarnessCheckPayload(t, checkStdout)
	require.True(t, checkPayload.OK)
	require.Empty(t, checkPayload.Findings)
}

func TestHarnessBindingsApplyAllowsUnresolvedBindingsAsWarnings(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
		Readiness    struct {
			Status    string `json:"status"`
			NextSteps []struct {
				Command string `json:"command"`
			} `json:"next_steps"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Equal(t, []string{
		"bindings apply kept template variables unresolved: project_name",
	}, payload.Warnings)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.Contains(t, readinessCommandsFromPayload(payload.Readiness.NextSteps), "hyard plumbing harness bindings missing --all --json")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Variables)
	require.Empty(t, record.Variables.ResolvedAtApply)
	require.Equal(t, []string{"project_name"}, record.Variables.UnresolvedAtApply)
	require.Equal(t, []string{"project_name"}, record.Variables.ObservedRuntimeUnresolved)
}

func TestHarnessBindingsApplyTextOutputIncludesReadinessStatus(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
			AgentsTemplate: "Follow $project_name docs workflow\n",
		},
	})

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--allow-unresolved-bindings")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "apply", "--orbit", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "applied bindings to orbit docs in harness "+repo.Root+"\n")
	require.Contains(t, stdout, "readiness_status: usable\n")
	require.Contains(t, stdout, "readiness_hint: run `hyard ready` for detailed readiness reasons\n")
	require.Contains(t, stdout, "next_step:")
}

func TestHarnessBindingsApplyRejectsDetachedOrbit(t *testing.T) {
	t.Parallel()

	repo := seedDetachedBindingsRepo(t)

	_, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "apply", "--orbit", "docs", "--dry-run")
	require.Error(t, err)
	require.ErrorContains(t, err, `preview bindings apply: orbit "docs" is detached`)
	require.Empty(t, stderr)
}

func TestHarnessBindingsApplyFailsClosedOnDriftWithoutForceAndForceRewritesInstallOwnedPaths(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Filled Orbit\n")
	repo.WriteFile(t, "docs/guide.md", "Locally drifted guide\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `apply bindings to orbit "docs"`)
	require.ErrorContains(t, err, "drift")
	require.ErrorContains(t, err, "--force")

	guideData, readErr := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, readErr)
	require.Equal(t, "Locally drifted guide\n", string(guideData))

	stdout, stderr, err = executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--force",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Forced       bool     `json:"forced"`
		WrittenPaths []string `json:"written_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.Forced)
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")

	guideData, err = os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Filled Orbit guide\n", string(guideData))
}

func TestHarnessBindingsApplyFailsWhenInstalledVariableDeclarationBecomesIncompatible(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
	})

	_, _, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--allow-unresolved-bindings",
	)
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID: "cmd",
		Source:  harnesspkg.MemberSourceInstallOrbit,
		AddedAt: time.Date(2026, time.April, 10, 9, 5, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "cmd",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/cmd",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 10, 9, 6, 0, 0, time.UTC),
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "CLI title",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Orbit",
					Description: "CLI title",
				},
			},
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--dry-run",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `variable conflict for "project_name"`)
	require.ErrorContains(t, err, ".harness/installs/docs.yaml")
	require.ErrorContains(t, err, "orbit-template/docs")
}

func TestHarnessBindingsApplyReportsInvalidLocalVarsBeforeRemoteReplay(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessInstallRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	record.Template.SourceRepo = filepath.Join(t.TempDir(), "missing-remote.git")
	_, err = harnesspkg.WriteInstallRecord(runtimeRepo.Root, record)
	require.NoError(t, err)

	runtimeRepo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"bindings",
		"apply",
		"--orbit",
		"docs",
		"--dry-run",
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "load runtime vars")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.NotContains(t, stderr, "progress: replaying install source\n")
}

func executeHarnessCLI(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := harnesscli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(harnesscommands.WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func executeHarnessCLIWithInput(t *testing.T, workingDir string, input string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := harnesscli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)
	rootCmd.SetIn(strings.NewReader(input))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(harnesscommands.WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func mustReadString(t *testing.T, filename string) string {
	t.Helper()

	data, err := os.ReadFile(filename)
	require.NoError(t, err)

	return string(data)
}

func executeHarnessCLIWithStderrAction(
	t *testing.T,
	workingDir string,
	marker string,
	action func(),
	args ...string,
) (string, string, bool, error) {
	t.Helper()

	rootCmd := harnesscli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)

	var stdout bytes.Buffer
	stderr := &actionBuffer{
		marker: marker,
		action: action,
	}
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(stderr)

	err := rootCmd.ExecuteContext(harnesscommands.WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), stderr.triggered, err
}

func executeOrbitCLI(t *testing.T, workingDir string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := orbitcli.NewCompatibilityRootCommand()
	rootCmd.SetArgs(args)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.ExecuteContext(orbitcommands.WithWorkingDir(context.Background(), workingDir))

	return stdout.String(), stderr.String(), err
}

func TestHarnessHelpIncludesBootstrapCommands(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := executeHarnessCLI(t, t.TempDir(), "--help")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "add")
	require.Contains(t, stdout, "agents")
	require.Contains(t, stdout, "bootstrap")
	require.Contains(t, stdout, "bindings")
	require.Contains(t, stdout, "check")
	require.Contains(t, stdout, "create")
	require.Contains(t, stdout, "init")
	require.Contains(t, stdout, "inspect")
	require.Contains(t, stdout, "install")
	require.Contains(t, stdout, "remove")
	require.Contains(t, stdout, "root")
	require.Contains(t, stdout, "template")
}

func TestHarnessAgentsComposeJSONCreatesRootAgentsFromRuntimeMembers(t *testing.T) {
	t.Parallel()

	repo := seedHarnessAgentsComposeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agents", "compose", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot    string   `json:"harness_root"`
		AgentsPath     string   `json:"agents_path"`
		MemberCount    int      `json:"member_count"`
		ComposedCount  int      `json:"composed_count"`
		SkippedCount   int      `json:"skipped_count"`
		ChangedCount   int      `json:"changed_count"`
		ComposedOrbits []string `json:"composed_orbits"`
		SkippedOrbits  []string `json:"skipped_orbits"`
		Forced         bool     `json:"forced"`
		Readiness      struct {
			Status    string `json:"status"`
			NextSteps []struct {
				Command string `json:"command"`
			} `json:"next_steps"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, filepath.Join(repo.Root, "AGENTS.md"), payload.AgentsPath)
	require.Equal(t, 3, payload.MemberCount)
	require.Equal(t, 3, payload.ComposedCount)
	require.Equal(t, 0, payload.SkippedCount)
	require.Equal(t, 3, payload.ChangedCount)
	require.ElementsMatch(t, []string{"docs", "cmd", "ops"}, payload.ComposedOrbits)
	require.Empty(t, payload.SkippedOrbits)
	require.False(t, payload.Forced)
	require.Equal(t, "ready", payload.Readiness.Status)
	require.Empty(t, payload.Readiness.NextSteps)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	agents := string(agentsData)
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"docs\" -->\nYou are the Acme docs orbit.\n<!-- orbit:end orbit_id=\"docs\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"cmd\" -->\nUse orbitctl for Acme releases.\n<!-- orbit:end orbit_id=\"cmd\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"ops\" -->\nOps orbit\n<!-- orbit:end orbit_id=\"ops\" -->\n")
	require.NotContains(t, agents, "Workspace overview.\n")
}

func TestHarnessAgentsComposeTextOutputIncludesReadinessStatus(t *testing.T) {
	t.Parallel()

	repo := seedHarnessAgentsComposeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agents", "compose")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "composed root AGENTS.md for harness "+repo.Root+"\n")
	require.Contains(t, stdout, "readiness_status: ready\n")
	require.NotContains(t, stdout, "readiness_hint:")
	require.NotContains(t, stdout, "next_step:")
}

func TestHarnessAgentsComposeFailsClosedOnDriftedOrbitBlockWithoutForce(t *testing.T) {
	t.Parallel()

	repo := seedHarnessAgentsComposeRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"You are the Drifted docs orbit.\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")

	originalAgents, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agents", "compose")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `compose orbit "docs" into root AGENTS.md`)
	require.ErrorContains(t, err, `root AGENTS.md already contains drifted orbit block "docs"`)
	require.ErrorContains(t, err, "orbit brief backfill --orbit docs")
	require.ErrorContains(t, err, "--force")

	agentsData, readErr := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, readErr)
	require.Equal(t, string(originalAgents), string(agentsData))
}

func TestHarnessAgentsComposeForcePreservesProseAndNonTargetBlocks(t *testing.T) {
	t.Parallel()

	repo := seedHarnessAgentsComposeRepo(t)
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace overview.\n"+
		"<!-- orbit:begin orbit_id=\"api\" -->\n"+
		"API brief.\n"+
		"<!-- orbit:end orbit_id=\"api\" -->\n"+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"You are the Drifted docs orbit.\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "agents", "compose", "--force", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ComposedCount  int      `json:"composed_count"`
		SkippedCount   int      `json:"skipped_count"`
		ChangedCount   int      `json:"changed_count"`
		ComposedOrbits []string `json:"composed_orbits"`
		Forced         bool     `json:"forced"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 3, payload.ComposedCount)
	require.Equal(t, 0, payload.SkippedCount)
	require.Equal(t, 3, payload.ChangedCount)
	require.ElementsMatch(t, []string{"docs", "cmd", "ops"}, payload.ComposedOrbits)
	require.True(t, payload.Forced)

	agentsData, err := os.ReadFile(filepath.Join(repo.Root, "AGENTS.md"))
	require.NoError(t, err)
	agents := string(agentsData)
	require.Contains(t, agents, "Workspace overview.\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"api\" -->\nAPI brief.\n<!-- orbit:end orbit_id=\"api\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"docs\" -->\nYou are the Acme docs orbit.\n<!-- orbit:end orbit_id=\"docs\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"cmd\" -->\nUse orbitctl for Acme releases.\n<!-- orbit:end orbit_id=\"cmd\" -->\n")
	require.Contains(t, agents, "<!-- orbit:begin orbit_id=\"ops\" -->\nOps orbit\n<!-- orbit:end orbit_id=\"ops\" -->\n")
	require.NotContains(t, agents, "You are the Drifted docs orbit.\n")
}

func TestHarnessBindingsPlanJSONMergesLocalTemplateVariablesAndPrefillsRepoValues(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBindingsPlanRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "Orbit guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run Orbit as `orbit`.\n",
			},
		},
	}, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"plan",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RepoRoot    string `json:"repo_root"`
		SourceCount int    `json:"source_count"`
		Sources     []struct {
			Kind    string `json:"kind"`
			Ref     string `json:"ref"`
			OrbitID string `json:"orbit_id"`
			Commit  string `json:"commit"`
		} `json:"sources"`
		ReusedValues    []string `json:"reused_values"`
		MissingRequired []string `json:"missing_required"`
		Bindings        struct {
			SchemaVersion int `json:"schema_version"`
			Variables     map[string]struct {
				Value       string `json:"value"`
				Description string `json:"description"`
			} `json:"variables"`
		} `json:"bindings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.RepoRoot)
	require.Equal(t, 2, payload.SourceCount)
	require.Len(t, payload.Sources, 2)
	require.Equal(t, []string{"project_name"}, payload.ReusedValues)
	require.Equal(t, []string{"binary_name"}, payload.MissingRequired)
	require.Equal(t, 1, payload.Bindings.SchemaVersion)
	require.Equal(t, "Orbit", payload.Bindings.Variables["project_name"].Value)
	require.Equal(t, "Product title", payload.Bindings.Variables["project_name"].Description)
	require.Empty(t, payload.Bindings.Variables["binary_name"].Value)
	require.Equal(t, "CLI binary", payload.Bindings.Variables["binary_name"].Description)
}

func TestHarnessBindingsPlanNamespacesVariableDescriptionConflict(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBindingsPlanRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "Orbit guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: CLI title\n",
			Files: map[string]string{
				"cmd/README.md": "Orbit command guide\n",
			},
		},
	}, ""+
		"schema_version: 1\n"+
		"variables: {}\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"plan",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		MissingRequired []string `json:"missing_required"`
		Bindings        struct {
			Variables       map[string]struct{} `json:"variables"`
			ScopedVariables map[string]struct {
				Variables map[string]struct {
					Value       string `json:"value"`
					Description string `json:"description"`
				} `json:"variables"`
			} `json:"scoped_variables"`
		} `json:"bindings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.ElementsMatch(t, []string{"docs:project_name", "cmd:project_name"}, payload.MissingRequired)
	require.Empty(t, payload.Bindings.Variables)
	require.Equal(t, "Product title", payload.Bindings.ScopedVariables["docs"].Variables["project_name"].Description)
	require.Equal(t, "CLI title", payload.Bindings.ScopedVariables["cmd"].Variables["project_name"].Description)
}

func TestHarnessBindingsPlanOutPreservesUnrelatedExistingVars(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBindingsPlanRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "Orbit guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run Orbit as `orbit`.\n",
			},
		},
	}, ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  github_token:\n"+
		"    value: \"${{ secrets.GITHUB_TOKEN }}\"\n"+
		"    description: CI token\n"+
		"scoped_variables:\n"+
		"  ops:\n"+
		"    variables:\n"+
		"      service_name:\n"+
		"        value: orbit-api\n"+
		"        description: Ops service\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"plan",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--out",
		".harness/vars.yaml",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "wrote bindings plan to "+filepath.Join(repo.Root, ".harness", "vars.yaml")+"\n", stdout)

	file, err := harnesspkg.LoadVarsFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"binary_name": {
				Value:       "",
				Description: "CLI binary",
			},
			"github_token": {
				Value:       "${{ secrets.GITHUB_TOKEN }}",
				Description: "CI token",
			},
			"project_name": {
				Value:       "Orbit",
				Description: "Product title",
			},
		},
		ScopedVariables: map[string]bindings.ScopedVariableBindings{
			"ops": {
				Variables: map[string]bindings.VariableBinding{
					"service_name": {
						Value:       "orbit-api",
						Description: "Ops service",
					},
				},
			},
		},
	}, file)
}

func TestHarnessBindingsPlanReportsInvalidLocalVarsBeforeRemoteResolution(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"bindings",
		"plan",
		missingRemote,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "load harness vars from worktree")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.NotContains(t, stderr, "progress: preflighting source 1/1\n")
}

func TestHarnessAddAndRemoveManageManualMembers(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".harness", "orbits"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), []byte(""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"include:\n"+
		"  - docs/**\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "added orbit docs to harness "+repo.Root+"\n", stdout)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceManual, runtimeFile.Members[0].Source)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "removed orbit docs from harness "+repo.Root+"\n", stdout)

	runtimeFile, err = harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)
}

func TestHarnessAddAndRemoveJSONOutputUsesManifestPath(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".harness", "orbits"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), []byte(""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"include:\n"+
		"  - docs/**\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "add", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var addPayload struct {
		HarnessRoot  string `json:"harness_root"`
		OrbitID      string `json:"orbit_id"`
		ManifestPath string `json:"manifest_path"`
		MemberCount  int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &addPayload))
	require.Equal(t, repo.Root, addPayload.HarnessRoot)
	require.Equal(t, "docs", addPayload.OrbitID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), addPayload.ManifestPath)
	require.Equal(t, 1, addPayload.MemberCount)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var removePayload struct {
		HarnessRoot  string `json:"harness_root"`
		OrbitID      string `json:"orbit_id"`
		ManifestPath string `json:"manifest_path"`
		MemberCount  int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &removePayload))
	require.Equal(t, repo.Root, removePayload.HarnessRoot)
	require.Equal(t, "docs", removePayload.OrbitID)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), removePayload.ManifestPath)
	require.Zero(t, removePayload.MemberCount)
}

func TestHarnessRemoveDeletesTemplateMemberFromHarnessTemplate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, err := harnesspkg.WriteTemplateManifest(repo.Root, harnesspkg.TemplateManifest{
		SchemaVersion: 1,
		Kind:          harnesspkg.TemplateKind,
		Template: harnesspkg.TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 30, 0, 0, time.UTC),
			RootGuidance:      harnesspkg.RootGuidanceMetadata{},
		},
		Members: []harnesspkg.TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]harnesspkg.TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindHarnessTemplate,
		Template: &harnesspkg.ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 30, 0, 0, time.UTC),
		},
		Members: []harnesspkg.ManifestMember{
			{OrbitID: "docs"},
		},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+testContentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.AddAndCommit(t, "seed harness template for remove")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "removed orbit docs from harness "+repo.Root+"\n", stdout)

	manifest, err := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, manifest.Members)

	templateManifest, err := harnesspkg.LoadTemplateManifest(repo.Root)
	require.NoError(t, err)
	require.Empty(t, templateManifest.Members)
	require.Empty(t, templateManifest.Variables)
}

func TestHarnessRemoveTemplateMemberJSONOutputIncludesTemplateSummary(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, err := harnesspkg.WriteTemplateManifest(repo.Root, harnesspkg.TemplateManifest{
		SchemaVersion: 1,
		Kind:          harnesspkg.TemplateKind,
		Template: harnesspkg.TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 45, 0, 0, time.UTC),
			RootGuidance: harnesspkg.RootGuidanceMetadata{
				Agents: true,
			},
		},
		Members: []harnesspkg.TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]harnesspkg.TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindHarnessTemplate,
		Template: &harnesspkg.ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 45, 0, 0, time.UTC),
		},
		Members: []harnesspkg.ManifestMember{
			{OrbitID: "docs"},
		},
		RootGuidance: harnesspkg.RootGuidanceMetadata{
			Agents: true,
		},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	docsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs guidance\n"))
	require.NoError(t, err)
	agentsContent := []byte(string(docsBlock))
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - AGENTS.md\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    AGENTS.md: "+testContentDigest(agentsContent)+"\n"+
		"    docs/guide.md: "+testContentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.WriteFile(t, "AGENTS.md", string(agentsContent))
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.AddAndCommit(t, "seed harness template remove json repo")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot        string   `json:"harness_root"`
		OrbitID            string   `json:"orbit_id"`
		RevisionKind       string   `json:"revision_kind"`
		RemoveMode         string   `json:"remove_mode"`
		ManifestPath       string   `json:"manifest_path"`
		TemplatePath       string   `json:"template_path"`
		MemberCount        int      `json:"member_count"`
		RemovedPaths       []string `json:"removed_paths"`
		RemovedAgentsBlock bool     `json:"removed_agents_block"`
		ZeroMemberTemplate bool     `json:"zero_member_template"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, harnesspkg.ManifestKindHarnessTemplate, payload.RevisionKind)
	require.Equal(t, "template_full_remove", payload.RemoveMode)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), payload.ManifestPath)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "template.yaml"), payload.TemplatePath)
	require.Zero(t, payload.MemberCount)
	require.Equal(t, []string{
		".harness/orbits/docs.yaml",
		".harness/template_members/docs.yaml",
		"AGENTS.md",
		"docs/guide.md",
	}, payload.RemovedPaths)
	require.True(t, payload.RemovedAgentsBlock)
	require.True(t, payload.ZeroMemberTemplate)
}

func TestHarnessRemoveTemplateMemberJSONOutputKeepsFalseSummaryFields(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, err := harnesspkg.WriteTemplateManifest(repo.Root, harnesspkg.TemplateManifest{
		SchemaVersion: 1,
		Kind:          harnesspkg.TemplateKind,
		Template: harnesspkg.TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 40, 0, 0, time.UTC),
			RootGuidance:      harnesspkg.RootGuidanceMetadata{},
		},
		Members: []harnesspkg.TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]harnesspkg.TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindHarnessTemplate,
		Template: &harnesspkg.ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 40, 0, 0, time.UTC),
		},
		Members: []harnesspkg.ManifestMember{
			{OrbitID: "docs"},
		},
		RootGuidance: harnesspkg.RootGuidanceMetadata{},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		"    - docs/guide.md\n"+
		"  file_digests:\n"+
		"    docs/guide.md: "+testContentDigest([]byte("Docs $project_name guide\n"))+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.AddAndCommit(t, "seed harness template remove false summary repo")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	require.Contains(t, raw, "removed_paths")
	require.Contains(t, raw, "removed_agents_block")
	require.Contains(t, raw, "auto_left_current_orbit")
	require.Contains(t, raw, "detached_install_record")
	require.Contains(t, raw, "zero_member_template")
	require.Equal(t, false, raw["removed_agents_block"])
	require.Equal(t, false, raw["auto_left_current_orbit"])
	require.Equal(t, false, raw["detached_install_record"])
	require.Equal(t, true, raw["zero_member_template"])
}

func TestHarnessRemoveTemplateMemberRejectsMissingSnapshot(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveCLIRepo(t, false)
	require.NoError(t, os.Remove(filepath.Join(repo.Root, ".harness", "template_members", "docs.yaml")))
	repo.Run(t, "add", "-A", ".")
	repo.Run(t, "commit", "-m", "remove template snapshot")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `template member snapshot for "docs" is required`)

	manifest, loadErr := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, loadErr)
	require.Equal(t, []harnesspkg.ManifestMember{{Package: testOrbitPackage("docs"), OrbitID: "docs"}}, manifest.Members)
}

func TestHarnessRemoveTemplateMemberRejectsDirtyAgentsPath(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveCLIRepo(t, true)
	repo.WriteFile(t, "AGENTS.md", "Locally edited guidance\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot remove template member "docs" with uncommitted changes`)
	require.ErrorContains(t, err, "AGENTS.md")
}

func TestHarnessRemoveTemplateMemberRejectsDirtyTemplateManifestPath(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveCLIRepo(t, false)
	templateManifestPath := filepath.Join(repo.Root, ".harness", "template.yaml")
	templateManifestData, err := os.ReadFile(templateManifestPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(templateManifestPath, append(templateManifestData, []byte("# local edit\n")...), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot remove template member "docs" with uncommitted changes`)
	require.ErrorContains(t, err, ".harness/template.yaml")
}

func TestHarnessRemoveTemplateMemberRejectsDirtyBranchManifestPath(t *testing.T) {
	t.Parallel()

	repo := seedSingleMemberHarnessTemplateRemoveCLIRepo(t, false)
	manifestPath := filepath.Join(repo.Root, ".harness", "manifest.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, append(manifestData, []byte("# local edit\n")...), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `cannot remove template member "docs" with uncommitted changes`)
	require.ErrorContains(t, err, ".harness/manifest.yaml")
}

func seedSingleMemberHarnessTemplateRemoveCLIRepo(t *testing.T, withAgents bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	repo.Run(t, "branch", "-m", "main")

	_, err := harnesspkg.WriteTemplateManifest(repo.Root, harnesspkg.TemplateManifest{
		SchemaVersion: 1,
		Kind:          harnesspkg.TemplateKind,
		Template: harnesspkg.TemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 50, 0, 0, time.UTC),
			RootGuidance: harnesspkg.RootGuidanceMetadata{
				Agents: withAgents,
			},
		},
		Members: []harnesspkg.TemplateMember{
			{OrbitID: "docs"},
		},
		Variables: map[string]harnesspkg.TemplateVariableSpec{
			"project_name": {Description: "Project name", Required: true},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindHarnessTemplate,
		Template: &harnesspkg.ManifestTemplateMetadata{
			HarnessID:         "workspace",
			DefaultTemplate:   false,
			CreatedFromBranch: "main",
			CreatedFromCommit: "abc123",
			CreatedAt:         time.Date(2026, time.April, 16, 12, 50, 0, 0, time.UTC),
		},
		Members: []harnesspkg.ManifestMember{
			{OrbitID: "docs"},
		},
		RootGuidance: harnesspkg.RootGuidanceMetadata{
			Agents: withAgents,
		},
	})
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	exportedPaths := []string{"docs/guide.md"}
	fileDigestLines := []string{
		"    docs/guide.md: " + testContentDigest([]byte("Docs $project_name guide\n")),
	}
	if withAgents {
		docsBlock, wrapErr := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs guidance\n"))
		require.NoError(t, wrapErr)
		agentsContent := []byte(string(docsBlock))
		exportedPaths = append([]string{"AGENTS.md"}, exportedPaths...)
		fileDigestLines = append([]string{
			"    AGENTS.md: " + testContentDigest(agentsContent),
		}, fileDigestLines...)
		repo.WriteFile(t, "AGENTS.md", string(agentsContent))
	}

	repo.WriteFile(t, ".harness/template_members/docs.yaml", ""+
		"schema_version: 1\n"+
		"kind: harness_template_member_snapshot\n"+
		"orbit_id: docs\n"+
		"member_source: manual\n"+
		"snapshot:\n"+
		"  exported_paths:\n"+
		templateSnapshotExportedPathsYAML(exportedPaths)+
		"  file_digests:\n"+
		strings.Join(fileDigestLines, "\n")+"\n"+
		"  variables:\n"+
		"    project_name:\n"+
		"      description: Project name\n"+
		"      required: true\n")
	repo.WriteFile(t, "docs/guide.md", "Docs $project_name guide\n")
	repo.AddAndCommit(t, "seed single member harness template remove cli repo")

	return repo
}

func templateSnapshotExportedPathsYAML(paths []string) string {
	var builder strings.Builder
	for _, path := range paths {
		builder.WriteString("    - ")
		builder.WriteString(path)
		builder.WriteString("\n")
	}

	return builder.String()
}

func TestHarnessRemoveRejectsBundleBackedRuntimeMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 16, 13, 20, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed bundle-backed runtime remove repo")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `bundle-backed member "docs" has no bundle record`)

	runtimeFile, err = harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Equal(t, []harnesspkg.RuntimeMember{{
		OrbitID:        "docs",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "workspace",
		AddedAt:        time.Date(2026, time.April, 16, 13, 20, 0, 0, time.UTC),
	}}, runtimeFile.Members)
}

func TestHarnessRemoveShrinksBundleBackedRuntimeMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "commit installed harness bundle before shrink remove")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		OrbitID      string   `json:"orbit_id"`
		MemberCount  int      `json:"member_count"`
		RemovedPaths []string `json:"removed_paths"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.MemberCount)
	require.Contains(t, payload.RemovedPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.RemovedPaths, "docs/guide.md")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Equal(t, []harnesspkg.RuntimeMember{{
		OrbitID:        "cmd",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root),
		AddedAt:        runtimeFile.Members[0].AddedAt,
	}}, runtimeFile.Members)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root))
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, bundleRecord.MemberIDs)
	require.NotContains(t, bundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, bundleRecord.OwnedPaths, "docs/guide.md")

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	cmdData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "cmd", "main.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n\nconst name = \"orbitctl\"\n", string(cmdData))
}

func TestHarnessMemberExtractDetachedKeepsPayloadAndRemovesBundleOwnership(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	for index := range runtimeFile.Members {
		if runtimeFile.Members[index].OrbitID != "docs" {
			continue
		}
		runtimeFile.Members[index].LastStandaloneOrigin = &orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs-legacy",
			TemplateCommit: "abc123def456",
		}
	}
	_, err = harnesspkg.WriteRuntimeFile(runtimeRepo.Root, runtimeFile)
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "record standalone origin before extract detached")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--detached", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot         string   `json:"harness_root"`
		OrbitID             string   `json:"orbit_id"`
		RevisionKind        string   `json:"revision_kind"`
		ExtractMode         string   `json:"extract_mode"`
		ManifestPath        string   `json:"manifest_path"`
		MemberCount         int      `json:"member_count"`
		WrittenPaths        []string `json:"written_paths"`
		RemovedPaths        []string `json:"removed_paths"`
		RemovedAgentsBlock  bool     `json:"removed_agents_block"`
		DeletedBundleRecord bool     `json:"deleted_bundle_record"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, harnesspkg.ManifestKindRuntime, payload.RevisionKind)
	require.Equal(t, "detached", payload.ExtractMode)
	require.Equal(t, 2, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)+".yaml")
	require.Empty(t, payload.RemovedPaths)
	require.False(t, payload.RemovedAgentsBlock)
	require.False(t, payload.DeletedBundleRecord)

	runtimeFile, err = harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID: "docs",
			Source:  harnesspkg.MemberSourceManual,
			AddedAt: runtimeFile.Members[0].AddedAt,
			LastStandaloneOrigin: &orbittemplate.Source{
				SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
				SourceRef:      "orbit-template/docs-legacy",
				TemplateCommit: "abc123def456",
			},
		},
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root),
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root))
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, bundleRecord.MemberIDs)
	require.NotContains(t, bundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, bundleRecord.OwnedPaths, "docs/guide.md")

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))
	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, err)
}

func TestHarnessMemberExtractDetachedRemovesSingleMemberBundleAgentsBlock(t *testing.T) {
	t.Parallel()

	sourceRepo := seedSingleMemberHarnessTemplateSaveRepo(t, true)
	harnessID := harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "commit single member bundle before detached extract")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--detached", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		RemovedPaths        []string `json:"removed_paths"`
		RemovedAgentsBlock  bool     `json:"removed_agents_block"`
		DeletedBundleRecord bool     `json:"deleted_bundle_record"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.RemovedPaths, ".harness/bundles/"+harnessID+".yaml")
	require.Contains(t, payload.RemovedPaths, "AGENTS.md")
	require.True(t, payload.RemovedAgentsBlock)
	require.True(t, payload.DeletedBundleRecord)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Equal(t, []harnesspkg.RuntimeMember{{
		OrbitID: "docs",
		Source:  harnesspkg.MemberSourceManual,
		AddedAt: runtimeFile.Members[0].AddedAt,
	}}, runtimeFile.Members)

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "bundles", harnessID+".yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))
}

func TestHarnessMemberExtractDetachedRejectsBundleWithSurvivingRootAgentsBlock(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "commit multi member bundle with root agents before detached extract")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--detached")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `root AGENTS.md`)
	require.ErrorContains(t, err, `surviving bundle members`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 2)
}

func TestHarnessMemberExtractToWritesInstallRecordAndRehomesMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--to", "orbit-template/docs-extracted", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot         string   `json:"harness_root"`
		OrbitID             string   `json:"orbit_id"`
		RevisionKind        string   `json:"revision_kind"`
		ExtractMode         string   `json:"extract_mode"`
		ManifestPath        string   `json:"manifest_path"`
		MemberCount         int      `json:"member_count"`
		TargetBranch        string   `json:"target_branch"`
		TemplateCommit      string   `json:"template_commit"`
		InstallRecordPath   string   `json:"install_record_path"`
		WrittenPaths        []string `json:"written_paths"`
		RemovedPaths        []string `json:"removed_paths"`
		RemovedAgentsBlock  bool     `json:"removed_agents_block"`
		DeletedBundleRecord bool     `json:"deleted_bundle_record"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, harnesspkg.ManifestKindRuntime, payload.RevisionKind)
	require.Equal(t, "to", payload.ExtractMode)
	require.Equal(t, "orbit-template/docs-extracted", payload.TargetBranch)
	require.NotEmpty(t, payload.TemplateCommit)
	require.Equal(t, filepath.Join(runtimeRepo.Root, ".harness", "installs", "docs.yaml"), payload.InstallRecordPath)
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)+".yaml")
	require.Empty(t, payload.RemovedPaths)
	require.False(t, payload.RemovedAgentsBlock)
	require.False(t, payload.DeletedBundleRecord)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID: "docs",
			Source:  harnesspkg.MemberSourceInstallOrbit,
			AddedAt: runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root),
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "docs", record.OrbitID)
	require.Equal(t, orbittemplate.InstallSourceKindLocalBranch, record.Template.SourceKind)
	require.Equal(t, "orbit-template/docs-extracted", record.Template.SourceRef)
	require.Equal(t, payload.TemplateCommit, record.Template.TemplateCommit)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root))
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, bundleRecord.MemberIDs)
	require.NotContains(t, bundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, bundleRecord.OwnedPaths, "docs/guide.md")

	manifestData := runtimeRepo.Run(t, "show", "orbit-template/docs-extracted:.harness/manifest.yaml")
	require.Contains(t, manifestData, "kind: orbit_template")
	require.Contains(t, manifestData, "orbit_id: docs")
}

func TestHarnessMemberExtractToRejectsDetachedHead(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	head := runtimeRepo.RevParse(t, "HEAD")
	runtimeRepo.Run(t, "checkout", head)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--to", "orbit-template/docs-detached-head")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "template save requires a current branch; detached HEAD is not supported")

	runtimeFile, loadErr := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, loadErr)
	require.Len(t, runtimeFile.Members, 2)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, runtimeFile.Members[0].Source)
}

func TestHarnessMemberExtractReuseOriginFailsWithoutHint(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--reuse-origin")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "last_standalone_origin")

	runtimeFile, loadErr := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, loadErr)
	require.Len(t, runtimeFile.Members, 2)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, runtimeFile.Members[0].Source)
}

func TestHarnessMemberExtractReuseOriginRestoresInstallOrbit(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	for index := range runtimeFile.Members {
		if runtimeFile.Members[index].OrbitID != "docs" {
			continue
		}
		runtimeFile.Members[index].LastStandaloneOrigin = &orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs-legacy",
			TemplateCommit: "abc123def456",
		}
	}
	_, err = harnesspkg.WriteRuntimeFile(runtimeRepo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--reuse-origin", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		ExtractMode       string `json:"extract_mode"`
		TargetBranch      string `json:"target_branch"`
		TemplateCommit    string `json:"template_commit"`
		InstallRecordPath string `json:"install_record_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "reuse_origin", payload.ExtractMode)
	require.Equal(t, "orbit-template/docs-legacy", payload.TargetBranch)
	require.NotEmpty(t, payload.TemplateCommit)
	require.Equal(t, filepath.Join(runtimeRepo.Root, ".harness", "installs", "docs.yaml"), payload.InstallRecordPath)

	runtimeFile, err = harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID: "docs",
			Source:  harnesspkg.MemberSourceInstallOrbit,
			AddedAt: runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root),
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	record, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallSourceKindLocalBranch, record.Template.SourceKind)
	require.Equal(t, "orbit-template/docs-legacy", record.Template.SourceRef)
	require.Equal(t, payload.TemplateCommit, record.Template.TemplateCommit)
}

func TestHarnessMemberExtractReuseOriginRejectsExternalGitHint(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	for index := range runtimeFile.Members {
		if runtimeFile.Members[index].OrbitID != "docs" {
			continue
		}
		runtimeFile.Members[index].LastStandaloneOrigin = &orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindExternalGit,
			SourceRepo:     "https://example.com/orbit.git",
			SourceRef:      "orbit-template/docs-legacy",
			TemplateCommit: "abc123def456",
		}
	}
	_, err = harnesspkg.WriteRuntimeFile(runtimeRepo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--reuse-origin")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `source_kind "external_git"`)
}

func TestHarnessMemberExtractToRollsBackSavedBranchOnMutationFailure(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(runtimeRepo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/conflict",
			TemplateCommit: "abc123def456",
		},
		AppliedAt: time.Date(2026, time.April, 23, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--to", "orbit-template/docs-extracted")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "already has install provenance")

	exists, branchErr := gitpkg.LocalBranchExists(context.Background(), runtimeRepo.Root, "orbit-template/docs-extracted")
	require.NoError(t, branchErr)
	require.False(t, exists)

	runtimeFile, loadErr := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, loadErr)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, runtimeFile.Members[0].Source)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), runtimeFile.Members[0].OwnerHarnessID)

	record, recordErr := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, recordErr)
	require.Equal(t, "orbit-template/conflict", record.Template.SourceRef)
}

func TestHarnessMemberExtractReuseOriginRollsBackSavedBranchOnMutationFailure(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath)
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	for index := range runtimeFile.Members {
		if runtimeFile.Members[index].OrbitID != "docs" {
			continue
		}
		runtimeFile.Members[index].LastStandaloneOrigin = &orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/docs-legacy",
			TemplateCommit: "abc123def456",
		}
	}
	_, err = harnesspkg.WriteRuntimeFile(runtimeRepo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteInstallRecord(runtimeRepo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/conflict",
			TemplateCommit: "def456abc123",
		},
		AppliedAt: time.Date(2026, time.April, 23, 11, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "member", "extract", "--orbit", "docs", "--reuse-origin")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "already has install provenance")

	exists, branchErr := gitpkg.LocalBranchExists(context.Background(), runtimeRepo.Root, "orbit-template/docs-legacy")
	require.NoError(t, branchErr)
	require.False(t, exists)

	runtimeFile, err = harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Equal(t, harnesspkg.MemberSourceInstallBundle, runtimeFile.Members[0].Source)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), runtimeFile.Members[0].OwnerHarnessID)

	record, recordErr := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, recordErr)
	require.Equal(t, "orbit-template/conflict", record.Template.SourceRef)
}

func TestHarnessRemoveRuntimeCleanupAutoLeavesCurrentOrbit(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-subject\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/guide.md\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/rules/**\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")
	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/rules/review.md", "Review checklist\n")
	repo.WriteFile(t, "docs/process/flow.md", "Process flow\n")

	docsBlock, err := orbittemplate.WrapRuntimeAgentsBlock("docs", []byte("Docs runtime guidance\n"))
	require.NoError(t, err)
	repo.WriteFile(t, "AGENTS.md", string(docsBlock))
	repo.AddAndCommit(t, "seed runtime cleanup repo")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members[0].Source = harnesspkg.MemberSourceInstallOrbit
	runtimeFile.Harness.UpdatedAt = time.Date(2026, time.April, 16, 10, 30, 0, 0, time.UTC)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)
	repo.Run(t, "add", ".")
	repo.Run(t, "commit", "-m", "mark docs as install-backed")

	stdout, stderr, err := executeOrbitCLI(t, repo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Contains(t, stderr, "harness runtime readiness is usable")
	require.Contains(t, stdout, "entered orbit docs")

	sparseEnabled, err := gitpkg.SparseCheckoutEnabled(context.Background(), repo.Root)
	require.NoError(t, err)
	require.True(t, sparseEnabled)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "remove", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot           string   `json:"harness_root"`
		OrbitID               string   `json:"orbit_id"`
		RevisionKind          string   `json:"revision_kind"`
		RemoveMode            string   `json:"remove_mode"`
		ManifestPath          string   `json:"manifest_path"`
		MemberCount           int      `json:"member_count"`
		RemovedPaths          []string `json:"removed_paths"`
		RemovedAgentsBlock    bool     `json:"removed_agents_block"`
		AutoLeftCurrentOrbit  bool     `json:"auto_left_current_orbit"`
		DetachedInstallRecord bool     `json:"detached_install_record"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, harnesspkg.ManifestKindRuntime, payload.RevisionKind)
	require.Equal(t, "runtime_cleanup", payload.RemoveMode)
	require.Equal(t, filepath.Join(repo.Root, ".harness", "manifest.yaml"), payload.ManifestPath)
	require.Zero(t, payload.MemberCount)
	require.Equal(t, []string{
		"AGENTS.md",
		"docs/process/flow.md",
		"docs/rules/review.md",
	}, payload.RemovedPaths)
	require.True(t, payload.RemovedAgentsBlock)
	require.True(t, payload.AutoLeftCurrentOrbit)
	require.True(t, payload.DetachedInstallRecord)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &raw))
	require.Contains(t, raw, "removed_agents_block")
	require.Contains(t, raw, "auto_left_current_orbit")
	require.Contains(t, raw, "detached_install_record")
	require.Contains(t, raw, "zero_member_template")
	require.Equal(t, false, raw["zero_member_template"])

	sparseEnabled, err = gitpkg.SparseCheckoutEnabled(context.Background(), repo.Root)
	require.NoError(t, err)
	require.False(t, sparseEnabled)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	_, err = store.ReadCurrentOrbit()
	require.ErrorIs(t, err, statepkg.ErrCurrentOrbitNotFound)

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo.Root, "docs", "rules", "review.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "docs", "process", "flow.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, "AGENTS.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, orbittemplate.InstallRecordStatusDetached, record.Status)
}

func TestHarnessRemoveRuntimeCleanupRejectsWhenAnotherOrbitProjectionHidesTouchedPaths(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"members:\n"+
		"  - key: docs-subject\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/guide.md\n"+
		"  - key: docs-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/rules/**\n"+
		"  - key: docs-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/process/**\n")
	repo.WriteFile(t, ".harness/orbits/api.yaml", ""+
		"id: api\n"+
		"description: api orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/api.yaml\n"+
		"members:\n"+
		"  - key: api-subject\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - api/spec.md\n"+
		"  - key: api-rules\n"+
		"    role: rule\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - api/rules/**\n"+
		"  - key: api-process\n"+
		"    role: process\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - api/process/**\n")
	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "docs/rules/review.md", "Review checklist\n")
	repo.WriteFile(t, "docs/process/flow.md", "Process flow\n")
	repo.WriteFile(t, "api/spec.md", "API spec\n")
	repo.WriteFile(t, "api/rules/review.md", "API review checklist\n")
	repo.WriteFile(t, "api/process/flow.md", "API process flow\n")
	repo.AddAndCommit(t, "seed multi-orbit runtime repo")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "api")
	require.NoError(t, err)
	repo.Run(t, "add", ".")
	repo.Run(t, "commit", "-m", "add runtime members")

	stdout, stderr, err := executeOrbitCLI(t, repo.Root, "enter", "api")
	require.NoError(t, err)
	require.Contains(t, stderr, "harness runtime readiness is usable")
	require.Contains(t, stdout, "entered orbit api")

	_, stderr, err = executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.Error(t, err)
	require.Contains(t, err.Error(), "leave the current orbit first")
	require.Contains(t, err.Error(), "docs/process/flow.md")
	require.Contains(t, err.Error(), "docs/rules/review.md")
	require.Empty(t, stderr)

	store, err := statepkg.NewFSStore(repo.GitDir(t))
	require.NoError(t, err)
	current, currentErr := store.ReadCurrentOrbit()
	require.NoError(t, currentErr)
	require.Equal(t, "api", current.Orbit)

	sparseEnabled, err := gitpkg.SparseCheckoutEnabled(context.Background(), repo.Root)
	require.NoError(t, err)
	require.True(t, sparseEnabled)
}

func testContentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestHarnessAddIgnoresLegacyRuntimeFileWhenManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	require.NoError(t, os.MkdirAll(filepath.Join(repo.Root, ".harness", "orbits"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"), []byte(""+
		"id: docs\n"+
		"description: docs orbit\n"+
		"include:\n"+
		"  - docs/**\n"), 0o600))
	repo.WriteFile(t, ".harness/runtime.yaml", "schema_version: nope\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "added orbit docs to harness "+repo.Root+"\n", stdout)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
}

func TestHarnessInstallLocalTemplateWritesInstallRecordVarsAndMember(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/vars.yaml")
	require.Equal(t, 1, payload.MemberCount)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)

	manifestFile, err := harnesspkg.LoadManifestFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, manifestFile.Members, 1)
	require.Equal(t, "docs", manifestFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.ManifestMemberSourceInstallOrbit, manifestFile.Members[0].Source)
}

func TestHarnessInstallLocalTemplateRollsBackWhenLateWriteFails(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	manifestPath := filepath.Join(repo.Root, ".harness", "manifest.yaml")

	stdout, stderr, actionTriggered, err := executeHarnessCLIWithStderrAction(
		t,
		repo.Root,
		"progress: updating runtime metadata\n",
		func() {
			replaceCLIInstallPathWithDirectory(t, manifestPath)
		},
		"install",
		"orbit-template/docs",
		"--bindings",
		bindingsPath,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.True(t, actionTriggered)
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.Contains(t, stderr, "progress: resolving bindings\n")
	require.Contains(t, stderr, "progress: checking conflicts\n")
	require.Contains(t, stderr, "progress: writing files\n")
	require.Contains(t, stderr, "progress: updating runtime metadata\n")
	require.ErrorContains(t, err, "record install-backed member")
	require.ErrorContains(t, err, "load harness runtime")
	require.ErrorContains(t, err, "is a directory")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))

	info, err := os.Stat(manifestPath)
	require.NoError(t, err)
	require.False(t, info.IsDir())

	transactionsDir := filepath.Join(repo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestHarnessInstallTextOutputContract(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"installed orbit docs into harness "+repo.Root+"\n"+
		"source_ref: orbit-template/docs\n"+
		"member_count: 1\n"+
		"files: 5\n"+
		"warnings: none\n"+
		"readiness_status: usable\n"+
		"readiness_hint: run `hyard ready` for detailed readiness reasons\n"+
		"next_step: hyard orbit show docs intent=inspect orbit entry contract\n", stdout)
}

func TestHarnessInstallDefaultsToUnresolvedBindingsAndPlaceholderRuntime(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		Warnings     []string `json:"warnings"`
		Readiness    struct {
			Status    string `json:"status"`
			NextSteps []struct {
				Command string `json:"command"`
			} `json:"next_steps"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.NotContains(t, payload.WrittenPaths, ".harness/vars.yaml")
	require.Equal(t, []string{
		"install kept template variables unresolved: project_name",
	}, payload.Warnings)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.Contains(t, readinessCommandsFromPayload(payload.Readiness.NextSteps), "hyard plumbing harness bindings missing --all --json")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotNil(t, record.Variables)
	require.Equal(t, map[string]bindings.VariableDeclaration{
		"project_name": {
			Description: "Product title",
			Required:    true,
		},
	}, record.Variables.Declarations)
	require.Empty(t, record.Variables.ResolvedAtApply)
	require.Equal(t, []string{"project_name"}, record.Variables.UnresolvedAtApply)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallTreatsBlankRepoVarAsUnresolved(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: \"  \"\n"+
		"    description: Product title\n")

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, []string{
		"install kept template variables unresolved: project_name",
	}, payload.Warnings)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(guideData))
}

func TestHarnessInstallStrictBindingsFailsOnMissingBindings(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"orbit-template/docs",
		"--strict-bindings",
		"--json",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "missing required bindings: project_name")

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHarnessInstallPlainProgressWritesStagesToStderr(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--progress", "plain")
	require.NoError(t, err)
	require.Equal(t, ""+
		"installed orbit docs into harness "+repo.Root+"\n"+
		"source_ref: orbit-template/docs\n"+
		"member_count: 1\n"+
		"files: 5\n"+
		"warnings: none\n"+
		"readiness_status: usable\n"+
		"readiness_hint: run `hyard ready` for detailed readiness reasons\n"+
		"next_step: hyard orbit show docs intent=inspect orbit entry contract\n", stdout)
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.Contains(t, stderr, "progress: resolving bindings\n")
	require.Contains(t, stderr, "progress: checking conflicts\n")
	require.Contains(t, stderr, "progress: writing files\n")
	require.Contains(t, stderr, "progress: updating runtime metadata\n")
	require.Contains(t, stderr, "progress: install complete\n")
}

func TestHarnessInstallPlainProgressPreservesJSONStdout(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--progress", "plain", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.Contains(t, stderr, "progress: install complete\n")

	var payload struct {
		OrbitID   string `json:"orbit_id"`
		Readiness struct {
			Status string `json:"status"`
		} `json:"readiness"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, "usable", payload.Readiness.Status)
}

func TestHarnessInstallQuietProgressSuppressesStages(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--progress", "quiet")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, ""+
		"installed orbit docs into harness "+repo.Root+"\n"+
		"source_ref: orbit-template/docs\n"+
		"member_count: 1\n"+
		"files: 5\n"+
		"warnings: none\n"+
		"readiness_status: usable\n"+
		"readiness_hint: run `hyard ready` for detailed readiness reasons\n"+
		"next_step: hyard orbit show docs intent=inspect orbit entry contract\n", stdout)
}

func TestHarnessInstallDryRunJSONIncludesRuntimeWriteAndDoesNotMutateRuntime(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Preview Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun      bool     `json:"dry_run"`
		HarnessRoot string   `json:"harness_root"`
		OrbitID     string   `json:"orbit_id"`
		Files       []string `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Contains(t, payload.Files, "docs/guide.md")
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, ".harness/installs/docs.yaml")
	require.Contains(t, payload.Files, ".harness/manifest.yaml")
	require.Contains(t, payload.Files, ".harness/vars.yaml")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallFailsWhenOrbitAlreadyInstalled(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before template update")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "already installed")
}

func TestHarnessInstallBatchDryRunJSONAggregatesMultipleOrbitPreviews(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
		},
	})
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Batch Orbit\n"+
		"  binary_name:\n"+
		"    value: orbitctl\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--bindings",
		bindingsPath,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun        bool     `json:"dry_run"`
		HarnessRoot   string   `json:"harness_root"`
		ItemCount     int      `json:"item_count"`
		OrbitIDs      []string `json:"orbit_ids"`
		ConflictCount int      `json:"conflict_count"`
		Items         []struct {
			OrbitID string   `json:"orbit_id"`
			Files   []string `json:"files"`
			Source  struct {
				Ref string `json:"ref"`
			} `json:"source"`
			Conflicts []struct {
				Path string `json:"path"`
			} `json:"conflicts"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, 2, payload.ItemCount)
	require.Equal(t, 0, payload.ConflictCount)
	require.ElementsMatch(t, []string{"docs", "cmd"}, payload.OrbitIDs)
	require.Len(t, payload.Items, 2)
	require.Equal(t, "orbit-template/docs", payload.Items[0].Source.Ref)
	require.Equal(t, "docs", payload.Items[0].OrbitID)
	require.Contains(t, payload.Items[0].Files, "docs/guide.md")
	require.Empty(t, payload.Items[0].Conflicts)
	require.Equal(t, "orbit-template/cmd", payload.Items[1].Source.Ref)
	require.Equal(t, "cmd", payload.Items[1].OrbitID)
	require.Contains(t, payload.Items[1].Files, "cmd/README.md")
	require.Empty(t, payload.Items[1].Conflicts)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallBatchInstallsAllItemsAfterSharedPreview(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
		},
	})
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Batch Orbit\n"+
		"  binary_name:\n"+
		"    value: orbitctl\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--bindings",
		bindingsPath,
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		HarnessRoot  string   `json:"harness_root"`
		ItemCount    int      `json:"item_count"`
		OrbitIDs     []string `json:"orbit_ids"`
		MemberCount  int      `json:"member_count"`
		WrittenPaths []string `json:"written_paths"`
		Readiness    struct {
			Status    string `json:"status"`
			NextSteps []struct {
				Command string `json:"command"`
			} `json:"next_steps"`
		} `json:"readiness"`
		Items []struct {
			OrbitID      string   `json:"orbit_id"`
			WrittenPaths []string `json:"written_paths"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, 2, payload.ItemCount)
	require.Equal(t, 2, payload.MemberCount)
	require.ElementsMatch(t, []string{"docs", "cmd"}, payload.OrbitIDs)
	require.Equal(t, "usable", payload.Readiness.Status)
	require.NotEmpty(t, payload.Readiness.NextSteps)
	require.Len(t, payload.Items, 2)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/cmd.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/cmd.yaml")
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")
	require.Contains(t, payload.WrittenPaths, "cmd/README.md")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 2)
	require.ElementsMatch(t, []string{"docs", "cmd"}, []string{runtimeFile.Members[0].OrbitID, runtimeFile.Members[1].OrbitID})

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Batch Orbit guide\n", string(guideData))
	cmdData, err := os.ReadFile(filepath.Join(repo.Root, "cmd", "README.md"))
	require.NoError(t, err)
	require.Equal(t, "Run Batch Orbit as `orbitctl`.\n", string(cmdData))
}

func TestHarnessInstallBatchRollsBackSharedVarsAndEarlierItemsWhenLaterInstallFails(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
		},
	})
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Batch Orbit\n"+
		"  binary_name:\n"+
		"    value: orbitctl\n"), 0o600))

	cmdInstallRecordPath, err := harnesspkg.InstallRecordPath(repo.Root, "cmd")
	require.NoError(t, err)

	stdout, stderr, actionTriggered, err := executeHarnessCLIWithStderrAction(
		t,
		repo.Root,
		"progress: writing install 2/2\n",
		func() {
			replaceCLIInstallPathWithDirectory(t, cmdInstallRecordPath)
		},
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--bindings",
		bindingsPath,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.True(t, actionTriggered)
	require.Contains(t, stderr, "progress: writing shared bindings\n")
	require.Contains(t, stderr, "progress: writing install 1/2\n")
	require.Contains(t, stderr, "progress: writing install 2/2\n")
	require.ErrorContains(t, err, `load existing install record for orbit "cmd"`)
	require.ErrorContains(t, err, "is a directory")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	require.NoFileExists(t, filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, "cmd", "README.md"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "orbits", "cmd.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.NoFileExists(t, filepath.Join(repo.Root, ".harness", "vars.yaml"))

	_, err = os.Stat(cmdInstallRecordPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	transactionsDir := filepath.Join(repo.GitDir(t), "orbit", "state", "transactions")
	entries, readErr := os.ReadDir(transactionsDir)
	if readErr == nil {
		require.Empty(t, entries)
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist)
	}
}

func TestHarnessInstallBatchDefaultsToUnresolvedBindingsAndWarnings(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
		},
	})

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		ItemCount    int      `json:"item_count"`
		OrbitIDs     []string `json:"orbit_ids"`
		MemberCount  int      `json:"member_count"`
		WarningCount int      `json:"warning_count"`
		Items        []struct {
			OrbitID  string   `json:"orbit_id"`
			Warnings []string `json:"warnings"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, 2, payload.ItemCount)
	require.ElementsMatch(t, []string{"docs", "cmd"}, payload.OrbitIDs)
	require.Equal(t, 2, payload.MemberCount)
	require.Equal(t, 2, payload.WarningCount)
	require.Len(t, payload.Items, 2)
	require.Equal(t, "docs", payload.Items[0].OrbitID)
	require.Equal(t, []string{"install kept template variables unresolved: project_name"}, payload.Items[0].Warnings)
	require.Equal(t, "cmd", payload.Items[1].OrbitID)
	require.Equal(t, []string{"install kept template variables unresolved: binary_name, project_name"}, payload.Items[1].Warnings)

	docsData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(docsData))

	cmdData, err := os.ReadFile(filepath.Join(repo.Root, "cmd", "README.md"))
	require.NoError(t, err)
	require.Equal(t, "Run $project_name as `$binary_name`.\n", string(cmdData))

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "vars.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallBatchStrictBindingsFailsOnMissingBindings(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n" +
				"  binary_name:\n" +
				"    value: orbit\n" +
				"    description: CLI binary\n",
			Files: map[string]string{
				"cmd/README.md": "Run $project_name as `$binary_name`.\n",
			},
		},
	})

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--strict-bindings",
		"--json",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "missing required bindings: project_name")

	_, statErr := os.Stat(filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHarnessInstallBatchNamespacesIncompatibleSharedVariableDeclarations(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
		{
			OrbitID: "cmd",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: CLI title\n",
			Files: map[string]string{
				"cmd/README.md": "$project_name CLI\n",
			},
		},
	})

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/cmd",
		"--allow-unresolved-bindings",
		"--json",
	)
	require.NoError(t, err)
	require.NotEmpty(t, stdout)
	require.Empty(t, stderr)

	docsData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name guide\n", string(docsData))
	cmdData, err := os.ReadFile(filepath.Join(repo.Root, "cmd", "README.md"))
	require.NoError(t, err)
	require.Equal(t, "$project_name CLI\n", string(cmdData))

	docsRecord, err := orbittemplate.LoadInstallRecordFile(filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"project_name": "docs"}, docsRecord.Variables.Namespaces)
	cmdRecord, err := orbittemplate.LoadInstallRecordFile(filepath.Join(repo.Root, ".harness", "installs", "cmd.yaml"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"project_name": "cmd"}, cmdRecord.Variables.Namespaces)

	missingStdout, missingStderr, err := executeHarnessCLI(t, repo.Root, "bindings", "missing", "--all", "--json")
	require.NoError(t, err)
	require.Empty(t, missingStderr)
	var missingPayload struct {
		MissingCount int `json:"missing_count"`
		Orbits       []struct {
			OrbitID   string `json:"orbit_id"`
			Variables []struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Missing   bool   `json:"missing"`
			} `json:"variables"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(missingStdout), &missingPayload))
	require.Equal(t, 2, missingPayload.MissingCount)
	for _, orbit := range missingPayload.Orbits {
		require.Len(t, orbit.Variables, 1)
		require.Equal(t, "project_name", orbit.Variables[0].Name)
		require.Equal(t, orbit.OrbitID, orbit.Variables[0].Namespace)
		require.True(t, orbit.Variables[0].Missing)
	}

	scanStdout, scanStderr, err := executeHarnessCLI(t, repo.Root, "bindings", "scan-runtime", "--all", "--json")
	require.NoError(t, err)
	require.Empty(t, scanStderr)
	var scanPayload struct {
		Orbits []struct {
			OrbitID            string            `json:"orbit_id"`
			VariableNamespaces map[string]string `json:"variable_namespaces"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(scanStdout), &scanPayload))
	for _, orbit := range scanPayload.Orbits {
		require.Equal(t, map[string]string{"project_name": orbit.OrbitID}, orbit.VariableNamespaces)
	}
}

func TestHarnessInstallBatchFailsClosedBeforeWritingWhenOrbitIDsRepeat(t *testing.T) {
	t.Parallel()

	repo := seedHarnessBatchInstallRepo(t, []bindingsPlanTemplateSpec{
		{
			OrbitID: "docs",
			VarsYAML: "" +
				"schema_version: 1\n" +
				"variables:\n" +
				"  project_name:\n" +
				"    value: Orbit\n" +
				"    description: Product title\n",
			Files: map[string]string{
				"docs/guide.md": "$project_name guide\n",
			},
		},
	})
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Batch Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		"orbit-template/docs",
		"orbit-template/docs",
		"--bindings",
		bindingsPath,
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `duplicate orbit_id "docs" in install batch`)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Empty(t, runtimeFile.Members)

	_, err = os.Stat(filepath.Join(repo.Root, ".harness", "installs", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallBatchReportsInvalidLocalVarsBeforeRemoteResolution(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		"batch",
		missingRemote,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "load runtime vars")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: preflighting install 1/1\n")
}

func TestHarnessInstallRemoteTemplateWritesInstallRecordVarsAndMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessInstallRepo(t)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	bindingsPath := filepath.Join(runtimeRepo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Remote Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "orbit-template/docs", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		MemberCount  int      `json:"member_count"`
		WrittenPaths []string `json:"written_paths"`
		Source       struct {
			Kind string `json:"kind"`
			Repo string `json:"repo"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.MemberCount)
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "orbit-template/docs", payload.Source.Ref)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/vars.yaml")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)

	installRecord, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, remoteURL, installRecord.Template.SourceRepo)
	require.Equal(t, "orbit-template/docs", installRecord.Template.SourceRef)
}

func TestHarnessInstallRemoteTemplateReportsInvalidLocalVarsBeforeRemoteResolution(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--ref",
		"orbit-template/docs",
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "load runtime vars")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallRemoteWithoutRefReportsInvalidBindingsBeforeRemoteProgress(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n"), 0o600))

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--bindings",
		bindingsPath,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "preflight remote install inputs: load --bindings file")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: resolving external template candidates\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallRemoteTemplateExplicitRefReportsLocalConflictBeforeFetch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--ref",
		"orbit-template/docs",
		"--bindings",
		bindingsPath,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, `orbit "docs" is already installed from "orbit-template/docs"; cross-install override requires --override docs`)
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallRemoteAliasRefReportsInvalidLocalVarsBeforeRemoteProgress(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--ref",
		"main",
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "preflight remote install inputs: load runtime vars")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallAcceptanceSmokeFromPublishedSourceBranch(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	_, _, err = executeOrbitCLI(t, sourceRepo.Root, "template", "publish", "--default")
	require.NoError(t, err)

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "orbit-template/docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var installPayload struct {
		OrbitID string `json:"orbit_id"`
		Source  struct {
			Kind string `json:"kind"`
			Repo string `json:"repo"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &installPayload))
	require.Equal(t, "docs", installPayload.OrbitID)
	require.Equal(t, "external_git", installPayload.Source.Kind)
	require.Equal(t, remoteURL, installPayload.Source.Repo)
	require.Equal(t, "orbit-template/docs", installPayload.Source.Ref)

	templateDefinition, err := gitpkg.ReadFileAtRev(context.Background(), sourceRepo.Root, "orbit-template/docs", ".harness/orbits/docs.yaml")
	require.NoError(t, err)
	runtimeDefinition, readErr := os.ReadFile(filepath.Join(runtimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.NoError(t, readErr)
	templateDefinitionParsed, parseErr := orbitpkg.ParseHostedOrbitSpecData(templateDefinition, ".harness/orbits/docs.yaml")
	require.NoError(t, parseErr)
	runtimeDefinitionParsed, parseErr := orbitpkg.ParseHostedOrbitSpecData(runtimeDefinition, ".harness/orbits/docs.yaml")
	require.NoError(t, parseErr)
	if templateDefinitionParsed.Exclude == nil {
		templateDefinitionParsed.Exclude = []string{}
	}
	if runtimeDefinitionParsed.Exclude == nil {
		runtimeDefinitionParsed.Exclude = []string{}
	}
	require.Equal(t, ".harness/orbits/docs.yaml", templateDefinitionParsed.SourcePath)
	require.Equal(t, ".harness/orbits/docs.yaml", runtimeDefinitionParsed.SourcePath)
	templateDefinitionParsed.SourcePath = ""
	runtimeDefinitionParsed.SourcePath = ""
	require.Equal(t, templateDefinitionParsed, runtimeDefinitionParsed)

	checkStdout, checkStderr, err := executeHarnessCLI(t, runtimeRepo.Root, "check", "--json")
	require.NoError(t, err)
	require.Empty(t, checkStderr)

	checkPayload := decodeHarnessCheckPayload(t, checkStdout)
	require.Truef(t, checkPayload.OK, "unexpected harness check payload: %s", checkStdout)
	require.Zero(t, checkPayload.FindingCount)

	guideData, readErr := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, readErr)
	require.Equal(t, "Orbit guide\n", string(guideData))
}

func TestOrbitRuntimeCommandsWorkAfterHarnessInstallWithoutLegacyOrbitConfig(t *testing.T) {
	t.Parallel()

	runtimeRepo := seedHarnessInstallRepo(t)

	_, err := os.Stat(filepath.Join(runtimeRepo.Root, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "orbit-template/docs", "--allow-unresolved-bindings")
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "commit installed runtime")

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	stdout, stderr, err := executeOrbitCLI(t, runtimeRepo.Root, "enter", "docs")
	require.NoError(t, err)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.Contains(t, stderr, "warning: harness runtime readiness is usable; run `hyard ready` for detailed reasons before worker handoff")

	stdout, stderr, err = executeOrbitCLI(t, runtimeRepo.Root, "current")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "docs\n", stdout)

	stdout, stderr, err = executeOrbitCLI(t, runtimeRepo.Root, "status")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "current: docs\n")
	require.Contains(t, stdout, "in-scope:\nnone\n")

	stdout, stderr, err = executeOrbitCLI(t, runtimeRepo.Root, "diff")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Empty(t, stdout)

	stdout, stderr, err = executeOrbitCLI(t, runtimeRepo.Root, "leave")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, "left orbit docs\n", stdout)
}

func TestOrbitEnterJSONWarnsWhenHarnessRuntimeIsNotReady(t *testing.T) {
	t.Parallel()

	runtimeRepo := seedHarnessInstallRepo(t)

	_, _, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "orbit-template/docs", "--allow-unresolved-bindings")
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "commit installed runtime")

	stdout, stderr, err := executeOrbitCLI(t, runtimeRepo.Root, "enter", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Orbit      string   `json:"orbit"`
		ScopeCount int      `json:"scope_count"`
		Warnings   []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.Orbit)
	require.Equal(t, 2, payload.ScopeCount)
	require.Contains(t, payload.Warnings, "harness runtime readiness is usable; run `hyard ready` for detailed reasons before worker handoff")
}

func TestOrbitRuntimeCommandsWorkInCreatedRuntimeAfterFirstCommit(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessInstallRepo(t)
	baseDir := t.TempDir()
	resolvedBaseDir, err := filepath.EvalSymlinks(baseDir)
	require.NoError(t, err)
	runtimeRoot := filepath.Join(resolvedBaseDir, "runtime-repo")
	bindingsPath := filepath.Join(baseDir, "install-bindings.yaml")

	_, _, err = executeHarnessCLI(t, baseDir, "create", "runtime-repo")
	require.NoError(t, err)
	runGitInDir(t, runtimeRoot, "config", "user.name", "Orbit Test")
	runGitInDir(t, runtimeRoot, "config", "user.email", "orbit@example.com")

	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, err = os.Stat(filepath.Join(runtimeRoot, ".orbit", "config.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, _, err = executeHarnessCLI(t, runtimeRoot, "install", sourceRepo.Root, "--ref", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	runGitInDir(t, runtimeRoot, "add", "-A")
	runGitInDir(t, runtimeRoot, "commit", "-m", "commit installed runtime")

	stdout, stderr, err := executeOrbitCLI(t, runtimeRoot, "enter", "docs")
	require.NoError(t, err)
	require.Equal(t, "entered orbit docs (2 file(s))\n", stdout)
	require.Contains(t, stderr, "warning: harness runtime readiness is usable; run `hyard ready` for detailed reasons before worker handoff")
}

func TestHarnessInstallAcceptanceSmokeFromSourceRepoURL(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	_, _, err = executeOrbitCLI(t, sourceRepo.Root, "template", "publish", "--default")
	require.NoError(t, err)

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var installPayload struct {
		OrbitID string `json:"orbit_id"`
		Source  struct {
			Kind           string `json:"kind"`
			Repo           string `json:"repo"`
			Ref            string `json:"ref"`
			RequestedRef   string `json:"requested_ref"`
			ResolvedRef    string `json:"resolved_ref"`
			ResolutionKind string `json:"resolution_kind"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &installPayload))
	require.Equal(t, "docs", installPayload.OrbitID)
	require.Equal(t, "external_git", installPayload.Source.Kind)
	require.Equal(t, remoteURL, installPayload.Source.Repo)
	require.Equal(t, "orbit-template/docs", installPayload.Source.Ref)
	require.Equal(t, "main", installPayload.Source.RequestedRef)
	require.Equal(t, "orbit-template/docs", installPayload.Source.ResolvedRef)
	require.Equal(t, "source_alias", installPayload.Source.ResolutionKind)
}

func TestHarnessInstallSourceRepoPlainProgressShowsAliasStages(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	_, _, err = executeOrbitCLI(t, sourceRepo.Root, "template", "publish", "--default")
	require.NoError(t, err)

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--progress", "plain", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.Contains(t, stderr, "progress: resolving external template candidates\n")
	require.Contains(t, stderr, "progress: source branch detected; resolving published template\n")
	require.Contains(t, stderr, "progress: fetching selected template\n")
	require.Contains(t, stderr, "progress: install complete\n")

	var payload struct {
		OrbitID string `json:"orbit_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "docs", payload.OrbitID)
}

func TestHarnessInstallAcceptanceSmokeFromSourceRepoAliasRef(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	_, _, err = executeOrbitCLI(t, sourceRepo.Root, "template", "publish", "--default")
	require.NoError(t, err)

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "main", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var installPayload struct {
		OrbitID string `json:"orbit_id"`
		Source  struct {
			Kind           string `json:"kind"`
			Repo           string `json:"repo"`
			Ref            string `json:"ref"`
			RequestedRef   string `json:"requested_ref"`
			ResolvedRef    string `json:"resolved_ref"`
			ResolutionKind string `json:"resolution_kind"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &installPayload))
	require.Equal(t, "docs", installPayload.OrbitID)
	require.Equal(t, "external_git", installPayload.Source.Kind)
	require.Equal(t, remoteURL, installPayload.Source.Repo)
	require.Equal(t, "orbit-template/docs", installPayload.Source.Ref)
	require.Equal(t, "main", installPayload.Source.RequestedRef)
	require.Equal(t, "orbit-template/docs", installPayload.Source.ResolvedRef)
	require.Equal(t, "source_alias", installPayload.Source.ResolutionKind)

	installRecord, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", installRecord.Template.SourceRef)
}

func TestHarnessInstallFailsClosedWhenSourceRepoPublishedTemplateIsMissing(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "main")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "orbit template publish")
	require.ErrorContains(t, err, "orbit-template/docs")
}

func TestHarnessInstallFailsClosedWhenSourceRepoURLPublishedTemplateIsMissing(t *testing.T) {
	t.Parallel()

	sourceRepo := testutil.NewRepo(t)
	sourceRepo.Run(t, "branch", "-m", "main")
	sourceRepo.WriteFile(t, ".orbit/config.yaml", ""+
		"version: 1\n"+
		"shared_scope: []\n"+
		"projection_visible: []\n"+
		"behavior:\n"+
		"  outside_changes_mode: warn\n"+
		"  block_switch_if_hidden_dirty: true\n"+
		"  commit_append_trailer: true\n"+
		"  sparse_checkout_mode: no-cone\n")
	sourceRepo.WriteFile(t, ".orbit/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	sourceRepo.WriteFile(t, "README.md", "author docs\n")
	sourceRepo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	sourceRepo.AddAndCommit(t, "seed source authoring repo")

	_, _, err := executeOrbitCLI(t, sourceRepo.Root, "template", "init-source")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "initialize source branch")

	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "orbit template publish")
	require.ErrorContains(t, err, "orbit-template/docs")
}

func TestHarnessInstallOverwriteExistingReplacesOwnedFilesAndUpdatesInstallRecord(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before reinstall checks")

	originalRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	updatedCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", runtimeBranch)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, "docs/reference.md")

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	referenceData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "reference.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit reference\n", string(referenceData))

	updatedRecord, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.NotEqual(t, originalRecord.Template.TemplateCommit, updatedRecord.Template.TemplateCommit)
	require.Equal(t, updatedCommit, updatedRecord.Template.TemplateCommit)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)
}

func TestHarnessInstallCrossInstallOverrideRequiresExplicitOverride(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before cross-install override check")

	createAlternateDocsTemplateBranch(t, repo)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs-alt", "--bindings", bindingsPath, "--overwrite-existing")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" is already installed from "orbit-template/docs"`)
	require.ErrorContains(t, err, `--override docs`)

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs", record.Template.SourceRef)
}

func TestHarnessInstallCrossInstallOverrideReplacesActiveInstallMember(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before cross-install override")

	alternateCommit := createAlternateDocsTemplateBranch(t, repo)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs-alt", "--bindings", bindingsPath, "--override", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 1, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, "docs/reference.md")

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	referenceData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "reference.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit reference\n", string(referenceData))

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs-alt", record.Template.SourceRef)
	require.Equal(t, alternateCommit, record.Template.TemplateCommit)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)
	require.Empty(t, runtimeFile.Members[0].OwnerHarnessID)
}

func TestHarnessInstallHarnessTemplateOverrideActiveOrbitInstall(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)

	runtimeRepo := seedHarnessInstallRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	orbitBindingsPath := filepath.Join(runtimeRepo.Root, "orbit-bindings.yaml")
	require.NoError(t, os.WriteFile(orbitBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "orbit-template/docs", "--bindings", orbitBindingsPath)
	require.NoError(t, err)

	harnessBindingsPath := filepath.Join(runtimeRepo.Root, "harness-bindings.yaml")
	require.NoError(t, os.WriteFile(harnessBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		"harness-template/workspace",
		"--bindings",
		harnessBindingsPath,
		"--override",
		"docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID    string   `json:"harness_id"`
		MemberIDs    []string `json:"member_ids"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
		BundleCount  int      `json:"bundle_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, []string{"cmd", "docs"}, payload.MemberIDs)
	require.Equal(t, 2, payload.MemberCount)
	require.Equal(t, 1, payload.BundleCount)
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+payload.HarnessID+".yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, "cmd/main.go")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID:        "docs",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "installs", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
}

func TestHarnessInstallOrbitTemplateOverrideBundleBackedMemberRequiresExplicitOverride(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	sourceRemoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)

	orbitRepo := seedHarnessInstallRepo(t)
	createAlternateDocsTemplateBranch(t, orbitRepo)
	orbitRemoteURL := testutil.NewBareRemoteFromRepo(t, orbitRepo)

	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", sourceRemoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")
	runtimeRepo.Run(t, "remote", "add", "orbit-source", orbitRemoteURL)
	runtimeRepo.Run(t, "fetch", "orbit-source", "orbit-template/docs-alt:orbit-template/docs-alt")

	harnessBindingsPath := filepath.Join(runtimeRepo.Root, "harness-bindings.yaml")
	require.NoError(t, os.WriteFile(harnessBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", harnessBindingsPath)
	require.NoError(t, err)

	orbitBindingsPath := filepath.Join(runtimeRepo.Root, "orbit-bindings.yaml")
	require.NoError(t, os.WriteFile(orbitBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "orbit-template/docs-alt", "--bindings", orbitBindingsPath)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `bundle-owned by harness`)
	require.ErrorContains(t, err, `--override docs`)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"cmd", "docs"}, bundleRecord.MemberIDs)

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallHarnessTemplateOverrideBundleBackedMemberShrinksPreviousBundle(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	sourceRemoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	oldHarnessID := harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)

	replacementRepo := seedSingleMemberHarnessTemplateSaveRepo(t, false)
	replacementRepo.WriteFile(t, "docs/guide.md", "Replacement bundle guide\n")
	replacementRepo.AddAndCommit(t, "update replacement bundle docs")
	_, _, err = executeHarnessCLI(t, replacementRepo.Root, "template", "save", "--to", "harness-template/docs-bundle")
	require.NoError(t, err)
	replacementRemoteURL := testutil.NewBareRemoteFromRepo(t, replacementRepo)
	newHarnessID := harnesspkg.DefaultHarnessIDForPath(replacementRepo.Root)

	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", sourceRemoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")
	runtimeRepo.Run(t, "remote", "add", "replacement", replacementRemoteURL)
	runtimeRepo.Run(t, "fetch", "replacement", "harness-template/docs-bundle:harness-template/docs-bundle")

	initialBindingsPath := filepath.Join(runtimeRepo.Root, "initial-bindings.yaml")
	require.NoError(t, os.WriteFile(initialBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", initialBindingsPath)
	require.NoError(t, err)

	replacementBindingsPath := filepath.Join(runtimeRepo.Root, "replacement-bindings.yaml")
	require.NoError(t, os.WriteFile(replacementBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		"harness-template/docs-bundle",
		"--bindings",
		replacementBindingsPath,
		"--override",
		"docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID    string   `json:"harness_id"`
		MemberIDs    []string `json:"member_ids"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
		BundleCount  int      `json:"bundle_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, newHarnessID, payload.HarnessID)
	require.Equal(t, []string{"docs"}, payload.MemberIDs)
	require.Equal(t, 2, payload.MemberCount)
	require.Equal(t, 2, payload.BundleCount)
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+newHarnessID+".yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: oldHarnessID,
			AddedAt:        runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID:        "docs",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: newHarnessID,
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	oldBundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, oldHarnessID)
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, oldBundleRecord.MemberIDs)
	require.NotContains(t, oldBundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, oldBundleRecord.OwnedPaths, "docs/guide.md")

	newBundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, newHarnessID)
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, newBundleRecord.MemberIDs)

	cmdData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "cmd", "main.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n\nconst name = \"orbitctl\"\n", string(cmdData))

	guideData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Replacement bundle guide\n", string(guideData))
}

func TestHarnessInstallOrbitTemplateOverrideBundleBackedMemberShrinksPreviousBundle(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepoWithoutAgents(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	sourceRemoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	oldHarnessID := harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)

	orbitRepo := seedHarnessInstallRepo(t)
	alternateCommit := createAlternateDocsTemplateBranch(t, orbitRepo)
	orbitRemoteURL := testutil.NewBareRemoteFromRepo(t, orbitRepo)

	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", sourceRemoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")
	runtimeRepo.Run(t, "remote", "add", "orbit-source", orbitRemoteURL)
	runtimeRepo.Run(t, "fetch", "orbit-source", "orbit-template/docs-alt:orbit-template/docs-alt")

	harnessBindingsPath := filepath.Join(runtimeRepo.Root, "harness-bindings.yaml")
	require.NoError(t, os.WriteFile(harnessBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", harnessBindingsPath)
	require.NoError(t, err)

	orbitBindingsPath := filepath.Join(runtimeRepo.Root, "orbit-bindings.yaml")
	require.NoError(t, os.WriteFile(orbitBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		"orbit-template/docs-alt",
		"--bindings",
		orbitBindingsPath,
		"--override",
		"docs",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot  string   `json:"harness_root"`
		OrbitID      string   `json:"orbit_id"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, "docs", payload.OrbitID)
	require.Equal(t, 2, payload.MemberCount)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/installs/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, "docs/reference.md")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: oldHarnessID,
			AddedAt:        runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID: "docs",
			Source:  harnesspkg.MemberSourceInstallOrbit,
			AddedAt: runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	oldBundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, oldHarnessID)
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, oldBundleRecord.MemberIDs)
	require.NotContains(t, oldBundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, oldBundleRecord.OwnedPaths, "docs/guide.md")

	installRecord, err := harnesspkg.LoadInstallRecord(runtimeRepo.Root, "docs")
	require.NoError(t, err)
	require.Equal(t, "orbit-template/docs-alt", installRecord.Template.SourceRef)
	require.Equal(t, alternateCommit, installRecord.Template.TemplateCommit)

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	referenceData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "docs", "reference.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit reference\n", string(referenceData))
}

func TestHarnessInstallReinstallAfterRemoveRequiresOverwriteExisting(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before corrupting install record")

	_, _, err = executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" is detached; reinstall requires --overwrite-existing`)

	stdout, stderr, err = executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		MemberCount int `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, 1, payload.MemberCount)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 1)
	require.Equal(t, "docs", runtimeFile.Members[0].OrbitID)
	require.Equal(t, harnesspkg.MemberSourceInstallOrbit, runtimeFile.Members[0].Source)
}

func TestHarnessInstallBatchReinstallAfterRemoveRequiresOverwriteExisting(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before detaching for batch reinstall")

	_, _, err = executeHarnessCLI(t, repo.Root, "remove", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "batch", "orbit-template/docs", "--bindings", bindingsPath)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `orbit "docs" is detached; reinstall requires --overwrite-existing`)
}

func TestHarnessInstallOverwriteFailsWhenExistingOwnedFilesCannotBeSafelyReconstructed(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before corrupting install record")

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	repo.Run(t, "checkout", runtimeBranch)

	installRecordPath, err := harnesspkg.InstallRecordPath(repo.Root, "docs")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(installRecordPath, []byte(""+
		"schema_version: 1\n"+
		"orbit_id: docs\n"+
		"template:\n"+
		"  source_kind: local_branch\n"+
		"  source_repo: \"\"\n"+
		"  source_ref: orbit-template/docs\n"+
		"  template_commit: deadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"+
		"applied_at: 2026-03-26T10:00:00Z\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "reconstruct existing install ownership")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessInstallOverwriteFailsClosedWhenInstallRecordLacksVariablesSnapshot(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)
	repo.AddAndCommit(t, "commit installed runtime before removing snapshot")

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	record.Variables = nil
	_, err = harnesspkg.WriteInstallRecord(repo.Root, record)
	require.NoError(t, err)
	repo.AddAndCommit(t, "remove install variable snapshot before overwrite")

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "update template branch contents")
	repo.Run(t, "checkout", runtimeBranch)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath, "--overwrite-existing")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "reconstruct existing install ownership")
	require.ErrorContains(t, err, "variables snapshot")
	require.ErrorContains(t, err, "overwrite replay")

	guideData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Installed Orbit guide\n", string(guideData))

	_, err = os.Stat(filepath.Join(repo.Root, "docs", "reference.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHarnessBindingsMissingReportsSnapshotlessInstallRecord(t *testing.T) {
	t.Parallel()

	repo := seedHarnessInstallRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"), 0o600))

	_, _, err := executeHarnessCLI(t, repo.Root, "install", "orbit-template/docs", "--bindings", bindingsPath)
	require.NoError(t, err)

	record, err := harnesspkg.LoadInstallRecord(repo.Root, "docs")
	require.NoError(t, err)
	record.Variables = nil
	_, err = harnesspkg.WriteInstallRecord(repo.Root, record)
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "bindings", "missing", "--orbit", "docs", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)

	var payload struct {
		HarnessRoot string   `json:"harness_root"`
		HarnessID   string   `json:"harness_id"`
		OrbitCount  int      `json:"orbit_count"`
		OrbitIDs    []string `json:"orbit_ids"`
		Orbits      []struct {
			OrbitID         string `json:"orbit_id"`
			SnapshotMissing bool   `json:"snapshot_missing"`
			DeclaredCount   int    `json:"declared_count"`
			MissingCount    int    `json:"missing_count"`
			Variables       []struct {
				Name string `json:"name"`
			} `json:"variables"`
		} `json:"orbits"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, runtimeFile.Harness.ID, payload.HarnessID)
	require.Equal(t, 1, payload.OrbitCount)
	require.Equal(t, []string{"docs"}, payload.OrbitIDs)
	require.Len(t, payload.Orbits, 1)
	require.Equal(t, "docs", payload.Orbits[0].OrbitID)
	require.True(t, payload.Orbits[0].SnapshotMissing)
	require.Zero(t, payload.Orbits[0].DeclaredCount)
	require.Zero(t, payload.Orbits[0].MissingCount)
	require.Empty(t, payload.Orbits[0].Variables)
}

func TestHarnessInstallDryRunHarnessTemplateLocalPreviewJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.Run(t, "remote", "add", "source", remoteURL)
	repo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool                          `json:"dry_run"`
		TemplateKind string                        `json:"template_kind"`
		HarnessRoot  string                        `json:"harness_root"`
		HarnessID    string                        `json:"harness_id"`
		MemberIDs    []string                      `json:"member_ids"`
		Files        []string                      `json:"files"`
		Conflicts    []orbittemplate.ApplyConflict `json:"conflicts"`
		Source       struct {
			Kind string `json:"kind"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, "harness_template", payload.TemplateKind)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, []string{"cmd", "docs"}, payload.MemberIDs)
	require.Equal(t, "local_branch", payload.Source.Kind)
	require.Equal(t, "harness-template/workspace", payload.Source.Ref)
	require.Contains(t, payload.Files, ".harness/orbits/cmd.yaml")
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, "AGENTS.md")
	require.Contains(t, payload.Files, "cmd/main.go")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.NotContains(t, payload.Files, ".harness/template.yaml")
	require.Empty(t, payload.Conflicts)
}

func TestHarnessInstallDryRunHarnessTemplateAllowsDisjointWithExistingBundleMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/qa.yaml", ""+
		"id: qa\n"+
		"description: QA orbit\n"+
		"include:\n"+
		"  - qa/**\n")
	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	runtimeFile.Members = append(runtimeFile.Members, harnesspkg.RuntimeMember{
		OrbitID:        "qa",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: "qa-bundle",
		AddedAt:        time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
	})
	_, err = harnesspkg.WriteRuntimeFile(runtimeRepo.Root, runtimeFile)
	require.NoError(t, err)
	_, err = harnesspkg.WriteBundleRecord(runtimeRepo.Root, harnesspkg.BundleRecord{
		SchemaVersion:      1,
		HarnessID:          "qa-bundle",
		Template:           orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/qa", TemplateCommit: "abc123"},
		MemberIDs:          []string{"qa"},
		AppliedAt:          time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		IncludesRootAgents: false,
		OwnedPaths:         []string{"qa/checklist.md"},
	})
	require.NoError(t, err)
	runtimeRepo.AddAndCommit(t, "seed disjoint bundle member")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Conflicts)
}

func TestHarnessInstallDryRunHarnessTemplateAllowsDisjointWithExistingOrbitMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/qa.yaml", ""+
		"id: qa\n"+
		"description: QA orbit\n"+
		"include:\n"+
		"  - qa/**\n")
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "add", "qa")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Empty(t, payload.Conflicts)
}

func TestHarnessInstallDryRunHarnessTemplateConflictsOnExistingMember(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "add", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Conflicts, orbittemplate.ApplyConflict{
		Path:    ".harness/manifest.yaml",
		Message: `member "docs" already exists in harness runtime`,
	})
}

func TestHarnessInstallDryRunHarnessTemplateTextOutputReportsConflicts(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "add", "docs")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "conflicts:\n")
	require.Contains(t, stdout, ".harness/manifest.yaml: member \"docs\" already exists in harness runtime\n")
}

func TestHarnessInstallDryRunHarnessTemplateConflictsOnPathContent(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, "docs/guide.md", "conflicting docs\n")
	runtimeRepo.AddAndCommit(t, "seed conflicting docs path")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Conflicts, orbittemplate.ApplyConflict{
		Path:    "docs/guide.md",
		Message: "target path already exists with different content",
	})
}

func TestHarnessInstallDryRunHarnessTemplateIgnoresStandaloneRuntimeVarsDescription(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	_, err = harnesspkg.WriteVarsFile(runtimeRepo.Root, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "Orbit",
				Description: "A different meaning",
			},
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.NotContains(t, payload.Conflicts, orbittemplate.ApplyConflict{
		Path:    ".harness/vars.yaml",
		Message: `variable conflict for "project_name"`,
	})
}

func TestHarnessInstallDryRunHarnessTemplateConflictsOnInstalledOrbitVariableDeclaration(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/ops.yaml", ""+
		"id: ops\n"+
		"description: Ops orbit\n"+
		"include:\n"+
		"  - ops/**\n")
	_, err = harnesspkg.AddInstallMember(context.Background(), runtimeRepo.Root, "ops", time.Date(2026, time.April, 11, 9, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = harnesspkg.WriteInstallRecord(runtimeRepo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "ops",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRef:      "orbit-template/ops",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 11, 9, 0, 0, 0, time.UTC),
		Variables: &orbittemplate.InstallVariablesSnapshot{
			Declarations: map[string]bindings.VariableDeclaration{
				"project_name": {
					Description: "Operations codename",
					Required:    true,
				},
			},
			ResolvedAtApply: map[string]bindings.VariableBinding{
				"project_name": {
					Value:       "Orbit",
					Description: "Operations codename",
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteVarsFile(runtimeRepo.Root, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "Orbit",
				Description: "Product title",
			},
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	requireVarsConflictMessageContains(t, payload.Conflicts, `variable conflict for "project_name"`)
	requireVarsConflictMessageContains(t, payload.Conflicts, ".harness/installs/ops.yaml")
}

func TestHarnessInstallDryRunHarnessTemplateConflictsOnInstalledBundleVariableDeclaration(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	conflictingRepo := testutil.NewRepo(t)
	_, _, err = executeHarnessCLI(t, conflictingRepo.Root, "init")
	require.NoError(t, err)
	conflictingRepo.WriteFile(t, ".harness/orbits/qa.yaml", ""+
		"id: qa\n"+
		"description: QA orbit\n"+
		"include:\n"+
		"  - qa/**\n")
	conflictingRepo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Conflicting bundle title\n"+
		"  qa_owner:\n"+
		"    value: release-team\n"+
		"    description: QA owner\n")
	conflictingRepo.WriteFile(t, "qa/checklist.md", "$project_name checklist for $qa_owner\n")
	_, _, err = executeHarnessCLI(t, conflictingRepo.Root, "add", "qa")
	require.NoError(t, err)
	conflictingRepo.AddAndCommit(t, "seed conflicting bundle source")
	_, _, err = executeHarnessCLI(t, conflictingRepo.Root, "template", "save", "--to", "harness-template/qa-space")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	conflictingRemoteURL := testutil.NewBareRemoteFromRepo(t, conflictingRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "remote", "add", "conflict", conflictingRemoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")
	runtimeRepo.Run(t, "fetch", "conflict", "harness-template/qa-space:harness-template/qa-space")

	initialBindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(initialBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", initialBindingsPath)
	require.NoError(t, err)

	_, err = harnesspkg.WriteVarsFile(runtimeRepo.Root, bindings.VarsFile{
		SchemaVersion: 1,
		Variables: map[string]bindings.VariableBinding{
			"project_name": {
				Value:       "Installed Orbit",
				Description: "Conflicting bundle title",
			},
			"qa_owner": {
				Value:       "release-team",
				Description: "QA owner",
			},
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/qa-space", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	requireVarsConflictMessageContains(t, payload.Conflicts, `variable conflict for "project_name"`)
	requireVarsConflictMessageContains(
		t,
		payload.Conflicts,
		".harness/bundles/"+harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root)+".yaml",
	)
}

func TestHarnessInstallDryRunHarnessTemplateConflictsOnInvalidAgentsLane(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, "AGENTS.md", "<<broken>>\n<!-- orbit:block:docs -->\n")
	runtimeRepo.AddAndCommit(t, "seed invalid agents")

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Contains(t, payload.Conflicts, orbittemplate.ApplyConflict{
		Path:    "AGENTS.md",
		Message: "runtime AGENTS.md is invalid for harness block merge (" + harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root) + ")",
	})
}

func TestHarnessInstallDryRunHarnessTemplateRemotePreviewJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		remoteURL,
		"--ref",
		"harness-template/workspace",
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		TemplateKind string   `json:"template_kind"`
		HarnessRoot  string   `json:"harness_root"`
		HarnessID    string   `json:"harness_id"`
		MemberIDs    []string `json:"member_ids"`
		Source       struct {
			Kind string `json:"kind"`
			Repo string `json:"repo"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, "harness_template", payload.TemplateKind)
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, []string{"cmd", "docs"}, payload.MemberIDs)
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "harness-template/workspace", payload.Source.Ref)
}

func TestHarnessInstallDryRunSelectsRemoteHarnessTemplateWhenItIsTheOnlyInstallableCandidate(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		remoteURL,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TemplateKind string `json:"template_kind"`
		HarnessID    string `json:"harness_id"`
		Source       struct {
			Kind string `json:"kind"`
			Repo string `json:"repo"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness_template", payload.TemplateKind)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "harness-template/workspace", payload.Source.Ref)
}

func TestHarnessInstallDryRunPrefersUniqueDefaultRemoteHarnessTemplate(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/qa", "--default")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(
		t,
		runtimeRepo.Root,
		"install",
		remoteURL,
		"--dry-run",
		"--json",
	)
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TemplateKind string `json:"template_kind"`
		HarnessID    string `json:"harness_id"`
		Source       struct {
			Kind string `json:"kind"`
			Repo string `json:"repo"`
			Ref  string `json:"ref"`
		} `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness_template", payload.TemplateKind)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, "external_git", payload.Source.Kind)
	require.Equal(t, remoteURL, payload.Source.Repo)
	require.Equal(t, "harness-template/qa", payload.Source.Ref)
}

func TestHarnessInstallHarnessTemplatePlainProgressShowsStages(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", remoteURL, "--ref", "harness-template/workspace", "--dry-run", "--progress", "plain", "--json")
	require.NoError(t, err)
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.Contains(t, stderr, "progress: fetching selected template\n")
	require.Contains(t, stderr, "progress: resolving bindings\n")
	require.Contains(t, stderr, "progress: checking conflicts\n")
	require.Contains(t, stderr, "progress: install complete\n")

	var payload struct {
		TemplateKind string `json:"template_kind"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness_template", payload.TemplateKind)
}

func TestHarnessInstallRemoteHarnessTemplateReportsInvalidBindingsBeforeRemoteResolution(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	bindingsPath := filepath.Join(repo.Root, "install-bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n"), 0o600))

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--ref",
		"harness-template/workspace",
		"--bindings",
		bindingsPath,
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "preflight remote install inputs: load --bindings file")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallRemoteHarnessTemplateReportsInvalidLocalVarsBeforeRemoteResolution(t *testing.T) {
	t.Parallel()

	repo := seedEmptyHarnessRuntimeRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables: {\n"+
		"  github_token: ${{ secrets.GITHUB_TOKEN }}\n"+
		"}\n")

	missingRemote := filepath.Join(t.TempDir(), "missing-remote.git")
	stdout, stderr, err := executeHarnessCLI(
		t,
		repo.Root,
		"install",
		missingRemote,
		"--ref",
		"harness-template/workspace",
		"--progress",
		"plain",
	)
	require.Error(t, err)
	require.Empty(t, stdout)
	require.ErrorContains(t, err, "preflight remote install inputs: load runtime vars")
	require.ErrorContains(t, err, "GitHub Actions expressions must be quoted")
	require.NotContains(t, err.Error(), "does not appear to be a git repository")
	require.Contains(t, stderr, "progress: resolving install source\n")
	require.NotContains(t, stderr, "progress: fetching selected template\n")
}

func TestHarnessInstallHarnessTemplateFailsClosedOnNonDisjointLocalRuntime(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "install", "harness-template/workspace", "--json")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "conflicts detected; mixed harness template install requires disjoint targets")
}

func TestHarnessInstallDryRunHarnessTemplateUsesRenderedContentForPathConflictAnalysis(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, "cmd/main.go", "package main\n\nconst name = \"orbitctl\"\n")
	runtimeRepo.AddAndCommit(t, "seed rendered command path")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath, "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	for _, conflict := range payload.Conflicts {
		require.NotEqual(t, "cmd/main.go", conflict.Path)
	}
}

func TestHarnessInstallHarnessTemplateLocalWriteJSON(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	bindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(bindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", bindingsPath, "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool     `json:"dry_run"`
		HarnessRoot  string   `json:"harness_root"`
		TemplateKind string   `json:"template_kind"`
		HarnessID    string   `json:"harness_id"`
		MemberIDs    []string `json:"member_ids"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
		BundleCount  int      `json:"bundle_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.Equal(t, runtimeRepo.Root, payload.HarnessRoot)
	require.Equal(t, "harness_template", payload.TemplateKind)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, []string{"cmd", "docs"}, payload.MemberIDs)
	require.Equal(t, 2, payload.MemberCount)
	require.Equal(t, 1, payload.BundleCount)
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/cmd.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+payload.HarnessID+".yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/vars.yaml")
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")
	require.Contains(t, payload.WrittenPaths, "cmd/main.go")
	require.Contains(t, payload.WrittenPaths, "docs/guide.md")

	cmdData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "cmd", "main.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n\nconst name = \"orbitctl\"\n", string(cmdData))

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Len(t, runtimeFile.Members, 2)
	require.ElementsMatch(t, []harnesspkg.RuntimeMember{
		{
			OrbitID:        "cmd",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        runtimeFile.Members[0].AddedAt,
		},
		{
			OrbitID:        "docs",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        runtimeFile.Members[1].AddedAt,
		},
	}, runtimeFile.Members)

	manifestFile, err := harnesspkg.LoadManifestFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.ElementsMatch(t, []harnesspkg.ManifestMember{
		{
			Package:        testOrbitPackage("cmd"),
			OrbitID:        "cmd",
			Source:         harnesspkg.ManifestMemberSourceInstallBundle,
			IncludedIn:     testIncludedIn(payload.HarnessID),
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        manifestFile.Members[0].AddedAt,
		},
		{
			Package:        testOrbitPackage("docs"),
			OrbitID:        "docs",
			Source:         harnesspkg.ManifestMemberSourceInstallBundle,
			IncludedIn:     testIncludedIn(payload.HarnessID),
			OwnerHarnessID: payload.HarnessID,
			AddedAt:        manifestFile.Members[1].AddedAt,
		},
	}, manifestFile.Members)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, payload.HarnessID)
	require.NoError(t, err)
	require.Equal(t, []string{"cmd", "docs"}, bundleRecord.MemberIDs)
	require.True(t, bundleRecord.IncludesRootAgents)
	require.Contains(t, bundleRecord.OwnedPaths, "AGENTS.md")
	require.Contains(t, bundleRecord.OwnedPaths, "cmd/main.go")

	agentsData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "AGENTS.md"))
	require.NoError(t, err)
	document, err := orbittemplate.ParseRuntimeAgentsDocument(agentsData)
	require.NoError(t, err)
	require.Len(t, document.Segments, 1)
	require.Equal(t, payload.HarnessID, document.Segments[0].OrbitID)
	require.Contains(t, string(document.Segments[0].Content), "Installed Orbit")
}

func TestHarnessInstallHarnessTemplateOverwriteExistingReplacesSameBundle(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	initialBindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(initialBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", initialBindingsPath)
	require.NoError(t, err)

	_, _, err = executeHarnessCLI(t, sourceRepo.Root, "remove", "docs")
	require.NoError(t, err)
	sourceRepo.WriteFile(t, "cmd/main.go", "package main\n\nconst name = \"orbit-next\"\n")
	sourceRepo.WriteFile(t, "AGENTS.md", "Workspace guide for $project_name v2\n")
	sourceRepo.AddAndCommit(t, "update harness template source")

	_, err = harnesspkg.SaveTemplateBranch(context.Background(), harnesspkg.TemplateSaveInput{
		Preview: harnesspkg.TemplateSavePreviewInput{
			RepoRoot:     sourceRepo.Root,
			TargetBranch: "harness-template/workspace",
			Now:          time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC),
		},
		Overwrite: true,
	})
	require.NoError(t, err)
	sourceRepo.Run(t, "push", remoteURL, "harness-template/workspace")
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	overwriteBindingsPath := filepath.Join(runtimeRepo.Root, "overwrite-bindings.yaml")
	require.NoError(t, os.WriteFile(overwriteBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-next\n"+
		"    description: CLI binary\n"), 0o600))

	dryRunStdout, dryRunStderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", overwriteBindingsPath, "--overwrite-existing", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, dryRunStderr)

	var dryRunPayload struct {
		Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
	}
	require.NoError(t, json.Unmarshal([]byte(dryRunStdout), &dryRunPayload))
	require.Empty(t, dryRunPayload.Conflicts)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", overwriteBindingsPath, "--overwrite-existing", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessID    string   `json:"harness_id"`
		MemberIDs    []string `json:"member_ids"`
		WrittenPaths []string `json:"written_paths"`
		MemberCount  int      `json:"member_count"`
		BundleCount  int      `json:"bundle_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(sourceRepo.Root), payload.HarnessID)
	require.Equal(t, []string{"cmd"}, payload.MemberIDs)
	require.Equal(t, 1, payload.MemberCount)
	require.Equal(t, 1, payload.BundleCount)
	require.Contains(t, payload.WrittenPaths, ".harness/bundles/"+payload.HarnessID+".yaml")
	require.Contains(t, payload.WrittenPaths, ".harness/manifest.yaml")
	require.Contains(t, payload.WrittenPaths, "cmd/main.go")
	require.Contains(t, payload.WrittenPaths, "AGENTS.md")

	cmdData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "cmd", "main.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n\nconst name = \"orbit-next\"\n", string(cmdData))

	_, err = os.Stat(filepath.Join(runtimeRepo.Root, ".harness", "orbits", "docs.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(runtimeRepo.Root, "docs", "guide.md"))
	require.ErrorIs(t, err, os.ErrNotExist)

	runtimeFile, err := harnesspkg.LoadRuntimeFile(runtimeRepo.Root)
	require.NoError(t, err)
	require.Equal(t, []harnesspkg.RuntimeMember{{
		OrbitID:        "cmd",
		Source:         harnesspkg.MemberSourceInstallBundle,
		OwnerHarnessID: payload.HarnessID,
		AddedAt:        runtimeFile.Members[0].AddedAt,
	}}, runtimeFile.Members)

	bundleRecord, err := harnesspkg.LoadBundleRecord(runtimeRepo.Root, payload.HarnessID)
	require.NoError(t, err)
	require.Equal(t, []string{"cmd"}, bundleRecord.MemberIDs)
	require.NotContains(t, bundleRecord.OwnedPaths, ".harness/orbits/docs.yaml")
	require.NotContains(t, bundleRecord.OwnedPaths, "docs/guide.md")

	agentsData, err := os.ReadFile(filepath.Join(runtimeRepo.Root, "AGENTS.md"))
	require.NoError(t, err)
	document, err := orbittemplate.ParseRuntimeAgentsDocument(agentsData)
	require.NoError(t, err)
	require.Len(t, document.Segments, 1)
	require.Equal(t, payload.HarnessID, document.Segments[0].OrbitID)
	require.Contains(t, string(document.Segments[0].Content), "Installed Orbit v2")
}

func TestHarnessInstallHarnessTemplateOverwriteExistingFailsAcrossInstallUnits(t *testing.T) {
	t.Parallel()

	sourceRepo := seedHarnessTemplateSaveRepo(t)
	_, _, err := executeHarnessCLI(t, sourceRepo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	remoteURL := testutil.NewBareRemoteFromRepo(t, sourceRepo)
	runtimeRepo := seedEmptyHarnessRuntimeRepo(t)
	runtimeRepo.Run(t, "remote", "add", "source", remoteURL)
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	initialBindingsPath := filepath.Join(runtimeRepo.Root, "bindings.yaml")
	require.NoError(t, os.WriteFile(initialBindingsPath, []byte(""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Installed Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbit-installed\n"+
		"    description: CLI binary\n"), 0o600))

	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", initialBindingsPath)
	require.NoError(t, err)

	sourceRepo.WriteFile(t, ".harness/orbits/qa.yaml", ""+
		"id: qa\n"+
		"description: QA orbit\n"+
		"include:\n"+
		"  - qa/**\n")
	sourceRepo.WriteFile(t, "qa/checklist.md", "QA checklist\n")
	_, _, err = executeHarnessCLI(t, sourceRepo.Root, "add", "qa")
	require.NoError(t, err)
	sourceRepo.AddAndCommit(t, "add qa member to bundle source")
	_, err = harnesspkg.SaveTemplateBranch(context.Background(), harnesspkg.TemplateSaveInput{
		Preview: harnesspkg.TemplateSavePreviewInput{
			RepoRoot:     sourceRepo.Root,
			TargetBranch: "harness-template/workspace",
			Now:          time.Date(2026, time.April, 1, 13, 0, 0, 0, time.UTC),
		},
		Overwrite: true,
	})
	require.NoError(t, err)
	sourceRepo.Run(t, "push", remoteURL, "harness-template/workspace")
	runtimeRepo.Run(t, "fetch", "source", "harness-template/workspace:harness-template/workspace")

	runtimeRepo.WriteFile(t, ".harness/orbits/qa.yaml", ""+
		"id: qa\n"+
		"description: QA orbit\n"+
		"include:\n"+
		"  - qa/**\n")
	_, _, err = executeHarnessCLI(t, runtimeRepo.Root, "add", "qa")
	require.NoError(t, err)

	stdout, stderr, err := executeHarnessCLI(t, runtimeRepo.Root, "install", "harness-template/workspace", "--bindings", initialBindingsPath, "--overwrite-existing")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, `member "qa" already exists in harness runtime`)
}

func TestHarnessTemplateSaveCreatesHarnessTemplateBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		HarnessRoot     string   `json:"harness_root"`
		HarnessID       string   `json:"harness_id"`
		TargetBranch    string   `json:"target_branch"`
		Commit          string   `json:"commit"`
		Files           []string `json:"files"`
		MemberCount     int      `json:"member_count"`
		DefaultTemplate bool     `json:"default_template"`
		RootGuidance    struct {
			Agents    bool `json:"agents"`
			Humans    bool `json:"humans"`
			Bootstrap bool `json:"bootstrap"`
		} `json:"root_guidance"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.NotEmpty(t, payload.Commit)
	require.Equal(t, 2, payload.MemberCount)
	require.False(t, payload.DefaultTemplate)
	require.True(t, payload.RootGuidance.Agents)
	require.False(t, payload.RootGuidance.Humans)
	require.False(t, payload.RootGuidance.Bootstrap)
	require.Contains(t, payload.Files, ".harness/template.yaml")
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, ".harness/orbits/cmd.yaml")
	require.Contains(t, payload.Files, "docs/guide.md")
	require.Contains(t, payload.Files, "cmd/main.go")
	require.Contains(t, payload.Files, "AGENTS.md")

	require.Equal(t, currentBranch, strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD")))

	files := splitLines(strings.TrimSpace(repo.Run(t, "ls-tree", "-r", "--name-only", "harness-template/workspace")))
	require.Equal(t, []string{
		".harness/manifest.yaml",
		".harness/orbits/cmd.yaml",
		".harness/orbits/docs.yaml",
		".harness/template.yaml",
		".harness/template_members/cmd.yaml",
		".harness/template_members/docs.yaml",
		"AGENTS.md",
		"cmd/main.go",
		"docs/guide.md",
	}, files)

	_, err = gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".orbit/template.yaml")
	require.Error(t, err)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/template.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "kind: harness_template")
	require.Contains(t, string(manifestData), "default_template: false")
	require.Contains(t, string(manifestData), "root_guidance:")
	require.Contains(t, string(manifestData), "agents: true")

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(branchManifestData), "kind: harness_template")
	require.Contains(t, string(branchManifestData), "default_template: false")

	agentsData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", "AGENTS.md")
	require.NoError(t, err)
	require.Contains(t, string(agentsData), "$project_name")
	require.Contains(t, string(agentsData), "$command_name")

	inspectStdout, inspectStderr, err := executeOrbitCLI(t, repo.Root, "branch", "inspect", "harness-template/workspace", "--json")
	require.NoError(t, err)
	require.Empty(t, inspectStderr)

	var inspectPayload struct {
		Inspection struct {
			Classification struct {
				Kind         string `json:"kind"`
				TemplateKind string `json:"template_kind"`
			} `json:"classification"`
			HarnessID       string `json:"harness_id"`
			MemberCount     int    `json:"member_count"`
			DefinitionCount int    `json:"definition_count"`
			RootGuidance    struct {
				Agents    bool `json:"agents"`
				Humans    bool `json:"humans"`
				Bootstrap bool `json:"bootstrap"`
			} `json:"root_guidance"`
		} `json:"inspection"`
	}
	require.NoError(t, json.Unmarshal([]byte(inspectStdout), &inspectPayload))
	require.Equal(t, "template", inspectPayload.Inspection.Classification.Kind)
	require.Equal(t, "harness", inspectPayload.Inspection.Classification.TemplateKind)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), inspectPayload.Inspection.HarnessID)
	require.Equal(t, 2, inspectPayload.Inspection.MemberCount)
	require.Equal(t, 2, inspectPayload.Inspection.DefinitionCount)
	require.True(t, inspectPayload.Inspection.RootGuidance.Agents)
}

func TestHarnessTemplateSaveIgnoresLegacyRuntimeFileWhenManifestIsValid(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/runtime.yaml", "schema_version: nope\n")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun       bool   `json:"dry_run"`
		HarnessID    string `json:"harness_id"`
		TargetBranch string `json:"target_branch"`
		MemberCount  int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.Equal(t, 2, payload.MemberCount)
}

func TestHarnessTemplateSaveMarksTemplateAsDefaultWhenRequested(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--default", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		TargetBranch    string `json:"target_branch"`
		DefaultTemplate bool   `json:"default_template"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.True(t, payload.DefaultTemplate)

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/template.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "default_template: true")

	branchManifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/manifest.yaml")
	require.NoError(t, err)
	require.Contains(t, string(branchManifestData), "default_template: true")
}

func TestHarnessTemplateSaveDryRunDoesNotWriteBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	currentBranch := strings.TrimSpace(repo.Run(t, "rev-parse", "--abbrev-ref", "HEAD"))
	currentCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "harness template save dry-run -> harness-template/workspace")
	require.Contains(t, stdout, "files:")
	require.Contains(t, stdout, ".harness/template.yaml")
	require.Contains(t, stdout, ".harness/orbits/docs.yaml")
	require.Contains(t, stdout, "AGENTS.md")
	require.Contains(t, stdout, "replacements:")
	require.Contains(t, stdout, "AGENTS.md: project_name <- Orbit (1)")
	require.Contains(t, stdout, "AGENTS.md: command_name <- orbitctl (1)")
	require.Contains(t, stdout, "ambiguities: none")
	require.Contains(t, stdout, "manifest:")
	require.Contains(t, stdout, "harness_id: "+harnesspkg.DefaultHarnessIDForPath(repo.Root))
	require.Contains(t, stdout, "default_template: false")
	require.Contains(t, stdout, "created_from_branch: "+currentBranch)
	require.Contains(t, stdout, "created_from_commit: "+currentCommit)
	require.Contains(t, stdout, "root_guidance:")
	require.Contains(t, stdout, "agents: true")
	require.Contains(t, stdout, "members:")
	require.Contains(t, stdout, "cmd")
	require.Contains(t, stdout, "docs")
	require.Contains(t, stdout, "variables:")
	require.Contains(t, stdout, "command_name [required] CLI binary")
	require.Contains(t, stdout, "project_name [required] Product title")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestHarnessTemplateSaveDryRunJSONContract(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run", "--default", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun          bool     `json:"dry_run"`
		HarnessRoot     string   `json:"harness_root"`
		HarnessID       string   `json:"harness_id"`
		TargetBranch    string   `json:"target_branch"`
		Files           []string `json:"files"`
		MemberCount     int      `json:"member_count"`
		DefaultTemplate bool     `json:"default_template"`
		RootGuidance    struct {
			Agents    bool `json:"agents"`
			Humans    bool `json:"humans"`
			Bootstrap bool `json:"bootstrap"`
		} `json:"root_guidance"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.Equal(t, 2, payload.MemberCount)
	require.True(t, payload.DefaultTemplate)
	require.True(t, payload.RootGuidance.Agents)
	require.Contains(t, payload.Files, ".harness/template.yaml")
	require.Contains(t, payload.Files, ".harness/orbits/docs.yaml")
	require.Contains(t, payload.Files, ".harness/template_members/docs.yaml")
	require.Contains(t, payload.Files, "AGENTS.md")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestHarnessTemplateSaveTextOutputContract(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "saved harness template "+harnesspkg.DefaultHarnessIDForPath(repo.Root)+" to branch harness-template/workspace\n")
	require.Contains(t, stdout, "commit: ")
	require.Contains(t, stdout, "files: 8\n")
	require.Contains(t, stdout, "member_count: 2\n")
	require.Contains(t, stdout, "default_template: false\n")
	require.Contains(t, stdout, "root_guidance:\n")
	require.Contains(t, stdout, "agents: true\n")
}

func TestHarnessTemplateSaveRequiresOverwriteForExistingBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)

	_, _, err = executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.Error(t, err)
	require.ErrorContains(t, err, "already exists; re-run with --overwrite to replace it")
}

func TestHarnessTemplateSaveJSONFailureRequiresOverwriteForExistingBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	firstCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "harness-template/workspace"))

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun            bool   `json:"dry_run"`
		Saved             bool   `json:"saved"`
		Stage             string `json:"stage"`
		Reason            string `json:"reason"`
		HarnessRoot       string `json:"harness_root"`
		HarnessID         string `json:"harness_id"`
		TargetBranch      string `json:"target_branch"`
		OverwriteRequired bool   `json:"overwrite_required"`
		Message           string `json:"message"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.False(t, payload.Saved)
	require.Equal(t, "write", payload.Stage)
	require.Equal(t, "target_branch_exists", payload.Reason)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.True(t, payload.OverwriteRequired)
	require.Contains(t, payload.Message, "already exists; re-run with --overwrite to replace it")
	require.Equal(t, firstCommit, strings.TrimSpace(repo.Run(t, "rev-parse", "harness-template/workspace")))
}

func TestHarnessTemplateSaveOverwriteRewritesExistingBranch(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.NoError(t, err)
	firstCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "harness-template/workspace"))

	repo.WriteFile(t, "docs/guide.md", "Orbit guide v2 for $project_name\n")
	repo.AddAndCommit(t, "update runtime docs")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--overwrite", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Commit       string `json:"commit"`
		TargetBranch string `json:"target_branch"`
		DryRun       bool   `json:"dry_run"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.False(t, payload.DryRun)
	require.NotEqual(t, firstCommit, payload.Commit)

	savedData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", "docs/guide.md")
	require.NoError(t, err)
	require.Contains(t, string(savedData), "$project_name")
	require.Contains(t, string(savedData), "v2")
}

func TestHarnessTemplateSaveEditTemplateWritesEditedTemplateWithoutMutatingRuntimeWorktree(t *testing.T) {
	repo := seedHarnessTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n"+
		"  service_url:\n"+
		"    value: http://localhost:3000\n"+
		"    description: Service URL\n")
	repo.AddAndCommit(t, "add service url binding")

	editorScript := filepath.Join(repo.Root, "edit-template.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"printf '%s\\n' '$project_name guide at $service_url' > \"$1/docs/guide.md\"\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--edit-template")
	require.NoError(t, err)

	runtimeData, err := os.ReadFile(filepath.Join(repo.Root, "docs", "guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Orbit guide\n", string(runtimeData))

	templateData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", "docs/guide.md")
	require.NoError(t, err)
	require.Equal(t, "$project_name guide at $service_url\n", string(templateData))

	manifestData, err := gitpkg.ReadFileAtRev(context.Background(), repo.Root, "harness-template/workspace", ".harness/template.yaml")
	require.NoError(t, err)
	require.Contains(t, string(manifestData), "service_url")
}

func TestHarnessTemplateSaveEditTemplateFailsWhenDefinitionIsRemoved(t *testing.T) {
	repo := seedHarnessTemplateSaveRepo(t)

	editorScript := filepath.Join(repo.Root, "edit-template.sh")
	require.NoError(t, os.WriteFile(editorScript, []byte(""+
		"#!/bin/sh\n"+
		"rm -f \"$1/.harness/orbits/docs.yaml\"\n"), 0o755))
	t.Setenv("EDITOR", editorScript)

	_, _, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--edit-template")
	require.Error(t, err)
	require.ErrorContains(t, err, "edited harness template must keep member definition")

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestHarnessTemplateSaveFailsClosedOnReplacementAmbiguity(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.AddAndCommit(t, "introduce template save ambiguity")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace")
	require.Error(t, err)
	require.Empty(t, stdout)
	require.Empty(t, stderr)
	require.ErrorContains(t, err, "replacement ambiguity detected")
	require.ErrorContains(t, err, `AGENTS.md [root_guidance]`)
	require.ErrorContains(t, err, `docs/guide.md [docs]`)

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestHarnessTemplateSaveJSONFailureIncludesAmbiguityContributors(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.AddAndCommit(t, "introduce template save ambiguity")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun          bool   `json:"dry_run"`
		Saved           bool   `json:"saved"`
		HarnessRoot     string `json:"harness_root"`
		HarnessID       string `json:"harness_id"`
		TargetBranch    string `json:"target_branch"`
		DefaultTemplate bool   `json:"default_template"`
		RootGuidance    struct {
			Agents    bool `json:"agents"`
			Humans    bool `json:"humans"`
			Bootstrap bool `json:"bootstrap"`
		} `json:"root_guidance"`
		Message     string `json:"message"`
		Ambiguities []struct {
			Path         string   `json:"path"`
			Contributors []string `json:"contributors"`
			Ambiguities  []struct {
				Literal   string   `json:"literal"`
				Variables []string `json:"variables"`
			} `json:"ambiguities"`
		} `json:"ambiguities"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.False(t, payload.DryRun)
	require.False(t, payload.Saved)
	require.Equal(t, repo.Root, payload.HarnessRoot)
	require.Equal(t, harnesspkg.DefaultHarnessIDForPath(repo.Root), payload.HarnessID)
	require.Equal(t, "harness-template/workspace", payload.TargetBranch)
	require.False(t, payload.DefaultTemplate)
	require.True(t, payload.RootGuidance.Agents)
	require.Contains(t, payload.Message, "replacement ambiguity detected")
	require.Contains(t, payload.Message, `AGENTS.md [root_guidance]`)
	require.Contains(t, payload.Message, `docs/guide.md [docs]`)
	require.Contains(t, payload.Ambiguities, struct {
		Path         string   `json:"path"`
		Contributors []string `json:"contributors"`
		Ambiguities  []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		} `json:"ambiguities"`
	}{
		Path:         "AGENTS.md",
		Contributors: []string{"root_guidance"},
		Ambiguities: []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		}{{
			Literal:   "Orbit",
			Variables: []string{"product_name", "project_name"},
		}},
	})

	exists, err := gitpkg.LocalBranchExists(context.Background(), repo.Root, "harness-template/workspace")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestHarnessTemplateSaveDryRunJSONIncludesAmbiguityContributors(t *testing.T) {
	t.Parallel()

	repo := seedHarnessTemplateSaveRepo(t)
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  product_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.AddAndCommit(t, "introduce template save ambiguity")

	stdout, stderr, err := executeHarnessCLI(t, repo.Root, "template", "save", "--to", "harness-template/workspace", "--dry-run", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		DryRun      bool `json:"dry_run"`
		Ambiguities []struct {
			Path         string   `json:"path"`
			Contributors []string `json:"contributors"`
			Ambiguities  []struct {
				Literal   string   `json:"literal"`
				Variables []string `json:"variables"`
			} `json:"ambiguities"`
		} `json:"ambiguities"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.True(t, payload.DryRun)
	require.Contains(t, payload.Ambiguities, struct {
		Path         string   `json:"path"`
		Contributors []string `json:"contributors"`
		Ambiguities  []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		} `json:"ambiguities"`
	}{
		Path:         "AGENTS.md",
		Contributors: []string{"root_guidance"},
		Ambiguities: []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		}{
			{Literal: "Orbit", Variables: []string{"product_name", "project_name"}},
		},
	})
	require.Contains(t, payload.Ambiguities, struct {
		Path         string   `json:"path"`
		Contributors []string `json:"contributors"`
		Ambiguities  []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		} `json:"ambiguities"`
	}{
		Path:         "docs/guide.md",
		Contributors: []string{"docs"},
		Ambiguities: []struct {
			Literal   string   `json:"literal"`
			Variables []string `json:"variables"`
		}{
			{Literal: "Orbit", Variables: []string{"product_name", "project_name"}},
		},
	})
}

func seedHarnessInstallRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.AddAndCommit(t, "seed runtime repo")

	_, _, err = executeOrbitCLI(t, repo.Root, "template", "save", "docs", "--to", "orbit-template/docs")
	require.NoError(t, err)

	repo.Run(t, "rm", "-f", ".harness/orbits/docs.yaml", ".harness/vars.yaml", "docs/guide.md")
	repo.AddAndCommit(t, "clear runtime branch")

	return repo
}

func createAlternateDocsTemplateBranch(t *testing.T, repo *testutil.Repo) string {
	t.Helper()

	runtimeBranch := strings.TrimSpace(repo.Run(t, "branch", "--show-current"))
	repo.Run(t, "checkout", "orbit-template/docs")
	repo.Run(t, "checkout", "-b", "orbit-template/docs-alt")
	repo.WriteFile(t, "docs/reference.md", "$project_name reference\n")
	repo.Run(t, "rm", "-f", "docs/guide.md")
	repo.AddAndCommit(t, "create alternate docs template branch")
	alternateCommit := strings.TrimSpace(repo.Run(t, "rev-parse", "HEAD"))
	repo.Run(t, "checkout", runtimeBranch)

	return alternateCommit
}

func seedHarnessAgentsComposeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Acme\n"+
		"  command_name:\n"+
		"    value: orbitctl\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the $project_name docs orbit.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
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
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"description: Cmd orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/cmd.yaml\n"+
		"  agents_template: |\n"+
		"    Use $command_name for $project_name releases.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: cmd-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - cmd/**\n"+
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
	repo.WriteFile(t, ".harness/orbits/ops.yaml", ""+
		"id: ops\n"+
		"description: Ops orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/ops.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"members:\n"+
		"  - key: ops-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - ops/**\n"+
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

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "ops")
	require.NoError(t, err)

	return repo
}

func seedHarnessBatchInstallRepo(t *testing.T, templates []bindingsPlanTemplateSpec) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	for _, template := range templates {
		orbitSpec := "" +
			"id: " + template.OrbitID + "\n" +
			"description: " + template.OrbitID + " orbit\n"
		if template.AgentsTemplate != "" {
			orbitSpec += "" +
				"meta:\n" +
				"  file: .harness/orbits/" + template.OrbitID + ".yaml\n" +
				"  agents_template: |\n"
			for _, line := range strings.Split(template.AgentsTemplate, "\n") {
				if line == "" {
					continue
				}
				orbitSpec += "    " + line + "\n"
			}
			orbitSpec += "" +
				"  include_in_projection: true\n" +
				"  include_in_write: true\n" +
				"  include_in_export: true\n" +
				"  include_description_in_orchestration: true\n" +
				"members:\n" +
				"  - key: " + template.OrbitID + "-content\n" +
				"    role: subject\n" +
				"    paths:\n" +
				"      include:\n" +
				"        - " + template.OrbitID + "/**\n" +
				"behavior:\n" +
				"  scope:\n" +
				"    projection_roles:\n" +
				"      - meta\n" +
				"      - subject\n" +
				"    write_roles:\n" +
				"      - meta\n" +
				"      - subject\n" +
				"    export_roles:\n" +
				"      - meta\n" +
				"      - subject\n" +
				"    orchestration_roles:\n" +
				"      - meta\n" +
				"      - subject\n" +
				"  orchestration:\n" +
				"    materialize_agents_from_meta: true\n"
		} else {
			orbitSpec += "" +
				"include:\n" +
				"  - " + template.OrbitID + "/**\n"
		}
		repo.WriteFile(t, filepath.Join(".harness", "orbits", template.OrbitID+".yaml"), orbitSpec)
		repo.WriteFile(t, ".harness/vars.yaml", template.VarsYAML)
		for path, content := range template.Files {
			repo.WriteFile(t, path, content)
		}
		repo.AddAndCommit(t, "seed "+template.OrbitID+" runtime content")

		_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
			Preview: orbittemplate.TemplateSavePreviewInput{
				RepoRoot:     repo.Root,
				OrbitID:      template.OrbitID,
				TargetBranch: "orbit-template/" + template.OrbitID,
				Now:          time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC),
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

func seedDetachedBindingsRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedEmptyHarnessRuntimeRepo(t)
	_, err := harnesspkg.WriteInstallRecord(repo.Root, orbittemplate.InstallRecord{
		SchemaVersion: 1,
		OrbitID:       "docs",
		Template: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
			SourceRepo:     "",
			SourceRef:      "orbit-template/docs",
			TemplateCommit: "abc123",
		},
		AppliedAt: time.Date(2026, time.April, 16, 20, 30, 0, 0, time.UTC),
		Status:    orbittemplate.InstallRecordStatusDetached,
	})
	require.NoError(t, err)

	return repo
}

func seedEmptyHarnessRuntimeRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)
	repo.WriteFile(t, "README.md", "runtime repo\n")
	repo.AddAndCommit(t, "seed runtime repo")

	return repo
}

func seedHarnessTemplateSaveRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"description: Cmd orbit\n"+
		"include:\n"+
		"  - cmd/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n\nconst name = \"orbitctl\"\n")
	repo.WriteFile(t, "AGENTS.md", ""+
		"Workspace guide for Orbit\n"+
		"<!-- keep -->\n"+
		"Use orbitctl consistently.\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed harness template save runtime")

	return repo
}

func seedHarnessTemplateSaveRepoWithoutAgents(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/cmd.yaml", ""+
		"id: cmd\n"+
		"description: Cmd orbit\n"+
		"include:\n"+
		"  - cmd/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n"+
		"  command_name:\n"+
		"    value: orbitctl\n"+
		"    description: CLI binary\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "cmd/main.go", "package main\n\nconst name = \"orbitctl\"\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "cmd")
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed harness template save runtime without root agents")

	return repo
}

func seedSingleMemberHarnessTemplateSaveRepo(t *testing.T, withAgents bool) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/vars.yaml", ""+
		"schema_version: 1\n"+
		"variables:\n"+
		"  project_name:\n"+
		"    value: Orbit\n"+
		"    description: Product title\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	if withAgents {
		repo.WriteFile(t, "AGENTS.md", ""+
			"Workspace guide for Orbit\n"+
			"<!-- keep -->\n"+
			"Use orbit guidance consistently.\n")
	}

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed single member harness template save runtime")

	return repo
}

type bindingsPlanTemplateSpec struct {
	OrbitID        string
	VarsYAML       string
	Files          map[string]string
	AgentsTemplate string
}

func seedHarnessBindingsPlanRepo(t *testing.T, templates []bindingsPlanTemplateSpec, currentVarsYAML string) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	for _, template := range templates {
		repo.WriteFile(t, filepath.Join(".harness", "orbits", template.OrbitID+".yaml"), ""+
			"id: "+template.OrbitID+"\n"+
			"description: "+template.OrbitID+" orbit\n"+
			"include:\n"+
			"  - "+template.OrbitID+"/**\n")
		repo.WriteFile(t, ".harness/vars.yaml", template.VarsYAML)
		for path, content := range template.Files {
			repo.WriteFile(t, path, content)
		}
		repo.AddAndCommit(t, "seed "+template.OrbitID+" runtime content")

		_, err = orbittemplate.SaveTemplateBranch(context.Background(), orbittemplate.TemplateSaveInput{
			Preview: orbittemplate.TemplateSavePreviewInput{
				RepoRoot:     repo.Root,
				OrbitID:      template.OrbitID,
				TargetBranch: "orbit-template/" + template.OrbitID,
				Now:          time.Date(2026, time.April, 9, 12, 0, 0, 0, time.UTC),
			},
		})
		require.NoError(t, err)
	}

	repo.WriteFile(t, ".harness/vars.yaml", currentVarsYAML)
	repo.AddAndCommit(t, "seed runtime shared vars")

	return repo
}

func seedHarnessFrameworkRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the docs orbit.\n"+
		"  humans_template: |\n"+
		"    Run the docs workflow.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/review.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/docs-style\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use docs style guide.\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness framework repo")

	return repo
}

func seedHarnessFrameworkConflictRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/ops.yaml", ""+
		"id: ops\n"+
		"description: Ops orbit\n"+
		"include:\n"+
		"  - ops/**\n")
	repo.WriteFile(t, "docs/guide.md", "Docs guide\n")
	repo.WriteFile(t, "ops/runbook.md", "Ops runbook\n")

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repo.Root)
	require.NoError(t, err)
	runtimeFile.Members = []harnesspkg.RuntimeMember{
		{
			OrbitID:        "docs",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: "docs_stack",
			AddedAt:        time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC),
		},
		{
			OrbitID:        "ops",
			Source:         harnesspkg.MemberSourceInstallBundle,
			OwnerHarnessID: "ops_stack",
			AddedAt:        time.Date(2026, time.April, 23, 12, 1, 0, 0, time.UTC),
		},
	}
	runtimeFile.Harness.UpdatedAt = time.Date(2026, time.April, 23, 12, 1, 0, 0, time.UTC)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "docs_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/docs-stack", TemplateCommit: "aaa111"},
		RecommendedFramework: "claude",
		MemberIDs:            []string{"docs"},
		AppliedAt:            time.Date(2026, time.April, 23, 12, 0, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"docs/guide.md"},
	})
	require.NoError(t, err)
	_, err = harnesspkg.WriteBundleRecord(repo.Root, harnesspkg.BundleRecord{
		SchemaVersion:        1,
		HarnessID:            "ops_stack",
		Template:             orbittemplate.Source{SourceKind: orbittemplate.InstallSourceKindLocalBranch, SourceRepo: "", SourceRef: "harness-template/ops-stack", TemplateCommit: "bbb222"},
		RecommendedFramework: "codex",
		MemberIDs:            []string{"ops"},
		AppliedAt:            time.Date(2026, time.April, 23, 12, 1, 0, 0, time.UTC),
		IncludesRootAgents:   false,
		OwnedPaths:           []string{"ops/runbook.md"},
	})
	require.NoError(t, err)

	repo.AddAndCommit(t, "seed harness framework conflict repo")

	return repo
}

func seedHarnessFrameworkRemoteSkillRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"description: Docs orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"  agents_template: |\n"+
		"    You are the docs orbit.\n"+
		"  humans_template: |\n"+
		"    Run the docs workflow.\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/review.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/docs-style\n"+
		"    remote:\n"+
		"      uris:\n"+
		"        - https://example.com/skills/docs-style\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "orbit/skills/docs-style/checklist.md", "Use docs style guide.\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness framework remote skill repo")

	return repo
}

func seedHarnessFrameworkRequiredRemoteSkillRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := seedHarnessFrameworkRemoteSkillRepo(t)
	definitionPath := filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")
	definitionData, err := os.ReadFile(definitionPath)
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", strings.ReplaceAll(
		string(definitionData),
		"      uris:\n        - https://example.com/skills/docs-style\n",
		"      dependencies:\n        - uri: https://example.com/skills/docs-style\n          required: true\n",
	))
	repo.AddAndCommit(t, "mark remote skill dependency required")

	return repo
}

func seedHarnessFrameworkCollisionRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
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
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/review.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/docs-style\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, ".harness/orbits/api.yaml", ""+
		"id: api\n"+
		"description: API orbit\n"+
		"meta:\n"+
		"  file: .harness/orbits/api.yaml\n"+
		"  include_in_projection: true\n"+
		"  include_in_write: true\n"+
		"  include_in_export: true\n"+
		"  include_description_in_orchestration: true\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - api/commands/review.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - api/skills/docs-style\n"+
		"members:\n"+
		"  - key: api-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - api/guide.md\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "api/guide.md", "API guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work.\n")
	repo.WriteFile(t, "api/commands/review.md", "Review API work.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: Docs style references.\n"+
		"---\n"+
		"# Docs Style\n")
	repo.WriteFile(t, "api/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"description: API style references.\n"+
		"---\n"+
		"# API Style\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	_, _, err = executeHarnessCLI(t, repo.Root, "add", "api")
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness framework collision repo")

	return repo
}

func seedHarnessFrameworkInvalidSkillRepo(t *testing.T) *testutil.Repo {
	t.Helper()

	repo := testutil.NewRepo(t)
	_, _, err := executeHarnessCLI(t, repo.Root, "init")
	require.NoError(t, err)

	repo.WriteFile(t, ".harness/frameworks.yaml", ""+
		"schema_version: 1\n"+
		"recommended_framework: claude\n")
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
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - orbit/commands/review.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - orbit/skills/docs-style\n"+
		"    remote:\n"+
		"      uris:\n"+
		"        - https://example.com/skills/docs-style\n"+
		"members:\n"+
		"  - key: docs-content\n"+
		"    role: subject\n"+
		"    scopes:\n"+
		"      export: true\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.WriteFile(t, "docs/guide.md", "Orbit guide\n")
	repo.WriteFile(t, "orbit/commands/review.md", "Review docs work.\n")
	repo.WriteFile(t, "orbit/skills/docs-style/SKILL.md", ""+
		"---\n"+
		"name: docs-style\n"+
		"---\n"+
		"# Docs Style\n")

	_, _, err = executeHarnessCLI(t, repo.Root, "add", "docs")
	require.NoError(t, err)
	repo.AddAndCommit(t, "seed harness framework invalid skill repo")

	return repo
}

type harnessCheckPayload struct {
	HarnessRoot  string `json:"harness_root"`
	HarnessID    string `json:"harness_id"`
	OK           bool   `json:"ok"`
	FindingCount int    `json:"finding_count"`
	Readiness    struct {
		Status         string `json:"status"`
		RuntimeReasons []struct {
			Code string `json:"code"`
		} `json:"runtime_reasons"`
	} `json:"readiness"`
	BindingsSummary *struct {
		UnresolvedInstallCount  int      `json:"unresolved_install_count"`
		UnresolvedVariableCount int      `json:"unresolved_variable_count"`
		OrbitIDs                []string `json:"orbit_ids"`
	} `json:"bindings_summary"`
	Findings []struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"findings"`
}

func requireVarsConflictMessageContains(
	t *testing.T,
	conflicts []orbittemplate.ApplyConflict,
	substring string,
) {
	t.Helper()

	for _, conflict := range conflicts {
		if conflict.Path == ".harness/vars.yaml" && strings.Contains(conflict.Message, substring) {
			return
		}
	}
	require.Failf(t, "missing apply conflict", "vars conflict did not contain substring %q in %#v", substring, conflicts)
}

func decodeHarnessCheckPayload(t *testing.T, stdout string) harnessCheckPayload {
	t.Helper()

	var payload harnessCheckPayload
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	return payload
}

func readinessCodesFromPayload(reasons []struct {
	Code string `json:"code"`
}) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, reason.Code)
	}
	return codes
}

func readinessCommandsFromPayload(steps []struct {
	Command string `json:"command"`
}) []string {
	commands := make([]string, 0, len(steps))
	for _, step := range steps {
		commands = append(commands, step.Command)
	}
	return commands
}

type actionBuffer struct {
	buffer    bytes.Buffer
	marker    string
	action    func()
	triggered bool
}

func (buffer *actionBuffer) Write(data []byte) (int, error) {
	written, err := buffer.buffer.Write(data)
	if err != nil {
		return written, err
	}

	if !buffer.triggered && strings.Contains(buffer.buffer.String(), buffer.marker) {
		buffer.triggered = true
		if buffer.action != nil {
			buffer.action()
		}
	}

	return written, nil
}

func (buffer *actionBuffer) String() string {
	return buffer.buffer.String()
}

func replaceCLIInstallPathWithDirectory(t *testing.T, absolutePath string) {
	t.Helper()

	require.NoError(t, os.RemoveAll(absolutePath))
	require.NoError(t, os.MkdirAll(absolutePath, 0o755))
}

func requireHarnessCheckFinding(t *testing.T, payload harnessCheckPayload, kind string) struct {
	Kind    string `json:"kind"`
	OrbitID string `json:"orbit_id"`
	Path    string `json:"path"`
	Message string `json:"message"`
} {
	t.Helper()

	for _, finding := range payload.Findings {
		if finding.Kind == kind {
			return finding
		}
	}

	t.Fatalf("finding %q not found in %+v", kind, payload.Findings)
	return struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{}
}

func requireHarnessCheckFindingForOrbit(t *testing.T, payload harnessCheckPayload, kind string, orbitID string) struct {
	Kind    string `json:"kind"`
	OrbitID string `json:"orbit_id"`
	Path    string `json:"path"`
	Message string `json:"message"`
} {
	t.Helper()

	for _, finding := range payload.Findings {
		if finding.Kind == kind && finding.OrbitID == orbitID {
			return finding
		}
	}

	t.Fatalf("finding %q for orbit %q not found in %+v", kind, orbitID, payload.Findings)
	return struct {
		Kind    string `json:"kind"`
		OrbitID string `json:"orbit_id"`
		Path    string `json:"path"`
		Message string `json:"message"`
	}{}
}

func splitLines(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return strings.Split(strings.TrimSpace(value), "\n")
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()

	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "git %s failed:\n%s", strings.Join(args, " "), string(output))
}
