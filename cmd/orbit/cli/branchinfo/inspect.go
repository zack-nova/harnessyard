package branchinfo

import (
	"context"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

const (
	installRecordsDir      = ".harness/installs"
	runtimeDefinitionsDir  = ".harness/orbits"
	templateDefinitionsDir = ".orbit/orbits"
)

const (
	// MemberCountScopeManifest identifies the legacy member_count/member_ids
	// fields as branch-manifest membership, not authored OrbitSpec members.
	MemberCountScopeManifest = "manifest"
)

type definitionLoader struct {
	definitionsDir string
	parser         func([]byte, string) (orbit.OrbitSpec, error)
}

type installRecordSummary struct {
	ActiveIDs   []string
	DetachedIDs []string
	InvalidIDs  []string
}

// DefinitionMemberSummary captures authored OrbitSpec members for one valid definition.
type DefinitionMemberSummary struct {
	ID          string   `json:"id"`
	MemberCount int      `json:"member_count"`
	MemberIDs   []string `json:"member_ids"`
}

// Inspection captures the stable branch-inspection details surfaced by CLI commands.
type Inspection struct {
	Classification        Classification                   `json:"classification"`
	SourceBranch          string                           `json:"source_branch,omitempty"`
	PublishOrbitID        string                           `json:"publish_orbit_id,omitempty"`
	HarnessID             string                           `json:"harness_id,omitempty"`
	ManifestMemberCount   int                              `json:"manifest_member_count"`
	ManifestMemberIDs     []string                         `json:"manifest_member_ids"`
	MemberCountScope      string                           `json:"member_count_scope"`
	MemberCount           int                              `json:"member_count"`
	MemberIDs             []string                         `json:"member_ids"`
	DefinitionCount       int                              `json:"definition_count"`
	DefinitionIDs         []string                         `json:"definition_ids"`
	DefinitionMemberCount int                              `json:"definition_member_count"`
	DefinitionMembers     []DefinitionMemberSummary        `json:"definition_members"`
	InstallCount          int                              `json:"install_count"`
	InstallIDs            []string                         `json:"install_ids"`
	DetachedInstallCount  int                              `json:"detached_install_count"`
	DetachedInstallIDs    []string                         `json:"detached_install_ids"`
	InvalidInstallCount   int                              `json:"invalid_install_count"`
	InvalidInstallIDs     []string                         `json:"invalid_install_ids"`
	RootGuidance          *harnesspkg.RootGuidanceMetadata `json:"root_guidance,omitempty"`
}

// InspectRevision classifies one revision and loads the documented branch summary fields.
func InspectRevision(ctx context.Context, repoRoot string, rev string) (Inspection, error) {
	classification, err := ClassifyRevision(ctx, repoRoot, rev)
	if err != nil {
		return Inspection{}, err
	}

	installSummary, err := loadInstallRecordSummary(ctx, repoRoot, rev)
	if err != nil {
		return Inspection{}, err
	}

	definitions, err := loadValidDefinitions(ctx, repoRoot, rev)
	if err != nil {
		return Inspection{}, err
	}

	inspection := Inspection{
		Classification:     classification,
		ManifestMemberIDs:  []string{},
		MemberCountScope:   MemberCountScopeManifest,
		MemberIDs:          []string{},
		DefinitionIDs:      definitionIDs(definitions),
		DefinitionMembers:  cloneDefinitionMemberSummaries(definitions),
		InstallIDs:         installSummary.ActiveIDs,
		DetachedInstallIDs: installSummary.DetachedIDs,
		InvalidInstallIDs:  installSummary.InvalidIDs,
	}
	inspection.DefinitionCount = len(inspection.DefinitionIDs)
	inspection.DefinitionMemberCount = definitionMemberCount(definitions)
	inspection.InstallCount = len(inspection.InstallIDs)
	inspection.DetachedInstallCount = len(inspection.DetachedInstallIDs)
	inspection.InvalidInstallCount = len(inspection.InvalidInstallIDs)

	switch classification.Kind {
	case KindSource:
		manifestFile, err := loadManifestFileAtRev(ctx, repoRoot, rev)
		if err != nil {
			return Inspection{}, err
		}

		inspection.SourceBranch = manifestFile.Source.SourceBranch
		inspection.PublishOrbitID = manifestFile.Source.OrbitID
	case KindTemplate:
		if err := populateTemplateInspection(ctx, repoRoot, rev, classification, &inspection); err != nil {
			return Inspection{}, err
		}
	case KindRuntime:
		manifestFile, err := loadManifestFileAtRev(ctx, repoRoot, rev)
		if err != nil {
			return Inspection{}, err
		}

		inspection.HarnessID = manifestFile.Runtime.ID
		inspection.setManifestMembers(manifestFile.Members)
	}

	return inspection, nil
}

func populateTemplateInspection(ctx context.Context, repoRoot string, rev string, classification Classification, inspection *Inspection) error {
	switch classification.TemplateKind {
	case TemplateKindOrbit, TemplateKindHarness:
		manifest, err := loadManifestFileAtRev(ctx, repoRoot, rev)
		if err != nil {
			return err
		}

		if classification.TemplateKind == TemplateKindHarness {
			inspection.HarnessID = manifest.Template.HarnessID
		}
		inspection.setManifestMembers(manifest.Members)
		if classification.TemplateKind == TemplateKindHarness {
			rootGuidance := manifest.RootGuidance
			inspection.RootGuidance = &rootGuidance
		}

		return nil
	case "":
		return nil
	default:
		return fmt.Errorf("unsupported template kind %q", classification.TemplateKind)
	}
}

func loadManifestFileAtRev(ctx context.Context, repoRoot string, rev string) (harnesspkg.ManifestFile, error) {
	data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, rev, manifestRelativePath)
	if err != nil {
		return harnesspkg.ManifestFile{}, fmt.Errorf("read %s at %s: %w", manifestRelativePath, rev, err)
	}

	file, err := harnesspkg.ParseManifestFileData(data)
	if err != nil {
		return harnesspkg.ManifestFile{}, fmt.Errorf("parse %s at %s: %w", manifestRelativePath, rev, err)
	}

	return file, nil
}

