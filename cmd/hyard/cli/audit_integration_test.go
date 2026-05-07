package cli_test

import (
	"encoding/json"
	"os"
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

func TestHyardAuditSourceRevisionRejectsInvalidHostedOrbitSpec(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindSource,
		Source: &harnesspkg.ManifestSourceMetadata{
			Package:      ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"},
			OrbitID:      "docs",
			SourceBranch: "main",
		},
	})
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"meta:\n"+
		"  file: .orbit/orbits/docs.yaml\n"+
		"content:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n")
	repo.AddAndCommit(t, "seed invalid source audit fixture")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	exitCode, ok := hyardcli.ErrorExitCode(err)
	require.True(t, ok)
	require.Equal(t, 1, exitCode)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Findings     []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	require.Equal(t, "source", payload.RevisionKind)
	require.Len(t, payload.Findings, 1)
	require.Equal(t, "fail", payload.Findings[0].Severity)
	require.Equal(t, "orbit_spec_schema_invalid", payload.Findings[0].Kind)
	require.Equal(t, "docs", payload.Findings[0].Package)
	require.Equal(t, ".harness/orbits/docs.yaml", payload.Findings[0].Path)
	require.NotContains(t, payload.Findings[0].Message, repo.Root)
	require.Contains(t, payload.Findings[0].Message, "meta.file")
	expectedMetaFile, err := orbitpkg.HostedDefinitionRelativePath("docs")
	require.NoError(t, err)
	require.Contains(t, payload.Findings[0].Message, expectedMetaFile)
}

func TestHyardAuditSourceRevisionReportsMissingPackagePathsAsWarnings(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindSource,
		Source: &harnesspkg.ManifestSourceMetadata{
			Package:      ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"},
			OrbitID:      "docs",
			SourceBranch: "main",
		},
	})
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"capabilities:\n"+
		"  commands:\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - commands/docs.md\n"+
		"  skills:\n"+
		"    local:\n"+
		"      paths:\n"+
		"        include:\n"+
		"          - skills/docs\n"+
		"content:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/missing.md\n")
	repo.AddAndCommit(t, "seed source audit warning fixture")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Findings     []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "warn", payload.Status)
	require.Equal(t, "source", payload.RevisionKind)
	require.Len(t, payload.Findings, 3)
	requireAuditFinding(t, payload.Findings, "warn", "command_capability_path_unmatched", "docs", "commands/docs.md")
	requireAuditFinding(t, payload.Findings, "warn", "local_skill_capability_path_unmatched", "docs", "skills/docs")
	requireAuditFinding(t, payload.Findings, "warn", "content_member_pattern_unmatched", "docs", "docs/missing.md")
}

func TestHyardAuditSourceRevisionRejectsPackageIdentityMismatch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindSource,
		Source: &harnesspkg.ManifestSourceMetadata{
			Package:      ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"},
			OrbitID:      "docs",
			SourceBranch: "main",
		},
	})
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/api.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: api\n"+
		"meta:\n"+
		"  file: .harness/orbits/api.yaml\n")
	repo.AddAndCommit(t, "seed mismatched source audit fixture")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status   string `json:"status"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	requireAuditFinding(t, payload.Findings, "fail", "package_identity_mismatch", "api", ".harness/orbits/api.yaml")
	requireAuditFinding(t, payload.Findings, "fail", "source_package_orbit_spec_missing", "docs", ".harness/orbits/docs.yaml")
}

func TestHyardAuditSourceRevisionBlocksUntrackedControlPlaneMemberPattern(t *testing.T) {
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
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"content:\n"+
		"  - name: manifest\n"+
		"    role: meta\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - .harness/manifest.yaml\n")
	repo.AddAndCommit(t, "track hosted spec only", ".harness/orbits/docs.yaml")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status   string `json:"status"`
		Findings []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Package  string `json:"package,omitempty"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	requireAuditFinding(t, payload.Findings, "fail", "manifest_untracked", "", ".harness/manifest.yaml")
	requireAuditFinding(t, payload.Findings, "fail", "content_member_pattern_unmatched", "docs", ".harness/manifest.yaml")
}

func TestHyardAuditSourceRevisionDoesNotRewriteLegacyRules(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := harnesspkg.WriteManifestFile(repo.Root, harnesspkg.ManifestFile{
		SchemaVersion: 1,
		Kind:          harnesspkg.ManifestKindSource,
		Source: &harnesspkg.ManifestSourceMetadata{
			Package:      ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: "docs"},
			OrbitID:      "docs",
			SourceBranch: "main",
		},
	})
	require.NoError(t, err)
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"package:\n"+
		"  type: orbit\n"+
		"  name: docs\n"+
		"meta:\n"+
		"  file: .harness/orbits/docs.yaml\n"+
		"content:\n"+
		"  - name: docs-content\n"+
		"    role: subject\n"+
		"    paths:\n"+
		"      include:\n"+
		"        - docs/**\n"+
		"rules:\n"+
		"  scope:\n"+
		"    write_roles: [meta, rule, subject]\n")
	repo.WriteFile(t, "docs/guide.md", "# Guide\n")
	repo.AddAndCommit(t, "seed legacy rules source fixture")

	specPath := filepath.Join(repo.Root, ".harness", "orbits", "docs.yaml")
	before, err := os.ReadFile(specPath)
	require.NoError(t, err)

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.NoError(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status   string `json:"status"`
		Findings []struct {
			Kind string `json:"kind"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "pass", payload.Status)
	require.Empty(t, payload.Findings)

	after, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	require.Contains(t, string(after), "rules:\n")
	require.NotContains(t, string(after), "behavior:\n")
}

func TestHyardAuditGPDWLikeLegacyHarnessFixtureFailsWithStableFindings(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	repo.WriteFile(t, ".harness/manifest.yaml", ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source_branch: main\n"+
		"publish:\n"+
		"  package:\n"+
		"    type: orbit\n"+
		"    name: docs\n")
	repo.WriteFile(t, ".harness/orbits/docs.yaml", ""+
		"id: docs\n"+
		"include:\n"+
		"  - docs/**\n")
	repo.AddAndCommit(t, "seed legacy harness fixture")

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit", "--json")
	require.Error(t, err)
	require.Empty(t, stderr)

	var payload struct {
		Status       string `json:"status"`
		RevisionKind string `json:"revision_kind"`
		Findings     []struct {
			Severity string `json:"severity"`
			Kind     string `json:"kind"`
			Path     string `json:"path"`
			Message  string `json:"message"`
		} `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	require.Equal(t, "fail", payload.Status)
	require.Equal(t, "none", payload.RevisionKind)
	require.Len(t, payload.Findings, 1)
	require.Equal(t, "fail", payload.Findings[0].Severity)
	require.Equal(t, "manifest_schema_invalid", payload.Findings[0].Kind)
	require.Equal(t, ".harness/manifest.yaml", payload.Findings[0].Path)
	require.NotContains(t, payload.Findings[0].Message, repo.Root)
	require.Contains(t, payload.Findings[0].Message, "source_branch")
}

func requireAuditFinding(t *testing.T, findings []struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Package  string `json:"package,omitempty"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}, severity string, kind string, packageName string, path string) {
	t.Helper()

	for _, finding := range findings {
		if finding.Severity == severity && finding.Kind == kind && finding.Package == packageName && finding.Path == path {
			require.Contains(t, finding.Message, path)
			return
		}
	}
	require.Failf(t, "missing audit finding", "severity=%s kind=%s package=%s path=%s findings=%+v", severity, kind, packageName, path, findings)
}
