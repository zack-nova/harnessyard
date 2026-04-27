package git

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLastScopedRef(t *testing.T) {
	t.Parallel()

	refName, err := LastScopedRef("docs")
	require.NoError(t, err)
	require.Equal(t, "refs/orbits/docs/last-scoped", refName)
}

func TestLastRestoreRef(t *testing.T) {
	t.Parallel()

	refName, err := LastRestoreRef("docs")
	require.NoError(t, err)
	require.Equal(t, "refs/orbits/docs/last-restore", refName)
}

func TestUpdateRefWrapsRunnerErrors(t *testing.T) {
	originalRunner := updateRefRunner
	updateRefRunner = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() {
		updateRefRunner = originalRunner
	})

	err := UpdateRef(context.Background(), "/tmp/repo", "refs/orbits/docs/last-scoped", "abc123")
	require.Error(t, err)
	require.ErrorContains(t, err, "git update-ref refs/orbits/docs/last-scoped abc123")
	require.ErrorContains(t, err, "boom")
}
