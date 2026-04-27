package state

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrLockHeld indicates another Orbit command currently holds the state lock.
var ErrLockHeld = errors.New("orbit state lock already held")

// Lock represents a held repo-local Orbit lock file.
type Lock struct {
	file *os.File
}

// AcquireLock obtains the repo-local lock that guards state and sparse-checkout changes.
func (store FSStore) AcquireLock() (*Lock, error) {
	if err := store.Ensure(); err != nil {
		return nil, fmt.Errorf("ensure state directory: %w", err)
	}

	lockFile, err := os.OpenFile(store.lockPath(), os.O_CREATE|os.O_RDWR, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	fileDescriptor, err := flockDescriptor(lockFile)
	if err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("read lock file descriptor: %w", err)
	}

	if err := syscall.Flock(fileDescriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrLockHeld
		}
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}

	if err := lockFile.Truncate(0); err != nil {
		return nil, errors.Join(
			fmt.Errorf("truncate lock file: %w", err),
			releaseLockFile(lockFile),
		)
	}
	if _, err := lockFile.Seek(0, 0); err != nil {
		return nil, errors.Join(
			fmt.Errorf("seek lock file: %w", err),
			releaseLockFile(lockFile),
		)
	}
	if _, err := fmt.Fprintf(lockFile, "%d\n", os.Getpid()); err != nil {
		return nil, errors.Join(
			fmt.Errorf("write lock file metadata: %w", err),
			releaseLockFile(lockFile),
		)
	}
	if err := lockFile.Sync(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("sync lock file metadata: %w", err),
			releaseLockFile(lockFile),
		)
	}

	return &Lock{file: lockFile}, nil
}

// Release unlocks and closes the held lock.
func (lock *Lock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}

	releaseErr := releaseLockFile(lock.file)
	lock.file = nil

	return releaseErr
}

func flockDescriptor(lockFile *os.File) (int, error) {
	fileDescriptor := lockFile.Fd()
	const maxInt = int(^uint(0) >> 1)
	if fileDescriptor > uintptr(maxInt) {
		return 0, errors.New("lock file descriptor exceeds int range")
	}

	return int(fileDescriptor), nil
}

func releaseLockFile(lockFile *os.File) error {
	if lockFile == nil {
		return nil
	}

	fileDescriptor, err := flockDescriptor(lockFile)
	if err != nil {
		return errors.Join(err, lockFile.Close())
	}

	unlockErr := syscall.Flock(fileDescriptor, syscall.LOCK_UN)
	closeErr := lockFile.Close()

	if unlockErr != nil || closeErr != nil {
		return errors.Join(unlockErr, closeErr)
	}

	return nil
}
