package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

type sourceBranchDefinitionHost string

const (
	sourceBranchDefinitionHostHosted sourceBranchDefinitionHost = "hosted"
	sourceBranchDefinitionHostLegacy sourceBranchDefinitionHost = "legacy"
)

// InitSourceInput captures optional source authoring bootstrap inputs.
type InitSourceInput struct {
	OrbitID     string
	Name        string
	Description string
	WithSpec    bool
}

// InitTemplateInput captures optional orbit_template authoring bootstrap inputs.
type InitTemplateInput struct {
	OrbitID     string
	Name        string
	Description string
	WithSpec    bool
	Now         time.Time
}

// InitTemplateResult summarizes one orbit_template branch initialization run.
type InitTemplateResult struct {
	RepoRoot      string
	ManifestPath  string
	CurrentBranch string
	OrbitID       string
	Changed       bool
}

type authoringOrbitSelection struct {
	Definition            orbitpkg.Definition
	Host                  sourceBranchDefinitionHost
	CreatedHostedOrbit    bool
	MigratedLegacyOrbit   bool
	RemovedLegacyTemplate bool
}

func resolveCurrentBranch(ctx context.Context, repoRoot string, operation string) (string, error) {
	state, err := LoadCurrentRepoState(ctx, repoRoot)
	if err != nil {
		return "", fmt.Errorf("load current repo state: %w", err)
	}

	return RequireCurrentBranch(state, operation)
}

func currentRevisionKind(repoRoot string) (string, error) {
	kind, err := loadCurrentRevisionManifestKind(repoRoot)
	if err != nil {
		return "", err
	}

	return kind, nil
}

func ensureCompatibleAuthoringRevision(repoRoot string, expectedKind string, operation string) error {
	kind, err := currentRevisionKind(repoRoot)
	switch {
	case err == nil:
		if kind != "plain" && kind != expectedKind {
			return fmt.Errorf("%s requires a plain or %s branch; current revision kind is %q", operation, expectedKind, kind)
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("load %s: %w", branchManifestPath, err)
	}
}

func resolveAuthoringOrbitSelection(
	ctx context.Context,
	repoRoot string,
	orbitID string,
	name string,
	description string,
	withSpec bool,
) (authoringOrbitSelection, error) {
	hostedDefinitions, err := orbitpkg.DiscoverHostedDefinitions(ctx, repoRoot)
	if err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("discover hosted orbit definitions: %w", err)
	}
	if len(hostedDefinitions) > 0 {
		if len(hostedDefinitions) != 1 {
			return authoringOrbitSelection{}, fmt.Errorf("authoring branch must contain exactly one hosted orbit definition")
		}
		selected := hostedDefinitions[0]
		if orbitID != "" && orbitID != selected.ID {
			return authoringOrbitSelection{}, fmt.Errorf("authoring branch already hosts orbit %q; requested --orbit %q does not match", selected.ID, orbitID)
		}

		return authoringOrbitSelection{
			Definition: selected,
			Host:       sourceBranchDefinitionHostHosted,
		}, nil
	}

	legacyDefinitions, err := orbitpkg.DiscoverDefinitions(ctx, repoRoot)
	if err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("discover legacy orbit definitions: %w", err)
	}
	if len(legacyDefinitions) > 0 {
		if len(legacyDefinitions) != 1 {
			return authoringOrbitSelection{}, fmt.Errorf("authoring branch must contain exactly one orbit definition")
		}
		selected := legacyDefinitions[0]
		if orbitID != "" && orbitID != selected.ID {
			return authoringOrbitSelection{}, fmt.Errorf("authoring branch already contains orbit %q; requested --orbit %q does not match", selected.ID, orbitID)
		}

		spec, err := orbitpkg.LoadOrbitSpec(ctx, repoRoot, selected.ID)
		if err != nil {
			return authoringOrbitSelection{}, fmt.Errorf("load legacy orbit spec: %w", err)
		}
		spec.SourcePath = ""
		if _, err := orbitpkg.WriteHostedOrbitSpec(repoRoot, spec); err != nil {
			return authoringOrbitSelection{}, fmt.Errorf("write hosted orbit definition for %q: %w", selected.ID, err)
		}

		return authoringOrbitSelection{
			Definition:          selected,
			Host:                sourceBranchDefinitionHostLegacy,
			MigratedLegacyOrbit: true,
		}, nil
	}

	if orbitID == "" {
		return authoringOrbitSelection{}, fmt.Errorf("authoring init requires --orbit when no hosted orbit definition is present")
	}

	spec, err := orbitpkg.DefaultHostedMemberSchemaSpec(orbitID)
	if err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("build default hosted orbit spec: %w", err)
	}
	if name != "" {
		spec.Name = name
	}
	if description != "" {
		spec.Description = description
	}
	if withSpec {
		spec, err = orbitpkg.AddSpecMember(spec)
		if err != nil {
			return authoringOrbitSelection{}, fmt.Errorf("add spec member: %w", err)
		}
		specDocPath, err := orbitpkg.SpecDocPath(repoRoot, orbitID)
		if err != nil {
			return authoringOrbitSelection{}, fmt.Errorf("build spec doc path: %w", err)
		}
		if _, err := os.Stat(specDocPath); err == nil {
			return authoringOrbitSelection{}, fmt.Errorf("spec doc file %q already exists", specDocPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return authoringOrbitSelection{}, fmt.Errorf("stat spec doc: %w", err)
		}
	}
	spec, err = orbitpkg.SeedDefaultCapabilityTruth(spec)
	if err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("seed default capability truth: %w", err)
	}
	if _, err := orbitpkg.WriteHostedOrbitSpec(repoRoot, spec); err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("write hosted orbit definition: %w", err)
	}

	definition, err := orbitpkg.CompatibilityDefinitionFromOrbitSpec(spec)
	if err != nil {
		return authoringOrbitSelection{}, fmt.Errorf("project hosted orbit definition: %w", err)
	}

	return authoringOrbitSelection{
		Definition:         definition,
		Host:               sourceBranchDefinitionHostHosted,
		CreatedHostedOrbit: true,
	}, nil
}

