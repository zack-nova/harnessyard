package orbittemplate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

// ReplacementSummary records how one literal was replaced in a text file.
type ReplacementSummary struct {
	Variable string
	Literal  string
	Count    int
}

// ReplacementAmbiguity records a literal that maps to multiple variables.
type ReplacementAmbiguity struct {
	Literal   string
	Variables []string
}

// ReplacementResult contains the replaced content plus summary metadata.
type ReplacementResult struct {
	Content       []byte
	SkippedBinary bool
	Replacements  []ReplacementSummary
	Ambiguities   []ReplacementAmbiguity
}

type replacementEntry struct {
	Variable string
	Literal  string
}

// ReplaceRuntimeValues applies runtime-to-template literal replacement for one candidate file.
func ReplaceRuntimeValues(file CandidateFile, variables map[string]bindings.VariableBinding) (ReplacementResult, error) {
	result := ReplacementResult{
		Content: append([]byte(nil), file.Content...),
	}

	if isBinaryOrInvalidText(file.Content) {
		result.SkippedBinary = true
		return result, nil
	}

	if !isMarkdownTemplateFile(file.Path) {
		return result, nil
	}

	entries, ambiguities, err := buildReplacementPlan(variables)
	if err != nil {
		return ReplacementResult{}, err
	}
	result.Ambiguities = ambiguities
	if len(ambiguities) > 0 {
		return result, nil
	}

	content := string(file.Content)
	summaries := make([]ReplacementSummary, 0, len(entries))
	for _, entry := range entries {
		count := strings.Count(content, entry.Literal)
		if count == 0 {
			continue
		}

		content = strings.ReplaceAll(content, entry.Literal, "$"+entry.Variable)
		summaries = append(summaries, ReplacementSummary{
			Variable: entry.Variable,
			Literal:  entry.Literal,
			Count:    count,
		})
	}

	result.Content = []byte(content)
	result.Replacements = summaries

	return result, nil
}

func buildReplacementPlan(variables map[string]bindings.VariableBinding) ([]replacementEntry, []ReplacementAmbiguity, error) {
	grouped := make(map[string][]string)
	for name, binding := range variables {
		if binding.Value == "" {
			return nil, nil, fmt.Errorf("replacement literal for %q must not be empty", name)
		}
		grouped[binding.Value] = append(grouped[binding.Value], name)
	}

	literals := make([]string, 0, len(grouped))
	for literal := range grouped {
		literals = append(literals, literal)
	}
	sort.Slice(literals, func(left, right int) bool {
		if len(literals[left]) == len(literals[right]) {
			return literals[left] < literals[right]
		}
		return len(literals[left]) > len(literals[right])
	})

	entries := make([]replacementEntry, 0, len(grouped))
	ambiguities := make([]ReplacementAmbiguity, 0)
	for _, literal := range literals {
		names := append([]string(nil), grouped[literal]...)
		sort.Strings(names)
		if len(names) > 1 {
			ambiguities = append(ambiguities, ReplacementAmbiguity{
				Literal:   literal,
				Variables: names,
			})
			continue
		}
		entries = append(entries, replacementEntry{
			Variable: names[0],
			Literal:  literal,
		})
	}

	return entries, ambiguities, nil
}
