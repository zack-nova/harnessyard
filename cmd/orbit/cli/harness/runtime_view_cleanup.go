package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	RuntimeViewCleanupCandidateRootGuidanceMarkerLines = "root_guidance_marker_lines"
	RuntimeViewCleanupCandidateMemberHint              = "member_hint"

	RuntimeViewCleanupActionStripMarkerLinesPreserveContent = "strip_marker_lines_preserve_content"
	RuntimeViewCleanupActionRemoveConsumedHint              = "remove_consumed_hint"

	RuntimeViewCleanupSkippedMissing       = "missing"
	RuntimeViewCleanupSkippedNoMarkerLines = "no_marker_lines"

	RuntimeViewDriftKindRootGuidance       = "root_guidance_drift"
	RuntimeViewDriftKindRootGuidanceSyntax = "root_guidance_syntax"
	RuntimeViewDriftKindMemberHint         = "member_hint_drift"
	RuntimeViewDriftKindRuntimeCheck       = "runtime_check_drift"
)

const runtimeViewCleanupPresentationNote = "marker removal is presentation cleanup only; later authoring requires explicit `hyard guide render` or reconciliation"

// RuntimeViewCleanupPlanResult reports Run View cleanup planning and write results.
type RuntimeViewCleanupPlanResult struct {
	Check             bool                              `json:"check"`
	Ready             bool                              `json:"ready"`
	Changed           bool                              `json:"changed"`
	SelectedView      statepkg.RuntimeView              `json:"selected_view"`
	CleanupCandidates []RuntimeViewCleanupCandidate     `json:"cleanup_candidates"`
	ChangedFiles      []RuntimeViewCleanupChangedFile   `json:"changed_files"`
	SkippedTargets    []RuntimeViewCleanupSkippedTarget `json:"skipped_targets"`
	Blockers          []string                          `json:"blockers"`
	DriftDiagnostics  []RuntimeViewDriftDiagnostic      `json:"drift_diagnostics"`
	NextActions       []string                          `json:"next_actions"`
	Notes             []string                          `json:"notes"`
	Runtime           RuntimeViewRuntimeSummary         `json:"runtime"`
}

// RuntimeViewCleanupCandidate is one previewed Run View presentation cleanup.
type RuntimeViewCleanupCandidate struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Target     string `json:"target,omitempty"`
	OwnerKind  string `json:"owner_kind,omitempty"`
	OrbitID    string `json:"orbit_id,omitempty"`
	WorkflowID string `json:"workflow_id,omitempty"`
	Action     string `json:"action"`
}

// RuntimeViewCleanupChangedFile reports one file changed by Run View cleanup.
type RuntimeViewCleanupChangedFile struct {
	Path              string `json:"path"`
	Target            string `json:"target"`
	Action            string `json:"action"`
	BlockCount        int    `json:"block_count"`
	PreservedMetadata bool   `json:"preserved_metadata,omitempty"`
}

// RuntimeViewCleanupSkippedTarget reports one root guidance target that did not need cleanup.
type RuntimeViewCleanupSkippedTarget struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// RuntimeViewDriftDiagnostic is one authored-truth drift signal blocking cleanup.
type RuntimeViewDriftDiagnostic struct {
	Kind            string `json:"kind"`
	Path            string `json:"path"`
	Target          string `json:"target,omitempty"`
	OrbitID         string `json:"orbit_id,omitempty"`
	State           string `json:"state,omitempty"`
	Message         string `json:"message"`
	RecoveryCommand string `json:"recovery_command,omitempty"`
}

type runtimeViewCleanupGuidanceTarget struct {
	target string
	path   string
}

// RuntimeViewCleanupBlockedError reports authored-truth drift that prevented writes.
type RuntimeViewCleanupBlockedError struct {
	Blockers []string
}

func (err RuntimeViewCleanupBlockedError) Error() string {
	if len(err.Blockers) == 0 {
		return "Run View cleanup blocked by Authored Truth Drift"
	}

	return "Run View cleanup blocked by Authored Truth Drift: " + strings.Join(err.Blockers, "; ")
}

