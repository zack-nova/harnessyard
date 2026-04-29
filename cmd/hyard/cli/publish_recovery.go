package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	orbitcommands "github.com/zack-nova/harnessyard/cmd/orbit/cli/commands"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type hyardPublishRecoveryDecision string

const (
	hyardPublishRecoveryContinue hyardPublishRecoveryDecision = "continue"
	hyardPublishRecoveryStop     hyardPublishRecoveryDecision = "stop"
)

type hyardPublishRecoveryChoice string

const (
	hyardPublishRecoveryChoiceAll        hyardPublishRecoveryChoice = "all"
	hyardPublishRecoveryChoicePrepare    hyardPublishRecoveryChoice = "prepare"
	hyardPublishRecoveryChoiceCheckpoint hyardPublishRecoveryChoice = "checkpoint"
	hyardPublishRecoveryChoiceAbort      hyardPublishRecoveryChoice = "abort"
)

func shouldOfferHyardPublishInteractiveRecovery(cmd *cobra.Command, jsonOutput bool) bool {
	if jsonOutput {
		return false
	}
	return hyardPublishStreamIsTerminal(cmd.InOrStdin()) && hyardPublishStreamIsTerminal(cmd.ErrOrStderr())
}

func hyardPublishStreamIsTerminal(stream any) bool {
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

func runHyardPublishInteractiveRecovery(ctx context.Context, cmd *cobra.Command, packageName string) (hyardPublishRecoveryDecision, error) {
	output, err := buildHyardOrbitPrepareOutput(ctx, cmd, packageName)
	if err != nil {
		return hyardPublishRecoveryStop, err
	}
	if output.Ready {
		return hyardPublishRecoveryContinue, nil
	}

	if hyardPublishStreamIsTerminal(cmd.InOrStdin()) && hyardPublishStreamIsTerminal(cmd.ErrOrStderr()) {
		cmd.SetContext(orbitcommands.WithTemplatePublishInteractive(cmd.Context()))
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	cmd.SetIn(reader)
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), formatHyardPublishRecoveryPrompt(output)); err != nil {
		return hyardPublishRecoveryStop, fmt.Errorf("write publish recovery prompt: %w", err)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return hyardPublishRecoveryStop, fmt.Errorf("read publish recovery choice: %w", err)
	}
	if errors.Is(err, io.EOF) && line == "" {
		return hyardPublishRecoveryStop, fmt.Errorf("read publish recovery choice: %w", err)
	}

	choice, err := parseHyardPublishRecoveryChoice(line)
	if err != nil {
		return hyardPublishRecoveryStop, err
	}

	switch choice {
	case hyardPublishRecoveryChoiceAll:
		if _, err := runHyardOrbitPrepareMutating(ctx, cmd, packageName, false); err != nil {
			return hyardPublishRecoveryStop, err
		}
		if _, err := runHyardOrbitCheckpointInteractive(ctx, cmd, packageName, "Update "+packageName, true, true); err != nil {
			return hyardPublishRecoveryStop, err
		}
		return hyardPublishRecoveryContinue, nil
	case hyardPublishRecoveryChoicePrepare:
		output, err := runHyardOrbitPrepareMutating(ctx, cmd, packageName, false)
		if err != nil {
			return hyardPublishRecoveryStop, err
		}
		if err := printHyardOrbitPrepareText(cmd, output); err != nil {
			return hyardPublishRecoveryStop, err
		}
		return hyardPublishRecoveryStop, nil
	case hyardPublishRecoveryChoiceCheckpoint:
		output, err := runHyardOrbitCheckpointInteractive(ctx, cmd, packageName, "Update "+packageName, false, false)
		if err != nil {
			return hyardPublishRecoveryStop, err
		}
		if err := printHyardOrbitCheckpointText(cmd, output); err != nil {
			return hyardPublishRecoveryStop, err
		}
		return hyardPublishRecoveryStop, nil
	case hyardPublishRecoveryChoiceAbort:
		return hyardPublishRecoveryStop, fmt.Errorf("publish canceled for package %q", packageName)
	default:
		return hyardPublishRecoveryStop, fmt.Errorf("unknown publish recovery choice %q", choice)
	}
}

