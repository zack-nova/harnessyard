package orbit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

// MemberHintBackfillResult reports one successful explicit member-hint backfill.
type MemberHintBackfillResult struct {
	OrbitID        string   `json:"orbit_id"`
	DefinitionPath string   `json:"definition_path"`
	UpdatedMembers []string `json:"updated_members,omitempty"`
	ConsumedHints  []string `json:"consumed_hints,omitempty"`
}

type memberBackfillPlan struct {
	specFilename string
	specOriginal []byte
	specNext     []byte
	mutations    []memberBackfillMutation
}

type memberBackfillMutation struct {
	filename string
	original []byte
	next     []byte
	remove   bool
}

// MemberHintConsumeEffect describes one consumed member hint.
type MemberHintConsumeEffect struct {
	Path              string
	PreservedMetadata bool
}

// Test-only hook for deterministic member hint consume rollback failures.
var beforeMemberHintConsumeMutationHook func(filename string)

// ConsumeMemberHintPaths removes already-applied member hints without changing
// authored Orbit truth.
func ConsumeMemberHintPaths(repoRoot string, hintPaths []string) ([]MemberHintConsumeEffect, error) {
	plan, effects, err := buildMemberHintConsumePlan(repoRoot, hintPaths)
	if err != nil {
		return nil, err
	}
	if err := applyMemberHintConsumePlan(plan); err != nil {
		return nil, err
	}

	sort.Slice(effects, func(left, right int) bool {
		return effects[left].Path < effects[right].Path
	})
	return effects, nil
}

// BackfillMemberHints updates one hosted OrbitSpec from resolved hints and
// consumes those hints on success.
func BackfillMemberHints(repoRoot string, spec OrbitSpec, candidateFiles []string) (MemberHintBackfillResult, error) {
	normalizedSpec, err := normalizeOrbitSpecMemberIdentities(spec)
	if err != nil {
		return MemberHintBackfillResult{}, fmt.Errorf("normalize orbit spec: %w", err)
	}

	inspection, err := InspectMemberHints(repoRoot, normalizedSpec, candidateFiles)
	if err != nil {
		return MemberHintBackfillResult{}, fmt.Errorf("inspect member hints: %w", err)
	}
	if !inspection.BackfillAllowed {
		return MemberHintBackfillResult{}, memberHintOperationError("member backfill cannot proceed", inspection.Hints)
	}

	filteredCandidateFiles, err := filterMemberHintCandidateFiles(normalizedSpec, candidateFiles)
	if err != nil {
		return MemberHintBackfillResult{}, fmt.Errorf("filter member hint candidates: %w", err)
	}

	_, validHints, err := scanResolvedMemberHints(repoRoot, filteredCandidateFiles)
	if err != nil {
		return MemberHintBackfillResult{}, err
	}

	nextSpec, updatedMembers, err := backfilledOrbitSpec(normalizedSpec, validHints)
	if err != nil {
		return MemberHintBackfillResult{}, err
	}
	plan, consumedHints, err := buildMemberBackfillPlan(repoRoot, normalizedSpec, nextSpec, validHints)
	if err != nil {
		return MemberHintBackfillResult{}, err
	}
	if err := applyMemberBackfillPlan(plan); err != nil {
		return MemberHintBackfillResult{}, err
	}

	sort.Strings(updatedMembers)
	sort.Strings(consumedHints)

	return MemberHintBackfillResult{
		OrbitID:        normalizedSpec.ID,
		DefinitionPath: normalizedSpec.SourcePath,
		UpdatedMembers: updatedMembers,
		ConsumedHints:  consumedHints,
	}, nil
}

