package git

import (
	"context"
	"fmt"
)

// LogPathspec returns path-limited git log output for the requested paths.
func LogPathspec(ctx context.Context, repoRoot string, paths []string, gitArgs []string) ([]byte, error) {
	output, err := runPathspecCommandWithFallback(ctx, repoRoot, "log", gitArgs, paths)
	if err != nil {
		return nil, fmt.Errorf("run path-limited git log: %w", err)
	}

	return output, nil
}
