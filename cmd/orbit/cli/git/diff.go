package git

import (
	"context"
	"fmt"
)

// DiffPathspec returns a path-limited git diff for the requested paths.
func DiffPathspec(ctx context.Context, repoRoot string, paths []string) ([]byte, error) {
	output, err := runPathspecCommandWithFallback(ctx, repoRoot, "diff", nil, paths)
	if err != nil {
		return nil, fmt.Errorf("run path-limited git diff: %w", err)
	}

	return output, nil
}
