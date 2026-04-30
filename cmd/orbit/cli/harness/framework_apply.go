package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FrameworkApplyInput captures one framework activation request for the current runtime.
type FrameworkApplyInput struct {
	RepoRoot            string
	GitDir              string
	HarnessID           string
	RouteChoice         FrameworkApplyRouteChoice
	AllowGlobalFallback bool
	EnableHooks         bool
}

// FrameworkApplyRouteChoice selects the command/skill activation route.
type FrameworkApplyRouteChoice string

const (
	FrameworkApplyRouteAuto    FrameworkApplyRouteChoice = ""
	FrameworkApplyRouteProject FrameworkApplyRouteChoice = "project"
	FrameworkApplyRouteGlobal  FrameworkApplyRouteChoice = "global"
)

// FrameworkApplyArtifactResult reports one command/skill activation outcome.
type FrameworkApplyArtifactResult struct {
	Framework      string   `json:"framework,omitempty"`
	OrbitID        string   `json:"orbit_id,omitempty"`
	Package        string   `json:"package,omitempty"`
	AddonID        string   `json:"addon_id,omitempty"`
	Artifact       string   `json:"artifact"`
	ArtifactType   string   `json:"artifact_type"`
	Route          string   `json:"route"`
	Mode           string   `json:"mode,omitempty"`
	Required       bool     `json:"required,omitempty"`
	Source         string   `json:"source,omitempty"`
	Sidecar        string   `json:"sidecar,omitempty"`
	SourceFiles    []string `json:"source_files,omitempty"`
	Path           string   `json:"path,omitempty"`
	Target         string   `json:"target,omitempty"`
	EffectiveScope string   `json:"effective_scope"`
	Status         string   `json:"status"`
	Message        string   `json:"message,omitempty"`
	Invocation     []string `json:"invocation,omitempty"`
	GeneratedKeys  []string `json:"generated_keys,omitempty"`
	PatchOwnedKeys []string `json:"patch_owned_keys,omitempty"`
	HandlerDigest  string   `json:"handler_digest,omitempty"`
}

// FrameworkApplyResult reports one completed framework activation.
type FrameworkApplyResult struct {
	Framework          string                         `json:"framework,omitempty"`
	ResolutionSource   FrameworkSelectionSource       `json:"resolution_source"`
	Status             string                         `json:"status"`
	ActivationPath     string                         `json:"activation_path,omitempty"`
	ProjectOutputCount int                            `json:"project_output_count"`
	GlobalOutputCount  int                            `json:"global_output_count"`
	ArtifactResults    []FrameworkApplyArtifactResult `json:"artifact_results,omitempty"`
	Warnings           []string                       `json:"warnings,omitempty"`
}

// FrameworkCheckFinding captures one observable framework activation problem.
type FrameworkCheckFinding struct {
	OrbitID  string `json:"orbit_id,omitempty"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking,omitempty"`
}

// FrameworkCheckResult reports current framework activation health for the runtime.
type FrameworkCheckResult struct {
	Framework        string                   `json:"framework,omitempty"`
	ResolutionSource FrameworkSelectionSource `json:"resolution_source"`
	Configured       bool                     `json:"configured"`
	Stale            bool                     `json:"stale"`
	OK               bool                     `json:"ok"`
	FindingCount     int                      `json:"finding_count"`
	Findings         []FrameworkCheckFinding  `json:"findings,omitempty"`
	ActivationIDs    []string                 `json:"activation_ids,omitempty"`
	Warnings         []string                 `json:"warnings,omitempty"`
}

// FrameworkRemoveResult reports one completed framework activation cleanup.
type FrameworkRemoveResult struct {
	RemovedActivationCount int      `json:"removed_activation_count"`
	RemovedOutputCount     int      `json:"removed_output_count"`
	SkippedOutputCount     int      `json:"skipped_output_count"`
	Warnings               []string `json:"warnings,omitempty"`
}

