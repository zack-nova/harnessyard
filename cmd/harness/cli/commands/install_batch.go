package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

type installBatchPreviewItemJSON struct {
	Source    installSourceJSON             `json:"source"`
	OrbitID   string                        `json:"orbit_id"`
	Bindings  []installBindingJSON          `json:"bindings"`
	Files     []string                      `json:"files"`
	Warnings  []string                      `json:"warnings,omitempty"`
	Conflicts []orbittemplate.ApplyConflict `json:"conflicts"`
}

type installBatchPreviewJSON struct {
	DryRun            bool                          `json:"dry_run"`
	HarnessRoot       string                        `json:"harness_root"`
	OverwriteExisting bool                          `json:"overwrite_existing"`
	ItemCount         int                           `json:"item_count"`
	OrbitIDs          []string                      `json:"orbit_ids"`
	WarningCount      int                           `json:"warning_count"`
	ConflictCount     int                           `json:"conflict_count"`
	Items             []installBatchPreviewItemJSON `json:"items"`
}

type installBatchResultItemJSON struct {
	Source       installSourceJSON `json:"source"`
	OrbitID      string            `json:"orbit_id"`
	WrittenPaths []string          `json:"written_paths"`
	Warnings     []string          `json:"warnings,omitempty"`
}

type installBatchResultJSON struct {
	DryRun       bool                         `json:"dry_run"`
	HarnessRoot  string                       `json:"harness_root"`
	ItemCount    int                          `json:"item_count"`
	OrbitIDs     []string                     `json:"orbit_ids"`
	MemberCount  int                          `json:"member_count"`
	WarningCount int                          `json:"warning_count"`
	Warnings     []string                     `json:"warnings,omitempty"`
	WrittenPaths []string                     `json:"written_paths"`
	Items        []installBatchResultItemJSON `json:"items"`
	Readiness    harnesspkg.ReadinessReport   `json:"readiness"`
}

type orbitInstallBatchCandidate struct {
	Preview       orbittemplate.TemplateApplyPreview
	LocalPreview  *orbittemplate.TemplateApplyPreviewInput
	RemotePreview *orbittemplate.RemoteTemplateApplyPreviewInput
}

type orbitInstallBatchApplied struct {
	Result       orbittemplate.TemplateApplyResult
	ManifestPath string
	Runtime      harnesspkg.RuntimeFile
}

