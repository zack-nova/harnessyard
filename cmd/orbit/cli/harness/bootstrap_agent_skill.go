package harness

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const BootstrapAgentSkillName = "harness-runtime-bootstrap"

// BootstrapAgentSkillSetupInput captures one repo-local bootstrap skill setup request.
type BootstrapAgentSkillSetupInput struct {
	RepoRoot  string
	GitDir    string
	Framework string
	Force     bool
	Remove    bool
}

// BootstrapAgentSkillSetupPlan previews one repo-local bootstrap skill setup mutation.
type BootstrapAgentSkillSetupPlan struct {
	RepoRoot         string                   `json:"repo_root"`
	Framework        string                   `json:"framework"`
	ResolutionSource FrameworkSelectionSource `json:"resolution_source"`
	SkillName        string                   `json:"skill_name"`
	SkillRoot        string                   `json:"skill_root"`
	SkillPath        string                   `json:"skill_path"`
	Action           string                   `json:"action"`
	Changed          bool                     `json:"changed"`
	Remove           bool                     `json:"remove"`
}

// PlanBootstrapAgentSkillSetup previews writing or removing the repo-local bootstrap skill.
func PlanBootstrapAgentSkillSetup(input BootstrapAgentSkillSetupInput) (BootstrapAgentSkillSetupPlan, error) {
	frameworkID, source, err := resolveBootstrapAgentSkillFramework(input)
	if err != nil {
		return BootstrapAgentSkillSetupPlan{}, err
	}
	skillRoot, skillPath, ok := BootstrapAgentSkillTarget(frameworkID, BootstrapAgentSkillName)
	if !ok {
		return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("framework %q does not support bootstrap project skills", frameworkID)
	}

	plan := BootstrapAgentSkillSetupPlan{
		RepoRoot:         input.RepoRoot,
		Framework:        frameworkID,
		ResolutionSource: source,
		SkillName:        BootstrapAgentSkillName,
		SkillRoot:        skillRoot,
		SkillPath:        skillPath,
		Remove:           input.Remove,
	}

	existing, err := os.ReadFile(filepath.Join(input.RepoRoot, filepath.FromSlash(skillPath))) //nolint:gosec // skillPath is framework-mapped repo-local output.
	switch {
	case input.Remove && errors.Is(err, os.ErrNotExist):
		plan.Action = "missing"
		return plan, nil
	case !input.Remove && errors.Is(err, os.ErrNotExist):
		plan.Action = "create"
		plan.Changed = true
		return plan, nil
	case err != nil:
		return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("read bootstrap skill %s: %w", skillPath, err)
	}

	matches := bytes.Equal(bytes.TrimSpace(existing), bytes.TrimSpace(BootstrapAgentSkillData()))
	switch {
	case input.Remove && matches:
		plan.Action = "remove"
		plan.Changed = true
	case input.Remove && input.Force:
		plan.Action = "remove"
		plan.Changed = true
	case input.Remove:
		plan.Action = "conflict"
	case matches:
		plan.Action = "unchanged"
	case input.Force:
		plan.Action = "update"
		plan.Changed = true
	default:
		plan.Action = "conflict"
	}

	return plan, nil
}

// ApplyBootstrapAgentSkillSetup writes or removes the repo-local bootstrap skill.
func ApplyBootstrapAgentSkillSetup(input BootstrapAgentSkillSetupInput) (BootstrapAgentSkillSetupPlan, error) {
	plan, err := PlanBootstrapAgentSkillSetup(input)
	if err != nil {
		return BootstrapAgentSkillSetupPlan{}, err
	}
	if plan.Action == "conflict" {
		if input.Remove {
			return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("bootstrap skill %s has local edits; rerun with --force to remove it", plan.SkillPath)
		}

		return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("bootstrap skill %s has local edits; rerun with --force to replace it", plan.SkillPath)
	}

	absRoot := filepath.Join(input.RepoRoot, filepath.FromSlash(plan.SkillRoot))
	absPath := filepath.Join(input.RepoRoot, filepath.FromSlash(plan.SkillPath))
	switch plan.Action {
	case "create", "update":
		if err := os.MkdirAll(absRoot, 0o755); err != nil {
			return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("create bootstrap skill directory %s: %w", plan.SkillRoot, err)
		}
		if err := contractutil.AtomicWriteFileMode(absPath, BootstrapAgentSkillData(), 0o644); err != nil {
			return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("write bootstrap skill %s: %w", plan.SkillPath, err)
		}
	case "remove":
		if err := os.RemoveAll(absRoot); err != nil {
			return BootstrapAgentSkillSetupPlan{}, fmt.Errorf("remove bootstrap skill %s: %w", plan.SkillRoot, err)
		}
	}

	return plan, nil
}

