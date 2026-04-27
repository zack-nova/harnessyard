package orbit

import (
	"errors"
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func normalizePattern(pattern string) (string, error) {
	if pattern == "" {
		return "", errors.New("pattern must not be empty")
	}

	normalized, err := ids.NormalizeRepoRelativePath(pattern)
	if err != nil {
		return "", fmt.Errorf("normalize pattern %q: %w", pattern, err)
	}

	if _, err := doublestar.Match(normalized, ""); err != nil {
		return "", fmt.Errorf("invalid pattern %q: %w", normalized, err)
	}

	return normalized, nil
}

func normalizePatterns(patterns []string) ([]string, error) {
	normalized := make([]string, 0, len(patterns))

	for index, pattern := range patterns {
		value, err := normalizePattern(pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern[%d]: %w", index, err)
		}
		normalized = append(normalized, value)
	}

	return normalized, nil
}

func matchNormalizedAny(patterns []string, normalizedCandidate string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := doublestar.Match(pattern, normalizedCandidate)
		if err != nil {
			return false, fmt.Errorf("match pattern %q: %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}

	return false, nil
}

// PathMatchesOrbit reports whether a repo-relative path belongs to an orbit's
// semantic scope, including shared-scope and projection-visible patterns. This is used for
// classifying untracked paths, which are not part of the resolved tracked-file
// scope cache.
func PathMatchesOrbit(config GlobalConfig, definition Definition, candidate string) (bool, error) {
	if err := ValidateGlobalConfig(config); err != nil {
		return false, fmt.Errorf("validate global config: %w", err)
	}
	if err := ValidateDefinition(definition); err != nil {
		return false, fmt.Errorf("validate orbit definition: %w", err)
	}

	normalizedCandidate, err := ids.NormalizeRepoRelativePath(candidate)
	if err != nil {
		return false, fmt.Errorf("normalize candidate path %q: %w", candidate, err)
	}
	if isControlPlanePath(normalizedCandidate) {
		return false, nil
	}

	includePatterns, err := normalizePatterns(definition.Include)
	if err != nil {
		return false, fmt.Errorf("normalize include patterns: %w", err)
	}
	excludePatterns, err := normalizePatterns(definition.Exclude)
	if err != nil {
		return false, fmt.Errorf("normalize exclude patterns: %w", err)
	}
	sharedScopePatterns, err := normalizePatterns(config.SharedScope)
	if err != nil {
		return false, fmt.Errorf("normalize shared scope patterns: %w", err)
	}
	projectionVisiblePatterns, err := normalizePatterns(config.ProjectionVisible)
	if err != nil {
		return false, fmt.Errorf("normalize projection-visible patterns: %w", err)
	}

	excluded, err := matchNormalizedAny(excludePatterns, normalizedCandidate)
	if err != nil {
		return false, fmt.Errorf("match exclude patterns for %q: %w", normalizedCandidate, err)
	}
	if excluded {
		return false, nil
	}

	included, err := matchNormalizedAny(includePatterns, normalizedCandidate)
	if err != nil {
		return false, fmt.Errorf("match include patterns for %q: %w", normalizedCandidate, err)
	}
	if included {
		return true, nil
	}

	inSharedScope, err := matchNormalizedAny(sharedScopePatterns, normalizedCandidate)
	if err != nil {
		return false, fmt.Errorf("match shared scope patterns for %q: %w", normalizedCandidate, err)
	}
	if inSharedScope {
		return true, nil
	}

	projectionVisible, err := matchNormalizedAny(projectionVisiblePatterns, normalizedCandidate)
	if err != nil {
		return false, fmt.Errorf("match projection-visible patterns for %q: %w", normalizedCandidate, err)
	}

	return projectionVisible, nil
}