// NewInstallBatchCommand creates the harness install batch command.
func NewInstallBatchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch <template-branch|git-source>...",
		Short: "Install multiple orbit templates with one shared preview and bindings contract",
		Long: "Install multiple orbit templates with one shared preview and bindings contract.\n" +
			"This command is a thin harness-author wrapper over the existing single-orbit install path.",
		Example: "" +
			"  harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml --dry-run\n" +
			"  harness install batch orbit-template/docs orbit-template/cmd --bindings .harness/vars.yaml --json\n",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetPath, err := pathFromCommand(cmd)
			if err != nil {
				return err
			}

			resolved, err := harnesspkg.ResolveRoot(cmd.Context(), targetPath)
			if err != nil {
				return fmt.Errorf("resolve harness root: %w", err)
			}

			bindingsPath, err := cmd.Flags().GetString("bindings")
			if err != nil {
				return fmt.Errorf("read --bindings flag: %w", err)
			}
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read --dry-run flag: %w", err)
			}
			overwriteExisting, err := cmd.Flags().GetBool("overwrite-existing")
			if err != nil {
				return fmt.Errorf("read --overwrite-existing flag: %w", err)
			}
			allowUnresolvedBindings, err := allowUnresolvedBindingsFromFlags(cmd)
			if err != nil {
				return err
			}
			interactive, err := cmd.Flags().GetBool("interactive")
			if err != nil {
				return fmt.Errorf("read --interactive flag: %w", err)
			}
			editorMode, err := cmd.Flags().GetBool("editor")
			if err != nil {
				return fmt.Errorf("read --editor flag: %w", err)
			}
			jsonOutput, err := wantJSON(cmd)
			if err != nil {
				return err
			}
			progressMode, err := cmd.Flags().GetString("progress")
			if err != nil {
				return fmt.Errorf("read --progress flag: %w", err)
			}
			progress, err := newInstallProgressEmitter(cmd.ErrOrStderr(), progressMode)
			if err != nil {
				return err
			}

			prompter := buildInstallPrompter(cmd, interactive)
			var editor orbittemplate.Editor
			if editorMode {
				editor, err = orbittemplate.NewEnvironmentEditor()
				if err != nil {
					return fmt.Errorf("configure bindings editor: %w", err)
				}
			}

			now := time.Now().UTC()
			candidates := make([]orbitInstallBatchCandidate, 0, len(args))
			for index, sourceArg := range args {
				if err := stageProgress(progress, fmt.Sprintf("preflighting install %d/%d", index+1, len(args))); err != nil {
					return err
				}
				candidate, err := buildOrbitInstallBatchCandidate(
					cmd,
					resolved.Repo.Root,
					sourceArg,
					bindingsPath,
					overwriteExisting,
					allowUnresolvedBindings,
					interactive,
					prompter,
					editorMode,
					editor,
					now,
				)
				if err != nil {
					return err
				}
				candidates = append(candidates, candidate)
			}

			if err := validateBatchOrbitIDs(candidates); err != nil {
				return err
			}
			candidates, err = applyBatchVariableNamespaces(cmd.Context(), candidates)
			if err != nil {
				return err
			}
			if err := validateBatchPathConflicts(resolved.Repo.Root, candidates); err != nil {
				return err
			}

			if dryRun {
				if err := stageProgress(progress, "install complete"); err != nil {
					return err
				}
				return emitInstallBatchPreview(cmd, resolved.Repo.Root, candidates, overwriteExisting, jsonOutput)
			}

			if err := failOnBlockingBatchConflicts(candidates, overwriteExisting); err != nil {
				return err
			}
			batchVarsFile, hasBatchVarsFile, err := buildBatchVarsFile(resolved.Repo.Root, candidates)
			if err != nil {
				return err
			}
			transactionPaths, err := buildInstallBatchTransactionPaths(cmd.Context(), resolved.Repo.Root, candidates)
			if err != nil {
				return err
			}
			if hasBatchVarsFile {
				if err := stageProgress(progress, "writing shared bindings"); err != nil {
					return err
				}
			}

			tx, err := harnesspkg.BeginInstallTransaction(cmd.Context(), resolved.Repo.Root, transactionPaths)
			if err != nil {
				return fmt.Errorf("begin install transaction: %w", err)
			}
			rollbackOnError := func(cause error) error {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					return errors.Join(cause, fmt.Errorf("rollback install batch transaction: %w", rollbackErr))
				}
				return cause
			}
			if hasBatchVarsFile {
				if _, err := bindings.WriteVarsFile(resolved.Repo.Root, batchVarsFile); err != nil {
					return rollbackOnError(fmt.Errorf("write harness vars file: %w", err))
				}
			}

			results := make([]orbitInstallBatchApplied, 0, len(candidates))
			for index, candidate := range candidates {
				if err := stageProgress(progress, fmt.Sprintf("writing install %d/%d", index+1, len(candidates))); err != nil {
					return rollbackOnError(err)
				}
				result, err := applyOrbitInstallBatchCandidate(cmd, resolved.Repo.Root, candidate, overwriteExisting, now)
				if err != nil {
					return rollbackOnError(err)
				}
				results = append(results, result)
			}
			tx.Commit()
			touchedOrbitIDs := make([]string, 0, len(candidates))
			for _, candidate := range candidates {
				touchedOrbitIDs = append(touchedOrbitIDs, candidate.Preview.Source.Manifest.Template.OrbitID)
			}
			guidanceOutcome := composeInstallScopedGuidance(cmd.Context(), resolved.Repo.Root, touchedOrbitIDs, overwriteExisting, composeRuntimeGuidanceForInstall)
			for index := range results {
				appendInstallGuidancePaths(resolved.Repo.Root, results[index].Result.Preview, &results[index].Result, guidanceOutcome.WrittenPaths)
			}
			if err := stageProgress(progress, "install complete"); err != nil {
				return err
			}

			return emitInstallBatchResult(cmd, resolved.Repo.Root, candidates, results, guidanceOutcome.Warnings, jsonOutput)
		},
	}

	cmd.Flags().String("bindings", "", "Path to an explicit bindings YAML file")
	cmd.Flags().Bool("overwrite-existing", false, "Allow overwriting an existing install-backed orbit and removing stale install-owned files")
	cmd.Flags().Bool("allow-unresolved-bindings", false, "Compatibility no-op: unresolved required bindings are preserved by default")
	cmd.Flags().Bool("strict-bindings", false, "Fail when required bindings are unresolved instead of preserving placeholders")
	cmd.Flags().Bool("dry-run", false, "Preview harness install without writing files")
	addProgressFlag(cmd)
	cmd.Flags().Bool("interactive", false, "Prompt for missing bindings interactively")
	cmd.Flags().Bool("editor", false, "Open an editor-backed bindings skeleton for missing required values")
	addPathFlag(cmd)
	addJSONFlag(cmd)

	return cmd
}

