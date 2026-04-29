package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	progresspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/progress"
	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	viewpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/view"
)

type contextKey string

const workingDirContextKey contextKey = "working_dir"
const templatePublishInteractiveContextKey contextKey = "template_publish_interactive"

var errOrbitNotInitialized = errors.New("orbit is not initialized; run `orbit init` first")

// WithWorkingDir injects the working directory used by command tests.
func WithWorkingDir(ctx context.Context, workingDir string) context.Context {
	return context.WithValue(ctx, workingDirContextKey, workingDir)
}

// WithTemplatePublishInteractive preserves terminal interactivity after a wrapper replaces command input.
func WithTemplatePublishInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, templatePublishInteractiveContextKey, true)
}

func templatePublishInteractiveFromContext(ctx context.Context) bool {
	interactive, ok := ctx.Value(templatePublishInteractiveContextKey).(bool)
	return ok && interactive
}

func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
}

func addProgressFlag(cmd *cobra.Command) {
	cmd.Flags().String("progress", "auto", "Progress output mode: auto, plain, or quiet")
}

func wantJSON(cmd *cobra.Command) (bool, error) {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return false, fmt.Errorf("read json flag: %w", err)
	}

	return jsonOutput, nil
}

func templatePublishStreamIsTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func emitJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode json output: %w", err)
	}

	return nil
}

func progressFromCommand(cmd *cobra.Command) (progresspkg.Emitter, error) {
	rawMode, err := cmd.Flags().GetString("progress")
	if err != nil {
		return progresspkg.Emitter{}, fmt.Errorf("read progress flag: %w", err)
	}

	emitter, err := newProgressEmitter(cmd.ErrOrStderr(), rawMode)
	if err != nil {
		return progresspkg.Emitter{}, err
	}

	return emitter, nil
}

func newProgressEmitter(writer io.Writer, rawMode string) (progresspkg.Emitter, error) {
	emitter, err := progresspkg.NewEmitter(writer, rawMode)
	if err != nil {
		return progresspkg.Emitter{}, fmt.Errorf("create progress emitter: %w", err)
	}

	return emitter, nil
}

func stageProgress(progress progresspkg.Emitter, stage string) error {
	if err := progress.Stage(stage); err != nil {
		return fmt.Errorf("update progress stage %q: %w", stage, err)
	}

	return nil
}

func workingDirFromCommand(cmd *cobra.Command) (string, error) {
	if cmd.Context() != nil {
		if workingDir, ok := cmd.Context().Value(workingDirContextKey).(string); ok && strings.TrimSpace(workingDir) != "" {
			return workingDir, nil
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	return workingDir, nil
}

func absolutePathFromArg(cmd *cobra.Command, value string) (string, error) {
	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}

	return filepath.Clean(filepath.Join(workingDir, value)), nil
}

func repoFromCommand(cmd *cobra.Command) (gitpkg.Repo, error) {
	workingDir, err := workingDirFromCommand(cmd)
	if err != nil {
		return gitpkg.Repo{}, err
	}

	repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
	if err != nil {
		return gitpkg.Repo{}, fmt.Errorf("discover git repository: %w", err)
	}

	return repo, nil
}

func loadValidatedRepositoryConfig(ctx context.Context, repoRoot string) (orbitpkg.RepositoryConfig, error) {
	config, err := orbitpkg.LoadRuntimeRepositoryConfig(ctx, repoRoot)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return orbitpkg.RepositoryConfig{}, fmt.Errorf("load repository config: %w", err)
		}

		hostedConfig, hostedErr := loadValidatedHostedRepositoryConfig(ctx, repoRoot)
		if hostedErr != nil {
			return orbitpkg.RepositoryConfig{}, errOrbitNotInitialized
		}
		return hostedConfig, nil
	}

	return validateVisibleRepositoryConfig(ctx, repoRoot, config)
}

