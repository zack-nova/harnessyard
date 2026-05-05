package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"gopkg.in/yaml.v3"
)

type TemplatePublishMode string

const (
	TemplatePublishModeSource   TemplatePublishMode = "source"
	TemplatePublishModeTemplate TemplatePublishMode = "orbit_template"
	TemplatePublishModeRuntime  TemplatePublishMode = "runtime"
)

type orbitTemplateBranchManifest struct {
	SchemaVersion int                               `yaml:"schema_version"`
	Kind          string                            `yaml:"kind"`
	Template      orbitTemplateBranchManifestSource `yaml:"template"`
	Variables     map[string]VariableSpec           `yaml:"variables"`
}

type orbitTemplateBranchManifestSource struct {
	OrbitID           string    `yaml:"orbit_id"`
	DefaultTemplate   *bool     `yaml:"default_template,omitempty"`
	CreatedFromBranch string    `yaml:"created_from_branch"`
	CreatedFromCommit string    `yaml:"created_from_commit"`
	CreatedAt         time.Time `yaml:"created_at"`
}

// TemplatePublishInput is the high-level author workflow input for local orbit template publish.
type TemplatePublishInput struct {
	RepoRoot                 string
	OrbitID                  string
	DefaultTemplate          bool
	DefaultTemplateSet       bool
	BackfillBrief            bool
	AggregateDetectedSkills  bool
	AllowOutOfRangeSkills    bool
	ConfirmPrompter          ConfirmPrompter
	SkillDetectionPrompter   ConfirmPrompter
	SourceBranchPushPrompter SourceBranchPushPrompter
	Push                     bool
	Remote                   string
	TargetBranch             string
	Progress                 func(string) error
}

// TemplatePublishPreview contains the resolved local publish plan.
type TemplatePublishPreview struct {
	RepoRoot          string
	OrbitID           string
	SourceBranch      string
	PublishBranch     string
	DefaultTemplate   bool
	Mode              TemplatePublishMode
	SavePreview       TemplateSavePreview
	TrackedAuthorOnly []string
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
	SourceBranchStatus SourceBranchStatus
	NextActions        []string
}

// PublishError reports a publish flow that produced a local result but failed later.
type PublishError struct {
	Result TemplatePublishResult
	Err    error
}

func (err *PublishError) Error() string {
	return err.Err.Error()
}

func (err *PublishError) Unwrap() error {
	return err.Err
}

// BuildTemplatePublishPreview validates source/runtime/orbit_template publish preconditions and constructs the local publish plan.
func BuildTemplatePublishPreview(ctx context.Context, input TemplatePublishInput) (TemplatePublishPreview, error) {
	state, err := LoadCurrentRepoState(ctx, input.RepoRoot)
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("load current repo state: %w", err)
	}
	switch state.Kind {
	case "harness_template":
		return TemplatePublishPreview{}, fmt.Errorf("publish requires a source, runtime, or orbit_template revision; current revision kind is %q", state.Kind)
	}
	currentBranch, err := RequireCurrentBranch(state, "publish")
	if err != nil {
		return TemplatePublishPreview{}, err
	}
	switch state.Kind {
	case "orbit_template":
		return buildOrbitTemplateGeneratedPublishPreview(ctx, input, currentBranch)
	case "runtime":
		return buildRuntimeTemplatePublishPreview(ctx, input, currentBranch)
	}

	return buildSourceTemplatePublishPreview(ctx, input, currentBranch)
}

