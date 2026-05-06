package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

type startWriteTarget struct {
	Path string
	Kind string
}

func detectStartWriteConflicts(
	ctx context.Context,
	input StartPlanInput,
	resolution FrameworkResolution,
	frameworkPlan FrameworkPlan,
	bootstrapPlan BootstrapAgentSkillSetupPlan,
) ([]StartWriteConflict, error) {
	conflicts := startBootstrapWriteConflicts(bootstrapPlan)
	selectionConflicts := startFrameworkSelectionWriteConflicts(input, resolution)
	conflicts = append(conflicts, selectionConflicts...)
	activationConflicts := startFrameworkActivationStateWriteConflicts(input, resolution)
	conflicts = append(conflicts, activationConflicts...)

	statusEntries, err := gitpkg.WorktreeStatus(ctx, input.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("load worktree status for Harness Start write preflight: %w", err)
	}

	for _, target := range startWriteTargets(frameworkPlan, bootstrapPlan) {
		for _, status := range statusEntries {
			if !startStatusConflictsWithTarget(status.Path, target.Path) {
				continue
			}
			if startWriteTargetAllowsDirtyStatus(ctx, input, resolution, target) {
				break
			}
			conflicts = append(conflicts, StartWriteConflict{
				Path: target.Path,
				Message: fmt.Sprintf(
					"would write %s, but %s has uncommitted worktree status %s",
					target.Kind,
					status.Path,
					status.Code,
				),
			})
			break
		}
	}

	sort.Slice(conflicts, func(left, right int) bool {
		if conflicts[left].Path != conflicts[right].Path {
			return conflicts[left].Path < conflicts[right].Path
		}
		return conflicts[left].Message < conflicts[right].Message
	})
	return compactStartWriteConflicts(conflicts), nil
}

func startBootstrapWriteConflicts(plan BootstrapAgentSkillSetupPlan) []StartWriteConflict {
	if plan.Action != "conflict" {
		return nil
	}

	return []StartWriteConflict{{
		Path:    plan.SkillPath,
		Message: "bootstrap agent skill has local edits; restore or move them before running Harness Start",
	}}
}

func startFrameworkSelectionWriteConflicts(input StartPlanInput, resolution FrameworkResolution) []StartWriteConflict {
	if strings.TrimSpace(input.FrameworkOverride) == "" {
		return nil
	}

	selection, selectionPath, err := loadStartFrameworkSelectionForConflict(input.GitDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return []StartWriteConflict{{
			Path:    startDisplayPath(input.RepoRoot, selectionPath),
			Message: fmt.Sprintf("repo-local framework selection cannot be safely read: %v", err),
		}}
	}

	selectedFrameworks := FrameworkSelectionIDs(selection)
	normalized := make([]string, 0, len(selectedFrameworks))
	for _, selectedFramework := range selectedFrameworks {
		frameworkID, ok := NormalizeFrameworkID(selectedFramework)
		if !ok {
			normalized = append(normalized, selectedFramework)
			continue
		}
		normalized = append(normalized, frameworkID)
	}
	if len(normalized) == 1 && normalized[0] == resolution.Framework {
		return nil
	}

	return []StartWriteConflict{{
		Path: startDisplayPath(input.RepoRoot, selectionPath),
		Message: fmt.Sprintf(
			"explicit --with %s would overwrite repo-local framework selection %s",
			resolution.Framework,
			strings.Join(selectedFrameworks, ", "),
		),
	}}
}