func validateLoadedRepositoryConfig(config orbitpkg.RepositoryConfig) (orbitpkg.RepositoryConfig, error) {
	orbitpkg.SortDefinitions(config.Orbits)

	if err := orbitpkg.ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("validate repository config: %w", err)
	}

	return config, nil
}

func validateVisibleRepositoryConfig(_ context.Context, repoRoot string, config orbitpkg.RepositoryConfig) (orbitpkg.RepositoryConfig, error) {
	filteredConfig, err := filterRuntimeVisibleRepositoryConfig(repoRoot, config)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, err
	}

	return validateLoadedRepositoryConfig(filteredConfig)
}

func runtimeManifestPresent(repoRoot string) (bool, error) {
	manifest, err := harnesspkg.LoadManifestFile(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("load harness manifest: %w", err)
	}

	return manifest.Kind == harnesspkg.ManifestKindRuntime, nil
}

func loadValidatedVisibleOrbitConfig(ctx context.Context, repoRoot string) (orbitpkg.RepositoryConfig, error) {
	runtimeManifest, err := runtimeManifestPresent(repoRoot)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, err
	}
	if runtimeManifest {
		return loadValidatedRepositoryConfig(ctx, repoRoot)
	}

	return loadValidatedAuthoringRepositoryConfig(ctx, repoRoot)
}

func loadValidatedAuthoringRepositoryConfig(ctx context.Context, repoRoot string) (orbitpkg.RepositoryConfig, error) {
	config, err := loadValidatedHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("load repository config: %w", err)
	}
	return config, nil
}

func loadValidatedHostedRepositoryConfig(ctx context.Context, repoRoot string) (orbitpkg.RepositoryConfig, error) {
	config, err := orbitpkg.LoadHostedRepositoryConfig(ctx, repoRoot)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("load hosted repository config: %w", err)
	}

	specs, err := orbitpkg.LoadHostedOrbitSpecs(ctx, repoRoot)
	if err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("load hosted orbit specs: %w", err)
	}
	if err := orbitpkg.ValidateHostedRepositoryConfig(config.Global, specs); err != nil {
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("validate repository config: %w", err)
	}

	orbitpkg.SortDefinitions(config.Orbits)
	return config, nil
}

func filterRuntimeVisibleRepositoryConfig(repoRoot string, config orbitpkg.RepositoryConfig) (orbitpkg.RepositoryConfig, error) {
	manifest, err := harnesspkg.LoadManifestFile(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, nil
		}
		return orbitpkg.RepositoryConfig{}, fmt.Errorf("load harness manifest: %w", err)
	}
	if manifest.Kind != harnesspkg.ManifestKindRuntime {
		return config, nil
	}

	definitionsByID := make(map[string]orbitpkg.Definition, len(config.Orbits))
	for _, definition := range config.Orbits {
		definitionsByID[definition.ID] = definition
	}

	filteredDefinitions := make([]orbitpkg.Definition, 0, len(manifest.Members))
	for _, member := range manifest.Members {
		definition, found := definitionsByID[member.OrbitID]
		if !found {
			return orbitpkg.RepositoryConfig{}, fmt.Errorf("runtime member %q is missing hosted definition", member.OrbitID)
		}
		filteredDefinitions = append(filteredDefinitions, definition)
	}

	config.Orbits = filteredDefinitions

	return config, nil
}

func definitionByID(config orbitpkg.RepositoryConfig, orbitID string) (orbitpkg.Definition, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return orbitpkg.Definition{}, fmt.Errorf("validate orbit id: %w", err)
	}

	definition, found := config.OrbitByID(orbitID)
	if !found {
		return orbitpkg.Definition{}, fmt.Errorf("orbit %q not found", orbitID)
	}

	return definition, nil
}

type currentOrbitCommandContext struct {
	Repo       gitpkg.Repo
	Store      statepkg.FSStore
	Config     orbitpkg.RepositoryConfig
	Current    statepkg.CurrentOrbitState
	Definition orbitpkg.Definition
}

