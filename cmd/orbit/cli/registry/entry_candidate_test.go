package registry_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/registry"
)

func TestCatalogIndexDataWithEntryCandidateMergesExistingNamespaceIndex(t *testing.T) {
	t.Parallel()

	existing := []byte("" +
		"schema_version: 1\n" +
		"namespace: acme\n" +
		"packages:\n" +
		"  api:\n" +
		"    handle: acme/api\n" +
		"    status: active\n" +
		"    package:\n" +
		"      type: orbit\n" +
		"      name: api\n" +
		"    source:\n" +
		"      repository: https://example.com/acme/api.git\n" +
		"    dist_tags:\n" +
		"      latest: 1.0.0\n" +
		"    versions:\n" +
		"      1.0.0:\n" +
		"        locator:\n" +
		"          kind: git\n" +
		"          repository: https://example.com/acme/api.git\n" +
		"          ref: orbit-template/api\n" +
		"          commit: 0123456789abcdef0123456789abcdef01234567\n" +
		"  docs:\n" +
		"    handle: acme/docs\n" +
		"    status: active\n" +
		"    package:\n" +
		"      type: orbit\n" +
		"      name: docs\n" +
		"    source:\n" +
		"      repository: https://example.com/acme/docs.git\n" +
		"    dist_tags:\n" +
		"      latest: 0.1.0\n" +
		"    versions:\n" +
		"      0.1.0:\n" +
		"        locator:\n" +
		"          kind: git\n" +
		"          repository: https://example.com/acme/docs.git\n" +
		"          ref: orbit-template/docs\n" +
		"          commit: abcdef0123456789abcdef0123456789abcdef01\n")

	data, err := registry.CatalogIndexDataWithEntryCandidate(existing, registry.EntryCandidate{
		SchemaVersion:   registry.EntryCandidateSchemaVersion,
		TargetPath:      "packages/acme/index.yaml",
		Submittable:     true,
		PackageType:     "orbit",
		PackageIdentity: "docs",
		PackageHandle:   "acme/docs",
		Version:         "0.2.0",
		PackageStatus:   string(registry.PackageStatusDeprecated),
		Source: registry.EntryCandidateSource{
			Remote: "https://example.com/acme/docs.git",
			Ref:    "orbit-template/docs-v2",
			Commit: "2222222222222222222222222222222222222222",
		},
	})
	require.NoError(t, err)

	var index registryCatalogIndexYAML
	require.NoError(t, yaml.Unmarshal(data, &index))
	require.Equal(t, "acme", index.Namespace)
	require.Contains(t, index.Packages, "api")
	require.Equal(t, "1.0.0", index.Packages["api"].DistTags["latest"])
	require.Contains(t, index.Packages["docs"].Versions, "0.1.0")
	require.Contains(t, index.Packages["docs"].Versions, "0.2.0")
	require.Equal(t, "deprecated", index.Packages["docs"].Status)
	require.Equal(t, "0.2.0", index.Packages["docs"].DistTags["latest"])
	require.Equal(t, "orbit-template/docs-v2", index.Packages["docs"].Versions["0.2.0"].Locator.Ref)
	require.Equal(t, "refs/heads/orbit-template/docs-v2", index.Packages["docs"].Versions["0.2.0"].Validation.RemoteRef)
}

type registryCatalogIndexYAML struct {
	Namespace string                                `yaml:"namespace"`
	Packages  map[string]registryCatalogPackageYAML `yaml:"packages"`
}

type registryCatalogPackageYAML struct {
	Status   string                                `yaml:"status"`
	DistTags map[string]string                     `yaml:"dist_tags"`
	Versions map[string]registryCatalogVersionYAML `yaml:"versions"`
}

type registryCatalogVersionYAML struct {
	Locator    registryCatalogLocatorYAML    `yaml:"locator"`
	Validation registryCatalogValidationYAML `yaml:"validation"`
}

type registryCatalogLocatorYAML struct {
	Ref string `yaml:"ref"`
}

type registryCatalogValidationYAML struct {
	RemoteRef string `yaml:"remote_ref"`
}
