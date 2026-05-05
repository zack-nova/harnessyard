package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
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

type adoptWriteOutput struct {
	SchemaVersion string                              `json:"schema_version"`
	RepoRoot      string                              `json:"repo_root"`
	Mode          string                              `json:"mode"`
	AdoptedOrbit  adoptCheckAdoptedOrbit              `json:"adopted_orbit"`
	WrittenPaths  []string                            `json:"written_paths"`
	AgentConfig   *harnesspkg.AgentConfigImportResult `json:"agent_config_import,omitempty"`
	Validations   []adoptWriteValidation              `json:"validations"`
	Check         harnesspkg.CheckResult              `json:"check"`
	Readiness     harnesspkg.ReadinessReport          `json:"readiness"`
	NextActions   []adoptCheckNextAction              `json:"next_actions"`
}

type adoptWriteValidation struct {
	Target string `json:"target"`
	OK     bool   `json:"ok"`
}

func newAdoptCommand() *cobra.Command {
	var check bool
	var orbitID string

	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Inspect or convert an Ordinary Repository into a Harness Runtime",
		Long: "Inspect or convert an Ordinary Repository into a Harness Runtime.\n" +
			"The first Adoption write slice converts root AGENTS.md into hosted Adopted Orbit truth\n" +
			"and rewrites root guidance as an orbit-owned marker block.",
		Example: "" +
			"  hyard adopt --check --json\n" +
			"  hyard adopt --json\n" +
			"  hyard adopt --check --json --orbit workspace\n",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOutput, err := wantHyardJSON(cmd)
			if err != nil {
				return err
			}

			if !check {
				output, err := buildAdoptWriteOutput(cmd, orbitID)
				if err != nil {
					return err
				}
				if jsonOutput {
					return emitHyardJSON(cmd, output)
				}

				return printAdoptWriteText(cmd, output)
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

func buildAdoptWriteOutput(cmd *cobra.Command, explicitOrbitID string) (adoptWriteOutput, error) {
	preflight, err := buildAdoptCheckOutput(cmd, explicitOrbitID)
	if err != nil {
		return adoptWriteOutput{}, err
	}
	if preflight.ExistingHarnessRuntime {
		return adoptWriteOutput{}, fmt.Errorf("existing Harness Runtime cannot be adopted again; use `hyard layout optimize`")
	}
	if preflight.DirtyWorktree.Dirty {
		return adoptWriteOutput{}, fmt.Errorf(
			"adoption write mode requires a clean worktree; dirty paths: %s",
			strings.Join(preflight.DirtyWorktree.Paths, ", "),
		)
	}
	if blockingMessages := adoptCheckBlockingDiagnosticMessages(preflight.Diagnostics); len(blockingMessages) > 0 {
		return adoptWriteOutput{}, fmt.Errorf(
			"adoption has blocking diagnostics: %s",
			strings.Join(blockingMessages, "; "),
		)
	}

	agentsPath := filepath.Join(preflight.RepoRoot, "AGENTS.md")
	//nolint:gosec // The root guidance path is fixed under the discovered repository root.
	originalAgents, err := os.ReadFile(agentsPath)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("read root agent guidance: %w", err)
	}
	if err := orbittemplate.ValidateRuntimeAgentsFile(preflight.RepoRoot); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("root AGENTS.md has malformed orbit markers: %w", err)
	}

	now := time.Now().UTC()
	manifestFile, err := harnesspkg.DefaultRuntimeManifestFile(preflight.RepoRoot, now)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("build adoption runtime manifest: %w", err)
	}
	manifestFile.Members = append(manifestFile.Members, harnesspkg.ManifestMember{
		Package: ids.PackageIdentity{
			Type: ids.PackageTypeOrbit,
			Name: preflight.AdoptedOrbit.ID,
		},
		OrbitID: preflight.AdoptedOrbit.ID,
		Source:  harnesspkg.ManifestMemberSourceManual,
		AddedAt: now,
	})
	if _, err := harnesspkg.WriteManifestFile(preflight.RepoRoot, manifestFile); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("write adoption runtime manifest: %w", err)
	}

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(preflight.AdoptedOrbit.ID)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("build adopted orbit spec: %w", err)
	}
	spec.Meta.AgentsTemplate = string(originalAgents)
	spec = applyAdoptedCodexLocalSkillCapabilityTruth(spec, preflight.Candidates)
	if _, err := orbitpkg.WriteHostedOrbitSpec(preflight.RepoRoot, spec); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("write adopted orbit spec: %w", err)
	}

	wrappedAgents, err := orbittemplate.WrapRuntimeAgentsBlock(preflight.AdoptedOrbit.ID, originalAgents)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("wrap adopted root guidance: %w", err)
	}
	if err := os.WriteFile(agentsPath, wrappedAgents, 0o644); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("write root agent guidance marker block: %w", err)
	}

	var agentConfigImport *harnesspkg.AgentConfigImportResult
	if preflight.Frameworks.Recommended == "codex" {
		importResult, err := harnesspkg.ImportAgentConfig(cmd.Context(), harnesspkg.AgentConfigImportInput{
			RepoRoot:     preflight.RepoRoot,
			Framework:    "codex",
			SourceScopes: []string{"project"},
			Write:        true,
		})
		if err != nil {
			return adoptWriteOutput{}, fmt.Errorf("import Codex project config during Adoption: %w", err)
		}
		agentConfigImport = &importResult
	}

	if _, err := harnesspkg.LoadManifestFile(preflight.RepoRoot); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("validate generated runtime manifest: %w", err)
	}
	validatedSpec, err := orbitpkg.LoadHostedOrbitSpec(cmd.Context(), preflight.RepoRoot, preflight.AdoptedOrbit.ID)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("validate generated adopted orbit spec: %w", err)
	}
	repoConfig, err := orbitpkg.LoadHostedRepositoryConfig(cmd.Context(), preflight.RepoRoot)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("load generated hosted repository config: %w", err)
	}
	trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), preflight.RepoRoot)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("load tracked files for generated projection plan: %w", err)
	}
	if _, err := orbitpkg.ResolveProjectionPlan(repoConfig, validatedSpec, trackedFiles); err != nil {
		return adoptWriteOutput{}, fmt.Errorf("validate generated projection plan: %w", err)
	}
	checkResult, err := harnesspkg.CheckRuntime(cmd.Context(), preflight.RepoRoot)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("validate generated runtime check summary: %w", err)
	}
	readiness, err := harnesspkg.EvaluateRuntimeReadiness(cmd.Context(), preflight.RepoRoot)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("validate generated runtime readiness: %w", err)
	}

	orbitSpecPath, err := harnesspkg.OrbitSpecRepoPath(preflight.AdoptedOrbit.ID)
	if err != nil {
		return adoptWriteOutput{}, fmt.Errorf("build adopted orbit spec path: %w", err)
	}
	writtenPaths := []string{
		harnesspkg.ManifestRepoPath(),
		orbitSpecPath,
		"AGENTS.md",
	}
	if agentConfigImport != nil {
		writtenPaths = append(writtenPaths, agentConfigImport.WrittenPaths...)
	}

	return adoptWriteOutput{
		SchemaVersion: adoptionCheckSchemaVersion,
		RepoRoot:      preflight.RepoRoot,
		Mode:          "write",
		AdoptedOrbit:  preflight.AdoptedOrbit,
		WrittenPaths:  writtenPaths,
		AgentConfig:   agentConfigImport,
		Validations:   adoptWriteValidations(checkResult, readiness),
		Check:         checkResult,
		Readiness:     readiness,
		NextActions:   adoptWriteNextActions(writtenPaths),
	}, nil
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
	localSkillCandidates, localSkillDiagnostics, err := inspectAdoptCheckCodexLocalSkillCandidates(cmd, repo.Root, orbitID)
	if err != nil {
		return adoptCheckOutput{}, err
	}
	candidates = append(candidates, localSkillCandidates...)
	candidateDiagnostics = append(candidateDiagnostics, localSkillDiagnostics...)

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
	if adoptCheckHasErrorDiagnostic(output.Diagnostics) {
		output.Adoptable = false
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
		if skillRoots := adoptCheckCodexLocalSkillReferenceRoots(reference.Path, tracked); len(skillRoots) > 0 {
			rejectedKey := "codex_local_skill:" + strings.Join(skillRoots, ",")
			if _, rejectedAlready := rejected[rejectedKey]; rejectedAlready {
				continue
			}
			rejected[rejectedKey] = struct{}{}
			diagnostics = append(diagnostics, adoptCheckCodexLocalSkillMemberOverlapAvoidedDiagnostic(reference, skillRoots))
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

func inspectAdoptCheckCodexLocalSkillCandidates(
	cmd *cobra.Command,
	repoRoot string,
	orbitID string,
) ([]adoptCheckCandidate, []adoptCheckDiagnostic, error) {
	trackedFiles, err := gitpkg.TrackedFiles(cmd.Context(), repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("load tracked files for Codex local skill discovery: %w", err)
	}

	skillRoots := adoptCheckCodexLocalSkillRoots(trackedFiles)
	candidates := make([]adoptCheckCandidate, 0, len(skillRoots))
	diagnostics := []adoptCheckDiagnostic{}
	validFrontmatter := map[string]struct{}{}
	rootsByName := map[string][]string{}
	for _, rootPath := range skillRoots {
		skillMDPath := rootPath + "/SKILL.md"
		name, err := orbitpkg.LoadSkillFrontmatterName(repoRoot, skillMDPath)
		if err != nil {
			diagnostics = append(diagnostics, adoptCheckCodexLocalSkillInvalidDiagnostic(rootPath, err))
			continue
		}
		validFrontmatter[rootPath] = struct{}{}
		rootsByName[name] = append(rootsByName[name], rootPath)
	}

	duplicateRoots := map[string]struct{}{}
	for name, roots := range rootsByName {
		if len(roots) < 2 {
			continue
		}
		sort.Strings(roots)
		for _, rootPath := range roots {
			duplicateRoots[rootPath] = struct{}{}
		}
		diagnostics = append(diagnostics, adoptCheckCodexLocalSkillDuplicateNameDiagnostic(name, roots))
	}

	for _, rootPath := range skillRoots {
		if _, ok := validFrontmatter[rootPath]; !ok {
			continue
		}
		if _, duplicate := duplicateRoots[rootPath]; duplicate {
			continue
		}
		skill, err := resolveAdoptCheckCodexLocalSkillRoot(repoRoot, rootPath, trackedFiles)
		if err != nil {
			diagnostics = append(diagnostics, adoptCheckCodexLocalSkillInvalidDiagnostic(rootPath, err))
			continue
		}
		diagnostics = append(diagnostics, adoptCheckCodexLocalSkillNonRecommendedPathDiagnostic(skill, orbitID))
		candidates = append(candidates, adoptCheckCandidate{
			Path:  skill.RootPath,
			Kind:  "local_skill_capability",
			Shape: "directory",
			Evidence: []adoptCheckEvidence{
				{Kind: "codex_skill_root", Path: skill.SkillMDPath, Detail: skill.Name},
			},
		})
	}

	return candidates, diagnostics, nil
}

func adoptCheckHasErrorDiagnostic(diagnostics []adoptCheckDiagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			return true
		}
	}

	return false
}

