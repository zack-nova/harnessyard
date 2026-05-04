package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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
	Candidates             []adoptCheckCandidate   `json:"candidates"`
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
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Detail string `json:"detail,omitempty"`
}

type adoptCheckCandidate struct {
	Path                  string                     `json:"path"`
	Kind                  string                     `json:"kind"`
	Shape                 string                     `json:"shape"`
	RecommendedMemberRole string                     `json:"recommended_member_role,omitempty"`
	RoleConfirmation      adoptCheckRoleConfirmation `json:"role_confirmation,omitempty"`
	Evidence              []adoptCheckEvidence       `json:"evidence"`
}

type adoptCheckRoleConfirmation struct {
	Required               bool     `json:"required"`
	BatchAcceptRecommended bool     `json:"batch_accept_recommended"`
	EditableRoles          []string `json:"editable_roles,omitempty"`
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
	candidates, candidateDiagnostics, err := inspectAdoptCheckGuidanceCandidates(cmd, repo.Root)
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
		Candidates:  candidates,
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
	output.Diagnostics = append(output.Diagnostics, candidateDiagnostics...)

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

func inspectAdoptCheckGuidanceCandidates(cmd *cobra.Command, repoRoot string) ([]adoptCheckCandidate, []adoptCheckDiagnostic, error) {
	trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load tracked files for guidance discovery: %w", err)
	}
	tracked := make(map[string]struct{}, len(trackedFiles))
	for _, path := range trackedFiles {
		tracked[path] = struct{}{}
	}
	if _, ok := tracked["AGENTS.md"]; !ok {
		return []adoptCheckCandidate{}, []adoptCheckDiagnostic{}, nil
	}

	candidates := []adoptCheckCandidate{
		{
			Path:  "AGENTS.md",
			Kind:  "root_agent_guidance",
			Shape: "file",
			Evidence: []adoptCheckEvidence{
				{Kind: "root_agent_guidance", Path: "AGENTS.md"},
			},
		},
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, "AGENTS.md"))
	if err != nil {
		return nil, nil, fmt.Errorf("read root agent guidance: %w", err)
	}
	seen := map[string]struct{}{
		"AGENTS.md": {},
	}
	rejected := map[string]struct{}{}
	diagnostics := []adoptCheckDiagnostic{}
	guidanceText := string(content)
	references := append(
		parseAdoptCheckMarkdownLinks(guidanceText),
		parseAdoptCheckPathMentions(adoptCheckMarkdownLinkPattern.ReplaceAllString(guidanceText, " "))...,
	)
	for _, reference := range references {
		if reference.Unsafe {
			if _, rejectedAlready := rejected[reference.Path]; rejectedAlready {
				continue
			}
			rejected[reference.Path] = struct{}{}
			diagnostics = append(diagnostics, adoptCheckUnsafeGuidanceReferenceDiagnostic(reference))
			continue
		}
		if adoptCheckIgnoredDependencyOrCachePath(reference.Path) {
			if _, rejectedAlready := rejected[reference.Path]; rejectedAlready {
				continue
			}
			rejected[reference.Path] = struct{}{}
			diagnostics = append(diagnostics, adoptCheckIgnoredGuidanceReferenceDiagnostic(reference))
			continue
		}
		if _, ok := seen[reference.Path]; ok {
			continue
		}
		shape, ok := adoptCheckGuidanceCandidateShape(reference.Path, trackedFiles, tracked)
		if !ok {
			if _, rejectedAlready := rejected[reference.Path]; rejectedAlready {
				continue
			}
			rejected[reference.Path] = struct{}{}
			diagnostic, err := adoptCheckRejectedGuidanceReferenceDiagnostic(repoRoot, reference)
			if err != nil {
				return nil, nil, err
			}
			diagnostics = append(diagnostics, diagnostic)
			continue
		}
		seen[reference.Path] = struct{}{}
		candidates = append(candidates, adoptCheckCandidate{
			Path:                  reference.Path,
			Kind:                  "referenced_guidance_document",
			Shape:                 shape,
			RecommendedMemberRole: "rule",
			RoleConfirmation: adoptCheckRoleConfirmation{
				Required:               true,
				BatchAcceptRecommended: true,
				EditableRoles:          []string{"rule", "subject", "process", "ignore"},
			},
			Evidence: []adoptCheckEvidence{
				{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
			},
		})
	}

	return candidates, diagnostics, nil
}

var adoptCheckMarkdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

type adoptCheckGuidanceReference struct {
	Kind   string
	Path   string
	Unsafe bool
}

func adoptCheckGuidanceCandidateShape(repoPath string, trackedFiles []string, tracked map[string]struct{}) (string, bool) {
	if _, ok := tracked[repoPath]; ok {
		return "file", true
	}
	prefix := strings.TrimSuffix(repoPath, "/") + "/"
	for _, trackedFile := range trackedFiles {
		if strings.HasPrefix(trackedFile, prefix) {
			return "directory", true
		}
	}

	return "", false
}

