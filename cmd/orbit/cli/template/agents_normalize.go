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

// StripRuntimeAgentsMarkerLinesData removes supported root guidance marker lines
// after validating the document, preserving every non-marker line in order.
func StripRuntimeAgentsMarkerLinesData(data []byte) ([]byte, int, error) {
	document, err := ParseRuntimeAgentsDocument(data)
	if err != nil {
		return nil, 0, err
	}

	blockCount := 0
	for _, segment := range document.Segments {
		if segment.Kind == AgentsRuntimeSegmentBlock {
			blockCount++
		}
	}
	if blockCount == 0 {
		return append([]byte(nil), data...), 0, nil
	}

	lines := splitLinesPreserveNewline(data)
	stripped := make([]byte, 0, len(data))
	for _, line := range lines {
		_, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, 0, err
		}
		if isMarker {
			continue
		}
		stripped = append(stripped, line...)
	}

	return stripped, blockCount, nil
}