func adoptCheckBlockingDiagnosticMessages(diagnostics []adoptCheckDiagnostic) []string {
	messages := []string{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != "error" {
			continue
		}
		messages = append(messages, diagnostic.Message)
	}

	return messages
}

func adoptCheckCodexLocalSkillRoots(trackedFiles []string) []string {
	rootSet := map[string]struct{}{}
	for _, trackedFile := range trackedFiles {
		if !strings.HasPrefix(trackedFile, ".codex/skills/") || !strings.HasSuffix(trackedFile, "/SKILL.md") {
			continue
		}
		remainder := strings.TrimPrefix(trackedFile, ".codex/skills/")
		if strings.Count(remainder, "/") != 1 {
			continue
		}
		rootSet[strings.TrimSuffix(trackedFile, "/SKILL.md")] = struct{}{}
	}

	roots := make([]string, 0, len(rootSet))
	for rootPath := range rootSet {
		roots = append(roots, rootPath)
	}
	sort.Strings(roots)

	return roots
}

func resolveAdoptCheckCodexLocalSkillRoot(
	repoRoot string,
	rootPath string,
	trackedFiles []string,
) (orbitpkg.ResolvedLocalSkillCapability, error) {
	spec := orbitpkg.OrbitSpec{
		Capabilities: &orbitpkg.OrbitCapabilities{
			Skills: &orbitpkg.OrbitSkillCapabilities{
				Local: &orbitpkg.OrbitLocalSkillCapabilityPaths{
					Paths: orbitpkg.OrbitMemberPaths{
						Include: []string{rootPath},
					},
				},
			},
		},
	}
	resolved, err := orbitpkg.ResolveLocalSkillCapabilities(repoRoot, spec, trackedFiles, trackedFiles)
	if err != nil {
		return orbitpkg.ResolvedLocalSkillCapability{}, err
	}
	if len(resolved) == 0 {
		return orbitpkg.ResolvedLocalSkillCapability{}, fmt.Errorf("local skill root %q: SKILL.md must exist and be tracked", rootPath)
	}

	return resolved[0], nil
}

