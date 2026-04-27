package harness

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func analyzeTemplateInstallVariableConflicts(
	repoRoot string,
	runtimeFile RuntimeFile,
	variables map[string]TemplateVariableSpec,
	excludedBundleHarnessID string,
) ([]orbittemplate.ApplyConflict, error) {
	if len(variables) == 0 {
		return nil, nil
	}

	merged, contributors, err := loadActiveRuntimeInstallUnitVariableDeclarations(repoRoot, runtimeFile, excludedBundleHarnessID)
	if err != nil {
		return nil, err
	}

	conflicts := make([]orbittemplate.ApplyConflict, 0)
	for _, name := range sortedTemplateVariableNames(variables) {
		current, ok := merged[name]
		if !ok {
			continue
		}

		next := bindings.VariableDeclaration{
			Description: variables[name].Description,
			Required:    variables[name].Required,
		}
		candidateMerged := cloneVariableDeclarations(merged)
		candidateContributors := cloneVariableContributors(contributors)
		err := bindings.MergeDeclarations(
			candidateMerged,
			candidateContributors,
			map[string]bindings.VariableDeclaration{name: next},
			"",
		)
		if err == nil {
			continue
		}

		message := err.Error()
		var conflictErr *bindings.DeclarationConflictError
		if errors.As(err, &conflictErr) && len(conflictErr.Sources) > 0 {
			message = fmt.Sprintf(`variable conflict for %q (sources: %s)`, name, strings.Join(conflictErr.Sources, ", "))
		} else if _, mergeErr := bindings.MergeVariableDeclaration(name, current, next); mergeErr != nil {
			message = mergeErr.Error()
		}
		conflicts = append(conflicts, orbittemplate.ApplyConflict{
			Path:    VarsRepoPath(),
			Message: message,
		})
	}

	return conflicts, nil
}

func loadActiveRuntimeInstallUnitVariableDeclarations(
	repoRoot string,
	runtimeFile RuntimeFile,
	excludedBundleHarnessID string,
) (map[string]bindings.VariableDeclaration, map[string][]string, error) {
	merged := make(map[string]bindings.VariableDeclaration)
	contributors := make(map[string][]string)

	validBundleRecords, _, _, err := scanBundleRecords(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("scan bundle records: %w", err)
	}

	seenBundles := make(map[string]struct{})
	for _, member := range runtimeFile.Members {
		switch member.Source {
		case MemberSourceInstallOrbit:
			record, err := LoadInstallRecord(repoRoot, member.OrbitID)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, nil, fmt.Errorf("load install record for %q: %w", member.OrbitID, err)
			}
			if err := mergeInstallUnitVariableDeclarations(
				merged,
				contributors,
				record.Variables,
				installUnitVariableCompatibilitySource(mustInstallRecordRepoPath(member.OrbitID), record.Template.SourceRef),
			); err != nil {
				return nil, nil, err
			}
		case MemberSourceInstallBundle:
			record, ok := findBundleRecordForOwnedMember(validBundleRecords, member)
			if !ok {
				continue
			}
			if record.Record.HarnessID == excludedBundleHarnessID {
				continue
			}
			if _, seen := seenBundles[record.Record.HarnessID]; seen {
				continue
			}
			seenBundles[record.Record.HarnessID] = struct{}{}
			repoPath, err := BundleRecordRepoPath(record.Record.HarnessID)
			if err != nil {
				return nil, nil, fmt.Errorf("build bundle record path for %q: %w", record.Record.HarnessID, err)
			}
			if err := mergeInstallUnitVariableDeclarations(
				merged,
				contributors,
				record.Record.Variables,
				installUnitVariableCompatibilitySource(repoPath, record.Record.Template.SourceRef),
			); err != nil {
				return nil, nil, err
			}
		}
	}

	return merged, contributors, nil
}

func mergeInstallUnitVariableDeclarations(
	merged map[string]bindings.VariableDeclaration,
	contributors map[string][]string,
	snapshot *orbittemplate.InstallVariablesSnapshot,
	source string,
) error {
	if snapshot == nil || len(snapshot.Declarations) == 0 {
		return nil
	}

	declarations := make(map[string]bindings.VariableDeclaration, len(snapshot.Declarations))
	for name, declaration := range snapshot.Declarations {
		if strings.TrimSpace(snapshot.Namespaces[name]) != "" {
			continue
		}
		declarations[name] = declaration
	}
	if len(declarations) == 0 {
		return nil
	}

	if err := bindings.MergeDeclarations(merged, contributors, declarations, source); err != nil {
		return fmt.Errorf("merge install-unit declarations from %s: %w", source, err)
	}
	return nil
}

func installUnitVariableCompatibilitySource(repoPath string, sourceRef string) string {
	if strings.TrimSpace(sourceRef) == "" {
		return repoPath
	}
	return fmt.Sprintf("%s (%s)", repoPath, sourceRef)
}

func mustInstallRecordRepoPath(orbitID string) string {
	repoPath, err := InstallRecordRepoPath(orbitID)
	if err != nil {
		panic(err)
	}
	return repoPath
}

func cloneVariableDeclarations(input map[string]bindings.VariableDeclaration) map[string]bindings.VariableDeclaration {
	cloned := make(map[string]bindings.VariableDeclaration, len(input))
	for name, declaration := range input {
		cloned[name] = declaration
	}
	return cloned
}

func cloneVariableContributors(input map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(input))
	for name, values := range input {
		cloned[name] = append([]string(nil), values...)
	}
	return cloned
}

func sortedTemplateVariableNames(values map[string]TemplateVariableSpec) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