// ApplyFramework materializes framework-managed side effects and records repo-local ownership.
func ApplyFramework(ctx context.Context, input FrameworkApplyInput) (FrameworkApplyResult, error) {
	state, err := loadFrameworkDesiredState(ctx, input.RepoRoot, input.GitDir)
	if err != nil {
		return FrameworkApplyResult{}, err
	}
	if state.Summary.ResolutionSource == FrameworkSelectionSourceUnresolvedConflict {
		return FrameworkApplyResult{}, fmt.Errorf("framework apply is blocked: framework resolution is unresolved_conflict")
	}
	if state.Summary.ResolvedFramework == "" {
		return FrameworkApplyResult{}, fmt.Errorf("framework resolution is unresolved")
	}
	if err := blockingFrameworkCapabilityError("framework apply", state.CapabilityFindings); err != nil {
		return FrameworkApplyResult{}, err
	}

	managedOutputs, artifactResults, err := buildFrameworkManagedOutputs(input.RepoRoot, input.GitDir, input.HarnessID, state.Summary, input.RouteChoice, input.EnableHooks)
	if err != nil {
		return FrameworkApplyResult{}, err
	}

	if state.Summary.HasAgentGuidance || state.Summary.HasHumanGuidance || state.Summary.HasPendingBootstrapGuidance {
		if _, err := ComposeRuntimeGuidance(ctx, ComposeRuntimeGuidanceInput{
			RepoRoot: input.RepoRoot,
			Target:   GuidanceTargetAll,
		}); err != nil {
			return FrameworkApplyResult{}, fmt.Errorf("compose runtime guidance: %w", err)
		}
	}

	createdOutputs := make([]FrameworkActivationOutput, 0, len(managedOutputs.ProjectOutputs)+len(managedOutputs.GlobalOutputs))
	rollbackOutputs := func() {
		for index := len(createdOutputs) - 1; index >= 0; index-- {
			_ = rollbackFrameworkOutput(createdOutputs[index]) //nolint:errcheck // Rollback is best-effort after apply failure.
		}
	}
	appliedProjectOutputs := []FrameworkActivationOutput{}
	appliedGlobalOutputs := []FrameworkActivationOutput{}
	fallbackOutputs := frameworkFallbackOutputsByArtifactKey(managedOutputs.FallbackGlobalOutputs)
	for _, output := range managedOutputs.ProjectOutputs {
		changed, err := ensureFrameworkOutput(output)
		if err != nil {
			if fallback, ok := fallbackOutputs[frameworkArtifactKey(output.OrbitID, output.ArtifactType, output.Artifact)]; input.AllowGlobalFallback && output.Artifact != "" && ok {
				markFrameworkApplyArtifactResult(artifactResults, output, "project_failed", err.Error())
				changed, fallbackErr := ensureFrameworkOutput(fallback)
				if fallbackErr != nil {
					rollbackOutputs()
					return FrameworkApplyResult{}, fallbackErr
				}
				if changed {
					createdOutputs = append(createdOutputs, fallback)
				}
				appliedGlobalOutputs = append(appliedGlobalOutputs, fallback)
				artifactResults = append(artifactResults, artifactResultFromActivationOutput(state.Summary.ResolvedFramework, fallback, "global_applied"))
				continue
			}
			if output.Artifact != "" && frameworkOutputConflictCanDefer(err) {
				markFrameworkApplyArtifactResult(artifactResults, output, "project_failed", err.Error())
				artifactResults = append(artifactResults, FrameworkApplyArtifactResult{
					Framework:      state.Summary.ResolvedFramework,
					OrbitID:        output.OrbitID,
					Artifact:       output.Artifact,
					ArtifactType:   output.ArtifactType,
					Route:          "project_compatibility",
					Mode:           "not_implemented",
					Source:         output.Source,
					Sidecar:        output.Sidecar,
					SourceFiles:    append([]string(nil), output.SourceFiles...),
					Path:           output.Path,
					EffectiveScope: output.EffectiveScope,
					Status:         "compatibility_pending",
					Message:        "project compatibility for this artifact is not implemented yet",
					Invocation:     append([]string(nil), output.Invocation...),
					GeneratedKeys:  append([]string(nil), output.GeneratedKeys...),
					PatchOwnedKeys: append([]string(nil), output.PatchOwnedKeys...),
				})
				continue
			}
			rollbackOutputs()
			return FrameworkApplyResult{}, err
		}
		if changed {
			createdOutputs = append(createdOutputs, output)
		}
		appliedProjectOutputs = append(appliedProjectOutputs, output)
	}
	for _, output := range managedOutputs.GlobalOutputs {
		changed, err := ensureFrameworkOutput(output)
		if err != nil {
			rollbackOutputs()
			return FrameworkApplyResult{}, err
		}
		if changed {
			createdOutputs = append(createdOutputs, output)
		}
		appliedGlobalOutputs = append(appliedGlobalOutputs, output)
	}

	guidanceHash, capabilitiesHash, selectionHash, runtimeAgentTruthHash, err := computeFrameworkDesiredHashes(input.RepoRoot, state)
	if err != nil {
		rollbackOutputs()
		return FrameworkApplyResult{}, err
	}
	packageHooks := []FrameworkActivationPackageHook{}
	if input.EnableHooks {
		packageHooks = frameworkActivationPackageHooks(state.Summary, input.RouteChoice)
	}
	activation := FrameworkActivation{
		Framework:             state.Summary.ResolvedFramework,
		ResolutionSource:      state.Summary.ResolutionSource,
		RepoRoot:              input.RepoRoot,
		AppliedAt:             time.Now().UTC(),
		GuidanceHash:          guidanceHash,
		CapabilitiesHash:      capabilitiesHash,
		SelectionHash:         selectionHash,
		RuntimeAgentTruthHash: runtimeAgentTruthHash,
		ProjectOutputs:        appliedProjectOutputs,
		GlobalOutputs:         appliedGlobalOutputs,
		PackageHooks:          packageHooks,
	}
	filename, err := WriteFrameworkActivation(input.GitDir, activation)
	if err != nil {
		rollbackOutputs()
		return FrameworkApplyResult{}, fmt.Errorf("write framework activation ledger: %w", err)
	}

	return FrameworkApplyResult{
		Framework:          activation.Framework,
		ResolutionSource:   activation.ResolutionSource,
		Status:             frameworkApplyOverallStatus(artifactResults),
		ActivationPath:     filename,
		ProjectOutputCount: len(activation.ProjectOutputs),
		GlobalOutputCount:  len(activation.GlobalOutputs),
		ArtifactResults:    artifactResults,
		Warnings:           append([]string(nil), state.Summary.Warnings...),
	}, nil
}

