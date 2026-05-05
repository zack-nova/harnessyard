package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const runtimeAgentsRepoPath = "AGENTS.md"

// MissingBindingsInput selects one or more install-backed orbits for declaration-vs-vars inspection.
type MissingBindingsInput struct {
	RepoRoot string
	OrbitID  string
	All      bool
}

// MissingBindingsResult captures stable missing-bindings diagnostics for one runtime.
type MissingBindingsResult struct {
	HarnessID string
	Orbits    []MissingBindingsOrbitResult
}

// MissingBindingsOrbitResult captures one orbit's declaration-vs-vars comparison.
type MissingBindingsOrbitResult struct {
	OrbitID         string
	SnapshotMissing bool
	DeclaredCount   int
	MissingCount    int
	Variables       []MissingBindingsVariableResult
}

// MissingBindingsVariableResult captures one declared variable's current value status.
type MissingBindingsVariableResult struct {
	Name                      string
	Namespace                 string
	Description               string
	Required                  bool
	HasValue                  bool
	ObservedRuntimeUnresolved bool
	Missing                   bool
}

// ScanRuntimeBindingsInput selects one or more install-backed orbits for runtime placeholder scanning.
type ScanRuntimeBindingsInput struct {
	RepoRoot     string
	OrbitID      string
	All          bool
	WriteInstall bool
}

// ScanRuntimeBindingsResult captures stable runtime placeholder observations for one runtime.
type ScanRuntimeBindingsResult struct {
	HarnessID    string
	WroteInstall bool
	Orbits       []ScanRuntimeBindingsOrbitResult
}

// ScanRuntimeBindingsOrbitResult captures one orbit's runtime placeholder observations.
type ScanRuntimeBindingsOrbitResult struct {
	OrbitID                   string
	PathCount                 int
	PlaceholderCount          int
	VariableNamespaces        map[string]string
	ObservedRuntimeUnresolved []string
	WroteInstall              bool
	Paths                     []ScanRuntimeBindingsPathResult
}

// ScanRuntimeBindingsPathResult captures placeholders still present in one current runtime file.
type ScanRuntimeBindingsPathResult struct {
	Path      string
	Variables []string
}

// InspectMissingBindings compares install declaration snapshots with the current .harness/vars.yaml file.
func InspectMissingBindings(ctx context.Context, input MissingBindingsInput) (MissingBindingsResult, error) {
	runtimeFile, targets, err := loadInstallBackedBindingsTargets(input.RepoRoot, input.OrbitID, input.All)
	if err != nil {
		return MissingBindingsResult{}, err
	}

	varsFile, err := loadOptionalBindingsDiagnosticsVars(ctx, input.RepoRoot)
	if err != nil {
		return MissingBindingsResult{}, err
	}

	results := make([]MissingBindingsOrbitResult, 0, len(targets))
	for _, member := range targets {
		record, err := LoadInstallRecord(input.RepoRoot, member.OrbitID)
		if err != nil {
			return MissingBindingsResult{}, fmt.Errorf("load install record for %q: %w", member.OrbitID, err)
		}
		if record.Variables == nil {
			results = append(results, MissingBindingsOrbitResult{
				OrbitID:         member.OrbitID,
				SnapshotMissing: true,
				Variables:       []MissingBindingsVariableResult{},
			})
			continue
		}

		result := MissingBindingsOrbitResult{
			OrbitID:       member.OrbitID,
			DeclaredCount: len(record.Variables.Declarations),
			Variables:     make([]MissingBindingsVariableResult, 0, len(record.Variables.Declarations)),
		}

		observedRuntimeUnresolved := stringSet(record.Variables.ObservedRuntimeUnresolved)
		for _, name := range sortedDeclarationNames(record.Variables.Declarations) {
			declaration := record.Variables.Declarations[name]
			namespace := record.Variables.Namespaces[name]
			hasValue := hasNamespaceAwareBinding(varsFile, namespace, name)
			observed := observedRuntimeUnresolved[name]
			missing := !hasValue && (declaration.Required || observed)
			if missing {
				result.MissingCount++
			}
			result.Variables = append(result.Variables, MissingBindingsVariableResult{
				Name:                      name,
				Namespace:                 namespace,
				Description:               declaration.Description,
				Required:                  declaration.Required,
				HasValue:                  hasValue,
				ObservedRuntimeUnresolved: observed,
				Missing:                   missing,
			})
		}

		results = append(results, result)
	}

	return MissingBindingsResult{
		HarnessID: runtimeFile.Harness.ID,
		Orbits:    results,
	}, nil
}

