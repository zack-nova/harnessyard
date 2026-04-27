package orbittemplate

import "fmt"

func extractRuntimeGuidanceBlock(document AgentsRuntimeDocument, orbitID string, containerLabel string) ([]byte, error) {
	for _, segment := range document.Segments {
		if segment.Kind != AgentsRuntimeSegmentBlock {
			continue
		}
		if segment.OrbitID != orbitID {
			continue
		}

		return append([]byte(nil), segment.Content...), nil
	}

	return nil, fmt.Errorf("%s does not contain orbit block %q", containerLabel, orbitID)
}

func replaceOrAppendRuntimeGuidanceBlock(existing []byte, orbitID string, wrappedBlock []byte, containerLabel string) ([]byte, error) {
	if _, err := ParseRuntimeAgentsDocument(existing); err != nil {
		return nil, fmt.Errorf("parse %s: %w", containerLabel, err)
	}

	lines := splitLinesPreserveNewline(existing)
	output := make([]byte, 0, len(existing)+len(wrappedBlock))
	replacing := false
	replaced := false

	for _, line := range lines {
		markerKind, markerOrbitID, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, err
		}

		if replacing {
			if isMarker && markerKind == "end" && markerOrbitID == orbitID {
				output = append(output, wrappedBlock...)
				replacing = false
				replaced = true
			}
			continue
		}

		if isMarker && markerKind == "begin" && markerOrbitID == orbitID {
			replacing = true
			continue
		}

		output = append(output, line...)
	}

	if replacing {
		return nil, fmt.Errorf("unterminated orbit block for %q", orbitID)
	}
	if replaced {
		return output, nil
	}

	merged := append([]byte(nil), existing...)
	if len(merged) > 0 {
		if merged[len(merged)-1] != '\n' {
			merged = append(merged, '\n')
		}
		if len(merged) < 2 || merged[len(merged)-2] != '\n' {
			merged = append(merged, '\n')
		}
	}
	merged = append(merged, wrappedBlock...)

	return merged, nil
}

// RemoveRuntimeGuidanceBlockData removes one runtime guidance block by identity from raw document data.
func RemoveRuntimeGuidanceBlockData(existing []byte, orbitID string, containerLabel string) ([]byte, bool, error) {
	if _, err := ParseRuntimeAgentsDocument(existing); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", containerLabel, err)
	}

	lines := splitLinesPreserveNewline(existing)
	output := make([]byte, 0, len(existing))
	removing := false
	removed := false

	for _, line := range lines {
		markerKind, markerOrbitID, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, false, err
		}

		if removing {
			if isMarker && markerKind == "end" && markerOrbitID == orbitID {
				removing = false
				removed = true
			}
			continue
		}

		if isMarker && markerKind == "begin" && markerOrbitID == orbitID {
			removing = true
			continue
		}

		output = append(output, line...)
	}

	if removing {
		return nil, false, fmt.Errorf("unterminated orbit block for %q", orbitID)
	}
	if !removed {
		return append([]byte(nil), existing...), false, nil
	}

	return output, true, nil
}
