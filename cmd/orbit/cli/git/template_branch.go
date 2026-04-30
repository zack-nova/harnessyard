package git

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const (
	localHeadsPrefix             = "refs/heads"
	templateIndexEnvName         = "GIT_INDEX_FILE"
	templateIndexFilePattern     = "orbit-template-index-*"
	defaultTemplateCommitMessage = "orbit template save"
)

// TemplateTreeFile is one repo-relative file that should appear in the saved template branch.
type TemplateTreeFile struct {
	Path    string
	Content []byte
	Mode    string
}

// WriteTemplateBranchInput is the Git-side contract for writing one template branch.
type WriteTemplateBranchInput struct {
	Branch             string
	AllowCurrentBranch bool
	Overwrite          bool
	ParentCommit       string
	Message            string
	ManifestPath       string
	Manifest           []byte
	Files              []TemplateTreeFile
}

// WriteTemplateBranchResult summarizes the written template ref.
type WriteTemplateBranchResult struct {
	Branch string
	Ref    string
	Commit string
}

// TemplateTargetBranchExistsError reports that the requested target branch already exists.
type TemplateTargetBranchExistsError struct {
	Branch string
}

func (err *TemplateTargetBranchExistsError) Error() string {
	return fmt.Sprintf("target branch %q already exists; re-run with --overwrite to replace it", err.Branch)
}

// CurrentBranch returns the current symbolic local branch name, or HEAD when detached.
func CurrentBranch(ctx context.Context, repoRoot string) (string, error) {
	output, err := runGit(ctx, repoRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	output, err = runGit(ctx, repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// LocalBranchExists reports whether refs/heads/<branch> already exists.
func LocalBranchExists(ctx context.Context, repoRoot string, branch string) (bool, error) {
	refName, normalizedBranch, err := localBranchRef(ctx, repoRoot, branch)
	if err != nil {
		return false, err
	}

	exists, err := RevisionExists(ctx, repoRoot, refName)
	if err != nil {
		return false, fmt.Errorf("check target branch %q: %w", normalizedBranch, err)
	}

	return exists, nil
}

// DeleteLocalBranch removes refs/heads/<branch> without switching the current worktree.
func DeleteLocalBranch(ctx context.Context, repoRoot string, branch string) error {
	refName, normalizedBranch, err := localBranchRef(ctx, repoRoot, branch)
	if err != nil {
		return err
	}

	exists, err := RevisionExists(ctx, repoRoot, refName)
	if err != nil {
		return fmt.Errorf("check target branch %q: %w", normalizedBranch, err)
	}
	if !exists {
		return nil
	}

	if _, err := runGit(ctx, repoRoot, "update-ref", "-d", refName); err != nil {
		return fmt.Errorf("delete target branch %q: %w", normalizedBranch, err)
	}

	return nil
}

// WriteTemplateBranch writes a template-only commit to the requested local branch
// without switching branches or mutating the current worktree.
func WriteTemplateBranch(ctx context.Context, repoRoot string, input WriteTemplateBranchInput) (WriteTemplateBranchResult, error) {
	refName, normalizedBranch, err := localBranchRef(ctx, repoRoot, input.Branch)
	if err != nil {
		return WriteTemplateBranchResult{}, err
	}

	currentBranch, err := CurrentBranch(ctx, repoRoot)
	if err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("resolve current branch: %w", err)
	}
	if currentBranch == normalizedBranch && !input.AllowCurrentBranch {
		return WriteTemplateBranchResult{}, fmt.Errorf("target branch must not be the current branch %q", normalizedBranch)
	}

	files, err := normalizeTemplateTree(input.Files, input.ManifestPath, input.Manifest)
	if err != nil {
		return WriteTemplateBranchResult{}, err
	}

	branchExists, err := LocalBranchExists(ctx, repoRoot, normalizedBranch)
	if err != nil {
		return WriteTemplateBranchResult{}, err
	}
	if branchExists && !input.Overwrite {
		return WriteTemplateBranchResult{}, &TemplateTargetBranchExistsError{Branch: normalizedBranch}
	}

	message := strings.TrimSpace(input.Message)
	if message == "" {
		message = defaultTemplateCommitMessage
	}

	indexFile, err := os.CreateTemp("", templateIndexFilePattern)
	if err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("create template temp index: %w", err)
	}
	indexPath := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		_ = os.Remove(indexPath)
		return WriteTemplateBranchResult{}, fmt.Errorf("close template temp index: %w", err)
	}
	defer func() {
		_ = os.Remove(indexPath)
	}()

	extraEnv := map[string]string{
		templateIndexEnvName: indexPath,
	}
	if _, err := runGitInputEnv(ctx, repoRoot, extraEnv, nil, "read-tree", "--empty"); err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("initialize template temp index: %w", err)
	}

	for _, file := range files {
		blobID, err := hashObject(ctx, repoRoot, file.Content)
		if err != nil {
			return WriteTemplateBranchResult{}, fmt.Errorf("hash template file %s: %w", file.Path, err)
		}

		if _, err := runGitInputEnv(
			ctx,
			repoRoot,
			extraEnv,
			nil,
			"update-index",
			"--add",
			"--cacheinfo",
			file.Mode,
			blobID,
			file.Path,
		); err != nil {
			return WriteTemplateBranchResult{}, fmt.Errorf("stage template file %s in temp index: %w", file.Path, err)
		}
	}

	treeIDRaw, err := runGitInputEnv(ctx, repoRoot, extraEnv, nil, "write-tree")
	if err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("write template tree: %w", err)
	}
	treeID := strings.TrimSpace(string(treeIDRaw))
	if treeID == "" {
		return WriteTemplateBranchResult{}, fmt.Errorf("write template tree: git write-tree returned empty tree id")
	}

	commitArgs := []string{"commit-tree", treeID, "-m", message}
	parentCommit := strings.TrimSpace(input.ParentCommit)
	if parentCommit != "" {
		parentCommit, err = resolveCommit(ctx, repoRoot, parentCommit)
		if err != nil {
			return WriteTemplateBranchResult{}, fmt.Errorf("resolve explicit parent commit for target branch %q: %w", normalizedBranch, err)
		}
		commitArgs = append(commitArgs, "-p", parentCommit)
	} else if branchExists {
		parentCommit, err = resolveCommit(ctx, repoRoot, refName)
		if err != nil {
			return WriteTemplateBranchResult{}, fmt.Errorf("resolve existing target branch %q: %w", normalizedBranch, err)
		}
		commitArgs = append(commitArgs, "-p", parentCommit)
	}

	commitRaw, err := runGit(ctx, repoRoot, commitArgs...)
	if err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("create template commit: %w", err)
	}
	commitID := strings.TrimSpace(string(commitRaw))
	if commitID == "" {
		return WriteTemplateBranchResult{}, fmt.Errorf("create template commit: git commit-tree returned empty commit id")
	}

	if err := UpdateRef(ctx, repoRoot, refName, commitID); err != nil {
		return WriteTemplateBranchResult{}, fmt.Errorf("update target branch %q: %w", normalizedBranch, err)
	}
	if currentBranch == normalizedBranch {
		if _, err := runGit(ctx, repoRoot, "reset", "--hard", commitID); err != nil {
			return WriteTemplateBranchResult{}, fmt.Errorf("sync current branch %q worktree to published commit: %w", normalizedBranch, err)
		}
	}

	return WriteTemplateBranchResult{
		Branch: normalizedBranch,
		Ref:    refName,
		Commit: commitID,
	}, nil
}

