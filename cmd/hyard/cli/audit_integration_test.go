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
		{Type: "harness", Name: "workspace", RevisionRole: "runtime", HarnessID: "workspace"},
		{Type: "orbit", Name: "docs", RevisionRole: "member", OrbitID: "docs", Source: "manual"},
	}, payload.Packages)
	require.NotNil(t, payload.Findings)
	require.Empty(t, payload.Findings)
}

func TestHyardAuditRuntimeRevisionReportsTextSummary(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
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

	stdout, stderr, err := executeHyardCLI(t, repo.Root, "audit")
	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "status: pass\n")
	require.Contains(t, stdout, "revision_kind: runtime\n")
	require.Contains(t, stdout, "packages:\n")
	require.Contains(t, stdout, "type=harness name=workspace revision_role=runtime harness_id=workspace\n")
	require.Contains(t, stdout, "type=orbit name=docs revision_role=member orbit_id=docs source=manual\n")
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
