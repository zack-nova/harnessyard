package orbittemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanVariablesCollectsUniqueReferencesAcrossFiles(t *testing.T) {
	t.Parallel()

	result := ScanVariables(
		[]CandidateFile{
			{
				Path:    "docs/intro.md",
				Content: []byte("Welcome to $project_name.\n"),
			},
			{
				Path:    "docs/setup.md",
				Content: []byte("Use $service_url for setup.\nDuplicate: $project_name.\nAlso $_internal_flag.\nIgnore $9bad.\n"),
			},
		},
		map[string]VariableSpec{
			"_internal_flag": {
				Required: true,
			},
			"project_name": {
				Required: true,
			},
			"service_url": {
				Required: true,
			},
			"unused_value": {
				Required: false,
			},
		},
	)

	require.Equal(t, []string{
		"_internal_flag",
		"project_name",
		"service_url",
	}, result.Referenced)
	require.Empty(t, result.Undeclared)
	require.Equal(t, []string{
		"unused_value",
	}, result.Unused)
}

func TestScanVariablesReportsUndeclaredAndUnusedWithStableOrdering(t *testing.T) {
	t.Parallel()

	result := ScanVariables(
		[]CandidateFile{
			{
				Path:    "README.md",
				Content: []byte("$zeta\n$alpha\n$missing_two\n$missing_one\n"),
			},
		},
		map[string]VariableSpec{
			"alpha": {
				Required: true,
			},
			"beta": {
				Required: true,
			},
			"zeta": {
				Required: true,
			},
		},
	)

	require.Equal(t, []string{
		"alpha",
		"missing_one",
		"missing_two",
		"zeta",
	}, result.Referenced)
	require.Equal(t, []string{
		"missing_one",
		"missing_two",
	}, result.Undeclared)
	require.Equal(t, []string{
		"beta",
	}, result.Unused)
}

func TestScanVariablesAllowsEmptyFilesAndEmptyDeclarations(t *testing.T) {
	t.Parallel()

	result := ScanVariables(nil, nil)
	require.Empty(t, result.Referenced)
	require.Empty(t, result.Undeclared)
	require.Empty(t, result.Unused)
}

func TestScanVariablesSkipsBinaryOrInvalidUTF8Files(t *testing.T) {
	t.Parallel()

	result := ScanVariables(
		[]CandidateFile{
			{
				Path:    "docs/guide.md",
				Content: []byte("Visible $project_name reference\n"),
			},
			{
				Path:    "assets/logo.bin",
				Content: []byte{0x00, '$', 's', 'h', 'o', 'u', 'l', 'd', '_', 's', 'k', 'i', 'p'},
			},
			{
				Path:    "assets/data.dat",
				Content: []byte{0xff, 0xfe, '$', 'a', 'l', 's', 'o', '_', 's', 'k', 'i', 'p'},
			},
		},
		map[string]VariableSpec{
			"project_name": {
				Required: true,
			},
		},
	)

	require.Equal(t, []string{"project_name"}, result.Referenced)
	require.Empty(t, result.Undeclared)
	require.Empty(t, result.Unused)
}

func TestScanVariablesIgnoresNonMarkdownFiles(t *testing.T) {
	t.Parallel()

	result := ScanVariables(
		[]CandidateFile{
			{
				Path:    "docs/guide.md",
				Content: []byte("Visible $project_name reference\n"),
			},
			{
				Path:    "schema/example.schema.json",
				Content: []byte("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"title\":\"$ignored_json\"}\n"),
			},
			{
				Path:    "tools/build.mjs",
				Content: []byte("const ref = \"$ignored_js\";\n"),
			},
		},
		map[string]VariableSpec{
			"project_name": {
				Required: true,
			},
		},
	)

	require.Equal(t, []string{"project_name"}, result.Referenced)
	require.Empty(t, result.Undeclared)
	require.Empty(t, result.Unused)
}