// ScanRuntimeBindings scans current runtime markdown plus the current orbit AGENTS block for unresolved placeholders.
func ScanRuntimeBindings(ctx context.Context, input ScanRuntimeBindingsInput) (ScanRuntimeBindingsResult, error) {
	runtimeFile, targets, err := loadInstallBackedBindingsTargets(input.RepoRoot, input.OrbitID, input.All)
	if err != nil {
		return ScanRuntimeBindingsResult{}, err
	}

	repoConfig, err := loadTemplateCandidateRepositoryConfig(ctx, input.RepoRoot)
	if err != nil {
		return ScanRuntimeBindingsResult{}, fmt.Errorf("load repository config: %w", err)
	}
	trackedFiles, err := gitpkg.TrackedFiles(ctx, input.RepoRoot)
	if err != nil {
		return ScanRuntimeBindingsResult{}, fmt.Errorf("load tracked files: %w", err)
	}
	candidatePaths, err := runtimeBindingsCandidatePaths(ctx, input.RepoRoot, trackedFiles)
	if err != nil {
		return ScanRuntimeBindingsResult{}, fmt.Errorf("load runtime candidate paths: %w", err)
	}

	results := make([]ScanRuntimeBindingsOrbitResult, 0, len(targets))
	wroteInstall := false
	for _, member := range targets {
		result, err := scanRuntimeBindingsForOrbit(ctx, input.RepoRoot, repoConfig, trackedFiles, candidatePaths, member.OrbitID)
		if err != nil {
			return ScanRuntimeBindingsResult{}, err
		}

		record, recordErr := LoadInstallRecord(input.RepoRoot, member.OrbitID)
		if recordErr == nil && record.Variables != nil && len(record.Variables.Namespaces) > 0 {
			result.VariableNamespaces = cloneStringMap(record.Variables.Namespaces)
		}

		if input.WriteInstall {
			if recordErr != nil {
				return ScanRuntimeBindingsResult{}, fmt.Errorf("load install record for %q: %w", member.OrbitID, recordErr)
			}
			if record.Variables == nil {
				record.Variables = &orbittemplate.InstallVariablesSnapshot{
					Declarations:    map[string]bindings.VariableDeclaration{},
					ResolvedAtApply: map[string]bindings.VariableBinding{},
				}
			}
			record.Variables.ObservedRuntimeUnresolved = append([]string(nil), result.ObservedRuntimeUnresolved...)
			if _, err := WriteInstallRecord(input.RepoRoot, record); err != nil {
				return ScanRuntimeBindingsResult{}, fmt.Errorf("write install record for %q: %w", member.OrbitID, err)
			}
			result.WroteInstall = true
			wroteInstall = true
		}

		results = append(results, result)
	}

	return ScanRuntimeBindingsResult{
		HarnessID:    runtimeFile.Harness.ID,
		WroteInstall: wroteInstall,
		Orbits:       results,
	}, nil
}

