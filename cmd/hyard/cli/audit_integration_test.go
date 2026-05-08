package cli_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	hyardcli "github.com/zack-nova/harnessyard/cmd/hyard/cli"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestHyardAuditOrdinaryRepoReportsNotHyardRevisionText(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "README.md", "ordinary repo\n")
	repo.AddAndCommit(t, "seed ordinary repo")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit")
	require.Error(t, err)
	require.Empty(t, stderr)

	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)
	require.Contains(t, stdout, "status: not_hyard_revision\n")
	require.Contains(t, stdout, "revision_kind: none\n")
	require.Contains(t, stdout, "packages: none\n")
	require.Contains(t, stdout, "findings: none\n")
}

func TestHyardAuditOrdinaryRepoReportsStableJSONShape(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, "package.json", "{}\n")
	repo.AddAndCommit(t, "seed ordinary repo")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Packages     []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"packages"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "not_hyard_revision", payload.Status)
	require.Equal(t, "none", payload.RevisionKind)
	require.NotNil(t, payload.Packages)
	require.Empty(t, payload.Packages)
	require.NotNil(t, payload.Findings)
	require.Empty(t, payload.Findings)
	require.NoDirExists(t, filepath.Join(repo.Root, ".harness"))
}

func TestHyardAuditRuntimeRevisionReportsPassJSON(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	runtimeFile, err := harnesspkg.DefaultRuntimeFile(repo.Root, now)
	require.NoError(t, err)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Packages     []struct {
			Type         string `json:"type"`
			Name         string `json:"name"`
			RevisionRole string `json:"revision_role"`
			OrbitID      string `json:"orbit_id,omitempty"`
			HarnessID    string `json:"harness_id,omitempty"`
			Source       string `json:"source,omitempty"`
		} `json:"packages"`
		Findings []struct {
			Kind string `json:"kind"`
		} `json:"findings"`
		Runtime *struct {
			Check struct {
				OK           bool `json:"ok"`
				FindingCount int  `json:"finding_count"`
			} `json:"check"`
			Readiness struct {
				Status        string `json:"status"`
				RuntimeStatus string `json:"runtime_status"`
				AgentStatus   string `json:"agent_status"`
				Summary       struct {
					OrbitCount          int `json:"orbit_count"`
					BlockingReasonCount int `json:"blocking_reason_count"`
					AdvisoryReasonCount int `json:"advisory_reason_count"`
				} `json:"summary"`
			} `json:"readiness"`
		} `json:"runtime,omitempty"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "pass", payload.Status)
	require.Equal(t, "runtime", payload.RevisionKind)
	require.Equal(t, []struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		RevisionRole string `json:"revision_role"`
		OrbitID      string `json:"orbit_id,omitempty"`
		HarnessID    string `json:"harness_id,omitempty"`
		Source       string `json:"source,omitempty"`
	}{
		{Type: "harness", Name: runtimeFile.Harness.ID, RevisionRole: "runtime", HarnessID: runtimeFile.Harness.ID},
	}, payload.Packages)
	require.NotNil(t, payload.Findings)
	require.Empty(t, payload.Findings)
	require.NotNil(t, payload.Runtime)
	require.True(t, payload.Runtime.Check.OK)
	require.Zero(t, payload.Runtime.Check.FindingCount)
	require.Equal(t, "ready", payload.Runtime.Readiness.Status)
	require.Equal(t, "ready", payload.Runtime.Readiness.RuntimeStatus)
	require.Equal(t, "ready", payload.Runtime.Readiness.AgentStatus)
	require.Zero(t, payload.Runtime.Readiness.Summary.OrbitCount)
	require.Zero(t, payload.Runtime.Readiness.Summary.BlockingReasonCount)
	require.Zero(t, payload.Runtime.Readiness.Summary.AdvisoryReasonCount)
}

func TestHyardAuditRuntimeRevisionMapsBlockingCheckFindingsToFailJSON(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 30, 0, 0, time.UTC)
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindRuntime,
		Runtime: &harnesspkg.ManifestRuntimeMetadata{
			Package:   ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: "workspace"},
			ID:        "workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.ManifestMember{
			{
				Package: ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"},
				OrbitID: "docs",
				Source:  harnesspkg.ManifestMemberSourceManual,
				AddedAt: now,
			},
		},
	})
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Runtime      *struct {
			Check struct {
				OK           bool `json:"ok"`
				FindingCount int  `json:"finding_count"`
			} `json:"check"`
			Readiness struct {
				Status  string `json:"status"`
				Summary struct {
					BlockingReasonCount int `json:"blocking_reason_count"`
				} `json:"summary"`
			} `json:"readiness"`
		} `json:"runtime,omitempty"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
			Path     string `json:"path,omitempty"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	require.Equal(t, "runtime", payload.RevisionKind)
	require.NotNil(t, payload.Runtime)
	require.False(t, payload.Runtime.Check.OK)
	require.Equal(t, 1, payload.Runtime.Check.FindingCount)
	require.Equal(t, "broken", payload.Runtime.Readiness.Status)
	require.Equal(t, 1, payload.Runtime.Readiness.Summary.BlockingReasonCount)
	require.Contains(t, payload.Findings, struct {
		Severity string `json:"severity"`
		Kind     string `json:"kind"`
		Package  string `json:"package,omitempty"`
		Path     string `json:"path,omitempty"`
	}{
		Severity: "fail",
		Kind:     "runtime_check_missing_definition",
		Package:  "docs",
		Path:     ".harness/orbits/docs.yaml",
	})
}

func TestHyardAuditRuntimeRevisionMapsAdvisoryReadinessToWarnJSON(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 11, 0, 0, 0, time.UTC)
	_, err := harnesspkg.WriteRuntimeFile(repo.Root, harnesspkg.RuntimeFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.RuntimeKind,
		Harness: harnesspkg.RuntimeMetadata{
			ID:        "workspace",
			Name:      "Workspace",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []harnesspkg.RuntimeMember{
			{OrbitID: "docs", Source: harnesspkg.MemberSourceManual, AddedAt: now},
		},
	})
	require.NoError(t, err)
	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec("docs")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedOrbitSpec(repo.Root, spec)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Runtime      *struct {
			Check struct {
				OK           bool `json:"ok"`
				FindingCount int  `json:"finding_count"`
			} `json:"check"`
			Readiness struct {
				Status  string `json:"status"`
				Summary struct {
					AdvisoryReasonCount int `json:"advisory_reason_count"`
				} `json:"summary"`
			} `json:"readiness"`
		} `json:"runtime,omitempty"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "warn", payload.Status)
	require.Equal(t, "runtime", payload.RevisionKind)
	require.NotNil(t, payload.Runtime)
	require.True(t, payload.Runtime.Check.OK)
	require.Zero(t, payload.Runtime.Check.FindingCount)
	require.Equal(t, "usable", payload.Runtime.Readiness.Status)
	require.Equal(t, 1, payload.Runtime.Readiness.Summary.AdvisoryReasonCount)
	require.Contains(t, payload.Findings, struct {
		Severity string `json:"severity"`
		Kind     string `json:"kind"`
		Package  string `json:"package,omitempty"`
	}{
		Severity: "warn",
		Kind:     "runtime_readiness_agents_not_composed",
		Package:  "docs",
	})
}

