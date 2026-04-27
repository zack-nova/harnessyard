package orbittemplate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

func resolveRuntimeInstallVariableNamespaces(
	repoRoot string,
	orbitID string,
	declared map[string]bindings.VariableDeclaration,
	runtimeInstallOrbitIDs []string,
) (map[string]string, error) {
	if len(declared) == 0 {
		return map[string]string{}, nil
	}

	merged, _, err := loadRuntimeInstallVariableDeclarations(repoRoot, runtimeInstallOrbitIDs)
	if err != nil {
		return nil, fmt.Errorf("load runtime install variable declarations: %w", err)
	}
	namespaces := make(map[string]string)
	for _, name := range sortedVariableDeclarationNames(declared) {
		current, ok := merged[name]
		if !ok {
			continue
		}
		if _, err := bindings.MergeVariableDeclaration(name, current, declared[name]); err != nil {
			namespaces[name] = orbitID
		}
	}
	if len(namespaces) == 0 {
		return map[string]string{}, nil
	}

	return namespaces, nil
}

func loadRuntimeInstallVariableDeclarations(
	repoRoot string,
	runtimeInstallOrbitIDs []string,
) (map[string]bindings.VariableDeclaration, map[string][]string, error) {
	if runtimeInstallOrbitIDs != nil {
		return loadRuntimeInstallVariableDeclarationsForOrbitIDs(repoRoot, runtimeInstallOrbitIDs)
	}

	installDir := filepath.Join(repoRoot, filepath.FromSlash(runtimeInstallRecordRelativeDir))
	entries, err := os.ReadDir(installDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bindings.VariableDeclaration{}, map[string][]string{}, nil
		}
		return nil, nil, fmt.Errorf("read runtime install records: %w", err)
	}

	merged := make(map[string]bindings.VariableDeclaration)
	contributors := make(map[string][]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		filename := filepath.Join(installDir, entry.Name())
		record, err := LoadInstallRecordFile(filename)
		if err != nil {
			return nil, nil, fmt.Errorf("load runtime install record %s: %w", mustRepoRelativePath(repoRoot, filename), err)
		}
		if err := mergeRuntimeInstallVariableDeclarations(repoRoot, filename, record, merged, contributors); err != nil {
			return nil, nil, err
		}
	}

	return merged, contributors, nil
}

func loadRuntimeInstallVariableDeclarationsForOrbitIDs(
	repoRoot string,
	orbitIDs []string,
) (map[string]bindings.VariableDeclaration, map[string][]string, error) {
	merged := make(map[string]bindings.VariableDeclaration)
	contributors := make(map[string][]string)
	for _, orbitID := range sortedUniqueOrbitIDs(orbitIDs) {
		filename, err := runtimeInstallRecordPath(repoRoot, orbitID)
		if err != nil {
			return nil, nil, fmt.Errorf("build runtime install record path for %q: %w", orbitID, err)
		}
		record, err := loadRuntimeInstallRecord(repoRoot, orbitID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, nil, fmt.Errorf("load runtime install record %s: %w", mustRepoRelativePath(repoRoot, filename), err)
		}
		if err := mergeRuntimeInstallVariableDeclarations(repoRoot, filename, record, merged, contributors); err != nil {
			return nil, nil, err
		}
	}

	return merged, contributors, nil
}

func mergeRuntimeInstallVariableDeclarations(
	repoRoot string,
	filename string,
	record InstallRecord,
	merged map[string]bindings.VariableDeclaration,
	contributors map[string][]string,
) error {
	if record.Variables == nil || len(record.Variables.Declarations) == 0 {
		return nil
	}

	declarations := make(map[string]bindings.VariableDeclaration, len(record.Variables.Declarations))
	for name, declaration := range record.Variables.Declarations {
		if strings.TrimSpace(record.Variables.Namespaces[name]) != "" {
			continue
		}
		declarations[name] = declaration
	}
	if len(declarations) == 0 {
		return nil
	}

	sourceLabel := installVariableCompatibilitySource(repoRoot, filename, record)
	if err := bindings.MergeDeclarations(merged, contributors, declarations, sourceLabel); err != nil {
		return fmt.Errorf("merge install record declarations from %s: %w", mustRepoRelativePath(repoRoot, filename), err)
	}
	return nil
}

func installVariableCompatibilitySource(repoRoot string, filename string, record InstallRecord) string {
	repoPath := mustRepoRelativePath(repoRoot, filename)
	sourceRef := strings.TrimSpace(record.Template.SourceRef)
	if sourceRef == "" {
		return repoPath
	}
	return fmt.Sprintf("%s (%s)", repoPath, sourceRef)
}

func sortedUniqueOrbitIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func sortedVariableDeclarationNames(values map[string]bindings.VariableDeclaration) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
