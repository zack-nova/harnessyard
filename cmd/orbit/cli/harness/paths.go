package harness

import (
	"fmt"
	"path/filepath"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	manifestRelativePath  = ".harness/manifest.yaml"
	varsRelativePath      = ".harness/vars.yaml"
	orbitSpecsRelativeDir = ".harness/orbits"
	installsRelativeDir   = ".harness/installs"
	bundlesRelativeDir    = ".harness/bundles"
	templateMembersDir    = ".harness/template_members"
	templateRelativePath  = ".harness/template.yaml"
)

// ManifestRepoPath returns the repository-relative path to .harness/manifest.yaml.
func ManifestRepoPath() string {
	return manifestRelativePath
}

// ManifestPath returns the absolute path to .harness/manifest.yaml.
func ManifestPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(manifestRelativePath))
}

// VarsRepoPath returns the repository-relative path to .harness/vars.yaml.
func VarsRepoPath() string {
	return varsRelativePath
}

// VarsPath returns the absolute path to .harness/vars.yaml.
func VarsPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(varsRelativePath))
}

// OrbitSpecsDirRepoPath returns the repository-relative path to the hosted OrbitSpec directory.
func OrbitSpecsDirRepoPath() string {
	return orbitSpecsRelativeDir
}

// OrbitSpecsDirPath returns the absolute path to the hosted OrbitSpec directory.
func OrbitSpecsDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(orbitSpecsRelativeDir))
}

// OrbitSpecRepoPath returns the repository-relative path to one hosted OrbitSpec.
func OrbitSpecRepoPath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return orbitSpecsRelativeDir + "/" + orbitID + ".yaml", nil
}

// OrbitSpecPath returns the absolute path to one hosted OrbitSpec.
func OrbitSpecPath(repoRoot string, orbitID string) (string, error) {
	repoPath, err := OrbitSpecRepoPath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}

// InstallRecordsDirRepoPath returns the repository-relative path to the install-record directory.
func InstallRecordsDirRepoPath() string {
	return installsRelativeDir
}

// InstallRecordsDirPath returns the absolute path to the install-record directory.
func InstallRecordsDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(installsRelativeDir))
}

// InstallRecordRepoPath returns the repository-relative path to one install record.
func InstallRecordRepoPath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return installsRelativeDir + "/" + orbitID + ".yaml", nil
}

// InstallRecordPath returns the absolute path to one install record.
func InstallRecordPath(repoRoot string, orbitID string) (string, error) {
	repoPath, err := InstallRecordRepoPath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}

// BundleRecordsDirRepoPath returns the repository-relative path to the bundle-record directory.
func BundleRecordsDirRepoPath() string {
	return bundlesRelativeDir
}

// BundleRecordsDirPath returns the absolute path to the bundle-record directory.
func BundleRecordsDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(bundlesRelativeDir))
}

// BundleRecordRepoPath returns the repository-relative path to one bundle record.
func BundleRecordRepoPath(harnessID string) (string, error) {
	if err := ids.ValidateOrbitID(harnessID); err != nil {
		return "", fmt.Errorf("validate harness id: %w", err)
	}

	return bundlesRelativeDir + "/" + harnessID + ".yaml", nil
}

// BundleRecordPath returns the absolute path to one bundle record.
func BundleRecordPath(repoRoot string, harnessID string) (string, error) {
	repoPath, err := BundleRecordRepoPath(harnessID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}

// TemplateRepoPath returns the repository-relative path to .harness/template.yaml.
func TemplateRepoPath() string {
	return templateRelativePath
}

// TemplatePath returns the absolute path to .harness/template.yaml.
func TemplatePath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(templateRelativePath))
}

// TemplateMembersDirRepoPath returns the repository-relative path to the template-member snapshot directory.
func TemplateMembersDirRepoPath() string {
	return templateMembersDir
}

// TemplateMembersDirPath returns the absolute path to the template-member snapshot directory.
func TemplateMembersDirPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(templateMembersDir))
}

// TemplateMemberSnapshotRepoPath returns the repository-relative path to one template-member snapshot file.
func TemplateMemberSnapshotRepoPath(orbitID string) (string, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return "", fmt.Errorf("validate orbit id: %w", err)
	}

	return templateMembersDir + "/" + orbitID + ".yaml", nil
}

// TemplateMemberSnapshotPath returns the absolute path to one template-member snapshot file.
func TemplateMemberSnapshotPath(repoRoot string, orbitID string) (string, error) {
	repoPath, err := TemplateMemberSnapshotRepoPath(orbitID)
	if err != nil {
		return "", err
	}

	return filepath.Join(repoRoot, filepath.FromSlash(repoPath)), nil
}
