package orbit

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	CapabilityFieldCommandsPaths   = "capabilities.commands.paths"
	CapabilityFieldLocalSkillPaths = "capabilities.skills.local.paths"
)

type capabilityPathScope struct {
	kind          string
	field         string
	pattern       string
	matchPatterns []string
	exclude       []string
}

// CapabilityOwnedMemberInclude describes one member include pattern that crosses
// into the capability-owned path lane.
type CapabilityOwnedMemberInclude struct {
	IncludeIndex      int
	Include           string
	CapabilityKind    string
	CapabilityField   string
	CapabilityPattern string
}

// MemberCapabilityPathOverlap describes persisted member truth that overlaps
// capability-owned path truth.
type MemberCapabilityPathOverlap struct {
	MemberIndex int
	MemberName  string
	CapabilityOwnedMemberInclude
}

// SplitCapabilityOwnedMemberIncludes separates ordinary member includes from
// includes that are already owned by command or local-skill capability truth.
func SplitCapabilityOwnedMemberIncludes(
	spec OrbitSpec,
	includes []string,
	excludes []string,
) ([]string, []CapabilityOwnedMemberInclude, error) {
	ordinary := make([]string, 0, len(includes))
	owned := make([]CapabilityOwnedMemberInclude, 0)

	for index, include := range includes {
		overlap, ok, err := findCapabilityOwnedMemberInclude(spec, index, include, excludes)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			owned = append(owned, overlap)
			continue
		}
		ordinary = append(ordinary, include)
	}

	return ordinary, owned, nil
}

// FindMemberCapabilityPathOverlaps returns persisted member include patterns
// that overlap command or local-skill capability-owned path ranges.
func FindMemberCapabilityPathOverlaps(spec OrbitSpec) ([]MemberCapabilityPathOverlap, error) {
	overlaps := make([]MemberCapabilityPathOverlap, 0)
	for memberIndex, member := range spec.Members {
		_, owned, err := SplitCapabilityOwnedMemberIncludes(spec, member.Paths.Include, member.Paths.Exclude)
		if err != nil {
			return nil, fmt.Errorf("members[%d].paths: %w", memberIndex, err)
		}
		for _, include := range owned {
			overlaps = append(overlaps, MemberCapabilityPathOverlap{
				MemberIndex:                  memberIndex,
				MemberName:                   orbitMemberIdentityName(member),
				CapabilityOwnedMemberInclude: include,
			})
		}
	}

	return overlaps, nil
}

func findCapabilityOwnedMemberInclude(
	spec OrbitSpec,
	includeIndex int,
	include string,
	memberExcludes []string,
) (CapabilityOwnedMemberInclude, bool, error) {
	normalizedInclude, err := normalizePattern(include)
	if err != nil {
		return CapabilityOwnedMemberInclude{}, false, fmt.Errorf("include[%d]: %w", includeIndex, err)
	}
	normalizedMemberExcludes, err := normalizePatterns(memberExcludes)
	if err != nil {
		return CapabilityOwnedMemberInclude{}, false, fmt.Errorf("exclude: %w", err)
	}

	for _, scope := range capabilityPathScopes(spec) {
		for _, matchPattern := range scope.matchPatterns {
			overlaps, err := patternsOverlap(normalizedInclude, normalizedMemberExcludes, matchPattern, scope.exclude)
			if err != nil {
				return CapabilityOwnedMemberInclude{}, false, err
			}
			if !overlaps {
				continue
			}

			return CapabilityOwnedMemberInclude{
				IncludeIndex:      includeIndex,
				Include:           include,
				CapabilityKind:    scope.kind,
				CapabilityField:   scope.field,
				CapabilityPattern: scope.pattern,
			}, true, nil
		}
	}

	return CapabilityOwnedMemberInclude{}, false, nil
}

