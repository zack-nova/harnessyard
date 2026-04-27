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

var runtimeAgentsMarkerPattern = regexp.MustCompile(`^<!--\s*orbit:(begin|end)\s+orbit_id="([^"]+)"\s*-->$`)

// AgentsRuntimeDocument is the ordered parsed representation of one runtime AGENTS.md file.
type AgentsRuntimeDocument struct {
	Segments []AgentsRuntimeSegment
}

// AgentsRuntimeSegment is either plain unmarked prose or one orbit-owned block body.
type AgentsRuntimeSegment struct {
	Kind    AgentsRuntimeSegmentKind
	OrbitID string
	Content []byte
}

// ParseRuntimeAgentsDocument parses one runtime AGENTS.md file and validates the frozen V0.2 marker contract.
func ParseRuntimeAgentsDocument(data []byte) (AgentsRuntimeDocument, error) {
	lines := splitLinesPreserveNewline(data)
	document := AgentsRuntimeDocument{
		Segments: make([]AgentsRuntimeSegment, 0, len(lines)),
	}

	var unmarked bytes.Buffer
	var currentBlock bytes.Buffer
	currentOrbitID := ""
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
		markerKind, orbitID, isMarker, err := parseRuntimeAgentsMarkerLine(line)
		if err != nil {
			return AgentsRuntimeDocument{}, err
		}
		if isMarker {
			switch markerKind {
			case "begin":
				if currentOrbitID != "" {
					return AgentsRuntimeDocument{}, fmt.Errorf("nested orbit block for %q", orbitID)
				}
				flushUnmarked()
				currentOrbitID = orbitID
				currentBlock.Reset()
			case "end":
				if currentOrbitID == "" {
					return AgentsRuntimeDocument{}, fmt.Errorf("unexpected orbit end marker for %q", orbitID)
				}
				if currentOrbitID != orbitID {
					return AgentsRuntimeDocument{}, fmt.Errorf("end orbit_id %q does not match begin orbit_id %q", orbitID, currentOrbitID)
				}
				if _, exists := seenBlocks[orbitID]; exists {
					return AgentsRuntimeDocument{}, fmt.Errorf("duplicate orbit block for %q", orbitID)
				}
				seenBlocks[orbitID] = struct{}{}
				document.Segments = append(document.Segments, AgentsRuntimeSegment{
					Kind:    AgentsRuntimeSegmentBlock,
					OrbitID: orbitID,
					Content: append([]byte(nil), currentBlock.Bytes()...),
				})
				currentOrbitID = ""
				currentBlock.Reset()
			default:
				return AgentsRuntimeDocument{}, fmt.Errorf("unsupported orbit marker kind %q", markerKind)
			}

			continue
		}

		if currentOrbitID != "" {
			_, _ = currentBlock.Write(line)
			continue
		}
		_, _ = unmarked.Write(line)
	}

	if currentOrbitID != "" {
		return AgentsRuntimeDocument{}, fmt.Errorf("unterminated orbit block for %q", currentOrbitID)
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

func beginRuntimeAgentsMarker(orbitID string) string {
	return fmt.Sprintf("<!-- orbit:begin orbit_id=%q -->", orbitID)
}

func endRuntimeAgentsMarker(orbitID string) string {
	return fmt.Sprintf("<!-- orbit:end orbit_id=%q -->", orbitID)
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

func parseRuntimeAgentsMarkerLine(line []byte) (kind string, orbitID string, isMarker bool, err error) {
	trimmed := strings.TrimSpace(string(line))
	if !strings.Contains(trimmed, "<!-- orbit:") {
		return "", "", false, nil
	}

	matches := runtimeAgentsMarkerPattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", "", false, fmt.Errorf("malformed orbit marker %q", trimmed)
	}
	if err := ids.ValidateOrbitID(matches[2]); err != nil {
		return "", "", false, fmt.Errorf("validate orbit id %q: %w", matches[2], err)
	}

	return matches[1], matches[2], true, nil
}
