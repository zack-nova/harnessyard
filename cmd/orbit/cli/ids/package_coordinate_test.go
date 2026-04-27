package ids_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func TestParsePackageCoordinateValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		input        string
		wantName     string
		wantKind     ids.PackageCoordinateKind
		wantVersion  string
		wantLocator  string
		wantString   string
		wantRelease  bool
		wantSnapshot bool
	}{
		{
			name:         "bare package name is a snapshot selector",
			input:        "execute",
			wantName:     "execute",
			wantKind:     ids.PackageCoordinateName,
			wantString:   "execute",
			wantSnapshot: true,
		},
		{
			name:        "semver release",
			input:       "execute@0.1.0",
			wantName:    "execute",
			wantKind:    ids.PackageCoordinateRelease,
			wantVersion: "0.1.0",
			wantString:  "execute@0.1.0",
			wantRelease: true,
		},
		{
			name:        "semver release normalizes v prefix",
			input:       "execute@v0.1.0",
			wantName:    "execute",
			wantKind:    ids.PackageCoordinateRelease,
			wantVersion: "0.1.0",
			wantString:  "execute@0.1.0",
			wantRelease: true,
		},
		{
			name:       "workspace pseudo coordinate",
			input:      "execute@workspace",
			wantName:   "execute",
			wantKind:   ids.PackageCoordinateWorkspace,
			wantString: "execute@workspace",
		},
		{
			name:        "git locator",
			input:       "execute@git:orbit-template/execute",
			wantName:    "execute",
			wantKind:    ids.PackageCoordinateGitLocator,
			wantLocator: "orbit-template/execute",
			wantString:  "execute@git:orbit-template/execute",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual, err := ids.ParsePackageCoordinate(testCase.input, ids.PackageCoordinateOptions{
				StrictUserLayer: true,
			})
			require.NoError(t, err)
			require.Equal(t, testCase.input, actual.Raw)
			require.Equal(t, testCase.wantName, actual.Name)
			require.Equal(t, testCase.wantKind, actual.Kind)
			require.Equal(t, testCase.wantVersion, actual.Version)
			require.Equal(t, testCase.wantLocator, actual.Locator)
			require.Equal(t, testCase.wantString, actual.String())
			require.Equal(t, testCase.wantRelease, actual.IsRelease())
			require.Equal(t, testCase.wantSnapshot, actual.IsUnversionedSnapshot())
		})
	}
}

func TestParsePackageCoordinateInvalid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		wantMessage string
	}{
		{
			name:        "empty",
			input:       "",
			wantMessage: "package coordinate must not be empty",
		},
		{
			name:        "empty package name",
			input:       "@0.1.0",
			wantMessage: "package name must not be empty",
		},
		{
			name:        "missing locator",
			input:       "execute@",
			wantMessage: "package coordinate must include a version or locator after @",
		},
		{
			name:        "malformed semver",
			input:       "execute@1.0",
			wantMessage: "package coordinate version must use SemVer",
		},
		{
			name:        "empty v-prefixed version",
			input:       "execute@v",
			wantMessage: "package coordinate version must use SemVer",
		},
		{
			name:        "bare ref rejected in strict user layer mode",
			input:       "execute@main",
			wantMessage: "package coordinate locator must be explicit",
		},
		{
			name:        "invalid package name uses package language",
			input:       "Execute@0.1.0",
			wantMessage: "package name must use lowercase letters",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ids.ParsePackageCoordinate(testCase.input, ids.PackageCoordinateOptions{
				StrictUserLayer: true,
			})
			require.Error(t, err)
			require.ErrorContains(t, err, testCase.wantMessage)
			require.NotContains(t, err.Error(), "orbit id")
			require.NotContains(t, err.Error(), "harness_id")
		})
	}
}
