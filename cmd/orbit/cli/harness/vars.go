package harness

import (
	"context"
	"fmt"
	"os"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

// LoadVarsFile reads, decodes, and validates .harness/vars.yaml.
func LoadVarsFile(repoRoot string) (bindings.VarsFile, error) {
	file, err := bindings.LoadVarsFileAtPath(VarsPath(repoRoot))
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("load harness vars file: %w", err)
	}

	return file, nil
}

// LoadVarsFileWorktreeOrHEAD reads .harness/vars.yaml from the worktree when visible
// and falls back to HEAD when sparse-checkout currently hides it.
func LoadVarsFileWorktreeOrHEAD(ctx context.Context, repoRoot string) (bindings.VarsFile, error) {
	file, err := bindings.LoadVarsFileWorktreeOrHEADAtRepoPath(ctx, repoRoot, VarsRepoPath())
	if err != nil {
		return bindings.VarsFile{}, fmt.Errorf("load harness vars file: %w", err)
	}

	return file, nil
}

// ValidateVarsFile reads and validates the canonical .harness/vars.yaml file.
func ValidateVarsFile(repoRoot string) error {
	data, err := os.ReadFile(VarsPath(repoRoot))
	if err != nil {
		return fmt.Errorf("read %s: %w", VarsRepoPath(), err)
	}
	if _, err := bindings.ParseVarsData(data); err != nil {
		return fmt.Errorf("validate %s: %w", VarsRepoPath(), err)
	}

	return nil
}

// WriteVarsFile validates and writes .harness/vars.yaml with stable field ordering.
func WriteVarsFile(repoRoot string, file bindings.VarsFile) (string, error) {
	filename, err := bindings.WriteVarsFileAtPath(VarsPath(repoRoot), file)
	if err != nil {
		return "", fmt.Errorf("write harness vars file: %w", err)
	}

	return filename, nil
}
