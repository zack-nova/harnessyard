package harness

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const rootAgentsContributor = "root_agents"

// TemplateSavePreviewInput describes the harness runtime-to-template preview pipeline.
type TemplateSavePreviewInput struct {
	RepoRoot                  string
	TargetBranch              string
	DefaultTemplate           bool
	EditTemplate              bool
	Editor                    orbittemplate.Editor
	Now                       time.Time
	IncludeCompletedBootstrap bool
}

// TemplateSavePreview contains the fully built harness template tree and manifest.
type TemplateSavePreview struct {
	RepoRoot             string
	HarnessID            string
	TargetBranch         string
	Files                []orbittemplate.CandidateFile
	MemberSnapshotFiles  []orbittemplate.CandidateFile
	ReplacementSummaries []orbittemplate.FileReplacementSummary
	Ambiguities          []orbittemplate.FileReplacementAmbiguity
	AmbiguitySources     map[string][]string
	Warnings             []string
	Manifest             TemplateManifest
	ManifestData         []byte
}

// FilePaths returns the stable preview file list including the generated harness manifest path.
func (preview TemplateSavePreview) FilePaths() []string {
	paths := make([]string, 0, len(preview.Files)+len(preview.MemberSnapshotFiles)+1)
	paths = append(paths, TemplateRepoPath())
	for _, file := range preview.MemberSnapshotFiles {
		paths = append(paths, file.Path)
	}
	for _, file := range preview.Files {
		paths = append(paths, file.Path)
	}

	return paths
}

// TemplateSaveInput describes the real harness-template branch write path.
type TemplateSaveInput struct {
	Preview      TemplateSavePreviewInput
	Overwrite    bool
	ParentCommit string
}

// TemplateSaveWriteInput describes writing one already-built template preview to a branch.
type TemplateSaveWriteInput struct {
	Preview      TemplateSavePreview
	Overwrite    bool
	ParentCommit string
}

// TemplateSaveResult contains the preview plus the written template branch result.
type TemplateSaveResult struct {
	Preview     TemplateSavePreview
	WriteResult gitpkg.WriteTemplateBranchResult
}

