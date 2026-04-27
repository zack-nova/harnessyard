package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	AgentCleanupStatusNotNeeded            = "not_needed"
	AgentCleanupStatusPlanned              = "planned"
	AgentCleanupStatusApplied              = "applied"
	AgentCleanupStatusBlocked              = "blocked"
	AgentCleanupStatusConfirmationRequired = "confirmation_required"
	AgentCleanupStatusSkippedByUser        = "skipped_by_user"
)

// AgentCleanupResult reports package-remove reconciliation for agent side effects.
type AgentCleanupResult struct {
	Status               string   `json:"status"`
	RemovedOutputs       []string `json:"removed_outputs,omitempty"`
	RecompiledOutputs    []string `json:"recompiled_outputs,omitempty"`
	GlobalOutputsTouched []string `json:"global_outputs_touched,omitempty"`
	BlockedOutputs       []string `json:"blocked_outputs,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

// AgentCleanupOptions controls package-remove agent side-effect reconciliation.
type AgentCleanupOptions struct {
	AllowGlobal bool
}

// PlanAgentCleanupForPackageRemove previews whether active agent ledgers need
// cleanup before the package payload is removed.
func PlanAgentCleanupForPackageRemove(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	removedOrbitIDs []string,
	options AgentCleanupOptions,
) (AgentCleanupResult, error) {
	_ = ctx
	_ = repoRoot

	activations, err := packageRemoveCleanupActivations(gitDir, removedOrbitIDs)
	if err != nil {
		return AgentCleanupResult{}, err
	}
	if len(activations) == 0 {
		return AgentCleanupResult{Status: AgentCleanupStatusNotNeeded}, nil
	}

	result := AgentCleanupResult{
		Status: AgentCleanupStatusPlanned,
	}
	removed := agentCleanupStringSet(removedOrbitIDs)
	for _, activation := range activations {
		for _, output := range packageRemoveCleanupOutputs(activation, removed) {
			if frameworkOutputRequiresGlobalConfirmation(output) {
				result.GlobalOutputsTouched = appendUniqueString(result.GlobalOutputsTouched, output.Path)
				if !options.AllowGlobal {
					result.Status = AgentCleanupStatusConfirmationRequired
				}
			}
			exists, err := frameworkOutputExists(output)
			if err != nil {
				return AgentCleanupResult{}, err
			}
			if !exists {
				continue
			}
			owned, err := frameworkOutputOwned(output)
			if err != nil {
				return AgentCleanupResult{}, err
			}
			if !owned {
				result.BlockedOutputs = appendUniqueString(result.BlockedOutputs, output.Path)
			}
		}
	}
	if len(result.BlockedOutputs) > 0 {
		result.Status = AgentCleanupStatusBlocked
	}
	sortAgentCleanupResult(&result)

	return result, nil
}

// ReconcileAgentCleanupAfterPackageRemove reconciles package-owned agent side
// effects after runtime/package truth has been updated.
func ReconcileAgentCleanupAfterPackageRemove(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	removedOrbitIDs []string,
	options AgentCleanupOptions,
) (AgentCleanupResult, error) {
	activations, err := packageRemoveCleanupActivations(gitDir, removedOrbitIDs)
	if err != nil {
		return AgentCleanupResult{}, err
	}
	if len(activations) == 0 {
		return AgentCleanupResult{Status: AgentCleanupStatusNotNeeded}, nil
	}

	preflight, err := PlanAgentCleanupForPackageRemove(ctx, repoRoot, gitDir, removedOrbitIDs, options)
	if err != nil {
		return AgentCleanupResult{}, err
	}
	switch preflight.Status {
	case AgentCleanupStatusBlocked, AgentCleanupStatusConfirmationRequired:
		return preflight, nil
	}

	state, err := loadFrameworkDesiredState(ctx, repoRoot, gitDir)
	if err != nil {
		return AgentCleanupResult{}, fmt.Errorf("load remaining agent desired state: %w", err)
	}
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return AgentCleanupResult{}, fmt.Errorf("load runtime file for agent cleanup: %w", err)
	}

	result := AgentCleanupResult{
		Status:               AgentCleanupStatusApplied,
		GlobalOutputsTouched: append([]string(nil), preflight.GlobalOutputsTouched...),
	}
	removed := agentCleanupStringSet(removedOrbitIDs)
	for _, activation := range activations {
		activationResult, err := reconcilePackageRemoveActivation(ctx, repoRoot, gitDir, runtimeFile.Harness.ID, state, activation, removed, options)
		if err != nil {
			return AgentCleanupResult{}, err
		}
		result.RemovedOutputs = append(result.RemovedOutputs, activationResult.RemovedOutputs...)
		result.RecompiledOutputs = append(result.RecompiledOutputs, activationResult.RecompiledOutputs...)
		result.GlobalOutputsTouched = append(result.GlobalOutputsTouched, activationResult.GlobalOutputsTouched...)
		result.BlockedOutputs = append(result.BlockedOutputs, activationResult.BlockedOutputs...)
		result.Warnings = append(result.Warnings, activationResult.Warnings...)
	}
	if len(result.BlockedOutputs) > 0 {
		result.Status = AgentCleanupStatusBlocked
	}
	sortAgentCleanupResult(&result)

	return result, nil
}

func reconcilePackageRemoveActivation(
	ctx context.Context,
	repoRoot string,
	gitDir string,
	harnessID string,
	state frameworkDesiredState,
	activation FrameworkActivation,
	removed map[string]struct{},
	options AgentCleanupOptions,
) (AgentCleanupResult, error) {
	_ = ctx

	result := AgentCleanupResult{Status: AgentCleanupStatusApplied}
	oldCleanupOutputs := packageRemoveCleanupOutputs(activation, removed)
	for _, output := range oldCleanupOutputs {
		if frameworkOutputRequiresGlobalConfirmation(output) {
			result.GlobalOutputsTouched = appendUniqueString(result.GlobalOutputsTouched, output.Path)
			if !options.AllowGlobal {
				result.Status = AgentCleanupStatusConfirmationRequired
				result.BlockedOutputs = appendUniqueString(result.BlockedOutputs, output.Path)
				return result, nil
			}
		}
		removed, err := removeFrameworkOwnedOutputIfPresent(output)
		if err != nil {
			return AgentCleanupResult{}, err
		}
		if removed {
			result.RemovedOutputs = appendUniqueString(result.RemovedOutputs, output.Path)
		}
	}

	routeChoice := packageRemoveRouteChoice(activation)
	enableHooks := packageRemoveActivationHadHooks(activation)
	newProjectOutputs := []FrameworkActivationOutput{}
	newGlobalOutputs := []FrameworkActivationOutput{}
	if state.Summary.ResolvedFramework == activation.Framework {
		managedOutputs, _, err := buildFrameworkManagedOutputs(repoRoot, gitDir, harnessID, state.Summary, routeChoice, enableHooks)
		if err != nil {
			return AgentCleanupResult{}, err
		}
		newProjectOutputs = managedOutputs.ProjectOutputs
		newGlobalOutputs = managedOutputs.GlobalOutputs
	}

	recompiled := map[string]struct{}{}
	newTargets := map[string]struct{}{}
	for _, output := range append(append([]FrameworkActivationOutput{}, newProjectOutputs...), newGlobalOutputs...) {
		if frameworkOutputRequiresGlobalConfirmation(output) {
			result.GlobalOutputsTouched = appendUniqueString(result.GlobalOutputsTouched, output.Path)
			if !options.AllowGlobal {
				result.Status = AgentCleanupStatusConfirmationRequired
				result.BlockedOutputs = appendUniqueString(result.BlockedOutputs, output.Path)
				return result, nil
			}
		}
		changed, err := ensureFrameworkOutput(output)
		if err != nil {
			return AgentCleanupResult{}, err
		}
		if changed {
			result.RecompiledOutputs = appendUniqueString(result.RecompiledOutputs, output.Path)
		}
		recompiled[output.Path] = struct{}{}
		if output.Target != "" {
			newTargets[filepath.Clean(output.Target)] = struct{}{}
		}
	}
	result.RemovedOutputs = removeStringsPresentInSet(result.RemovedOutputs, recompiled)
	if err := removeUnreferencedPackageHookCompiledCaches(gitDir, oldCleanupOutputs, newTargets); err != nil {
		return AgentCleanupResult{}, err
	}

	activation.ProjectOutputs = newProjectOutputs
	activation.GlobalOutputs = newGlobalOutputs
	sortFrameworkActivationOutputs(activation.ProjectOutputs)
	sortFrameworkActivationOutputs(activation.GlobalOutputs)

	activation.PackageHooks = []FrameworkActivationPackageHook{}
	if enableHooks && state.Summary.ResolvedFramework == activation.Framework {
		activation.PackageHooks = frameworkActivationPackageHooks(state.Summary, routeChoice)
	}
	if len(activation.ProjectOutputs) == 0 && len(activation.GlobalOutputs) == 0 && len(activation.PackageHooks) == 0 {
		if err := RemoveFrameworkActivation(gitDir, activation.Framework); err != nil {
			return AgentCleanupResult{}, fmt.Errorf("remove empty framework activation %q: %w", activation.Framework, err)
		}
		if err := removeUnreferencedFrameworkCompiledCaches(gitDir, activation.Framework); err != nil {
			return AgentCleanupResult{}, err
		}
		return result, nil
	}

	guidanceHash, capabilitiesHash, selectionHash, runtimeAgentTruthHash, err := computeFrameworkDesiredHashes(repoRoot, state)
	if err != nil {
		return AgentCleanupResult{}, err
	}
	activation.GuidanceHash = guidanceHash
	activation.CapabilitiesHash = capabilitiesHash
	activation.SelectionHash = selectionHash
	activation.RuntimeAgentTruthHash = runtimeAgentTruthHash
	activation.AppliedAt = time.Now().UTC()
	if _, err := WriteFrameworkActivation(gitDir, activation); err != nil {
		return AgentCleanupResult{}, fmt.Errorf("write reconciled framework activation %q: %w", activation.Framework, err)
	}

	return result, nil
}

func packageRemoveCleanupActivations(gitDir string, removedOrbitIDs []string) ([]FrameworkActivation, error) {
	removed := agentCleanupStringSet(removedOrbitIDs)
	activationIDs, err := ListFrameworkActivationIDs(gitDir)
	if err != nil {
		return nil, fmt.Errorf("list framework activations for agent cleanup: %w", err)
	}
	activations := []FrameworkActivation{}
	for _, frameworkID := range activationIDs {
		activation, err := LoadFrameworkActivation(gitDir, frameworkID)
		if err != nil {
			return nil, fmt.Errorf("load framework activation %q for agent cleanup: %w", frameworkID, err)
		}
		if !activationReferencesRemovedPackage(activation, removed) {
			continue
		}
		activations = append(activations, activation)
	}

	return activations, nil
}

func activationReferencesRemovedPackage(activation FrameworkActivation, removed map[string]struct{}) bool {
	for _, hook := range activation.PackageHooks {
		if _, ok := removed[hook.OrbitID]; ok {
			return true
		}
		if _, ok := removed[hook.Package]; ok {
			return true
		}
	}
	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if outputReferencesRemovedPackage(output, removed) {
			return true
		}
	}

	return false
}

func packageRemoveCleanupOutputs(activation FrameworkActivation, removed map[string]struct{}) []FrameworkActivationOutput {
	hookCleanup := activationHasRemovedPackageHook(activation, removed)
	outputs := []FrameworkActivationOutput{}
	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if outputReferencesRemovedPackage(output, removed) || (hookCleanup && isFrameworkHookOutput(output)) {
			outputs = append(outputs, output)
		}
	}
	sortFrameworkActivationOutputs(outputs)

	return outputs
}

func activationHasRemovedPackageHook(activation FrameworkActivation, removed map[string]struct{}) bool {
	for _, hook := range activation.PackageHooks {
		if _, ok := removed[hook.OrbitID]; ok {
			return true
		}
		if _, ok := removed[hook.Package]; ok {
			return true
		}
	}

	return false
}

func outputReferencesRemovedPackage(output FrameworkActivationOutput, removed map[string]struct{}) bool {
	if _, ok := removed[output.OrbitID]; ok && output.OrbitID != "" {
		return true
	}
	if _, ok := removed[output.Package]; ok && output.Package != "" {
		return true
	}

	return false
}

func packageRemoveActivationHadHooks(activation FrameworkActivation) bool {
	if len(activation.PackageHooks) > 0 {
		return true
	}
	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if isFrameworkHookOutput(output) {
			return true
		}
	}

	return false
}

func isFrameworkHookOutput(output FrameworkActivationOutput) bool {
	return output.ArtifactType == "hook-config" || output.ArtifactType == "hook-implementation"
}

func removeFrameworkOwnedOutputIfPresent(output FrameworkActivationOutput) (bool, error) {
	exists, err := frameworkOutputExists(output)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	owned, err := frameworkOutputOwned(output)
	if err != nil {
		return false, err
	}
	if !owned {
		return false, fmt.Errorf("framework output %s exists but is not owned by this runtime activation", output.Path)
	}
	if isFrameworkConfigOutput(output) {
		return removeFrameworkConfigOutput(output)
	}
	if isFrameworkGeneratedFileOutput(output) {
		return removeFrameworkGeneratedFileOutput(output)
	}
	if err := os.Remove(output.AbsolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove framework output %s: %w", output.Path, err)
	}

	return true, nil
}

func removeUnreferencedPackageHookCompiledCaches(gitDir string, oldOutputs []FrameworkActivationOutput, newTargets map[string]struct{}) error {
	for _, output := range oldOutputs {
		if strings.TrimSpace(output.Target) == "" {
			continue
		}
		target := filepath.Clean(output.Target)
		if _, retained := newTargets[target]; retained {
			continue
		}
		if !pathInsideFrameworkCompiledRoot(gitDir, target) {
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove compiled hook cache for %s: %w", output.Path, err)
		}
	}

	return nil
}

func frameworkOutputExists(output FrameworkActivationOutput) (bool, error) {
	_, err := os.Lstat(output.AbsolutePath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat framework output %s: %w", output.Path, err)
}

func frameworkOutputRequiresGlobalConfirmation(output FrameworkActivationOutput) bool {
	return strings.HasPrefix(output.Path, "~/") ||
		output.EffectiveScope == "global" ||
		output.EffectiveScope == "hybrid"
}

func packageRemoveRouteChoice(activation FrameworkActivation) FrameworkApplyRouteChoice {
	for _, hook := range activation.PackageHooks {
		if hook.HookApplyMode == "global_hooks" {
			return FrameworkApplyRouteGlobal
		}
	}
	for _, output := range append(append([]FrameworkActivationOutput{}, activation.ProjectOutputs...), activation.GlobalOutputs...) {
		if frameworkOutputRequiresGlobalConfirmation(output) {
			return FrameworkApplyRouteGlobal
		}
	}

	return FrameworkApplyRouteProject
}

func sortFrameworkActivationOutputs(outputs []FrameworkActivationOutput) {
	sort.Slice(outputs, func(left, right int) bool {
		if outputs[left].Path == outputs[right].Path {
			return outputs[left].Artifact < outputs[right].Artifact
		}
		return outputs[left].Path < outputs[right].Path
	})
}

func sortAgentCleanupResult(result *AgentCleanupResult) {
	result.RemovedOutputs = sortedUniqueStrings(result.RemovedOutputs)
	result.RecompiledOutputs = sortedUniqueStrings(result.RecompiledOutputs)
	result.GlobalOutputsTouched = sortedUniqueStrings(result.GlobalOutputsTouched)
	result.BlockedOutputs = sortedUniqueStrings(result.BlockedOutputs)
	result.Warnings = sortedUniqueStrings(result.Warnings)
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}

	return append(values, value)
}

func removeStringsPresentInSet(values []string, remove map[string]struct{}) []string {
	filtered := []string{}
	for _, value := range values {
		if _, ok := remove[value]; ok {
			continue
		}
		filtered = append(filtered, value)
	}

	return filtered
}

func agentCleanupStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}

	return result
}

func agentCleanupRequiresConfirmation(result AgentCleanupResult) bool {
	return result.Status == AgentCleanupStatusConfirmationRequired
}

func agentCleanupBlocked(result AgentCleanupResult) bool {
	return result.Status == AgentCleanupStatusBlocked
}

func agentCleanupErrorMessage(result AgentCleanupResult) string {
	switch result.Status {
	case AgentCleanupStatusConfirmationRequired:
		return fmt.Sprintf("agent cleanup for user-level outputs requires --yes: %s", strings.Join(result.GlobalOutputsTouched, ", "))
	case AgentCleanupStatusBlocked:
		return fmt.Sprintf("agent cleanup is blocked by unowned outputs: %s", strings.Join(result.BlockedOutputs, ", "))
	default:
		return "agent cleanup cannot continue"
	}
}

func appendAgentCleanupRemovedPaths(paths []string, cleanup AgentCleanupResult) []string {
	paths = append(paths, cleanup.RemovedOutputs...)
	return sortedUniqueStrings(paths)
}
