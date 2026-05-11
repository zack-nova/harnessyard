package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
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

type auditedHostedOrbitSpec struct {
	Spec    orbitpkg.OrbitSpec
	Path    string
	Package string
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

	if manifest.Kind == ManifestKindSource {
		return auditSourceRevision(ctx, repo.Root, manifest)
	}

	result := AuditResult{
		RepoRoot:     repo.Root,
		Status:       DeriveAuditStatus([]AuditFinding{}),
		RevisionKind: manifest.Kind,
		Packages:     auditPackageSummaries(manifest),
		Findings:     []AuditFinding{},
	}
	switch manifest.Kind {
	case ManifestKindRuntime:
		runtimeSummary, runtimeFindings, err := auditRuntimeRevision(ctx, repo.Root)
		if err != nil {
			return AuditResult{}, err
		}
		result.Runtime = &runtimeSummary
		result.Findings = runtimeFindings
		result.Status = DeriveAuditStatus(result.Findings)
	case ManifestKindHarnessTemplate:
		result.Findings = auditHarnessTemplateRevision(ctx, repo.Root)
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

func auditHarnessTemplateRevision(ctx context.Context, repoRoot string) []AuditFinding {
	if _, err := ResolveWorktreeTemplateInstallSource(ctx, repoRoot); err != nil {
		return []AuditFinding{auditFindingFromTemplateInstallSourceError(err, repoRoot)}
	}

	return []AuditFinding{}
}

func auditFindingFromTemplateInstallSourceError(err error, repoRoot string) AuditFinding {
	finding := AuditFinding{
		Severity: AuditStatusFail,
		Kind:     templateInstallSourceFindingInstallabilityInvalid,
		Message:  stableAuditRepoError(err, repoRoot),
	}

	var validationErr *templateInstallSourceValidationError
	if errors.As(err, &validationErr) {
		if strings.TrimSpace(validationErr.Kind) != "" {
			finding.Kind = validationErr.Kind
		}
		finding.Package = validationErr.Package
		finding.Path = validationErr.Path
	}

	return finding
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

func auditSourceRevision(ctx context.Context, repoRoot string, manifest ManifestFile) (AuditResult, error) {
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return AuditResult{}, fmt.Errorf("list tracked files: %w", err)
	}
	trackedSet, err := auditPathSet(trackedFiles)
	if err != nil {
		return AuditResult{}, fmt.Errorf("index tracked files: %w", err)
	}
	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return AuditResult{}, fmt.Errorf("list worktree files: %w", err)
	}
	trackedDirectories, err := auditTrackedDirectories(trackedFiles)
	if err != nil {
		return AuditResult{}, fmt.Errorf("list tracked directories: %w", err)
	}

	findings := []AuditFinding{}
	specs := []auditedHostedOrbitSpec{}
	if _, ok := trackedSet[ManifestRepoPath()]; !ok {
		findings = append(findings, AuditFinding{
			Severity: AuditStatusFail,
			Kind:     "manifest_untracked",
			Path:     ManifestRepoPath(),
			Message:  fmt.Sprintf("required source manifest %q is not tracked", ManifestRepoPath()),
		})
	}

	sourcePackageName := ""
	expectedSourceSpecPath := ""
	if manifest.Source != nil {
		sourcePackageName = manifest.Source.OrbitID
		expectedSourceSpecPath, err = orbitpkg.HostedDefinitionRelativePath(sourcePackageName)
		if err != nil {
			return AuditResult{}, fmt.Errorf("build source hosted OrbitSpec path: %w", err)
		}
	}

	hostedSpecPaths := auditHostedOrbitSpecPaths(worktreeFiles)
	hostedSpecPathSet, err := auditPathSet(hostedSpecPaths)
	if err != nil {
		return AuditResult{}, fmt.Errorf("index hosted OrbitSpec paths: %w", err)
	}
	if expectedSourceSpecPath != "" {
		if _, ok := hostedSpecPathSet[expectedSourceSpecPath]; !ok {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "source_package_orbit_spec_missing",
				Package:  sourcePackageName,
				Path:     expectedSourceSpecPath,
				Message:  fmt.Sprintf("source package %q has no hosted OrbitSpec at %q", sourcePackageName, expectedSourceSpecPath),
			})
		}
	}

	for _, relativePath := range hostedSpecPaths {
		if _, ok := trackedSet[relativePath]; !ok {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "orbit_spec_untracked",
				Package:  auditHostedOrbitSpecPackage(relativePath),
				Path:     relativePath,
				Message:  fmt.Sprintf("required hosted OrbitSpec %q is not tracked", relativePath),
			})
		}

		data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, relativePath)
		if err != nil {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "orbit_spec_unreadable",
				Package:  auditHostedOrbitSpecPackage(relativePath),
				Path:     relativePath,
				Message:  stableAuditRepoError(err, repoRoot),
			})
			continue
		}

		spec, err := orbitpkg.ParseHostedOrbitSpecData(data, filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "orbit_spec_schema_invalid",
				Package:  auditHostedOrbitSpecPackage(relativePath),
				Path:     relativePath,
				Message:  stableAuditRepoError(err, repoRoot),
			})
			continue
		}
		if sourcePackageName != "" && spec.ID != sourcePackageName {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "package_identity_mismatch",
				Package:  spec.ID,
				Path:     relativePath,
				Message:  fmt.Sprintf("hosted OrbitSpec %q package %q does not match source manifest package %q", relativePath, spec.ID, sourcePackageName),
			})
		}
		specs = append(specs, auditedHostedOrbitSpec{
			Spec:    spec,
			Path:    relativePath,
			Package: auditHostedOrbitSpecPackage(relativePath),
		})
	}

	for _, spec := range specs {
		findings = append(findings, auditHostedOrbitSpecPathFindings(spec, trackedFiles, trackedDirectories)...)
	}

	return AuditResult{
		RepoRoot:     repoRoot,
		Status:       DeriveAuditStatus(findings),
		RevisionKind: manifest.Kind,
		Packages:     auditPackageSummaries(manifest),
		Findings:     findings,
	}, nil
}

