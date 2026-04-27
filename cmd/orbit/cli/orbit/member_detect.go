package orbit

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	MemberHintActionMatchExisting = "match_existing"
	MemberHintActionMergeExisting = "merge_existing"
	MemberHintActionAppendInclude = "append_include"
	MemberHintActionCreateNew     = "create_new"
	MemberHintActionConflict      = "conflict"
	MemberHintActionInvalidHint   = "invalid_hint"
)

// DetectedMemberHint is the shared detect/check result model for one parsed hint.
type DetectedMemberHint struct {
	Kind         string   `json:"kind"`
	HintPath     string   `json:"hint_path"`
	RootPath     string   `json:"root_path"`
	ResolvedName string   `json:"resolved_name,omitempty"`
	ResolvedRole string   `json:"resolved_role,omitempty"`
	Action       string   `json:"action"`
	TargetName   string   `json:"target_name,omitempty"`
	Diagnostics  []string `json:"diagnostics,omitempty"`
}

// MemberHintInspection captures one read-only detect/check pass.
type MemberHintInspection struct {
	Hints           []DetectedMemberHint `json:"hints"`
	DriftDetected   bool                 `json:"drift_detected"`
	BackfillAllowed bool                 `json:"backfill_allowed"`
}

// InspectMemberHints scans candidate files, resolves member hints, and classifies
// how backfill would treat each one without mutating any files.
func InspectMemberHints(repoRoot string, spec OrbitSpec, candidateFiles []string) (MemberHintInspection, error) {
	normalizedSpec, err := normalizeOrbitSpecMemberIdentities(spec)
	if err != nil {
		return MemberHintInspection{}, fmt.Errorf("normalize orbit spec: %w", err)
	}

	filteredCandidateFiles, err := filterMemberHintCandidateFiles(normalizedSpec, candidateFiles)
	if err != nil {
		return MemberHintInspection{}, fmt.Errorf("filter member hint candidates: %w", err)
	}

	invalidHints, validHints, err := scanResolvedMemberHints(repoRoot, filteredCandidateFiles)
	if err != nil {
		return MemberHintInspection{}, err
	}

	existingMembers := make(map[string]OrbitMember, len(normalizedSpec.Members))
	for _, member := range normalizedSpec.Members {
		existingMembers[orbitMemberIdentityName(member)] = member
	}

	duplicateDiagnostics := duplicateMemberHintDiagnostics(validHints)
	results := make([]DetectedMemberHint, 0, len(invalidHints)+len(validHints))
	results = append(results, invalidHints...)

	driftDetected := len(invalidHints) > 0
	backfillAllowed := len(invalidHints) == 0

	for _, hint := range validHints {
		if diagnostics, duplicate := duplicateDiagnostics[hint.Name]; duplicate {
			results = append(results, detectedMemberHintFromResolved(
				hint,
				MemberHintActionConflict,
				"",
				diagnostics,
			))
			driftDetected = true
			backfillAllowed = false
			continue
		}

		candidate := buildMemberHintCandidate(hint)
		existingMember, found := existingMembers[hint.Name]
		if !found {
			results = append(results, detectedMemberHintFromResolved(
				hint,
				MemberHintActionCreateNew,
				"",
				nil,
			))
			driftDetected = true
			continue
		}

		classification, err := classifyExistingMemberHint(hint, existingMember, candidate.Member)
		if err != nil {
			return MemberHintInspection{}, err
		}
		results = append(results, detectedMemberHintFromResolved(
			hint,
			classification.action,
			orbitMemberIdentityName(existingMember),
			classification.diagnostics,
		))
		if classification.driftDetected {
			driftDetected = true
		}
		if !classification.backfillAllowed {
			backfillAllowed = false
		}
	}

	sort.Slice(results, func(left, right int) bool {
		if results[left].HintPath == results[right].HintPath {
			return results[left].Kind < results[right].Kind
		}
		return results[left].HintPath < results[right].HintPath
	})

	return MemberHintInspection{
		Hints:           results,
		DriftDetected:   driftDetected,
		BackfillAllowed: backfillAllowed,
	}, nil
}