func buildOrbitInstallBatchCandidate(
	cmd *cobra.Command,
	repoRoot string,
	sourceArg string,
	bindingsPath string,
	overwriteExisting bool,
	allowUnresolvedBindings bool,
	interactive bool,
	prompter orbittemplate.BindingPrompter,
	editorMode bool,
	editor orbittemplate.Editor,
	now time.Time,
) (orbitInstallBatchCandidate, error) {
	localSource, err := installSourceUsesLocalRevision(cmd.Context(), repoRoot, sourceArg)
	if err != nil {
		return orbitInstallBatchCandidate{}, err
	}
	runtimeFile, err := harnesspkg.LoadRuntimeFile(repoRoot)
	if err != nil {
		return orbitInstallBatchCandidate{}, fmt.Errorf("load harness runtime: %w", err)
	}
	activeInstallOrbitIDs := harnesspkg.ActiveInstallOrbitIDs(runtimeFile)

	if localSource {
		previewInput := orbittemplate.TemplateApplyPreviewInput{
			RepoRoot:                repoRoot,
			SourceRef:               sourceArg,
			BindingsFilePath:        bindingsPath,
			RuntimeInstallOrbitIDs:  activeInstallOrbitIDs,
			OverwriteExisting:       overwriteExisting,
			AllowUnresolvedBindings: allowUnresolvedBindings,
			Interactive:             interactive,
			Prompter:                prompter,
			EditorMode:              editorMode,
			Editor:                  editor,
			Now:                     now,
		}
		preview, err := orbittemplate.BuildTemplateApplyPreview(cmd.Context(), previewInput)
		if err != nil {
			return orbitInstallBatchCandidate{}, fmt.Errorf("build harness install preview: %w", err)
		}
		if err := validateBatchTargetState(repoRoot, preview.Source.Manifest.Template.OrbitID, overwriteExisting); err != nil {
			return orbitInstallBatchCandidate{}, err
		}
		return orbitInstallBatchCandidate{
			Preview:      preview,
			LocalPreview: &previewInput,
		}, nil
	}

	previewInput := orbittemplate.RemoteTemplateApplyPreviewInput{
		RepoRoot:                repoRoot,
		RemoteURL:               sourceArg,
		BindingsFilePath:        bindingsPath,
		RuntimeInstallOrbitIDs:  activeInstallOrbitIDs,
		OverwriteExisting:       overwriteExisting,
		AllowUnresolvedBindings: allowUnresolvedBindings,
		Interactive:             interactive,
		Prompter:                prompter,
		EditorMode:              editorMode,
		Editor:                  editor,
		Now:                     now,
	}
	preview, err := orbittemplate.BuildRemoteTemplateApplyPreview(cmd.Context(), previewInput)
	if err != nil {
		return orbitInstallBatchCandidate{}, fmt.Errorf("build harness install preview: %w", err)
	}
	if err := validateBatchTargetState(repoRoot, preview.Source.Manifest.Template.OrbitID, overwriteExisting); err != nil {
		return orbitInstallBatchCandidate{}, err
	}
	return orbitInstallBatchCandidate{
		Preview:       preview,
		RemotePreview: &previewInput,
	}, nil
}