func stableAuditManifestError(err error, repoRoot string) string {
	return strings.ReplaceAll(err.Error(), ManifestPath(repoRoot), ManifestRepoPath())
}

func stableAuditRepoError(err error, repoRoot string) string {
	message := err.Error()
	message = strings.ReplaceAll(message, filepath.Join(repoRoot, "."), ".")
	message = strings.ReplaceAll(message, repoRoot+string(filepath.Separator), "")
	message = strings.ReplaceAll(message, repoRoot, ".")

	return message
}

func auditHostedOrbitSpecPaths(trackedFiles []string) []string {
	paths := make([]string, 0)
	for _, trackedFile := range trackedFiles {
		if path.Dir(trackedFile) != OrbitSpecsDirRepoPath() || strings.ToLower(path.Ext(trackedFile)) != ".yaml" {
			continue
		}
		paths = append(paths, trackedFile)
	}
	sort.Strings(paths)

	return paths
}

func auditHostedOrbitSpecPackage(relativePath string) string {
	return strings.TrimSuffix(path.Base(relativePath), path.Ext(relativePath))
}

func auditPathSet(paths []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(paths))
	for _, rawPath := range paths {
		normalizedPath, err := ids.NormalizeRepoRelativePath(rawPath)
		if err != nil {
			return nil, fmt.Errorf("normalize path %q: %w", rawPath, err)
		}
		set[normalizedPath] = struct{}{}
	}

	return set, nil
}

func auditHostedOrbitSpecPathFindings(
	auditedSpec auditedHostedOrbitSpec,
	trackedFiles []string,
	trackedDirectories []string,
) []AuditFinding {
	spec := auditedSpec.Spec
	findings := []AuditFinding{}
	packageName := spec.ID
	if packageName == "" {
		packageName = auditedSpec.Package
	}

	if spec.Capabilities != nil {
		if spec.Capabilities.Commands != nil {
			findings = append(
				findings,
				auditUnmatchedPatternFindings(
					packageName,
					"command_capability_path_unmatched",
					spec.Capabilities.Commands.Paths,
					trackedFiles,
					"command capability path %q matches no tracked files",
					AuditStatusWarn,
				)...,
			)
		}
		if spec.Capabilities.Skills != nil && spec.Capabilities.Skills.Local != nil {
			findings = append(
				findings,
				auditUnmatchedPatternFindings(
					packageName,
					"local_skill_capability_path_unmatched",
					spec.Capabilities.Skills.Local.Paths,
					trackedDirectories,
					"local skill capability root %q matches no tracked directories",
					AuditStatusWarn,
				)...,
			)
		}
	}

	for _, member := range spec.Members {
		findings = append(
			findings,
			auditContentMemberPatternFindings(
				packageName,
				member.Paths,
				trackedFiles,
			)...,
		)
	}

	return findings
}

