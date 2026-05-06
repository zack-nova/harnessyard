package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// ReadinessStatus is one stable runtime readiness classification.
type ReadinessStatus string

const (
	ReadinessStatusBroken ReadinessStatus = "broken"
	ReadinessStatusUsable ReadinessStatus = "usable"
	ReadinessStatusReady  ReadinessStatus = "ready"
)

// ReadinessReasonSeverity captures whether one readiness reason is blocking or advisory.
type ReadinessReasonSeverity string

const (
	ReadinessReasonSeverityBlocking ReadinessReasonSeverity = "blocking"
	ReadinessReasonSeverityAdvisory ReadinessReasonSeverity = "advisory"
)

// ReadinessReasonCode is one stable readiness explanation code.
type ReadinessReasonCode string

const (
	ReadinessReasonInvalidManifest             ReadinessReasonCode = "invalid_manifest"
	ReadinessReasonInvalidOrbitSpec            ReadinessReasonCode = "invalid_orbit_spec"
	ReadinessReasonInvalidInstallRecord        ReadinessReasonCode = "invalid_install_record"
	ReadinessReasonInstallConflict             ReadinessReasonCode = "install_conflict"
	ReadinessReasonInvalidAgentsContainer      ReadinessReasonCode = "invalid_agents_container"
	ReadinessReasonUnresolvedRequiredBindings  ReadinessReasonCode = "unresolved_required_bindings"
	ReadinessReasonRuntimePlaceholdersObserved ReadinessReasonCode = "runtime_placeholders_observed"
	ReadinessReasonAgentsNotComposed           ReadinessReasonCode = "agents_not_composed"
	ReadinessReasonAgentsBlockDrift            ReadinessReasonCode = "agents_block_drift"
	ReadinessReasonOrbitEntryIncomplete        ReadinessReasonCode = "orbit_entry_incomplete"
	ReadinessReasonInstallMemberDrift          ReadinessReasonCode = "install_member_drift"
	ReadinessReasonAgentNotSelected            ReadinessReasonCode = "agent_not_selected"
	ReadinessReasonAgentActivationMissing      ReadinessReasonCode = "agent_activation_missing"
	ReadinessReasonAgentActivationStale        ReadinessReasonCode = "agent_activation_stale"
	ReadinessReasonAgentHooksPending           ReadinessReasonCode = "agent_hooks_pending"
	ReadinessReasonAgentRequiredDependency     ReadinessReasonCode = "agent_required_dependency_unresolved"
	ReadinessReasonAgentRequiredEvent          ReadinessReasonCode = "agent_required_event_unsupported"
	ReadinessReasonAgentCleanupBlocked         ReadinessReasonCode = "agent_cleanup_blocked"
	ReadinessReasonInvalidAgentTruth           ReadinessReasonCode = "invalid_agent_truth"
	ReadinessReasonInvalidAgentActivation      ReadinessReasonCode = "invalid_agent_activation_ledger"
	ReadinessReasonAgentOwnershipConflict      ReadinessReasonCode = "agent_activation_ownership_conflict"
)

// ReadinessSummary captures high-level readiness counts.
type ReadinessSummary struct {
	OrbitCount          int `json:"orbit_count"`
	ReadyOrbitCount     int `json:"ready_orbit_count"`
	UsableOrbitCount    int `json:"usable_orbit_count"`
	BrokenOrbitCount    int `json:"broken_orbit_count"`
	BlockingReasonCount int `json:"blocking_reason_count"`
	AdvisoryReasonCount int `json:"advisory_reason_count"`
}

// ReadinessReason captures one stable readiness explanation.
type ReadinessReason struct {
	Code     ReadinessReasonCode     `json:"code"`
	Severity ReadinessReasonSeverity `json:"severity"`
	Message  string                  `json:"message"`
	OrbitID  string                  `json:"orbit_id,omitempty"`
	OrbitIDs []string                `json:"orbit_ids,omitempty"`
}

// ReadinessOrbitReport captures readiness state for one runtime member orbit.
type ReadinessOrbitReport struct {
	OrbitID      string            `json:"orbit_id"`
	MemberSource string            `json:"member_source"`
	Status       ReadinessStatus   `json:"status"`
	Reasons      []ReadinessReason `json:"reasons,omitempty"`
}

// ReadinessNextStep captures one stable suggested next action.
type ReadinessNextStep struct {
	Command string `json:"command"`
	Intent  string `json:"intent"`
}

// ReadinessRuntimeReport captures the runtime-only layer underneath handoff readiness.
type ReadinessRuntimeReport struct {
	Status  ReadinessStatus   `json:"status"`
	Reasons []ReadinessReason `json:"reasons,omitempty"`
}

