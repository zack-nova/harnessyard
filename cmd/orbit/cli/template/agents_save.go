package orbittemplate

import (
	"sort"
)

func normalizeTemplateSaveFiles(files []CandidateFile) []CandidateFile {
	filtered := make([]CandidateFile, 0, len(files))
	for _, file := range files {
		if file.Path == sharedFilePathAgents && len(file.Content) == 0 {
			continue
		}
		filtered = append(filtered, file)
	}

	sort.Slice(filtered, func(left, right int) bool {
		return filtered[left].Path < filtered[right].Path
	})

	return filtered
}

func sortFileReplacementSummaries(values []FileReplacementSummary) {
	sort.Slice(values, func(left, right int) bool {
		return values[left].Path < values[right].Path
	})
}

func sortFileReplacementAmbiguities(values []FileReplacementAmbiguity) {
	sort.Slice(values, func(left, right int) bool {
		return values[left].Path < values[right].Path
	})
}
