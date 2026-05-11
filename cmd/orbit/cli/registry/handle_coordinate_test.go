package registry_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/registry"
)

func TestParsePackageHandleCoordinateNormalizesNamespacedExactVersion(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("Acme/Docs@v0.1.0")
	require.NoError(t, err)
	require.Equal(t, "Acme/Docs@v0.1.0", coordinate.Raw)
	require.Equal(t, "acme", coordinate.Namespace)
	require.Equal(t, "docs", coordinate.Name)
	require.Equal(t, "0.1.0", coordinate.Version)
	require.Equal(t, registry.HandleCoordinateExactVersion, coordinate.Kind)
	require.Equal(t, "acme/docs@0.1.0", coordinate.String())
}

func TestParsePackageHandleCoordinateRejectsNPMStyleNamespace(t *testing.T) {
	t.Parallel()

	_, err := registry.ParsePackageHandleCoordinate("@acme/docs")
	require.Error(t, err)
	require.ErrorContains(t, err, "package handle coordinates use namespace/name[@version-or-tag]")
	require.ErrorContains(t, err, "not npm-style @namespace/name")
}

func TestParsePackageHandleCoordinateParsesDistTagButDoesNotCallItExact(t *testing.T) {
	t.Parallel()

	coordinate, err := registry.ParsePackageHandleCoordinate("ACME/Docs@Latest")
	require.NoError(t, err)
	require.Equal(t, registry.HandleCoordinateDistTag, coordinate.Kind)
	require.Equal(t, "latest", coordinate.Tag)
	require.False(t, coordinate.IsExactVersion())
	require.Equal(t, "acme/docs@latest", coordinate.String())
}
