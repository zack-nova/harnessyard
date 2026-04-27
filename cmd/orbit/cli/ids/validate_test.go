package ids_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func TestValidateOrbitID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{name: "valid simple", id: "docs"},
		{name: "valid with hyphen", id: "docs-api"},
		{name: "valid with underscore", id: "docs_api"},
		{name: "empty", id: "", wantErr: true},
		{name: "uppercase rejected", id: "Docs", wantErr: true},
		{name: "space rejected", id: "docs api", wantErr: true},
		{name: "slash rejected", id: "docs/api", wantErr: true},
		{name: "leading punctuation rejected", id: "-docs", wantErr: true},
		{name: "trailing punctuation rejected", id: "docs-", wantErr: true},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ids.ValidateOrbitID(testCase.id)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestNormalizeRepoRelativePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		input     string
		want      string
		expectErr bool
	}{
		{name: "simple path", input: "docs/guide.md", want: "docs/guide.md"},
		{name: "cleans relative path", input: "./docs/../README.md", want: "README.md"},
		{name: "normalizes windows separators", input: `docs\guide.md`, want: "docs/guide.md"},
		{name: "rejects absolute unix path", input: "/tmp/docs.md", expectErr: true},
		{name: "rejects absolute windows path", input: `C:\tmp\docs.md`, expectErr: true},
		{name: "rejects repo escape", input: "../docs.md", expectErr: true},
		{name: "rejects repo root", input: ".", expectErr: true},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual, err := ids.NormalizeRepoRelativePath(testCase.input)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, testCase.want, actual)
		})
	}
}