func buildSourceTemplatePublishPreview(ctx context.Context, input TemplatePublishInput, currentBranch string) (TemplatePublishPreview, error) {
	sourceManifest, err := LoadSourceManifest(input.RepoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TemplatePublishPreview{}, fmt.Errorf("load %s: %w", sourceManifestRelativePath, err)
		}
		return TemplatePublishPreview{}, fmt.Errorf("load %s: %w", sourceManifestRelativePath, err)
	}

	if err := ensurePublishSourceContracts(ctx, input.RepoRoot, sourceManifest, currentBranch); err != nil {
		return TemplatePublishPreview{}, err
	}

	orbitID, err := resolvePublishOrbitID(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return TemplatePublishPreview{}, err
	}
	if err := EnsureMemberHintExportSync(ctx, input.RepoRoot, orbitID, "publishing"); err != nil {
		return TemplatePublishPreview{}, err
	}
	skillDetection, err := RunTemplateLocalSkillDetection(ctx, TemplateLocalSkillDetectionInput{
		RepoRoot:                input.RepoRoot,
		OrbitID:                 orbitID,
		AggregateDetectedSkills: input.AggregateDetectedSkills,
		AllowOutOfRangeSkills:   input.AllowOutOfRangeSkills,
		ConfirmPrompter:         input.SkillDetectionPrompter,
	})
	if err != nil {
		return TemplatePublishPreview{}, err
	}
	briefSync, err := EnsureBriefExportSync(ctx, input.RepoRoot, orbitID, "publishing", input.BackfillBrief)
	if err != nil {
		return TemplatePublishPreview{}, err
	}

	publishBranch := strings.TrimSpace(input.TargetBranch)
	if publishBranch == "" {
		publishBranch = fmt.Sprintf("orbit-template/%s", orbitID)
	}
	savePreview, err := BuildTemplateSavePreview(ctx, TemplateSavePreviewInput{
		RepoRoot:      input.RepoRoot,
		OrbitID:       orbitID,
		TargetBranch:  publishBranch,
		DefaultBranch: input.DefaultTemplate,
		Warnings:      append([]string(nil), skillDetection.Warnings...),
	})
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("build publish preview: %w", err)
	}
	if briefSync.Warning != "" {
		savePreview.Warnings = append(savePreview.Warnings, briefSync.Warning)
	}

	return TemplatePublishPreview{
		RepoRoot:        input.RepoRoot,
		OrbitID:         orbitID,
		SourceBranch:    sourceManifest.SourceBranch,
		PublishBranch:   publishBranch,
		DefaultTemplate: input.DefaultTemplate,
		Mode:            TemplatePublishModeSource,
		SavePreview:     savePreview,
	}, nil
}

func buildRuntimeTemplatePublishPreview(ctx context.Context, input TemplatePublishInput, currentBranch string) (TemplatePublishPreview, error) {
	if err := ensureCleanTrackedWorktree(ctx, input.RepoRoot); err != nil {
		return TemplatePublishPreview{}, err
	}

	orbitID, err := resolveRuntimePublishOrbitID(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return TemplatePublishPreview{}, err
	}
	if err := EnsureMemberHintExportSync(ctx, input.RepoRoot, orbitID, "publishing"); err != nil {
		return TemplatePublishPreview{}, err
	}
	skillDetection, err := RunTemplateLocalSkillDetection(ctx, TemplateLocalSkillDetectionInput{
		RepoRoot:                input.RepoRoot,
		OrbitID:                 orbitID,
		AggregateDetectedSkills: input.AggregateDetectedSkills,
		AllowOutOfRangeSkills:   input.AllowOutOfRangeSkills,
		ConfirmPrompter:         input.SkillDetectionPrompter,
	})
	if err != nil {
		return TemplatePublishPreview{}, err
	}
	briefSync, err := EnsureBriefExportSync(ctx, input.RepoRoot, orbitID, "publishing", input.BackfillBrief)
	if err != nil {
		return TemplatePublishPreview{}, err
	}

	publishBranch := strings.TrimSpace(input.TargetBranch)
	if publishBranch == "" {
		publishBranch = fmt.Sprintf("orbit-template/%s", orbitID)
	}
	savePreview, err := BuildTemplateSavePreview(ctx, TemplateSavePreviewInput{
		RepoRoot:      input.RepoRoot,
		OrbitID:       orbitID,
		TargetBranch:  publishBranch,
		DefaultBranch: input.DefaultTemplate,
		Warnings:      append([]string(nil), skillDetection.Warnings...),
	})
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("build publish preview: %w", err)
	}
	if briefSync.Warning != "" {
		savePreview.Warnings = append(savePreview.Warnings, briefSync.Warning)
	}

	return TemplatePublishPreview{
		RepoRoot:        input.RepoRoot,
		OrbitID:         orbitID,
		SourceBranch:    currentBranch,
		PublishBranch:   publishBranch,
		DefaultTemplate: input.DefaultTemplate,
		Mode:            TemplatePublishModeRuntime,
		SavePreview:     savePreview,
	}, nil
}

