package harness

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// TemplatePublishInput is the high-level author workflow input for harness template publish.
type TemplatePublishInput struct {
	RepoRoot                 string
	TargetBranch             string
	DefaultTemplate          bool
	Push                     bool
	Remote                   string
	SourceBranchPushPrompter orbittemplate.SourceBranchPushPrompter
}

// TemplatePublishPreview contains the resolved local publish plan.
type TemplatePublishPreview struct {
	RepoRoot        string
	HarnessID       string
	SourceBranch    string
	PublishBranch   string
	DefaultTemplate bool
	SavePreview     TemplateSavePreview
}

// TemplatePublishResult contains the preview plus the local publish outcome.
type TemplatePublishResult struct {
	Preview      TemplatePublishPreview
	LocalSuccess bool
	Changed      bool
	Commit       string
	RemotePush   TemplatePublishRemoteResult
}

// TemplatePublishRemoteResult reports the remote side of one publish execution.
type TemplatePublishRemoteResult struct {
	Attempted          bool
	Success            bool
	Remote             string
	Reason             string
	SourceBranchStatus orbittemplate.SourceBranchStatus
	NextActions        []string
}

// TemplatePublishError reports a publish flow that produced a local result but failed later.
type TemplatePublishError struct {
	Result TemplatePublishResult
	Err    error
}

func (err *TemplatePublishError) Error() string {
	return err.Err.Error()
}

func (err *TemplatePublishError) Unwrap() error {
	return err.Err
}

type templateLocalPublishResult struct {
	Changed bool
	Commit  string
}

// BuildTemplatePublishPreview validates runtime preconditions and constructs the fixed local publish plan.
func BuildTemplatePublishPreview(ctx context.Context, input TemplatePublishInput) (TemplatePublishPreview, error) {
	state, err := orbittemplate.LoadCurrentRepoState(ctx, input.RepoRoot)
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("load current repo state: %w", err)
	}
	currentBranch, err := orbittemplate.RequireCurrentBranch(state, "harness template publish")
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("require current branch: %w", err)
	}
	if state.Kind != ManifestKindRuntime {
		return TemplatePublishPreview{}, fmt.Errorf("publish requires a runtime revision; current revision kind is %q", state.Kind)
	}

	preview, err := BuildTemplateSavePreview(ctx, TemplateSavePreviewInput{
		RepoRoot:        input.RepoRoot,
		TargetBranch:    input.TargetBranch,
		DefaultTemplate: input.DefaultTemplate,
	})
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("build publish preview: %w", err)
	}

	return TemplatePublishPreview{
		RepoRoot:        input.RepoRoot,
		HarnessID:       preview.HarnessID,
		SourceBranch:    currentBranch,
		PublishBranch:   preview.TargetBranch,
		DefaultTemplate: preview.Manifest.Template.DefaultTemplate,
		SavePreview:     preview,
	}, nil
}

