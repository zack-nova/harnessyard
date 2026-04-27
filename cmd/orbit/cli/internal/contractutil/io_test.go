package contractutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicWriteFilePreservesExistingDestinationWhenRenameFails(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "nested", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), directoryPerm))
	require.NoError(t, os.WriteFile(filename, []byte("original\n"), filePerm))

	previousRename := renameFile
	renameFile = func(string, string) error {
		return errors.New("injected rename failure")
	}
	t.Cleanup(func() {
		renameFile = previousRename
	})

	err := AtomicWriteFile(filename, []byte("updated\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "injected rename failure")

	data, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	require.Equal(t, "original\n", string(data))

	tempFiles, globErr := filepath.Glob(filepath.Join(filepath.Dir(filename), tempFilePattern))
	require.NoError(t, globErr)
	require.Empty(t, tempFiles)
}

func TestAtomicWriteFilePreservesExistingDestinationWhenTempWriteFails(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "nested", "vars.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), directoryPerm))
	require.NoError(t, os.WriteFile(filename, []byte("original\n"), filePerm))

	previousCreateTempFile := createTempFile
	createTempFile = func(dir string, pattern string) (atomicWriteFileHandle, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}

		return &failingAtomicWriteFileHandle{File: file}, nil
	}
	t.Cleanup(func() {
		createTempFile = previousCreateTempFile
	})

	err := AtomicWriteFile(filename, []byte("updated\n"))
	require.Error(t, err)
	require.ErrorContains(t, err, "injected write failure")

	data, readErr := os.ReadFile(filename)
	require.NoError(t, readErr)
	require.Equal(t, "original\n", string(data))

	tempFiles, globErr := filepath.Glob(filepath.Join(filepath.Dir(filename), tempFilePattern))
	require.NoError(t, globErr)
	require.Empty(t, tempFiles)
}

type failingAtomicWriteFileHandle struct {
	*os.File
}

func (file *failingAtomicWriteFileHandle) Write(p []byte) (int, error) {
	written, err := file.File.Write(p[:1])
	if err != nil {
		return written, err
	}

	return written, errors.New("injected write failure")
}
