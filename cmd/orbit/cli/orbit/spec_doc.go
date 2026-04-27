package orbit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	orbitSpecMemberName = "spec"
)

// DefaultSpecMember returns the optional spec-doc member used by authoring create/init flows.
func DefaultSpecMember(orbitID string) (OrbitMember, error) {
	relativePath, err := SpecDocRelativePath(orbitID)
	if err != nil {
		return OrbitMember{}, err
	}

	return OrbitMember{
		Name: orbitSpecMemberName,
		Role: OrbitMemberRule,
		Paths: OrbitMemberPaths{
			Include: []string{relativePath},
		},
	}, nil
}

// SpecDocRelativePath returns the repo-relative docs path for one orbit spec file.
func SpecDocRelativePath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return filepath.ToSlash(filepath.Join("docs", orbitID+".md")), nil
}

// SpecDocPath returns the absolute docs path for one orbit spec file.
func SpecDocPath(repoRoot string, orbitID string) (string, error) {
	relativePath, err := SpecDocRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(relativePath)), nil
}

// DefaultSpecDocContent returns the minimal spec-doc scaffold for one orbit.
func DefaultSpecDocContent(orbitID string) ([]byte, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return nil, fmt.Errorf("validate orbit id: %w", err)
	}

	return []byte("# " + orbitID + " Spec\n"), nil
}

// AddSpecMember appends the optional spec-doc member to one member-schema orbit spec.
func AddSpecMember(spec OrbitSpec) (OrbitSpec, error) {
	if !spec.HasMemberSchema() {
		return OrbitSpec{}, errors.New("spec member requires member schema")
	}
	for _, member := range spec.Members {
		if orbitMemberIdentityName(member) == orbitSpecMemberName {
			return OrbitSpec{}, fmt.Errorf("member %q already exists in orbit %q", orbitSpecMemberName, spec.ID)
		}
	}

	member, err := DefaultSpecMember(spec.ID)
	if err != nil {
		return OrbitSpec{}, err
	}
	spec.Members = append(spec.Members, member)

	return spec, nil
}

// WriteSpecDoc writes the minimal spec doc file for one orbit.
func WriteSpecDoc(repoRoot string, orbitID string) (string, error) {
	filename, err := SpecDocPath(repoRoot, orbitID)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filename); err == nil {
		return "", fmt.Errorf("spec doc file %q already exists", filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat spec doc: %w", err)
	}

	content, err := DefaultSpecDocContent(orbitID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(filename), orbitDirPerm); err != nil {
		return "", fmt.Errorf("create spec doc directory: %w", err)
	}
	if err := atomicWriteFile(filename, content); err != nil {
		return "", fmt.Errorf("write spec doc: %w", err)
	}

	return filename, nil
}