// CheckFramework inspects current framework activation health for the runtime.
func CheckFramework(ctx context.Context, repoRoot string, gitDir string) (FrameworkCheckResult, error) {
	state, err := loadFrameworkDesiredState(ctx, repoRoot, gitDir)
	if err != nil {
		return FrameworkCheckResult{}, err
	}
	if state.Summary.ResolutionSource == FrameworkSelectionSourceUnresolvedConflict {
		return FrameworkCheckResult{}, fmt.Errorf("framework check is blocked: framework resolution is unresolved_conflict")
	}

	activationIDs, err := ListFrameworkActivationIDs(gitDir)
	if err != nil {
		return FrameworkCheckResult{}, fmt.Errorf("list framework activations: %w", err)
	}

	result := FrameworkCheckResult{
		Framework:        state.Summary.ResolvedFramework,
		ResolutionSource: state.Summary.ResolutionSource,
		ActivationIDs:    activationIDs,
		Findings:         []FrameworkCheckFinding{},
		Warnings:         append([]string(nil), state.Summary.Warnings...),
	}

	if state.Summary.ResolvedFramework == "" {
		result.OK = len(activationIDs) == 0
		result.FindingCount = len(result.Findings)
		return result, nil
	}
	result.Findings = append(result.Findings, state.CapabilityFindings...)
	if err := appendFrameworkHookFindings(repoRoot, state.Summary, &result.Findings); err != nil {
		return FrameworkCheckResult{}, err
	}

	activation, err := LoadFrameworkActivation(gitDir, state.Summary.ResolvedFramework)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Findings = append(result.Findings, FrameworkCheckFinding{
				Kind:    "activation_missing",
				Message: "framework activation ledger is missing for the resolved framework",
			})
			result.Findings = sortFrameworkCheckFindings(result.Findings)
			result.FindingCount = len(result.Findings)
			return result, nil
		}
		return FrameworkCheckResult{}, fmt.Errorf("load framework activation ledger: %w", err)
	}

	result.Configured = true
	guidanceHash, capabilitiesHash, selectionHash, runtimeAgentTruthHash, err := computeFrameworkDesiredHashes(repoRoot, state)
	if err != nil {
		return FrameworkCheckResult{}, err
	}
	if activation.RepoRoot != repoRoot ||
		activation.GuidanceHash != guidanceHash ||
		activation.CapabilitiesHash != capabilitiesHash ||
		activation.SelectionHash != selectionHash ||
		activation.RuntimeAgentTruthHash != runtimeAgentTruthHash {
		result.Stale = true
		result.Findings = append(result.Findings, FrameworkCheckFinding{
			Kind:    "activation_stale",
			Message: "framework activation is stale relative to current runtime truth",
		})
	}

	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		findings, err := frameworkOutputCheckFindings(repoRoot, output)
		if err != nil {
			return FrameworkCheckResult{}, err
		}
		result.Findings = append(result.Findings, findings...)
	}
	appendPackageHookActivationFindings(state.Summary, activation, &result.Findings)

	adapter, ok := LookupFrameworkAdapter(state.Summary.ResolvedFramework)
	if ok {
		for _, executableName := range adapter.ExecutableNames {
			if executableName == "" {
				continue
			}
			if _, err := exec.LookPath(executableName); err != nil {
				result.Findings = append(result.Findings, FrameworkCheckFinding{
					Kind:    "executable_missing",
					Path:    executableName,
					Message: "required framework executable is not available on PATH",
				})
			}
		}
		for _, envVar := range adapter.RequiredEnvVars {
			if strings.TrimSpace(os.Getenv(envVar)) == "" {
				result.Findings = append(result.Findings, FrameworkCheckFinding{
					Kind:    "env_missing",
					Path:    envVar,
					Message: "required framework environment variable is not set",
				})
			}
		}
	}

	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if writable, err := frameworkOutputParentWritable(output.AbsolutePath); err != nil {
			return FrameworkCheckResult{}, err
		} else if !writable {
			result.Findings = append(result.Findings, FrameworkCheckFinding{
				Kind:    "write_permission_missing",
				Path:    output.Path,
				Message: "parent directory for framework-managed output is not writable",
			})
		}
	}

	result.Findings = sortFrameworkCheckFindings(result.Findings)
	result.FindingCount = len(result.Findings)
	result.OK = result.Configured && !result.Stale && result.FindingCount == 0

	return result, nil
}

// RemoveFrameworkActivations removes framework-managed side effects still owned by this runtime.
func RemoveFrameworkActivations(ctx context.Context, repoRoot string, gitDir string) (FrameworkRemoveResult, error) {
	resolution, err := ResolveFramework(ctx, FrameworkResolutionInput{
		RepoRoot: repoRoot,
		GitDir:   gitDir,
	})
	if err != nil {
		return FrameworkRemoveResult{}, err
	}
	if resolution.Source == FrameworkSelectionSourceUnresolvedConflict {
		return FrameworkRemoveResult{}, fmt.Errorf("framework remove is blocked: framework resolution is unresolved_conflict")
	}

	activationIDs, err := ListFrameworkActivationIDs(gitDir)
	if err != nil {
		return FrameworkRemoveResult{}, fmt.Errorf("list framework activations: %w", err)
	}

	result := FrameworkRemoveResult{Warnings: []string{}}
	for _, frameworkID := range activationIDs {
		activation, err := LoadFrameworkActivation(gitDir, frameworkID)
		if err != nil {
			return FrameworkRemoveResult{}, fmt.Errorf("load framework activation %q: %w", frameworkID, err)
		}

		for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
			owned, err := frameworkOutputOwned(output)
			if err != nil {
				return FrameworkRemoveResult{}, err
			}
			if !owned {
				result.SkippedOutputCount++
				result.Warnings = append(result.Warnings, fmt.Sprintf("skip removing %s output %s because it is no longer owned by this runtime activation", frameworkOutputScope(output), output.Path))
				continue
			}
			if isFrameworkConfigOutput(output) {
				removed, err := removeFrameworkConfigOutput(output)
				if err != nil {
					return FrameworkRemoveResult{}, fmt.Errorf("remove framework config output %s: %w", output.Path, err)
				}
				if removed {
					result.RemovedOutputCount++
				}
				continue
			}
			if isFrameworkGeneratedFileOutput(output) {
				removed, err := removeFrameworkGeneratedFileOutput(output)
				if err != nil {
					return FrameworkRemoveResult{}, fmt.Errorf("remove generated framework output %s: %w", output.Path, err)
				}
				if removed {
					result.RemovedOutputCount++
				}
				continue
			}
			if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return FrameworkRemoveResult{}, fmt.Errorf("remove framework output %s: %w", output.Path, err)
			}
			result.RemovedOutputCount++
		}

		if err := RemoveFrameworkActivation(gitDir, frameworkID); err != nil {
			return FrameworkRemoveResult{}, fmt.Errorf("remove framework activation %q: %w", frameworkID, err)
		}
		if err := removeUnreferencedFrameworkCompiledCaches(gitDir, frameworkID); err != nil {
			return FrameworkRemoveResult{}, err
		}
		result.RemovedActivationCount++
	}

	sort.Strings(result.Warnings)
	return result, nil
}

