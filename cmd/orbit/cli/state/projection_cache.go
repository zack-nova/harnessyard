package state

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// ErrProjectionCacheNotFound indicates no projection cache exists for the requested orbit.
var ErrProjectionCacheNotFound = errors.New("projection cache not found")

// WriteProjectionCache writes the sorted projection cache for an orbit.
func (store FSStore) WriteProjectionCache(orbitID string, paths []string) error {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return fmt.Errorf("validate orbit id: %w", err)
	}

	normalizedPaths, err := normalizeProjectionCachePaths(paths)
	if err != nil {
		return fmt.Errorf("normalize projection cache paths: %w", err)
	}

	data := []byte(strings.Join(normalizedPaths, "\n"))
	if len(normalizedPaths) > 0 {
		data = append(data, '\n')
	}

	if err := atomicWriteFile(store.projectionCachePath(orbitID), data); err != nil {
		return fmt.Errorf("write projection cache: %w", err)
	}

	return nil
}

// ReadProjectionCache reads the cached projection scope for an orbit.
func (store FSStore) ReadProjectionCache(orbitID string) ([]string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return nil, fmt.Errorf("validate orbit id: %w", err)
	}

	data, err := os.ReadFile(store.projectionCachePath(orbitID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrProjectionCacheNotFound
		}
		return nil, fmt.Errorf("read projection cache: %w", err)
	}

	if len(data) == 0 {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

	normalizedPaths, err := normalizeProjectionCachePaths(lines)
	if err != nil {
		return nil, fmt.Errorf("normalize projection cache paths: %w", err)
	}

	return normalizedPaths, nil
}

func normalizeProjectionCachePaths(paths []string) ([]string, error) {
	normalizedSet := make(map[string]struct{}, len(paths))

	for _, pathValue := range paths {
		normalized, err := ids.NormalizeRepoRelativePath(pathValue)
		if err != nil {
			return nil, fmt.Errorf("normalize path %q: %w", pathValue, err)
		}
		normalizedSet[normalized] = struct{}{}
	}

	normalizedPaths := make([]string, 0, len(normalizedSet))
	for normalized := range normalizedSet {
		normalizedPaths = append(normalizedPaths, normalized)
	}

	sort.Strings(normalizedPaths)

	return normalizedPaths, nil
}