func validateBatchTargetState(repoRoot string, orbitID string, overwriteExisting bool) error {
	runtimeFile, err := harnesspkg.LoadRuntimeFile(repoRoot)
	if err != nil {
		return fmt.Errorf("load harness runtime: %w", err)
	}
	targetState, err := inspectInstallTargetState(repoRoot, runtimeFile, orbitID)
	if err != nil {
		return err
	}
	if err := validateInstallTargetState(targetState, orbitID, overwriteExisting); err != nil {
		return err
	}
	return nil
}

func validateBatchOrbitIDs(candidates []orbitInstallBatchCandidate) error {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		orbitID := candidate.Preview.Source.Manifest.Template.OrbitID
		if _, ok := seen[orbitID]; ok {
			return fmt.Errorf("duplicate orbit_id %q in install batch", orbitID)
		}
		seen[orbitID] = struct{}{}
	}
	return nil
}

func validateBatchPathConflicts(repoRoot string, candidates []orbitInstallBatchCandidate) error {
	seen := make(map[string]string)
	for _, candidate := range candidates {
		orbitID := candidate.Preview.Source.Manifest.Template.OrbitID
		for _, path := range installPreviewPaths(repoRoot, candidate.Preview) {
			if allowSharedBatchPath(path) {
				continue
			}
			if existingOrbitID, ok := seen[path]; ok && existingOrbitID != orbitID {
				return fmt.Errorf("batch install path collision between orbit %q and orbit %q at %s", existingOrbitID, orbitID, path)
			}
			seen[path] = orbitID
		}
	}
	return nil
}

type batchVariableDeclaration struct {
	OrbitID     string
	Declaration bindings.VariableDeclaration
}

func applyBatchVariableNamespaces(
	ctx context.Context,
	candidates []orbitInstallBatchCandidate,
) ([]orbitInstallBatchCandidate, error) {
	namespacesByOrbit := batchVariableNamespaces(candidates)
	if len(namespacesByOrbit) == 0 {
		return candidates, nil
	}

	rebuilt := make([]orbitInstallBatchCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		orbitID := candidate.Preview.Source.Manifest.Template.OrbitID
		next, err := rebuildOrbitInstallBatchCandidatePreview(ctx, candidate, namespacesByOrbit[orbitID])
		if err != nil {
			return nil, fmt.Errorf("rebuild namespaced batch preview for orbit %q: %w", orbitID, err)
		}
		rebuilt = append(rebuilt, next)
	}
	return rebuilt, nil
}

func batchVariableNamespaces(candidates []orbitInstallBatchCandidate) map[string]map[string]string {
	declarationsByName := make(map[string][]batchVariableDeclaration)
	for _, candidate := range candidates {
		orbitID := candidate.Preview.Source.Manifest.Template.OrbitID
		for name, spec := range candidate.Preview.Source.Manifest.Variables {
			declarationsByName[name] = append(declarationsByName[name], batchVariableDeclaration{
				OrbitID: orbitID,
				Declaration: bindings.VariableDeclaration{
					Description: spec.Description,
					Required:    spec.Required,
				},
			})
		}
	}

	namespacesByOrbit := make(map[string]map[string]string)
	for name, declarations := range declarationsByName {
		if !batchVariableHasDeclarationConflict(name, declarations) {
			continue
		}
		for _, declaration := range declarations {
			if namespacesByOrbit[declaration.OrbitID] == nil {
				namespacesByOrbit[declaration.OrbitID] = map[string]string{}
			}
			namespacesByOrbit[declaration.OrbitID][name] = declaration.OrbitID
		}
	}
	return namespacesByOrbit
}

