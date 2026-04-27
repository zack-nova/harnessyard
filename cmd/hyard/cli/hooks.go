package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
)

type hookRunExitError struct {
	code int
}

func (err hookRunExitError) Error() string {
	return fmt.Sprintf("hook exited with code %d", err.code)
}

func (err hookRunExitError) ExitCode() int {
	return err.code
}

func newHooksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Run Harness-managed agent hooks",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newHooksRunCommand())

	return cmd
}

func newHooksRunCommand() *cobra.Command {
	var root string
	var target string
	var hook string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one Harness-managed hook through the unified protocol bridge",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoRoot, err := hooksRunRootFromCommand(cmd, root)
			if err != nil {
				return err
			}
			if strings.TrimSpace(target) == "" {
				return fmt.Errorf("--target must not be empty")
			}
			if strings.TrimSpace(hook) == "" {
				return fmt.Errorf("--hook must not be empty")
			}
			nativeStdin, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("read native hook stdin: %w", err)
			}
			result, err := harnesspkg.RunAgentHook(cmd.Context(), harnesspkg.AgentHookRunInput{
				RepoRoot:    repoRoot,
				Target:      target,
				HookID:      hook,
				NativeStdin: nativeStdin,
			})
			if err != nil {
				return fmt.Errorf("run unified hook: %w", err)
			}
			if len(result.Stdout) > 0 {
				if _, err := cmd.OutOrStdout().Write(result.Stdout); err != nil {
					return fmt.Errorf("write hook stdout: %w", err)
				}
			}
			if len(result.Stderr) > 0 {
				if _, err := cmd.ErrOrStderr().Write(result.Stderr); err != nil {
					return fmt.Errorf("write hook stderr: %w", err)
				}
			}
			if result.ExitCode != 0 {
				return hookRunExitError{code: result.ExitCode}
			}

			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Repository root containing .harness/agents/config.yaml")
	cmd.Flags().StringVar(&target, "target", "", "Agent target id such as codex, claude, or openclaw")
	cmd.Flags().StringVar(&hook, "hook", "", "Unified hook id to run")

	return cmd
}

func hooksRunRootFromCommand(cmd *cobra.Command, rawRoot string) (string, error) {
	if strings.TrimSpace(rawRoot) != "" {
		if filepath.IsAbs(rawRoot) {
			return filepath.Clean(rawRoot), nil
		}
		workingDir, err := hyardWorkingDirFromCommand(cmd)
		if err != nil {
			return "", err
		}

		return filepath.Clean(filepath.Join(workingDir, rawRoot)), nil
	}

	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}

	return filepath.Clean(workingDir), nil
}
