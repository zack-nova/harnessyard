package git

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const remoteTempRefPrefix = "refs/orbits/tmp/remote-source"

// RemoteHead is one remote branch head discovered via ls-remote.
type RemoteHead struct {
	Name   string
	Ref    string
	Commit string
}

// ResolveRemoteBranchHead resolves one named remote branch head without fetching its full tree.
func ResolveRemoteBranchHead(ctx context.Context, repoRoot string, remoteURL string, branch string) (RemoteHead, bool, error) {
	trimmedBranch := strings.TrimSpace(branch)
	if trimmedBranch == "" {
		return RemoteHead{}, false, errors.New("remote branch must not be empty")
	}

	heads, err := ListRemoteHeads(ctx, repoRoot, remoteURL)
	if err != nil {
		return RemoteHead{}, false, err
	}
	for _, head := range heads {
		if head.Name == trimmedBranch || head.Ref == "refs/heads/"+trimmedBranch {
			return head, true, nil
		}
	}

	return RemoteHead{}, false, nil
}

// ListRemoteHeads enumerates remote branch heads in stable refname order.
func ListRemoteHeads(ctx context.Context, repoRoot string, remoteURL string) ([]RemoteHead, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	if trimmedURL == "" {
		return nil, errors.New("remote URL must not be empty")
	}

	output, err := runGit(ctx, repoRoot, "ls-remote", "--heads", trimmedURL)
	if err != nil {
		return nil, fmt.Errorf("git ls-remote --heads %s: %w", trimmedURL, err)
	}

	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		return nil, nil
	}

	lines := strings.Split(trimmedOutput, "\n")
	heads := make([]RemoteHead, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			return nil, fmt.Errorf("parse ls-remote output line %q", line)
		}

		refName := fields[1]
		if !strings.HasPrefix(refName, "refs/heads/") {
			continue
		}

		heads = append(heads, RemoteHead{
			Name:   strings.TrimPrefix(refName, "refs/heads/"),
			Ref:    refName,
			Commit: fields[0],
		})
	}

	sort.Slice(heads, func(left int, right int) bool {
		return heads[left].Ref < heads[right].Ref
	})

	return heads, nil
}

// ResolveRemoteDefaultBranch resolves the remote HEAD symbolic ref to one branch head.
func ResolveRemoteDefaultBranch(ctx context.Context, repoRoot string, remoteURL string) (RemoteHead, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	if trimmedURL == "" {
		return RemoteHead{}, errors.New("remote URL must not be empty")
	}

	output, err := runGit(ctx, repoRoot, "ls-remote", "--symref", trimmedURL, "HEAD")
	if err != nil {
		return RemoteHead{}, fmt.Errorf("git ls-remote --symref %s HEAD: %w", trimmedURL, err)
	}

	trimmedOutput := strings.TrimSpace(string(output))
	if trimmedOutput == "" {
		return RemoteHead{}, errors.New("remote default branch is not available")
	}

	var headRef string
	var headCommit string
	for _, line := range strings.Split(trimmedOutput, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "\t")
		if len(parts) != 2 || parts[1] != "HEAD" {
			continue
		}
		switch {
		case strings.HasPrefix(parts[0], "ref: "):
			headRef = strings.TrimPrefix(parts[0], "ref: ")
		default:
			headCommit = parts[0]
		}
	}

	if headRef == "" {
		return RemoteHead{}, errors.New("remote default branch is not available")
	}
	if !strings.HasPrefix(headRef, "refs/heads/") {
		return RemoteHead{}, fmt.Errorf("remote HEAD must resolve to refs/heads/*, got %q", headRef)
	}

	return RemoteHead{
		Name:   strings.TrimPrefix(headRef, "refs/heads/"),
		Ref:    headRef,
		Commit: headCommit,
	}, nil
}