func loadInstallBackedBindingsTargets(repoRoot string, orbitID string, all bool) (RuntimeFile, []RuntimeMember, error) {
	if err := validateBindingsTargetSelection(orbitID, all); err != nil {
		return RuntimeFile{}, nil, err
	}

	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return RuntimeFile{}, nil, fmt.Errorf("load harness runtime: %w", err)
	}

	targets := make([]RuntimeMember, 0, len(runtimeFile.Members))
	if all {
		for _, member := range runtimeFile.Members {
			if member.Source == MemberSourceInstallOrbit {
				targets = append(targets, member)
			}
		}
		return runtimeFile, targets, nil
	}

	for _, member := range runtimeFile.Members {
		if member.OrbitID != orbitID {
			continue
		}
		if member.Source != MemberSourceInstallOrbit {
			return RuntimeFile{}, nil, fmt.Errorf("orbit %q is not install-backed", orbitID)
		}
		return runtimeFile, []RuntimeMember{member}, nil
	}

	detached, err := hasDetachedInstallRecord(repoRoot, orbitID)
	if err != nil {
		return RuntimeFile{}, nil, err
	}
	if detached {
		return RuntimeFile{}, nil, fmt.Errorf("orbit %q is detached; reinstall it before using bindings commands", orbitID)
	}

	return RuntimeFile{}, nil, fmt.Errorf("orbit %q not found in harness runtime", orbitID)
}

func validateBindingsTargetSelection(orbitID string, all bool) error {
	trimmedOrbitID := strings.TrimSpace(orbitID)
	switch {
	case all && trimmedOrbitID != "":
		return fmt.Errorf("exactly one of --orbit or --all must be set")
	case !all && trimmedOrbitID == "":
		return fmt.Errorf("exactly one of --orbit or --all must be set")
	default:
		return nil
	}
}

func hasDetachedInstallRecord(repoRoot string, orbitID string) (bool, error) {
	record, err := LoadInstallRecord(repoRoot, orbitID)
	if err == nil {
		return orbittemplate.EffectiveInstallRecordStatus(record) == orbittemplate.InstallRecordStatusDetached, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("load install record for %q: %w", orbitID, err)
}

func loadOptionalBindingsDiagnosticsVars(ctx context.Context, repoRoot string) (bindings.VarsFile, error) {
	file, err := LoadVarsFileWorktreeOrHEAD(ctx, repoRoot)
	if err == nil {
		return file, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{
			SchemaVersion: 1,
			Variables:     map[string]bindings.VariableBinding{},
		}, nil
	}

	return bindings.VarsFile{}, fmt.Errorf("load harness vars: %w", err)
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		result[trimmed] = true
	}
	return result
}

func hasNamespaceAwareBinding(file bindings.VarsFile, namespace string, name string) bool {
	if strings.TrimSpace(namespace) != "" {
		scoped := bindings.ScopedVariablesForNamespace(file, namespace)
		if binding, ok := scoped[name]; ok && strings.TrimSpace(binding.Value) != "" {
			return true
		}
	}

	binding, ok := file.Variables[name]
	return ok && strings.TrimSpace(binding.Value) != ""
}

