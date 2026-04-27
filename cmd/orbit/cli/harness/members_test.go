package harness

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/testutil"
)

func TestAddManualMemberAppendsRuntimeMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	runtimeFile, err := DefaultRuntimeFile(repo.Root, time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	definition, err := orbitpkg.DefaultDefinition("docs")
	require.NoError(t, err)
	_, err = orbitpkg.WriteHostedDefinition(repo.Root, definition)
	require.NoError(t, err)

	result, err := AddManualMember(context.Background(), repo.Root, "docs", time.Date(2026, time.March, 26, 11, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, ManifestPath(repo.Root), result.ManifestPath)
	require.Equal(t, []RuntimeMember{
		{
			OrbitID: "docs",
			Source:  MemberSourceManual,
			AddedAt: time.Date(2026, time.March, 26, 11, 0, 0, 0, time.UTC),
		},
	}, result.Runtime.Members)
	require.Equal(t, time.Date(2026, time.March, 26, 11, 0, 0, 0, time.UTC), result.Runtime.Harness.UpdatedAt)
}

func TestActiveInstallOrbitIDsReturnsOnlyInstallBackedMembers(t *testing.T) {
	t.Parallel()

	result := ActiveInstallOrbitIDs(RuntimeFile{
		Members: []RuntimeMember{
			{OrbitID: "manual", Source: MemberSourceManual},
			{OrbitID: "docs", Source: MemberSourceInstallOrbit},
			{OrbitID: "bundle", Source: MemberSourceInstallBundle},
			{OrbitID: "cli", Source: MemberSourceInstallOrbit},
		},
	})
	require.Equal(t, []string{"cli", "docs"}, result)
}

func TestAddManualMemberRejectsMissingDefinition(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	runtimeFile, err := DefaultRuntimeFile(repo.Root, time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = AddManualMember(context.Background(), repo.Root, "docs", time.Time{})
	require.Error(t, err)
	require.ErrorContains(t, err, "load orbit definition")
}

func TestRemoveMemberDeletesExistingRuntimeMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	_, err := WriteRuntimeFile(repo.Root, RuntimeFile{
		SchemaVersion: 1,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        "workspace",
			CreatedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC),
		},
		Members: []RuntimeMember{
			{
				OrbitID: "docs",
				Source:  MemberSourceManual,
				AddedAt: time.Date(2026, time.March, 26, 10, 30, 0, 0, time.UTC),
			},
		},
	})
	require.NoError(t, err)

	result, err := RemoveMember(repo.Root, "docs", time.Date(2026, time.March, 26, 11, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Empty(t, result.Runtime.Members)
	require.Equal(t, time.Date(2026, time.March, 26, 11, 0, 0, 0, time.UTC), result.Runtime.Harness.UpdatedAt)
}

func TestRemoveMemberRejectsMissingMember(t *testing.T) {
	t.Parallel()

	repo := testutil.NewRepo(t)
	runtimeFile, err := DefaultRuntimeFile(repo.Root, time.Date(2026, time.March, 26, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = WriteRuntimeFile(repo.Root, runtimeFile)
	require.NoError(t, err)

	_, err = RemoveMember(repo.Root, "docs", time.Time{})
	require.Error(t, err)
	require.ErrorContains(t, err, "member \"docs\" not found")
}
