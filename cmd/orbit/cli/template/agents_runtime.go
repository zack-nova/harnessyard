package orbittemplate

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// AgentsRuntimeSegmentKind distinguishes unmarked prose from one orbit-owned block.
type AgentsRuntimeSegmentKind string

const (
	AgentsRuntimeSegmentUnmarked AgentsRuntimeSegmentKind = "unmarked"
	AgentsRuntimeSegmentBlock    AgentsRuntimeSegmentKind = "block"
)

// RuntimeGuidanceOwnerKind names the package family that owns one root guidance block.
type RuntimeGuidanceOwnerKind string

const (
	RuntimeGuidanceOwnerKindOrbit   RuntimeGuidanceOwnerKind = "orbit"
	RuntimeGuidanceOwnerKindHarness RuntimeGuidanceOwnerKind = "harness"
)

var runtimeAgentsMarkerPattern = regexp.MustCompile(`^<!--\s*(orbit|harness):(begin|end)\s+workflow="([^"]+)"\s*-->$`)

// AgentsRuntimeDocument is the ordered parsed representation of one runtime AGENTS.md file.
type AgentsRuntimeDocument struct {
	Segments []AgentsRuntimeSegment
}

// AgentsRuntimeSegment is either plain unmarked prose or one orbit-owned block body.
type AgentsRuntimeSegment struct {
	Kind       AgentsRuntimeSegmentKind
	OwnerKind  RuntimeGuidanceOwnerKind
	WorkflowID string
	OrbitID    string
	Content    []byte
}

// ParseRuntimeAgentsDocument parses one runtime AGENTS.md file and validates the frozen V0.2 marker contract.
func ParseRuntimeAgentsDocument(data []byte) (AgentsRuntimeDocument, error) {
	lines := splitLinesPreserveNewline(data)
	document := AgentsRuntimeDocument{
		Segments: make([]AgentsRuntimeSegment, 0, len(lines)),
	}

	var unmarked bytes.Buffer
	var currentBlock bytes.Buffer
	currentOwnerKind := RuntimeGuidanceOwnerKind("")
	currentWorkflowID := ""
	seenBlocks := make(map[string]struct{})

	flushUnmarked := func() {
		if unmarked.Len() == 0 {
			return
		}
		document.Segments = append(document.Segments, AgentsRuntimeSegment{
			Kind:    AgentsRuntimeSegmentUnmarked,
			Content: append([]byte(nil), unmarked.Bytes()...),
		})
		unmarked.Reset()
	}

	for _, line := range lines {
		ownerKind, markerKind, workflowID, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return AgentsRuntimeDocument{}, err
		}
		if isMarker {
			switch markerKind {
			case "begin":
				if currentWorkflowID != "" {
					return AgentsRuntimeDocument{}, fmt.Errorf("nested %s block for %q", ownerKind, workflowID)
				}
				flushUnmarked()
				currentOwnerKind = ownerKind
				currentWorkflowID = workflowID
				currentBlock.Reset()
			case "end":
				if currentWorkflowID == "" {
					return AgentsRuntimeDocument{}, fmt.Errorf("unexpected %s end marker for %q", ownerKind, workflowID)
				}
				if currentOwnerKind != ownerKind || currentWorkflowID != workflowID {
					return AgentsRuntimeDocument{}, fmt.Errorf(
						"end workflow %q does not match begin workflow %q",
						workflowID,
						currentWorkflowID,
					)
				}
				blockKey := runtimeGuidanceBlockKey(ownerKind, workflowID)
				if _, exists := seenBlocks[blockKey]; exists {
					return AgentsRuntimeDocument{}, fmt.Errorf("duplicate %s block for %q", ownerKind, workflowID)
				}
				seenBlocks[blockKey] = struct{}{}
				document.Segments = append(document.Segments, AgentsRuntimeSegment{
					Kind:       AgentsRuntimeSegmentBlock,
					OwnerKind:  ownerKind,
					WorkflowID: workflowID,
					OrbitID:    orbitIDForRuntimeGuidanceSegment(ownerKind, workflowID),
					Content:    trimRuntimeAgentsMarkerPadding(currentBlock.Bytes()),
				})
				currentOwnerKind = ""
				currentWorkflowID = ""
				currentBlock.Reset()
			default:
				return AgentsRuntimeDocument{}, fmt.Errorf("unsupported %s marker kind %q", ownerKind, markerKind)
			}

			continue
		}

		if currentWorkflowID != "" {
			_, _ = currentBlock.Write(line)
			continue
		}
		_, _ = unmarked.Write(line)
	}

	if currentWorkflowID != "" {
		return AgentsRuntimeDocument{}, fmt.Errorf("unterminated %s block for %q", currentOwnerKind, currentWorkflowID)
	}

	flushUnmarked()

	return document, nil
}

