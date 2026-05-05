package orbittemplate

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// OwnerKind identifies the root guidance block marker namespace.
type OwnerKind string

const (
	OwnerKindOrbit   OwnerKind = "orbit"
	OwnerKindHarness OwnerKind = "harness"
)

// AgentsRuntimeSegmentKind distinguishes unmarked prose from one owner-scoped block.
type AgentsRuntimeSegmentKind string

const (
	AgentsRuntimeSegmentUnmarked AgentsRuntimeSegmentKind = "unmarked"
	AgentsRuntimeSegmentBlock    AgentsRuntimeSegmentKind = "block"
)

var runtimeAgentsMarkerPattern = regexp.MustCompile(`^<!-- (orbit|harness):(begin|end) workflow="([^"]+)" -->$`)

// AgentsRuntimeDocument is the ordered parsed representation of one runtime AGENTS.md file.
type AgentsRuntimeDocument struct {
	Segments []AgentsRuntimeSegment
}

// AgentsRuntimeSegment is either plain unmarked prose or one owner-scoped block body.
type AgentsRuntimeSegment struct {
	Kind       AgentsRuntimeSegmentKind
	OwnerKind  OwnerKind
	WorkflowID string
	OrbitID    string
	Content    []byte
}

type runtimeAgentsMarker struct {
	Kind       string
	OwnerKind  OwnerKind
	WorkflowID string
}

// ParseRuntimeAgentsDocument parses one runtime AGENTS.md file and validates the workflow marker contract.
func ParseRuntimeAgentsDocument(data []byte) (AgentsRuntimeDocument, error) {
	lines := splitLinesPreserveNewline(data)
	document := AgentsRuntimeDocument{
		Segments: make([]AgentsRuntimeSegment, 0, len(lines)),
	}

	var unmarked bytes.Buffer
	var currentBlock bytes.Buffer
	var currentMarker runtimeAgentsMarker
	inBlock := false
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
		marker, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return AgentsRuntimeDocument{}, err
		}
		if isMarker {
			switch marker.Kind {
			case "begin":
				if inBlock {
					return AgentsRuntimeDocument{}, fmt.Errorf("nested %s block for %q", blockOwnerLabel(marker.OwnerKind), marker.WorkflowID)
				}
				flushUnmarked()
				currentMarker = marker
				inBlock = true
				currentBlock.Reset()
			case "end":
				if !inBlock {
					return AgentsRuntimeDocument{}, fmt.Errorf("unexpected %s end marker for %q", blockOwnerLabel(marker.OwnerKind), marker.WorkflowID)
				}
				if currentMarker.OwnerKind != marker.OwnerKind || currentMarker.WorkflowID != marker.WorkflowID {
					return AgentsRuntimeDocument{}, fmt.Errorf(
						"end %s block %q does not match begin %s block %q",
						blockOwnerLabel(marker.OwnerKind),
						marker.WorkflowID,
						blockOwnerLabel(currentMarker.OwnerKind),
						currentMarker.WorkflowID,
					)
				}
				seenKey := runtimeAgentsBlockKey(marker.OwnerKind, marker.WorkflowID)
				if _, exists := seenBlocks[seenKey]; exists {
					return AgentsRuntimeDocument{}, fmt.Errorf("duplicate %s block for %q", blockOwnerLabel(marker.OwnerKind), marker.WorkflowID)
				}
				seenBlocks[seenKey] = struct{}{}
				document.Segments = append(document.Segments, AgentsRuntimeSegment{
					Kind:       AgentsRuntimeSegmentBlock,
					OwnerKind:  marker.OwnerKind,
					WorkflowID: marker.WorkflowID,
					OrbitID:    marker.WorkflowID,
					Content:    trimRuntimeAgentsMarkerPadding(currentBlock.Bytes()),
				})
				currentMarker = runtimeAgentsMarker{}
				inBlock = false
				currentBlock.Reset()
			default:
				return AgentsRuntimeDocument{}, fmt.Errorf("unsupported workflow marker kind %q", marker.Kind)
			}

			continue
		}

		if inBlock {
			_, _ = currentBlock.Write(line)
			continue
		}
		_, _ = unmarked.Write(line)
	}

	if inBlock {
		return AgentsRuntimeDocument{}, fmt.Errorf("unterminated %s block for %q", blockOwnerLabel(currentMarker.OwnerKind), currentMarker.WorkflowID)
	}

	flushUnmarked()

	return document, nil
}

// WrapRuntimeAgentsBlock renders one runtime AGENTS marker block around the provided payload.
func WrapRuntimeAgentsBlock(orbitID string, payload []byte) ([]byte, error) {
	return WrapRuntimeAgentsOwnerBlock(OwnerKindOrbit, orbitID, payload)
}

