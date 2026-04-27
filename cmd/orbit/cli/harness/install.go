package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// InstallRecordSummary groups visible install provenance by effective runtime status.
type InstallRecordSummary struct {
	ActiveIDs   []string
	DetachedIDs []string
	InvalidIDs  []string
}

// LoadInstallRecord reads, decodes, and validates one install record from .harness/installs/.
func LoadInstallRecord(repoRoot string, orbitID string) (orbittemplate.InstallRecord, error) {
	filename, err := InstallRecordPath(repoRoot, orbitID)
	if err != nil {
		return orbittemplate.InstallRecord{}, fmt.Errorf("build install record path: %w", err)
	}
	repoPath, err := InstallRecordRepoPath(orbitID)
	if err != nil {
		return orbittemplate.InstallRecord{}, fmt.Errorf("build install record path: %w", err)
	}

	data, err := gitpkg.ReadTrackedFileWorktreeOrHEAD(context.Background(), repoRoot, repoPath)
	if err != nil {
		return orbittemplate.InstallRecord{}, fmt.Errorf("load harness install record: read %s: %w", filename, err)
	}

	record, err := orbittemplate.ParseInstallRecordData(data)
	if err != nil {
		return orbittemplate.InstallRecord{}, fmt.Errorf("load harness install record: parse %s: %w", filename, err)
	}
	if record.OrbitID != orbitID {
		return orbittemplate.InstallRecord{}, fmt.Errorf("validate %s: orbit_id must match install path", filename)
	}

	return record, nil
}

// WriteInstallRecord validates and writes one install record into .harness/installs/.
func WriteInstallRecord(repoRoot string, record orbittemplate.InstallRecord) (string, error) {
	filename, err := InstallRecordPath(repoRoot, record.OrbitID)
	if err != nil {
		return "", fmt.Errorf("build install record path: %w", err)
	}

	written, err := orbittemplate.WriteInstallRecordFile(filename, record)
	if err != nil {
		return "", fmt.Errorf("write harness install record: %w", err)
	}

	return written, nil
}

// ListInstallRecordIDs lists valid install-record ids from .harness/installs/ with stable ordering.
func ListInstallRecordIDs(repoRoot string) ([]string, error) {
	entries, err := os.ReadDir(InstallRecordsDirPath(repoRoot))
	if err == nil {
		return installRecordIDsFromEntryNames(entryNames(entries))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read harness install records directory: %w", err)
	}

	paths, err := gitpkg.ListFilesAtRev(context.Background(), repoRoot, "HEAD", InstallRecordsDirRepoPath())
	if err != nil {
		return nil, fmt.Errorf("list hidden harness install records from HEAD: %w", err)
	}

	entryNames := make([]string, 0, len(paths))
	for _, repoPath := range paths {
		if pathpkg.Dir(repoPath) != InstallRecordsDirRepoPath() {
			continue
		}
		entryNames = append(entryNames, pathpkg.Base(repoPath))
	}

	return installRecordIDsFromEntryNames(entryNames)
}

// SummarizeInstallRecords groups install provenance into active, detached, and invalid buckets.
func SummarizeInstallRecords(repoRoot string) (InstallRecordSummary, error) {
	idsList, err := ListInstallRecordIDs(repoRoot)
	if err != nil {
		return InstallRecordSummary{}, err
	}

	summary := InstallRecordSummary{
		ActiveIDs:   make([]string, 0, len(idsList)),
		DetachedIDs: make([]string, 0),
		InvalidIDs:  make([]string, 0),
	}
	for _, orbitID := range idsList {
		record, err := LoadInstallRecord(repoRoot, orbitID)
		if err != nil {
			summary.InvalidIDs = append(summary.InvalidIDs, orbitID)
			continue
		}

		if orbittemplate.EffectiveInstallRecordStatus(record) == orbittemplate.InstallRecordStatusDetached {
			summary.DetachedIDs = append(summary.DetachedIDs, orbitID)
			continue
		}
		summary.ActiveIDs = append(summary.ActiveIDs, orbitID)
	}

	return summary, nil
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	return names
}

func installRecordIDsFromEntryNames(names []string) ([]string, error) {
	idsList := make([]string, 0, len(names))
	for _, name := range names {
		if filepath.Ext(name) != ".yaml" {
			continue
		}

		orbitID := strings.TrimSuffix(name, ".yaml")
		if err := ids.ValidateOrbitID(orbitID); err != nil {
			return nil, fmt.Errorf("validate install record filename %q: %w", name, err)
		}

		idsList = append(idsList, orbitID)
	}

	sort.Strings(idsList)

	return idsList, nil
}
