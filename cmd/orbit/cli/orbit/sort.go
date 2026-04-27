package orbit

import "sort"

// SortDefinitions orders definitions by id and then source path for stable output.
func SortDefinitions(definitions []Definition) {
	sort.Slice(definitions, func(left, right int) bool {
		if definitions[left].ID == definitions[right].ID {
			return definitions[left].SourcePath < definitions[right].SourcePath
		}

		return definitions[left].ID < definitions[right].ID
	})
}
