package contractutil

import (
	"fmt"
	"regexp"
	"sort"
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateVariableName enforces the frozen Phase 2 variable-name contract.
func ValidateVariableName(name string) error {
	if !variableNamePattern.MatchString(name) {
		return fmt.Errorf("must match %q", variableNamePattern.String())
	}

	return nil
}

// SortedKeys returns stable lexical ordering for string-keyed maps.
func SortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
