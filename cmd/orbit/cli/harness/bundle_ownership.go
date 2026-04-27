package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type bundleOwnershipProofError struct {
	Path    string
	Message string
}

func (err *bundleOwnershipProofError) Error() string {
	return err.Message
}

func buildBundleOwnershipDigests(
	definitionFiles []orbittemplate.CandidateFile,
	renderedFiles []orbittemplate.CandidateFile,
	renderedRootAgentsFile *orbittemplate.CandidateFile,
) (map[string]string, string) {
	pathDigests := make(map[string]string, len(definitionFiles)+len(renderedFiles))
	for _, file := range definitionFiles {
		pathDigests[file.Path] = contentDigest(file.Content)
	}
	for _, file := range renderedFiles {
		pathDigests[file.Path] = contentDigest(file.Content)
	}

	rootAgentsDigest := ""
	if renderedRootAgentsFile != nil {
		rootAgentsDigest = contentDigest(renderedRootAgentsFile.Content)
	}

	return pathDigests, rootAgentsDigest
}

func buildBundleOwnedCleanupPlan(
	repoRoot string,
	existingRecord BundleRecord,
	nextRecord BundleRecord,
) (bundleOwnedCleanupPlan, error) {
	nextOwnedPaths := make(map[string]struct{}, len(nextRecord.OwnedPaths))
	for _, path := range nextRecord.OwnedPaths {
		nextOwnedPaths[path] = struct{}{}
	}

	plan := bundleOwnedCleanupPlan{
		DeletePaths:       make([]string, 0),
		PreviousMemberIDs: append([]string(nil), existingRecord.MemberIDs...),
	}
	for _, oldPath := range existingRecord.OwnedPaths {
		if _, stillOwned := nextOwnedPaths[oldPath]; stillOwned {
			continue
		}
		if oldPath == rootAgentsPath {
			if err := verifyBundleAgentsBlockOwnership(repoRoot, existingRecord); err != nil {
				return bundleOwnedCleanupPlan{}, err
			}
			plan.RemoveRootAgentsBlock = true
			continue
		}
		if err := verifyBundleOwnedPathOwnership(repoRoot, existingRecord, oldPath); err != nil {
			return bundleOwnedCleanupPlan{}, err
		}
		plan.DeletePaths = append(plan.DeletePaths, oldPath)
	}
	sort.Strings(plan.DeletePaths)

	return plan, nil
}

func verifyBundleOwnedPathOwnership(repoRoot string, record BundleRecord, repoPath string) error {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	info, err := os.Stat(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat stale bundle-owned path %s: %w", repoPath, err)
	}
	if info.IsDir() {
		return &bundleOwnershipProofError{
			Path:    repoPath,
			Message: "stale bundle-owned path no longer matches recorded content",
		}
	}

	expectedDigest, ok := record.OwnedPathDigests[repoPath]
	if !ok || strings.TrimSpace(expectedDigest) == "" {
		return &bundleOwnershipProofError{
			Path:    repoPath,
			Message: "stale bundle-owned path cannot be safely removed because recorded ownership proof is missing",
		}
	}

	//nolint:gosec // Path is repo-local and derived from the fixed bundle owned_paths contract.
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read stale bundle-owned path %s: %w", repoPath, err)
	}
	if contentDigest(data) != expectedDigest {
		return &bundleOwnershipProofError{
			Path:    repoPath,
			Message: "stale bundle-owned path no longer matches recorded content",
		}
	}

	return nil
}

func verifyBundleAgentsBlockOwnership(repoRoot string, record BundleRecord) error {
	filename := filepath.Join(repoRoot, rootAgentsPath)
	//nolint:gosec // Path is the fixed repo-local runtime AGENTS.md location.
	data, err := os.ReadFile(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read runtime AGENTS.md: %w", err)
	}

	document, err := orbittemplate.ParseRuntimeAgentsDocument(data)
	if err != nil {
		return &bundleOwnershipProofError{
			Path:    rootAgentsPath,
			Message: "stale bundle AGENTS block no longer matches recorded content",
		}
	}

	for _, segment := range document.Segments {
		if segment.OrbitID != record.HarnessID {
			continue
		}
		if strings.TrimSpace(record.RootAgentsDigest) == "" {
			return &bundleOwnershipProofError{
				Path:    rootAgentsPath,
				Message: "stale bundle AGENTS block cannot be safely removed because recorded ownership proof is missing",
			}
		}
		if contentDigest(segment.Content) != record.RootAgentsDigest {
			return &bundleOwnershipProofError{
				Path:    rootAgentsPath,
				Message: "stale bundle AGENTS block no longer matches recorded content",
			}
		}
		return nil
	}

	return nil
}

func contentDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
