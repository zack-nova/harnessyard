package harness

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func TestDefaultHarnessIDForPathSanitizesBasename(t *testing.T) {
	t.Parallel()

	require.Equal(t, "harnessos", DefaultHarnessIDForPath("/tmp/HarnessOS"))
	require.Equal(t, "project-a", DefaultHarnessIDForPath("/tmp/Project A"))
	require.Equal(t, defaultHarnessID, DefaultHarnessIDForPath("/tmp/!!!"))
}

func TestDefaultRuntimeFileUsesDerivedHarnessIdentity(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 25, 15, 0, 0, 0, time.UTC)
	repoRoot := filepath.Join(t.TempDir(), "HarnessOS")

	file, err := DefaultRuntimeFile(repoRoot, now)
	require.NoError(t, err)
	require.Equal(t, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "harnessos",
			Name:      "HarnessOS",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Members: []RuntimeMember{},
	}, file)
}

func TestRuntimeManifestConversionPreservesInstallSourceKinds(t *testing.T) {
	t.Parallel()

	runtimeFile := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 9, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 9, 10, 30, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{OrbitID: "docs", Source: MemberSourceInstallOrbit, AddedAt: time.Date(2026, time.April, 9, 10, 5, 0, 0, time.UTC)},
			{OrbitID: "qa", Source: MemberSourceInstallBundle, OwnerHarnessID: "workspace", AddedAt: time.Date(2026, time.April, 9, 10, 10, 0, 0, time.UTC)},
			{OrbitID: "cli", Source: MemberSourceManual, AddedAt: time.Date(2026, time.April, 9, 10, 15, 0, 0, time.UTC)},
		},
	}

	manifestFile := ManifestFileFromRuntimeFile(runtimeFile)
	require.Equal(t, []ManifestMember{
		{Package: testOrbitPackage("docs"), OrbitID: "docs", Source: ManifestMemberSourceInstallOrbit, AddedAt: time.Date(2026, time.April, 9, 10, 5, 0, 0, time.UTC)},
		{Package: testOrbitPackage("qa"), OrbitID: "qa", Source: ManifestMemberSourceInstallBundle, IncludedIn: testIncludedIn("workspace"), OwnerHarnessID: "workspace", AddedAt: time.Date(2026, time.April, 9, 10, 10, 0, 0, time.UTC)},
		{Package: testOrbitPackage("cli"), OrbitID: "cli", Source: ManifestMemberSourceManual, AddedAt: time.Date(2026, time.April, 9, 10, 15, 0, 0, time.UTC)},
	}, manifestFile.Members)

	convertedRuntimeFile, err := RuntimeFileFromManifestFile(manifestFile)
	require.NoError(t, err)
	require.Equal(t, runtimeFile, convertedRuntimeFile)
}

func TestRuntimeManifestConversionPreservesAffiliationMetadata(t *testing.T) {
	t.Parallel()

	runtimeFile := RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.April, 22, 11, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.April, 22, 11, 30, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID:        "docs",
				Source:         MemberSourceInstallBundle,
				OwnerHarnessID: "writing_stack",
				AddedAt:        time.Date(2026, time.April, 22, 11, 5, 0, 0, time.UTC),
				LastStandaloneOrigin: &orbittemplate.Source{
					SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
					SourceRepo:     "",
					SourceRef:      "orbit-template/docs",
					TemplateCommit: "abc123",
				},
			},
		},
	}

	manifestFile := ManifestFileFromRuntimeFile(runtimeFile)
	require.Equal(t, []ManifestMember{
		{
			Package:        testOrbitPackage("docs"),
			OrbitID:        "docs",
			Source:         ManifestMemberSourceInstallBundle,
			IncludedIn:     testIncludedIn("writing_stack"),
			OwnerHarnessID: "writing_stack",
			AddedAt:        time.Date(2026, time.April, 22, 11, 5, 0, 0, time.UTC),
			LastStandaloneOrigin: &orbittemplate.Source{
				SourceKind:     orbittemplate.InstallSourceKindLocalBranch,
				SourceRepo:     "",
				SourceRef:      "orbit-template/docs",
				TemplateCommit: "abc123",
			},
		},
	}, manifestFile.Members)

	convertedRuntimeFile, err := RuntimeFileFromManifestFile(manifestFile)
	require.NoError(t, err)
	require.Equal(t, runtimeFile, convertedRuntimeFile)
}