// PublishTemplate performs the local publish path only. Remote push is intentionally out of scope here.
func PublishTemplate(ctx context.Context, input TemplatePublishInput) (TemplatePublishResult, error) {
	if err := publishStage(input.Progress, "building publish preview"); err != nil {
		return TemplatePublishResult{}, err
	}
	preview, err := BuildTemplatePublishPreview(ctx, input)
	if err != nil {
		return TemplatePublishResult{}, err
	}
	result := TemplatePublishResult{
		Preview: preview,
	}

	if !input.Push {
		localResult, err := publishTemplateLocally(ctx, input, preview, "", input.Progress)
		if err != nil {
			return TemplatePublishResult{}, err
		}
		result.LocalSuccess = true
		result.Changed = localResult.Changed
		result.Commit = localResult.Commit
		result.Preview.SavePreview.Warnings = append(result.Preview.SavePreview.Warnings, localResult.Warnings...)
		if err := publishStage(input.Progress, "publish complete"); err != nil {
			return TemplatePublishResult{}, err
		}
		return result, nil
	}

	remote := strings.TrimSpace(input.Remote)
	if remote == "" {
		remote = "origin"
	}
	result.RemotePush.Remote = remote

	if err := publishStage(input.Progress, "checking remote freshness"); err != nil {
		return TemplatePublishResult{}, err
	}

	if preview.Mode == TemplatePublishModeSource || preview.Mode == TemplatePublishModeRuntime {
		relation, err := gitpkg.CompareBranchToRemoteBranchFullHistory(ctx, preview.RepoRoot, remote, preview.SourceBranch)
		if err != nil {
			result.RemotePush.Reason = "remote_source_branch_unavailable"
			return result, &PublishError{
				Result: result,
				Err:    fmt.Errorf("check remote source branch %q on %q: %w", preview.SourceBranch, remote, err),
			}
		}
		sourceBranchResult, err := PrepareSourceBranchForPush(ctx, SourceBranchPushInput{
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
			return result, &PublishError{
				Result: result,
				Err:    err,
			}
		}
		head, remoteExists, err := gitpkg.ResolveRemoteBranchHead(ctx, preview.RepoRoot, remote, preview.PublishBranch)
		if err != nil {
			result.RemotePush.Reason = "remote_publish_branch_unavailable"
			return result, &PublishError{
				Result: result,
				Err:    fmt.Errorf("check remote template branch %q on %q: %w", preview.PublishBranch, remote, err),
			}
		}

		if remoteExists {
			err = gitpkg.WithFetchedRemoteRefFullHistory(ctx, preview.RepoRoot, remote, head.Ref, func(tempRef string) error {
				remoteCommit, resolveErr := gitpkg.ResolveRevision(ctx, preview.RepoRoot, tempRef)
				if resolveErr != nil {
					return fmt.Errorf("resolve fetched remote template branch %q: %w", head.Ref, resolveErr)
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

				localResult, publishErr := publishTemplateLocally(ctx, input, preview, remoteCommit, input.Progress)
				if publishErr != nil {
					return publishErr
				}
				result.Changed = localResult.Changed
				result.Commit = localResult.Commit
				result.Preview.SavePreview.Warnings = append(result.Preview.SavePreview.Warnings, localResult.Warnings...)
				return nil
			})
			if err != nil {
				result.RemotePush.Reason = "remote_publish_branch_unavailable"
				return result, &PublishError{
					Result: result,
					Err:    fmt.Errorf("check remote template branch %q on %q: %w", preview.PublishBranch, remote, err),
				}
			}
			result.LocalSuccess = true
		} else {
			localResult, publishErr := publishTemplateLocally(ctx, input, preview, "", input.Progress)
			if publishErr != nil {
				return TemplatePublishResult{}, publishErr
			}
			result.LocalSuccess = true
			result.Changed = localResult.Changed
			result.Commit = localResult.Commit
			result.Preview.SavePreview.Warnings = append(result.Preview.SavePreview.Warnings, localResult.Warnings...)
		}
	} else {
		remoteExists, err := remoteBranchExists(ctx, preview.RepoRoot, remote, preview.PublishBranch)
		if err != nil {
			result.RemotePush.Reason = "remote_publish_branch_unavailable"
			return result, &PublishError{
				Result: result,
				Err:    fmt.Errorf("check remote template branch %q on %q: %w", preview.PublishBranch, remote, err),
			}
		}
		if remoteExists {
			relation, err := gitpkg.CompareBranchToRemoteBranch(ctx, preview.RepoRoot, remote, preview.PublishBranch)
			if err != nil {
				result.RemotePush.Reason = "remote_publish_branch_unavailable"
				return result, &PublishError{
					Result: result,
					Err:    fmt.Errorf("check remote template branch %q on %q: %w", preview.PublishBranch, remote, err),
				}
			}
			if relation == gitpkg.BranchRelationBehind || relation == gitpkg.BranchRelationDiverged {
				result.RemotePush.Reason = "publish_branch_not_up_to_date"
				return result, &PublishError{
					Result: result,
					Err:    fmt.Errorf("local template branch %q is not up to date with %s/%s", preview.PublishBranch, remote, preview.PublishBranch),
				}
			}
		}

		localResult, publishErr := publishTemplateLocally(ctx, input, preview, "", input.Progress)
		if publishErr != nil {
			return TemplatePublishResult{}, publishErr
		}
		result.LocalSuccess = true
		result.Changed = localResult.Changed
		result.Commit = localResult.Commit
		result.Preview.SavePreview.Warnings = append(result.Preview.SavePreview.Warnings, localResult.Warnings...)
	}

	result.RemotePush.Attempted = true
	if err := publishStage(input.Progress, "pushing published branch"); err != nil {
		return TemplatePublishResult{}, err
	}
	if err := gitpkg.PushBranch(ctx, preview.RepoRoot, remote, preview.PublishBranch); err != nil {
		result.RemotePush.Reason = "push_failed"
		return result, &PublishError{
			Result: result,
			Err:    fmt.Errorf("push published branch %q to %q: %w", preview.PublishBranch, remote, err),
		}
	}
	result.RemotePush.Success = true

	if err := publishStage(input.Progress, "publish complete"); err != nil {
		return TemplatePublishResult{}, err
	}

	return result, nil
}

func publishStage(progress func(string) error, stage string) error {
	if progress == nil {
		return nil
	}

	return progress(stage)
}

type templateLocalPublishResult struct {
	Changed  bool
	Commit   string
	Warnings []string
}

type preservedWorktreeFile struct {
	Path          string
	Mode          os.FileMode
	Content       []byte
	SymlinkTarget string
}

func publishTemplateLocally(
	ctx context.Context,
	input TemplatePublishInput,
	preview TemplatePublishPreview,
	parentCommit string,
	progress func(string) error,
) (templateLocalPublishResult, error) {
	if strings.TrimSpace(parentCommit) == "" {
		if err := publishStage(progress, "checking local publish state"); err != nil {
			return templateLocalPublishResult{}, err
		}
		noop, err := isTemplatePublishNoOp(ctx, preview)
		if err != nil {
			return templateLocalPublishResult{}, fmt.Errorf("compare existing publish branch: %w", err)
		}
		if noop {
			return templateLocalPublishResult{}, nil
		}
	}

	if err := publishStage(progress, "writing published template"); err != nil {
		return templateLocalPublishResult{}, err
	}
	preservedFiles, warning, err := prepareTrackedAuthorOnlyWorktreeFiles(ctx, input, preview)
	if err != nil {
		return templateLocalPublishResult{}, err
	}
	saveResult, err := WriteTemplateSavePreview(ctx, TemplateSaveWriteInput{
		Preview:            preview.SavePreview,
		Overwrite:          true,
		ParentCommit:       parentCommit,
		AllowCurrentBranch: preview.Mode == TemplatePublishModeTemplate,
	})
	if err != nil {
		return templateLocalPublishResult{}, fmt.Errorf("publish template branch: %w", err)
	}
	if len(preservedFiles) > 0 {
		if err := publishStage(progress, "restoring author-only worktree files"); err != nil {
			return templateLocalPublishResult{}, err
		}
		if err := restorePreservedWorktreeFiles(preview.RepoRoot, preservedFiles); err != nil {
			return templateLocalPublishResult{}, fmt.Errorf("restore author-only files after publish commit %s: %w", saveResult.WriteResult.Commit, err)
		}
	}

	return templateLocalPublishResult{
		Changed:  true,
		Commit:   saveResult.WriteResult.Commit,
		Warnings: append([]string(nil), warning...),
	}, nil
}

func buildOrbitTemplateGeneratedPublishPreview(ctx context.Context, input TemplatePublishInput, currentBranch string) (TemplatePublishPreview, error) {
	if err := ensureCleanTrackedWorktree(ctx, input.RepoRoot); err != nil {
		return TemplatePublishPreview{}, err
	}

	branchManifest, err := loadCurrentOrbitTemplateBranchManifest(input.RepoRoot)
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("load %s: %w", branchManifestPath, err)
	}
	orbitID := branchManifest.Template.OrbitID
	if input.OrbitID != "" && input.OrbitID != orbitID {
		return TemplatePublishPreview{}, fmt.Errorf("requested orbit %q must match current template orbit %q", input.OrbitID, orbitID)
	}
	if _, err := loadDirectTemplatePublishOrbitSpec(ctx, input.RepoRoot, orbitID); err != nil {
		return TemplatePublishPreview{}, err
	}
	if err := EnsureMemberHintExportSync(ctx, input.RepoRoot, orbitID, "publishing"); err != nil {
		return TemplatePublishPreview{}, err
	}
	skillDetection, err := RunTemplateLocalSkillDetection(ctx, TemplateLocalSkillDetectionInput{
		RepoRoot:                input.RepoRoot,
		OrbitID:                 orbitID,
		AggregateDetectedSkills: input.AggregateDetectedSkills,
		AllowOutOfRangeSkills:   input.AllowOutOfRangeSkills,
		ConfirmPrompter:         input.SkillDetectionPrompter,
	})
	if err != nil {
		return TemplatePublishPreview{}, err
	}

	briefSync, err := EnsureBriefExportSync(ctx, input.RepoRoot, orbitID, "publishing", input.BackfillBrief)
	if err != nil {
		return TemplatePublishPreview{}, err
	}

	manifest, err := manifestFromOrbitTemplateBranchManifest(branchManifest)
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("load %s payload: %w", branchManifestPath, err)
	}
	defaultTemplate := manifest.Template.DefaultTemplate
	if input.DefaultTemplateSet {
		defaultTemplate = input.DefaultTemplate
	}

	savePreview, err := BuildTemplateSavePreview(ctx, TemplateSavePreviewInput{
		RepoRoot:      input.RepoRoot,
		OrbitID:       orbitID,
		TargetBranch:  currentBranch,
		DefaultBranch: defaultTemplate,
		ManifestMeta:  &manifest.Template,
		Warnings:      append([]string(nil), skillDetection.Warnings...),
	})
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("build publish preview: %w", err)
	}
	if briefSync.Warning != "" {
		savePreview.Warnings = append(savePreview.Warnings, briefSync.Warning)
	}
	trackedAuthorOnly, err := collectTrackedAuthorOnlyFiles(ctx, input.RepoRoot, savePreview)
	if err != nil {
		return TemplatePublishPreview{}, fmt.Errorf("collect tracked author-only files: %w", err)
	}

	return TemplatePublishPreview{
		RepoRoot:          input.RepoRoot,
		OrbitID:           orbitID,
		SourceBranch:      currentBranch,
		PublishBranch:     currentBranch,
		DefaultTemplate:   defaultTemplate,
		Mode:              TemplatePublishModeTemplate,
		SavePreview:       savePreview,
		TrackedAuthorOnly: trackedAuthorOnly,
	}, nil
}

