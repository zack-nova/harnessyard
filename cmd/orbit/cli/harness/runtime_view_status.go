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
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	RuntimeViewPresentationRuntimeContent     = "runtime_content"
	RuntimeViewPresentationAuthoringScaffolds = "authoring_scaffolds"

	PublicationActionCurrentRuntimeHarnessPackage = "current_runtime_harness_package"
	PublicationActionOrbitPackage                 = "orbit_package"
)

// RuntimeViewStatusResult describes the current Runtime View selection and the
// visible worktree presentation.
type RuntimeViewStatusResult struct {
	SelectedView              statepkg.RuntimeView             `json:"selected_view"`
	SelectionPersisted        bool                             `json:"selection_persisted"`
	ActualPresentation        RuntimeViewPresentation          `json:"actual_presentation"`
	GuidanceMarkers           []RuntimeViewGuidanceMarker      `json:"guidance_markers"`
	MemberHints               RuntimeViewMemberHintSummary     `json:"member_hints"`
	DriftBlockers             []string                         `json:"drift_blockers"`
	AllowedPublicationActions []string                         `json:"allowed_publication_actions"`
	NextActions               []string                         `json:"next_actions"`
	Runtime                   RuntimeViewRuntimeSummary        `json:"runtime"`
	MemberHintOrbits          []RuntimeViewMemberHintOrbitInfo `json:"member_hint_orbits,omitempty"`
}

// RuntimeViewRuntimeSummary identifies the current Harness Runtime.
type RuntimeViewRuntimeSummary struct {
	HarnessID   string   `json:"harness_id"`
	MemberIDs   []string `json:"member_ids"`
	MemberCount int      `json:"member_count"`
}

// RuntimeViewPresentation reports what the worktree currently looks like,
// independent from the selected Runtime View.
type RuntimeViewPresentation struct {
	Mode                       string `json:"mode"`
	AuthoringScaffoldsPresent  bool   `json:"authoring_scaffolds_present"`
	GuidanceMarkersPresent     bool   `json:"guidance_markers_present"`
	MemberHintsPresent         bool   `json:"member_hints_present"`
	CurrentOrbit               string `json:"current_orbit,omitempty"`
	CurrentOrbitSparseCheckout bool   `json:"current_orbit_sparse_checkout,omitempty"`
}

// RuntimeViewGuidanceMarker summarizes root guidance marker presence for one target.
type RuntimeViewGuidanceMarker struct {
	Target            string `json:"target"`
	Path              string `json:"path"`
	Present           bool   `json:"present"`
	BlockCount        int    `json:"block_count"`
	OrbitBlockCount   int    `json:"orbit_block_count"`
	HarnessBlockCount int    `json:"harness_block_count"`
	ParseError        string `json:"parse_error,omitempty"`
}

// RuntimeViewMemberHintSummary aggregates existing Member Hint inspection concepts.
type RuntimeViewMemberHintSummary struct {
	HintCount       int      `json:"hint_count"`
	DriftDetected   bool     `json:"drift_detected"`
	BackfillAllowed bool     `json:"backfill_allowed"`
	BlockerCount    int      `json:"blocker_count"`
	Blockers        []string `json:"blockers"`
}

// RuntimeViewMemberHintOrbitInfo reports per-orbit Member Hint status.
type RuntimeViewMemberHintOrbitInfo struct {
	OrbitID         string                        `json:"orbit_id"`
	HintCount       int                           `json:"hint_count"`
	DriftDetected   bool                          `json:"drift_detected"`
	BackfillAllowed bool                          `json:"backfill_allowed"`
	Hints           []orbitpkg.DetectedMemberHint `json:"hints,omitempty"`
}

type runtimeViewGuidanceTarget struct {
	target string
	path   string
}

