package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const adoptionCheckSchemaVersion = "1.0"

type adoptCheckOutput struct {
	SchemaVersion          string                  `json:"schema_version"`
	RepoRoot               string                  `json:"repo_root"`
	Mode                   string                  `json:"mode"`
	Adoptable              bool                    `json:"adoptable"`
	ExistingHarnessRuntime bool                    `json:"existing_harness_runtime"`
	DirtyWorktree          adoptCheckDirtyWorktree `json:"dirty_worktree"`
	AdoptedOrbit           adoptCheckAdoptedOrbit  `json:"adopted_orbit"`
	Frameworks             adoptCheckFrameworks    `json:"frameworks"`
	Diagnostics            []adoptCheckDiagnostic  `json:"diagnostics"`
	NextActions            []adoptCheckNextAction  `json:"next_actions"`
}

type adoptCheckDirtyWorktree struct {
	Dirty bool     `json:"dirty"`
	Paths []string `json:"paths"`
}

type adoptCheckAdoptedOrbit struct {
	ID          string `json:"id"`
	DerivedFrom string `json:"derived_from"`
}

type adoptCheckFrameworks struct {
	Recommended string                `json:"recommended,omitempty"`
	Detected    []adoptCheckFramework `json:"detected"`
	Unsupported []adoptCheckFramework `json:"unsupported,omitempty"`
}

type adoptCheckFramework struct {
	ID       string               `json:"id"`
	Status   string               `json:"status"`
	Evidence []adoptCheckEvidence `json:"evidence"`
}

type adoptCheckEvidence struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type adoptCheckDiagnostic struct {
	Code     string               `json:"code"`
	Severity string               `json:"severity"`
	Message  string               `json:"message"`
	Evidence []adoptCheckEvidence `json:"evidence,omitempty"`
}

type adoptCheckNextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

func newAdoptCommand() *cobra.Command {
	var check bool
	var orbitID string

	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Inspect or convert an Ordinary Repository into a Harness Runtime",
		Long: "Inspect or convert an Ordinary Repository into a Harness Runtime.\n" +
			"The first Adoption slice supports `--check --json` to preview adoptability\n" +
			"without mutating the repository.",
		Example: "" +
			"  hyard adopt --check --json\n" +
			"  hyard adopt --check --json --orbit workspace\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !check {
				return fmt.Errorf("adoption write mode is not available yet; use --check to preview without mutating")
			}

			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}

			output, err := buildAdoptCheckOutput(cmd, orbitID)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitHyardJSON(cmd, output)
			}

			return printAdoptCheckText(cmd, output)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Inspect Adoption readiness without mutating")
	cmd.Flags().StringVar(&orbitID, "orbit", "", "Adopted Orbit id to use instead of deriving one from the repository name")
	addHyardJSONFlag(cmd)

	return cmd
}

func buildAdoptCheckOutput(cmd *cobra.Command, explicitOrbitID string) (adoptCheckOutput, error) {
	workingDir, err := hyardWorkingDirFromCommand(cmd)
	if err != nil {
		return adoptCheckOutput{}, err
	}
	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err != nil {
		return adoptCheckOutput{}, fmt.Errorf("discover git repository: %w", err)
	}

	orbitID, derivedFrom, err := adoptCheckOrbitID(repo.Root, explicitOrbitID)
	if err != nil {
		return adoptCheckOutput{}, err
	}
	dirtyWorktree, err := inspectAdoptCheckDirtyWorktree(cmd, repo.Root)
	if err != nil {
		return adoptCheckOutput{}, err
	}
	frameworks, err := inspectAdoptCheckFrameworks(repo.Root)
	if err != nil {
		return adoptCheckOutput{}, err
	}

	output := adoptCheckOutput{
		SchemaVersion:          adoptionCheckSchemaVersion,
		RepoRoot:               repo.Root,
		Mode:                   "check",
		Adoptable:              true,
		ExistingHarnessRuntime: false,
		DirtyWorktree:          dirtyWorktree,
		AdoptedOrbit: adoptCheckAdoptedOrbit{
			ID:          orbitID,
			DerivedFrom: derivedFrom,
		},
		Frameworks:  frameworks,
		Diagnostics: []adoptCheckDiagnostic{},
		NextActions: []adoptCheckNextAction{},
	}
	if dirtyWorktree.Dirty {
		output.Diagnostics = append(output.Diagnostics, adoptCheckDiagnostic{
			Code:     "dirty_worktree",
			Severity: "info",
			Message:  "dirty worktree is allowed in adoption check mode",
			Evidence: adoptCheckWorktreeEvidence(dirtyWorktree.Paths),
		})
	}
	output.Diagnostics = append(output.Diagnostics, adoptCheckUnsupportedFootprintDiagnostics(frameworks.Unsupported)...)

	manifest, err := harnesspkg.LoadManifestFile(repo.Root)
	if err == nil && manifest.Kind == harnesspkg.ManifestKindRuntime {
		output.Adoptable = false
		output.ExistingHarnessRuntime = true
		output.Diagnostics = append(output.Diagnostics, adoptCheckDiagnostic{
			Code:     "existing_harness_runtime",
			Severity: "error",
			Message:  "existing Harness Runtime cannot be adopted again",
			Evidence: []adoptCheckEvidence{
				{Kind: "harness_manifest", Path: harnesspkg.ManifestRepoPath()},
			},
		})
		output.NextActions = append(output.NextActions, adoptCheckNextAction{
			Command: "hyard layout optimize",
			Reason:  "existing Harness Runtimes should use Layout Optimization",
		})
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return adoptCheckOutput{}, fmt.Errorf("inspect harness manifest: %w", err)
	}

	return output, nil
}