func writeInitialSpecDocIfRequested(repoRoot string, orbitID string, requested bool, created bool) error {
	if !requested {
		return nil
	}
	if !created {
		return fmt.Errorf("--with-spec requires creating a new hosted orbit definition; orbit %q already exists", orbitID)
	}
	if _, err := orbitpkg.WriteSpecDoc(repoRoot, orbitID); err != nil {
		return fmt.Errorf("write spec doc: %w", err)
	}

	return nil
}

func materializeInitialGuidanceIfCreated(ctx context.Context, repoRoot string, orbitID string, created bool) error {
	if !created {
		return nil
	}
	if err := MaterializeInitialOrbitGuidance(ctx, repoRoot, orbitID); err != nil {
		return fmt.Errorf("materialize initial guidance: %w", err)
	}

	return nil
}

func removeLegacyOrbitArtifacts(repoRoot string, selection authoringOrbitSelection) error {
	if selection.RemovedLegacyTemplate {
		if err := removeLegacyTemplateManifest(repoRoot); err != nil {
			return err
		}
	}
	if selection.Host == sourceBranchDefinitionHostLegacy {
		if err := removeLegacySourceOrbitDefinition(repoRoot, selection.Definition); err != nil {
			return err
		}
	}

	return nil
}

// InitTemplateBranch initializes the current branch as one direct orbit_template authoring branch.
func InitTemplateBranch(ctx context.Context, repoRoot string, input InitTemplateInput) (InitTemplateResult, error) {
	currentBranch, err := resolveCurrentBranch(ctx, repoRoot, "template init")
	if err != nil {
		return InitTemplateResult{}, err
	}
	if err := ensureCompatibleAuthoringRevision(repoRoot, "orbit_template", "template init"); err != nil {
		return InitTemplateResult{}, err
	}

	selection, err := resolveAuthoringOrbitSelection(ctx, repoRoot, input.OrbitID, input.Name, input.Description, input.WithSpec)
	if err != nil {
		return InitTemplateResult{}, err
	}

	legacyTemplateExists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, "HEAD", manifestRelativePath)
	if err != nil {
		return InitTemplateResult{}, fmt.Errorf("check %s at HEAD: %w", manifestRelativePath, err)
	}
	selection.RemovedLegacyTemplate = legacyTemplateExists

	createdFromCommit, err := CurrentCommitOrZero(ctx, repoRoot)
	if err != nil {
		return InitTemplateResult{}, err
	}

	createdAt := input.Now.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	result := InitTemplateResult{
		RepoRoot:      repoRoot,
		ManifestPath:  filepath.Join(repoRoot, filepath.FromSlash(branchManifestPath)),
		CurrentBranch: currentBranch,
		OrbitID:       selection.Definition.ID,
	}

	existingCreatedAt := createdAt
	existingManifest, err := loadCurrentOrbitTemplateBranchManifest(repoRoot)
	switch {
	case err == nil:
		existingCreatedAt = existingManifest.Template.CreatedAt
		if existingManifest.Template.OrbitID == selection.Definition.ID &&
			existingManifest.Template.CreatedFromBranch == currentBranch &&
			existingManifest.Template.CreatedFromCommit == createdFromCommit {
			result.Changed = selection.CreatedHostedOrbit || selection.MigratedLegacyOrbit || selection.RemovedLegacyTemplate
			if err := writeInitialSpecDocIfRequested(repoRoot, selection.Definition.ID, input.WithSpec, selection.CreatedHostedOrbit); err != nil {
				return InitTemplateResult{}, err
			}
			if err := materializeInitialGuidanceIfCreated(ctx, repoRoot, selection.Definition.ID, selection.CreatedHostedOrbit); err != nil {
				return InitTemplateResult{}, err
			}
			if err := removeLegacyOrbitArtifacts(repoRoot, selection); err != nil {
				return InitTemplateResult{}, err
			}
			return result, nil
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return InitTemplateResult{}, fmt.Errorf("load %s: %w", branchManifestPath, err)
	}

	manifest := orbitTemplateBranchManifest{
		SchemaVersion: manifestSchemaVersion,
		Kind:          "orbit_template",
		Template: orbitTemplateBranchManifestSource{
			OrbitID:           selection.Definition.ID,
			DefaultTemplate:   boolPointer(false),
			CreatedFromBranch: currentBranch,
			CreatedFromCommit: createdFromCommit,
			CreatedAt:         existingCreatedAt,
		},
		Variables: map[string]VariableSpec{},
	}

	writtenPath, err := writeOrbitTemplateBranchManifest(repoRoot, manifest)
	if err != nil {
		return InitTemplateResult{}, fmt.Errorf("write %s: %w", branchManifestPath, err)
	}
	result.ManifestPath = writtenPath
	result.Changed = true
	if err := writeInitialSpecDocIfRequested(repoRoot, selection.Definition.ID, input.WithSpec, selection.CreatedHostedOrbit); err != nil {
		return InitTemplateResult{}, err
	}
	if err := materializeInitialGuidanceIfCreated(ctx, repoRoot, selection.Definition.ID, selection.CreatedHostedOrbit); err != nil {
		return InitTemplateResult{}, err
	}

	if err := removeLegacyOrbitArtifacts(repoRoot, selection); err != nil {
		return InitTemplateResult{}, err
	}

	return result, nil
}

