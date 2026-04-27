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

// ScanVariables scans Markdown files for $var_name references and compares them
// with the declared manifest variable set. Non-Markdown, binary, or invalid UTF-8
// files are skipped.
func ScanVariables(files []CandidateFile, declared map[string]VariableSpec) ScanResult {
	referencedSet := make(map[string]struct{})
	declaredSet := make(map[string]struct{}, len(declared))

	for name := range declared {
		declaredSet[name] = struct{}{}
	}

	for _, file := range files {
		if !isMarkdownTemplateFile(file.Path) || isBinaryOrInvalidText(file.Content) {
			continue
		}

		for _, match := range variableReferencePattern.FindAllString(string(file.Content), -1) {
			referencedSet[match[1:]] = struct{}{}
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