func adoptCheckCodexLocalSkillInvalidDiagnostic(rootPath string, err error) adoptCheckDiagnostic {
	detail := err.Error()
	prefix := fmt.Sprintf("local skill root %q: ", rootPath)
	detail = strings.TrimPrefix(detail, prefix)
	code := "codex_local_skill_invalid_frontmatter"
	message := "Codex local skill frontmatter is invalid: " + detail
	if strings.Contains(detail, "invalid skill basename") ||
		strings.Contains(detail, "frontmatter name") ||
		strings.Contains(detail, "duplicate") {
		code = "codex_local_skill_invalid_identity"
		message = "Codex local skill identity is invalid: " + detail
	}

	return adoptCheckDiagnostic{
		Code:     code,
		Severity: "error",
		Message:  message,
		Evidence: []adoptCheckEvidence{
			{Kind: "codex_skill_root", Path: rootPath + "/SKILL.md"},
		},
	}
}

func adoptCheckCodexLocalSkillDuplicateNameDiagnostic(name string, rootPaths []string) adoptCheckDiagnostic {
	evidence := make([]adoptCheckEvidence, 0, len(rootPaths))
	for _, rootPath := range rootPaths {
		evidence = append(evidence, adoptCheckEvidence{
			Kind:   "codex_skill_root",
			Path:   rootPath + "/SKILL.md",
			Detail: name,
		})
	}

	return adoptCheckDiagnostic{
		Code:     "codex_local_skill_duplicate_name",
		Severity: "error",
		Message:  fmt.Sprintf("Codex local skill name %q is declared by multiple roots", name),
		Evidence: evidence,
	}
}

