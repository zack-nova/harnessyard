package harness

import (
	"fmt"
	"sort"
)

// TemplateMemberOwnership describes one member's payload ownership within a harness template.
type TemplateMemberOwnership struct {
	OrbitID        string
	ExclusivePaths []string
	SharedPaths    []string
}

// AnalyzeTemplateMemberOwnership validates member snapshot consistency and resolves
// the target member's exclusive/shared payload paths from the saved harness template.
func AnalyzeTemplateMemberOwnership(
	source LocalTemplateInstallSource,
	orbitID string,
) (TemplateMemberOwnership, error) {
	validation, err := validateTemplateMemberSnapshots(source)
	if err != nil {
		return TemplateMemberOwnership{}, err
	}

	memberSet := make(map[string]struct{}, len(source.Manifest.Members))
	for _, member := range source.Manifest.Members {
		memberSet[member.OrbitID] = struct{}{}
		if _, ok := source.MemberSnapshots[member.OrbitID]; !ok {
			return TemplateMemberOwnership{}, fmt.Errorf("template member snapshot for %q is required", member.OrbitID)
		}
	}

	for memberID := range source.MemberSnapshots {
		if _, ok := memberSet[memberID]; !ok {
			return TemplateMemberOwnership{}, fmt.Errorf("template member snapshot for %q is not declared in template members", memberID)
		}
	}
	if _, ok := memberSet[orbitID]; !ok {
		return TemplateMemberOwnership{}, fmt.Errorf("template member %q not found", orbitID)
	}

	targetSnapshot := source.MemberSnapshots[orbitID]
	result := TemplateMemberOwnership{
		OrbitID:        orbitID,
		ExclusivePaths: []string{},
		SharedPaths:    []string{},
	}
	for _, path := range targetSnapshot.Snapshot.ExportedPaths {
		if path == rootAgentsPath {
			continue
		}

		contributors := validation.pathContributors[path]
		if len(contributors) == 1 {
			result.ExclusivePaths = append(result.ExclusivePaths, path)
			continue
		}
		result.SharedPaths = append(result.SharedPaths, path)
	}

	sort.Strings(result.ExclusivePaths)
	sort.Strings(result.SharedPaths)

	return result, nil
}
