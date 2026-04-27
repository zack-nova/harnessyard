package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo contains the core Git repository paths Orbit needs.
type Repo struct {
	Root   string
	GitDir string
}

// IsNotGitRepositoryError reports whether the git failure is a standard non-repository error.
func IsNotGitRepositoryError(err error) bool {
	return strings.Contains(err.Error(), "not a git repository")
}

// DiscoverRepo resolves both the repository root and absolute git dir.
func DiscoverRepo(ctx context.Context, workingDir string) (Repo, error) {
	root, err := RepoRoot(ctx, workingDir)
	if err != nil {
		return Repo{}, fmt.Errorf("resolve repo root: %w", err)
	}

	gitDir, err := Dir(ctx, workingDir)
	if err != nil {
		return Repo{}, fmt.Errorf("resolve git dir: %w", err)
	}

	return Repo{
		Root:   root,
		GitDir: gitDir,
	}, nil
}

// RepoRoot resolves the repository root from any working directory inside the repo.
func RepoRoot(ctx context.Context, workingDir string) (string, error) {
	output, err := runGit(ctx, workingDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}

	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

// Dir resolves the absolute git dir from any working directory inside the repo.
func Dir(ctx context.Context, workingDir string) (string, error) {
	output, err := runGit(ctx, workingDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --absolute-git-dir: %w", err)
	}

	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

func runGit(ctx context.Context, workingDir string, args ...string) ([]byte, error) {
	return runGitInputEnv(ctx, workingDir, nil, nil, args...)
}

func runGitInput(ctx context.Context, workingDir string, stdin []byte, args ...string) ([]byte, error) {
	return runGitInputEnv(ctx, workingDir, nil, stdin, args...)
}

func runGitInputEnv(ctx context.Context, workingDir string, env map[string]string, stdin []byte, args ...string) ([]byte, error) {
	if workingDir == "" {
		workingDir = "."
	}

	commandArgs := append([]string{"-C", workingDir}, args...)
	//nolint:gosec // Git is invoked with explicit argument lists from internal callers only.
	cmd := exec.CommandContext(ctx, "git", commandArgs...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(env)...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	} else {
		cmd.Stdin = io.Reader(nil)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return nil, fmt.Errorf("run git %s: %w: %s", strings.Join(args, " "), err, stderrText)
		}
		return nil, fmt.Errorf("run git %s: %w", strings.Join(args, " "), err)
	}

	return stdout.Bytes(), nil
}

func flattenEnv(values map[string]string) []string {
	flattened := make([]string, 0, len(values))
	for key, value := range values {
		flattened = append(flattened, key+"="+value)
	}

	return flattened
}