func batchVariableHasDeclarationConflict(name string, declarations []batchVariableDeclaration) bool {
	for left := 0; left < len(declarations); left++ {
		for right := left + 1; right < len(declarations); right++ {
			if _, err := bindings.MergeVariableDeclaration(name, declarations[left].Declaration, declarations[right].Declaration); err != nil {
				return true
			}
		}
	}
	return false
}

func rebuildOrbitInstallBatchCandidatePreview(
	ctx context.Context,
	candidate orbitInstallBatchCandidate,
	variableNamespaces map[string]string,
) (orbitInstallBatchCandidate, error) {
	if len(variableNamespaces) == 0 {
		return candidate, nil
	}

	if candidate.LocalPreview != nil {
		previewInput := *candidate.LocalPreview
		previewInput.VariableNamespaces = mergeBatchVariableNamespaces(previewInput.VariableNamespaces, variableNamespaces)
		preview, err := orbittemplate.BuildTemplateApplyPreview(ctx, previewInput)
		if err != nil {
			return orbitInstallBatchCandidate{}, fmt.Errorf("build local template preview: %w", err)
		}
		return orbitInstallBatchCandidate{
			Preview:      preview,
			LocalPreview: &previewInput,
		}, nil
	}

	previewInput := *candidate.RemotePreview
	previewInput.VariableNamespaces = mergeBatchVariableNamespaces(previewInput.VariableNamespaces, variableNamespaces)
	preview, err := orbittemplate.BuildRemoteTemplateApplyPreview(ctx, previewInput)
	if err != nil {
		return orbitInstallBatchCandidate{}, fmt.Errorf("build external template preview: %w", err)
	}
	return orbitInstallBatchCandidate{
		Preview:       preview,
		RemotePreview: &previewInput,
	}, nil
}

func mergeBatchVariableNamespaces(left map[string]string, right map[string]string) map[string]string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	merged := make(map[string]string, len(left)+len(right))
	for name, namespace := range left {
		merged[name] = namespace
	}
	for name, namespace := range right {
		if strings.TrimSpace(namespace) == "" {
			continue
		}
		merged[name] = namespace
	}
	return merged
}

func allowSharedBatchPath(path string) bool {
	switch strings.TrimSpace(path) {
	case ".harness/manifest.yaml", ".harness/vars.yaml", "AGENTS.md", "HUMANS.md", "BOOTSTRAP.md":
		return true
	default:
		return false
	}
}

func failOnBlockingBatchConflicts(candidates []orbitInstallBatchCandidate, overwriteExisting bool) error {
	if overwriteExisting {
		return nil
	}
	for _, candidate := range candidates {
		if len(candidate.Preview.Conflicts) == 0 {
			continue
		}
		return fmt.Errorf(
			"batch install blocked by conflicts for orbit %q: %s",
			candidate.Preview.Source.Manifest.Template.OrbitID,
			candidate.Preview.Conflicts[0].Message,
		)
	}
	return nil
}