// RuntimeViewCleanup applies Run View cleanup unless check mode is requested.
func RuntimeViewCleanup(ctx context.Context, repo gitpkg.Repo, store statepkg.FSStore, check bool) (RuntimeViewCleanupPlanResult, error) {
	result, err := RuntimeViewCleanupPlan(ctx, repo, store, check)
	if err != nil {
		return RuntimeViewCleanupPlanResult{}, err
	}
	if check {
		return result, nil
	}
	if len(result.Blockers) > 0 {
		return result, RuntimeViewCleanupBlockedError{Blockers: append([]string(nil), result.Blockers...)}
	}

	changedFiles, skippedTargets, err := applyRuntimeViewRootGuidanceCleanup(repo.Root)
	if err != nil {
		return result, err
	}
	memberHintChangedFiles, err := applyRuntimeViewMemberHintCleanup(repo.Root, result.CleanupCandidates)
	if err != nil {
		return result, err
	}
	changedFiles = append(changedFiles, memberHintChangedFiles...)
	sortRuntimeViewCleanupChangedFiles(changedFiles)
	result.ChangedFiles = changedFiles
	result.SkippedTargets = skippedTargets
	result.Changed = len(changedFiles) > 0
	result.Notes = runtimeViewCleanupNotes(result)
	result.NextActions = runtimeViewCleanupNextActions(result)

	return result, nil
}

// RuntimeViewCleanupPlan computes the Run View cleanup preview without mutating files.
func RuntimeViewCleanupPlan(ctx context.Context, repo gitpkg.Repo, store statepkg.FSStore, check bool) (RuntimeViewCleanupPlanResult, error) {
	selection, err := store.ReadRuntimeViewSelection()
	if err != nil {
		return RuntimeViewCleanupPlanResult{}, fmt.Errorf("read runtime view selection: %w", err)
	}

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RuntimeViewCleanupPlanResult{}, fmt.Errorf("load harness runtime: %w", err)
	}

	candidates, driftDiagnostics, blockers := inspectRuntimeViewGuidanceCleanup(ctx, repo.Root, runtimeFile.Members)

	memberHints, memberHintOrbits, err := inspectRuntimeViewMemberHints(ctx, repo.Root, runtimeFile.Members)
	if err != nil {
		return RuntimeViewCleanupPlanResult{}, err
	}
	memberCandidates, memberDrift, memberBlockers := runtimeViewMemberHintCleanupPlan(memberHints, memberHintOrbits)
	candidates = append(candidates, memberCandidates...)
	driftDiagnostics = append(driftDiagnostics, memberDrift...)
	blockers = append(blockers, memberBlockers...)

	checkDrift, checkBlockers, err := runtimeViewRuntimeCheckDrift(ctx, repo.Root)
	if err != nil {
		return RuntimeViewCleanupPlanResult{}, err
	}
	driftDiagnostics = append(driftDiagnostics, checkDrift...)
	blockers = append(blockers, checkBlockers...)

	sortRuntimeViewCleanupCandidates(candidates)
	sortRuntimeViewDriftDiagnostics(driftDiagnostics)
	blockers = sortedUniqueRuntimeViewStrings(blockers)

	result := RuntimeViewCleanupPlanResult{
		Check:             check,
		Ready:             len(blockers) == 0,
		Changed:           false,
		SelectedView:      selection.View,
		CleanupCandidates: candidates,
		ChangedFiles:      []RuntimeViewCleanupChangedFile{},
		SkippedTargets:    []RuntimeViewCleanupSkippedTarget{},
		Blockers:          blockers,
		DriftDiagnostics:  driftDiagnostics,
		Notes:             []string{},
		Runtime: RuntimeViewRuntimeSummary{
			HarnessID:   runtimeFile.Harness.ID,
			MemberIDs:   runtimeViewMemberIDs(runtimeFile.Members),
			MemberCount: len(runtimeFile.Members),
		},
	}
	result.NextActions = runtimeViewCleanupNextActions(result)

	return result, nil
}

func runtimeViewCleanupGuidanceTargets() []runtimeViewCleanupGuidanceTarget {
	return []runtimeViewCleanupGuidanceTarget{
		{target: "agents", path: "AGENTS.md"},
		{target: "humans", path: "HUMANS.md"},
		{target: "bootstrap", path: "BOOTSTRAP.md"},
	}
}