func loadCurrentOrbitCommandContext(cmd *cobra.Command) (currentOrbitCommandContext, error) {
	repo, err := repoFromCommand(cmd)
	if err != nil {
		return currentOrbitCommandContext{}, err
	}

	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return currentOrbitCommandContext{}, fmt.Errorf("create state store: %w", err)
	}

	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return currentOrbitCommandContext{}, errors.New("current orbit is not set; run `orbit enter <orbit-id>` first")
		}
		return currentOrbitCommandContext{}, fmt.Errorf("read current orbit state: %w", err)
	}

	config, err := loadValidatedRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return currentOrbitCommandContext{}, err
	}

	definition, err := viewpkg.CurrentDefinition(config, current)
	if err != nil {
		return currentOrbitCommandContext{}, fmt.Errorf("resolve current orbit definition: %w", err)
	}

	return currentOrbitCommandContext{
		Repo:       repo,
		Store:      store,
		Config:     config,
		Current:    current,
		Definition: definition,
	}, nil
}

type orbitTargetResolutionPolicy string

const (
	orbitTargetAuthoringBranchPreferred  orbitTargetResolutionPolicy = "authoring_branch_preferred"
	orbitTargetAuthoringBranchOrExplicit orbitTargetResolutionPolicy = "authoring_branch_or_explicit"
)

type resolvedOrbitTarget struct {
	OrbitID   string
	Source    string
	RepoState orbittemplate.CurrentRepoState
	Explicit  bool
}

const (
	orbitTargetSourceExplicit       = "explicit"
	orbitTargetSourceBranchManifest = "branch_manifest"
	orbitTargetSourceRuntimeCurrent = "runtime_current"
)

func resolveOrbitTarget(
	cmd *cobra.Command,
	repo gitpkg.Repo,
	requestedOrbitID string,
	policy orbitTargetResolutionPolicy,
) (resolvedOrbitTarget, error) {
	state, err := orbittemplate.LoadCurrentRepoState(cmd.Context(), repo.Root)
	if err != nil {
		return resolvedOrbitTarget{}, fmt.Errorf("load current repo state: %w", err)
	}

	explicitOrbitID := strings.TrimSpace(requestedOrbitID)
	if explicitOrbitID != "" {
		if err := ids.ValidateOrbitID(explicitOrbitID); err != nil {
			return resolvedOrbitTarget{}, fmt.Errorf("validate orbit id: %w", err)
		}
	}

	switch policy {
	case orbitTargetAuthoringBranchPreferred:
		if isAuthoringOrbitBranchKind(state.Kind) {
			return resolveAuthoringBranchOrbitTarget(cmd, repo, state, explicitOrbitID)
		}
		if explicitOrbitID != "" {
			return explicitOrbitTarget(explicitOrbitID, state), nil
		}
		return resolveRuntimeCurrentOrbitTarget(repo, state)
	case orbitTargetAuthoringBranchOrExplicit:
		if isAuthoringOrbitBranchKind(state.Kind) {
			return resolveAuthoringBranchOrbitTarget(cmd, repo, state, explicitOrbitID)
		}
		if explicitOrbitID != "" {
			return explicitOrbitTarget(explicitOrbitID, state), nil
		}
		return resolvedOrbitTarget{}, errors.New("target orbit is required outside a single-orbit source/orbit_template branch; pass an orbit id explicitly")
	default:
		return resolvedOrbitTarget{}, fmt.Errorf("unsupported orbit target resolution policy %q", policy)
	}
}

func explicitOrbitTarget(orbitID string, state orbittemplate.CurrentRepoState) resolvedOrbitTarget {
	return resolvedOrbitTarget{
		OrbitID:   orbitID,
		Source:    orbitTargetSourceExplicit,
		RepoState: state,
		Explicit:  true,
	}
}