func parseHyardPublishRecoveryChoice(raw string) (hyardPublishRecoveryChoice, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "y", "yes":
		return hyardPublishRecoveryChoiceAll, nil
	case "p", "prepare":
		return hyardPublishRecoveryChoicePrepare, nil
	case "c", "checkpoint":
		return hyardPublishRecoveryChoiceCheckpoint, nil
	case "q", "n", "no", "abort":
		return hyardPublishRecoveryChoiceAbort, nil
	default:
		return "", fmt.Errorf("publish recovery choice must be enter, p, c, or q")
	}
}

func runHyardOrbitCheckpointInteractive(ctx context.Context, cmd *cobra.Command, packageName string, message string, allowNoop bool, trackNew bool) (orbitCheckpointOutput, error) {
	packageCtx, err := loadHyardOrbitPackageContext(ctx, cmd, packageName, "publish checkpoint")
	if err != nil {
		return orbitCheckpointOutput{}, err
	}
	plan, err := buildHyardOrbitCheckpointPlan(ctx, packageCtx)
	if err != nil {
		return orbitCheckpointOutput{}, err
	}

	output := orbitCheckpointOutput{
		RepoRoot:             packageCtx.RepoRoot,
		PackageName:          packageCtx.PackageName,
		OrbitID:              packageCtx.OrbitID,
		Ready:                !plan.Required && !plan.Blocked,
		Blocked:              plan.Blocked,
		CandidatePaths:       append([]string(nil), plan.CandidatePaths...),
		BlockedPaths:         append([]string(nil), plan.BlockedPaths...),
		UntrackedExportPaths: append([]string(nil), plan.UntrackedExportPaths...),
	}

	if trackNew && len(plan.UntrackedExportPaths) > 0 && len(plan.BlockedPaths) == 0 {
		prompter := orbittemplate.LineConfirmPrompter{
			Reader: cmd.InOrStdin(),
			Writer: cmd.ErrOrStderr(),
		}
		confirmed, err := prompter.Confirm(ctx, formatHyardOrbitTrackNewPrompt(packageCtx.PackageName, plan.UntrackedExportPaths))
		if err != nil {
			return output, fmt.Errorf("confirm tracking new package files: %w", err)
		}
		if !confirmed {
			return output, fmt.Errorf("checkpoint canceled for package %q; new package files were not tracked", packageCtx.PackageName)
		}
		plan = includeHyardCheckpointUntrackedExportPaths(plan)
		output.CandidatePaths = append([]string(nil), plan.CandidatePaths...)
		output.UntrackedExportPaths = nil
		output.Blocked = plan.Blocked
		output.Ready = !plan.Required && !plan.Blocked
	}
	if plan.Blocked {
		return output, formatHyardOrbitCheckpointBlockedError(packageCtx.PackageName, plan)
	}
	if len(plan.CandidatePaths) == 0 {
		if allowNoop {
			return output, nil
		}
		return output, fmt.Errorf("no package changes to checkpoint for %q", packageCtx.PackageName)
	}
	if strings.TrimSpace(message) == "" {
		return output, fmt.Errorf("checkpoint requires -m/--message")
	}

	prompter := orbittemplate.LineConfirmPrompter{
		Reader: cmd.InOrStdin(),
		Writer: cmd.ErrOrStderr(),
	}
	confirmed, err := prompter.Confirm(ctx, formatHyardOrbitCheckpointPrompt(packageCtx.PackageName, plan.CandidatePaths, message))
	if err != nil {
		return output, fmt.Errorf("confirm checkpoint commit: %w", err)
	}
	if !confirmed {
		return output, fmt.Errorf("checkpoint canceled for package %q; no commit was created", packageCtx.PackageName)
	}

	if err := gitpkg.StageAllPathspec(ctx, packageCtx.RepoRoot, plan.CandidatePaths); err != nil {
		return output, fmt.Errorf("stage package checkpoint paths: %w", err)
	}
	if err := gitpkg.CommitPathspec(ctx, packageCtx.RepoRoot, plan.CandidatePaths, message); err != nil {
		return output, fmt.Errorf("create package checkpoint commit: %w", err)
	}
	commit, err := gitpkg.HeadCommit(ctx, packageCtx.RepoRoot)
	if err != nil {
		return output, fmt.Errorf("resolve checkpoint commit: %w", err)
	}
	output.Committed = true
	output.Commit = commit
	output.Ready = true

	return output, nil
}