func backfilledOrbitSpec(spec OrbitSpec, hints []resolvedMemberHint) (OrbitSpec, []string, error) {
	next := spec
	updatedMembers := make([]string, 0, len(hints))

	existingByName := make(map[string]int, len(next.Members))
	for index, member := range next.Members {
		existingByName[orbitMemberIdentityName(member)] = index
	}

	sortedHints := append([]resolvedMemberHint(nil), hints...)
	sort.Slice(sortedHints, func(left, right int) bool {
		return sortedHints[left].HintPath < sortedHints[right].HintPath
	})

	for _, hint := range sortedHints {
		candidate := buildMemberHintCandidate(hint).Member
		updatedMembers = append(updatedMembers, candidate.Name)

		if index, found := existingByName[candidate.Name]; found {
			member, err := backfilledExistingMember(next.Members[index], hint, candidate)
			if err != nil {
				return OrbitSpec{}, nil, err
			}
			next.Members[index] = member
			continue
		}

		existingByName[candidate.Name] = len(next.Members)
		next.Members = append(next.Members, candidate)
	}

	return next, updatedMembers, nil
}

func backfilledExistingMember(existing OrbitMember, hint resolvedMemberHint, candidate OrbitMember) (OrbitMember, error) {
	if isHintManageableMember(existing) {
		return candidate, nil
	}

	next := existing
	next.Description = candidate.Description
	next.Role = candidate.Role
	next.Lane = candidate.Lane
	next.Scopes = cloneOrbitMemberScopePatch(candidate.Scopes)

	included, err := memberHintMatchesPatterns(existing.Paths.Include, hint, candidate.Paths.Include[0])
	if err != nil {
		return OrbitMember{}, fmt.Errorf("match member hint against existing includes: %w", err)
	}
	if included {
		return next, nil
	}

	excluded, err := memberHintMatchesPatterns(existing.Paths.Exclude, hint, candidate.Paths.Include[0])
	if err != nil {
		return OrbitMember{}, fmt.Errorf("match member hint against existing excludes: %w", err)
	}
	if excluded {
		return next, nil
	}

	if !stringSliceContains(next.Paths.Include, candidate.Paths.Include[0]) {
		next.Paths.Include = append(next.Paths.Include, candidate.Paths.Include[0])
	}

	return next, nil
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func buildMemberBackfillPlan(
	repoRoot string,
	currentSpec OrbitSpec,
	nextSpec OrbitSpec,
	hints []resolvedMemberHint,
) (memberBackfillPlan, []string, error) {
	plan := memberBackfillPlan{
		specFilename: currentSpec.SourcePath,
	}

	specOriginal, err := os.ReadFile(currentSpec.SourcePath)
	if err != nil {
		return memberBackfillPlan{}, nil, fmt.Errorf("read orbit spec %q: %w", currentSpec.SourcePath, err)
	}
	plan.specOriginal = specOriginal

	nextSpecData, err := marshalHostedOrbitSpecData(nextSpec)
	if err != nil {
		return memberBackfillPlan{}, nil, err
	}
	plan.specNext = nextSpecData

	consumedHints := make([]string, 0, len(hints))
	for _, hint := range hints {
		filename := filepath.Join(repoRoot, filepath.FromSlash(hint.HintPath))
		//nolint:gosec // The path is repo-local and derived from normalized tracked hint paths.
		originalData, err := os.ReadFile(filename)
		if err != nil {
			return memberBackfillPlan{}, nil, fmt.Errorf("read member hint %q: %w", hint.HintPath, err)
		}

		mutation := memberBackfillMutation{
			filename: filename,
			original: originalData,
		}
		switch hint.Kind {
		case memberHintKindFileFrontmatter:
			nextData, err := removeOrbitMemberFrontmatter(originalData, hint.HintPath)
			if err != nil {
				return memberBackfillPlan{}, nil, err
			}
			mutation.next = nextData
		case memberHintKindDirectoryMarker:
			mutation.remove = true
		default:
			return memberBackfillPlan{}, nil, fmt.Errorf("unsupported member hint kind %q", hint.Kind)
		}

		plan.mutations = append(plan.mutations, mutation)
		consumedHints = append(consumedHints, hint.HintPath)
	}

	return plan, consumedHints, nil
}

func buildMemberHintConsumePlan(repoRoot string, hintPaths []string) ([]memberBackfillMutation, []MemberHintConsumeEffect, error) {
	mutations := make([]memberBackfillMutation, 0, len(hintPaths))
	effects := make([]MemberHintConsumeEffect, 0, len(hintPaths))

	for _, hintPath := range hintPaths {
		normalizedHintPath, err := ids.NormalizeRepoRelativePath(hintPath)
		if err != nil {
			return nil, nil, fmt.Errorf("normalize consumed member hint %q: %w", hintPath, err)
		}
		filename := filepath.Join(repoRoot, filepath.FromSlash(normalizedHintPath))
		//nolint:gosec // The path is repo-local and derived from normalized detected hint paths.
		originalData, err := os.ReadFile(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("read member hint %q: %w", normalizedHintPath, err)
		}

		mutation := memberBackfillMutation{
			filename: filename,
			original: originalData,
		}
		switch {
		case filepath.Base(filename) == memberHintMarkerFileName:
			mutation.remove = true
			effects = append(effects, MemberHintConsumeEffect{
				Path: normalizedHintPath,
			})
		case filepath.Ext(filename) == ".md":
			nextData, err := removeOrbitMemberFrontmatter(originalData, normalizedHintPath)
			if err != nil {
				return nil, nil, err
			}
			mutation.next = nextData
			effects = append(effects, MemberHintConsumeEffect{
				Path:              normalizedHintPath,
				PreservedMetadata: strings.HasPrefix(string(nextData), "---\n"),
			})
		default:
			return nil, nil, fmt.Errorf("unsupported consumed member hint %q", normalizedHintPath)
		}

		mutations = append(mutations, mutation)
	}

	return mutations, effects, nil
}

func applyMemberHintConsumePlan(plan []memberBackfillMutation) error {
	applied := make([]memberBackfillMutation, 0, len(plan))

	for _, mutation := range plan {
		runBeforeMemberHintConsumeMutationHook(mutation.filename)
		if err := applyMemberBackfillMutation(mutation); err != nil {
			if rollbackErr := rollbackMemberHintConsume(applied); rollbackErr != nil {
				return fmt.Errorf("member hint cleanup rollback after %s failed: %w", mutation.filename, rollbackErr)
			}
			return fmt.Errorf("member hint cleanup rollback after %s: %w", mutation.filename, err)
		}
		applied = append(applied, mutation)
	}

	return nil
}

func runBeforeMemberHintConsumeMutationHook(filename string) {
	if beforeMemberHintConsumeMutationHook != nil {
		beforeMemberHintConsumeMutationHook(filename)
	}
}

func rollbackMemberHintConsume(applied []memberBackfillMutation) error {
	var rollbackErrs []string

	for index := len(applied) - 1; index >= 0; index-- {
		mutation := applied[index]
		if err := contractutil.AtomicWriteFile(mutation.filename, mutation.original); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore %s: %v", mutation.filename, err))
		}
	}

	if len(rollbackErrs) > 0 {
		sort.Strings(rollbackErrs)
		return errors.New(strings.Join(rollbackErrs, "; "))
	}

	return nil
}