func sortedDeclarationNames(values map[string]bindings.VariableDeclaration) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func scanRuntimeBindingsForOrbit(
	ctx context.Context,
	repoRoot string,
	repoConfig orbitpkg.RepositoryConfig,
	trackedFiles []string,
	candidatePaths []string,
	orbitID string,
) (ScanRuntimeBindingsOrbitResult, error) {
	// Runtime placeholder scans need to classify the current worktree view, which can
	// include freshly installed but not-yet-tracked files. Use the candidate path set
	// for plan resolution, while still preserving the tracked/untracked bit for path
	// classification below.
	spec, plan, err := orbitpkg.LoadOrbitSpecAndProjectionPlan(ctx, repoRoot, repoConfig, orbitID, candidatePaths)
	if err != nil {
		return ScanRuntimeBindingsOrbitResult{}, fmt.Errorf("resolve orbit plan for %q: %w", orbitID, err)
	}

	trackedSet := make(map[string]struct{}, len(trackedFiles))
	for _, path := range trackedFiles {
		trackedSet[path] = struct{}{}
	}

	pathResults := make([]ScanRuntimeBindingsPathResult, 0)
	for _, path := range candidatePaths {
		if path == runtimeAgentsRepoPath || !strings.EqualFold(filepath.Ext(path), ".md") {
			continue
		}
		_, tracked := trackedSet[path]
		classification, err := orbitpkg.ClassifyOrbitPath(repoConfig, spec, plan, path, tracked)
		if err != nil {
			return ScanRuntimeBindingsOrbitResult{}, fmt.Errorf("classify runtime path %q for %q: %w", path, orbitID, err)
		}
		if !classification.Projection {
			continue
		}

		variables, err := scanRuntimeMarkdownFile(repoRoot, path)
		if err != nil {
			return ScanRuntimeBindingsOrbitResult{}, err
		}
		if len(variables) == 0 {
			continue
		}
		pathResults = append(pathResults, ScanRuntimeBindingsPathResult{
			Path:      path,
			Variables: variables,
		})
	}

	agentsVariables, err := scanRuntimeAgentsBlock(repoRoot, orbitID)
	if err != nil {
		return ScanRuntimeBindingsOrbitResult{}, err
	}
	if len(agentsVariables) > 0 {
		pathResults = append(pathResults, ScanRuntimeBindingsPathResult{
			Path:      runtimeAgentsRepoPath,
			Variables: agentsVariables,
		})
	}

	sort.Slice(pathResults, func(left, right int) bool {
		return pathResults[left].Path < pathResults[right].Path
	})

	observedSet := make(map[string]struct{})
	placeholderCount := 0
	for _, pathResult := range pathResults {
		placeholderCount += len(pathResult.Variables)
		for _, name := range pathResult.Variables {
			observedSet[name] = struct{}{}
		}
	}

	return ScanRuntimeBindingsOrbitResult{
		OrbitID:                   orbitID,
		PathCount:                 len(pathResults),
		PlaceholderCount:          placeholderCount,
		ObservedRuntimeUnresolved: sortedPlaceholderNames(observedSet),
		Paths:                     pathResults,
	}, nil
}

func scanRuntimeMarkdownFile(repoRoot string, repoPath string) ([]string, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	//nolint:gosec // The runtime markdown path is repo-relative and selected from tracked/status-derived candidate paths.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runtime markdown %s: %w", repoPath, err)
	}

	return scanPlaceholderVariables(repoPath, data), nil
}

func scanRuntimeAgentsBlock(repoRoot string, orbitID string) ([]string, error) {
	filename := filepath.Join(repoRoot, runtimeAgentsRepoPath)
	//nolint:gosec // The runtime AGENTS path is fixed under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}

	for _, segment := range document.Segments {
		if segment.Kind != orbittemplate.AgentsRuntimeSegmentBlock ||
			segment.OwnerKind != orbittemplate.OwnerKindOrbit ||
			segment.WorkflowID != orbitID {
			continue
		}
		return scanPlaceholderVariables(runtimeAgentsRepoPath, segment.Content), nil
	}

	return nil, nil
}

func scanPlaceholderVariables(path string, data []byte) []string {
	result := orbittemplate.ScanVariables([]orbittemplate.CandidateFile{
		{
			Path:    path,
			Content: data,
		},
	}, nil)

	return append([]string(nil), result.Referenced...)
}

func sortedPlaceholderNames(values map[string]struct{}) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runtimeBindingsCandidatePaths(ctx context.Context, repoRoot string, trackedFiles []string) ([]string, error) {
	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load worktree status: %w", err)
	}

	paths := make(map[string]struct{}, len(trackedFiles)+len(statusEntries))
	for _, path := range trackedFiles {
		paths[path] = struct{}{}
	}
	for _, entry := range statusEntries {
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		paths[entry.Path] = struct{}{}
	}

	return sortedPlaceholderNames(paths), nil
}