func loadValidDefinitions(
	ctx context.Context,
	repoRoot string,
	rev string,
) ([]DefinitionMemberSummary, error) {
	loaders := definitionLoadersForClassification()
	for _, loader := range loaders {
		paths, err := gitpkg.ListFilesAtRev(ctx, repoRoot, rev, loader.definitionsDir)
		if err != nil {
			return nil, fmt.Errorf("list %s at %s: %w", loader.definitionsDir, rev, err)
		}
		if len(paths) == 0 {
			continue
		}

		definitions := make([]DefinitionMemberSummary, 0, len(paths))
		for _, definitionPath := range paths {
			data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, rev, definitionPath)
			if err != nil {
				return nil, fmt.Errorf("read %s at %s: %w", definitionPath, rev, err)
			}

			spec, err := loader.parser(data, definitionPath)
			if err != nil {
				continue
			}
			memberIDs := orbitSpecMemberIDs(spec.Members)
			definitions = append(definitions, DefinitionMemberSummary{
				ID:          spec.ID,
				MemberCount: len(memberIDs),
				MemberIDs:   memberIDs,
			})
		}
		sort.Slice(definitions, func(left, right int) bool {
			return definitions[left].ID < definitions[right].ID
		})

		return definitions, nil
	}

	return nil, nil
}

func definitionLoadersForClassification() []definitionLoader {
	return []definitionLoader{
		{
			definitionsDir: runtimeDefinitionsDir,
			parser:         orbit.ParseHostedOrbitSpecData,
		},
	}
}

func (inspection *Inspection) setManifestMembers(members []harnesspkg.ManifestMember) {
	inspection.ManifestMemberIDs = manifestMemberIDs(members)
	inspection.ManifestMemberCount = len(inspection.ManifestMemberIDs)

	// Deprecated compatibility aliases for the original branch inspect contract.
	inspection.MemberIDs = append([]string{}, inspection.ManifestMemberIDs...)
	inspection.MemberCount = inspection.ManifestMemberCount
}

func loadInstallRecordSummary(ctx context.Context, repoRoot string, rev string) (installRecordSummary, error) {
	paths, err := gitpkg.ListFilesAtRev(ctx, repoRoot, rev, installRecordsDir)
	if err != nil {
		return installRecordSummary{}, fmt.Errorf("list %s at %s: %w", installRecordsDir, rev, err)
	}

	summary := installRecordSummary{
		ActiveIDs:   make([]string, 0, len(paths)),
		DetachedIDs: make([]string, 0),
		InvalidIDs:  make([]string, 0),
	}
	for _, recordPath := range paths {
		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, rev, recordPath)
		if err != nil {
			return installRecordSummary{}, fmt.Errorf("read %s at %s: %w", recordPath, rev, err)
		}

		expectedOrbitID := strings.TrimSuffix(pathpkg.Base(recordPath), pathpkg.Ext(recordPath))
		record, err := orbittemplate.ParseInstallRecordData(data)
		if err != nil {
			summary.InvalidIDs = append(summary.InvalidIDs, expectedOrbitID)
			continue
		}
		if record.OrbitID != expectedOrbitID {
			summary.InvalidIDs = append(summary.InvalidIDs, expectedOrbitID)
			continue
		}

		if orbittemplate.EffectiveInstallRecordStatus(record) == orbittemplate.InstallRecordStatusDetached {
			summary.DetachedIDs = append(summary.DetachedIDs, record.OrbitID)
			continue
		}

		summary.ActiveIDs = append(summary.ActiveIDs, record.OrbitID)
	}

	sort.Strings(summary.ActiveIDs)
	sort.Strings(summary.DetachedIDs)
	sort.Strings(summary.InvalidIDs)

	return summary, nil
}

func definitionIDs(definitions []DefinitionMemberSummary) []string {
	idsList := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		idsList = append(idsList, definition.ID)
	}
	sort.Strings(idsList)

	return idsList
}

func definitionMemberCount(definitions []DefinitionMemberSummary) int {
	count := 0
	for _, definition := range definitions {
		count += definition.MemberCount
	}

	return count
}

func cloneDefinitionMemberSummaries(definitions []DefinitionMemberSummary) []DefinitionMemberSummary {
	cloned := make([]DefinitionMemberSummary, 0, len(definitions))
	for _, definition := range definitions {
		cloned = append(cloned, DefinitionMemberSummary{
			ID:          definition.ID,
			MemberCount: definition.MemberCount,
			MemberIDs:   append([]string{}, definition.MemberIDs...),
		})
	}

	return cloned
}

func orbitSpecMemberIDs(members []orbit.OrbitMember) []string {
	idsList := make([]string, 0, len(members))
	for _, member := range members {
		switch {
		case member.Name != "":
			idsList = append(idsList, member.Name)
		case member.Key != "":
			idsList = append(idsList, member.Key)
		}
	}
	sort.Strings(idsList)

	return idsList
}

func manifestMemberIDs(members []harnesspkg.ManifestMember) []string {
	idsList := make([]string, 0, len(members))
	for _, member := range members {
		idsList = append(idsList, member.OrbitID)
	}
	sort.Strings(idsList)

	return idsList
}