// RuntimeViewStatus inspects Runtime View selection and visible authoring scaffolds.
func RuntimeViewStatus(ctx context.Context, repo gitpkg.Repo, store statepkg.FSStore) (RuntimeViewStatusResult, error) {
	selection, err := store.ReadRuntimeViewSelection()
	if err != nil {
		return RuntimeViewStatusResult{}, fmt.Errorf("read runtime view selection: %w", err)
	}

	runtimeFile, err := LoadRuntimeFile(repo.Root)
	if err != nil {
		return RuntimeViewStatusResult{}, fmt.Errorf("load harness runtime: %w", err)
	}

	currentOrbit, sparseCheckout, err := currentOrbitPresentation(store)
	if err != nil {
		return RuntimeViewStatusResult{}, err
	}

	guidanceMarkers := inspectRuntimeViewGuidanceMarkers(repo.Root)
	memberHints, memberHintOrbits, err := inspectRuntimeViewMemberHints(ctx, repo.Root, runtimeFile.Members)
	if err != nil {
		return RuntimeViewStatusResult{}, err
	}

	guidanceMarkersPresent := runtimeViewGuidanceMarkersPresent(guidanceMarkers)
	memberHintsPresent := memberHints.HintCount > 0
	authoringScaffoldsPresent := guidanceMarkersPresent || memberHintsPresent
	presentationMode := RuntimeViewPresentationRuntimeContent
	if authoringScaffoldsPresent {
		presentationMode = RuntimeViewPresentationAuthoringScaffolds
	}

	driftBlockers := runtimeViewDriftBlockers(guidanceMarkers, memberHints)

	result := RuntimeViewStatusResult{
		SelectedView:       selection.View,
		SelectionPersisted: selection.Persisted,
		ActualPresentation: RuntimeViewPresentation{
			Mode:                       presentationMode,
			AuthoringScaffoldsPresent:  authoringScaffoldsPresent,
			GuidanceMarkersPresent:     guidanceMarkersPresent,
			MemberHintsPresent:         memberHintsPresent,
			CurrentOrbit:               currentOrbit,
			CurrentOrbitSparseCheckout: sparseCheckout,
		},
		GuidanceMarkers: guidanceMarkers,
		MemberHints:     memberHints,
		DriftBlockers:   driftBlockers,
		AllowedPublicationActions: runtimeViewAllowedPublicationActions(
			selection.View,
		),
		Runtime: RuntimeViewRuntimeSummary{
			HarnessID:   runtimeFile.Harness.ID,
			MemberIDs:   runtimeViewMemberIDs(runtimeFile.Members),
			MemberCount: len(runtimeFile.Members),
		},
		MemberHintOrbits: memberHintOrbits,
	}
	result.NextActions = runtimeViewNextActions(result)

	return result, nil
}

func currentOrbitPresentation(store statepkg.FSStore) (string, bool, error) {
	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read current orbit state: %w", err)
	}

	return current.Orbit, current.SparseEnabled, nil
}

func inspectRuntimeViewGuidanceMarkers(repoRoot string) []RuntimeViewGuidanceMarker {
	targets := []runtimeViewGuidanceTarget{
		{target: "agents", path: "AGENTS.md"},
		{target: "humans", path: "HUMANS.md"},
		{target: "bootstrap", path: "BOOTSTRAP.md"},
	}
	results := make([]RuntimeViewGuidanceMarker, 0, len(targets))

	for _, target := range targets {
		result := RuntimeViewGuidanceMarker{
			Target: target.target,
			Path:   target.path,
		}

		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(target.path)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				results = append(results, result)
				continue
			}
			result.ParseError = fmt.Sprintf("read %s: %v", target.path, err)
			results = append(results, result)
			continue
		}

		document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
		if err != nil {
			result.Present = true
			result.ParseError = err.Error()
			results = append(results, result)
			continue
		}

		for _, segment := range document.Segments {
			if segment.Kind != orbittemplate.AgentsRuntimeSegmentBlock {
				continue
			}
			result.BlockCount++
			switch segment.OwnerKind {
			case orbittemplate.OwnerKindOrbit:
				result.OrbitBlockCount++
			case orbittemplate.OwnerKindHarness:
				result.HarnessBlockCount++
			}
		}
		result.Present = result.BlockCount > 0
		results = append(results, result)
	}

	return results
}