func ensurePublishSourceContracts(ctx context.Context, repoRoot string, sourceManifest SourceManifest, currentBranch string) error {
	if currentBranch != sourceManifest.SourceBranch {
		return fmt.Errorf("publish requires current branch %q to match source branch %q", currentBranch, sourceManifest.SourceBranch)
	}

	paths, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("list tracked files: %w", err)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, ".harness/") && !isAllowedSourceBranchHarnessPath(path) {
			return fmt.Errorf("source branch must not contain %s", path)
		}
	}

	return ensureCleanTrackedWorktree(ctx, repoRoot)
}

func resolvePublishOrbitID(ctx context.Context, repoRoot string, requestedOrbitID string) (string, error) {
	sourceManifest, err := LoadSourceManifest(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load %s: %w", sourceManifestRelativePath, err)
	}

	definition, err := loadSingleHostedSourceOrbitDefinition(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	orbitID := definition.ID
	if sourceManifest.Publish == nil || strings.TrimSpace(sourceManifest.Publish.OrbitID) == "" {
		return "", fmt.Errorf("source.orbit_id must be present")
	}
	if sourceManifest.Publish != nil && sourceManifest.Publish.OrbitID != orbitID {
		return "", fmt.Errorf("source.orbit_id %q must match single source orbit %q", sourceManifest.Publish.OrbitID, orbitID)
	}
	if requestedOrbitID != "" {
		if requestedOrbitID != orbitID {
			return "", fmt.Errorf("requested orbit %q must match single source orbit %q", requestedOrbitID, orbitID)
		}
		return requestedOrbitID, nil
	}
	return orbitID, nil
}

type runtimePublishManifest struct {
	Kind     string                         `yaml:"kind"`
	Members  []runtimePublishManifestMember `yaml:"members"`
	Packages []runtimePublishManifestMember `yaml:"packages"`
}

type runtimePublishManifestMember struct {
	Package runtimePublishPackageIdentity `yaml:"package"`
	OrbitID string                        `yaml:"orbit_id"`
}

type runtimePublishPackageIdentity struct {
	Name string `yaml:"name"`
}

func resolveRuntimePublishOrbitID(ctx context.Context, repoRoot string, requestedOrbitID string) (string, error) {
	memberIDs, err := loadRuntimePublishMemberIDs(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	memberSet := make(map[string]struct{}, len(memberIDs))
	for _, memberID := range memberIDs {
		memberSet[memberID] = struct{}{}
	}

	config, err := orbitpkg.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return "", fmt.Errorf("load runtime repository config: %w", err)
	}
	if err := orbitpkg.ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return "", fmt.Errorf("validate repository config: %w", err)
	}
	orbitpkg.SortDefinitions(config.Orbits)

	orbitID := strings.TrimSpace(requestedOrbitID)
	if orbitID == "" {
		switch len(memberIDs) {
		case 1:
			orbitID = memberIDs[0]
		case 0:
			return "", fmt.Errorf("runtime Orbit Package publication requires at least one runtime member")
		default:
			return "", fmt.Errorf("runtime Orbit Package publication requires a package argument when the current runtime has multiple orbits")
		}
	}

	if _, ok := memberSet[orbitID]; !ok {
		return "", fmt.Errorf("requested orbit %q is not a member of the current runtime", orbitID)
	}
	if _, found := config.OrbitByID(orbitID); !found {
		return "", fmt.Errorf("runtime member %q is missing hosted definition", orbitID)
	}

	return orbitID, nil
}