func inspectAdoptCheckFrameworks(repoRoot string) (adoptCheckFrameworks, error) {
	frameworks := adoptCheckFrameworks{
		Detected:    []adoptCheckFramework{},
		Unsupported: []adoptCheckFramework{},
	}

	claudeCode, err := inspectAdoptCheckProjectFootprint(repoRoot, "claudecode", []string{
		"CLAUDE.md",
		"CLAUDE.local.md",
		".claude/settings.json",
		".claude/commands",
		".claude/skills",
	})
	if err != nil {
		return adoptCheckFrameworks{}, err
	}
	if len(claudeCode.Evidence) > 0 {
		claudeCode.Status = "unsupported"
		frameworks.Detected = append(frameworks.Detected, claudeCode)
		frameworks.Unsupported = append(frameworks.Unsupported, claudeCode)
	}

	codex, err := inspectAdoptCheckProjectFootprint(repoRoot, "codex", []string{
		".codex/config.toml",
		".codex/hooks.json",
		".codex/prompts",
		".codex/skills",
	})
	if err != nil {
		return adoptCheckFrameworks{}, err
	}
	if len(codex.Evidence) > 0 {
		codex.Status = "supported"
		frameworks.Recommended = "codex"
		frameworks.Detected = append(frameworks.Detected, codex)
	}

	openClaw, err := inspectAdoptCheckProjectFootprint(repoRoot, "openclaw", []string{
		".openclaw/openclaw.json",
		".openclaw/commands",
		".openclaw/skills",
		"OPENCLAW.md",
	})
	if err != nil {
		return adoptCheckFrameworks{}, err
	}
	if len(openClaw.Evidence) > 0 {
		openClaw.Status = "unsupported"
		frameworks.Detected = append(frameworks.Detected, openClaw)
		frameworks.Unsupported = append(frameworks.Unsupported, openClaw)
	}

	return frameworks, nil
}

func adoptCheckUnsupportedFootprintDiagnostics(unsupported []adoptCheckFramework) []adoptCheckDiagnostic {
	diagnostics := make([]adoptCheckDiagnostic, 0, len(unsupported))
	for _, framework := range unsupported {
		diagnostics = append(diagnostics, adoptCheckDiagnostic{
			Code:     "unsupported_agent_footprint",
			Severity: "warning",
			Message:  adoptCheckUnsupportedFootprintMessage(framework.ID),
			Evidence: framework.Evidence,
		})
	}

	return diagnostics
}

func adoptCheckUnsupportedFootprintMessage(frameworkID string) string {
	switch frameworkID {
	case "claudecode":
		return "Claude Code project footprint is detected but unsupported by first-version Adoption"
	case "openclaw":
		return "OpenClaw project footprint is detected but unsupported by first-version Adoption"
	default:
		return fmt.Sprintf("%s project footprint is detected but unsupported by first-version Adoption", frameworkID)
	}
}

func inspectAdoptCheckProjectFootprint(repoRoot string, frameworkID string, repoPaths []string) (adoptCheckFramework, error) {
	framework := adoptCheckFramework{
		ID:       frameworkID,
		Evidence: []adoptCheckEvidence{},
	}
	for _, repoPath := range repoPaths {
		absolutePath := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if _, err := os.Lstat(absolutePath); err == nil {
			framework.Evidence = append(framework.Evidence, adoptCheckEvidence{
				Kind: "project_footprint",
				Path: repoPath,
			})
		} else if !errors.Is(err, os.ErrNotExist) {
			return adoptCheckFramework{}, fmt.Errorf("inspect %s project footprint %s: %w", frameworkID, repoPath, err)
		}
	}
	sort.Slice(framework.Evidence, func(left, right int) bool {
		return framework.Evidence[left].Path < framework.Evidence[right].Path
	})

	return framework, nil
}

func adoptCheckWorktreeEvidence(paths []string) []adoptCheckEvidence {
	evidence := make([]adoptCheckEvidence, 0, len(paths))
	for _, path := range paths {
		evidence = append(evidence, adoptCheckEvidence{
			Kind: "worktree_status",
			Path: path,
		})
	}

	return evidence
}

func adoptCheckOrbitID(repoRoot string, explicitOrbitID string) (string, string, error) {
	if explicitOrbitID != "" {
		if err := ids.ValidateOrbitID(explicitOrbitID); err != nil {
			return "", "", fmt.Errorf("validate --orbit: %w", err)
		}
		return explicitOrbitID, "flag", nil
	}

	return harnesspkg.DefaultHarnessIDForPath(repoRoot), "repository_name", nil
}

func inspectAdoptCheckDirtyWorktree(cmd *cobra.Command, repoRoot string) (adoptCheckDirtyWorktree, error) {
	status, err := gitpkg.WorktreeStatus(cmd.Context(), repoRoot)
	if err != nil {
		return adoptCheckDirtyWorktree{}, fmt.Errorf("inspect worktree status: %w", err)
	}

	paths := make([]string, 0, len(status))
	for _, entry := range status {
		paths = append(paths, entry.Path)
	}

	return adoptCheckDirtyWorktree{
		Dirty: len(paths) > 0,
		Paths: paths,
	}, nil
}

func printAdoptCheckText(cmd *cobra.Command, output adoptCheckOutput) error {
	if output.Adoptable {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "adoptable: true\nadopted_orbit: %s\n", output.AdoptedOrbit.ID)
		if err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
		return nil
	}

	_, err := fmt.Fprintln(cmd.OutOrStdout(), "adoptable: false")
	if err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	return nil
}
