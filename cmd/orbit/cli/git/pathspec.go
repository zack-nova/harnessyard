package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	pathspecFilePerm = 0o600
	pathspecPattern  = "orbit-pathspec-*"
)

// PathspecFile is a temporary NUL-delimited pathspec file for Git commands.
type PathspecFile struct {
	Path string
}

// CreatePathspecFile writes a normalized NUL-delimited pathspec temp file.
func CreatePathspecFile(paths []string) (PathspecFile, error) {
	normalizedPaths, err := normalizePathspecPaths(paths)
	if err != nil {
		return PathspecFile{}, fmt.Errorf("normalize pathspec paths: %w", err)
	}

	tempFile, err := os.CreateTemp("", pathspecPattern)
	if err != nil {
		return PathspecFile{}, fmt.Errorf("create pathspec temp file: %w", err)
	}

	tempName := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempName)
		}
	}()

	if err := tempFile.Chmod(pathspecFilePerm); err != nil {
		_ = tempFile.Close()
		return PathspecFile{}, fmt.Errorf("set pathspec temp file permissions: %w", err)
	}

	var data bytes.Buffer
	for _, pathValue := range normalizedPaths {
		if _, err := data.WriteString(pathValue); err != nil {
			_ = tempFile.Close()
			return PathspecFile{}, fmt.Errorf("buffer pathspec path %q: %w", pathValue, err)
		}
		if err := data.WriteByte(0); err != nil {
			_ = tempFile.Close()
			return PathspecFile{}, fmt.Errorf("buffer pathspec delimiter for %q: %w", pathValue, err)
		}
	}

	if _, err := tempFile.Write(data.Bytes()); err != nil {
		_ = tempFile.Close()
		return PathspecFile{}, fmt.Errorf("write pathspec temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return PathspecFile{}, fmt.Errorf("sync pathspec temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return PathspecFile{}, fmt.Errorf("close pathspec temp file: %w", err)
	}

	cleanupTemp = false

	return PathspecFile{Path: tempName}, nil
}

// Cleanup removes the temp file.
func (file PathspecFile) Cleanup() error {
	if file.Path == "" {
		return nil
	}
	if err := os.Remove(file.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove pathspec temp file: %w", err)
	}

	return nil
}

func normalizePathspecPaths(paths []string) ([]string, error) {
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