func loadRuntimePublishMemberIDs(ctx context.Context, repoRoot string) ([]string, error) {
	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, branchManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", branchManifestPath, err)
	}

	var manifest runtimePublishManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode runtime manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Kind) != "runtime" {
		return nil, fmt.Errorf("kind must be %q", "runtime")
	}

	members := manifest.Packages
	if len(members) == 0 {
		members = manifest.Members
	}
	ids := make([]string, 0, len(members))
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberID := strings.TrimSpace(member.Package.Name)
		if memberID == "" {
			memberID = strings.TrimSpace(member.OrbitID)
		}
		if memberID == "" {
			return nil, fmt.Errorf("runtime package member must include package.name")
		}
		if _, ok := seen[memberID]; ok {
			continue
		}
		seen[memberID] = struct{}{}
		ids = append(ids, memberID)
	}
	sort.Strings(ids)

	return ids, nil
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

	branchManifestValid := false
	branchManifestData, readBranchManifestErr := gitpkg.ReadFileAtRev(ctx, preview.RepoRoot, rev, branchManifestPath)
	if readBranchManifestErr == nil {
		currentBranchManifest, parseBranchManifestErr := parseOrbitTemplateBranchManifestData(branchManifestData)
		if parseBranchManifestErr == nil {
			branchManifestValid = publishBranchManifestEquivalent(preview.SavePreview.Manifest, currentBranchManifest)
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

	expected := make(map[string]CandidateFile, len(preview.Files))
	for _, file := range preview.Files {
		expected[file.Path] = file
	}

	actualCount := 0
	for _, path := range paths {
		if path == manifestRelativePath || path == branchManifestPath {
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
		if !reflect.DeepEqual(expectedFile.Content, content) {
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

func parseOrbitTemplateBranchManifestData(data []byte) (orbitTemplateBranchManifest, error) {
	var manifest orbitTemplateBranchManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return orbitTemplateBranchManifest{}, fmt.Errorf("decode branch manifest: %w", err)
	}

	return manifest, nil
}

func publishBranchManifestEquivalent(expected Manifest, actual orbitTemplateBranchManifest) bool {
	return actual.SchemaVersion == manifestSchemaVersion &&
		actual.Kind == "orbit_template" &&
		actual.Template.OrbitID == expected.Template.OrbitID &&
		(actual.Template.DefaultTemplate == nil || *actual.Template.DefaultTemplate == expected.Template.DefaultTemplate) &&
		actual.Template.CreatedFromBranch == expected.Template.CreatedFromBranch &&
		actual.Template.CreatedFromCommit == expected.Template.CreatedFromCommit &&
		branchManifestVariablesEquivalent(expected.Variables, actual.Variables) &&
		!actual.Template.CreatedAt.IsZero()
}

func branchManifestVariablesEquivalent(expected map[string]VariableSpec, actual map[string]VariableSpec) bool {
	if len(expected) != len(actual) {
		return false
	}
	for name, spec := range expected {
		if actualSpec, ok := actual[name]; !ok || actualSpec != spec {
			return false
		}
	}

	return true
}

func ensureCleanTrackedWorktree(ctx context.Context, repoRoot string) error {
	headExists, err := gitpkg.RevisionExists(ctx, repoRoot, "HEAD")
	if err != nil {
		return fmt.Errorf("check HEAD revision: %w", err)
	}
	if !headExists {
		matchesIndex, err := gitpkg.WorktreeMatchesIndex(ctx, repoRoot)
		if err != nil {
			return fmt.Errorf("compare unborn worktree to index: %w", err)
		}
		if !matchesIndex {
			return fmt.Errorf("publish requires a clean tracked worktree; staged baseline files in an unborn repository must match the current worktree")
		}
		return nil
	}

	statusEntries, err := gitpkg.WorktreeStatus(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("load worktree status: %w", err)
	}
	for _, entry := range statusEntries {
		if entry.Tracked {
			return fmt.Errorf("publish requires a clean tracked worktree; found %s %s", entry.Code, entry.Path)
		}
	}

	return nil
}

func collectTrackedAuthorOnlyFiles(ctx context.Context, repoRoot string, preview TemplateSavePreview) ([]string, error) {
	trackedFiles, err := gitpkg.TrackedFiles(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load tracked files: %w", err)
	}
	payloadPaths := map[string]struct{}{
		branchManifestPath: {},
	}
	for _, file := range preview.Files {
		payloadPaths[file.Path] = struct{}{}
	}

	paths := make([]string, 0, len(trackedFiles))
	for _, path := range trackedFiles {
		if _, included := payloadPaths[path]; included {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func prepareTrackedAuthorOnlyWorktreeFiles(ctx context.Context, input TemplatePublishInput, preview TemplatePublishPreview) ([]preservedWorktreeFile, []string, error) {
	if preview.Mode != TemplatePublishModeTemplate || len(preview.TrackedAuthorOnly) == 0 {
		return nil, nil, nil
	}
	if input.ConfirmPrompter == nil {
		return nil, nil, fmt.Errorf("publish requires a confirmation prompter before dropping tracked author-only files")
	}
	confirmed, err := input.ConfirmPrompter.Confirm(ctx, formatTrackedAuthorOnlyPrompt(preview.TrackedAuthorOnly))
	if err != nil {
		return nil, nil, fmt.Errorf("confirm dropping tracked author-only files: %w", err)
	}
	if !confirmed {
		return nil, nil, fmt.Errorf("publish canceled; tracked author-only files remain on the current branch")
	}

	files, err := snapshotPreservedWorktreeFiles(preview.RepoRoot, preview.TrackedAuthorOnly)
	if err != nil {
		return nil, nil, err
	}
	return files, []string{
		fmt.Sprintf(
			"restored tracked author-only files to the worktree as untracked files: %s",
			strings.Join(preview.TrackedAuthorOnly, ", "),
		),
	}, nil
}

func formatTrackedAuthorOnlyPrompt(paths []string) string {
	lines := []string{
		"publish will remove these tracked author-only files from the new commit and keep them in your worktree as untracked files:",
	}
	for _, path := range paths {
		lines = append(lines, "  - "+path)
	}
	lines = append(lines, "continue? [y/N] ")
	return strings.Join(lines, "\n")
}

func snapshotPreservedWorktreeFiles(repoRoot string, paths []string) ([]preservedWorktreeFile, error) {
	files := make([]preservedWorktreeFile, 0, len(paths))
	for _, path := range paths {
		filename := filepath.Join(repoRoot, filepath.FromSlash(path))
		info, err := os.Lstat(filename)
		if err != nil {
			return nil, fmt.Errorf("stat author-only file %s: %w", path, err)
		}
		snapshot := preservedWorktreeFile{
			Path: path,
			Mode: info.Mode(),
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filename)
			if err != nil {
				return nil, fmt.Errorf("read author-only symlink %s: %w", path, err)
			}
			snapshot.SymlinkTarget = target
		} else {
			//nolint:gosec // The restored author-only path is repo-local and derived from normalized tracked paths.
			data, err := os.ReadFile(filename)
			if err != nil {
				return nil, fmt.Errorf("read author-only file %s: %w", path, err)
			}
			snapshot.Content = data
		}
		files = append(files, snapshot)
	}
	return files, nil
}

func restorePreservedWorktreeFiles(repoRoot string, files []preservedWorktreeFile) error {
	for _, file := range files {
		filename := filepath.Join(repoRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(filename), 0o750); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", file.Path, err)
		}
		if err := os.RemoveAll(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear restored path %s: %w", file.Path, err)
		}
		if file.Mode&os.ModeSymlink != 0 {
			if err := os.Symlink(file.SymlinkTarget, filename); err != nil {
				return fmt.Errorf("restore author-only symlink %s: %w", file.Path, err)
			}
			continue
		}
		if err := os.WriteFile(filename, file.Content, 0o600); err != nil {
			return fmt.Errorf("restore author-only file %s: %w", file.Path, err)
		}
		if err := os.Chmod(filename, file.Mode.Perm()); err != nil {
			return fmt.Errorf("restore author-only mode %s: %w", file.Path, err)
		}
	}
	return nil
}

func loadCurrentRevisionManifestKind(repoRoot string) (string, error) {
	state, err := LoadCurrentRepoState(context.Background(), repoRoot)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(state.Kind), nil
}

func loadCurrentOrbitTemplateBranchManifest(repoRoot string) (orbitTemplateBranchManifest, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(branchManifestPath))
	data, err := gitpkg.ReadFileWorktreeOrHEAD(context.Background(), repoRoot, branchManifestPath)
	if err != nil {
		return orbitTemplateBranchManifest{}, fmt.Errorf("read %s: %w", filename, err)
	}

	manifest, err := parseOrbitTemplateBranchManifestData(data)
	if err != nil {
		return orbitTemplateBranchManifest{}, err
	}
	if manifest.Kind != "orbit_template" {
		return orbitTemplateBranchManifest{}, fmt.Errorf("kind must be %q", "orbit_template")
	}
	if strings.TrimSpace(manifest.Template.OrbitID) == "" {
		return orbitTemplateBranchManifest{}, fmt.Errorf("template.orbit_id must not be empty")
	}

	return manifest, nil
}

func remoteBranchExists(ctx context.Context, repoRoot string, remote string, branch string) (bool, error) {
	heads, err := gitpkg.ListRemoteHeads(ctx, repoRoot, remote)
	if err != nil {
		return false, fmt.Errorf("list remote heads for %q: %w", remote, err)
	}
	for _, head := range heads {
		if head.Name == branch || head.Ref == "refs/heads/"+branch {
			return true, nil
		}
	}

	return false, nil
}

func syncLocalPublishBranchToCommit(ctx context.Context, repoRoot string, branch string, commit string) error {
	refName := "refs/heads/" + strings.TrimSpace(branch)
	if err := gitpkg.UpdateRef(ctx, repoRoot, refName, commit); err != nil {
		return fmt.Errorf("sync local publish branch %q to %s: %w", branch, commit, err)
	}

	return nil
}

func loadDirectTemplatePublishOrbitSpec(ctx context.Context, repoRoot string, orbitID string) (orbitpkg.OrbitSpec, error) {
	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID)
	if err == nil {
		return spec, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return orbitpkg.OrbitSpec{}, fmt.Errorf("load hosted orbit spec for %q: %w", orbitID, err)
	}

	return orbitpkg.OrbitSpec{}, fmt.Errorf(
		"current template branch requires hosted orbit definitions under .harness/orbits/; recreate the template branch from a hosted source branch before publishing",
	)
}