func frameworkOutputCheckFindings(repoRoot string, output FrameworkActivationOutput) ([]FrameworkCheckFinding, error) {
	findings := []FrameworkCheckFinding{}
	findings = append(findings, frameworkSourceFileFindings(repoRoot, output)...)

	if isFrameworkConfigOutput(output) {
		finding, found, err := frameworkConfigOutputFinding(output)
		if err != nil {
			return nil, err
		}
		if found {
			findings = append(findings, finding)
		}

		return findings, nil
	}
	if isFrameworkGeneratedFileOutput(output) {
		finding, found, err := frameworkGeneratedFileOutputFinding(output)
		if err != nil {
			return nil, err
		}
		if found {
			findings = append(findings, finding)
		}

		return findings, nil
	}

	owned, err := frameworkOutputOwned(output)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil || !owned {
		findings = append(findings, FrameworkCheckFinding{
			Kind:    "missing_output",
			Path:    output.Path,
			Message: "framework-managed output is missing or no longer points at the expected runtime target",
		})
	}
	finding, found, err := compiledCommandSkillFinding(repoRoot, output)
	if err != nil {
		return nil, err
	}
	if found {
		findings = append(findings, finding)
	}

	return findings, nil
}

func frameworkSourceFileFindings(repoRoot string, output FrameworkActivationOutput) []FrameworkCheckFinding {
	sourceFiles := append([]string(nil), output.SourceFiles...)
	if len(sourceFiles) == 0 && output.Source != "" {
		sourceFiles = append(sourceFiles, output.Source)
	}
	findings := []FrameworkCheckFinding{}
	seen := map[string]struct{}{}
	for _, source := range sourceFiles {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(source))); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				findings = append(findings, FrameworkCheckFinding{
					Kind:    "source_missing",
					Path:    source,
					Message: "activation source file is missing",
				})
				continue
			}
			findings = append(findings, FrameworkCheckFinding{
				Kind:    "source_unreadable",
				Path:    source,
				Message: err.Error(),
			})
		}
	}

	return findings
}

func compiledCommandSkillFinding(repoRoot string, output FrameworkActivationOutput) (FrameworkCheckFinding, bool, error) {
	if output.Kind != "command_as_skill" || output.Route != "project_skill" {
		return FrameworkCheckFinding{}, false, nil
	}
	expected, err := renderCommandAsProjectSkillData(repoRoot, output.Source, output.Artifact)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{}, false, nil
		}
		return FrameworkCheckFinding{
			Kind:    "compiled_skill_stale",
			Path:    output.Path,
			Message: "compiled command-as-skill cache cannot be rebuilt from source",
		}, true, nil
	}
	actual, err := os.ReadFile(filepath.Join(output.Target, "SKILL.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{
				Kind:    "compiled_skill_missing",
				Path:    output.Path,
				Message: "compiled command-as-skill cache is missing",
			}, true, nil
		}
		return FrameworkCheckFinding{}, false, fmt.Errorf("read compiled command skill for %s: %w", output.Path, err)
	}
	if !bytes.Equal(bytes.TrimSpace(actual), bytes.TrimSpace(expected)) {
		return FrameworkCheckFinding{
			Kind:    "compiled_skill_stale",
			Path:    output.Path,
			Message: "compiled command-as-skill cache is stale relative to source",
		}, true, nil
	}

	return FrameworkCheckFinding{}, false, nil
}

func frameworkGeneratedFileOutputFinding(output FrameworkActivationOutput) (FrameworkCheckFinding, bool, error) {
	compiledData, err := os.ReadFile(output.Target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{
				Kind:    generatedOutputFindingKind(output),
				Path:    output.Path,
				Message: "compiled generated output cache is missing",
			}, true, nil
		}
		return FrameworkCheckFinding{}, false, fmt.Errorf("read compiled generated output for %s: %w", output.Path, err)
	}
	existingData, err := os.ReadFile(output.AbsolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FrameworkCheckFinding{
				Kind:    generatedOutputFindingKind(output),
				Path:    output.Path,
				Message: "generated framework output is missing",
			}, true, nil
		}
		return FrameworkCheckFinding{}, false, fmt.Errorf("read generated output %s: %w", output.Path, err)
	}
	if !bytes.Equal(existingData, compiledData) {
		return FrameworkCheckFinding{
			Kind:    generatedOutputFindingKind(output),
			Path:    output.Path,
			Message: "generated framework output differs from the compiled activation output",
		}, true, nil
	}

	return FrameworkCheckFinding{}, false, nil
}

func generatedOutputFindingKind(output FrameworkActivationOutput) string {
	if output.ArtifactType == "hook-config" || output.ArtifactType == "hook-implementation" {
		return "hook_output_stale"
	}

	return "generated_output_stale"
}