func boolPointer(value bool) *bool {
	return &value
}

func writeOrbitTemplateBranchManifest(repoRoot string, manifest orbitTemplateBranchManifest) (string, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(branchManifestPath))
	data, err := contractutil.EncodeYAMLDocument(orbitTemplateBranchManifestNode(manifest))
	if err != nil {
		return "", fmt.Errorf("encode orbit template branch manifest: %w", err)
	}
	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

func orbitTemplateBranchManifestNode(manifest orbitTemplateBranchManifest) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(manifest.SchemaVersion))
	contractutil.AppendMapping(root, "kind", contractutil.StringNode(manifest.Kind))

	templateNode := contractutil.MappingNode()
	contractutil.AppendMapping(templateNode, "package", templatePackageNode(ids.PackageTypeOrbit, manifest.Template.OrbitID))
	if manifest.Template.DefaultTemplate != nil {
		contractutil.AppendMapping(templateNode, "default_template", contractutil.BoolNode(*manifest.Template.DefaultTemplate))
	}
	contractutil.AppendMapping(templateNode, "created_from_branch", contractutil.StringNode(manifest.Template.CreatedFromBranch))
	contractutil.AppendMapping(templateNode, "created_from_commit", contractutil.StringNode(manifest.Template.CreatedFromCommit))
	contractutil.AppendMapping(templateNode, "created_at", contractutil.TimestampNode(manifest.Template.CreatedAt))
	contractutil.AppendMapping(root, "template", templateNode)
	contractutil.AppendMapping(root, "variables", manifestVariablesNode(manifest.Variables))

	return root
}

func templatePackageNode(packageType string, name string) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "type", contractutil.StringNode(packageType))
	contractutil.AppendMapping(node, "name", contractutil.StringNode(name))
	return node
}