// PublishTemplate performs the local publish path and optional remote push.
func PublishTemplate(ctx context.Context, input TemplatePublishInput) (TemplatePublishResult, error) {
	preview, err := BuildTemplatePublishPreview(ctx, input)
	if err != nil {
		return TemplatePublishResult{}, err
	}
	result := TemplatePublishResult{
		Preview: preview,
	}

	if !input.Push {
		localResult, err := publishTemplateLocally(ctx, preview, "")
		if err != nil {
			return TemplatePublishResult{}, err
		}
		result.LocalSuccess = true
		result.Changed = localResult.Changed
		result.Commit = localResult.Commit
		return result, nil
	}

	remote := strings.TrimSpace(input.Remote)
	if remote == "" {
		remote = "origin"
	}
	result.RemotePush.Remote = remote

	relation, err := gitpkg.CompareBranchToRemoteBranchFullHistory(ctx, preview.RepoRoot, remote, preview.SourceBranch)
	if err != nil {
		result.RemotePush.Reason = "remote_source_branch_unavailable"
		return result, &TemplatePublishError{
			Result: result,
			Err:    fmt.Errorf("check remote runtime branch %q on %q: %w", preview.SourceBranch, remote, err),
		}
	}
	sourceBranchResult, err := orbittemplate.PrepareSourceBranchForPush(ctx, orbittemplate.SourceBranchPushInput{
		RepoRoot:     preview.RepoRoot,
		Remote:       remote,
		SourceBranch: preview.SourceBranch,
		Relation:     relation,
		Prompter:     input.SourceBranchPushPrompter,
	})
	result.RemotePush.SourceBranchStatus = sourceBranchResult.Status
	result.RemotePush.Reason = sourceBranchResult.Reason
	result.RemotePush.NextActions = sourceBranchResult.NextActions
	if err != nil {
		return result, &TemplatePublishError{
			Result: result,
			Err:    err,
		}
	}

	head, remoteExists, err := gitpkg.ResolveRemoteBranchHead(ctx, preview.RepoRoot, remote, preview.PublishBranch)
	if err != nil {
		result.RemotePush.Reason = "remote_publish_branch_unavailable"
		return result, &TemplatePublishError{
			Result: result,
			Err:    fmt.Errorf("check remote harness template branch %q on %q: %w", preview.PublishBranch, remote, err),
		}
	}
	if remoteExists {
		err = gitpkg.WithFetchedRemoteRefFullHistory(ctx, preview.RepoRoot, remote, head.Ref, func(tempRef string) error {
			remoteCommit, resolveErr := gitpkg.ResolveRevision(ctx, preview.RepoRoot, tempRef)
			if resolveErr != nil {
				return fmt.Errorf("resolve fetched remote harness template branch %q: %w", head.Ref, resolveErr)
			}

			noop, compareErr := templatePublishMatchesRevision(ctx, preview, tempRef)
			if compareErr != nil {
				return compareErr
			}
			if noop {
				if syncErr := syncLocalPublishBranchToCommit(ctx, preview.RepoRoot, preview.PublishBranch, remoteCommit); syncErr != nil {
					return syncErr
				}
				return nil
			}

			localResult, publishErr := publishTemplateLocally(ctx, preview, remoteCommit)
			if publishErr != nil {
				return publishErr
			}
			result.Changed = localResult.Changed
			result.Commit = localResult.Commit
			return nil
		})
		if err != nil {
			result.RemotePush.Reason = "remote_publish_branch_unavailable"
			return result, &TemplatePublishError{
				Result: result,
				Err:    fmt.Errorf("check remote harness template branch %q on %q: %w", preview.PublishBranch, remote, err),
			}
		}
		result.LocalSuccess = true
	} else {
		localResult, publishErr := publishTemplateLocally(ctx, preview, "")
		if publishErr != nil {
			return TemplatePublishResult{}, publishErr
		}
		result.LocalSuccess = true
		result.Changed = localResult.Changed
		result.Commit = localResult.Commit
	}

	result.RemotePush.Attempted = true
	if err := gitpkg.PushBranch(ctx, preview.RepoRoot, remote, preview.PublishBranch); err != nil {
		result.RemotePush.Reason = "push_failed"
		return result, &TemplatePublishError{
			Result: result,
			Err:    fmt.Errorf("push published branch %q to %q: %w", preview.PublishBranch, remote, err),
		}
	}
	result.RemotePush.Success = true

	return result, nil
}

func publishTemplateLocally(ctx context.Context, preview TemplatePublishPreview, parentCommit string) (templateLocalPublishResult, error) {
	if strings.TrimSpace(parentCommit) == "" {
		noop, err := isTemplatePublishNoOp(ctx, preview)
		if err != nil {
			return templateLocalPublishResult{}, fmt.Errorf("compare existing publish branch: %w", err)
		}
		if noop {
			return templateLocalPublishResult{}, nil
		}
	}

	saveResult, err := SaveTemplateBranch(ctx, TemplateSaveInput{
		Preview: TemplateSavePreviewInput{
			RepoRoot:        preview.RepoRoot,
			TargetBranch:    preview.PublishBranch,
			DefaultTemplate: preview.DefaultTemplate,
		},
		Overwrite:    true,
		ParentCommit: parentCommit,
	})
	if err != nil {
		return templateLocalPublishResult{}, fmt.Errorf("publish harness template branch: %w", err)
	}

	return templateLocalPublishResult{
		Changed: true,
		Commit:  saveResult.WriteResult.Commit,
	}, nil
}

func isTemplatePublishNoOp(ctx context.Context, preview TemplatePublishPreview) (bool, error) {
	exists, err := gitpkg.LocalBranchExists(ctx, preview.RepoRoot, preview.PublishBranch)
	if err != nil {
		return false, fmt.Errorf("check target publish branch %q: %w", preview.PublishBranch, err)
	}
	if !exists {
		return false, nil
	}

	return templatePublishMatchesRevision(ctx, preview, preview.PublishBranch)
}

func templatePublishMatchesRevision(ctx context.Context, preview TemplatePublishPreview, rev string) (bool, error) {
	if !templateFilesMatchRevision(ctx, preview.RepoRoot, rev, preview.SavePreview) {
		return false, nil
	}

	templateManifestValid := false
	templateManifestData, readTemplateManifestErr := gitpkg.ReadFileAtRev(ctx, preview.RepoRoot, rev, TemplateRepoPath())
	if readTemplateManifestErr == nil {
		actualTemplateManifest, parseTemplateManifestErr := ParseTemplateManifestData(templateManifestData)
		if parseTemplateManifestErr == nil {
			templateManifestValid = templateManifestEquivalent(preview.SavePreview.Manifest, actualTemplateManifest)
		}
	}
	if !templateManifestValid {
		return false, nil
	}

	branchManifestValid := false
	branchManifestData, readBranchManifestErr := gitpkg.ReadFileAtRev(ctx, preview.RepoRoot, rev, ManifestRepoPath())
	if readBranchManifestErr == nil {
		actualBranchManifest, parseBranchManifestErr := ParseManifestFileData(branchManifestData)
		if parseBranchManifestErr == nil {
			expectedBranchManifest := buildTemplateBranchManifest(preview.SavePreview)
			branchManifestValid = branchManifestEquivalent(expectedBranchManifest, actualBranchManifest)
		}
	}
	if !branchManifestValid {
		return false, nil
	}

	return true, nil
}

