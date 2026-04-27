package orbittemplate

import "fmt"

// NormalizeRuntimeAgentsPayload strips runtime block markers and returns the ordered payload-only content.
func NormalizeRuntimeAgentsPayload(data []byte) ([]byte, error) {
	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parse runtime AGENTS.md: %w", err)
	}

	var normalized []byte
	for _, segment := range document.Segments {
		normalized = append(normalized, segment.Content...)
	}

	return normalized, nil
}
