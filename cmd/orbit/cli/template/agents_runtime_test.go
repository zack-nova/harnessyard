package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRuntimeAgentsDocumentReturnsOrderedSegments(t *testing.T) {
	t.Parallel()

	document, err := ParseRuntimeAgentsDocument([]byte("" +
		"global guidance\n" +
		"<!-- orbit:begin workflow=\"docs\" -->\n" +
		"docs guidance\n" +
		"<!-- orbit:end workflow=\"docs\" -->\n" +
		"tail guidance\n"))
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:    AgentsRuntimeSegmentUnmarked,
				Content: []byte("global guidance\n"),
			},
			{
				Kind:       AgentsRuntimeSegmentBlock,
				OwnerKind:  OwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("docs guidance\n"),
			},
			{
				Kind:    AgentsRuntimeSegmentUnmarked,
				Content: []byte("tail guidance\n"),
			},
		},
	}, document)
}

func TestParseRuntimeAgentsDocumentDistinguishesOwnerKindAndWorkflowID(t *testing.T) {
	t.Parallel()

	document, err := ParseRuntimeAgentsDocument([]byte("" +
		"<!-- orbit:begin workflow=\"docs\" -->\n" +
		"docs orbit guidance\n" +
		"<!-- orbit:end workflow=\"docs\" -->\n" +
		"<!-- harness:begin workflow=\"docs\" -->\n" +
		"docs harness guidance\n" +
		"<!-- harness:end workflow=\"docs\" -->\n"))
	require.NoError(t, err)
	require.Len(t, document.Segments, 2)
	require.Equal(t, AgentsRuntimeSegmentBlock, document.Segments[0].Kind)
	require.Equal(t, OwnerKindOrbit, document.Segments[0].OwnerKind)
	require.Equal(t, "docs", document.Segments[0].WorkflowID)
	require.Equal(t, []byte("docs orbit guidance\n"), document.Segments[0].Content)
	require.Equal(t, AgentsRuntimeSegmentBlock, document.Segments[1].Kind)
	require.Equal(t, OwnerKindHarness, document.Segments[1].OwnerKind)
	require.Equal(t, "docs", document.Segments[1].WorkflowID)
	require.Equal(t, []byte("docs harness guidance\n"), document.Segments[1].Content)
}

func TestWrapRuntimeAgentsBlockProducesParseableBlock(t *testing.T) {
	t.Parallel()

	block, err := WrapRuntimeAgentsBlock("docs", []byte("docs guidance"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"docs guidance\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n", string(block))

	document, err := ParseRuntimeAgentsDocument(block)
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:       AgentsRuntimeSegmentBlock,
				OwnerKind:  OwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("docs guidance\n"),
			},
		},
	}, document)
}

func TestRuntimeAgentsOwnerBlockOperationsTargetOwnerKindAndWorkflowID(t *testing.T) {
	t.Parallel()

	orbitBlock, err := WrapRuntimeAgentsOwnerBlock(OwnerKindOrbit, "docs", []byte("orbit guidance\n"))
	require.NoError(t, err)
	harnessBlock, err := WrapRuntimeAgentsOwnerBlock(OwnerKindHarness, "docs", []byte("old harness guidance\n"))
	require.NoError(t, err)
	replacement, err := WrapRuntimeAgentsOwnerBlock(OwnerKindHarness, "docs", []byte("new harness guidance\n"))
	require.NoError(t, err)

	replaced, err := ReplaceOrAppendRuntimeAgentsOwnerBlockData(
		append(orbitBlock, harnessBlock...),
		OwnerKindHarness,
		"docs",
		replacement,
	)
	require.NoError(t, err)
	require.Contains(t, string(replaced), "<!-- orbit:begin workflow=\"docs\" -->\norbit guidance\n<!-- orbit:end workflow=\"docs\" -->\n")
	require.NotContains(t, string(replaced), "old harness guidance")
	require.Contains(t, string(replaced), "<!-- harness:begin workflow=\"docs\" -->\nnew harness guidance\n<!-- harness:end workflow=\"docs\" -->\n")

	removed, didRemove, err := RemoveRuntimeAgentsOwnerBlockData(replaced, OwnerKindHarness, "docs")
	require.NoError(t, err)
	require.True(t, didRemove)
	require.Contains(t, string(removed), "<!-- orbit:begin workflow=\"docs\" -->\norbit guidance\n<!-- orbit:end workflow=\"docs\" -->\n")
	require.NotContains(t, string(removed), "<!-- harness:begin workflow=\"docs\" -->")
}

