package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

const (
	RuntimeBindingsDoctorStatusOK    = "ok"
	RuntimeBindingsDoctorStatusWarn  = "warn"
	RuntimeBindingsDoctorStatusError = "error"
)

// RuntimeBindingsDoctorInput describes the runtime context for P0 Runtime Bindings diagnostics.
type RuntimeBindingsDoctorInput struct {
	RepoRoot  string
	LookupEnv func(string) (string, bool)
	ReadFile  func(string) ([]byte, error)
}

// RuntimeBindingDiagnostic reports one P0 Runtime Bindings finding.
type RuntimeBindingDiagnostic struct {
	Code     string `json:"code"`
	Variable string `json:"variable,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// RuntimeBindingsDoctorResult is the stable result for hyard vars doctor.
type RuntimeBindingsDoctorResult struct {
	Path     string                     `json:"path"`
	Status   string                     `json:"status"`
	Errors   []RuntimeBindingDiagnostic `json:"errors"`
	Warnings []RuntimeBindingDiagnostic `json:"warnings"`
}

type runtimeBindingDeclaration struct {
	Name        string
	OrbitID     string
	Declaration bindings.VariableDeclaration
}

// DoctorRuntimeBindings reports P0 Runtime Bindings errors and warnings for installed packages.
func DoctorRuntimeBindings(ctx context.Context, input RuntimeBindingsDoctorInput) (RuntimeBindingsDoctorResult, error) {
	_ = ctx

	result := RuntimeBindingsDoctorResult{
		Path:   VarsRepoPath(),
		Status: RuntimeBindingsDoctorStatusOK,
	}

	varsFile, err := LoadVarsFile(input.RepoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			varsFile = bindings.VarsFile{
				SchemaVersion: bindings.VarsSchemaVersion,
				Variables:     map[string]bindings.VariableBinding{},
			}
		} else {
			result.Errors = append(result.Errors, RuntimeBindingDiagnostic{
				Code:    "invalid_schema",
				Source:  VarsRepoPath(),
				Message: fmt.Sprintf("invalid Runtime Bindings schema: %v", err),
			})
			return finalizeRuntimeBindingsDoctorResult(result), nil
		}
	}

	declarations, err := loadRuntimeBindingDeclarations(input.RepoRoot)
	if err != nil {
		return RuntimeBindingsDoctorResult{}, err
	}
	declaredByName, declaredByScope := runtimeBindingDeclarationIndexes(declarations)

	for _, item := range declarations {
		resolution, err := bindings.ResolveRuntimeBinding(bindings.RuntimeBindingInput{
			Name:          item.Name,
			SelectedScope: item.OrbitID,
			Declaration:   item.Declaration,
			VarsFile:      varsFile,
			FileRoot:      input.RepoRoot,
			LookupEnv:     input.LookupEnv,
			ReadFile:      input.ReadFile,
		})
		if err != nil {
			result.Errors = append(result.Errors, RuntimeBindingDiagnostic{
				Code:     "sensitive_value_source",
				Variable: item.Name,
				Scope:    selectedDiagnosticScope(resolution, item.OrbitID),
				Message:  err.Error(),
			})
			continue
		}

		switch {
		case resolution.Resolved:
			continue
		case strings.HasPrefix(resolution.ValueSource, "env:"):
			result.Errors = append(result.Errors, RuntimeBindingDiagnostic{
				Code:     "unset_env",
				Variable: item.Name,
				Scope:    selectedDiagnosticScope(resolution, item.OrbitID),
				Source:   resolution.ValueSource,
				Message:  fmt.Sprintf("%s references unset environment variable %s", item.Name, strings.TrimPrefix(resolution.ValueSource, "env:")),
			})
		case strings.HasPrefix(resolution.ValueSource, "file:"):
			result.Warnings = append(result.Warnings, RuntimeBindingDiagnostic{
				Code:     "missing_file",
				Variable: item.Name,
				Scope:    selectedDiagnosticScope(resolution, item.OrbitID),
				Source:   resolution.ValueSource,
				Message:  fmt.Sprintf("%s references missing file %s", item.Name, strings.TrimPrefix(resolution.ValueSource, "file:")),
			})
		case item.Declaration.Required:
			result.Errors = append(result.Errors, RuntimeBindingDiagnostic{
				Code:     "missing_required",
				Variable: item.Name,
				Scope:    item.OrbitID,
				Message:  fmt.Sprintf("%s is required but has no Runtime Binding or declaration default", item.Name),
			})
		}
	}

	appendUndeclaredRuntimeBindingWarnings(&result, varsFile, declaredByName, declaredByScope)

	return finalizeRuntimeBindingsDoctorResult(result), nil
}

func loadRuntimeBindingDeclarations(repoRoot string) ([]runtimeBindingDeclaration, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load harness runtime: %w", err)
	}

	declarations := make([]runtimeBindingDeclaration, 0)
	for _, member := range runtimeFile.Members {
		if member.Source != MemberSourceInstallOrbit {
			continue
		}
		record, err := LoadInstallRecord(repoRoot, member.OrbitID)
		if err != nil {
			return nil, fmt.Errorf("load install record for %q: %w", member.OrbitID, err)
		}
		if record.Variables == nil {
			continue
		}
		for _, name := range sortedRuntimeBindingDeclarationNames(record.Variables.Declarations) {
			declarations = append(declarations, runtimeBindingDeclaration{
				Name:        name,
				OrbitID:     member.OrbitID,
				Declaration: record.Variables.Declarations[name],
			})
		}
	}

	return declarations, nil
}

func runtimeBindingDeclarationIndexes(declarations []runtimeBindingDeclaration) (map[string]bool, map[string]map[string]bool) {
	byName := make(map[string]bool)
	byScope := make(map[string]map[string]bool)
	for _, item := range declarations {
		byName[item.Name] = true
		if byScope[item.OrbitID] == nil {
			byScope[item.OrbitID] = map[string]bool{}
		}
		byScope[item.OrbitID][item.Name] = true
	}
	return byName, byScope
}

func appendUndeclaredRuntimeBindingWarnings(
	result *RuntimeBindingsDoctorResult,
	varsFile bindings.VarsFile,
	declaredByName map[string]bool,
	declaredByScope map[string]map[string]bool,
) {
	for _, name := range sortedRuntimeBindingNames(varsFile.Variables) {
		if declaredByName[name] {
			continue
		}
		result.Warnings = append(result.Warnings, RuntimeBindingDiagnostic{
			Code:     "undeclared_binding",
			Variable: name,
			Scope:    "global",
			Source:   VarsRepoPath(),
			Message:  fmt.Sprintf("%s has a Runtime Binding but no installed package declares it", name),
		})
	}
	for _, scope := range sortedRuntimeBindingScopes(varsFile.ScopedVariables) {
		for _, name := range sortedRuntimeBindingNames(varsFile.ScopedVariables[scope].Variables) {
			if declaredByScope[scope] != nil && declaredByScope[scope][name] {
				continue
			}
			result.Warnings = append(result.Warnings, RuntimeBindingDiagnostic{
				Code:     "undeclared_binding",
				Variable: name,
				Scope:    scope,
				Source:   VarsRepoPath(),
				Message:  fmt.Sprintf("%s has a scoped Runtime Binding for %s but no installed package declares it there", name, scope),
			})
		}
	}
}

func finalizeRuntimeBindingsDoctorResult(result RuntimeBindingsDoctorResult) RuntimeBindingsDoctorResult {
	sortRuntimeBindingDiagnostics(result.Errors)
	sortRuntimeBindingDiagnostics(result.Warnings)
	switch {
	case len(result.Errors) > 0:
		result.Status = RuntimeBindingsDoctorStatusError
	case len(result.Warnings) > 0:
		result.Status = RuntimeBindingsDoctorStatusWarn
	default:
		result.Status = RuntimeBindingsDoctorStatusOK
	}
	if result.Errors == nil {
		result.Errors = []RuntimeBindingDiagnostic{}
	}
	if result.Warnings == nil {
		result.Warnings = []RuntimeBindingDiagnostic{}
	}
	return result
}

func selectedDiagnosticScope(resolution bindings.RuntimeBindingResolution, fallback string) string {
	if strings.TrimSpace(resolution.SelectedScope) != "" {
		return resolution.SelectedScope
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "global"
}

func sortRuntimeBindingDiagnostics(diagnostics []RuntimeBindingDiagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		left := diagnostics[i]
		right := diagnostics[j]
		return left.Code+left.Variable+left.Scope+left.Source < right.Code+right.Variable+right.Scope+right.Source
	})
}

func sortedRuntimeBindingDeclarationNames(values map[string]bindings.VariableDeclaration) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedRuntimeBindingNames(values map[string]bindings.VariableBinding) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedRuntimeBindingScopes(values map[string]bindings.ScopedVariableBindings) []string {
	scopes := make([]string, 0, len(values))
	for scope := range values {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes
}
