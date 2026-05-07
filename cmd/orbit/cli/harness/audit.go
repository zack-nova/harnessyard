package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	AuditStatusPass             = "pass"
	AuditStatusWarn             = "warn"
	AuditStatusFail             = "fail"
	AuditStatusNotHyardRevision = "not_hyard_revision"

	AuditRevisionKindNone = "none"
)

// AuditResult is the stable read-only audit contract for the current Git worktree.
type AuditResult struct {
	RepoRoot     string                `json:"repo_root,omitempty"`
	Status       string                `json:"status"`
	RevisionKind string                `json:"revision_kind"`
	Packages     []AuditPackageSummary `json:"packages"`
	Findings     []AuditFinding        `json:"findings"`
}

// AuditPackageSummary is a compact package identity row emitted by audit.
type AuditPackageSummary struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	Version      string `json:"version,omitempty"`
	RevisionRole string `json:"revision_role"`
	OrbitID      string `json:"orbit_id,omitempty"`
	HarnessID    string `json:"harness_id,omitempty"`
	Source       string `json:"source,omitempty"`
}

// AuditFinding is one flat audit diagnostic.
type AuditFinding struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Package  string `json:"package,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
}

// AuditRevision inspects the current Git worktree through the Harness Yard revision identity contract.
func AuditRevision(ctx context.Context, workingDir string) (AuditResult, error) {
	repo, err := gitpkg.DiscoverRepo(ctx, workingDir)
	if err != nil {
		return AuditResult{}, fmt.Errorf("discover git repository: %w", err)
	}

	manifest, err := LoadManifestFile(repo.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AuditResult{
				RepoRoot:     repo.Root,
				Status:       AuditStatusNotHyardRevision,
				RevisionKind: AuditRevisionKindNone,
				Packages:     []AuditPackageSummary{},
				Findings:     []AuditFinding{},
			}, nil
		}
		findings := []AuditFinding{
			{
				Severity: AuditStatusFail,
				Kind:     "manifest_schema_invalid",
				Path:     ManifestRepoPath(),
				Message:  stableAuditManifestError(err, repo.Root),
			},
		}
		return AuditResult{
			RepoRoot:     repo.Root,
			Status:       DeriveAuditStatus(findings),
			RevisionKind: AuditRevisionKindNone,
			Packages:     []AuditPackageSummary{},
			Findings:     findings,
		}, nil
	}

	return AuditResult{
		RepoRoot:     repo.Root,
		Status:       DeriveAuditStatus([]AuditFinding{}),
		RevisionKind: manifest.Kind,
		Packages:     auditPackageSummaries(manifest),
		Findings:     []AuditFinding{},
	}, nil
}

// DeriveAuditStatus reduces flat audit findings to the command status contract.
func DeriveAuditStatus(findings []AuditFinding) string {
	status := AuditStatusPass
	for _, finding := range findings {
		switch finding.Severity {
		case AuditStatusFail:
			return AuditStatusFail
		case AuditStatusWarn:
			if status == AuditStatusPass {
				status = AuditStatusWarn
			}
		case "":
			continue
		default:
			return AuditStatusFail
		}
	}

	return status
}

func auditPackageSummaries(manifest ManifestFile) []AuditPackageSummary {
	switch manifest.Kind {
	case ManifestKindSource:
		if manifest.Source == nil {
			return []AuditPackageSummary{}
		}
		return []AuditPackageSummary{
			auditPackageSummary(manifest.Source.Package, "source", manifest.Source.OrbitID, "", ""),
		}
	case ManifestKindRuntime:
		summaries := make([]AuditPackageSummary, 0, 1+len(manifest.Members))
		if manifest.Runtime != nil {
			summaries = append(
				summaries,
				auditPackageSummary(manifest.Runtime.Package, "runtime", "", manifest.Runtime.ID, ""),
			)
		}
		for _, member := range manifest.Members {
			summaries = append(
				summaries,
				auditPackageSummary(member.Package, "member", member.OrbitID, member.OwnerHarnessID, member.Source),
			)
		}
		return summaries
	case ManifestKindOrbitTemplate:
		if manifest.Template == nil {
			return []AuditPackageSummary{}
		}
		return []AuditPackageSummary{
			auditPackageSummary(manifest.Template.Package, "template", manifest.Template.OrbitID, "", ""),
		}
	case ManifestKindHarnessTemplate:
		summaries := make([]AuditPackageSummary, 0, 1+len(manifest.Members))
		if manifest.Template != nil {
			summaries = append(
				summaries,
				auditPackageSummary(manifest.Template.Package, "template", "", manifest.Template.HarnessID, ""),
			)
		}
		for _, member := range manifest.Members {
			summaries = append(
				summaries,
				auditPackageSummary(member.Package, "member", member.OrbitID, member.OwnerHarnessID, member.Source),
			)
		}
		return summaries
	default:
		return []AuditPackageSummary{}
	}
}

func stableAuditManifestError(err error, repoRoot string) string {
	return strings.ReplaceAll(err.Error(), ManifestPath(repoRoot), ManifestRepoPath())
}

func auditPackageSummary(identity ids.PackageIdentity, revisionRole string, orbitID string, harnessID string, source string) AuditPackageSummary {
	return AuditPackageSummary{
		Type:         identity.Type,
		Name:         identity.Name,
		Version:      identity.Version,
		RevisionRole: revisionRole,
		OrbitID:      orbitID,
		HarnessID:    harnessID,
		Source:       source,
	}
}