func buildBatchVarsFile(repoRoot string, candidates []orbitInstallBatchCandidate) (bindings.VarsFile, bool, error) {
	existing := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     map[string]bindings.VariableBinding{},
	}
	hasExisting := false
	if file, err := bindings.LoadVarsFile(repoRoot); err == nil {
		existing = file
		hasExisting = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return bindings.VarsFile{}, false, fmt.Errorf("load runtime vars file: %w", err)
	}

	merged := make(map[string]bindings.VariableBinding, len(existing.Variables))
	for name, binding := range existing.Variables {
		merged[name] = binding
	}
	scopedMerged := make(map[string]bindings.ScopedVariableBindings, len(existing.ScopedVariables))
	for namespace, scoped := range existing.ScopedVariables {
		scopedMerged[namespace] = bindings.ScopedVariableBindings{
			Variables: cloneBatchVariableBindings(scoped.Variables),
		}
	}

	changed := false
	for _, candidate := range candidates {
		for name, binding := range candidate.Preview.ResolvedBindings {
			if binding.Source == bindings.SourceRepoVars || binding.Source == bindings.SourceRepoVarsScoped {
				continue
			}

			next := bindings.VariableBinding{
				Value:       binding.Value,
				Description: binding.Description,
			}
			if strings.TrimSpace(binding.Namespace) != "" {
				scoped := scopedMerged[binding.Namespace]
				if scoped.Variables == nil {
					scoped.Variables = map[string]bindings.VariableBinding{}
				}
				if current, ok := scoped.Variables[name]; ok && reflect.DeepEqual(current, next) {
					continue
				}

				scoped.Variables[name] = next
				scopedMerged[binding.Namespace] = scoped
				changed = true
				continue
			}

			if current, ok := merged[name]; ok && reflect.DeepEqual(current, next) {
				continue
			}

			merged[name] = next
			changed = true
		}
	}
	if !changed {
		return bindings.VarsFile{}, false, nil
	}

	planned := bindings.VarsFile{
		SchemaVersion: 1,
		Variables:     merged,
	}
	if len(scopedMerged) > 0 {
		planned.ScopedVariables = scopedMerged
	}
	if hasExisting && reflect.DeepEqual(existing, planned) {
		return bindings.VarsFile{}, false, nil
	}

	return planned, true, nil
}

func buildInstallBatchTransactionPaths(
	ctx context.Context,
	repoRoot string,
	candidates []orbitInstallBatchCandidate,
) ([]string, error) {
	runtimeFile, err := harnesspkg.LoadRuntimeFile(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("load harness runtime: %w", err)
	}

	paths := make([]string, 0, len(candidates)*6)
	for _, candidate := range candidates {
		orbitID := candidate.Preview.Source.Manifest.Template.OrbitID
		paths = append(paths, installPreviewPaths(repoRoot, candidate.Preview)...)

		targetState, err := inspectInstallTargetState(repoRoot, runtimeFile, orbitID)
		if err != nil {
			return nil, err
		}
		if !targetState.RequiresOverwrite {
			continue
		}

		cleanupPlan, err := orbittemplate.BuildInstallOwnedCleanupPlan(ctx, repoRoot, targetState.ExistingRecord, candidate.Preview)
		if err != nil {
			return nil, fmt.Errorf("reconstruct existing install ownership for orbit %q: %w", orbitID, err)
		}
		paths = append(paths, cleanupPlan.DeletePaths...)
		if cleanupPlan.RemoveSharedAgentsFile {
			paths = append(paths, "AGENTS.md")
		}
	}

	return dedupeSortedStrings(paths), nil
}

func cloneBatchVariableBindings(values map[string]bindings.VariableBinding) map[string]bindings.VariableBinding {
	if values == nil {
		return nil
	}
	cloned := make(map[string]bindings.VariableBinding, len(values))
	for name, binding := range values {
		cloned[name] = binding
	}
	return cloned
}