func capabilityPathScopes(spec OrbitSpec) []capabilityPathScope {
	if spec.Capabilities == nil {
		return nil
	}

	scopes := make([]capabilityPathScope, 0)
	if spec.Capabilities.Commands != nil {
		excludes := normalizedCapabilityExcludes(spec.Capabilities.Commands.Paths.Exclude, false)
		for _, include := range spec.Capabilities.Commands.Paths.Include {
			normalized, err := normalizePattern(include)
			if err != nil {
				continue
			}
			scopes = append(scopes, capabilityPathScope{
				kind:          "commands",
				field:         CapabilityFieldCommandsPaths,
				pattern:       normalized,
				matchPatterns: []string{normalized},
				exclude:       excludes,
			})
		}
	}

	if spec.Capabilities.Skills != nil && spec.Capabilities.Skills.Local != nil {
		excludes := normalizedCapabilityExcludes(spec.Capabilities.Skills.Local.Paths.Exclude, true)
		for _, include := range spec.Capabilities.Skills.Local.Paths.Include {
			normalized, err := normalizePattern(include)
			if err != nil {
				continue
			}
			scopes = append(scopes, capabilityPathScope{
				kind:          "local skills",
				field:         CapabilityFieldLocalSkillPaths,
				pattern:       normalized,
				matchPatterns: []string{normalized, path.Join(normalized, "**")},
				exclude:       excludes,
			})
		}
	}

	return scopes
}

func normalizedCapabilityExcludes(patterns []string, includeDescendants bool) []string {
	excludes := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		normalized, err := normalizePattern(pattern)
		if err != nil {
			continue
		}
		excludes = append(excludes, normalized)
		if includeDescendants {
			excludes = append(excludes, path.Join(normalized, "**"))
		}
	}
	sort.Strings(excludes)

	return excludes
}

func patternsOverlap(memberPattern string, memberExcludes []string, capabilityPattern string, capabilityExcludes []string) (bool, error) {
	candidates := representativePatternCandidates(memberPattern, capabilityPattern)
	for _, candidate := range candidates {
		memberMatches, err := doublestar.Match(memberPattern, candidate)
		if err != nil {
			return false, fmt.Errorf("match member pattern %q: %w", memberPattern, err)
		}
		if !memberMatches {
			continue
		}

		capabilityMatches, err := doublestar.Match(capabilityPattern, candidate)
		if err != nil {
			return false, fmt.Errorf("match capability pattern %q: %w", capabilityPattern, err)
		}
		if !capabilityMatches {
			continue
		}

		excludedByMember, err := matchNormalizedAny(memberExcludes, candidate)
		if err != nil {
			return false, fmt.Errorf("match member excludes for %q: %w", candidate, err)
		}
		if excludedByMember {
			continue
		}

		excludedByCapability, err := matchNormalizedAny(capabilityExcludes, candidate)
		if err != nil {
			return false, fmt.Errorf("match capability excludes for %q: %w", candidate, err)
		}
		if excludedByCapability {
			continue
		}

		return true, nil
	}

	return false, nil
}

func representativePatternCandidates(patterns ...string) []string {
	candidateSet := make(map[string]struct{})
	for _, pattern := range patterns {
		for _, candidate := range representativeCandidatesForPattern(pattern) {
			if candidate == "" || candidate == "." {
				continue
			}
			candidateSet[candidate] = struct{}{}
		}
	}

	candidates := make([]string, 0, len(candidateSet))
	for candidate := range candidateSet {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)

	return candidates
}

func representativeCandidatesForPattern(pattern string) []string {
	parts := strings.Split(pattern, "/")
	candidates := []string{""}
	for _, part := range parts {
		next := make([]string, 0, len(candidates)*2)
		if part == "**" {
			next = append(next, candidates...)
			for _, candidate := range candidates {
				next = append(next, joinRepresentativeSegment(candidate, "sample"))
			}
			candidates = next
			continue
		}

		segment := representativeGlobSegment(part)
		for _, candidate := range candidates {
			next = append(next, joinRepresentativeSegment(candidate, segment))
		}
		candidates = next
	}

	normalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		value, err := ids.NormalizeRepoRelativePath(candidate)
		if err != nil {
			continue
		}
		normalized = append(normalized, value)
	}

	return normalized
}

func joinRepresentativeSegment(base string, segment string) string {
	if base == "" {
		return segment
	}

	return base + "/" + segment
}

func representativeGlobSegment(segment string) string {
	var builder strings.Builder
	for index := 0; index < len(segment); {
		switch segment[index] {
		case '*':
			builder.WriteString("sample")
			for index < len(segment) && segment[index] == '*' {
				index++
			}
		case '?':
			builder.WriteByte('x')
			index++
		case '[':
			end := strings.IndexByte(segment[index:], ']')
			if end < 0 {
				builder.WriteByte('a')
				index++
				continue
			}
			builder.WriteByte('a')
			index += end + 1
		default:
			builder.WriteByte(segment[index])
			index++
		}
	}

	if builder.Len() == 0 {
		return "sample"
	}

	return builder.String()
}