// ReadFileAtRemoteRef fetches one remote ref into a temporary local ref, reads a file, and cleans up.
func ReadFileAtRemoteRef(ctx context.Context, repoRoot string, remoteURL string, remoteRef string, path string) ([]byte, error) {
	var data []byte
	if err := WithFetchedRemoteRef(ctx, repoRoot, remoteURL, remoteRef, func(tempRef string) error {
		exists, err := PathExistsAtRev(ctx, repoRoot, tempRef, path)
		if err != nil {
			return fmt.Errorf("check %s at remote ref %q: %w", path, remoteRef, err)
		}
		if !exists {
			return fmt.Errorf("read %s at remote ref %q: %w", path, remoteRef, os.ErrNotExist)
		}

		data, err = ReadFileAtRev(ctx, repoRoot, tempRef, path)
		if err != nil {
			return fmt.Errorf("read %s at remote ref %q: %w", path, remoteRef, err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return data, nil
}

// WithFetchedRemoteRef fetches one remote ref into a temporary local ref and guarantees cleanup.
func WithFetchedRemoteRef(ctx context.Context, repoRoot string, remoteURL string, remoteRef string, fn func(tempRef string) error) error {
	return withFetchedRemoteRef(ctx, repoRoot, remoteURL, remoteRef, true, fn)
}

// WithFetchedRemoteRefFullHistory fetches one remote ref into a temporary local ref with its full parent chain.
// Use this only when the fetched commit will become part of a local long-lived branch history.
func WithFetchedRemoteRefFullHistory(ctx context.Context, repoRoot string, remoteURL string, remoteRef string, fn func(tempRef string) error) error {
	return withFetchedRemoteRef(ctx, repoRoot, remoteURL, remoteRef, false, fn)
}

func withFetchedRemoteRef(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	remoteRef string,
	shallow bool,
	fn func(tempRef string) error,
) error {
	trimmedURL := strings.TrimSpace(remoteURL)
	if trimmedURL == "" {
		return errors.New("remote URL must not be empty")
	}
	trimmedRef := strings.TrimSpace(remoteRef)
	if trimmedRef == "" {
		return errors.New("remote ref must not be empty")
	}

	tempRef, err := fetchRemoteSpecToTempRef(ctx, repoRoot, trimmedURL, trimmedRef, shallow)
	if err != nil {
		return fmt.Errorf("fetch remote ref %q from %s: %w", trimmedRef, trimmedURL, err)
	}

	return withFetchedTempRef(ctx, repoRoot, tempRef, fn)
}

// WithFetchedRemoteRevisionOrRef first tries to fetch the recorded revision directly and falls back
// to the recorded ref when the direct fetch transport is unavailable.
func WithFetchedRemoteRevisionOrRef(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	remoteRevision string,
	remoteRef string,
	fn func(tempRef string) error,
) error {
	trimmedURL := strings.TrimSpace(remoteURL)
	if trimmedURL == "" {
		return errors.New("remote URL must not be empty")
	}
	trimmedRevision := strings.TrimSpace(remoteRevision)
	trimmedRef := strings.TrimSpace(remoteRef)
	if trimmedRevision == "" && trimmedRef == "" {
		return errors.New("remote revision or ref must not be empty")
	}

	var revisionErr error
	if trimmedRevision != "" {
		tempRef, err := fetchRemoteSpecToTempRef(ctx, repoRoot, trimmedURL, trimmedRevision, true)
		if err == nil {
			return withFetchedTempRef(ctx, repoRoot, tempRef, fn)
		}
		revisionErr = fmt.Errorf("fetch remote revision %q from %s: %w", trimmedRevision, trimmedURL, err)
		if trimmedRef == "" || trimmedRef == trimmedRevision {
			return revisionErr
		}
	}

	if trimmedRef == "" {
		return revisionErr
	}

	tempRef, err := fetchRemoteSpecToTempRef(ctx, repoRoot, trimmedURL, trimmedRef, true)
	if err != nil {
		refErr := fmt.Errorf("fetch remote ref %q from %s: %w", trimmedRef, trimmedURL, err)
		if revisionErr != nil {
			return errors.Join(revisionErr, refErr)
		}
		return refErr
	}

	return withFetchedTempRef(ctx, repoRoot, tempRef, fn)
}

func newRemoteTempRef() (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate remote temp ref suffix: %w", err)
	}

	return remoteTempRefPrefix + "/" + hex.EncodeToString(randomBytes), nil
}

func deleteRemoteTempRef(ctx context.Context, repoRoot string, tempRef string) error {
	if _, err := runGit(ctx, repoRoot, "update-ref", "-d", tempRef); err != nil {
		return fmt.Errorf("delete temp remote ref %q: %w", tempRef, err)
	}

	return nil
}

func fetchRemoteSpecToTempRef(ctx context.Context, repoRoot string, remoteURL string, remoteSpec string, shallow bool) (string, error) {
	tempRef, err := newRemoteTempRef()
	if err != nil {
		return "", fmt.Errorf("allocate temp remote ref: %w", err)
	}

	args := []string{"fetch"}
	if shallow {
		args = append(args, "--depth=1")
	}
	args = append(args, "--no-tags", remoteURL, remoteSpec+":"+tempRef)
	if _, err := runGit(ctx, repoRoot, args...); err != nil {
		return "", err
	}

	return tempRef, nil
}

func withFetchedTempRef(ctx context.Context, repoRoot string, tempRef string, fn func(tempRef string) error) error {
	callbackErr := fn(tempRef)
	if cleanupErr := deleteRemoteTempRef(ctx, repoRoot, tempRef); cleanupErr != nil {
		if callbackErr == nil {
			return cleanupErr
		}

		return errors.Join(callbackErr, cleanupErr)
	}

	return callbackErr
}
