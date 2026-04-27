package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

func runPathspecCommandWithFallback(
	ctx context.Context,
	repoRoot string,
	command string,
	commandArgs []string,
	paths []string,
) (output []byte, err error) {
	if len(paths) == 0 {
		return []byte{}, nil
	}

	normalizedPaths, err := normalizePathspecPaths(paths)
	if err != nil {
		return nil, fmt.Errorf("normalize %s pathspec paths: %w", command, err)
	}

	pathspecFile, err := CreatePathspecFile(normalizedPaths)
	if err != nil {
		return nil, fmt.Errorf("create %s pathspec file: %w", command, err)
	}
	defer func() {
		cleanupErr := pathspecFile.Cleanup()
		if cleanupErr == nil {
			return
		}

		wrappedCleanupErr := fmt.Errorf("cleanup %s pathspec file: %w", command, cleanupErr)
		if err == nil {
			err = wrappedCleanupErr
			return
		}

		err = errors.Join(err, wrappedCleanupErr)
	}()

	pathspecArgs := append([]string{command}, commandArgs...)
	pathspecArgs = append(
		pathspecArgs,
		"--pathspec-from-file="+pathspecFile.Path,
		"--pathspec-file-nul",
	)

	output, err = runGit(ctx, repoRoot, pathspecArgs...)
	if err == nil {
		return output, nil
	}

	if !pathspecFromFileUnsupported(err) {
		return nil, fmt.Errorf("git %v: %w", pathspecArgs, err)
	}

	fallbackArgs := append([]string{command}, commandArgs...)
	fallbackArgs = append(fallbackArgs, "--")
	fallbackArgs = append(fallbackArgs, normalizedPaths...)

	output, err = runGit(ctx, repoRoot, fallbackArgs...)
	if err != nil {
		return nil, fmt.Errorf("git %v: %w", fallbackArgs, err)
	}

	return output, nil
}

func pathspecFromFileUnsupported(err error) bool {
	if err == nil {
		return false
	}

	errText := err.Error()

	return strings.Contains(errText, "invalid option: --pathspec-from-file=") ||
		strings.Contains(errText, "unrecognized argument: --pathspec-from-file=")
}