// BuildTemplateSavePreview builds one merge-checked harness template preview from the current runtime.
func BuildTemplateSavePreview(ctx context.Context, input TemplateSavePreviewInput) (TemplateSavePreview, error) {
	state, err := orbittemplate.LoadCurrentRepoState(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load current repo state: %w", err)
	}
	if state.Kind != ManifestKindRuntime {
		return TemplateSavePreview{}, fmt.Errorf("harness template save requires a runtime revision; current revision kind is %q", state.Kind)
	}

	runtimeFile, err := LoadRuntimeFile(input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load harness runtime: %w", err)
	}
	varsFile, err := loadOptionalTemplateCandidateVars(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load harness vars: %w", err)
	}

	candidates := make([]TemplateMemberCandidate, 0, len(runtimeFile.Members))
	replacementSummaries := make([]orbittemplate.FileReplacementSummary, 0, len(runtimeFile.Members))
	ambiguities := make([]orbittemplate.FileReplacementAmbiguity, 0, len(runtimeFile.Members))
	ambiguitySources := make(map[string][]string)
	warnings := make([]string, 0, len(runtimeFile.Members))

	for _, member := range runtimeFile.Members {
		candidate, err := BuildTemplateMemberCandidateWithOptions(
			ctx,
			input.RepoRoot,
			member,
			input.IncludeCompletedBootstrap,
		)
		if err != nil {
			return TemplateSavePreview{}, fmt.Errorf("build member candidate for %q: %w", member.OrbitID, err)
		}

		candidates = append(candidates, candidate)
		replacementSummaries = append(replacementSummaries, candidate.ReplacementSummaries...)
		ambiguities = append(ambiguities, candidate.Ambiguities...)
		addTemplateAmbiguitySource(ambiguitySources, candidate.Ambiguities, member.OrbitID)
		warnings = append(warnings, candidate.Warnings...)
	}

	merged, err := MergeTemplateMemberCandidates(candidates)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("merge member candidates: %w", err)
	}

	rootAgents, err := BuildRootAgentsTemplateFile(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("build root AGENTS template file: %w", err)
	}
	merged, err = mergeRootAgentsTemplateResult(merged, rootAgents)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("merge root AGENTS template file: %w", err)
	}
	if rootAgents.ReplacementSummary != nil {
		replacementSummaries = append(replacementSummaries, *rootAgents.ReplacementSummary)
	}
	if rootAgents.Ambiguity != nil {
		ambiguities = append(ambiguities, *rootAgents.Ambiguity)
		addTemplateAmbiguitySource(ambiguitySources, []orbittemplate.FileReplacementAmbiguity{*rootAgents.Ambiguity}, rootAgentsContributor)
	}

	sortTemplateReplacementSummaries(replacementSummaries)
	sortTemplateAmbiguities(ambiguities)
	sort.Strings(warnings)

	currentBranch, err := orbittemplate.RequireCurrentBranch(state, "harness template save")
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("require current branch: %w", err)
	}
	headCommit, err := orbittemplate.CurrentCommitOrZero(ctx, input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("resolve current provenance commit: %w", err)
	}

	files := append([]orbittemplate.CandidateFile(nil), merged.Files...)
	agentPackageFiles, err := loadTemplateSaveAgentPackageCandidates(input.RepoRoot)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("load harness agent package candidates: %w", err)
	}
	files = append(files, agentPackageFiles...)
	if input.EditTemplate {
		memberIDs := make([]string, 0, len(merged.Members))
		for _, member := range merged.Members {
			memberIDs = append(memberIDs, member.OrbitID)
		}
		files, err = editHarnessTemplateFiles(ctx, memberIDs, files, input.Editor)
		if err != nil {
			return TemplateSavePreview{}, fmt.Errorf("edit harness template candidate: %w", err)
		}
	}

	variableSpecs, err := buildTemplateSaveVariableSpecs(memberIDsFromTemplateMembers(merged.Members), files, varsFile.Variables)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("build harness template variables: %w", err)
	}
	memberSnapshotFiles, err := buildTemplateMemberSnapshotFiles(candidates, files, varsFile.Variables)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("build harness template member snapshots: %w", err)
	}

	manifest := TemplateManifest{
		SchemaVersion: templateSchemaVersion,
		Kind:          TemplateKind,
		Template: TemplateMetadata{
			HarnessID:          runtimeFile.Harness.ID,
			DefaultTemplate:    input.DefaultTemplate,
			CreatedFromBranch:  currentBranch,
			CreatedFromCommit:  headCommit,
			CreatedAt:          resolveTemplateSaveTime(input.Now),
			IncludesRootAgents: includesRootAgentsFile(files),
		},
		Members:   append([]TemplateMember(nil), merged.Members...),
		Variables: variableSpecs,
	}
	manifestData, err := MarshalTemplateManifest(manifest)
	if err != nil {
		return TemplateSavePreview{}, fmt.Errorf("marshal harness template manifest: %w", err)
	}

	return TemplateSavePreview{
		RepoRoot:             input.RepoRoot,
		HarnessID:            runtimeFile.Harness.ID,
		TargetBranch:         input.TargetBranch,
		Files:                files,
		MemberSnapshotFiles:  memberSnapshotFiles,
		ReplacementSummaries: replacementSummaries,
		Ambiguities:          ambiguities,
		AmbiguitySources:     ambiguitySources,
		Warnings:             warnings,
		Manifest:             manifest,
		ManifestData:         manifestData,
	}, nil
}

