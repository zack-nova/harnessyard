package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRuntimeAgentsDocumentReturnsOrderedSegments(t *testing.T) {
	t.Parallel()

	document, err := ParseRuntimeAgentsDocument([]byte("" +
		"global guidance\n" +
		"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
		"docs guidance\n" +
		"<!-- orbit:end orbit_id=\"docs\" -->\n" +
		"tail guidance\n"))
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:    AgentsRuntimeSegmentUnmarked,
				Content: []byte("global guidance\n"),
			},
			{
				Kind:    AgentsRuntimeSegmentBlock,
				OrbitID: "docs",
				Content: []byte("docs guidance\n"),
			},
			{
				Kind:    AgentsRuntimeSegmentUnmarked,
				Content: []byte("tail guidance\n"),
			},
		},
	}, document)
}

func TestWrapRuntimeAgentsBlockProducesParseableBlock(t *testing.T) {
	t.Parallel()

	block, err := WrapRuntimeAgentsBlock("docs", []byte("docs guidance"))
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- orbit:begin orbit_id=\"docs\" -->\n"+
		"docs guidance\n"+
		"<!-- orbit:end orbit_id=\"docs\" -->\n", string(block))

	document, err := ParseRuntimeAgentsDocument(block)
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:    AgentsRuntimeSegmentBlock,
				OrbitID: "docs",
				Content: []byte("docs guidance\n"),
			},
		},
	}, document)
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
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"<!-- orbit:begin orbit_id=\"api\" -->\n" +
				"<!-- orbit:end orbit_id=\"api\" -->\n" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n",
			contains: "nested orbit block",
		},
		{
			name: "mismatched end fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"docs guidance\n" +
				"<!-- orbit:end orbit_id=\"api\" -->\n",
			contains: "does not match begin orbit_id",
		},
		{
			name: "duplicate orbit blocks fail closed",
			input: "" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"first\n" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"second\n" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n",
			contains: "duplicate orbit block",
		},
		{
			name: "unmatched end fails closed",
			input: "" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n",
			contains: "unexpected orbit end marker",
		},
		{
			name: "malformed begin marker fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=docs -->\n",
			contains: "malformed orbit marker",
		},
		{
			name: "unclosed begin marker fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
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
	require.ErrorContains(t, err, "validate orbit id")
}