func startFrameworkActivationStateWriteConflicts(input StartPlanInput, resolution FrameworkResolution) []StartWriteConflict {
	if strings.TrimSpace(resolution.Framework) == "" {
		return nil
	}

	activationPath := FrameworkActivationPath(input.GitDir, resolution.Framework)
	activation, err := loadFrameworkActivationAtPath(activationPath)
	if err == nil {
		return startFrameworkActivationStateOwnershipConflicts(input, activationPath, activation)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return []StartWriteConflict{{
			Path:    startDisplayPath(input.RepoRoot, activationPath),
			Message: fmt.Sprintf("repo-local framework activation state cannot be safely read: %v", err),
		}}
	}

	legacyPath := legacyFrameworkActivationPath(input.GitDir, resolution.Framework)
	activation, err = loadFrameworkActivationAtPath(legacyPath)
	if err == nil {
		return startFrameworkActivationStateOwnershipConflicts(input, legacyPath, activation)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return []StartWriteConflict{{
		Path:    startDisplayPath(input.RepoRoot, legacyPath),
		Message: fmt.Sprintf("legacy repo-local framework activation state cannot be safely read: %v", err),
	}}
}

func startFrameworkActivationStateOwnershipConflicts(input StartPlanInput, path string, activation FrameworkActivation) []StartWriteConflict {
	if filepath.Clean(activation.RepoRoot) == filepath.Clean(input.RepoRoot) {
		return nil
	}

	return []StartWriteConflict{{
		Path: startDisplayPath(input.RepoRoot, path),
		Message: fmt.Sprintf(
			"repo-local framework activation state belongs to %s, not %s",
			activation.RepoRoot,
			input.RepoRoot,
		),
	}}
}

func loadStartFrameworkSelectionForConflict(gitDir string) (FrameworkSelection, string, error) {
	selectionPath := FrameworkSelectionPath(gitDir)
	selection, err := loadFrameworkSelectionAtPath(selectionPath)
	if err == nil {
		return selection, selectionPath, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return FrameworkSelection{}, selectionPath, err
	}

	legacyPath := legacyFrameworkSelectionPath(gitDir)
	selection, err = loadFrameworkSelectionAtPath(legacyPath)
	if err != nil {
		return FrameworkSelection{}, legacyPath, err
	}

	return selection, legacyPath, nil
}

func startWriteTargets(frameworkPlan FrameworkPlan, bootstrapPlan BootstrapAgentSkillSetupPlan) []startWriteTarget {
	targets := make([]startWriteTarget, 0,
		len(frameworkPlan.ProjectOutputs)+
			len(frameworkPlan.RecommendedProjectOutputs)+
			1,
	)
	for _, output := range frameworkPlan.ProjectOutputs {
		targets = append(targets, startWriteTarget{
			Path: output.Path,
			Kind: "framework activation guidance output",
		})
	}
	for _, output := range frameworkPlan.RecommendedProjectOutputs {
		if strings.HasPrefix(output.Path, "~/") || strings.TrimSpace(output.Path) == "" {
			continue
		}
		targets = append(targets, startWriteTarget{
			Path: output.Path,
			Kind: "framework activation output",
		})
	}
	if bootstrapPlan.Changed && strings.TrimSpace(bootstrapPlan.SkillPath) != "" {
		targets = append(targets, startWriteTarget{
			Path: bootstrapPlan.SkillPath,
			Kind: "bootstrap agent skill",
		})
	}

	return targets
}

func startWriteTargetAllowsDirtyStatus(ctx context.Context, input StartPlanInput, resolution FrameworkResolution, target startWriteTarget) bool {
	if isRootGuidancePath(target.Path) {
		return startRootGuidanceTargetPreflightOK(ctx, input, target.Path)
	}

	return startFrameworkOutputOwned(input, resolution, target.Path)
}

func startRootGuidanceTargetPreflightOK(ctx context.Context, input StartPlanInput, path string) bool {
	runtimeFile, err := LoadRuntimeFile(input.RepoRoot)
	if err != nil {
		return false
	}

	var target runtimeGuidanceTarget
	switch path {
	case rootAgentsPath:
		target = agentGuidanceTarget()
	case rootHumansPath:
		target = humanGuidanceTarget()
	case rootBootstrapPath:
		target = bootstrapGuidanceTarget()
	default:
		return false
	}

	_, _, err = prepareRuntimeGuidanceTarget(ctx, input.RepoRoot, input.GitDir, runtimeFile.Members, false, target)
	return err == nil
}

func startFrameworkOutputOwned(input StartPlanInput, resolution FrameworkResolution, path string) bool {
	if strings.TrimSpace(resolution.Framework) == "" {
		return false
	}

	activation, err := LoadFrameworkActivation(input.GitDir, resolution.Framework)
	if err != nil {
		return false
	}
	if filepath.Clean(activation.RepoRoot) != filepath.Clean(input.RepoRoot) {
		return false
	}

	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if output.Path != path {
			continue
		}
		owned, err := frameworkOutputOwned(output)
		return err == nil && owned
	}

	return false
}

func startStatusConflictsWithTarget(statusPath string, targetPath string) bool {
	statusPath = strings.Trim(strings.TrimSpace(statusPath), "/")
	targetPath = strings.Trim(strings.TrimSpace(targetPath), "/")
	if statusPath == "" || targetPath == "" {
		return false
	}
	if statusPath == targetPath {
		return true
	}
	if strings.HasPrefix(statusPath, targetPath+"/") {
		return true
	}
	return strings.HasPrefix(targetPath, statusPath+"/")
}

func compactStartWriteConflicts(conflicts []StartWriteConflict) []StartWriteConflict {
	compacted := make([]StartWriteConflict, 0, len(conflicts))
	seen := map[string]struct{}{}
	for _, conflict := range conflicts {
		key := conflict.Path + "\x00" + conflict.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		compacted = append(compacted, conflict)
	}

	return compacted
}

func startDisplayPath(repoRoot string, path string) string {
	relative, err := filepath.Rel(repoRoot, path)
	if err == nil && relative != "." && relative != "" && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." {
		return filepath.ToSlash(relative)
	}

	return filepath.ToSlash(path)
}

func startWriteConflictsError(conflicts []StartWriteConflict) error {
	if len(conflicts) == 0 {
		return nil
	}

	parts := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		if conflict.Path == "" {
			parts = append(parts, conflict.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", conflict.Path, conflict.Message))
	}

	return fmt.Errorf("write conflicts block Harness Start: %s", strings.Join(parts, "; "))
}