func resolveAuthoringBranchOrbitTarget(
	cmd *cobra.Command,
	repo gitpkg.Repo,
	state orbittemplate.CurrentRepoState,
	explicitOrbitID string,
) (resolvedOrbitTarget, error) {
	branchOrbitID := strings.TrimSpace(state.OrbitID)
	if branchOrbitID == "" {
		return resolvedOrbitTarget{}, fmt.Errorf("current %s branch does not declare orbit identity; pass --orbit after repairing the branch manifest", state.Kind)
	}
	if err := ids.ValidateOrbitID(branchOrbitID); err != nil {
		return resolvedOrbitTarget{}, fmt.Errorf("validate branch manifest orbit id: %w", err)
	}
	if explicitOrbitID != "" {
		if err := ids.ValidateOrbitID(explicitOrbitID); err != nil {
			return resolvedOrbitTarget{}, fmt.Errorf("validate orbit id: %w", err)
		}
	}
	if explicitOrbitID != "" && explicitOrbitID != branchOrbitID {
		return resolvedOrbitTarget{}, fmt.Errorf(
			"current %s manifest hosts orbit %q; requested --orbit %q does not match",
			state.Kind,
			branchOrbitID,
			explicitOrbitID,
		)
	}

	config, err := loadValidatedAuthoringRepositoryConfig(cmd.Context(), repo.Root)
	if err != nil {
		return resolvedOrbitTarget{}, err
	}
	if len(config.Orbits) != 1 {
		return resolvedOrbitTarget{}, fmt.Errorf("current %s branch must host exactly one orbit; found %d", state.Kind, len(config.Orbits))
	}
	if _, err := definitionByID(config, branchOrbitID); err != nil {
		return resolvedOrbitTarget{}, fmt.Errorf("current %s manifest hosts orbit %q but hosted orbit definition is not usable: %w", state.Kind, branchOrbitID, err)
	}

	return resolvedOrbitTarget{
		OrbitID:   branchOrbitID,
		Source:    orbitTargetSourceBranchManifest,
		RepoState: state,
		Explicit:  explicitOrbitID != "",
	}, nil
}

func resolveRuntimeCurrentOrbitTarget(repo gitpkg.Repo, state orbittemplate.CurrentRepoState) (resolvedOrbitTarget, error) {
	store, err := statepkg.NewFSStore(repo.GitDir)
	if err != nil {
		return resolvedOrbitTarget{}, fmt.Errorf("create state store: %w", err)
	}

	current, err := store.ReadCurrentOrbit()
	if err != nil {
		if errors.Is(err, statepkg.ErrCurrentOrbitNotFound) {
			return resolvedOrbitTarget{}, errors.New("current orbit is not set; run `orbit enter <orbit-id>` first")
		}
		return resolvedOrbitTarget{}, fmt.Errorf("read current orbit state: %w", err)
	}

	return resolvedOrbitTarget{
		OrbitID:   current.Orbit,
		Source:    orbitTargetSourceRuntimeCurrent,
		RepoState: state,
	}, nil
}

func isAuthoringOrbitBranchKind(kind string) bool {
	return kind == harnesspkg.ManifestKindSource || kind == harnesspkg.ManifestKindOrbitTemplate
}

func resolveBriefOrbitID(cmd *cobra.Command, repo gitpkg.Repo, requestedOrbitID string) (string, error) {
	target, err := resolveOrbitTarget(cmd, repo, requestedOrbitID, orbitTargetAuthoringBranchPreferred)
	if err != nil {
		return "", err
	}

	return target.OrbitID, nil
}

func resolveAuthoredTruthOrbitID(cmd *cobra.Command, repo gitpkg.Repo, requestedOrbitID string) (string, error) {
	target, err := resolveOrbitTarget(cmd, repo, requestedOrbitID, orbitTargetAuthoringBranchOrExplicit)
	if err != nil {
		return "", err
	}

	return target.OrbitID, nil
}