func inspectRuntimeViewGuidanceCleanup(
	ctx context.Context,
	repoRoot string,
	members []RuntimeMember,
) ([]RuntimeViewCleanupCandidate, []RuntimeViewDriftDiagnostic, []string) {
	candidates := make([]RuntimeViewCleanupCandidate, 0)
	diagnostics := make([]RuntimeViewDriftDiagnostic, 0)
	blockers := make([]string, 0)

	for _, target := range runtimeViewCleanupGuidanceTargets() {
		targetCandidates, targetDiagnostics, targetBlockers := inspectRuntimeViewGuidanceTargetCleanup(repoRoot, target)
		candidates = append(candidates, targetCandidates...)
		diagnostics = append(diagnostics, targetDiagnostics...)
		blockers = append(blockers, targetBlockers...)
	}

	for _, member := range members {
		memberDiagnostics, memberBlockers := inspectRuntimeViewGuidanceDrift(ctx, repoRoot, member.OrbitID)
		diagnostics = append(diagnostics, memberDiagnostics...)
		blockers = append(blockers, memberBlockers...)
	}

	return candidates, diagnostics, blockers
}

func applyRuntimeViewRootGuidanceCleanup(
	repoRoot string,
) ([]RuntimeViewCleanupChangedFile, []RuntimeViewCleanupSkippedTarget, error) {
	changedFiles := make([]RuntimeViewCleanupChangedFile, 0, 3)
	skippedTargets := make([]RuntimeViewCleanupSkippedTarget, 0, 3)

	for _, target := range runtimeViewCleanupGuidanceTargets() {
		filename := filepath.Join(repoRoot, filepath.FromSlash(target.path))
		data, err := os.ReadFile(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				skippedTargets = append(skippedTargets, RuntimeViewCleanupSkippedTarget{
					Path:   target.path,
					Target: target.target,
					Reason: RuntimeViewCleanupSkippedMissing,
				})
				continue
			}
			return nil, nil, fmt.Errorf("read %s: %w", target.path, err)
		}

		stripped, blockCount, err := orbittemplate.StripRuntimeAgentsMarkerLinesData(data)
		if err != nil {
			return nil, nil, fmt.Errorf("strip %s root guidance marker lines: %w", target.path, err)
		}
		if blockCount == 0 {
			skippedTargets = append(skippedTargets, RuntimeViewCleanupSkippedTarget{
				Path:   target.path,
				Target: target.target,
				Reason: RuntimeViewCleanupSkippedNoMarkerLines,
			})
			continue
		}

		fileInfo, err := os.Stat(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("stat %s: %w", target.path, err)
		}
		if err := contractutil.AtomicWriteFileMode(filename, stripped, fileInfo.Mode().Perm()); err != nil {
			return nil, nil, fmt.Errorf("write %s: %w", target.path, err)
		}
		changedFiles = append(changedFiles, RuntimeViewCleanupChangedFile{
			Path:       target.path,
			Target:     target.target,
			Action:     RuntimeViewCleanupActionStripMarkerLinesPreserveContent,
			BlockCount: blockCount,
		})
	}

	sortRuntimeViewCleanupChangedFiles(changedFiles)
	sortRuntimeViewCleanupSkippedTargets(skippedTargets)

	return changedFiles, skippedTargets, nil
}

func applyRuntimeViewMemberHintCleanup(
	repoRoot string,
	candidates []RuntimeViewCleanupCandidate,
) ([]RuntimeViewCleanupChangedFile, error) {
	hintPaths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Kind != RuntimeViewCleanupCandidateMemberHint ||
			candidate.Action != RuntimeViewCleanupActionRemoveConsumedHint {
			continue
		}
		hintPaths = append(hintPaths, candidate.Path)
	}
	if len(hintPaths) == 0 {
		return []RuntimeViewCleanupChangedFile{}, nil
	}

	consumedHints, err := orbitpkg.ConsumeMemberHintPaths(repoRoot, hintPaths)
	if err != nil {
		return nil, fmt.Errorf("consume member hints: %w", err)
	}

	changedFiles := make([]RuntimeViewCleanupChangedFile, 0, len(consumedHints))
	for _, effect := range consumedHints {
		changedFiles = append(changedFiles, RuntimeViewCleanupChangedFile{
			Path:              effect.Path,
			Action:            RuntimeViewCleanupActionRemoveConsumedHint,
			PreservedMetadata: effect.PreservedMetadata,
		})
	}
	sortRuntimeViewCleanupChangedFiles(changedFiles)

	return changedFiles, nil
}