func scanResolvedMemberHints(repoRoot string, candidateFiles []string) ([]DetectedMemberHint, []resolvedMemberHint, error) {
	invalidHints := make([]DetectedMemberHint, 0)
	validHints := make([]resolvedMemberHint, 0)
	for _, candidateFile := range candidateFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(candidateFile)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize member hint candidate file %q: %w", candidateFile, err)
		}

		switch {
		case path.Base(normalizedPath) == memberHintMarkerFileName:
			hint, invalid, err := inspectDirectoryMemberHintFile(repoRoot, normalizedPath)
			if err != nil {
				return nil, nil, err
			}
			if invalid != nil {
				invalidHints = append(invalidHints, *invalid)
				continue
			}
			validHints = append(validHints, hint)
		case path.Ext(normalizedPath) == ".md":
			hint, found, invalid, err := inspectMarkdownMemberHintFile(repoRoot, normalizedPath)
			if err != nil {
				return nil, nil, err
			}
			if invalid != nil {
				invalidHints = append(invalidHints, *invalid)
				continue
			}
			if found {
				validHints = append(validHints, hint)
			}
		}
	}

	return invalidHints, validHints, nil
}

func inspectMarkdownMemberHintFile(repoRoot string, hintPath string) (resolvedMemberHint, bool, *DetectedMemberHint, error) {
	//nolint:gosec // The path is repo-local and built from normalized tracked markdown files.
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(hintPath)))
	if err != nil {
		return resolvedMemberHint{}, false, nil, fmt.Errorf("read member hint %q: %w", hintPath, err)
	}

	hint, found, err := parseMarkdownMemberHint(hintPath, data)
	if err != nil {
		invalid := detectedInvalidMemberHint(memberHintKindFileFrontmatter, hintPath, hintPath, err)
		return resolvedMemberHint{}, false, &invalid, nil
	}

	return hint, found, nil, nil
}

func inspectDirectoryMemberHintFile(repoRoot string, markerPath string) (resolvedMemberHint, *DetectedMemberHint, error) {
	//nolint:gosec // The path is repo-local and built from normalized tracked marker files.
	data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(markerPath)))
	if err != nil {
		return resolvedMemberHint{}, nil, fmt.Errorf("read member marker %q: %w", markerPath, err)
	}

	hint, err := parseDirectoryMemberHint(markerPath, data)
	if err != nil {
		rootPath := path.Dir(markerPath)
		invalid := detectedInvalidMemberHint(memberHintKindDirectoryMarker, markerPath, rootPath, err)
		return resolvedMemberHint{}, &invalid, nil
	}

	return hint, nil, nil
}

func detectedInvalidMemberHint(kind string, hintPath string, rootPath string, err error) DetectedMemberHint {
	return DetectedMemberHint{
		Kind:        kind,
		HintPath:    hintPath,
		RootPath:    rootPath,
		Action:      MemberHintActionInvalidHint,
		Diagnostics: []string{err.Error()},
	}
}

func filterMemberHintCandidateFiles(spec OrbitSpec, candidateFiles []string) ([]string, error) {
	filtered := make([]string, 0, len(candidateFiles))
	seen := make(map[string]struct{}, len(candidateFiles))
	for _, candidateFile := range candidateFiles {
		normalizedPath, err := ids.NormalizeRepoRelativePath(candidateFile)
		if err != nil {
			return nil, fmt.Errorf("normalize member hint candidate file %q: %w", candidateFile, err)
		}

		excluded, err := memberHintCandidateExcluded(spec, normalizedPath)
		if err != nil {
			return nil, err
		}
		if excluded {
			continue
		}
		if _, ok := seen[normalizedPath]; ok {
			continue
		}
		seen[normalizedPath] = struct{}{}
		filtered = append(filtered, normalizedPath)
	}

	sort.Strings(filtered)
	return filtered, nil
}

func memberHintCandidateExcluded(spec OrbitSpec, normalizedPath string) (bool, error) {
	switch {
	case normalizedPath == ".harness" || strings.HasPrefix(normalizedPath, ".harness/"):
		return true, nil
	case normalizedPath == "AGENTS.md" || normalizedPath == "HUMANS.md" || normalizedPath == "BOOTSTRAP.md":
		return true, nil
	case path.Base(normalizedPath) == "SKILL.md":
		return true, nil
	case strings.HasPrefix(normalizedPath, "commands/") || strings.HasPrefix(normalizedPath, "skills/"):
		return true, nil
	}

	inCapabilityOverlay, err := pathInCapabilityOverlay(spec, normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match capability-owned member hint candidate %q: %w", normalizedPath, err)
	}
	return inCapabilityOverlay, nil
}

func detectedMemberHintFromResolved(
	hint resolvedMemberHint,
	action string,
	targetName string,
	diagnostics []string,
) DetectedMemberHint {
	return DetectedMemberHint{
		Kind:         hint.Kind,
		HintPath:     hint.HintPath,
		RootPath:     hint.RootPath,
		ResolvedName: hint.Name,
		ResolvedRole: string(hint.Role),
		Action:       action,
		TargetName:   targetName,
		Diagnostics:  append([]string(nil), diagnostics...),
	}
}