func auditContentMemberPatternFindings(
	packageName string,
	paths orbitpkg.OrbitMemberPaths,
	trackedFiles []string,
) []AuditFinding {
	findings := []AuditFinding{}
	for _, pattern := range paths.Include {
		matches, err := auditPatternMatchesCandidates(pattern, paths.Exclude, trackedFiles)
		if err != nil {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "audit_pattern_match_error",
				Package:  packageName,
				Path:     pattern,
				Message:  err.Error(),
			})
			continue
		}
		if matches {
			continue
		}

		severity := AuditStatusWarn
		if auditRequiredControlPlanePattern(pattern, packageName) {
			severity = AuditStatusFail
		}
		findings = append(findings, AuditFinding{
			Severity: severity,
			Kind:     "content_member_pattern_unmatched",
			Package:  packageName,
			Path:     pattern,
			Message:  fmt.Sprintf("content member pattern %q matches no tracked files", pattern),
		})
	}

	return findings
}

func auditUnmatchedPatternFindings(
	packageName string,
	kind string,
	paths orbitpkg.OrbitMemberPaths,
	candidates []string,
	messageFormat string,
	severity string,
) []AuditFinding {
	findings := []AuditFinding{}
	for _, pattern := range paths.Include {
		matches, err := auditPatternMatchesCandidates(pattern, paths.Exclude, candidates)
		if err != nil {
			findings = append(findings, AuditFinding{
				Severity: AuditStatusFail,
				Kind:     "audit_pattern_match_error",
				Package:  packageName,
				Path:     pattern,
				Message:  err.Error(),
			})
			continue
		}
		if matches {
			continue
		}
		findings = append(findings, AuditFinding{
			Severity: severity,
			Kind:     kind,
			Package:  packageName,
			Path:     pattern,
			Message:  fmt.Sprintf(messageFormat, pattern),
		})
	}

	return findings
}

func auditPatternMatchesCandidates(pattern string, excludePatterns []string, candidates []string) (bool, error) {
	for _, candidate := range candidates {
		included, err := doublestar.Match(pattern, candidate)
		if err != nil {
			return false, fmt.Errorf("match include pattern %q: %w", pattern, err)
		}
		if !included {
			continue
		}

		excluded := false
		for _, excludePattern := range excludePatterns {
			matched, err := doublestar.Match(excludePattern, candidate)
			if err != nil {
				return false, fmt.Errorf("match exclude pattern %q: %w", excludePattern, err)
			}
			if matched {
				excluded = true
				break
			}
		}
		if !excluded {
			return true, nil
		}
	}

	return false, nil
}

func auditRequiredControlPlanePattern(pattern string, packageName string) bool {
	if strings.ContainsAny(pattern, "*?[") {
		return false
	}
	if pattern == ManifestRepoPath() {
		return true
	}

	hostedSpecPath, err := orbitpkg.HostedDefinitionRelativePath(packageName)
	if err != nil {
		return false
	}

	return pattern == hostedSpecPath
}

func auditTrackedDirectories(trackedFiles []string) ([]string, error) {
	directories := map[string]struct{}{}
	for _, trackedFile := range trackedFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(trackedFile)
		if err != nil {
			return nil, fmt.Errorf("normalize tracked file %q: %w", trackedFile, err)
		}

		dir := path.Dir(normalizedPath)
		for dir != "." && dir != "/" {
			directories[dir] = struct{}{}
			dir = path.Dir(dir)
		}
	}

	values := make([]string, 0, len(directories))
	for directory := range directories {
		values = append(values, directory)
	}
	sort.Strings(values)

	return values, nil
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
