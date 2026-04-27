package contractutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	directoryPerm   = 0o750
	filePerm        = 0o600
	tempFilePattern = ".orbit-*"
)

type atomicWriteFileHandle interface {
	Name() string
	Chmod(mode os.FileMode) error
	Write(p []byte) (int, error)
	Sync() error
	Close() error
}

var (
	mkdirAll       = os.MkdirAll
	renameFile     = os.Rename
	createTempFile = func(dir string, pattern string) (atomicWriteFileHandle, error) {
		return os.CreateTemp(dir, pattern)
	}
)

// DecodeKnownFields unmarshals YAML while rejecting unknown struct fields.
func DecodeKnownFields(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("decode yaml: %w", err)
	}

	return nil
}

// EncodeYAMLDocument encodes a YAML document using stable indentation.
func EncodeYAMLDocument(root *yaml.Node) ([]byte, error) {
	document := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(4)

	if err := encoder.Encode(document); err != nil {
		_ = encoder.Close()
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close yaml encoder: %w", err)
	}

	return buffer.Bytes(), nil
}

// AtomicWriteFile writes a file via temp file + rename.
func AtomicWriteFile(filename string, data []byte) error {
	return AtomicWriteFileMode(filename, data, filePerm)
}

// AtomicWriteFileMode writes a file via temp file + rename using the requested file permissions.
func AtomicWriteFileMode(filename string, data []byte, perm os.FileMode) error {
	if err := mkdirAll(filepath.Dir(filename), directoryPerm); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", filename, err)
	}

	tempFile, err := createTempFile(filepath.Dir(filename), tempFilePattern)
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", filename, err)
	}

	tempName := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempName)
		}
	}()

	if err := tempFile.Chmod(perm); err != nil {
		_ = tempFile.Close() //nolint:errcheck // Best-effort cleanup after the primary chmod failure.
		return fmt.Errorf("set temp file permissions for %s: %w", filename, err)
	}
	if err := writeAll(tempFile, data); err != nil {
		_ = tempFile.Close() //nolint:errcheck // Best-effort cleanup after the primary write failure.
		return fmt.Errorf("write temp file for %s: %w", filename, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close() //nolint:errcheck // Best-effort cleanup after the primary sync failure.
		return fmt.Errorf("sync temp file for %s: %w", filename, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", filename, err)
	}
	if err := renameFile(tempName, filename); err != nil {
		return fmt.Errorf("rename temp file for %s: %w", filename, err)
	}

	cleanupTemp = false

	return nil
}

func writeAll(file atomicWriteFileHandle, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return fmt.Errorf("write chunk: %w", err)
		}
		if written <= 0 {
			return fmt.Errorf("short write")
		}
		data = data[written:]
	}

	return nil
}