func inspectRuntimeViewGuidanceTargetCleanup(
	repoRoot string,
	target runtimeViewCleanupGuidanceTarget,
) ([]RuntimeViewCleanupCandidate, []RuntimeViewDriftDiagnostic, []string) {
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(target.path)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		message := fmt.Sprintf("read %s: %v", target.path, err)
		return nil, []RuntimeViewDriftDiagnostic{{
			Kind:            RuntimeViewDriftKindRootGuidanceSyntax,
			Path:            target.path,
			Target:          target.target,
			Message:         message,
			RecoveryCommand: "hyard check --json",
		}}, []string{message}
	}

	document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
	if err != nil {
		message := fmt.Sprintf("%s: %s", target.path, err.Error())
		return nil, []RuntimeViewDriftDiagnostic{{
			Kind:            RuntimeViewDriftKindRootGuidanceSyntax,
			Path:            target.path,
			Target:          target.target,
			Message:         err.Error(),
			RecoveryCommand: "hyard check --json",
		}}, []string{message}
	}

	candidates := make([]RuntimeViewCleanupCandidate, 0)
	for _, segment := range document.Segments {
		if segment.Kind != orbittemplate.AgentsRuntimeSegmentBlock {
			continue
		}
		candidate := RuntimeViewCleanupCandidate{
			Kind:       RuntimeViewCleanupCandidateRootGuidanceMarkerLines,
			Path:       target.path,
			Target:     target.target,
			OwnerKind:  string(segment.OwnerKind),
			WorkflowID: segment.WorkflowID,
			Action:     RuntimeViewCleanupActionStripMarkerLinesPreserveContent,
		}
		if segment.OwnerKind == orbittemplate.OwnerKindOrbit {
			candidate.OrbitID = segment.WorkflowID
		}
		candidates = append(candidates, candidate)
	}

	return candidates, nil, nil
}

func inspectRuntimeViewGuidanceDrift(
	ctx context.Context,
	repoRoot string,
	orbitID string,
) ([]RuntimeViewDriftDiagnostic, []string) {
	diagnostics := make([]RuntimeViewDriftDiagnostic, 0, 3)
	blockers := make([]string, 0, 3)

	agents, err := orbittemplate.InspectOrbitBriefLaneForOperation(ctx, repoRoot, orbitID, "backfill")
	if err != nil {
		return []RuntimeViewDriftDiagnostic{runtimeViewGuidanceInspectionError("agents", "AGENTS.md", orbitID, err)}, []string{err.Error()}
	}
	appendRuntimeViewGuidanceDrift(&diagnostics, &blockers, "agents", "AGENTS.md", orbitID, string(agents.State), agents.HasOrbitBlock)

	humans, err := orbittemplate.InspectOrbitHumansLaneForOperation(ctx, repoRoot, orbitID, "backfill")
	if err != nil {
		return []RuntimeViewDriftDiagnostic{runtimeViewGuidanceInspectionError("humans", "HUMANS.md", orbitID, err)}, []string{err.Error()}
	}
	appendRuntimeViewGuidanceDrift(&diagnostics, &blockers, "humans", "HUMANS.md", orbitID, string(humans.State), humans.HasOrbitBlock)

	bootstrap, err := orbittemplate.InspectOrbitBootstrapLaneForOperation(ctx, repoRoot, orbitID, "backfill")
	if err != nil {
		return []RuntimeViewDriftDiagnostic{runtimeViewGuidanceInspectionError("bootstrap", "BOOTSTRAP.md", orbitID, err)}, []string{err.Error()}
	}
	appendRuntimeViewGuidanceDrift(&diagnostics, &blockers, "bootstrap", "BOOTSTRAP.md", orbitID, string(bootstrap.State), bootstrap.HasOrbitBlock)

	return diagnostics, blockers
}

func runtimeViewGuidanceInspectionError(target string, path string, orbitID string, err error) RuntimeViewDriftDiagnostic {
	return RuntimeViewDriftDiagnostic{
		Kind:            RuntimeViewDriftKindRootGuidance,
		Path:            path,
		Target:          target,
		OrbitID:         orbitID,
		Message:         err.Error(),
		RecoveryCommand: "hyard check --json",
	}
}

