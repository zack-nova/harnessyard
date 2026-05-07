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
	Runtime      *AuditRuntimeSummary  `json:"runtime,omitempty"`
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

// AuditRuntimeSummary is the audit-level summary of existing runtime diagnostics.
type AuditRuntimeSummary struct {
	Check     AuditRuntimeCheckSummary     `json:"check"`
	Readiness AuditRuntimeReadinessSummary `json:"readiness"`
}

// AuditRuntimeCheckSummary summarizes harness runtime check output without replacing hyard check.
type AuditRuntimeCheckSummary struct {
	HarnessID       string                `json:"harness_id,omitempty"`
	OK              bool                  `json:"ok"`
	FindingCount    int                   `json:"finding_count"`
	BindingsSummary *CheckBindingsSummary `json:"bindings_summary,omitempty"`
}

// AuditRuntimeReadinessSummary summarizes derived runtime readiness without replacing hyard ready.
type AuditRuntimeReadinessSummary struct {
	Status                ReadinessStatus  `json:"status"`
	RuntimeStatus         ReadinessStatus  `json:"runtime_status"`
	AgentStatus           ReadinessStatus  `json:"agent_status"`
	AgentActivationStatus string           `json:"agent_activation_status,omitempty"`
	Summary               ReadinessSummary `json:"summary"`
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

	result := AuditResult{
		RepoRoot:     repo.Root,
		Status:       DeriveAuditStatus([]AuditFinding{}),
		RevisionKind: manifest.Kind,
		Packages:     auditPackageSummaries(manifest),
		Findings:     []AuditFinding{},
	}
	if manifest.Kind == ManifestKindRuntime {
		runtimeSummary, runtimeFindings, err := auditRuntimeRevision(ctx, repo.Root)
		if err != nil {
			return AuditResult{}, err
		}
		result.Runtime = &runtimeSummary
		result.Findings = runtimeFindings
		result.Status = DeriveAuditStatus(result.Findings)
	}

	return result, nil
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

func auditRuntimeRevision(ctx context.Context, repoRoot string) (AuditRuntimeSummary, []AuditFinding, error) {
	checkResult, err := CheckRuntime(ctx, repoRoot)
	if err != nil {
		return AuditRuntimeSummary{}, nil, fmt.Errorf("check runtime for audit: %w", err)
	}
	readiness, err := EvaluateRuntimeReadiness(ctx, repoRoot)
	if err != nil {
		return AuditRuntimeSummary{}, nil, fmt.Errorf("evaluate runtime readiness for audit: %w", err)
	}

	findings := auditFindingsFromRuntimeCheck(checkResult.Findings)
	findings = append(findings, auditFindingsFromRuntimeReadiness(readiness.RuntimeReasons)...)

	return AuditRuntimeSummary{
		Check: AuditRuntimeCheckSummary{
			HarnessID:       checkResult.HarnessID,
			OK:              checkResult.OK,
			FindingCount:    checkResult.FindingCount,
			BindingsSummary: checkResult.BindingsSummary,
		},
		Readiness: AuditRuntimeReadinessSummary{
			Status:                readiness.Status,
			RuntimeStatus:         readiness.Runtime.Status,
			AgentStatus:           readiness.Agent.Status,
			AgentActivationStatus: readiness.Agent.ActivationStatus,
			Summary:               readiness.Summary,
		},
	}, findings, nil
}

func auditFindingsFromRuntimeCheck(findings []CheckFinding) []AuditFinding {
	auditFindings := make([]AuditFinding, 0, len(findings))
	for _, finding := range findings {
		severity := AuditStatusFail
		if checkFindingIsWarningOnly(finding.Kind) {
			severity = AuditStatusWarn
		}
		auditFindings = append(auditFindings, AuditFinding{
			Severity: severity,
			Kind:     "runtime_check_" + string(finding.Kind),
			Package:  finding.OrbitID,
			Path:     finding.Path,
			Message:  finding.Message,
		})
	}

	return auditFindings
}

func auditFindingsFromRuntimeReadiness(reasons []ReadinessReason) []AuditFinding {
	auditFindings := make([]AuditFinding, 0, len(reasons))
	for _, reason := range reasons {
		severity := AuditStatusFail
		switch reason.Severity {
		case ReadinessReasonSeverityAdvisory:
			severity = AuditStatusWarn
		case ReadinessReasonSeverityBlocking:
			severity = AuditStatusFail
		}

		code := strings.TrimSpace(string(reason.Code))
		if code == "" {
			code = "unknown"
		}
		auditFindings = append(auditFindings, AuditFinding{
			Severity: severity,
			Kind:     "runtime_readiness_" + code,
			Package:  auditReadinessReasonPackage(reason),
			Message:  reason.Message,
		})
	}

	return auditFindings
}

func auditReadinessReasonPackage(reason ReadinessReason) string {
	if strings.TrimSpace(reason.OrbitID) != "" {
		return reason.OrbitID
	}
	if len(reason.OrbitIDs) == 1 {
		return reason.OrbitIDs[0]
	}
	if len(reason.OrbitIDs) > 1 {
		return strings.Join(reason.OrbitIDs, ",")
	}

	return ""
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
