package orbittemplate

import (
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var variableReferencePattern = regexp.MustCompile(`\$[A-Za-z_][A-Za-z0-9_]*`)

// CandidateFile is one file from the template candidate tree.
type CandidateFile struct {
	Path    string
	Content []byte
	Mode    string
}

// ScanResult captures the unique referenced variables and declaration mismatches.
type ScanResult struct {
	Referenced []string
	Undeclared []string
	Unused     []string
}

// ScanVariables scans text files for Package Template References plus legacy
// $var_name placeholders and compares them with the declared manifest variable set.
// Binary or invalid UTF-8 files are skipped.
func ScanVariables(files []CandidateFile, declared map[string]VariableSpec) ScanResult {
	referencedSet := make(map[string]struct{})
	declaredSet := make(map[string]struct{}, len(declared))

	for name := range declared {
		declaredSet[name] = struct{}{}
	}

	for _, file := range files {
		if isBinaryOrInvalidText(file.Content) {
			continue
		}

		content := string(file.Content)
		if isMarkdownTemplateFile(file.Path) {
			for _, match := range variableReferencePattern.FindAllString(content, -1) {
				referencedSet[match[1:]] = struct{}{}
			}
		}
		for _, name := range scanPackageTemplateReferenceNames(content) {
			referencedSet[name] = struct{}{}
		}
	}

	referenced := sortedNames(referencedSet)
	undeclared := make([]string, 0)
	for _, name := range referenced {
		if _, ok := declaredSet[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}

	unused := make([]string, 0)
	for _, name := range sortedNames(declaredSet) {
		if _, ok := referencedSet[name]; !ok {
			unused = append(unused, name)
		}
	}

	return ScanResult{
		Referenced: referenced,
		Undeclared: undeclared,
		Unused:     unused,
	}
}

func scanPackageTemplateReferenceNames(text string) []string {
	names := make(map[string]struct{})
	cursor := 0
	for {
		startOffset := strings.Index(text[cursor:], "{{")
		if startOffset < 0 {
			return sortedNames(names)
		}
		start := cursor + startOffset
		if start > 0 && text[start-1] == '$' {
			cursor = start + 2
			continue
		}
		endOffset := strings.Index(text[start+2:], "}}")
		if endOffset < 0 {
			return sortedNames(names)
		}
		end := start + 2 + endOffset
		ref, err := parsePackageTemplateReference(strings.TrimSpace(text[start+2 : end]))
		if err == nil && ref.Namespace == "vars" {
			names[ref.Name] = struct{}{}
		}
		cursor = end + 2
	}
}

func isMarkdownTemplateFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

func isBinaryOrInvalidText(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content)
}

func sortedNames(values map[string]struct{}) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