func TestHyardAuditRuntimeRevisionReportsTextSummary(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	runtimeFile, err := harnesspkg.DefaultRuntimeFile(repo.Root, now)
	require.NoError(t, err)
	_, err = harnesspkg.WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "status: pass\n")
	require.Contains(t, stdout, "revision_kind: runtime\n")
	require.Contains(t, stdout, "packages:\n")
	require.Contains(t, stdout, "type=harness name="+runtimeFile.Harness.ID+" revision_role=runtime harness_id="+runtimeFile.Harness.ID+"\n")
	require.Contains(t, stdout, "findings: none\n")
}

func TestHyardAuditInvalidManifestReportsFailJSON(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", "schema_version: 1\nkind: broken\n")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Packages     []struct {
			Name string `json:"name"`
		} `json:"packages"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	require.Equal(t, "none", payload.RevisionKind)
	require.NotNil(t, payload.Packages)
	require.Empty(t, payload.Packages)
	require.Len(t, payload.Findings, 1)
	require.Equal(t, "fail", payload.Findings[0].Severity)
	require.Equal(t, "manifest_schema_invalid", payload.Findings[0].Kind)
	require.Equal(t, ".harness/manifest.yaml", payload.Findings[0].Path)
	require.NotContains(t, payload.Findings[0].Message, repo.Root)
	require.Contains(t, payload.Findings[0].Message, "kind must be one of")
}