func appendFrameworkHookFindings(repoRoot string, summary FrameworkInspectSummary, findings *[]FrameworkCheckFinding) error {
	effectiveHooks := effectiveAgentHooksForFramework(summary, summary.ResolvedFramework)
	if len(effectiveHooks) == 0 {
		return nil
	}
	supported, unsupported := supportedEffectiveAgentHookEntries(summary.ResolvedFramework, effectiveHooks)
	for _, entry := range unsupported {
		if entry.PackageHook {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:     "package_hook_event_unsupported",
				OrbitID:  entry.OrbitID,
				Path:     hookDisplayIDFromEffective(entry),
				Message:  packageHookUnsupportedMessage(entry),
				Blocking: entry.Required || normalizedPackageHookUnsupportedBehavior(entry.UnsupportedBehavior) == "block",
			})
			continue
		}
		*findings = append(*findings, FrameworkCheckFinding{
			Kind:     "hook_event_unsupported",
			Path:     entry.Entry.ID,
			Message:  "hook event is not supported by the resolved framework",
			Blocking: normalizedPackageHookUnsupportedBehavior(entry.UnsupportedBehavior) == "block",
		})
	}
	seenHandlers := map[string]struct{}{}
	for _, entry := range supported {
		handlerPath := entry.Entry.Handler.Path
		if _, ok := seenHandlers[handlerPath]; ok {
			continue
		}
		seenHandlers[handlerPath] = struct{}{}
		if err := validateAgentHookHandlerPath(handlerPath); err != nil {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:    "hook_handler_invalid",
				Path:    handlerPath,
				Message: "hook handler path is invalid",
			})
			continue
		}
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(handlerPath)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				*findings = append(*findings, FrameworkCheckFinding{
					Kind:    "hook_handler_missing",
					Path:    handlerPath,
					Message: "hook handler is missing",
				})
				continue
			}
			return fmt.Errorf("stat hook handler %s: %w", handlerPath, err)
		}
		if info.IsDir() {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:    "hook_handler_invalid",
				Path:    handlerPath,
				Message: "hook handler is a directory",
			})
			continue
		}
		if info.Mode().Perm()&0o111 == 0 {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:    "hook_handler_not_executable",
				Path:    handlerPath,
				Message: "hook handler is not executable",
			})
		}
	}

	return nil
}

func packageHookUnsupportedMessage(entry effectiveAgentHookEntry) string {
	if entry.Required {
		return "required package hook event is not supported by the resolved framework"
	}

	return "package hook event is not supported by the resolved framework"
}

func appendPackageHookActivationFindings(summary FrameworkInspectSummary, activation FrameworkActivation, findings *[]FrameworkCheckFinding) {
	if len(summary.PackageAgentHooks) == 0 {
		return
	}
	applied := map[string]FrameworkActivationPackageHook{}
	for _, hook := range activation.PackageHooks {
		applied[hook.DisplayID] = hook
	}
	effectiveHooks := effectiveAgentHooksForFramework(summary, summary.ResolvedFramework)
	supported, _ := supportedEffectiveAgentHookEntries(summary.ResolvedFramework, effectiveHooks)
	for _, hook := range supported {
		if !hook.PackageHook {
			continue
		}
		appliedHook, ok := applied[hook.DisplayID]
		if !ok {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:     "package_hook_pending",
				OrbitID:  hook.OrbitID,
				Path:     hook.DisplayID,
				Message:  "package hook add-on has not been applied with --hooks",
				Blocking: true,
			})
			continue
		}
		if appliedHook.HandlerDigest != hook.HandlerDigest ||
			appliedHook.HandlerPath != hook.Entry.Handler.Path ||
			appliedHook.EventKind != hook.Entry.Event.Kind ||
			appliedHook.NativeEvent != hook.NativeEvent {
			*findings = append(*findings, FrameworkCheckFinding{
				Kind:     "package_hook_stale",
				OrbitID:  hook.OrbitID,
				Path:     hook.DisplayID,
				Message:  "package hook activation is stale relative to current package add-on truth",
				Blocking: hook.Required,
			})
		}
	}
}

func removeUnreferencedFrameworkCompiledCaches(gitDir string, frameworkID string) error {
	compiledRoot := filepath.Join(gitDir, "orbit", "state", "agents", "compiled", frameworkID)
	if !pathInsideFrameworkCompiledRoot(gitDir, compiledRoot) {
		return nil
	}
	if err := os.RemoveAll(compiledRoot); err != nil {
		return fmt.Errorf("remove compiled framework cache %s: %w", compiledRoot, err)
	}

	return nil
}

func pathInsideFrameworkCompiledRoot(gitDir string, path string) bool {
	root := filepath.Join(gitDir, "orbit", "state", "agents", "compiled")
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != "." && relative != "" && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}

type frameworkManagedOutputs struct {
	ProjectOutputs        []FrameworkActivationOutput
	GlobalOutputs         []FrameworkActivationOutput
	FallbackGlobalOutputs []FrameworkActivationOutput
}

