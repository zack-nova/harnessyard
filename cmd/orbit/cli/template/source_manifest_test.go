package orbittemplate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteAndLoadSourceManifestRoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	input := SourceManifest{
		SchemaVersion: 1,
		Kind:          SourceKind,
		SourceBranch:  "main",
		Publish: &SourcePublishConfig{
			Package: testOrbitPackage("docs"),
			OrbitID: "docs",
		},
	}

	filename, err := WriteSourceManifest(repoRoot, input)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(repoRoot, ".harness", "manifest.yaml"), filename)

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, ""+
		"schema_version: 1\n"+
		"kind: source\n"+
		"source:\n"+
		"    package:\n"+
		"        type: orbit\n"+
		"        name: docs\n"+
		"    source_branch: main\n", string(data))

	loaded, err := LoadSourceManifest(repoRoot)
	require.NoError(t, err)
	require.Equal(t, input, loaded)
}

func TestParseSourceManifestDataRejectsMissingSourceBranch(t *testing.T) {
	t.Parallel()

	_, err := ParseSourceManifestData([]byte("" +
		"schema_version: 1\n" +
		"kind: source\n" +
		"source:\n" +
		"  package:\n" +
		"    type: orbit\n" +
		"    name: docs\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "source_branch")
}

func TestValidateSourceManifestRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    SourceManifest
		contains string
	}{
		{
			name: "schema version must stay frozen",
			input: SourceManifest{
				SchemaVersion: 2,
				Kind:          SourceKind,
				SourceBranch:  "main",
			},
			contains: "schema_version must be 1",
		},
		{
			name: "kind must stay frozen",
			input: SourceManifest{
				SchemaVersion: 1,
				Kind:          "template",
				SourceBranch:  "main",
			},
			contains: "kind must be \"source\"",
		},
		{
			name: "source branch must not be empty",
			input: SourceManifest{
				SchemaVersion: 1,
				Kind:          SourceKind,
				SourceBranch:  "",
			},
			contains: "source_branch must not be empty",
		},
		{
			name: "publish orbit id must be valid when present",
			input: SourceManifest{
				SchemaVersion: 1,
				Kind:          SourceKind,
				SourceBranch:  "main",
				Publish: &SourcePublishConfig{
					OrbitID: "Docs",
				},
			},
			contains: "source.package.name",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSourceManifest(testCase.input)
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.contains)
		})
	}
}