func duplicateMemberHintDiagnostics(hints []resolvedMemberHint) map[string][]string {
	pathsByName := make(map[string][]string)
	for _, hint := range hints {
		pathsByName[hint.Name] = append(pathsByName[hint.Name], hint.HintPath)
	}

	diagnostics := make(map[string][]string)
	for name, paths := range pathsByName {
		if len(paths) < 2 {
			continue
		}

		sortedPaths := append([]string(nil), paths...)
		sort.Strings(sortedPaths)
		diagnostics[name] = []string{
			fmt.Sprintf("resolved member name %q is declared by multiple hints: %s", name, strings.Join(sortedPaths, ", ")),
		}
	}

	return diagnostics
}

type existingMemberHintClassification struct {
	action          string
	diagnostics     []string
	driftDetected   bool
	backfillAllowed bool
}

func classifyExistingMemberHint(
	hint resolvedMemberHint,
	existing OrbitMember,
	candidate OrbitMember,
) (existingMemberHintClassification, error) {
	if isHintManageableMember(existing) {
		return existingMemberHintClassification{
			action:          MemberHintActionMatchExisting,
			driftDetected:   !memberMatchesHintCandidate(existing, candidate),
			backfillAllowed: true,
		}, nil
	}

	targetName := orbitMemberIdentityName(existing)
	excluded, err := memberHintMatchesPatterns(existing.Paths.Exclude, hint, candidate.Paths.Include[0])
	if err != nil {
		return existingMemberHintClassification{}, fmt.Errorf("match member hint against existing excludes: %w", err)
	}
	if excluded {
		return existingMemberHintClassification{
			action: MemberHintActionConflict,
			diagnostics: []string{
				fmt.Sprintf("member hint path %q is excluded by existing member %q", hint.RootPath, targetName),
			},
			driftDetected:   true,
			backfillAllowed: false,
		}, nil
	}

	included, err := memberHintMatchesPatterns(existing.Paths.Include, hint, candidate.Paths.Include[0])
	if err != nil {
		return existingMemberHintClassification{}, fmt.Errorf("match member hint against existing includes: %w", err)
	}
	if included {
		return existingMemberHintClassification{
			action:          MemberHintActionMergeExisting,
			driftDetected:   !memberNonPathFieldsMatchHintCandidate(existing, candidate),
			backfillAllowed: true,
		}, nil
	}

	return existingMemberHintClassification{
		action: MemberHintActionAppendInclude,
		diagnostics: []string{
			fmt.Sprintf("will add include path %q to existing member %q", candidate.Paths.Include[0], targetName),
		},
		driftDetected:   true,
		backfillAllowed: true,
	}, nil
}

func memberHintMatchesPatterns(patterns []string, hint resolvedMemberHint, generatedInclude string) (bool, error) {
	normalizedPatterns, err := normalizeMemberPatterns(patterns)
	if err != nil {
		return false, err
	}

	candidates := []string{hint.RootPath}
	if generatedInclude != hint.RootPath {
		candidates = append(candidates, generatedInclude)
	}

	for _, pattern := range normalizedPatterns {
		if pattern == generatedInclude {
			return true, nil
		}
		for _, candidate := range candidates {
			matches, err := matchMemberPatterns([]string{pattern}, candidate)
			if err != nil {
				return false, err
			}
			if matches {
				return true, nil
			}
		}
	}

	return false, nil
}

func memberMatchesHintCandidate(existing OrbitMember, candidate OrbitMember) bool {
	if !memberNonPathFieldsMatchHintCandidate(existing, candidate) {
		return false
	}
	if len(existing.Paths.Include) != 1 || len(candidate.Paths.Include) != 1 {
		return false
	}
	if existing.Paths.Include[0] != candidate.Paths.Include[0] {
		return false
	}
	if len(existing.Paths.Exclude) != 0 || len(candidate.Paths.Exclude) != 0 {
		return false
	}

	return true
}

func memberNonPathFieldsMatchHintCandidate(existing OrbitMember, candidate OrbitMember) bool {
	if orbitMemberIdentityName(existing) != orbitMemberIdentityName(candidate) {
		return false
	}
	if existing.Description != candidate.Description {
		return false
	}
	if existing.Role != candidate.Role {
		return false
	}
	if existing.Lane != candidate.Lane {
		return false
	}

	return reflect.DeepEqual(cloneOrbitMemberScopePatch(existing.Scopes), cloneOrbitMemberScopePatch(candidate.Scopes))
}