func buildFrameworkManagedOutputs(repoRoot string, gitDir string, harnessID string, summary FrameworkInspectSummary, routeChoice FrameworkApplyRouteChoice, enableHooks bool) (frameworkManagedOutputs, []FrameworkApplyArtifactResult, error) {
	if summary.ResolvedFramework == "" {
		return frameworkManagedOutputs{}, nil, fmt.Errorf("framework resolution is unresolved")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return frameworkManagedOutputs{}, nil, fmt.Errorf("resolve user home directory: %w", err)
	}
	adapter, ok := LookupFrameworkAdapter(summary.ResolvedFramework)
	if !ok {
		return frameworkManagedOutputs{}, nil, fmt.Errorf("framework %q is not supported by this build", summary.ResolvedFramework)
	}

	outputs := frameworkManagedOutputs{
		ProjectOutputs:        []FrameworkActivationOutput{},
		GlobalOutputs:         []FrameworkActivationOutput{},
		FallbackGlobalOutputs: []FrameworkActivationOutput{},
	}
	artifactResults := []FrameworkApplyArtifactResult{}
	if adapter.ProjectAliasPath != "" && summary.HasAgentGuidance {
		outputs.ProjectOutputs = append(outputs.ProjectOutputs, FrameworkActivationOutput{
			Path:         adapter.ProjectAliasPath,
			AbsolutePath: filepath.Join(repoRoot, filepath.FromSlash(adapter.ProjectAliasPath)),
			Kind:         "framework_alias",
			Action:       "symlink",
			Target:       filepath.Join(repoRoot, "AGENTS.md"),
		})
	}

	switch routeChoice {
	case FrameworkApplyRouteGlobal:
		for _, routeOutput := range buildOptionalGlobalRouteOutputs(adapter.ID, harnessID, summary) {
			activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
			if err != nil {
				return frameworkManagedOutputs{}, nil, err
			}
			outputs.GlobalOutputs = append(outputs.GlobalOutputs, activationOutput)
			artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "global_applied"))
		}
		for _, routeOutput := range buildCompatibilityRouteOutputs(adapter.ID, summary) {
			if routeOutput.ArtifactType == "agent-config" {
				continue
			}
			artifactResults = append(artifactResults, artifactResultFromRoute(summary.ResolvedFramework, routeOutput, "compatibility_pending", "project compatibility for this artifact is not implemented yet"))
		}
		if enableHooks {
			hookRoutes := buildAgentHookRouteOutputs(adapter.ID, summary)
			if err := appendUnsupportedHookResults(summary.ResolvedFramework, hookRoutes, &artifactResults); err != nil {
				return frameworkManagedOutputs{}, nil, err
			}
			hookGlobalOutputs := hookRoutes.OptionalGlobalOutputs
			if adapter.ID == "openclaw" {
				hookGlobalOutputs = append(append([]FrameworkRoutePlanOutput{}, hookRoutes.HybridProjectOutputs...), hookRoutes.HybridGlobalOutputs...)
			}
			for _, routeOutput := range hookGlobalOutputs {
				activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
				if err != nil {
					return frameworkManagedOutputs{}, nil, err
				}
				if strings.HasPrefix(routeOutput.Path, "~/") {
					outputs.GlobalOutputs = append(outputs.GlobalOutputs, activationOutput)
					artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "global_applied"))
					continue
				}
				outputs.ProjectOutputs = append(outputs.ProjectOutputs, activationOutput)
				artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "project_applied"))
			}
		}
	case FrameworkApplyRouteProject, FrameworkApplyRouteAuto:
		for _, routeOutput := range buildRecommendedProjectRouteOutputs(adapter.ID, summary) {
			activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
			if err != nil {
				return frameworkManagedOutputs{}, nil, err
			}
			outputs.ProjectOutputs = append(outputs.ProjectOutputs, activationOutput)
			artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "project_applied"))
		}
		for _, routeOutput := range buildCompatibilityRouteOutputs(adapter.ID, summary) {
			artifactResults = append(artifactResults, artifactResultFromRoute(summary.ResolvedFramework, routeOutput, "compatibility_pending", "project compatibility for this artifact is not implemented yet"))
		}
		for _, routeOutput := range buildOptionalGlobalRouteOutputs(adapter.ID, harnessID, summary) {
			activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
			if err != nil {
				return frameworkManagedOutputs{}, nil, err
			}
			outputs.FallbackGlobalOutputs = append(outputs.FallbackGlobalOutputs, activationOutput)
		}
		if enableHooks {
			hookRoutes := buildAgentHookRouteOutputs(adapter.ID, summary)
			if err := appendUnsupportedHookResults(summary.ResolvedFramework, hookRoutes, &artifactResults); err != nil {
				return frameworkManagedOutputs{}, nil, err
			}
			for _, routeOutput := range append(append([]FrameworkRoutePlanOutput{}, hookRoutes.ProjectOutputs...), hookRoutes.HybridProjectOutputs...) {
				activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
				if err != nil {
					return frameworkManagedOutputs{}, nil, err
				}
				outputs.ProjectOutputs = append(outputs.ProjectOutputs, activationOutput)
				artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "project_applied"))
			}
			for _, routeOutput := range hookRoutes.HybridGlobalOutputs {
				activationOutput, err := activationOutputFromRoute(repoRoot, gitDir, homeDir, adapter.ID, routeOutput, summary)
				if err != nil {
					return frameworkManagedOutputs{}, nil, err
				}
				outputs.GlobalOutputs = append(outputs.GlobalOutputs, activationOutput)
				artifactResults = append(artifactResults, artifactResultFromActivationOutput(summary.ResolvedFramework, activationOutput, "global_applied"))
			}
		}
	default:
		return frameworkManagedOutputs{}, nil, fmt.Errorf("unsupported framework apply route choice %q", routeChoice)
	}

	if len(summary.RemoteSkills) > 0 && !adapter.RemoteSkillsSupported {
		for _, remoteSkill := range summary.RemoteSkills {
			if !remoteSkill.Required {
				continue
			}
			return frameworkManagedOutputs{}, nil, fmt.Errorf(
				"framework %q does not support remote skill URI %q",
				adapter.ID,
				remoteSkill.URI,
			)
		}
	}

	sort.Slice(outputs.ProjectOutputs, func(left, right int) bool {
		return outputs.ProjectOutputs[left].Path < outputs.ProjectOutputs[right].Path
	})
	sort.Slice(outputs.GlobalOutputs, func(left, right int) bool {
		return outputs.GlobalOutputs[left].Path < outputs.GlobalOutputs[right].Path
	})
	sortFrameworkApplyArtifactResults(artifactResults)

	return outputs, artifactResults, nil
}