func formatHyardPublishRecoveryPrompt(output orbitPrepareOutput) string {
	lines := []string{
		fmt.Sprintf("Orbit package %q is not ready to publish.", output.PackageName),
		"",
		"Required before publish:",
	}
	if output.Schema.MigrationRequired {
		lines = append(lines, fmt.Sprintf("  - Normalize legacy top-level rules to behavior: .harness/orbits/%s.yaml", output.OrbitID))
	}
	if output.ContentHints.DriftDetected {
		if output.ContentHints.BackfillAllowed {
			lines = append(lines, fmt.Sprintf("  - Apply %d content hint(s): %s", output.ContentHints.HintCount, strings.Join(output.ContentHints.HintPaths, " ")))
		} else {
			lines = append(lines, "  - Review content hint diagnostics before prepare can mutate")
		}
	}
	for _, artifact := range output.Guidance.Artifacts {
		if artifact.NeedsSave {
			lines = append(lines, fmt.Sprintf("  - Save guide changes from %s", hyardPublishGuidanceArtifactPath(artifact.Target)))
		}
	}
	if output.LocalSkills.DetectedCount > 0 {
		lines = append(lines, fmt.Sprintf("  - Review detected local skill roots: %s", strings.Join(output.LocalSkills.DetectedRoots, " ")))
	}
	if len(output.RemoteSkills.Diagnostics) > 0 {
		lines = append(lines, "  - Review remote skill dependency diagnostics")
	}
	if len(output.Checkpoint.UntrackedExportPaths) > 0 {
		lines = append(lines, fmt.Sprintf("  - Track new package files: %s", strings.Join(output.Checkpoint.UntrackedExportPaths, " ")))
	}
	if len(output.Checkpoint.BlockedPaths) > 0 {
		lines = append(lines, fmt.Sprintf("  - Resolve unrelated tracked paths: %s", strings.Join(output.Checkpoint.BlockedPaths, " ")))
	}
	if len(output.Checkpoint.CandidatePaths) > 0 {
		lines = append(lines, fmt.Sprintf("  - Checkpoint tracked package paths: %s", strings.Join(output.Checkpoint.CandidatePaths, " ")))
	}
	lines = append(lines, "", "Continue?")
	if hyardPublishRecoveryNeedsPrepare(output) {
		lines = append(lines,
			"  [Enter] prepare, track, checkpoint, then publish",
			"  p       prepare only",
			"  q       abort",
			"> ",
		)
		return strings.Join(lines, "\n")
	}
	lines = append(lines,
		"  [Enter] prepare, track, checkpoint, then publish",
		"  p       prepare only",
		"  c       checkpoint only",
		"  q       abort",
		"> ",
	)
	return strings.Join(lines, "\n")
}

func hyardPublishRecoveryNeedsPrepare(output orbitPrepareOutput) bool {
	return output.Schema.MigrationRequired || output.ContentHints.DriftDetected || output.Guidance.DriftDetected
}

func formatHyardOrbitCheckpointPrompt(packageName string, paths []string, message string) string {
	lines := []string{
		fmt.Sprintf("Create checkpoint commit %q for package %q?", message, packageName),
		"Paths:",
	}
	for _, path := range paths {
		lines = append(lines, "  - "+path)
	}
	lines = append(lines, "continue? [y/N] ")
	return strings.Join(lines, "\n")
}

func formatHyardOrbitTrackNewPrompt(packageName string, paths []string) string {
	noun := "new package files"
	if len(paths) == 1 {
		noun = "new package file"
	}
	lines := []string{
		fmt.Sprintf("Track %d %s for package %q?", len(paths), noun, packageName),
		"Paths:",
	}
	for _, path := range paths {
		lines = append(lines, "  - "+path)
	}
	lines = append(lines, "continue? [y/N] ")
	return strings.Join(lines, "\n")
}

func hyardPublishGuidanceArtifactPath(target string) string {
	switch target {
	case "agents":
		return "AGENTS.md"
	case "humans":
		return "HUMANS.md"
	case "bootstrap":
		return "BOOTSTRAP.md"
	default:
		return target
	}
}