func appendRuntimeViewGuidanceDrift(
	diagnostics *[]RuntimeViewDriftDiagnostic,
	blockers *[]string,
	target string,
	path string,
	orbitID string,
	state string,
	hasOrbitBlock bool,
) {
	switch orbittemplate.BriefLaneState(state) {
	case orbittemplate.BriefLaneStateMaterializedDrifted:
	case orbittemplate.BriefLaneStateMissingTruth:
		if !hasOrbitBlock {
			return
		}
	case orbittemplate.BriefLaneStateInvalidContainer:
	default:
		return
	}

	recovery := "hyard guide save --orbit " + orbitID + " --target " + target
	if orbittemplate.BriefLaneState(state) == orbittemplate.BriefLaneStateInvalidContainer {
		recovery = "hyard check --json"
	}
	message := fmt.Sprintf("%s %s block %q has authored truth drift", path, target, orbitID)
	*diagnostics = append(*diagnostics, RuntimeViewDriftDiagnostic{
		Kind:            RuntimeViewDriftKindRootGuidance,
		Path:            path,
		Target:          target,
		OrbitID:         orbitID,
		State:           state,
		Message:         message,
		RecoveryCommand: recovery,
	})
	*blockers = append(*blockers, message+"; run `"+recovery+"`")
}

func runtimeViewMemberHintCleanupPlan(
	summary RuntimeViewMemberHintSummary,
	orbits []RuntimeViewMemberHintOrbitInfo,
) ([]RuntimeViewCleanupCandidate, []RuntimeViewDriftDiagnostic, []string) {
	candidates := make([]RuntimeViewCleanupCandidate, 0, summary.HintCount)
	diagnostics := make([]RuntimeViewDriftDiagnostic, 0)
	blockers := make([]string, 0)

	for _, orbit := range orbits {
		for _, hint := range orbit.Hints {
			switch hint.Action {
			case orbitpkg.MemberHintActionMatchExisting, orbitpkg.MemberHintActionMergeExisting:
				if orbit.DriftDetected {
					recovery := "hyard orbit content apply " + orbit.OrbitID + " --check --json"
					message := fmt.Sprintf("%s %s has pending member hint drift", orbit.OrbitID, hint.HintPath)
					diagnostics = append(diagnostics, RuntimeViewDriftDiagnostic{
						Kind:            RuntimeViewDriftKindMemberHint,
						Path:            hint.HintPath,
						OrbitID:         orbit.OrbitID,
						State:           hint.Action,
						Message:         message,
						RecoveryCommand: recovery,
					})
					blockers = append(blockers, message+"; run `"+recovery+"`")
					continue
				}
				candidates = append(candidates, RuntimeViewCleanupCandidate{
					Kind:    RuntimeViewCleanupCandidateMemberHint,
					Path:    hint.HintPath,
					OrbitID: orbit.OrbitID,
					Action:  RuntimeViewCleanupActionRemoveConsumedHint,
				})
			default:
				recovery := "hyard orbit content apply " + orbit.OrbitID + " --check --json"
				message := runtimeViewMemberHintDriftMessage(orbit.OrbitID, hint)
				diagnostics = append(diagnostics, RuntimeViewDriftDiagnostic{
					Kind:            RuntimeViewDriftKindMemberHint,
					Path:            hint.HintPath,
					OrbitID:         orbit.OrbitID,
					State:           hint.Action,
					Message:         message,
					RecoveryCommand: recovery,
				})
				blockers = append(blockers, message+"; run `"+recovery+"`")
			}
		}
	}

	return candidates, diagnostics, blockers
}

func runtimeViewMemberHintDriftMessage(orbitID string, hint orbitpkg.DetectedMemberHint) string {
	if len(hint.Diagnostics) > 0 {
		return fmt.Sprintf("%s %s: %s", orbitID, hint.HintPath, hint.Diagnostics[0])
	}
	return fmt.Sprintf("%s %s has unapplied member hint action %q", orbitID, hint.HintPath, hint.Action)
}