// WrapRuntimeAgentsOwnerBlock renders one owner-scoped runtime AGENTS marker block around the provided payload.
func WrapRuntimeAgentsOwnerBlock(ownerKind OwnerKind, workflowID string, payload []byte) ([]byte, error) {
	if err := validateRuntimeAgentsBlockID(ownerKind, workflowID); err != nil {
		return nil, err
	}

	var rendered bytes.Buffer
	rendered.WriteString(beginRuntimeAgentsOwnerMarker(ownerKind, workflowID))
	rendered.WriteByte('\n')
	rendered.Write(payload)
	if len(payload) > 0 && payload[len(payload)-1] != '\n' {
		rendered.WriteByte('\n')
	}
	rendered.WriteString(endRuntimeAgentsOwnerMarker(ownerKind, workflowID))
	rendered.WriteByte('\n')

	return rendered.Bytes(), nil
}

func runtimeAgentsDocumentHasNoBlocks(document AgentsRuntimeDocument) bool {
	for _, segment := range document.Segments {
		if segment.Kind == AgentsRuntimeSegmentBlock {
			return false
		}
	}
	return true
}

func runtimeAgentsDocumentContainsRunViewPayload(document AgentsRuntimeDocument, data []byte, payload []byte) bool {
	if !runtimeAgentsDocumentHasNoBlocks(document) {
		return false
	}
	normalizedData := normalizeRuntimeAgentsPayload(data)
	normalizedPayload := normalizeRuntimeAgentsPayload(payload)
	if len(bytes.TrimSpace(normalizedPayload)) == 0 {
		return bytes.Equal(normalizedData, normalizedPayload)
	}
	return bytes.Contains(normalizedData, normalizedPayload)
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

func beginRuntimeAgentsOwnerMarker(ownerKind OwnerKind, workflowID string) string {
	return fmt.Sprintf("<!-- %s:begin workflow=%q -->", ownerKind, workflowID)
}

func endRuntimeAgentsOwnerMarker(ownerKind OwnerKind, workflowID string) string {
	return fmt.Sprintf("<!-- %s:end workflow=%q -->", ownerKind, workflowID)
}

func validateRuntimeAgentsBlockID(ownerKind OwnerKind, workflowID string) error {
	if err := validateOwnerKind(ownerKind); err != nil {
		return err
	}
	if err := ids.ValidateOrbitID(workflowID); err != nil {
		return fmt.Errorf("validate %s block workflow id %q: %w", blockOwnerLabel(ownerKind), workflowID, err)
	}

	return nil
}

func validateOwnerKind(ownerKind OwnerKind) error {
	switch ownerKind {
	case OwnerKindOrbit, OwnerKindHarness:
		return nil
	default:
		return fmt.Errorf("unsupported workflow owner kind %q", ownerKind)
	}
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

func parseRuntimeAgentsMarkerLine(line []byte) (runtimeAgentsMarker, bool, error) {
	trimmed := strings.TrimSpace(string(line))
	if !looksLikeRuntimeAgentsMarker(trimmed) {
		return runtimeAgentsMarker{}, false, nil
	}

	matches := runtimeAgentsMarkerPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		if ownerKind, ok := malformedRuntimeAgentsMarkerOwnerKind(trimmed); ok {
			return runtimeAgentsMarker{}, false, fmt.Errorf("malformed %s block marker %q", blockOwnerLabel(ownerKind), trimmed)
		}
		return runtimeAgentsMarker{}, false, fmt.Errorf("malformed workflow marker %q", trimmed)
	}
	ownerKind := OwnerKind(matches[1])
	if err := ids.ValidateOrbitID(matches[3]); err != nil {
		return runtimeAgentsMarker{}, false, fmt.Errorf("validate %s block workflow id %q: %w", blockOwnerLabel(ownerKind), matches[3], err)
	}

	return runtimeAgentsMarker{
		OwnerKind:  ownerKind,
		Kind:       matches[2],
		WorkflowID: matches[3],
	}, true, nil
}

func looksLikeRuntimeAgentsMarker(trimmed string) bool {
	if strings.HasPrefix(trimmed, "<!-- "+string(OwnerKindOrbit)+":") ||
		strings.HasPrefix(trimmed, "<!-- "+string(OwnerKindHarness)+":") {
		return true
	}
	if !strings.HasPrefix(trimmed, "<!-- ") {
		return false
	}
	body := strings.TrimPrefix(trimmed, "<!-- ")
	firstField, _, _ := strings.Cut(body, " ")
	_, action, ok := strings.Cut(firstField, ":")
	if !ok {
		return false
	}
	return action == "begin" || action == "end"
}

func malformedRuntimeAgentsMarkerOwnerKind(trimmed string) (OwnerKind, bool) {
	if !strings.HasPrefix(trimmed, "<!-- ") {
		return "", false
	}
	body := strings.TrimPrefix(trimmed, "<!-- ")
	firstField, _, _ := strings.Cut(body, " ")
	namespace, _, ok := strings.Cut(firstField, ":")
	if !ok {
		return "", false
	}
	ownerKind := OwnerKind(namespace)
	if err := validateOwnerKind(ownerKind); err != nil {
		return "", false
	}
	return ownerKind, true
}

func runtimeAgentsBlockKey(ownerKind OwnerKind, workflowID string) string {
	return string(ownerKind) + "\x00" + workflowID
}

func blockOwnerLabel(ownerKind OwnerKind) string {
	return string(ownerKind)
}