func localBranchRef(ctx context.Context, repoRoot string, branch string) (string, string, error) {
	normalizedBranch, err := normalizeLocalBranchName(ctx, repoRoot, branch)
	if err != nil {
		return "", "", err
	}

	return localHeadsPrefix + "/" + normalizedBranch, normalizedBranch, nil
}

func normalizeLocalBranchName(ctx context.Context, repoRoot string, branch string) (string, error) {
	trimmedBranch := strings.TrimSpace(branch)
	if trimmedBranch == "" {
		return "", fmt.Errorf("target branch must not be empty")
	}

	output, err := runGit(ctx, repoRoot, "check-ref-format", "--branch", trimmedBranch)
	if err != nil {
		return "", fmt.Errorf("validate target branch %q: %w", trimmedBranch, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func normalizeTemplateTree(files []TemplateTreeFile, manifestPath string, manifest []byte) ([]TemplateTreeFile, error) {
	if len(manifest) == 0 {
		return nil, fmt.Errorf("template manifest must not be empty")
	}

	normalizedFiles := make([]TemplateTreeFile, 0, len(files)+1)
	seen := make(map[string]struct{}, len(files)+1)

	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return nil, fmt.Errorf("template branch manifest path must not be empty")
	}

	normalizedManifestPath, err := ids.NormalizeRepoRelativePath(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("normalize template manifest path: %w", err)
	}
	seen[normalizedManifestPath] = struct{}{}
	normalizedFiles = append(normalizedFiles, TemplateTreeFile{
		Path:    normalizedManifestPath,
		Content: append([]byte(nil), manifest...),
		Mode:    FileModeRegular,
	})

	for _, file := range files {
		normalizedPath, err := ids.NormalizeRepoRelativePath(file.Path)
		if err != nil {
			return nil, fmt.Errorf("normalize template file %q: %w", file.Path, err)
		}
		if _, exists := seen[normalizedPath]; exists {
			return nil, fmt.Errorf("template file %q appears more than once", normalizedPath)
		}
		normalizedMode, err := normalizeFileMode(file.Mode)
		if err != nil {
			return nil, fmt.Errorf("normalize template file mode for %s: %w", normalizedPath, err)
		}
		seen[normalizedPath] = struct{}{}
		normalizedFiles = append(normalizedFiles, TemplateTreeFile{
			Path:    normalizedPath,
			Content: append([]byte(nil), file.Content...),
			Mode:    normalizedMode,
		})
	}

	return normalizedFiles, nil
}

func hashObject(ctx context.Context, repoRoot string, content []byte) (string, error) {
	output, err := runGitInput(ctx, repoRoot, content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func resolveCommit(ctx context.Context, repoRoot string, revision string) (string, error) {
	output, err := runGit(ctx, repoRoot, "rev-parse", revision)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