func activationOutputFromRoute(repoRoot string, gitDir string, homeDir string, frameworkID string, routeOutput FrameworkRoutePlanOutput, summary FrameworkInspectSummary) (FrameworkActivationOutput, error) {
	if routeOutput.ArtifactType == "agent-config" {
		return activationOutputFromAgentConfigRoute(repoRoot, gitDir, homeDir, frameworkID, routeOutput)
	}
	if routeOutput.ArtifactType == "hook-config" || routeOutput.ArtifactType == "hook-implementation" {
		return activationOutputFromAgentHookRoute(repoRoot, gitDir, homeDir, frameworkID, routeOutput, summary)
	}

	target := filepath.Join(repoRoot, filepath.FromSlash(routeOutput.Source))
	if routeOutput.Route == "project_skill" && routeOutput.Kind == "command_as_skill" {
		compiledRoot, err := compileCommandAsProjectSkill(repoRoot, gitDir, frameworkID, routeOutput)
		if err != nil {
			return FrameworkActivationOutput{}, err
		}
		target = compiledRoot
	}

	return FrameworkActivationOutput{
		Path:           routeOutput.Path,
		AbsolutePath:   frameworkRouteAbsolutePath(repoRoot, homeDir, routeOutput.Path),
		Kind:           routeOutput.Kind,
		Action:         routeOutput.Action,
		Target:         target,
		OrbitID:        routeOutput.OrbitID,
		Package:        routeOutput.Package,
		AddonID:        routeOutput.AddonID,
		Artifact:       routeOutput.Artifact,
		ArtifactType:   routeOutput.ArtifactType,
		Source:         routeOutput.Source,
		Sidecar:        routeOutput.Sidecar,
		SourceFiles:    append([]string(nil), routeOutput.SourceFiles...),
		Route:          routeOutput.Route,
		Mode:           routeOutput.Mode,
		EffectiveScope: routeOutput.EffectiveScope,
		Invocation:     append([]string(nil), routeOutput.Invocation...),
		GeneratedKeys:  append([]string(nil), routeOutput.GeneratedKeys...),
		PatchOwnedKeys: append([]string(nil), routeOutput.PatchOwnedKeys...),
		HandlerDigest:  routeOutput.HandlerDigest,
	}, nil
}

func appendUnsupportedHookResults(frameworkID string, hookRoutes agentHookRouteSet, artifactResults *[]FrameworkApplyArtifactResult) error {
	for _, routeOutput := range hookRoutes.UnsupportedOutputs {
		if routeOutput.Required && routeOutput.Package != "" {
			return fmt.Errorf("required package hook %q event is not supported by framework %q", routeOutput.Artifact, frameworkID)
		}
		if routeOutput.Mode == "block" {
			return fmt.Errorf("hook %q event is not supported by framework %q", routeOutput.Artifact, frameworkID)
		}
		*artifactResults = append(*artifactResults, artifactResultFromRoute(frameworkID, routeOutput, "unsupported_skipped", "hook event is not supported by the selected framework"))
	}

	return nil
}

func frameworkRouteAbsolutePath(repoRoot string, homeDir string, routePath string) string {
	if strings.HasPrefix(routePath, "~/") {
		return filepath.Join(homeDir, filepath.FromSlash(strings.TrimPrefix(routePath, "~/")))
	}

	return filepath.Join(repoRoot, filepath.FromSlash(routePath))
}

func artifactResultFromActivationOutput(frameworkID string, output FrameworkActivationOutput, status string) FrameworkApplyArtifactResult {
	return FrameworkApplyArtifactResult{
		Framework:      frameworkID,
		OrbitID:        output.OrbitID,
		Package:        output.Package,
		AddonID:        output.AddonID,
		Artifact:       output.Artifact,
		ArtifactType:   output.ArtifactType,
		Route:          output.Route,
		Mode:           output.Mode,
		Source:         output.Source,
		Sidecar:        output.Sidecar,
		SourceFiles:    append([]string(nil), output.SourceFiles...),
		Path:           output.Path,
		Target:         output.Target,
		EffectiveScope: output.EffectiveScope,
		Status:         status,
		Invocation:     append([]string(nil), output.Invocation...),
		GeneratedKeys:  append([]string(nil), output.GeneratedKeys...),
		PatchOwnedKeys: append([]string(nil), output.PatchOwnedKeys...),
		HandlerDigest:  output.HandlerDigest,
	}
}

func artifactResultFromRoute(frameworkID string, routeOutput FrameworkRoutePlanOutput, status string, message string) FrameworkApplyArtifactResult {
	return FrameworkApplyArtifactResult{
		Framework:      frameworkID,
		OrbitID:        routeOutput.OrbitID,
		Package:        routeOutput.Package,
		AddonID:        routeOutput.AddonID,
		Artifact:       routeOutput.Artifact,
		ArtifactType:   routeOutput.ArtifactType,
		Route:          routeOutput.Route,
		Mode:           routeOutput.Mode,
		Required:       routeOutput.Required,
		Source:         routeOutput.Source,
		Sidecar:        routeOutput.Sidecar,
		SourceFiles:    append([]string(nil), routeOutput.SourceFiles...),
		Path:           routeOutput.Path,
		EffectiveScope: routeOutput.EffectiveScope,
		Status:         status,
		Message:        message,
		Invocation:     append([]string(nil), routeOutput.Invocation...),
		GeneratedKeys:  append([]string(nil), routeOutput.GeneratedKeys...),
		PatchOwnedKeys: append([]string(nil), routeOutput.PatchOwnedKeys...),
		HandlerDigest:  routeOutput.HandlerDigest,
	}
}

