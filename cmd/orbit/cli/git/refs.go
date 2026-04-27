package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const orbitRefsPrefix = "refs/orbits"

var updateRefRunner = runGit

// LastScopedRef returns the documented best-effort ref for scoped commits.
func LastScopedRef(orbitID string) (string, error) {
	return orbitRefPath(orbitID, "last-scoped")
}

// LastRestoreRef returns the documented best-effort ref for scoped restores.
func LastRestoreRef(orbitID string) (string, error) {
	return orbitRefPath(orbitID, "last-restore")
}

// UpdateRef updates a best-effort Orbit auxiliary ref.
func UpdateRef(ctx context.Context, repoRoot string, refName string, target string) error {
	args := []string{"update-ref", refName, target}

	if _, err := updateRefRunner(ctx, repoRoot, args...); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return nil
}

func orbitRefPath(orbitID string, suffix string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", orbitRefsPrefix, orbitID, suffix), nil
}