// WrapRuntimeAgentsBlock renders one runtime AGENTS marker block around the provided payload.
func WrapRuntimeAgentsBlock(orbitID string, payload []byte) ([]byte, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return nil, fmt.Errorf("validate orbit id %q: %w", orbitID, err)
	}

	var rendered bytes.Buffer
	rendered.WriteString(beginRuntimeAgentsMarker(orbitID))
	rendered.WriteByte('\n')
	rendered.Write(payload)
	if len(payload) > 0 && payload[len(payload)-1] != '\n' {
		rendered.WriteByte('\n')
	}
	rendered.WriteString(endRuntimeAgentsMarker(orbitID))
	rendered.WriteByte('\n')

	return rendered.Bytes(), nil
}

func trimRuntimeAgentsMarkerPadding(content []byte) []byte {
	lines := splitLinesPreserveNewline(content)
	if len(lines) == 0 {
		return nil
	}

	start := 0
	end := len(lines)
	if isBlankRuntimeAgentsLine(lines[start]) {
		start++
	}
	if start < end && isBlankRuntimeAgentsLine(lines[end-1]) {
		end--
	}

	var trimmed bytes.Buffer
	for _, line := range lines[start:end] {
		_, _ = trimmed.Write(line)
	}

	return trimmed.Bytes()
}

func isBlankRuntimeAgentsLine(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

func beginRuntimeAgentsMarker(orbitID string) string {
	return fmt.Sprintf("<!-- orbit:begin workflow=%q -->", orbitID)
}

func endRuntimeAgentsMarker(orbitID string) string {
	return fmt.Sprintf("<!-- orbit:end workflow=%q -->", orbitID)
}

func splitLinesPreserveNewline(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	lines := make([][]byte, 0, bytes.Count(data, []byte{'\n'})+1)
	start := 0
	for index, value := range data {
		if value != '\n' {
			continue
		}
		lines = append(lines, append([]byte(nil), data[start:index+1]...))
		start = index + 1
	}
	if start < len(data) {
		lines = append(lines, append([]byte(nil), data[start:]...))
	}

	return lines
}

func parseRuntimeAgentsMarkerLine(line []byte) (ownerKind RuntimeGuidanceOwnerKind, kind string, workflowID string, isMarker bool, err error) {
	trimmed := strings.TrimSpace(string(line))
	if !strings.Contains(trimmed, "<!-- orbit:") && !strings.Contains(trimmed, "<!-- harness:") {
		return "", "", "", false, nil
	}

	matches := runtimeAgentsMarkerPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", "", "", false, fmt.Errorf("malformed %s marker %q", malformedRuntimeGuidanceMarkerOwner(trimmed), trimmed)
	}
	ownerKind = RuntimeGuidanceOwnerKind(matches[1])
	workflowID = matches[3]
	if err := ids.ValidateOrbitID(workflowID); err != nil {
		return "", "", "", false, fmt.Errorf("validate %s workflow id %q: %w", ownerKind, workflowID, err)
	}

	return ownerKind, matches[2], workflowID, true, nil
}

func malformedRuntimeGuidanceMarkerOwner(marker string) string {
	switch {
	case strings.Contains(marker, "<!-- orbit:"):
		return "orbit"
	case strings.Contains(marker, "<!-- harness:"):
		return "harness"
	default:
		return "root guidance"
	}
}

func runtimeGuidanceBlockKey(ownerKind RuntimeGuidanceOwnerKind, workflowID string) string {
	return string(ownerKind) + "\x00" + workflowID
}

func orbitIDForRuntimeGuidanceSegment(ownerKind RuntimeGuidanceOwnerKind, workflowID string) string {
	if ownerKind != RuntimeGuidanceOwnerKindOrbit {
		return ""
	}

	return workflowID
}
