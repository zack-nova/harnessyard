package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

const (
	installTransactionJournalVersion = 1
	installTransactionJournalKind    = "harness_template_install"

	installTransactionEntryMissing = "missing"
	installTransactionEntryFile    = "file"
	installTransactionEntryDir     = "dir"
)

// InstallTransaction stores one durable repo-local rollback journal for install mutations.
type InstallTransaction struct {
	repoRoot    string
	journalPath string
	entries     []installTransactionEntry
}

type installTransactionJournal struct {
	SchemaVersion int                       `json:"schema_version"`
	Kind          string                    `json:"kind"`
	CreatedAt     time.Time                 `json:"created_at"`
	Entries       []installTransactionEntry `json:"entries"`
}

type installTransactionEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Mode uint32 `json:"mode,omitempty"`
	Data []byte `json:"data,omitempty"`
}

// BeginInstallTransaction snapshots one bounded write set under .git/orbit/state/transactions/.
func BeginInstallTransaction(
	ctx context.Context,
	repoRoot string,
	paths []string,
) (*InstallTransaction, error) {
	gitDir, err := gitpkg.Dir(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve git dir for install transaction: %w", err)
	}
	store, err := statepkg.NewFSStore(gitDir)
	if err != nil {
		return nil, fmt.Errorf("create state store for install transaction: %w", err)
	}

	transactionsDir := filepath.Join(store.StateDir, "transactions")
	if err := os.MkdirAll(transactionsDir, 0o750); err != nil {
		return nil, fmt.Errorf("create install transaction directory: %w", err)
	}

	dedupedPaths := slicesCompactStrings(append([]string(nil), paths...))
	entries := make([]installTransactionEntry, 0, len(dedupedPaths))
	for _, path := range dedupedPaths {
		entry, err := snapshotInstallTransactionPath(repoRoot, path)
		if err != nil {
			return nil, fmt.Errorf("snapshot install transaction path %s: %w", path, err)
		}
		entries = append(entries, entry)
	}

	journal := installTransactionJournal{
		SchemaVersion: installTransactionJournalVersion,
		Kind:          installTransactionJournalKind,
		CreatedAt:     time.Now().UTC(),
		Entries:       entries,
	}
	journalData, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal install transaction journal: %w", err)
	}
	journalData = append(journalData, '\n')

	journalPath := filepath.Join(
		transactionsDir,
		fmt.Sprintf("harness-template-install-%d.json", time.Now().UTC().UnixNano()),
	)
	if err := contractutil.AtomicWriteFileMode(journalPath, journalData, 0o600); err != nil {
		return nil, fmt.Errorf("write install transaction journal: %w", err)
	}

	return &InstallTransaction{
		repoRoot:    repoRoot,
		journalPath: journalPath,
		entries:     entries,
	}, nil
}

func snapshotInstallTransactionPath(repoRoot string, repoPath string) (installTransactionEntry, error) {
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	info, err := os.Lstat(absolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return installTransactionEntry{
				Path: repoPath,
				Kind: installTransactionEntryMissing,
			}, nil
		}
		return installTransactionEntry{}, fmt.Errorf("lstat existing path: %w", err)
	}

	if info.IsDir() {
		return installTransactionEntry{
			Path: repoPath,
			Kind: installTransactionEntryDir,
			Mode: uint32(info.Mode().Perm()),
		}, nil
	}

	//nolint:gosec // The transaction write set is repo-local and built from validated install targets.
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return installTransactionEntry{}, fmt.Errorf("read existing file: %w", err)
	}

	return installTransactionEntry{
		Path: repoPath,
		Kind: installTransactionEntryFile,
		Mode: uint32(info.Mode().Perm()),
		Data: data,
	}, nil
}

// Rollback restores the pre-transaction state for every journaled path.
func (tx *InstallTransaction) Rollback() error {
	if tx == nil {
		return nil
	}

	errs := make([]error, 0)
	for index := len(tx.entries) - 1; index >= 0; index-- {
		entry := tx.entries[index]
		if err := restoreInstallTransactionEntry(tx.repoRoot, entry); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", entry.Path, err))
		}
	}
	if err := removeInstallTransactionJournal(tx.journalPath); err != nil {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}

	return errors.Join(errs...)
}

// Commit removes the rollback journal after the install mutation becomes durable.
func (tx *InstallTransaction) Commit() {
	if tx == nil {
		return
	}
	if err := removeInstallTransactionJournal(tx.journalPath); err != nil {
		return
	}
}

func restoreInstallTransactionEntry(repoRoot string, entry installTransactionEntry) error {
	absolutePath := filepath.Join(repoRoot, filepath.FromSlash(entry.Path))

	switch entry.Kind {
	case installTransactionEntryMissing:
		if err := os.RemoveAll(absolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove created path: %w", err)
		}
		if err := pruneInstallTransactionParents(repoRoot, filepath.Dir(absolutePath)); err != nil {
			return err
		}
		return nil
	case installTransactionEntryFile:
		if err := os.RemoveAll(absolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear conflicting path: %w", err)
		}
		if err := contractutil.AtomicWriteFileMode(absolutePath, entry.Data, os.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("restore file contents: %w", err)
		}
		return nil
	case installTransactionEntryDir:
		if err := os.RemoveAll(absolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear conflicting path: %w", err)
		}
		if err := os.MkdirAll(absolutePath, os.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("restore directory: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown install transaction entry kind %q", entry.Kind)
	}
}

func pruneInstallTransactionParents(repoRoot string, start string) error {
	cleanRoot := filepath.Clean(repoRoot)
	for current := filepath.Clean(start); current != cleanRoot && current != "." && current != string(filepath.Separator); current = filepath.Dir(current) {
		err := os.Remove(current)
		if err == nil {
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		entries, readErr := os.ReadDir(current)
		if readErr == nil && len(entries) > 0 {
			return nil
		}
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("inspect parent directory %s: %w", current, readErr)
		}

		return fmt.Errorf("remove empty parent directory %s: %w", current, err)
	}

	return nil
}

func removeInstallTransactionJournal(filename string) error {
	if strings.TrimSpace(filename) == "" {
		return nil
	}
	if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove install transaction journal: %w", err)
	}
	return nil
}