func templateFilesMatchRevision(ctx context.Context, repoRoot string, rev string, preview TemplateSavePreview) bool {
	paths, err := gitpkg.ListAllFilesAtRev(ctx, repoRoot, rev)
	if err != nil {
		return false
	}

	expected := make(map[string]gitpkg.TemplateTreeFile, len(preview.MemberSnapshotFiles)+len(preview.Files))
	for _, file := range preview.MemberSnapshotFiles {
		expected[file.Path] = gitpkg.TemplateTreeFile{
			Path:    file.Path,
			Content: file.Content,
			Mode:    file.Mode,
		}
	}
	for _, file := range preview.Files {
		expected[file.Path] = gitpkg.TemplateTreeFile{
			Path:    file.Path,
			Content: file.Content,
			Mode:    file.Mode,
		}
	}

	actualCount := 0
	for _, path := range paths {
		if path == ManifestRepoPath() || path == TemplateRepoPath() {
			continue
		}
		actualCount++

		expectedFile, ok := expected[path]
		if !ok {
			return false
		}

		content, err := gitpkg.ReadFileAtRev(ctx, repoRoot, rev, path)
		if err != nil {
			return false
		}
		if !bytes.Equal(expectedFile.Content, content) {
			return false
		}

		mode, err := gitpkg.FileModeAtRev(ctx, repoRoot, rev, path)
		if err != nil {
			return false
		}
		if expectedFile.Mode != mode {
			return false
		}
	}

	return actualCount == len(expected)
}

func buildTemplateBranchManifest(preview TemplateSavePreview) ManifestFile {
	return ManifestFile{
		SchemaVersion: manifestSchemaVersion,
		Kind:          ManifestKindHarnessTemplate,
		Template: &ManifestTemplateMetadata{
			Package:           ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: preview.Manifest.Template.HarnessID},
			HarnessID:         preview.Manifest.Template.HarnessID,
			DefaultTemplate:   preview.Manifest.Template.DefaultTemplate,
			CreatedFromBranch: preview.Manifest.Template.CreatedFromBranch,
			CreatedFromCommit: preview.Manifest.Template.CreatedFromCommit,
			CreatedAt:         preview.Manifest.Template.CreatedAt,
		},
		Members:            manifestMembersFromTemplateMembers(preview.Manifest.Members),
		IncludesRootAgents: preview.Manifest.Template.IncludesRootAgents,
	}
}

func templateManifestEquivalent(expected TemplateManifest, actual TemplateManifest) bool {
	return actual.SchemaVersion == templateSchemaVersion &&
		actual.Kind == TemplateKind &&
		actual.Template.HarnessID == expected.Template.HarnessID &&
		actual.Template.DefaultTemplate == expected.Template.DefaultTemplate &&
		actual.Template.CreatedFromBranch == expected.Template.CreatedFromBranch &&
		actual.Template.CreatedFromCommit == expected.Template.CreatedFromCommit &&
		actual.Template.IncludesRootAgents == expected.Template.IncludesRootAgents &&
		!actual.Template.CreatedAt.IsZero() &&
		reflect.DeepEqual(actual.Members, expected.Members) &&
		reflect.DeepEqual(actual.Variables, expected.Variables)
}

func branchManifestEquivalent(expected ManifestFile, actual ManifestFile) bool {
	return actual.SchemaVersion == manifestSchemaVersion &&
		actual.Kind == ManifestKindHarnessTemplate &&
		actual.Template != nil &&
		expected.Template != nil &&
		actual.Template.HarnessID == expected.Template.HarnessID &&
		actual.Template.DefaultTemplate == expected.Template.DefaultTemplate &&
		actual.Template.CreatedFromBranch == expected.Template.CreatedFromBranch &&
		actual.Template.CreatedFromCommit == expected.Template.CreatedFromCommit &&
		!actual.Template.CreatedAt.IsZero() &&
		actual.IncludesRootAgents == expected.IncludesRootAgents &&
		reflect.DeepEqual(actual.Members, expected.Members)
}

func syncLocalPublishBranchToCommit(ctx context.Context, repoRoot string, branch string, commit string) error {
	refName := "refs/heads/" + strings.TrimSpace(branch)
	if err := gitpkg.UpdateRef(ctx, repoRoot, refName, commit); err != nil {
		return fmt.Errorf("sync local publish branch %q to %s: %w", branch, commit, err)
	}

	return nil
}