func applyMemberBackfillPlan(plan memberBackfillPlan) error {
	applied := make([]memberBackfillMutation, 0, len(plan.mutations))
	specWritten := false

	if !bytes.Equal(plan.specOriginal, plan.specNext) {
		if err := atomicWriteFile(plan.specFilename, plan.specNext); err != nil {
			return fmt.Errorf("write orbit spec: %w", err)
		}
		specWritten = true
	}

	for _, mutation := range plan.mutations {
		if err := applyMemberBackfillMutation(mutation); err != nil {
			if rollbackErr := rollbackMemberBackfill(plan, applied, specWritten); rollbackErr != nil {
				return fmt.Errorf("member backfill rollback after %s failed: %w", mutation.filename, rollbackErr)
			}
			return fmt.Errorf("member backfill rollback after %s: %w", mutation.filename, err)
		}
		applied = append(applied, mutation)
	}

	return nil
}

func rollbackMemberBackfill(plan memberBackfillPlan, applied []memberBackfillMutation, specWritten bool) error {
	var rollbackErrs []string

	for index := len(applied) - 1; index >= 0; index-- {
		mutation := applied[index]
		if err := contractutil.AtomicWriteFile(mutation.filename, mutation.original); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore %s: %v", mutation.filename, err))
		}
	}

	if specWritten {
		if err := contractutil.AtomicWriteFile(plan.specFilename, plan.specOriginal); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore %s: %v", plan.specFilename, err))
		}
	}

	if len(rollbackErrs) > 0 {
		sort.Strings(rollbackErrs)
		return errors.New(strings.Join(rollbackErrs, "; "))
	}

	return nil
}

