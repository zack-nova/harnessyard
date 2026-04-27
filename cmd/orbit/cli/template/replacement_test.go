package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

func TestReplaceRuntimeValuesReplacesExactLiteralsAndTracksSummary(t *testing.T) {
	t.Parallel()

	result, err := ReplaceRuntimeValues(
		CandidateFile{
			Path: "docs/guide.md",
			Content: []byte(
				"Orbit lives at http://localhost:3000/api.\n" +
					"Fallback: http://localhost:3000\n",
			),
		},
		map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
			"service_api": {
				Value: "http://localhost:3000/api",
			},
			"service_url": {
				Value: "http://localhost:3000",
			},
		},
	)
	require.NoError(t, err)
	require.False(t, result.SkippedBinary)
	require.Empty(t, result.Ambiguities)
	require.Equal(t,
		"$project_name lives at $service_api.\nFallback: $service_url\n",
		string(result.Content),
	)
	require.Equal(t, []ReplacementSummary{
		{
			Variable: "service_api",
			Literal:  "http://localhost:3000/api",
			Count:    1,
		},
		{
			Variable: "service_url",
			Literal:  "http://localhost:3000",
			Count:    1,
		},
		{
			Variable: "project_name",
			Literal:  "Orbit",
			Count:    1,
		},
	}, result.Replacements)
}

func TestReplaceRuntimeValuesDetectsAmbiguityAndDoesNotReplace(t *testing.T) {
	t.Parallel()

	result, err := ReplaceRuntimeValues(
		CandidateFile{
			Path:    "README.md",
			Content: []byte("Orbit docs\n"),
		},
		map[string]bindings.VariableBinding{
			"product_name": {
				Value: "Orbit",
			},
			"project_name": {
				Value: "Orbit",
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "Orbit docs\n", string(result.Content))
	require.Empty(t, result.Replacements)
	require.Equal(t, []ReplacementAmbiguity{
		{
			Literal:   "Orbit",
			Variables: []string{"product_name", "project_name"},
		},
	}, result.Ambiguities)
}

func TestReplaceRuntimeValuesSkipsBinaryFiles(t *testing.T) {
	t.Parallel()

	result, err := ReplaceRuntimeValues(
		CandidateFile{
			Path:    "assets/logo.bin",
			Content: []byte{0x00, 'O', 'r', 'b', 'i', 't'},
		},
		map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
	)
	require.NoError(t, err)
	require.True(t, result.SkippedBinary)
	require.Equal(t, []byte{0x00, 'O', 'r', 'b', 'i', 't'}, result.Content)
	require.Empty(t, result.Replacements)
	require.Empty(t, result.Ambiguities)
}

func TestReplaceRuntimeValuesRejectsEmptyLiteral(t *testing.T) {
	t.Parallel()

	_, err := ReplaceRuntimeValues(
		CandidateFile{
			Path:    "docs/guide.md",
			Content: []byte("Orbit\n"),
		},
		map[string]bindings.VariableBinding{
			"project_name": {
				Value: "",
			},
		},
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "project_name")
	require.ErrorContains(t, err, "must not be empty")
}

func TestReplaceRuntimeValuesIgnoresNonMarkdownFiles(t *testing.T) {
	t.Parallel()

	result, err := ReplaceRuntimeValues(
		CandidateFile{
			Path:    "schema/example.schema.json",
			Content: []byte("{\"title\":\"Orbit\",\"x-template\":\"$project_name\"}\n"),
		},
		map[string]bindings.VariableBinding{
			"project_name": {
				Value: "Orbit",
			},
		},
	)
	require.NoError(t, err)
	require.False(t, result.SkippedBinary)
	require.Contains(t, string(result.Content), "\"title\":\"Orbit\"")
	require.Contains(t, string(result.Content), "\"x-template\":\"$project_name\"")
	require.Empty(t, result.Replacements)
	require.Empty(t, result.Ambiguities)
}
