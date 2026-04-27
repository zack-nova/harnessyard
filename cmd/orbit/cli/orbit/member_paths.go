package orbit

import (
	"fmt"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// MemberMatchesPath reports whether one normalized repo-relative path belongs to the member.
func MemberMatchesPath(member OrbitMember, normalizedPath string) (bool, error) {
	includePatterns, err := normalizeMemberPatterns(member.Paths.Include)
	if err != nil {
		return false, fmt.Errorf("normalize include patterns: %w", err)
	}
	excludePatterns, err := normalizeMemberPatterns(member.Paths.Exclude)
	if err != nil {
		return false, fmt.Errorf("normalize exclude patterns: %w", err)
	}

	included, err := matchMemberPatterns(includePatterns, normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match include patterns: %w", err)
	}
	if !included {
		return false, nil
	}

	excluded, err := matchMemberPatterns(excludePatterns, normalizedPath)
	if err != nil {
		return false, fmt.Errorf("match exclude patterns: %w", err)
	}

	return !excluded, nil
}

func normalizeMemberPatterns(patterns []string) ([]string, error) {
	normalized := make([]string, 0, len(patterns))
	for index, pattern := range patterns {
		value, err := ids.NormalizeRepoRelativePath(pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern[%d]: %w", index, err)
		}
		if _, err := doublestar.Match(value, ""); err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", value, err)
		}
		normalized = append(normalized, value)
	}

	return normalized, nil
}

func matchMemberPatterns(patterns []string, normalizedCandidate string) (bool, error) {
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