// BootstrapAgentSkillTarget maps one framework id to the project skill output path.
func BootstrapAgentSkillTarget(frameworkID string, skillName string) (string, string, bool) {
	normalized, ok := NormalizeFrameworkID(frameworkID)
	if !ok {
		return "", "", false
	}
	name := strings.TrimSpace(skillName)
	if name == "" {
		name = BootstrapAgentSkillName
	}
	switch normalized {
	case "codex":
		root := filepath.ToSlash(filepath.Join(".codex", "skills", name))
		return root, filepath.ToSlash(filepath.Join(root, "SKILL.md")), true
	case "claudecode":
		root := filepath.ToSlash(filepath.Join(".claude", "skills", name))
		return root, filepath.ToSlash(filepath.Join(root, "SKILL.md")), true
	case "openclaw":
		root := filepath.ToSlash(filepath.Join("skills", name))
		return root, filepath.ToSlash(filepath.Join(root, "SKILL.md")), true
	default:
		return "", "", false
	}
}

// BootstrapAgentSkillData returns the built-in bootstrap skill payload.
func BootstrapAgentSkillData() []byte {
	return []byte(`---
name: harness-runtime-bootstrap
description: Use when entering, initializing, preparing, or first working in a Harness Yard runtime that may have pending bootstrap guidance.
---

Use this skill when you enter, initialize, prepare, or first work in a Harness Yard runtime that may still have bootstrap guidance.

1. Run ` + "`hyard guide render --target bootstrap --json`" + ` from the runtime root.
2. If no bootstrap guidance is rendered or ` + "`BOOTSTRAP.md`" + ` does not exist afterward, stop this bootstrap flow and continue with the user's normal request.
3. Read ` + "`BOOTSTRAP.md`" + ` and execute the initialization guide it contains.
4. Run ` + "`hyard bootstrap complete --check --json`" + `.
5. Inspect ` + "`removed_paths`" + ` and ` + "`removed_bootstrap_blocks`" + `. Only continue if every removed path is an expected bootstrap-lane runtime artifact or root ` + "`BOOTSTRAP.md`" + ` removal caused by completed bootstrap guidance.
6. If the preview includes unexpected project files, stop and report the preview instead of completing bootstrap.
7. When the preview is safe, run ` + "`hyard bootstrap complete --yes`" + `.
`)
}

func resolveBootstrapAgentSkillFramework(input BootstrapAgentSkillSetupInput) (string, FrameworkSelectionSource, error) {
	if strings.TrimSpace(input.Framework) != "" {
		frameworkID, ok := NormalizeFrameworkID(input.Framework)
		if !ok {
			return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("framework %q is not supported by bootstrap setup; supported frameworks: claudecode, codex, openclaw", input.Framework)
		}
		if _, _, ok := BootstrapAgentSkillTarget(frameworkID, BootstrapAgentSkillName); !ok {
			return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("framework %q does not support bootstrap project skills", frameworkID)
		}

		return frameworkID, FrameworkSelectionSourceExplicitLocal, nil
	}

	if selection, err := LoadFrameworkSelection(input.GitDir); err == nil {
		selectedFrameworks := FrameworkSelectionIDs(selection)
		if len(selectedFrameworks) > 1 {
			return "", FrameworkSelectionSourceUnresolvedConflict, fmt.Errorf("multiple selected frameworks configured (%s); pass one framework explicitly", strings.Join(selectedFrameworks, ", "))
		}
		if len(selectedFrameworks) == 1 {
			frameworkID, ok := NormalizeFrameworkID(selectedFrameworks[0])
			if !ok {
				return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("selected framework %q is not supported by bootstrap setup; pass one of claudecode, codex, or openclaw", selectedFrameworks[0])
			}
			if _, _, ok := BootstrapAgentSkillTarget(frameworkID, BootstrapAgentSkillName); !ok {
				return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("selected framework %q does not support bootstrap project skills", frameworkID)
			}

			return frameworkID, selection.SelectionSource, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("load framework selection: %w", err)
	}

	frameworksFile, err := LoadOptionalFrameworksFile(input.RepoRoot)
	if err != nil {
		return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("load recommended framework: %w", err)
	}
	if strings.TrimSpace(frameworksFile.RecommendedFramework) != "" {
		frameworkID, ok := NormalizeFrameworkID(frameworksFile.RecommendedFramework)
		if !ok {
			return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("recommended framework %q is not supported by bootstrap setup; pass one of claudecode, codex, or openclaw", frameworksFile.RecommendedFramework)
		}
		if _, _, ok := BootstrapAgentSkillTarget(frameworkID, BootstrapAgentSkillName); !ok {
			return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("recommended framework %q does not support bootstrap project skills", frameworkID)
		}

		return frameworkID, FrameworkSelectionSourceRecommendedDefault, nil
	}

	return "", FrameworkSelectionSourceUnresolved, fmt.Errorf("bootstrap setup could not resolve a target framework; run `hyard bootstrap setup <framework>` or `hyard agent use <framework>` first")
}