// ReadinessAgentReport captures the agent activation layer underneath handoff readiness.
type ReadinessAgentReport struct {
	Status           ReadinessStatus          `json:"status"`
	Required         bool                     `json:"required"`
	ResolvedAgent    string                   `json:"resolved_agent,omitempty"`
	RecommendedAgent string                   `json:"recommended_agent,omitempty"`
	ResolutionSource FrameworkSelectionSource `json:"resolution_source,omitempty"`
	ActivationStatus string                   `json:"activation_status"`
	Reasons          []ReadinessReason        `json:"reasons,omitempty"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

// ReadinessReport captures one derived readiness view of the current runtime.
type ReadinessReport struct {
	HarnessID      string                 `json:"harness_id,omitempty"`
	Status         ReadinessStatus        `json:"status"`
	Runtime        ReadinessRuntimeReport `json:"runtime"`
	Agent          ReadinessAgentReport   `json:"agent"`
	Summary        ReadinessSummary       `json:"summary"`
	RuntimeReasons []ReadinessReason      `json:"runtime_reasons,omitempty"`
	OrbitReports   []ReadinessOrbitReport `json:"orbit_reports,omitempty"`
	NextSteps      []ReadinessNextStep    `json:"next_steps,omitempty"`
}

// EvaluateRuntimeReadiness derives the current runtime readiness view without mutating repo state.
func EvaluateRuntimeReadiness(ctx context.Context, repoRoot string) (ReadinessReport, error) {
	checkResult, err := CheckRuntime(ctx, repoRoot)
	if err != nil {
		return ReadinessReport{}, fmt.Errorf("check runtime readiness prerequisites: %w", err)
	}

	report := ReadinessReport{
		HarnessID: checkResult.HarnessID,
		Status:    ReadinessStatusReady,
		Runtime: ReadinessRuntimeReport{
			Status: ReadinessStatusReady,
		},
		Agent: ReadinessAgentReport{
			Status:           ReadinessStatusReady,
			ActivationStatus: "not_required",
		},
	}

	runtimeFile, runtimeErr := LoadRuntimeFile(repoRoot)
	hasRuntimeView := runtimeErr == nil
	if hasRuntimeView {
		report.HarnessID = runtimeFile.Harness.ID
		report.OrbitReports = make([]ReadinessOrbitReport, 0, len(runtimeFile.Members))
		for _, member := range runtimeFile.Members {
			report.OrbitReports = append(report.OrbitReports, ReadinessOrbitReport{
				OrbitID:      member.OrbitID,
				MemberSource: string(member.Source),
				Status:       ReadinessStatusReady,
			})
		}
		sort.Slice(report.OrbitReports, func(left, right int) bool {
			return report.OrbitReports[left].OrbitID < report.OrbitReports[right].OrbitID
		})
	}

	orbitIndex := make(map[string]*ReadinessOrbitReport, len(report.OrbitReports))
	for index := range report.OrbitReports {
		orbitIndex[report.OrbitReports[index].OrbitID] = &report.OrbitReports[index]
	}

	runtimeScopedReasons := make([]ReadinessReason, 0)
	for _, finding := range checkResult.Findings {
		reason, orbitScoped := readinessReasonForCheckFinding(finding)
		if reason.Code == "" {
			continue
		}
		if orbitScoped && finding.OrbitID != "" {
			if orbitReport, ok := orbitIndex[finding.OrbitID]; ok {
				appendOrbitReadinessReason(orbitReport, reason)
				continue
			}
		}
		appendRuntimeReadinessReason(&runtimeScopedReasons, reason)
	}

	if hasRuntimeView {
		populateAgentsContainerReadiness(repoRoot, &runtimeScopedReasons)
		if err := populateBriefLaneReadiness(ctx, repoRoot, runtimeFile, &report); err != nil {
			return ReadinessReport{}, err
		}
		if err := populateBindingsReadiness(ctx, repoRoot, runtimeFile, &report); err != nil {
			return ReadinessReport{}, err
		}
		if (len(runtimeFile.Members) > 0 || agentReadinessInputsMayExist(ctx, repoRoot)) && !hasBlockingReadinessReason(runtimeScopedReasons, report.OrbitReports) {
			agentReport, agentReasons, err := evaluateAgentReadiness(ctx, repoRoot)
			if err != nil {
				return ReadinessReport{}, err
			}
			report.Agent = agentReport
			for _, reason := range agentReasons {
				appendRuntimeReadinessReason(&runtimeScopedReasons, reason)
			}
		}
	}

	for index := range report.OrbitReports {
		sort.Slice(report.OrbitReports[index].Reasons, func(left, right int) bool {
			return report.OrbitReports[index].Reasons[left].Code < report.OrbitReports[index].Reasons[right].Code
		})
		report.OrbitReports[index].Status = deriveOrbitReadinessStatus(report.OrbitReports[index].Reasons)
	}

	runtimeOnlyReasons := aggregateRuntimeReadinessReasons(
		filterReadinessReasons(runtimeScopedReasons, func(reason ReadinessReason) bool {
			return !isAgentReadinessReason(reason.Code)
		}),
		report.OrbitReports,
	)
	report.Runtime = ReadinessRuntimeReport{
		Status:  deriveRuntimeReadinessStatus(runtimeOnlyReasons),
		Reasons: runtimeOnlyReasons,
	}
	report.RuntimeReasons = aggregateRuntimeReadinessReasons(runtimeScopedReasons, report.OrbitReports)
	report.Summary = buildReadinessSummary(report.RuntimeReasons, report.OrbitReports)
	report.Status = deriveRuntimeReadinessStatus(report.RuntimeReasons)
	report.NextSteps = buildReadinessNextSteps(report)

	return report, nil
}

func readinessReasonForCheckFinding(finding CheckFinding) (ReadinessReason, bool) {
	switch finding.Kind {
	case CheckFindingManifestSchemaInvalid:
		return readinessReason(ReadinessReasonInvalidManifest, ReadinessReasonSeverityBlocking, ""), false
	case CheckFindingMissingDefinition:
		return readinessReason(ReadinessReasonInvalidOrbitSpec, ReadinessReasonSeverityBlocking, finding.OrbitID), true
	case CheckFindingInstallRecordInvalid:
		return readinessReason(ReadinessReasonInvalidInstallRecord, ReadinessReasonSeverityBlocking, finding.OrbitID), finding.OrbitID != ""
	case CheckFindingInstallMemberMismatch, CheckFindingInstallPathMismatch, CheckFindingBundleMemberMismatch, CheckFindingBundlePathMismatch:
		return readinessReason(ReadinessReasonInstallConflict, ReadinessReasonSeverityBlocking, finding.OrbitID), finding.OrbitID != ""
	case CheckFindingKind(orbittemplate.DriftKindDefinition),
		CheckFindingKind(orbittemplate.DriftKindRuntimeFile),
		CheckFindingKind(orbittemplate.DriftKindProvenanceUnresolvable):
		return readinessReason(ReadinessReasonInstallMemberDrift, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	default:
		return ReadinessReason{}, false
	}
}

func populateAgentsContainerReadiness(repoRoot string, runtimeScopedReasons *[]ReadinessReason) {
	if err := orbittemplate.ValidateRuntimeAgentsFile(repoRoot); err != nil {
		appendRuntimeReadinessReason(
			runtimeScopedReasons,
			readinessReason(ReadinessReasonInvalidAgentsContainer, ReadinessReasonSeverityBlocking, ""),
		)
	}
}

func populateBriefLaneReadiness(ctx context.Context, repoRoot string, runtimeFile RuntimeFile, report *ReadinessReport) error {
	membersByOrbitID := make(map[string]RuntimeMember, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		membersByOrbitID[member.OrbitID] = member
	}

	for index := range report.OrbitReports {
		orbitReport := &report.OrbitReports[index]
		if member, ok := membersByOrbitID[orbitReport.OrbitID]; ok && !runtimeMemberUsesStandaloneBriefReadiness(member) {
			continue
		}
		if orbitHasBlockingReadinessReason(*orbitReport) && orbitHasReasonCode(*orbitReport, ReadinessReasonInvalidOrbitSpec) {
			continue
		}

		status, err := orbittemplate.InspectOrbitBriefLane(ctx, repoRoot, orbitReport.OrbitID)
		if err != nil {
			if orbitHasReasonCode(*orbitReport, ReadinessReasonInvalidOrbitSpec) {
				continue
			}
			return fmt.Errorf("inspect orbit %q brief lane: %w", orbitReport.OrbitID, err)
		}

		switch status.State {
		case orbittemplate.BriefLaneStateStructuredOnly:
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonAgentsNotComposed, ReadinessReasonSeverityAdvisory, orbitReport.OrbitID))
		case orbittemplate.BriefLaneStateMaterializedDrifted:
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonAgentsBlockDrift, ReadinessReasonSeverityAdvisory, orbitReport.OrbitID))
		case orbittemplate.BriefLaneStateInvalidContainer:
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonInvalidAgentsContainer, ReadinessReasonSeverityBlocking, orbitReport.OrbitID))
		case orbittemplate.BriefLaneStateMissingTruth:
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonOrbitEntryIncomplete, ReadinessReasonSeverityAdvisory, orbitReport.OrbitID))
		}
	}

	return nil
}

func runtimeMemberUsesStandaloneBriefReadiness(member RuntimeMember) bool {
	return member.Source != MemberSourceInstallBundle && strings.TrimSpace(member.OwnerHarnessID) == ""
}

func populateBindingsReadiness(ctx context.Context, repoRoot string, runtimeFile RuntimeFile, report *ReadinessReport) error {
	varsFile, err := loadOptionalBindingsDiagnosticsVars(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load readiness vars: %w", err)
	}

	hasInstallBackedOrbit := false
	for _, member := range runtimeFile.Members {
		if member.Source == MemberSourceInstallOrbit {
			hasInstallBackedOrbit = true
			break
		}
	}
	if !hasInstallBackedOrbit {
		return nil
	}

	skipPlaceholderScan := runtimeHasReasonCode(*report, ReadinessReasonInvalidAgentsContainer)

	var repoConfig orbitpkg.RepositoryConfig
	var trackedFiles []string
	var candidatePaths []string
	if !skipPlaceholderScan {
		repoConfig, err = loadTemplateCandidateRepositoryConfig(ctx, repoRoot)
		if err != nil {
			return fmt.Errorf("load readiness repository config: %w", err)
		}
		trackedFiles, err = gitpkg.TrackedFiles(ctx, repoRoot)
		if err != nil {
			return fmt.Errorf("load readiness tracked files: %w", err)
		}
		candidatePaths, err = runtimeBindingsCandidatePaths(ctx, repoRoot, trackedFiles)
		if err != nil {
			return fmt.Errorf("load readiness candidate paths: %w", err)
		}
	}

	orbitIndex := make(map[string]*ReadinessOrbitReport, len(report.OrbitReports))
	for index := range report.OrbitReports {
		orbitIndex[report.OrbitReports[index].OrbitID] = &report.OrbitReports[index]
	}

	for _, member := range runtimeFile.Members {
		if member.Source != MemberSourceInstallOrbit {
			continue
		}

		orbitReport := orbitIndex[member.OrbitID]
		if orbitReport == nil || orbitHasBlockingReadinessReason(*orbitReport) {
			continue
		}

		record, err := LoadInstallRecord(repoRoot, member.OrbitID)
		if err != nil {
			if orbitHasReasonCode(*orbitReport, ReadinessReasonInstallConflict) || orbitHasReasonCode(*orbitReport, ReadinessReasonInvalidInstallRecord) {
				continue
			}
			return fmt.Errorf("load install record for readiness %q: %w", member.OrbitID, err)
		}

		if installRecordHasMissingRequiredBindings(record, varsFile) {
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonUnresolvedRequiredBindings, ReadinessReasonSeverityAdvisory, member.OrbitID))
		}

		if skipPlaceholderScan {
			continue
		}
		scanResult, err := scanRuntimeBindingsForOrbit(ctx, repoRoot, repoConfig, trackedFiles, candidatePaths, member.OrbitID)
		if err != nil {
			return fmt.Errorf("scan runtime bindings for readiness %q: %w", member.OrbitID, err)
		}
		if scanResult.PlaceholderCount > 0 {
			appendOrbitReadinessReason(orbitReport, readinessReason(ReadinessReasonRuntimePlaceholdersObserved, ReadinessReasonSeverityAdvisory, member.OrbitID))
		}
	}

	return nil
}

func installRecordHasMissingRequiredBindings(record orbittemplate.InstallRecord, varsFile bindings.VarsFile) bool {
	if record.Variables == nil {
		return false
	}
	for _, name := range sortedDeclarationNames(record.Variables.Declarations) {
		declaration := record.Variables.Declarations[name]
		if !declaration.Required {
			continue
		}
		namespace := record.Variables.Namespaces[name]
		if !hasNamespaceAwareBinding(varsFile, namespace, name) {
			return true
		}
	}
	return false
}

func evaluateAgentReadiness(ctx context.Context, repoRoot string) (ReadinessAgentReport, []ReadinessReason, error) {
	report := ReadinessAgentReport{
		Status:           ReadinessStatusReady,
		ActivationStatus: "not_required",
	}
	reasons := []ReadinessReason{}

	gitDir, err := gitpkg.Dir(ctx, repoRoot)
	if err != nil {
		return ReadinessAgentReport{}, nil, fmt.Errorf("resolve readiness git dir: %w", err)
	}
	activationIDs, err := ListFrameworkActivationIDs(gitDir)
	if err != nil {
		return agentReadinessReportWithReason(ReadinessReasonInvalidAgentActivation, ReadinessReasonSeverityBlocking, "invalid")
	}

	state, err := loadFrameworkDesiredState(ctx, repoRoot, gitDir)
	if err != nil {
		return agentReadinessReportWithReason(ReadinessReasonInvalidAgentTruth, ReadinessReasonSeverityBlocking, "invalid")
	}

	report.ResolvedAgent = state.Summary.ResolvedFramework
	report.RecommendedAgent = state.Summary.RecommendedFramework
	report.ResolutionSource = state.Summary.ResolutionSource
	report.Required = agentReadinessRequired(state.Summary, activationIDs)
	if !report.Required {
		return report, reasons, nil
	}

	if state.Summary.ResolutionSource == FrameworkSelectionSourceUnresolvedConflict {
		appendRuntimeReadinessReason(
			&reasons,
			readinessReason(ReadinessReasonInvalidAgentTruth, ReadinessReasonSeverityBlocking, ""),
		)
		report.Status = deriveRuntimeReadinessStatus(reasons)
		report.ActivationStatus = "invalid"
		report.Reasons = reasons
		return report, reasons, nil
	}
	if state.Summary.ResolvedFramework == "" {
		appendRuntimeReadinessReason(
			&reasons,
			readinessReason(ReadinessReasonAgentNotSelected, ReadinessReasonSeverityAdvisory, ""),
		)
		report.Status = deriveRuntimeReadinessStatus(reasons)
		report.ActivationStatus = "unselected"
		report.Reasons = reasons
		return report, reasons, nil
	}

	check, err := CheckFramework(ctx, repoRoot, gitDir)
	if err != nil {
		code := ReadinessReasonInvalidAgentTruth
		if strings.Contains(err.Error(), "activation ledger") {
			code = ReadinessReasonInvalidAgentActivation
		}
		return agentReadinessReportWithBaseAndReason(report, code, ReadinessReasonSeverityBlocking, "invalid")
	}
	for _, finding := range check.Findings {
		reason, ok := readinessReasonForFrameworkCheckFinding(finding)
		if !ok {
			continue
		}
		appendRuntimeReadinessReason(&reasons, reason)
	}
	report.Warnings = append([]string(nil), check.Warnings...)

	report.Status = deriveRuntimeReadinessStatus(reasons)
	report.ActivationStatus = agentReadinessActivationStatus(reasons)
	report.Reasons = reasons
	return report, reasons, nil
}

func agentReadinessReportWithBaseAndReason(base ReadinessAgentReport, code ReadinessReasonCode, severity ReadinessReasonSeverity, activationStatus string) (ReadinessAgentReport, []ReadinessReason, error) {
	report, reasons, err := agentReadinessReportWithReason(code, severity, activationStatus)
	report.Required = base.Required
	report.ResolvedAgent = base.ResolvedAgent
	report.RecommendedAgent = base.RecommendedAgent
	report.ResolutionSource = base.ResolutionSource

	return report, reasons, err
}

func agentReadinessReportWithReason(code ReadinessReasonCode, severity ReadinessReasonSeverity, activationStatus string) (ReadinessAgentReport, []ReadinessReason, error) {
	reasons := []ReadinessReason{}
	appendRuntimeReadinessReason(&reasons, readinessReason(code, severity, ""))
	report := ReadinessAgentReport{
		Status:           deriveRuntimeReadinessStatus(reasons),
		ActivationStatus: activationStatus,
		Reasons:          reasons,
	}

	return report, reasons, nil
}

func agentReadinessRequired(summary FrameworkInspectSummary, activationIDs []string) bool {
	return len(activationIDs) > 0 ||
		summary.ResolvedFramework != "" ||
		summary.RecommendedFramework != "" ||
		len(summary.PackageRecommendations) > 0 ||
		summary.CommandCount > 0 ||
		summary.SkillCount > 0 ||
		summary.RemoteSkillCount > 0 ||
		summary.HasAgentConfig ||
		summary.HasAgentHooks ||
		summary.AgentHookCount > 0 ||
		summary.PackageAgentHookCount > 0
}

func readinessReasonForFrameworkCheckFinding(finding FrameworkCheckFinding) (ReadinessReason, bool) {
	switch finding.Kind {
	case "activation_missing":
		return readinessReason(ReadinessReasonAgentActivationMissing, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	case "activation_stale", "package_hook_stale", "missing_output", "source_missing", "source_unreadable", "compiled_skill_missing", "compiled_skill_stale", "generated_output_stale", "hook_output_stale":
		return readinessReason(ReadinessReasonAgentActivationStale, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	case "package_hook_pending":
		return readinessReason(ReadinessReasonAgentHooksPending, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	case "skill_remote_uri_unsupported":
		return readinessReason(ReadinessReasonAgentRequiredDependency, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	case "package_hook_event_unsupported", "hook_event_unsupported":
		return readinessReason(ReadinessReasonAgentRequiredEvent, ReadinessReasonSeverityAdvisory, finding.OrbitID), true
	case "command_invalid", "command_name_collision", "skill_invalid", "skill_missing_skill_md", "skill_missing_name", "skill_missing_description", "skill_invalid_frontmatter", "skill_name_mismatch", "skill_name_collision", "agent_addon_hook_invalid":
		return readinessReason(ReadinessReasonInvalidAgentTruth, ReadinessReasonSeverityBlocking, finding.OrbitID), true
	default:
		return ReadinessReason{}, false
	}
}

func agentReadinessActivationStatus(reasons []ReadinessReason) string {
	status := "current"
	for _, reason := range reasons {
		switch reason.Code {
		case ReadinessReasonInvalidAgentTruth, ReadinessReasonInvalidAgentActivation, ReadinessReasonAgentOwnershipConflict:
			return "invalid"
		case ReadinessReasonAgentActivationMissing:
			status = "missing"
		case ReadinessReasonAgentActivationStale:
			if status != "missing" {
				status = "stale"
			}
		case ReadinessReasonAgentHooksPending:
			if status == "current" {
				status = "hooks_pending"
			}
		case ReadinessReasonAgentNotSelected:
			if status == "current" {
				status = "unselected"
			}
		}
	}

	return status
}

func agentReadinessInputsMayExist(ctx context.Context, repoRoot string) bool {
	gitDir, err := gitpkg.Dir(ctx, repoRoot)
	if err != nil {
		return false
	}
	if _, err := LoadFrameworkSelection(gitDir); err == nil || !errors.Is(err, os.ErrNotExist) {
		return true
	}
	activationIDs, err := ListFrameworkActivationIDs(gitDir)
	if err != nil || len(activationIDs) > 0 {
		return true
	}
	if _, err := os.Stat(AgentUnifiedConfigPath(repoRoot)); err == nil || !errors.Is(err, os.ErrNotExist) {
		return true
	}

	return false
}

func readinessReason(code ReadinessReasonCode, severity ReadinessReasonSeverity, orbitID string) ReadinessReason {
	return ReadinessReason{
		Code:     code,
		Severity: severity,
		Message:  readinessReasonMessage(code),
		OrbitID:  strings.TrimSpace(orbitID),
	}
}

func readinessReasonMessage(code ReadinessReasonCode) string {
	switch code {
	case ReadinessReasonInvalidManifest:
		return "runtime manifest is invalid"
	case ReadinessReasonInvalidOrbitSpec:
		return "runtime contains a missing or invalid orbit definition"
	case ReadinessReasonInvalidInstallRecord:
		return "runtime contains an invalid install record"
	case ReadinessReasonInstallConflict:
		return "runtime membership or provenance records are inconsistent"
	case ReadinessReasonInvalidAgentsContainer:
		return "root AGENTS.md container is invalid or cannot be tracked"
	case ReadinessReasonUnresolvedRequiredBindings:
		return "required bindings are still unresolved"
	case ReadinessReasonRuntimePlaceholdersObserved:
		return "runtime still contains placeholder variables"
	case ReadinessReasonAgentsNotComposed:
		return "root AGENTS.md has not been composed for this orbit"
	case ReadinessReasonAgentsBlockDrift:
		return "root AGENTS.md contains a drifted orbit block"
	case ReadinessReasonOrbitEntryIncomplete:
		return "orbit entry contract is incomplete for worker handoff"
	case ReadinessReasonInstallMemberDrift:
		return "install-backed member has drift relative to recorded provenance"
	case ReadinessReasonAgentNotSelected:
		return "agent-facing package add-ons exist but no local agent is selected"
	case ReadinessReasonAgentActivationMissing:
		return "agent activation has not been applied for package add-ons"
	case ReadinessReasonAgentActivationStale:
		return "agent activation is stale relative to package add-ons"
	case ReadinessReasonAgentHooksPending:
		return "agent hook activation is pending"
	case ReadinessReasonAgentRequiredDependency:
		return "required agent dependency is unresolved for the selected agent"
	case ReadinessReasonAgentRequiredEvent:
		return "required agent hook event is unsupported by the selected agent"
	case ReadinessReasonAgentCleanupBlocked:
		return "agent cleanup is blocked by ownership safety"
	case ReadinessReasonInvalidAgentTruth:
		return "agent desired state is invalid"
	case ReadinessReasonInvalidAgentActivation:
		return "agent activation ledger is invalid"
	case ReadinessReasonAgentOwnershipConflict:
		return "agent activation ownership is unsafe to interpret"
	default:
		return "runtime readiness issue detected"
	}
}

func appendOrbitReadinessReason(report *ReadinessOrbitReport, reason ReadinessReason) {
	for _, existing := range report.Reasons {
		if existing.Code == reason.Code && existing.Severity == reason.Severity {
			return
		}
	}
	report.Reasons = append(report.Reasons, reason)
}

func appendRuntimeReadinessReason(reasons *[]ReadinessReason, reason ReadinessReason) {
	for _, existing := range *reasons {
		if existing.Code == reason.Code && existing.Severity == reason.Severity {
			return
		}
	}
	*reasons = append(*reasons, reason)
}

func hasBlockingReadinessReason(runtimeScoped []ReadinessReason, orbitReports []ReadinessOrbitReport) bool {
	for _, reason := range runtimeScoped {
		if reason.Severity == ReadinessReasonSeverityBlocking {
			return true
		}
	}
	for _, orbitReport := range orbitReports {
		if orbitHasBlockingReadinessReason(orbitReport) {
			return true
		}
	}

	return false
}

func filterReadinessReasons(reasons []ReadinessReason, keep func(ReadinessReason) bool) []ReadinessReason {
	filtered := make([]ReadinessReason, 0, len(reasons))
	for _, reason := range reasons {
		if keep(reason) {
			filtered = append(filtered, reason)
		}
	}

	return filtered
}

func isAgentReadinessReason(code ReadinessReasonCode) bool {
	switch code {
	case ReadinessReasonAgentNotSelected,
		ReadinessReasonAgentActivationMissing,
		ReadinessReasonAgentActivationStale,
		ReadinessReasonAgentHooksPending,
		ReadinessReasonAgentRequiredDependency,
		ReadinessReasonAgentRequiredEvent,
		ReadinessReasonAgentCleanupBlocked,
		ReadinessReasonInvalidAgentTruth,
		ReadinessReasonInvalidAgentActivation,
		ReadinessReasonAgentOwnershipConflict:
		return true
	default:
		return false
	}
}

func orbitHasReasonCode(report ReadinessOrbitReport, code ReadinessReasonCode) bool {
	for _, reason := range report.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func orbitHasBlockingReadinessReason(report ReadinessOrbitReport) bool {
	for _, reason := range report.Reasons {
		if reason.Severity == ReadinessReasonSeverityBlocking {
			return true
		}
	}
	return false
}

func deriveOrbitReadinessStatus(reasons []ReadinessReason) ReadinessStatus {
	for _, reason := range reasons {
		if reason.Severity == ReadinessReasonSeverityBlocking {
			return ReadinessStatusBroken
		}
	}
	if len(reasons) > 0 {
		return ReadinessStatusUsable
	}
	return ReadinessStatusReady
}

func aggregateRuntimeReadinessReasons(runtimeScoped []ReadinessReason, orbitReports []ReadinessOrbitReport) []ReadinessReason {
	type aggregate struct {
		reason   ReadinessReason
		orbitIDs map[string]struct{}
	}

	order := make([]string, 0)
	aggregates := make(map[string]*aggregate)

	addAggregate := func(reason ReadinessReason) {
		key := string(reason.Code) + ":" + string(reason.Severity)
		entry, ok := aggregates[key]
		if !ok {
			cloned := reason
			cloned.OrbitIDs = nil
			cloned.OrbitID = ""
			entry = &aggregate{
				reason:   cloned,
				orbitIDs: make(map[string]struct{}),
			}
			aggregates[key] = entry
			order = append(order, key)
		}
		if strings.TrimSpace(reason.OrbitID) != "" {
			entry.orbitIDs[reason.OrbitID] = struct{}{}
		}
		for _, orbitID := range reason.OrbitIDs {
			if strings.TrimSpace(orbitID) == "" {
				continue
			}
			entry.orbitIDs[orbitID] = struct{}{}
		}
	}

	for _, reason := range runtimeScoped {
		addAggregate(reason)
	}
	for _, orbitReport := range orbitReports {
		for _, reason := range orbitReport.Reasons {
			addAggregate(reason)
		}
	}

	results := make([]ReadinessReason, 0, len(order))
	for _, key := range order {
		entry := aggregates[key]
		orbitIDs := make([]string, 0, len(entry.orbitIDs))
		for orbitID := range entry.orbitIDs {
			orbitIDs = append(orbitIDs, orbitID)
		}
		sort.Strings(orbitIDs)
		entry.reason.OrbitIDs = orbitIDs
		results = append(results, entry.reason)
	}

	sort.Slice(results, func(left, right int) bool {
		if results[left].Severity == results[right].Severity {
			return results[left].Code < results[right].Code
		}
		return results[left].Severity < results[right].Severity
	})

	return results
}

func buildReadinessSummary(runtimeReasons []ReadinessReason, orbitReports []ReadinessOrbitReport) ReadinessSummary {
	summary := ReadinessSummary{
		OrbitCount: len(orbitReports),
	}
	for _, reason := range runtimeReasons {
		switch reason.Severity {
		case ReadinessReasonSeverityBlocking:
			summary.BlockingReasonCount++
		case ReadinessReasonSeverityAdvisory:
			summary.AdvisoryReasonCount++
		}
	}
	for _, orbitReport := range orbitReports {
		switch orbitReport.Status {
		case ReadinessStatusReady:
			summary.ReadyOrbitCount++
		case ReadinessStatusUsable:
			summary.UsableOrbitCount++
		case ReadinessStatusBroken:
			summary.BrokenOrbitCount++
		}
	}
	return summary
}

func deriveRuntimeReadinessStatus(runtimeReasons []ReadinessReason) ReadinessStatus {
	for _, reason := range runtimeReasons {
		if reason.Severity == ReadinessReasonSeverityBlocking {
			return ReadinessStatusBroken
		}
	}
	if len(runtimeReasons) > 0 {
		return ReadinessStatusUsable
	}
	return ReadinessStatusReady
}

func buildReadinessNextSteps(report ReadinessReport) []ReadinessNextStep {
	steps := make([]ReadinessNextStep, 0)
	addStep := func(command string, intent string) {
		for _, existing := range steps {
			if existing.Command == command {
				return
			}
		}
		steps = append(steps, ReadinessNextStep{
			Command: command,
			Intent:  intent,
		})
	}

	for _, reason := range report.RuntimeReasons {
		switch reason.Code {
		case ReadinessReasonInvalidManifest, ReadinessReasonInvalidOrbitSpec, ReadinessReasonInvalidInstallRecord, ReadinessReasonInstallConflict, ReadinessReasonInvalidAgentsContainer, ReadinessReasonInstallMemberDrift:
			addStep("hyard check --json", "inspect structural or drift diagnostics")
		case ReadinessReasonUnresolvedRequiredBindings:
			addStep("hyard plumbing harness bindings missing --all --json", "inspect missing required bindings")
		case ReadinessReasonRuntimePlaceholdersObserved:
			addStep("hyard plumbing harness bindings scan-runtime --all --json", "inspect runtime placeholders")
		case ReadinessReasonAgentsNotComposed, ReadinessReasonAgentsBlockDrift:
			addStep("hyard guide sync --target agents --output", "refresh root AGENTS orchestration")
		case ReadinessReasonOrbitEntryIncomplete:
			if len(reason.OrbitIDs) > 0 {
				addStep("hyard orbit show "+reason.OrbitIDs[0], "inspect orbit entry contract")
			}
		case ReadinessReasonAgentNotSelected:
			addStep("hyard agent detect --json", "select a local agent before activation")
		case ReadinessReasonAgentActivationMissing, ReadinessReasonAgentActivationStale:
			addStep("hyard agent plan --hooks", "review agent add-on activation")
		case ReadinessReasonAgentHooksPending:
			addStep("hyard agent apply --hooks --yes", "apply pending agent hooks")
		case ReadinessReasonAgentRequiredDependency, ReadinessReasonAgentRequiredEvent, ReadinessReasonInvalidAgentTruth, ReadinessReasonInvalidAgentActivation, ReadinessReasonAgentOwnershipConflict:
			addStep("hyard agent check --json", "inspect agent activation diagnostics")
		}
	}

	return steps
}

func runtimeHasReasonCode(report ReadinessReport, code ReadinessReasonCode) bool {
	for _, reason := range report.RuntimeReasons {
		if reason.Code == code {
			return true
		}
	}
	for _, orbitReport := range report.OrbitReports {
		if orbitHasReasonCode(orbitReport, code) {
			return true
		}
	}
	return false
}