func inspectRuntimeViewMemberHints(
	ctx context.Context,
	repoRoot string,
	members []RuntimeMember,
) (RuntimeViewMemberHintSummary, []RuntimeViewMemberHintOrbitInfo, error) {
	if len(members) == 0 {
		return RuntimeViewMemberHintSummary{
			BackfillAllowed: true,
			Blockers:        []string{},
		}, []RuntimeViewMemberHintOrbitInfo{}, nil
	}

	worktreeFiles, err := gitpkg.WorktreeFiles(ctx, repoRoot)
	if err != nil {
		return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("list worktree files: %w", err)
	}
	config, err := orbitpkg.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("load runtime repository config: %w", err)
	}

	summary := RuntimeViewMemberHintSummary{
		BackfillAllowed: true,
		Blockers:        []string{},
	}
	orbits := make([]RuntimeViewMemberHintOrbitInfo, 0, len(members))

	for _, member := range members {
		definition, found := config.OrbitByID(member.OrbitID)
		if !found {
			return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("runtime member %q is missing hosted definition", member.OrbitID)
		}
		spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, member.OrbitID)
		if err != nil {
			return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("load hosted orbit spec %q: %w", member.OrbitID, err)
		}
		candidateFiles, err := runtimeViewMemberHintCandidateFiles(config.Global, definition, worktreeFiles)
		if err != nil {
			return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("filter member hint files for orbit %q: %w", member.OrbitID, err)
		}

		inspection, err := orbitpkg.InspectMemberHints(repoRoot, spec, candidateFiles)
		if err != nil {
			return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("inspect member hints for orbit %q: %w", member.OrbitID, err)
		}
		ambiguousHints, err := orbitpkg.InspectAmbiguousFlatMemberHints(repoRoot, candidateFiles)
		if err != nil {
			return RuntimeViewMemberHintSummary{}, nil, fmt.Errorf("inspect ambiguous flat member hints for orbit %q: %w", member.OrbitID, err)
		}
		if len(ambiguousHints) > 0 {
			inspection.Hints = append(inspection.Hints, ambiguousHints...)
			sort.Slice(inspection.Hints, func(left, right int) bool {
				if inspection.Hints[left].HintPath == inspection.Hints[right].HintPath {
					return inspection.Hints[left].Kind < inspection.Hints[right].Kind
				}
				return inspection.Hints[left].HintPath < inspection.Hints[right].HintPath
			})
			inspection.DriftDetected = true
			inspection.BackfillAllowed = false
		}

		summary.HintCount += len(inspection.Hints)
		summary.DriftDetected = summary.DriftDetected || inspection.DriftDetected
		summary.BackfillAllowed = summary.BackfillAllowed && inspection.BackfillAllowed
		for _, hint := range inspection.Hints {
			if !runtimeViewMemberHintBlocksCleanup(hint) {
				continue
			}
			blocker := runtimeViewMemberHintBlocker(member.OrbitID, hint)
			summary.Blockers = append(summary.Blockers, blocker)
		}

		orbits = append(orbits, RuntimeViewMemberHintOrbitInfo{
			OrbitID:         member.OrbitID,
			HintCount:       len(inspection.Hints),
			DriftDetected:   inspection.DriftDetected,
			BackfillAllowed: inspection.BackfillAllowed,
			Hints:           inspection.Hints,
		})
	}

	summary.BlockerCount = len(summary.Blockers)

	return summary, orbits, nil
}

func runtimeViewMemberHintCandidateFiles(
	config orbitpkg.GlobalConfig,
	definition orbitpkg.Definition,
	worktreeFiles []string,
) ([]string, error) {
	candidateFiles := make([]string, 0, len(worktreeFiles))
	for _, worktreeFile := range worktreeFiles {
		matches, err := orbitpkg.PathMatchesOrbit(config, definition, worktreeFile)
		if err != nil {
			return nil, fmt.Errorf("match %q: %w", worktreeFile, err)
		}
		if !matches {
			continue
		}
		candidateFiles = append(candidateFiles, worktreeFile)
	}

	return candidateFiles, nil
}

func runtimeViewMemberHintBlocksCleanup(hint orbitpkg.DetectedMemberHint) bool {
	return hint.Action == orbitpkg.MemberHintActionInvalidHint || hint.Action == orbitpkg.MemberHintActionConflict
}

func runtimeViewMemberHintBlocker(orbitID string, hint orbitpkg.DetectedMemberHint) string {
	detail := strings.Join(hint.Diagnostics, "; ")
	if detail == "" {
		detail = hint.Action
	}

	return fmt.Sprintf("%s %s: %s", orbitID, hint.HintPath, detail)
}

func runtimeViewGuidanceMarkersPresent(markers []RuntimeViewGuidanceMarker) bool {
	for _, marker := range markers {
		if marker.Present {
			return true
		}
	}

	return false
}

func runtimeViewDriftBlockers(markers []RuntimeViewGuidanceMarker, hints RuntimeViewMemberHintSummary) []string {
	blockers := make([]string, 0, hints.BlockerCount+len(markers))
	blockers = append(blockers, hints.Blockers...)
	for _, marker := range markers {
		if marker.ParseError == "" {
			continue
		}
		blockers = append(blockers, fmt.Sprintf("%s: %s", marker.Path, marker.ParseError))
	}

	return blockers
}

func runtimeViewAllowedPublicationActions(view statepkg.RuntimeView) []string {
	switch view {
	case statepkg.RuntimeViewAuthor:
		return []string{
			PublicationActionOrbitPackage,
			PublicationActionCurrentRuntimeHarnessPackage,
		}
	default:
		return []string{PublicationActionCurrentRuntimeHarnessPackage}
	}
}

func runtimeViewNextActions(result RuntimeViewStatusResult) []string {
	actions := make([]string, 0, 4)
	if len(result.DriftBlockers) > 0 {
		actions = append(actions, "resolve authored truth blockers")
	}

	switch result.SelectedView {
	case statepkg.RuntimeViewAuthor:
		actions = append(actions, "publish an Orbit Package")
		actions = append(actions, "publish current runtime as a Harness Package")
	default:
		actions = append(actions, "publish current runtime as a Harness Package")
		if result.ActualPresentation.AuthoringScaffoldsPresent {
			actions = append(actions, "review visible authoring scaffolds before publishing")
		}
	}

	return actions
}

func runtimeViewMemberIDs(members []RuntimeMember) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.OrbitID)
	}

	return ids
}