func adoptCheckCodexLocalSkillNonRecommendedPathDiagnostic(
	skill orbitpkg.ResolvedLocalSkillCapability,
	orbitID string,
) adoptCheckDiagnostic {
	recommendedPath := "skills/" + orbitID + "/" + skill.Name

	return adoptCheckDiagnostic{
		Code:     "codex_local_skill_non_recommended_path",
		Severity: "warning",
		Message:  "Codex local skill root is outside the recommended position; if recommended moves are declined, Adoption will keep it as a capability path",
		Evidence: []adoptCheckEvidence{
			{Kind: "codex_skill_root", Path: skill.SkillMDPath, Detail: "recommended: " + recommendedPath},
		},
	}
}

func applyAdoptedCodexLocalSkillCapabilityTruth(spec orbitpkg.OrbitSpec, candidates []adoptCheckCandidate) orbitpkg.OrbitSpec {
	skillRoots := []string{}
	for _, candidate := range candidates {
		if candidate.Kind != "local_skill_capability" || !strings.HasPrefix(candidate.Path, ".codex/skills/") {
			continue
		}
		skillRoots = append(skillRoots, candidate.Path)
	}
	if len(skillRoots) == 0 {
		return spec
	}
	sort.Strings(skillRoots)

	if spec.Capabilities == nil {
		spec.Capabilities = &orbitpkg.OrbitCapabilities{}
	}
	if spec.Capabilities.Skills == nil {
		spec.Capabilities.Skills = &orbitpkg.OrbitSkillCapabilities{}
	}
	spec.Capabilities.Skills.Local = &orbitpkg.OrbitLocalSkillCapabilityPaths{
		Paths: orbitpkg.OrbitMemberPaths{
			Include: skillRoots,
		},
	}

	return spec
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

func adoptCheckCodexLocalSkillReferenceRoots(repoPath string, tracked map[string]struct{}) []string {
	if !strings.HasPrefix(repoPath, ".codex/skills/") {
		if repoPath != ".codex/skills" {
			return nil
		}
	}

	if repoPath == ".codex/skills" {
		roots := []string{}
		for trackedPath := range tracked {
			if !strings.HasPrefix(trackedPath, ".codex/skills/") || !strings.HasSuffix(trackedPath, "/SKILL.md") {
				continue
			}
			remainder := strings.TrimPrefix(trackedPath, ".codex/skills/")
			if strings.Count(remainder, "/") != 1 {
				continue
			}
			roots = append(roots, strings.TrimSuffix(trackedPath, "/SKILL.md"))
		}
		sort.Strings(roots)
		return roots
	}

	remainder := strings.TrimPrefix(repoPath, ".codex/skills/")
	if remainder == "" {
		return nil
	}
	skillName := remainder
	if slashIndex := strings.Index(skillName, "/"); slashIndex >= 0 {
		skillName = skillName[:slashIndex]
	}
	if skillName == "" {
		return nil
	}
	rootPath := ".codex/skills/" + skillName
	if _, ok := tracked[rootPath+"/SKILL.md"]; !ok {
		return nil
	}

	return []string{rootPath}
}

func adoptCheckCodexLocalSkillMemberOverlapAvoidedDiagnostic(
	reference adoptCheckGuidanceReference,
	skillRoots []string,
) adoptCheckDiagnostic {
	evidence := []adoptCheckEvidence{
		{Kind: reference.Kind, Path: "AGENTS.md", Detail: reference.Path},
	}
	for _, skillRoot := range skillRoots {
		evidence = append(evidence, adoptCheckEvidence{
			Kind: "codex_skill_root",
			Path: skillRoot + "/SKILL.md",
		})
	}

	return adoptCheckDiagnostic{
		Code:     "codex_local_skill_member_overlap_avoided",
		Severity: "warning",
		Message:  "referenced Codex local skill root is capability-owned and will not be adopted as ordinary member content",
		Evidence: evidence,
	}
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

func adoptWriteValidations(checkResult harnesspkg.CheckResult, readiness harnesspkg.ReadinessReport) []adoptWriteValidation {
	return []adoptWriteValidation{
		{Target: "runtime_manifest", OK: true},
		{Target: "adopted_orbit_spec", OK: true},
		{Target: "projection_plan", OK: true},
		{Target: "runtime_check", OK: checkResult.OK},
		{Target: "runtime_readiness", OK: readiness.Runtime.Status != harnesspkg.ReadinessStatusBroken},
	}
}

func adoptWriteNextActions(writtenPaths []string) []adoptCheckNextAction {
	return []adoptCheckNextAction{
		{
			Command: "hyard check",
			Reason:  "validate the generated Harness Runtime",
		},
		{
			Command: "hyard agent apply --yes",
			Reason:  "optionally activate agent-facing runtime guidance",
		},
		{
			Command: "hyard publish harness",
			Reason:  "optionally publish a Harness Template after review",
		},
		{
			Command: "git status && git add " + strings.Join(adoptWriteReviewPaths(writtenPaths), " ") + " && git commit",
			Reason:  "review and commit Adoption changes when ready",
		},
	}
}

func adoptWriteReviewPaths(writtenPaths []string) []string {
	reviewPaths := make([]string, 0, len(writtenPaths))
	for _, path := range writtenPaths {
		if path == "AGENTS.md" {
			reviewPaths = append(reviewPaths, path)
		}
	}
	for _, path := range writtenPaths {
		if path != "AGENTS.md" {
			reviewPaths = append(reviewPaths, path)
		}
	}

	return reviewPaths
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

func printAdoptWriteText(cmd *cobra.Command, output adoptWriteOutput) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "adopted: true\nadopted_orbit: %s\n", output.AdoptedOrbit.ID); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, path := range output.WrittenPaths {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "written: %s\n", path); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	for _, validation := range output.Validations {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "validation: %s ok=%t\n", validation.Target, validation.OK); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "check_ok: %t\n", output.Check.OK); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "readiness_status: %s\n", output.Readiness.Status); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, action := range output.NextActions {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "next_action: %s (%s)\n", action.Command, action.Reason); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}

	return nil
}