func adoptCheckRejectedGuidanceReferenceDiagnostic(repoRoot string, reference adoptCheckGuidanceReference) (adoptCheckDiagnostic, error) {
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(reference.Path))
	if _, err := os.Lstat(absolutePath); err == nil {
		return adoptCheckDiagnostic{
			Code:     "referenced_guidance_untracked",
			Severity: "warning",
			Message:  "referenced guidance path is untracked and will not be adopted",
			Evidence: []adoptCheckEvidence{
				{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
			},
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return adoptCheckDiagnostic{}, fmt.Errorf("inspect referenced guidance path %s: %w", reference.Path, err)
	}

	return adoptCheckDiagnostic{
		Code:     "referenced_guidance_missing",
		Severity: "warning",
		Message:  "referenced guidance path is missing",
		Evidence: []adoptCheckEvidence{
			{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
		},
	}, nil
}

func adoptCheckUnsafeGuidanceReferenceDiagnostic(reference adoptCheckGuidanceReference) adoptCheckDiagnostic {
	return adoptCheckDiagnostic{
		Code:     "referenced_guidance_unsafe",
		Severity: "warning",
		Message:  "referenced guidance path is unsafe and will not be adopted",
		Evidence: []adoptCheckEvidence{
			{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
		},
	}
}

func adoptCheckIgnoredGuidanceReferenceDiagnostic(reference adoptCheckGuidanceReference) adoptCheckDiagnostic {
	return adoptCheckDiagnostic{
		Code:     "referenced_guidance_ignored",
		Severity: "warning",
		Message:  "referenced guidance path is ignored dependency or cache content and will not be adopted",
		Evidence: []adoptCheckEvidence{
			{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
		},
	}
}

func adoptCheckIgnoredDependencyOrCachePath(repoPath string) bool {
	ignoredRoots := []string{
		".cache",
		".next",
		".pnpm-store",
		".turbo",
		".yarn/cache",
		"build",
		"coverage",
		"dist",
		"node_modules",
	}
	for _, root := range ignoredRoots {
		if repoPath == root || strings.HasPrefix(repoPath, root+"/") {
			return true
		}
	}

	return false
}

func parseAdoptCheckMarkdownLinks(content string) []adoptCheckGuidanceReference {
	matches := adoptCheckMarkdownLinkPattern.FindAllStringSubmatch(content, -1)
	references := make([]adoptCheckGuidanceReference, 0, len(matches))

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if reference, ok := normalizeAdoptCheckGuidanceReference("markdown_link", match[1]); ok {
			references = append(references, reference)
		}
	}

	return references
}

var adoptCheckPathMentionPattern = regexp.MustCompile(`(?:^|[\s("'` + "`" + `])([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+/?|[A-Za-z0-9._-]+\.(?:md|markdown|txt|toml|ya?ml|json))(?:$|[\s).,;:"'` + "`" + `])`)

func parseAdoptCheckPathMentions(content string) []adoptCheckGuidanceReference {
	matches := adoptCheckPathMentionPattern.FindAllStringSubmatch(content, -1)
	references := make([]adoptCheckGuidanceReference, 0, len(matches))

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if reference, ok := normalizeAdoptCheckGuidanceReference("path_mention", match[1]); ok {
			references = append(references, reference)
		}
	}

	return references
}

func normalizeAdoptCheckGuidanceReference(kind string, value string) (adoptCheckGuidanceReference, bool) {
	target := adoptCheckGuidanceReferenceTarget(value)
	target = strings.TrimRight(target, ".,;:")
	if target == "" || strings.HasPrefix(target, "#") || strings.Contains(target, "://") {
		return adoptCheckGuidanceReference{}, false
	}
	target = stripAdoptCheckGuidanceReferenceFragment(target)
	if target == "" {
		return adoptCheckGuidanceReference{}, false
	}
	normalized, err := ids.NormalizeRepoRelativePath(target)
	if err != nil {
		return adoptCheckGuidanceReference{
			Kind:   kind,
			Path:   target,
			Unsafe: true,
		}, true
	}

	return adoptCheckGuidanceReference{
		Kind: kind,
		Path: normalized,
	}, true
}

func adoptCheckGuidanceReferenceTarget(value string) string {
	target := strings.TrimSpace(value)
	if strings.HasPrefix(target, "<") {
		if end := strings.Index(target, ">"); end >= 0 {
			target = target[1:end]
		}
	} else if end := strings.IndexAny(target, " \t\r\n"); end >= 0 {
		target = target[:end]
	}

	return strings.Trim(strings.TrimSpace(target), `"'`)
}

func stripAdoptCheckGuidanceReferenceFragment(target string) string {
	if end := strings.IndexAny(target, "#?"); end >= 0 {
		return target[:end]
	}

	return target
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
