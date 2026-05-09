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
	directoryInclude, err := SpecDocDirectoryIncludePath(orbitID)
	if err != nil {
		return OrbitMember{}, err
	}

	return OrbitMember{
		Name: orbitSpecMemberName,
		Role: OrbitMemberRule,
		Paths: OrbitMemberPaths{
			Include: []string{relativePath, directoryInclude},
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

// SpecDocDirectoryRelativePath returns the repo-relative docs directory path for one orbit's rule content.
func SpecDocDirectoryRelativePath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return filepath.ToSlash(filepath.Join("docs", orbitID)), nil
}

// SpecDocDirectoryIncludePath returns the repo-relative include scope for one orbit's rule content directory.
func SpecDocDirectoryIncludePath(orbitID string) (string, error) {
	relativePath, err := SpecDocDirectoryRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return relativePath + "/**", nil
}

// SpecDocPath returns the absolute docs path for one orbit spec file.
func SpecDocPath(repoRoot string, orbitID string) (string, error) {
	relativePath, err := SpecDocRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(relativePath)), nil
}

// SpecDocDirectoryPath returns the absolute docs directory path for one orbit's rule content.
func SpecDocDirectoryPath(repoRoot string, orbitID string) (string, error) {
	relativePath, err := SpecDocDirectoryRelativePath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(relativePath)), nil
}

// SpecDocReadmePath returns the absolute README path for one orbit's rule content directory.
func SpecDocReadmePath(repoRoot string, orbitID string) (string, error) {
	directoryPath, err := SpecDocDirectoryPath(repoRoot, orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(directoryPath, "README.md"), nil
}

// DefaultSpecDocContent returns the minimal spec-doc scaffold for one orbit.
func DefaultSpecDocContent(orbitID string) ([]byte, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return nil, fmt.Errorf("validate orbit id: %w", err)
	}

	return []byte("# " + orbitID + " Spec\n"), nil
}

// DefaultSpecDocReadmeContent returns the minimal rule-directory README scaffold for one orbit.
func DefaultSpecDocReadmeContent(orbitID string) ([]byte, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return nil, fmt.Errorf("validate orbit id: %w", err)
	}

	return []byte("# " + orbitID + "\n"), nil
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

// PreflightSpecScaffold fails when any --with-spec scaffold path already exists.
func PreflightSpecScaffold(repoRoot string, orbitID string) error {
	filename, err := SpecDocPath(repoRoot, orbitID)
	if err != nil {
		return err
	}
	if err := failIfSpecScaffoldPathExists("spec doc file", filename); err != nil {
		return err
	}

	directoryPath, err := SpecDocDirectoryPath(repoRoot, orbitID)
	if err != nil {
		return err
	}
	if err := failIfSpecScaffoldPathExists("spec doc directory", directoryPath); err != nil {
		return err
	}

	return nil
}

// WriteSpecScaffold writes the minimal spec doc and rule-directory README for one orbit.
func WriteSpecScaffold(repoRoot string, orbitID string) (string, error) {
	if err := PreflightSpecScaffold(repoRoot, orbitID); err != nil {
		return "", err
	}

	filename, err := SpecDocPath(repoRoot, orbitID)
	if err != nil {
		return "", err
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

	readmePath, err := SpecDocReadmePath(repoRoot, orbitID)
	if err != nil {
		return "", err
	}
	readmeContent, err := DefaultSpecDocReadmeContent(orbitID)
	if err != nil {
		return "", err
	}
	if err := atomicWriteFile(readmePath, readmeContent); err != nil {
		return "", fmt.Errorf("write spec doc README: %w", err)
	}

	return filename, nil
}

// WriteSpecDoc writes the --with-spec scaffold and returns the primary spec doc path.
func WriteSpecDoc(repoRoot string, orbitID string) (string, error) {
	return WriteSpecScaffold(repoRoot, orbitID)
}

func failIfSpecScaffoldPathExists(description string, filename string) error {
	if _, err := os.Lstat(filename); err == nil {
		return fmt.Errorf("%s %q already exists", description, filename)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", description, err)
	}

	return nil
}
