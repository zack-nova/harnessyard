package orbittemplate

import "fmt"

func extractRuntimeGuidanceBlock(document AgentsRuntimeDocument, orbitID string, containerLabel string) ([]byte, error) {
	return extractRuntimeGuidanceOwnerBlock(document, OwnerKindOrbit, orbitID, containerLabel)
}

func extractRuntimeGuidanceOwnerBlock(document AgentsRuntimeDocument, ownerKind OwnerKind, workflowID string, containerLabel string) ([]byte, error) {
	for _, segment := range document.Segments {
		if segment.Kind != AgentsRuntimeSegmentBlock {
			continue
		}
		if segment.OwnerKind != ownerKind || segment.WorkflowID != workflowID {
			continue
		}

		return append([]byte(nil), segment.Content...), nil
	}

	return nil, fmt.Errorf("%s does not contain %s block %q", containerLabel, blockOwnerLabel(ownerKind), workflowID)
}

func replaceOrAppendRuntimeGuidanceBlock(existing []byte, orbitID string, wrappedBlock []byte, containerLabel string) ([]byte, error) {
	return replaceOrAppendRuntimeGuidanceOwnerBlock(existing, OwnerKindOrbit, orbitID, wrappedBlock, containerLabel)
}

func replaceOrAppendRuntimeGuidanceOwnerBlock(
	existing []byte,
	ownerKind OwnerKind,
	workflowID string,
	wrappedBlock []byte,
	containerLabel string,
) ([]byte, error) {
	if _, err := ParseRuntimeAgentsDocument(existing); err != nil {
		return nil, fmt.Errorf("parse %s: %w", containerLabel, err)
	}

	lines := splitLinesPreserveNewline(existing)
	output := make([]byte, 0, len(existing)+len(wrappedBlock))
	replacing := false
	replaced := false

	for _, line := range lines {
		marker, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, err
		}

		if replacing {
			if isMarker && marker.Kind == "end" && marker.OwnerKind == ownerKind && marker.WorkflowID == workflowID {
				output = append(output, wrappedBlock...)
				replacing = false
				replaced = true
			}
			continue
		}

		if isMarker && marker.Kind == "begin" && marker.OwnerKind == ownerKind && marker.WorkflowID == workflowID {
			replacing = true
			continue
		}

		output = append(output, line...)
	}

	if replacing {
		return nil, fmt.Errorf("unterminated %s block for %q", blockOwnerLabel(ownerKind), workflowID)
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
	return RemoveRuntimeGuidanceOwnerBlockData(existing, OwnerKindOrbit, orbitID, containerLabel)
}

// RemoveRuntimeGuidanceOwnerBlockData removes one owner-scoped runtime guidance block from raw document data.
func RemoveRuntimeGuidanceOwnerBlockData(existing []byte, ownerKind OwnerKind, workflowID string, containerLabel string) ([]byte, bool, error) {
	if _, err := ParseRuntimeAgentsDocument(existing); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", containerLabel, err)
	}

	lines := splitLinesPreserveNewline(existing)
	output := make([]byte, 0, len(existing))
	removing := false
	removed := false

	for _, line := range lines {
		marker, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return nil, false, err
		}

		if removing {
			if isMarker && marker.Kind == "end" && marker.OwnerKind == ownerKind && marker.WorkflowID == workflowID {
				removing = false
				removed = true
			}
			continue
		}

		if isMarker && marker.Kind == "begin" && marker.OwnerKind == ownerKind && marker.WorkflowID == workflowID {
			removing = true
			continue
		}

		output = append(output, line...)
	}

	if removing {
		return nil, false, fmt.Errorf("unterminated %s block for %q", blockOwnerLabel(ownerKind), workflowID)
	}
	if !removed {
		return append([]byte(nil), existing...), false, nil
	}

	return output, true, nil
}
