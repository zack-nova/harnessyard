package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// BranchRelation captures the ancestry relationship between a local branch and its remote counterpart.
type BranchRelation string

const (
	BranchRelationEqual    BranchRelation = "equal"
	BranchRelationAhead    BranchRelation = "ahead"
	BranchRelationBehind   BranchRelation = "behind"
	BranchRelationDiverged BranchRelation = "diverged"
	BranchRelationMissing  BranchRelation = "missing"
)

// CompareBranchToRemoteBranch compares one local branch to its remote counterpart.
func CompareBranchToRemoteBranch(ctx context.Context, repoRoot string, remote string, branch string) (BranchRelation, error) {
	return compareBranchToRemoteBranch(ctx, repoRoot, remote, branch, true)
}

// CompareBranchToRemoteBranchFullHistory compares one local branch to its remote counterpart with full remote parent history.
func CompareBranchToRemoteBranchFullHistory(ctx context.Context, repoRoot string, remote string, branch string) (BranchRelation, error) {
	return compareBranchToRemoteBranch(ctx, repoRoot, remote, branch, false)
}

func compareBranchToRemoteBranch(ctx context.Context, repoRoot string, remote string, branch string, shallow bool) (BranchRelation, error) {
	trimmedRemote := strings.TrimSpace(remote)
	if trimmedRemote == "" {
		return "", errors.New("remote must not be empty")
	}

	normalizedBranch, err := normalizeLocalBranchName(ctx, repoRoot, branch)
	if err != nil {
		return "", fmt.Errorf("validate branch %q: %w", branch, err)
	}

	localCommit, err := ResolveRevision(ctx, repoRoot, normalizedBranch)
	if err != nil {
		return "", fmt.Errorf("resolve local branch %q: %w", normalizedBranch, err)
	}

	head, exists, err := ResolveRemoteBranchHead(ctx, repoRoot, trimmedRemote, normalizedBranch)
	if err != nil {
		return "", fmt.Errorf("compare %s against %s/%s: %w", normalizedBranch, trimmedRemote, normalizedBranch, err)
	}
	if !exists {
		return BranchRelationMissing, nil
	}

	var relation BranchRelation
	fetchRemoteRef := WithFetchedRemoteRef
	if !shallow {
		fetchRemoteRef = WithFetchedRemoteRefFullHistory
	}
	if err := fetchRemoteRef(ctx, repoRoot, trimmedRemote, head.Ref, func(tempRef string) error {
		remoteCommit, err := ResolveRevision(ctx, repoRoot, tempRef)
		if err != nil {
			return fmt.Errorf("resolve fetched remote branch %q: %w", head.Ref, err)
		}

		if localCommit == remoteCommit {
			relation = BranchRelationEqual
			return nil
		}

		remoteAncestorOfLocal, err := isAncestor(ctx, repoRoot, remoteCommit, localCommit)
		if err != nil {
			return fmt.Errorf("compare remote ancestry: %w", err)
		}
		if remoteAncestorOfLocal {
			relation = BranchRelationAhead
			return nil
		}

		localAncestorOfRemote, err := isAncestor(ctx, repoRoot, localCommit, remoteCommit)
		if err != nil {
			return fmt.Errorf("compare local ancestry: %w", err)
		}
		if localAncestorOfRemote {
			relation = BranchRelationBehind
			return nil
		}

		relation = BranchRelationDiverged
		return nil
	}); err != nil {
		return "", fmt.Errorf("compare %s against %s/%s: %w", normalizedBranch, trimmedRemote, normalizedBranch, err)
	}

	return relation, nil
}

// PushBranch performs a normal push of one local branch to the same branch name on the remote.
func PushBranch(ctx context.Context, repoRoot string, remote string, branch string) error {
	trimmedRemote := strings.TrimSpace(remote)
	if trimmedRemote == "" {
		return errors.New("remote must not be empty")
	}

	normalizedBranch, err := normalizeLocalBranchName(ctx, repoRoot, branch)
	if err != nil {
		return fmt.Errorf("validate branch %q: %w", branch, err)
	}

	if _, err := runGit(ctx, repoRoot, "push", trimmedRemote, localHeadsPrefix+"/"+normalizedBranch+":"+localHeadsPrefix+"/"+normalizedBranch); err != nil {
		return fmt.Errorf("push %s to %s: %w", normalizedBranch, trimmedRemote, err)
	}

	return nil
}

// PushBranchSetUpstream pushes one local branch to the same branch name on the remote and records upstream tracking.
func PushBranchSetUpstream(ctx context.Context, repoRoot string, remote string, branch string) error {
	trimmedRemote := strings.TrimSpace(remote)
	if trimmedRemote == "" {
		return errors.New("remote must not be empty")
	}

	normalizedBranch, err := normalizeLocalBranchName(ctx, repoRoot, branch)
	if err != nil {
		return fmt.Errorf("validate branch %q: %w", branch, err)
	}

	if _, err := runGit(ctx, repoRoot, "push", "--set-upstream", trimmedRemote, localHeadsPrefix+"/"+normalizedBranch+":"+localHeadsPrefix+"/"+normalizedBranch); err != nil {
		return fmt.Errorf("push %s to %s with upstream: %w", normalizedBranch, trimmedRemote, err)
	}

	return nil
}

func isAncestor(ctx context.Context, repoRoot string, ancestor string, descendant string) (bool, error) {
	commandArgs := []string{"-C", repoRoot, "merge-base", "--is-ancestor", ancestor, descendant}
	//nolint:gosec // Git is invoked with explicit argument lists from internal callers only.
	cmd := exec.CommandContext(ctx, "git", commandArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}

		stderrText := strings.TrimSpace(stderr.String())
		if stderrText != "" {
			return false, fmt.Errorf("run git merge-base --is-ancestor %s %s: %w: %s", ancestor, descendant, err, stderrText)
		}

		return false, fmt.Errorf("run git merge-base --is-ancestor %s %s: %w", ancestor, descendant, err)
	}

	return true, nil
}