func applyMemberBackfillMutation(mutation memberBackfillMutation) error {
	if mutation.remove {
		if err := os.Remove(mutation.filename); err != nil {
			return fmt.Errorf("remove consumed member hint %s: %w", mutation.filename, err)
		}
		return nil
	}

	if reflect.DeepEqual(mutation.original, mutation.next) {
		return nil
	}
	if err := contractutil.AtomicWriteFile(mutation.filename, mutation.next); err != nil {
		return fmt.Errorf("rewrite consumed member hint %s: %w", mutation.filename, err)
	}

	return nil
}

func memberHintOperationError(prefix string, hints []DetectedMemberHint) error {
	diagnostics := make([]string, 0)
	for _, hint := range hints {
		if hint.Action != MemberHintActionConflict && hint.Action != MemberHintActionInvalidHint {
			continue
		}
		if len(hint.Diagnostics) == 0 {
			diagnostics = append(diagnostics, fmt.Sprintf("%s at %s", hint.Action, hint.HintPath))
			continue
		}
		diagnostics = append(diagnostics, hint.Diagnostics...)
	}

	sort.Strings(diagnostics)
	if len(diagnostics) == 0 {
		return errors.New(prefix)
	}

	return fmt.Errorf("%s: %s", prefix, strings.Join(diagnostics, "; "))
}

func marshalHostedOrbitSpecData(spec OrbitSpec) ([]byte, error) {
	normalized, err := normalizeOrbitSpecMemberIdentities(spec)
	if err != nil {
		return nil, fmt.Errorf("normalize orbit spec: %w", err)
	}
	if err := ValidateHostedOrbitSpec(normalized); err != nil {
		return nil, fmt.Errorf("validate orbit spec: %w", err)
	}

	data, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal orbit spec: %w", err)
	}

	return append(data, '\n'), nil
}

func removeOrbitMemberFrontmatter(data []byte, hintPath string) ([]byte, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("%s must start with YAML frontmatter", hintPath)
	}

	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, fmt.Errorf("%s frontmatter must terminate with ---", hintPath)
	}

	frontmatterContent := rest[:end]
	body := rest[end+len("\n---\n"):]

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatterContent), &document); err != nil {
		return nil, fmt.Errorf("%s frontmatter is invalid YAML: %w", hintPath, err)
	}
	if len(document.Content) == 0 {
		return nil, fmt.Errorf("%s frontmatter must be a mapping", hintPath)
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s frontmatter must be a mapping", hintPath)
	}

	filtered := make([]*yaml.Node, 0, len(root.Content))
	removed := false
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		if keyNode.Value == "orbit_member" {
			removed = true
			continue
		}
		filtered = append(filtered, keyNode, valueNode)
	}
	if !removed {
		if isFlatMemberHintRoot(root) {
			return []byte(body), nil
		}
		return nil, fmt.Errorf("%s frontmatter does not define orbit_member or flat member hint fields", hintPath)
	}

	if len(filtered) == 0 {
		return []byte(body), nil
	}

	root.Content = filtered
	frontmatterData, err := marshalYAMLNode(root)
	if err != nil {
		return nil, fmt.Errorf("encode %s frontmatter: %w", hintPath, err)
	}

	return []byte("---\n" + string(frontmatterData) + "---\n" + body), nil
}

func marshalYAMLNode(node *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(4)
	if err := encoder.Encode(node); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("encode yaml node: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}

	content := buffer.String()
	content = strings.TrimPrefix(content, "---\n")
	content = strings.TrimSuffix(content, "...\n")

	return []byte(content), nil
}