func frameworkApplyOverallStatus(results []FrameworkApplyArtifactResult) string {
	fallbackApplied := map[string]struct{}{}
	compatibilityPending := map[string]struct{}{}
	for _, result := range results {
		key := frameworkArtifactKey(result.OrbitID, result.ArtifactType, result.Artifact)
		if result.Status == "global_applied" {
			fallbackApplied[key] = struct{}{}
		}
		if result.Status == "compatibility_pending" {
			compatibilityPending[key] = struct{}{}
		}
	}
	for _, result := range results {
		switch result.Status {
		case "project_failed", "global_failed":
			if result.Status == "project_failed" {
				key := frameworkArtifactKey(result.OrbitID, result.ArtifactType, result.Artifact)
				if _, ok := fallbackApplied[key]; ok {
					continue
				}
				if _, ok := compatibilityPending[key]; ok {
					continue
				}
			}
			return "blocked"
		}
	}
	for _, result := range results {
		switch result.Status {
		case "compatibility_pending", "unsupported_skipped", "global_fallback_declined", "skipped", "confirmation_required":
			return "partial"
		}
	}

	return "ok"
}

func sortFrameworkApplyArtifactResults(results []FrameworkApplyArtifactResult) {
	sort.Slice(results, func(left, right int) bool {
		if results[left].EffectiveScope != results[right].EffectiveScope {
			return results[left].EffectiveScope < results[right].EffectiveScope
		}
		if results[left].Path != results[right].Path {
			return results[left].Path < results[right].Path
		}
		if results[left].ArtifactType != results[right].ArtifactType {
			return results[left].ArtifactType < results[right].ArtifactType
		}
		return results[left].Artifact < results[right].Artifact
	})
}

func frameworkFallbackOutputsByArtifactKey(outputs []FrameworkActivationOutput) map[string]FrameworkActivationOutput {
	byArtifact := make(map[string]FrameworkActivationOutput, len(outputs))
	for _, output := range outputs {
		if output.Artifact == "" {
			continue
		}
		byArtifact[frameworkArtifactKey(output.OrbitID, output.ArtifactType, output.Artifact)] = output
	}

	return byArtifact
}

func markFrameworkApplyArtifactResult(results []FrameworkApplyArtifactResult, output FrameworkActivationOutput, status string, message string) {
	outputKey := frameworkArtifactKey(output.OrbitID, output.ArtifactType, output.Artifact)
	for index := range results {
		resultKey := frameworkArtifactKey(results[index].OrbitID, results[index].ArtifactType, results[index].Artifact)
		if resultKey != outputKey || results[index].Route != output.Route {
			continue
		}
		results[index].Status = status
		results[index].Message = message
		return
	}
}

func frameworkArtifactKey(orbitID string, artifactType string, artifact string) string {
	return orbitID + "\x00" + artifactType + "\x00" + artifact
}

func frameworkOutputConflictCanDefer(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "conflicting unmanaged key")
}

func ensureFrameworkOutput(output FrameworkActivationOutput) (bool, error) {
	if isFrameworkConfigOutput(output) {
		return ensureFrameworkConfigOutput(output)
	}
	if isFrameworkGeneratedFileOutput(output) {
		return ensureFrameworkGeneratedFileOutput(output)
	}

	if err := os.MkdirAll(filepath.Dir(output.AbsolutePath), 0o750); err != nil {
		return false, fmt.Errorf("create parent directory for framework output %s: %w", output.Path, err)
	}

	owned, err := frameworkOutputOwned(output)
	if err == nil && owned {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil && !owned {
		return false, fmt.Errorf("framework output %s already exists but is not owned by this runtime activation", output.Path)
	}

	info, statErr := os.Lstat(output.AbsolutePath)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return false, fmt.Errorf("framework output %s already exists but is not a symlink", output.Path)
		}
		return false, fmt.Errorf("framework output %s already exists but points to a different target", output.Path)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("stat framework output %s: %w", output.Path, statErr)
	}

	if err := os.Symlink(output.Target, output.AbsolutePath); err != nil {
		return false, fmt.Errorf("create symlink for framework output %s: %w", output.Path, err)
	}

	return true, nil
}

func frameworkOutputOwned(output FrameworkActivationOutput) (bool, error) {
	if isFrameworkConfigOutput(output) {
		return frameworkConfigOutputOwned(output)
	}
	if isFrameworkGeneratedFileOutput(output) {
		return frameworkGeneratedFileOutputOwned(output)
	}

	info, err := os.Lstat(output.AbsolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, os.ErrNotExist
		}
		return false, fmt.Errorf("lstat framework output %s: %w", output.Path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}

	target, err := os.Readlink(output.AbsolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, os.ErrNotExist
		}
		return false, fmt.Errorf("readlink framework output %s: %w", output.Path, err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(output.AbsolutePath), target)
	}

	return filepath.Clean(target) == filepath.Clean(output.Target), nil
}

func rollbackFrameworkOutput(output FrameworkActivationOutput) error {
	if isFrameworkConfigOutput(output) {
		return rollbackFrameworkConfigOutput(output)
	}
	if isFrameworkGeneratedFileOutput(output) {
		_, err := removeFrameworkGeneratedFileOutput(output)
		return err
	}
	if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove framework output %s: %w", output.Path, err)
	}

	return nil
}

func frameworkOutputParentWritable(path string) (bool, error) {
	current := filepath.Dir(path)
	for {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return false, nil
			}
			return info.Mode().Perm()&0o200 != 0, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("stat %s: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		current = parent
	}
}

func frameworkOutputScope(output FrameworkActivationOutput) string {
	if strings.HasPrefix(output.Path, "~") {
		return "global"
	}

	return "project"
}