func loadTemplateSaveAgentPackageCandidates(repoRoot string) ([]orbittemplate.CandidateFile, error) {
	files := make([]orbittemplate.CandidateFile, 0, 4)

	_, newErr := os.Stat(FrameworksPath(repoRoot))
	_, legacyErr := os.Stat(legacyFrameworksPath(repoRoot))
	if !os.IsNotExist(newErr) || !os.IsNotExist(legacyErr) {
		file, err := LoadOptionalFrameworksFile(repoRoot)
		if err != nil {
			return nil, err
		}
		data, err := MarshalFrameworksFile(file)
		if err != nil {
			return nil, err
		}
		files = append(files, orbittemplate.CandidateFile{
			Path:    FrameworksRepoPath(),
			Content: data,
			Mode:    gitpkg.FileModeRegular,
		})
	}

	agentConfig, hasAgentConfig, err := LoadOptionalAgentConfigFile(repoRoot)
	if err != nil {
		return nil, err
	}
	if hasAgentConfig {
		data, err := MarshalAgentConfigFile(agentConfig)
		if err != nil {
			return nil, err
		}
		files = append(files, orbittemplate.CandidateFile{
			Path:    AgentConfigRepoPath(),
			Content: data,
			Mode:    gitpkg.FileModeRegular,
		})
	}

	overlayIDs, err := ListAgentOverlayIDs(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, agentID := range overlayIDs {
		overlay, err := LoadAgentOverlayFile(repoRoot, agentID)
		if err != nil {
			return nil, err
		}
		files = append(files, orbittemplate.CandidateFile{
			Path:    AgentOverlayRepoPath(agentID),
			Content: append([]byte(nil), overlay.Content...),
			Mode:    gitpkg.FileModeRegular,
		})
	}

	return files, nil
}

// SaveTemplateBranch writes the previewed harness template tree to a branch without mutating the current worktree.
func SaveTemplateBranch(ctx context.Context, input TemplateSaveInput) (TemplateSaveResult, error) {
	preview, err := BuildTemplateSavePreview(ctx, input.Preview)
	if err != nil {
		return TemplateSaveResult{}, err
	}

	return WriteTemplateSavePreview(ctx, TemplateSaveWriteInput{
		Preview:      preview,
		Overwrite:    input.Overwrite,
		ParentCommit: input.ParentCommit,
	})
}

// WriteTemplateSavePreview writes one already-built harness template preview to a branch.
func WriteTemplateSavePreview(ctx context.Context, input TemplateSaveWriteInput) (TemplateSaveResult, error) {
	preview := input.Preview
	if len(preview.Ambiguities) > 0 {
		return TemplateSaveResult{}, fmt.Errorf(
			"replacement ambiguity detected in %s; resolve the previewed ambiguities before saving",
			FormatTemplateAmbiguitySources(preview.AmbiguitySources),
		)
	}

	files := make([]gitpkg.TemplateTreeFile, 0, len(preview.MemberSnapshotFiles)+len(preview.Files))
	for _, file := range preview.MemberSnapshotFiles {
		files = append(files, gitpkg.TemplateTreeFile{
			Path:    file.Path,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}
	for _, file := range preview.Files {
		files = append(files, gitpkg.TemplateTreeFile{
			Path:    file.Path,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}
	files = append(files, gitpkg.TemplateTreeFile{
		Path:    TemplateRepoPath(),
		Content: preview.ManifestData,
		Mode:    gitpkg.FileModeRegular,
	})

	branchManifest, err := MarshalManifestFile(ManifestFile{
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
	})
	if err != nil {
		return TemplateSaveResult{}, fmt.Errorf("build branch manifest: %w", err)
	}

	writeResult, err := gitpkg.WriteTemplateBranch(ctx, preview.RepoRoot, gitpkg.WriteTemplateBranchInput{
		Branch:       preview.TargetBranch,
		Overwrite:    input.Overwrite,
		ParentCommit: input.ParentCommit,
		Message:      fmt.Sprintf("harness template save %s", preview.HarnessID),
		ManifestPath: ManifestRepoPath(),
		Manifest:     branchManifest,
		Files:        files,
	})
	if err != nil {
		return TemplateSaveResult{}, fmt.Errorf("write harness template branch: %w", err)
	}

	return TemplateSaveResult{
		Preview:     preview,
		WriteResult: writeResult,
	}, nil
}

func manifestMembersFromTemplateMembers(members []TemplateMember) []ManifestMember {
	converted := make([]ManifestMember, 0, len(members))
	for _, member := range members {
		converted = append(converted, ManifestMember{
			Package: ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: member.OrbitID},
			OrbitID: member.OrbitID,
		})
	}

	return converted
}

func memberIDsFromTemplateMembers(members []TemplateMember) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.OrbitID)
	}

	return ids
}

func mergeRootAgentsTemplateResult(
	merged TemplateMergeResult,
	rootAgents RootAgentsTemplateResult,
) (TemplateMergeResult, error) {
	if !rootAgents.IncludesRootAgents || rootAgents.File == nil {
		return merged, nil
	}

	files := append([]orbittemplate.CandidateFile(nil), merged.Files...)
	foundRootAgents := false
	for index, file := range files {
		if file.Path != rootAgentsPath {
			continue
		}
		foundRootAgents = true
		if !candidateFilesEqual(file, *rootAgents.File) {
			return TemplateMergeResult{}, fmt.Errorf("path conflict for %q", rootAgentsPath)
		}
		files[index] = cloneCandidateFile(file)
		break
	}
	if !foundRootAgents {
		files = append(files, cloneCandidateFile(*rootAgents.File))
		sort.Slice(files, func(left, right int) bool {
			return files[left].Path < files[right].Path
		})
	}

	variables := cloneTemplateVariables(merged.Variables)
	for name, next := range rootAgents.Variables {
		current, ok := variables[name]
		if !ok {
			variables[name] = next
			continue
		}

		mergedSpec, err := mergeTemplateVariableSpec(name, current, next)
		if err != nil {
			return TemplateMergeResult{}, err
		}
		variables[name] = mergedSpec
	}

	return TemplateMergeResult{
		Members:   append([]TemplateMember(nil), merged.Members...),
		Files:     files,
		Variables: variables,
	}, nil
}

func cloneTemplateVariables(values map[string]TemplateVariableSpec) map[string]TemplateVariableSpec {
	cloned := make(map[string]TemplateVariableSpec, len(values))
	for _, name := range contractutil.SortedKeys(values) {
		cloned[name] = values[name]
	}

	return cloned
}

func resolveTemplateSaveTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}

	return now.UTC()
}

func sortTemplateReplacementSummaries(values []orbittemplate.FileReplacementSummary) {
	sort.Slice(values, func(left, right int) bool {
		return values[left].Path < values[right].Path
	})
}

func sortTemplateAmbiguities(values []orbittemplate.FileReplacementAmbiguity) {
	sort.Slice(values, func(left, right int) bool {
		return values[left].Path < values[right].Path
	})
}

func addTemplateAmbiguitySource(
	sources map[string][]string,
	ambiguities []orbittemplate.FileReplacementAmbiguity,
	contributor string,
) {
	for _, ambiguity := range ambiguities {
		sources[ambiguity.Path] = appendContributor(sources[ambiguity.Path], contributor)
	}
}

// FormatTemplateAmbiguitySources returns a stable contributor summary for replacement ambiguities.
func FormatTemplateAmbiguitySources(sources map[string][]string) string {
	if len(sources) == 0 {
		return "unknown contributors"
	}

	paths := make([]string, 0, len(sources))
	for path := range sources {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	items := make([]string, 0, len(paths))
	for _, path := range paths {
		items = append(items, fmt.Sprintf("%s [%s]", path, strings.Join(sources[path], ", ")))
	}

	return strings.Join(items, "; ")
}
