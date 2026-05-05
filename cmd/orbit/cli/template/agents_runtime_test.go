package orbittemplate

import (
	"os"
	"path/filepath"
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
				OwnerKind:  RuntimeGuidanceOwnerKindOrbit,
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
				OwnerKind:  RuntimeGuidanceOwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("docs guidance\n"),
			},
		},
	}, document)
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
				OwnerKind:  RuntimeGuidanceOwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("docs guidance\n"),
			},
		},
	}, document)
}

func TestParseRuntimeAgentsDocumentScopesBlocksByOwnerKindAndWorkflow(t *testing.T) {
	t.Parallel()

	document, err := ParseRuntimeAgentsDocument([]byte("" +
		"<!-- harness:begin workflow=\"docs\" -->\n" +
		"harness docs guidance\n" +
		"<!-- harness:end workflow=\"docs\" -->\n" +
		"<!-- orbit:begin workflow=\"docs\" -->\n" +
		"orbit docs guidance\n" +
		"<!-- orbit:end workflow=\"docs\" -->\n"))
	require.NoError(t, err)
	require.Equal(t, AgentsRuntimeDocument{
		Segments: []AgentsRuntimeSegment{
			{
				Kind:       AgentsRuntimeSegmentBlock,
				OwnerKind:  RuntimeGuidanceOwnerKindHarness,
				WorkflowID: "docs",
				Content:    []byte("harness docs guidance\n"),
			},
			{
				Kind:       AgentsRuntimeSegmentBlock,
				OwnerKind:  RuntimeGuidanceOwnerKindOrbit,
				WorkflowID: "docs",
				OrbitID:    "docs",
				Content:    []byte("orbit docs guidance\n"),
			},
		},
	}, document)
}

func TestReplaceOrAppendRuntimeAgentsBlockDataPreservesHarnessBlockWithSameWorkflow(t *testing.T) {
	t.Parallel()

	wrappedBlock, err := WrapRuntimeAgentsBlock("docs", []byte("new orbit docs guidance\n"))
	require.NoError(t, err)

	merged, err := ReplaceOrAppendRuntimeAgentsBlockData([]byte(""+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"harness docs guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"old orbit docs guidance\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n"), "docs", wrappedBlock)
	require.NoError(t, err)
	require.Equal(t, ""+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"harness docs guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n"+
		"<!-- orbit:begin workflow=\"docs\" -->\n"+
		"new orbit docs guidance\n"+
		"<!-- orbit:end workflow=\"docs\" -->\n", string(merged))
}

func TestRuntimeAgentsDriftUsesOrbitOwnerKindAndWorkflow(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	filename := filepath.Join(repoRoot, sharedFilePathAgents)
	require.NoError(t, os.WriteFile(filename, []byte(""+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"docs guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n"), 0o600))

	drifted, err := runtimeAgentsHasDrift(repoRoot, "docs", &CandidateFile{
		Content: []byte("docs guidance\n"),
	})
	require.NoError(t, err)
	require.True(t, drifted)

	orbitBlock, err := WrapRuntimeAgentsBlock("docs", []byte("docs guidance\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filename, append([]byte(""+
		"<!-- harness:begin workflow=\"docs\" -->\n"+
		"docs guidance\n"+
		"<!-- harness:end workflow=\"docs\" -->\n"), orbitBlock...), 0o600))

	drifted, err = runtimeAgentsHasDrift(repoRoot, "docs", &CandidateFile{
		Content: []byte("docs guidance\n"),
	})
	require.NoError(t, err)
	require.False(t, drifted)
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
			contains: "does not match begin workflow",
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
			name: "malformed begin marker fails closed",
			input: "" +
				"<!-- orbit:begin workflow=docs -->\n",
			contains: "malformed orbit marker",
		},
		{
			name: "legacy orbit_id marker fails closed",
			input: "" +
				"<!-- orbit:begin orbit_id=\"docs\" -->\n" +
				"docs guidance\n" +
				"<!-- orbit:end orbit_id=\"docs\" -->\n",
			contains: "malformed orbit marker",
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
	require.ErrorContains(t, err, "validate orbit id")
}
