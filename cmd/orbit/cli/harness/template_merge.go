package harness

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// TemplateMergeResult captures the stable merged harness template candidate set.
type TemplateMergeResult struct {
	Members   []TemplateMember
	Files     []orbittemplate.CandidateFile
	Variables map[string]TemplateVariableSpec
}

// TemplatePathConflictError reports one conflicting output path plus the contributing members.
type TemplatePathConflictError struct {
	Path    string
	Members []string
}

func (err *TemplatePathConflictError) Error() string {
	return fmt.Sprintf("path conflict for %q (members: %s)", err.Path, strings.Join(err.Members, ", "))
}

// TemplateVariableConflictError reports one conflicting template variable plus the contributing members.
type TemplateVariableConflictError struct {
	Name    string
	Members []string
}

func (err *TemplateVariableConflictError) Error() string {
	return fmt.Sprintf("variable conflict for %q (members: %s)", err.Name, strings.Join(err.Members, ", "))
}

// FilePaths returns the stable merged file path list.
func (result TemplateMergeResult) FilePaths() []string {
	paths := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// MergeTemplateMemberCandidates merges multiple member candidates into one harness template candidate set.
func MergeTemplateMemberCandidates(candidates []TemplateMemberCandidate) (TemplateMergeResult, error) {
	members := make([]TemplateMember, 0, len(candidates))
	filesByPath := make(map[string]orbittemplate.CandidateFile)
	fileContributors := make(map[string][]string)
	variables := make(map[string]TemplateVariableSpec)
	variableContributors := make(map[string][]string)

	for _, candidate := range candidates {
		members = append(members, TemplateMember{OrbitID: candidate.OrbitID})

		for _, file := range candidate.Files {
			existing, ok := filesByPath[file.Path]
			if !ok {
				filesByPath[file.Path] = cloneCandidateFile(file)
				fileContributors[file.Path] = appendContributor(fileContributors[file.Path], candidate.OrbitID)
				continue
			}
			if !candidateFilesEqual(existing, file) {
				return TemplateMergeResult{}, &TemplatePathConflictError{
					Path:    file.Path,
					Members: appendContributor(fileContributors[file.Path], candidate.OrbitID),
				}
			}
			fileContributors[file.Path] = appendContributor(fileContributors[file.Path], candidate.OrbitID)
		}

		for name, next := range candidate.Variables {
			current, ok := variables[name]
			if !ok {
				variables[name] = next
				variableContributors[name] = appendContributor(variableContributors[name], candidate.OrbitID)
				continue
			}

			merged, err := mergeTemplateVariableSpec(name, current, next)
			if err != nil {
				return TemplateMergeResult{}, &TemplateVariableConflictError{
					Name:    name,
					Members: appendContributor(variableContributors[name], candidate.OrbitID),
				}
			}
			variables[name] = merged
			variableContributors[name] = appendContributor(variableContributors[name], candidate.OrbitID)
		}
	}

	sort.Slice(members, func(left, right int) bool {
		return members[left].OrbitID < members[right].OrbitID
	})

	sortedPaths := make([]string, 0, len(filesByPath))
	for path := range filesByPath {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	files := make([]orbittemplate.CandidateFile, 0, len(sortedPaths))
	for _, path := range sortedPaths {
		files = append(files, filesByPath[path])
	}

	return TemplateMergeResult{
		Members:   members,
		Files:     files,
		Variables: variables,
	}, nil
}

func mergeTemplateVariableSpec(name string, current TemplateVariableSpec, next TemplateVariableSpec) (TemplateVariableSpec, error) {
	switch {
	case current.Description == next.Description:
	case current.Description == "":
		current.Description = next.Description
	case next.Description == "":
	default:
		return TemplateVariableSpec{}, fmt.Errorf("variable conflict for %q", name)
	}

	current.Required = current.Required || next.Required

	return current, nil
}

func appendContributor(existing []string, contributor string) []string {
	if contributor == "" {
		return sortedUniqueStrings(existing)
	}

	merged := append(append([]string(nil), existing...), contributor)
	return sortedUniqueStrings(merged)
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	sorted := append([]string(nil), values...)
	sort.Strings(sorted)

	result := sorted[:0]
	var previous string
	for index, value := range sorted {
		if index == 0 || value != previous {
			result = append(result, value)
			previous = value
		}
	}

	return append([]string(nil), result...)
}

func candidateFilesEqual(left orbittemplate.CandidateFile, right orbittemplate.CandidateFile) bool {
	return left.Path == right.Path &&
		left.Mode == right.Mode &&
		bytes.Equal(left.Content, right.Content)
}

func cloneCandidateFile(file orbittemplate.CandidateFile) orbittemplate.CandidateFile {
	return orbittemplate.CandidateFile{
		Path:    file.Path,
		Content: append([]byte(nil), file.Content...),
		Mode:    file.Mode,
	}
}