func runtimeViewRuntimeCheckDrift(ctx context.Context, repoRoot string) ([]RuntimeViewDriftDiagnostic, []string, error) {
	check, err := CheckRuntime(ctx, repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("run harness check: %w", err)
	}

	diagnostics := make([]RuntimeViewDriftDiagnostic, 0)
	blockers := make([]string, 0)
	for _, finding := range check.Findings {
		if !runtimeViewCheckFindingIsDrift(finding) {
			continue
		}
		diagnostics = append(diagnostics, RuntimeViewDriftDiagnostic{
			Kind:            RuntimeViewDriftKindRuntimeCheck,
			Path:            finding.Path,
			OrbitID:         finding.OrbitID,
			State:           string(finding.Kind),
			Message:         finding.Message,
			RecoveryCommand: "hyard check --json",
		})
		blockers = append(blockers, finding.Message+"; run `hyard check --json`")
	}

	return diagnostics, blockers, nil
}

func runtimeViewCheckFindingIsDrift(finding CheckFinding) bool {
	switch string(finding.Kind) {
	case string(orbittemplate.DriftKindDefinition),
		string(orbittemplate.DriftKindRuntimeFile),
		string(orbittemplate.DriftKindProvenanceUnresolvable):
		return true
	default:
		return false
	}
}

func runtimeViewCleanupNextActions(result RuntimeViewCleanupPlanResult) []string {
	if len(result.Blockers) > 0 {
		actions := make([]string, 0, len(result.DriftDiagnostics)+1)
		for _, diagnostic := range result.DriftDiagnostics {
			if diagnostic.RecoveryCommand == "" {
				continue
			}
			actions = append(actions, diagnostic.RecoveryCommand)
		}
		if len(actions) == 0 {
			actions = append(actions, "hyard check --json")
		}
		return sortedUniqueRuntimeViewStrings(actions)
	}

	if !result.Check {
		if len(result.ChangedFiles) > 0 {
			return []string{"review cleaned Run View files before publishing"}
		}
		return []string{"Run View cleanup is already clean"}
	}

	if len(result.CleanupCandidates) == 0 {
		return []string{"Run View cleanup is already clean"}
	}

	return []string{"run `hyard view run` to apply Run View cleanup"}
}

func runtimeViewCleanupNotes(result RuntimeViewCleanupPlanResult) []string {
	if result.Check {
		return []string{}
	}
	for _, changedFile := range result.ChangedFiles {
		if changedFile.Action == RuntimeViewCleanupActionStripMarkerLinesPreserveContent {
			return []string{runtimeViewCleanupPresentationNote}
		}
	}

	return []string{}
}

func sortRuntimeViewCleanupCandidates(candidates []RuntimeViewCleanupCandidate) {
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].Kind != candidates[right].Kind {
			return candidates[left].Kind < candidates[right].Kind
		}
		if candidates[left].Path != candidates[right].Path {
			return candidates[left].Path < candidates[right].Path
		}
		if candidates[left].Target != candidates[right].Target {
			return candidates[left].Target < candidates[right].Target
		}
		if candidates[left].OrbitID != candidates[right].OrbitID {
			return candidates[left].OrbitID < candidates[right].OrbitID
		}
		return candidates[left].WorkflowID < candidates[right].WorkflowID
	})
}

func sortRuntimeViewCleanupChangedFiles(changedFiles []RuntimeViewCleanupChangedFile) {
	sort.Slice(changedFiles, func(left, right int) bool {
		if changedFiles[left].Path != changedFiles[right].Path {
			return changedFiles[left].Path < changedFiles[right].Path
		}
		return changedFiles[left].Target < changedFiles[right].Target
	})
}

func sortRuntimeViewCleanupSkippedTargets(skippedTargets []RuntimeViewCleanupSkippedTarget) {
	sort.Slice(skippedTargets, func(left, right int) bool {
		if skippedTargets[left].Path != skippedTargets[right].Path {
			return skippedTargets[left].Path < skippedTargets[right].Path
		}
		return skippedTargets[left].Target < skippedTargets[right].Target
	})
}

func sortRuntimeViewDriftDiagnostics(diagnostics []RuntimeViewDriftDiagnostic) {
	sort.Slice(diagnostics, func(left, right int) bool {
		if diagnostics[left].Kind != diagnostics[right].Kind {
			return diagnostics[left].Kind < diagnostics[right].Kind
		}
		if diagnostics[left].Path != diagnostics[right].Path {
			return diagnostics[left].Path < diagnostics[right].Path
		}
		if diagnostics[left].Target != diagnostics[right].Target {
			return diagnostics[left].Target < diagnostics[right].Target
		}
		return diagnostics[left].OrbitID < diagnostics[right].OrbitID
	})
}

func sortedUniqueRuntimeViewStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