func applyOrbitInstallBatchCandidate(
	cmd *cobra.Command,
	repoRoot string,
	candidate orbitInstallBatchCandidate,
	overwriteExisting bool,
	now time.Time,
) (orbitInstallBatchApplied, error) {
	orbitID := candidate.Preview.Source.Manifest.Template.OrbitID

	runtimeFile, err := harnesspkg.LoadRuntimeFile(repoRoot)
	if err != nil {
		return orbitInstallBatchApplied{}, fmt.Errorf("load harness runtime: %w", err)
	}
	targetState, err := inspectInstallTargetState(repoRoot, runtimeFile, orbitID)
	if err != nil {
		return orbitInstallBatchApplied{}, err
	}
	if err := validateInstallTargetState(targetState, orbitID, overwriteExisting); err != nil {
		return orbitInstallBatchApplied{}, err
	}

	var (
		result      orbittemplate.TemplateApplyResult
		cleanupPlan orbittemplate.InstallOwnedCleanupPlan
	)
	if candidate.LocalPreview != nil {
		previewInput := *candidate.LocalPreview
		previewInput.OverwriteExisting = true
		previewInput.SkipSharedAgentsWrite = true
		if targetState.RequiresOverwrite {
			cleanupPlan, err = orbittemplate.BuildInstallOwnedCleanupPlan(cmd.Context(), repoRoot, targetState.ExistingRecord, candidate.Preview)
			if err != nil {
				return orbitInstallBatchApplied{}, fmt.Errorf("reconstruct existing install ownership: %w", err)
			}
		}
		result, err = orbittemplate.ApplyLocalTemplate(cmd.Context(), orbittemplate.TemplateApplyInput{Preview: previewInput})
		if err != nil {
			return orbitInstallBatchApplied{}, fmt.Errorf("install local template: %w", err)
		}
	} else {
		previewInput := *candidate.RemotePreview
		previewInput.OverwriteExisting = true
		previewInput.SkipSharedAgentsWrite = true
		if targetState.RequiresOverwrite {
			cleanupPlan, err = orbittemplate.BuildInstallOwnedCleanupPlan(cmd.Context(), repoRoot, targetState.ExistingRecord, candidate.Preview)
			if err != nil {
				return orbitInstallBatchApplied{}, fmt.Errorf("reconstruct existing install ownership: %w", err)
			}
		}
		result, err = orbittemplate.ApplyRemoteTemplate(cmd.Context(), orbittemplate.RemoteTemplateApplyInput{Preview: previewInput})
		if err != nil {
			return orbitInstallBatchApplied{}, fmt.Errorf("install external template: %w", err)
		}
	}

	if targetState.RequiresOverwrite {
		runBeforeInstallOwnedCleanupHook(repoRoot, orbitID, cleanupPlan)
		removedPaths, err := orbittemplate.ApplyInstallOwnedCleanup(repoRoot, orbitID, cleanupPlan)
		if err != nil {
			return orbitInstallBatchApplied{}, fmt.Errorf("remove stale install-owned paths: %w", err)
		}
		result.WrittenPaths = append(result.WrittenPaths, removedPaths...)
	}

	memberResult, err := upsertInstallMemberForState(cmd.Context(), repoRoot, orbitID, now, targetState)
	if err != nil {
		return orbitInstallBatchApplied{}, fmt.Errorf("record install-backed member: %w", err)
	}

	return orbitInstallBatchApplied{
		Result:       result,
		ManifestPath: memberResult.ManifestPath,
		Runtime:      memberResult.Runtime,
	}, nil
}

