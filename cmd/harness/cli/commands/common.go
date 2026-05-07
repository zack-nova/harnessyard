package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	progresspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/progress"
)

type contextKey string

const workingDirContextKey contextKey = "working_dir"
const guidanceComposeInteractiveContextKey contextKey = "guidance_compose_interactive"

// WithWorkingDir injects the working directory used by command tests.
func WithWorkingDir(ctx context.Context, workingDir string) context.Context {
	return context.WithValue(ctx, workingDirContextKey, workingDir)
}

// WithGuidanceComposeInteractive preserves terminal interactivity after tests or wrappers replace command streams.
func WithGuidanceComposeInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, guidanceComposeInteractiveContextKey, true)
}

func guidanceComposeInteractiveFromContext(ctx context.Context) bool {
	interactive, ok := ctx.Value(guidanceComposeInteractiveContextKey).(bool)
	return ok && interactive
}

func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func addPathFlag(cmd *cobra.Command) {
	cmd.Flags().String("path", "", "Resolve the harness root starting from this path")
}

func addProgressFlag(cmd *cobra.Command) {
	cmd.Flags().String("progress", "auto", "Progress output mode: auto, plain, or quiet")
}

func wantJSON(cmd *cobra.Command) (bool, error) {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return false, fmt.Errorf("read json flag: %w", err)
	}

	return jsonOutput, nil
}

func templatePublishStreamIsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func emitJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}

	return nil
}

func progressFromCommand(cmd *cobra.Command) (progresspkg.Emitter, error) {
	rawMode, err := cmd.Flags().GetString("progress")
	if err != nil {
		return progresspkg.Emitter{}, fmt.Errorf("read progress flag: %w", err)
	}

	emitter, err := newProgressEmitter(cmd.ErrOrStderr(), rawMode)
	if err != nil {
		return progresspkg.Emitter{}, err
	}

	return emitter, nil
}

func newProgressEmitter(writer io.Writer, rawMode string) (progresspkg.Emitter, error) {
	emitter, err := progresspkg.NewEmitter(writer, rawMode)
	if err != nil {
		return progresspkg.Emitter{}, fmt.Errorf("create progress emitter: %w", err)
	}

	return emitter, nil
}

func stageProgress(progress progresspkg.Emitter, stage string) error {
	if err := progress.Stage(stage); err != nil {
		return fmt.Errorf("update progress stage %q: %w", stage, err)
	}

	return nil
}

func stageProgressFunc(progress func(string) error, stage string) error {
	if progress == nil {
		return nil
	}
	if err := progress(stage); err != nil {
		return fmt.Errorf("update progress stage %q: %w", stage, err)
	}

	return nil
}

func workingDirFromCommand(cmd *cobra.Command) (string, error) {
	if cmd.Context() != nil {
		if workingDir, ok := cmd.Context().Value(workingDirContextKey).(string); ok && strings.TrimSpace(workingDir) != "" {
			return workingDir, nil
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	return workingDir, nil
}

func pathFromCommand(cmd *cobra.Command) (string, error) {
	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}

	pathValue, err := cmd.Flags().GetString("path")
	if err != nil {
		return "", fmt.Errorf("read path flag: %w", err)
	}
	if strings.TrimSpace(pathValue) == "" {
		return filepath.Clean(workingDir), nil
	}
	if filepath.IsAbs(pathValue) {
		return filepath.Clean(pathValue), nil
	}

	return filepath.Clean(filepath.Join(workingDir, pathValue)), nil
}

func pathArgumentFromCommand(cmd *cobra.Command) (string, error) {
	pathValue, err := cmd.Flags().GetString("path")
	if err != nil {
		return "", fmt.Errorf("read path flag: %w", err)
	}
	if strings.TrimSpace(pathValue) == "" {
		return ".", nil
	}

	return pathValue, nil
}

func absolutePathFromArg(cmd *cobra.Command, value string) (string, error) {
	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}

	return filepath.Clean(filepath.Join(workingDir, value)), nil
}

func repoFromInitCommand(cmd *cobra.Command, createCommand string) (gitpkg.Repo, error) {
	targetPath, err := pathFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, err
	}

	repo, err := gitpkg.DiscoverRepo(cmd.Context(), targetPath)
	if err == nil {
		return repo, nil
	}
	if !gitpkg.IsNotGitRepositoryError(err) {
		return gitpkg.Repo{}, fmt.Errorf("discover git repository: %w", err)
	}

	pathArg, err := pathArgumentFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, err
	}
	if pathArg == "." {
		return gitpkg.Repo{}, fmt.Errorf(
			"current directory is not a Git repository\n"+
				"to start a new harness runtime repo here, run:\n"+
				"  %s %s",
			createCommand,
			shellQuoteArg(pathArg),
		)
	}

	return gitpkg.Repo{}, fmt.Errorf(
		"path %q is not a Git repository\n"+
			"to start a new harness runtime repo there, run:\n"+
			"  %s %s",
		pathArg,
		createCommand,
		shellQuoteArg(pathArg),
	)
}

func shellQuoteArg(value string) string {
	if strings.ContainsAny(value, " \t\n\"'") {
		return fmt.Sprintf("%q", value)
	}

	return value
}