func TestParseRuntimeAgentsDocumentIgnoresFormatterPaddingAroundMarkers(t *testing.T) {
	t.Parallel()

	document, err := ParseRuntimeAgentsDocument([]byte("" +
		"<!-- orbit:begin workflow=\"docs\" -->\n" +
		"\n" +
		"docs guidance\n" +
		"\n" +
		"<!-- orbit:end workflow=\"docs\" -->\n"))
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:       AgentsRuntimeSegmentBlock,
				OwnerKind:  OwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("docs guidance\n"),
			},
		},
	}, document)
}

func TestParseRuntimeAgentsDocumentNamesOwnerKindForMalformedKnownNamespaceMarker(t *testing.T) {
	t.Parallel()

	_, err := ParseRuntimeAgentsDocument([]byte("<!-- harness:begin orbit_id=\"docs\" -->\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "malformed harness block marker")
}

func TestParseRuntimeAgentsDocumentNamesOwnerKindForInvalidWorkflowID(t *testing.T) {
	t.Parallel()

	_, err := ParseRuntimeAgentsDocument([]byte("<!-- harness:begin workflow=\"Docs\" -->\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, `validate harness block workflow id "Docs"`)
}

func TestParseRuntimeAgentsDocumentRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name: "nested blocks fail closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"<!-- orbit:begin workflow=\"api\" -->\n" +
				"<!-- orbit:end workflow=\"api\" -->\n" +
				"<!-- orbit:end workflow=\"docs\" -->\n",
			contains: "nested orbit block",
		},
		{
			name: "mismatched end fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"docs guidance\n" +
				"<!-- orbit:end workflow=\"api\" -->\n",
			contains: "does not match begin orbit block",
		},
		{
			name: "mismatched owner kind fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"docs guidance\n" +
				"<!-- harness:end workflow=\"docs\" -->\n",
			contains: "does not match begin orbit block",
		},
		{
			name: "duplicate orbit blocks fail closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"first\n" +
				"<!-- orbit:end workflow=\"docs\" -->\n" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"second\n" +
				"<!-- orbit:end workflow=\"docs\" -->\n",
			contains: "duplicate orbit block",
		},
		{
			name: "unmatched end fails closed",
			input: "" +
				"<!-- orbit:end workflow=\"docs\" -->\n",
			contains: "unexpected orbit end marker",
		},
		{
			name: "orbit_id attribute fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=docs -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "legacy quoted orbit_id attribute fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"docs guidance\n" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "duplicate workflow attribute fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" workflow=\"api\" -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "unknown attribute fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" extra=\"value\" -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "unknown namespace fails closed",
			input: "" +
				"<!-- workspace:begin workflow=\"docs\" -->\n",
			contains: "malformed workflow marker",
		},
		{
			name: "missing closing delimiter fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\"\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "single quoted workflow fails closed",
			input: "" +
				"<!-- orbit:begin workflow='docs' -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "unquoted workflow fails closed",
			input: "" +
				"<!-- orbit:begin workflow=docs -->\n",
			contains: "malformed orbit block marker",
		},
		{
			name: "invalid workflow id fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"Docs\" -->\n",
			contains: "validate orbit block workflow id",
		},
		{
			name: "unclosed begin marker fails closed",
			input: "" +
				"<!-- orbit:begin workflow=\"docs\" -->\n" +
				"docs guidance\n",
			contains: "unterminated orbit block",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseRuntimeAgentsDocument([]byte(testCase.input))
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}

func TestWrapRuntimeAgentsBlockRejectsInvalidOrbitID(t *testing.T) {
	t.Parallel()

	_, err := WrapRuntimeAgentsBlock("Docs", []byte("content"))
	require.Error(t, err)
	require.ErrorContains(t, err, "validate orbit block workflow id")
}

func TestWrapRuntimeAgentsOwnerBlockRejectsInvalidWorkflowIDWithOwnerKind(t *testing.T) {
	t.Parallel()

	_, err := WrapRuntimeAgentsOwnerBlock(OwnerKindHarness, "Docs", []byte("content"))
	require.Error(t, err)
	require.ErrorContains(t, err, "validate harness block workflow id")
}