func emitInstallBatchPreview(
	cmd *cobra.Command,
	harnessRoot string,
	candidates []orbitInstallBatchCandidate,
	overwriteExisting bool,
	jsonOutput bool,
) error {
	items := make([]installBatchPreviewItemJSON, 0, len(candidates))
	orbitIDs := make([]string, 0, len(candidates))
	warningCount := 0
	conflictCount := 0
	for _, candidate := range candidates {
		orbitIDs = append(orbitIDs, candidate.Preview.Source.Manifest.Template.OrbitID)
		warningCount += len(candidate.Preview.Warnings)
		conflictCount += len(candidate.Preview.Conflicts)
		items = append(items, installBatchPreviewItemJSON{
			Source:    installSourcePayload(candidate.Preview),
			OrbitID:   candidate.Preview.Source.Manifest.Template.OrbitID,
			Bindings:  installBindingsPayload(candidate.Preview.ResolvedBindings),
			Files:     installPreviewPaths(harnessRoot, candidate.Preview),
			Warnings:  append([]string(nil), candidate.Preview.Warnings...),
			Conflicts: append([]orbittemplate.ApplyConflict(nil), candidate.Preview.Conflicts...),
		})
	}

	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), installBatchPreviewJSON{
			DryRun:            true,
			HarnessRoot:       harnessRoot,
			OverwriteExisting: overwriteExisting,
			ItemCount:         len(items),
			OrbitIDs:          orbitIDs,
			WarningCount:      warningCount,
			ConflictCount:     conflictCount,
			Items:             items,
		})
	}

	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "harness install batch dry-run"); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "item_count: %d\n", len(items)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(orbitIDs) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "orbit_ids: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_ids: %s\n", strings.Join(orbitIDs, ", ")); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning_count: %d\n", warningCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "conflict_count: %d\n", conflictCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "item: orbit_id=%s source_ref=%s files=%d conflicts=%d warnings=%d\n", item.OrbitID, item.Source.Ref, len(item.Files), len(item.Conflicts), len(item.Warnings)); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	return nil
}

func emitInstallBatchResult(
	cmd *cobra.Command,
	harnessRoot string,
	candidates []orbitInstallBatchCandidate,
	results []orbitInstallBatchApplied,
	guidanceWarnings []string,
	jsonOutput bool,
) error {
	readiness, err := evaluateCommandReadiness(cmd.Context(), harnessRoot)
	if err != nil {
		return err
	}
	items := make([]installBatchResultItemJSON, 0, len(results))
	orbitIDs := make([]string, 0, len(results))
	allWrittenPaths := make([]string, 0)
	warningCount := len(guidanceWarnings)
	warnings := append([]string(nil), guidanceWarnings...)
	memberCount := 0
	for index, applied := range results {
		candidate := candidates[index]
		orbitIDs = append(orbitIDs, candidate.Preview.Source.Manifest.Template.OrbitID)
		writtenPaths := installResultPaths(harnessRoot, applied.Result.WrittenPaths, applied.ManifestPath)
		allWrittenPaths = append(allWrittenPaths, writtenPaths...)
		warningCount += len(applied.Result.Preview.Warnings)
		warnings = append(warnings, applied.Result.Preview.Warnings...)
		memberCount = len(applied.Runtime.Members)
		items = append(items, installBatchResultItemJSON{
			Source:       installSourcePayload(applied.Result.Preview),
			OrbitID:      candidate.Preview.Source.Manifest.Template.OrbitID,
			WrittenPaths: writtenPaths,
			Warnings:     append([]string(nil), applied.Result.Preview.Warnings...),
		})
	}
	allWrittenPaths = dedupeSortedStrings(allWrittenPaths)

	if jsonOutput {
		return emitJSON(cmd.OutOrStdout(), installBatchResultJSON{
			DryRun:       false,
			HarnessRoot:  harnessRoot,
			ItemCount:    len(items),
			OrbitIDs:     orbitIDs,
			MemberCount:  memberCount,
			WarningCount: warningCount,
			Warnings:     dedupeSortedStrings(warnings),
			WrittenPaths: allWrittenPaths,
			Items:        items,
			Readiness:    readiness,
		})
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "installed %d orbits into harness %s\n", len(items), harnessRoot); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if len(orbitIDs) == 0 {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "orbit_ids: none"); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	} else {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "orbit_ids: %s\n", strings.Join(orbitIDs, ", ")); err != nil {
			return fmt.Errorf("write command output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "member_count: %d\n", memberCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "files: %d\n", len(allWrittenPaths)); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "warning_count: %d\n", warningCount); err != nil {
		return fmt.Errorf("write command output: %w", err)
	}
	if warningCount > 0 {
		if err := emitInstallWarnings(cmd, dedupeSortedStrings(warnings)); err != nil {
			return err
		}
	}
	if err := emitPostActionReadinessText(cmd.OutOrStdout(), readiness); err != nil {
		return err
	}
	return nil
}
