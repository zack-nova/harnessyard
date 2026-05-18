package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	harnesspkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/harness"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
	"gopkg.in/yaml.v3"
)

// EntryCandidateSchemaVersion is the YAML schema version for generated registry entry candidates.
const EntryCandidateSchemaVersion = 1

// EntryCandidateInput describes one registry entry candidate generation request.
type EntryCandidateInput struct {
	RepoRoot        string
	Coordinate      string
	PackageType     string
	PackageIdentity string
	PackageStatus   PackageStatus
	SourceRemote    string
	SourceRef       string
	ExpectedCommit  string
	TargetPath      string
	Now             time.Time
}

// EntryCandidate is the generated YAML proposal authors submit to a registry.
type EntryCandidate struct {
	SchemaVersion   int                      `yaml:"schema_version"`
	TargetPath      string                   `yaml:"target_path"`
	Submittable     bool                     `yaml:"submittable"`
	PackageType     string                   `yaml:"package_type"`
	PackageIdentity string                   `yaml:"package_identity"`
	PackageHandle   string                   `yaml:"package_handle"`
	Version         string                   `yaml:"version"`
	PackageStatus   string                   `yaml:"package_status"`
	Source          EntryCandidateSource     `yaml:"source"`
	Validation      EntryCandidateValidation `yaml:"validation"`
}

// EntryCandidateSource records the commit-pinned install locator evidence.
type EntryCandidateSource struct {
	Remote string `yaml:"remote,omitempty"`
	Ref    string `yaml:"ref"`
	Commit string `yaml:"commit"`
}

// EntryCandidateValidation records the validation evidence for a candidate.
type EntryCandidateValidation struct {
	SourceRemoteReachable EntryCandidateValidationCheck `yaml:"source_remote_reachable"`
	SourceRefResolved     EntryCandidateValidationCheck `yaml:"source_ref_resolved"`
	SourceCommitReachable EntryCandidateValidationCheck `yaml:"source_commit_reachable"`
	PackageIdentityMatch  EntryCandidateValidationCheck `yaml:"package_identity_match"`
	InstallPreview        EntryCandidateValidationCheck `yaml:"install_preview"`
}

// EntryCandidateValidationCheck is one validation evidence row.
type EntryCandidateValidationCheck struct {
	OK     bool   `yaml:"ok"`
	Detail string `yaml:"detail,omitempty"`
}

// BuildOrbitEntryCandidate validates and builds one Orbit Package registry entry candidate.
func BuildOrbitEntryCandidate(ctx context.Context, input EntryCandidateInput) (EntryCandidate, error) {
	normalized, err := normalizeEntryCandidateInput(input, ids.PackageTypeOrbit)
	if err != nil {
		return EntryCandidate{}, err
	}

	candidate := EntryCandidate{
		SchemaVersion:   EntryCandidateSchemaVersion,
		TargetPath:      normalized.TargetPath,
		Submittable:     strings.TrimSpace(normalized.SourceRemote) != "",
		PackageType:     ids.PackageTypeOrbit,
		PackageIdentity: normalized.PackageIdentity,
		PackageHandle:   normalized.coordinate.Handle(),
		Version:         normalized.coordinate.Version,
		PackageStatus:   string(normalized.PackageStatus),
		Source: EntryCandidateSource{
			Remote: strings.TrimSpace(normalized.SourceRemote),
			Ref:    strings.TrimSpace(normalized.SourceRef),
		},
	}

	if candidate.Submittable {
		if err := validateRemoteOrbitEntryCandidate(ctx, normalized, &candidate); err != nil {
			return EntryCandidate{}, err
		}
		return candidate, nil
	}

	if err := validateLocalOrbitEntryCandidate(ctx, normalized, &candidate); err != nil {
		return EntryCandidate{}, err
	}

	return candidate, nil
}

// BuildHarnessEntryCandidate validates and builds one Harness Package registry entry candidate.
func BuildHarnessEntryCandidate(ctx context.Context, input EntryCandidateInput) (EntryCandidate, error) {
	normalized, err := normalizeEntryCandidateInput(input, ids.PackageTypeHarness)
	if err != nil {
		return EntryCandidate{}, err
	}

	candidate := EntryCandidate{
		SchemaVersion:   EntryCandidateSchemaVersion,
		TargetPath:      normalized.TargetPath,
		Submittable:     strings.TrimSpace(normalized.SourceRemote) != "",
		PackageType:     ids.PackageTypeHarness,
		PackageIdentity: normalized.PackageIdentity,
		PackageHandle:   normalized.coordinate.Handle(),
		Version:         normalized.coordinate.Version,
		PackageStatus:   string(normalized.PackageStatus),
		Source: EntryCandidateSource{
			Remote: strings.TrimSpace(normalized.SourceRemote),
			Ref:    strings.TrimSpace(normalized.SourceRef),
		},
	}

	if candidate.Submittable {
		if err := validateRemoteHarnessEntryCandidate(ctx, normalized, &candidate); err != nil {
			return EntryCandidate{}, err
		}
		return candidate, nil
	}

	if err := validateLocalHarnessEntryCandidate(ctx, normalized, &candidate); err != nil {
		return EntryCandidate{}, err
	}

	return candidate, nil
}

// MarshalEntryCandidate returns the stable YAML representation of a registry entry candidate.
func MarshalEntryCandidate(candidate EntryCandidate) ([]byte, error) {
	data, err := contractutil.EncodeYAMLDocument(entryCandidateNode(candidate))
	if err != nil {
		return nil, fmt.Errorf("marshal registry entry candidate: %w", err)
	}

	return data, nil
}

// CatalogIndexDataWithEntryCandidate merges one submittable candidate into the
// official namespace catalog index schema.
func CatalogIndexDataWithEntryCandidate(existing []byte, candidate EntryCandidate) ([]byte, error) {
	coordinate, err := catalogCoordinateFromEntryCandidate(candidate)
	if err != nil {
		return nil, err
	}
	candidate.PackageType = strings.ToLower(strings.TrimSpace(candidate.PackageType))
	candidate.PackageIdentity = strings.ToLower(strings.TrimSpace(candidate.PackageIdentity))
	candidate.Source.Remote = strings.TrimSpace(candidate.Source.Remote)
	candidate.Source.Ref = strings.TrimSpace(candidate.Source.Ref)
	candidate.Source.Commit = strings.ToLower(strings.TrimSpace(candidate.Source.Commit))
	status, err := normalizePackageStatus(candidate.PackageStatus)
	if err != nil {
		return nil, fmt.Errorf("registry entry candidate package_status: %w", err)
	}

	index := namespaceIndexFile{
		SchemaVersion: namespaceIndexSchemaVersion,
		Namespace:     coordinate.Namespace,
		Packages:      map[string]packageEntry{},
	}
	if strings.TrimSpace(string(existing)) != "" {
		if err := yaml.Unmarshal(existing, &index); err != nil {
			return nil, fmt.Errorf("parse registry namespace index: %w", err)
		}
		if index.SchemaVersion != namespaceIndexSchemaVersion {
			return nil, fmt.Errorf("registry namespace index schema_version must be %d", namespaceIndexSchemaVersion)
		}
		if strings.ToLower(strings.TrimSpace(index.Namespace)) != coordinate.Namespace {
			return nil, fmt.Errorf("registry namespace index namespace must be %q", coordinate.Namespace)
		}
		if index.Packages == nil {
			index.Packages = map[string]packageEntry{}
		}
	}

	entry := index.Packages[coordinate.Name]
	if err := validateCatalogEntryUpsert(coordinate, entry, candidate); err != nil {
		return nil, err
	}
	if entry.DistTags == nil {
		entry.DistTags = map[string]string{}
	}
	if entry.Versions == nil {
		entry.Versions = map[string]versionEntry{}
	}

	entry.Handle = coordinate.Handle()
	entry.Status = string(status)
	entry.Package = packageDescriptorEntry{
		Type: candidate.PackageType,
		Name: candidate.PackageIdentity,
	}
	entry.Source = packageSourceEntry{
		Repository: candidate.Source.Remote,
	}
	entry.DistTags["latest"] = coordinate.Version
	entry.Versions[coordinate.Version] = versionEntry{
		Locator: versionLocatorEntry{
			Kind:       "git",
			Repository: candidate.Source.Remote,
			Ref:        candidate.Source.Ref,
			Commit:     strings.ToLower(strings.TrimSpace(candidate.Source.Commit)),
		},
		Validation: versionValidationEntry{
			RemoteRef:       catalogRemoteRef(candidate.Source.Ref),
			Manifest:        harnesspkg.ManifestRepoPath(),
			PackageManifest: catalogPackageManifestPath(candidate),
			PackageIdentity: packageDescriptorEntry{
				Type: candidate.PackageType,
				Name: candidate.PackageIdentity,
			},
		},
	}
	index.Packages[coordinate.Name] = entry

	data, err := contractutil.EncodeYAMLDocument(namespaceIndexNode(index))
	if err != nil {
		return nil, fmt.Errorf("marshal registry namespace index: %w", err)
	}

	return data, nil
}

type normalizedEntryCandidateInput struct {
	EntryCandidateInput
	coordinate PackageHandleCoordinate
}

func normalizeEntryCandidateInput(input EntryCandidateInput, expectedType string) (normalizedEntryCandidateInput, error) {
	coordinate, err := ParsePackageHandleCoordinate(input.Coordinate)
	if err != nil {
		return normalizedEntryCandidateInput{}, fmt.Errorf("parse package handle coordinate: %w", err)
	}
	if coordinate.Namespace == "" {
		return normalizedEntryCandidateInput{}, fmt.Errorf("registry entry candidates require a namespaced Package Handle Coordinate")
	}
	if !coordinate.IsExactVersion() {
		return normalizedEntryCandidateInput{}, fmt.Errorf("registry entry candidate coordinate %s must use an exact SemVer version", coordinate.String())
	}

	packageName, err := normalizeEntryCandidatePackageIdentity(input.PackageIdentity, expectedType, coordinate.Version)
	if err != nil {
		return normalizedEntryCandidateInput{}, err
	}
	status := input.PackageStatus
	if status == "" {
		status = PackageStatusActive
	}
	if _, err := normalizePackageStatus(string(status)); err != nil {
		return normalizedEntryCandidateInput{}, err
	}

	sourceRef := strings.TrimSpace(input.SourceRef)
	if sourceRef == "" {
		return normalizedEntryCandidateInput{}, fmt.Errorf("source ref must not be empty")
	}

	targetPath := strings.TrimSpace(input.TargetPath)
	if targetPath == "" {
		targetPath = fmt.Sprintf("packages/%s/index.yaml", coordinate.Namespace)
	}
	normalizedTargetPath, err := ids.NormalizeRepoRelativePath(targetPath)
	if err != nil {
		return normalizedEntryCandidateInput{}, fmt.Errorf("validate target path: %w", err)
	}

	normalized := input
	normalized.PackageType = expectedType
	normalized.PackageIdentity = packageName
	normalized.PackageStatus = status
	normalized.SourceRef = sourceRef
	normalized.TargetPath = normalizedTargetPath
	if normalized.Now.IsZero() {
		normalized.Now = time.Now().UTC()
	}

	return normalizedEntryCandidateInput{
		EntryCandidateInput: normalized,
		coordinate:          coordinate,
	}, nil
}

func normalizeEntryCandidatePackageIdentity(raw string, packageType string, version string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("package identity must not be empty")
	}

	coordinate, err := ids.ParsePackageCoordinate(trimmed, ids.PackageCoordinateOptions{StrictUserLayer: true})
	if err != nil {
		return "", fmt.Errorf("parse package identity: %w", err)
	}
	switch coordinate.Kind {
	case ids.PackageCoordinateName:
	case ids.PackageCoordinateRelease:
		if coordinate.Version != version {
			return "", fmt.Errorf("package identity version %q must match candidate version %q", coordinate.Version, version)
		}
	default:
		return "", fmt.Errorf("package identity must be a package name or SemVer package coordinate")
	}
	if _, err := ids.NewPackageIdentity(packageType, coordinate.Name, ""); err != nil {
		return "", fmt.Errorf("validate package identity: %w", err)
	}

	return coordinate.Name, nil
}

func validateRemoteOrbitEntryCandidate(ctx context.Context, input normalizedEntryCandidateInput, candidate *EntryCandidate) error {
	manifest, err := validateRemoteEntryCandidateSource(ctx, input, candidate)
	if err != nil {
		return err
	}

	if err := validateOrbitEntryCandidateManifestIdentity(manifest, input.PackageIdentity, input.coordinate.Version); err != nil {
		return fmt.Errorf("validate package identity match: %w", err)
	}
	candidate.Validation.PackageIdentityMatch = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("remote orbit template package matches %s", input.PackageIdentity),
	}

	if err := validateOrbitEntryCandidateInstallPreview(ctx, remoteEntryCandidateSource(input), input.SourceRef, input.Now); err != nil {
		return fmt.Errorf("validate installability: %w", err)
	}
	candidate.Validation.InstallPreview = EntryCandidateValidationCheck{
		OK:     true,
		Detail: "existing orbit template install preview path succeeded",
	}

	return nil
}

func validateRemoteHarnessEntryCandidate(ctx context.Context, input normalizedEntryCandidateInput, candidate *EntryCandidate) error {
	manifest, err := validateRemoteEntryCandidateSource(ctx, input, candidate)
	if err != nil {
		return err
	}

	if err := validateHarnessEntryCandidateManifestIdentity(manifest, input.PackageIdentity, input.coordinate.Version); err != nil {
		return fmt.Errorf("validate package identity match: %w", err)
	}
	candidate.Validation.PackageIdentityMatch = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("remote harness template package matches %s", input.PackageIdentity),
	}

	if err := validateHarnessEntryCandidateInstallPreview(ctx, remoteEntryCandidateSource(input), input.SourceRef, input.Now); err != nil {
		return fmt.Errorf("validate installability: %w", err)
	}
	candidate.Validation.InstallPreview = EntryCandidateValidationCheck{
		OK:     true,
		Detail: "existing harness template install preview path succeeded",
	}

	return nil
}

func validateRemoteEntryCandidateSource(ctx context.Context, input normalizedEntryCandidateInput, candidate *EntryCandidate) (harnesspkg.ManifestFile, error) {
	remote := strings.TrimSpace(input.SourceRemote)
	heads, err := gitpkg.ListRemoteHeads(ctx, input.RepoRoot, remote)
	if err != nil {
		return harnesspkg.ManifestFile{}, fmt.Errorf("validate source remote reachability: %w", err)
	}
	candidate.Validation.SourceRemoteReachable = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("reachable; %d branch heads advertised", len(heads)),
	}

	var manifest harnesspkg.ManifestFile
	if err := gitpkg.WithFetchedRemoteRef(ctx, input.RepoRoot, remote, input.SourceRef, func(tempRef string) error {
		commit, err := gitpkg.ResolveRevision(ctx, input.RepoRoot, tempRef)
		if err != nil {
			return fmt.Errorf("resolve fetched source ref: %w", err)
		}
		expectedCommit := strings.ToLower(strings.TrimSpace(input.ExpectedCommit))
		if expectedCommit != "" && expectedCommit != strings.ToLower(commit) {
			return fmt.Errorf("source ref %q resolved to %s, not expected commit %s", input.SourceRef, commit, expectedCommit)
		}
		candidate.Source.Commit = commit

		data, err := gitpkg.ReadFileAtRev(ctx, input.RepoRoot, tempRef, harnesspkg.ManifestRepoPath())
		if err != nil {
			return fmt.Errorf("read package manifest: %w", err)
		}
		parsed, err := harnesspkg.ParseManifestFileData(data)
		if err != nil {
			return fmt.Errorf("parse package manifest: %w", err)
		}
		manifest = parsed
		return nil
	}); err != nil {
		return harnesspkg.ManifestFile{}, fmt.Errorf("validate source ref resolution: %w", err)
	}
	candidate.Validation.SourceRefResolved = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s resolved to %s", input.SourceRef, candidate.Source.Commit),
	}

	if err := validateRemoteCommitReachable(ctx, input.RepoRoot, remote, candidate.Source.Commit, input.SourceRef); err != nil {
		return harnesspkg.ManifestFile{}, fmt.Errorf("validate source commit reachability: %w", err)
	}
	candidate.Validation.SourceCommitReachable = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s is reachable from %s", candidate.Source.Commit, input.SourceRef),
	}

	return manifest, nil
}

func remoteEntryCandidateSource(input normalizedEntryCandidateInput) string {
	return strings.TrimSpace(input.SourceRemote)
}

func validateLocalOrbitEntryCandidate(ctx context.Context, input normalizedEntryCandidateInput, candidate *EntryCandidate) error {
	source, err := orbittemplate.ResolveLocalTemplateSource(ctx, input.RepoRoot, input.SourceRef)
	if err != nil {
		return fmt.Errorf("validate local orbit template source: %w", err)
	}
	candidate.Source.Commit = source.Commit
	candidate.Validation.SourceRemoteReachable = EntryCandidateValidationCheck{
		OK:     false,
		Detail: "local-only preview has no source Git remote",
	}
	candidate.Validation.SourceRefResolved = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s resolved locally to %s", input.SourceRef, source.Commit),
	}
	candidate.Validation.SourceCommitReachable = EntryCandidateValidationCheck{
		OK:     false,
		Detail: "local-only preview cannot prove remote commit reachability",
	}
	if source.Manifest.Template.OrbitID != input.PackageIdentity {
		return fmt.Errorf("validate package identity match: local orbit template package is %q, not %q", source.Manifest.Template.OrbitID, input.PackageIdentity)
	}
	candidate.Validation.PackageIdentityMatch = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("local orbit template package matches %s", input.PackageIdentity),
	}
	candidate.Validation.InstallPreview = EntryCandidateValidationCheck{
		OK:     true,
		Detail: "local orbit template source validation succeeded",
	}

	return nil
}

func validateLocalHarnessEntryCandidate(ctx context.Context, input normalizedEntryCandidateInput, candidate *EntryCandidate) error {
	source, err := harnesspkg.ResolveLocalTemplateInstallSource(ctx, input.RepoRoot, input.SourceRef)
	if err != nil {
		return fmt.Errorf("validate local harness template source: %w", err)
	}
	candidate.Source.Commit = source.Commit
	candidate.Validation.SourceRemoteReachable = EntryCandidateValidationCheck{
		OK:     false,
		Detail: "local-only preview has no source Git remote",
	}
	candidate.Validation.SourceRefResolved = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s resolved locally to %s", input.SourceRef, source.Commit),
	}
	candidate.Validation.SourceCommitReachable = EntryCandidateValidationCheck{
		OK:     false,
		Detail: "local-only preview cannot prove remote commit reachability",
	}
	if source.Manifest.Template.HarnessID != input.PackageIdentity {
		return fmt.Errorf("validate package identity match: local harness template package is %q, not %q", source.Manifest.Template.HarnessID, input.PackageIdentity)
	}
	candidate.Validation.PackageIdentityMatch = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("local harness template package matches %s", input.PackageIdentity),
	}
	candidate.Validation.InstallPreview = EntryCandidateValidationCheck{
		OK:     true,
		Detail: "local harness template source validation succeeded",
	}

	return nil
}

func validateRemoteCommitReachable(ctx context.Context, repoRoot string, remote string, commit string, sourceRef string) error {
	if err := gitpkg.WithFetchedRemoteRevisionOrRef(ctx, repoRoot, remote, commit, sourceRef, func(tempRef string) error {
		resolved, err := gitpkg.ResolveRevision(ctx, repoRoot, tempRef)
		if err != nil {
			return fmt.Errorf("resolve fetched source commit: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(resolved), strings.TrimSpace(commit)) {
			return fmt.Errorf("fetched source commit resolved to %s, not %s", resolved, commit)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("fetch source commit from remote: %w", err)
	}

	return nil
}

func validateOrbitEntryCandidateManifestIdentity(manifest harnesspkg.ManifestFile, packageName string, version string) error {
	if manifest.Kind != harnesspkg.ManifestKindOrbitTemplate {
		return fmt.Errorf("%s kind must be %q", harnesspkg.ManifestRepoPath(), harnesspkg.ManifestKindOrbitTemplate)
	}
	if manifest.Template == nil {
		return fmt.Errorf("%s template must be present", harnesspkg.ManifestRepoPath())
	}
	identity := manifest.Template.Package
	if identity.Name == "" {
		identity.Name = manifest.Template.OrbitID
	}
	if identity.Type == "" {
		identity.Type = ids.PackageTypeOrbit
	}
	if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeOrbit, "template.package"); err != nil {
		return fmt.Errorf("validate template package identity: %w", err)
	}
	if identity.Name != packageName {
		return fmt.Errorf("template.package.name is %q, not %q", identity.Name, packageName)
	}
	if identity.Version != "" && identity.Version != version {
		return fmt.Errorf("template.package.version is %q, not %q", identity.Version, version)
	}

	return nil
}

func validateHarnessEntryCandidateManifestIdentity(manifest harnesspkg.ManifestFile, packageName string, version string) error {
	if manifest.Kind != harnesspkg.ManifestKindHarnessTemplate {
		return fmt.Errorf("%s kind must be %q", harnesspkg.ManifestRepoPath(), harnesspkg.ManifestKindHarnessTemplate)
	}
	if manifest.Template == nil {
		return fmt.Errorf("%s template must be present", harnesspkg.ManifestRepoPath())
	}
	identity := manifest.Template.Package
	if identity.Name == "" {
		identity.Name = manifest.Template.HarnessID
	}
	if identity.Type == "" {
		identity.Type = ids.PackageTypeHarness
	}
	if err := ids.ValidatePackageIdentity(identity, ids.PackageTypeHarness, "template.package"); err != nil {
		return fmt.Errorf("validate template package identity: %w", err)
	}
	if identity.Name != packageName {
		return fmt.Errorf("template.package.name is %q, not %q", identity.Name, packageName)
	}
	if identity.Version != "" && identity.Version != version {
		return fmt.Errorf("template.package.version is %q, not %q", identity.Version, version)
	}

	return nil
}

func validateOrbitEntryCandidateInstallPreview(ctx context.Context, remote string, sourceRef string, now time.Time) error {
	tempDir, err := os.MkdirTemp("", "hyard-registry-entry-*")
	if err != nil {
		return fmt.Errorf("create temporary runtime: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if _, err := gitpkg.EnsureRepoRoot(ctx, tempDir); err != nil {
		return fmt.Errorf("initialize temporary runtime repo: %w", err)
	}
	if _, err := harnesspkg.BootstrapRuntimeControlPlane(tempDir, now); err != nil {
		return fmt.Errorf("bootstrap temporary runtime: %w", err)
	}

	_, err = orbittemplate.BuildRemoteTemplateApplyPreview(ctx, orbittemplate.RemoteTemplateApplyPreviewInput{
		RepoRoot:                tempDir,
		RemoteURL:               remote,
		RequestedRef:            sourceRef,
		AllowUnresolvedBindings: true,
		Now:                     now,
	})
	if err != nil {
		return fmt.Errorf("build orbit template install preview: %w", err)
	}

	return nil
}

func validateHarnessEntryCandidateInstallPreview(ctx context.Context, remote string, sourceRef string, now time.Time) error {
	tempDir, err := os.MkdirTemp("", "hyard-registry-entry-*")
	if err != nil {
		return fmt.Errorf("create temporary runtime: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if _, err := gitpkg.EnsureRepoRoot(ctx, tempDir); err != nil {
		return fmt.Errorf("initialize temporary runtime repo: %w", err)
	}
	if _, err := harnesspkg.BootstrapRuntimeControlPlane(tempDir, now); err != nil {
		return fmt.Errorf("bootstrap temporary runtime: %w", err)
	}

	candidate, source, err := harnesspkg.ResolveRemoteTemplateInstallSource(ctx, tempDir, remote, sourceRef)
	if err != nil {
		return fmt.Errorf("resolve harness template install source: %w", err)
	}
	_, err = harnesspkg.BuildTemplateInstallPreview(ctx, harnesspkg.TemplateInstallPreviewInput{
		RepoRoot: tempDir,
		Source:   source,
		InstallSource: orbittemplate.Source{
			SourceKind:     orbittemplate.InstallSourceKindExternalGit,
			SourceRepo:     candidate.RepoURL,
			SourceRef:      candidate.Branch,
			TemplateCommit: source.Commit,
		},
		RequireResolvedBindings: false,
		Now:                     now,
	})
	if err != nil {
		return fmt.Errorf("build harness template install preview: %w", err)
	}

	return nil
}

func entryCandidateNode(candidate EntryCandidate) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(candidate.SchemaVersion))
	contractutil.AppendMapping(root, "target_path", contractutil.StringNode(candidate.TargetPath))
	contractutil.AppendMapping(root, "submittable", contractutil.BoolNode(candidate.Submittable))
	contractutil.AppendMapping(root, "package_type", contractutil.StringNode(candidate.PackageType))
	contractutil.AppendMapping(root, "package_identity", contractutil.StringNode(candidate.PackageIdentity))
	contractutil.AppendMapping(root, "package_handle", contractutil.StringNode(candidate.PackageHandle))
	contractutil.AppendMapping(root, "version", contractutil.StringNode(candidate.Version))
	contractutil.AppendMapping(root, "package_status", contractutil.StringNode(candidate.PackageStatus))

	sourceNode := contractutil.MappingNode()
	if candidate.Source.Remote != "" {
		contractutil.AppendMapping(sourceNode, "remote", contractutil.StringNode(candidate.Source.Remote))
	}
	contractutil.AppendMapping(sourceNode, "ref", contractutil.StringNode(candidate.Source.Ref))
	contractutil.AppendMapping(sourceNode, "commit", contractutil.StringNode(candidate.Source.Commit))
	contractutil.AppendMapping(root, "source", sourceNode)

	validationNode := contractutil.MappingNode()
	contractutil.AppendMapping(validationNode, "source_remote_reachable", entryCandidateCheckNode(candidate.Validation.SourceRemoteReachable))
	contractutil.AppendMapping(validationNode, "source_ref_resolved", entryCandidateCheckNode(candidate.Validation.SourceRefResolved))
	contractutil.AppendMapping(validationNode, "source_commit_reachable", entryCandidateCheckNode(candidate.Validation.SourceCommitReachable))
	contractutil.AppendMapping(validationNode, "package_identity_match", entryCandidateCheckNode(candidate.Validation.PackageIdentityMatch))
	contractutil.AppendMapping(validationNode, "install_preview", entryCandidateCheckNode(candidate.Validation.InstallPreview))
	contractutil.AppendMapping(root, "validation", validationNode)

	return root
}

func entryCandidateCheckNode(check EntryCandidateValidationCheck) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "ok", contractutil.BoolNode(check.OK))
	if check.Detail != "" {
		contractutil.AppendMapping(node, "detail", contractutil.StringNode(check.Detail))
	}
	return node
}

func catalogCoordinateFromEntryCandidate(candidate EntryCandidate) (PackageHandleCoordinate, error) {
	if !candidate.Submittable {
		return PackageHandleCoordinate{}, errors.New("registry catalog indexes require a submittable entry candidate")
	}

	status, err := normalizePackageStatus(candidate.PackageStatus)
	if err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("registry entry candidate package_status: %w", err)
	}
	if status == "" {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate package_status must be present")
	}

	coordinate, err := ParsePackageHandleCoordinate(strings.TrimSpace(candidate.PackageHandle) + "@" + strings.TrimSpace(candidate.Version))
	if err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("parse registry entry candidate package handle: %w", err)
	}
	if coordinate.Namespace == "" {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate package_handle must be namespaced")
	}
	if !coordinate.IsExactVersion() {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate version must be an exact SemVer version")
	}

	expectedTargetPath := fmt.Sprintf("packages/%s/index.yaml", coordinate.Namespace)
	targetPath, err := ids.NormalizeRepoRelativePath(candidate.TargetPath)
	if err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("validate registry entry candidate target_path: %w", err)
	}
	if targetPath != expectedTargetPath {
		return PackageHandleCoordinate{}, fmt.Errorf("registry entry candidate target_path must be %q for catalog index output, got %q", expectedTargetPath, targetPath)
	}

	packageType := strings.ToLower(strings.TrimSpace(candidate.PackageType))
	switch packageType {
	case ids.PackageTypeOrbit, ids.PackageTypeHarness:
	default:
		return PackageHandleCoordinate{}, fmt.Errorf("registry entry candidate package_type must be %q or %q", ids.PackageTypeOrbit, ids.PackageTypeHarness)
	}
	packageIdentity := strings.ToLower(strings.TrimSpace(candidate.PackageIdentity))
	if _, err := ids.NewPackageIdentity(packageType, packageIdentity, coordinate.Version); err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("registry entry candidate package_identity: %w", err)
	}

	if strings.TrimSpace(candidate.Source.Remote) == "" {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate source.remote must be present")
	}
	if strings.TrimSpace(candidate.Source.Ref) == "" {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate source.ref must be present")
	}
	if !commitPattern.MatchString(strings.ToLower(strings.TrimSpace(candidate.Source.Commit))) {
		return PackageHandleCoordinate{}, errors.New("registry entry candidate source.commit must be a full Git commit SHA")
	}

	return coordinate, nil
}

func validateCatalogEntryUpsert(coordinate PackageHandleCoordinate, entry packageEntry, candidate EntryCandidate) error {
	if entry.Handle != "" {
		if err := validatePackageEntryHandle(coordinate, entry); err != nil {
			return err
		}
	}
	existingType := strings.ToLower(strings.TrimSpace(entry.Package.Type))
	if existingType != "" && existingType != candidate.PackageType {
		return fmt.Errorf("registry package %s package.type is %q, not %q", coordinate.Handle(), existingType, candidate.PackageType)
	}
	existingName := strings.ToLower(strings.TrimSpace(entry.Package.Name))
	if existingName != "" && existingName != candidate.PackageIdentity {
		return fmt.Errorf("registry package %s package.name is %q, not %q", coordinate.Handle(), existingName, candidate.PackageIdentity)
	}

	return nil
}

func catalogPackageManifestPath(candidate EntryCandidate) string {
	switch candidate.PackageType {
	case ids.PackageTypeHarness:
		return harnesspkg.TemplateRepoPath()
	default:
		path, err := harnesspkg.OrbitSpecRepoPath(candidate.PackageIdentity)
		if err != nil {
			return ".harness/orbits/" + candidate.PackageIdentity + ".yaml"
		}
		return path
	}
}

func catalogRemoteRef(ref string) string {
	trimmed := strings.TrimSpace(ref)
	if strings.HasPrefix(trimmed, "refs/") {
		return trimmed
	}
	return "refs/heads/" + trimmed
}

func namespaceIndexNode(index namespaceIndexFile) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(index.SchemaVersion))
	contractutil.AppendMapping(root, "namespace", contractutil.StringNode(index.Namespace))

	packagesNode := contractutil.MappingNode()
	for _, packageName := range sortedPackageEntryKeys(index.Packages) {
		contractutil.AppendMapping(packagesNode, packageName, packageEntryNode(index.Packages[packageName]))
	}
	contractutil.AppendMapping(root, "packages", packagesNode)

	return root
}

func packageEntryNode(entry packageEntry) *yaml.Node {
	node := contractutil.MappingNode()
	contractutil.AppendMapping(node, "handle", contractutil.StringNode(entry.Handle))
	contractutil.AppendMapping(node, "status", contractutil.StringNode(entry.Status))

	packageNode := contractutil.MappingNode()
	contractutil.AppendMapping(packageNode, "type", contractutil.StringNode(entry.Package.Type))
	contractutil.AppendMapping(packageNode, "name", contractutil.StringNode(entry.Package.Name))
	contractutil.AppendMapping(node, "package", packageNode)

	if strings.TrimSpace(entry.Source.Repository) != "" {
		sourceNode := contractutil.MappingNode()
		contractutil.AppendMapping(sourceNode, "repository", contractutil.StringNode(entry.Source.Repository))
		contractutil.AppendMapping(node, "source", sourceNode)
	}
	contractutil.AppendMapping(node, "dist_tags", stringMapNode(entry.DistTags))

	versionsNode := contractutil.MappingNode()
	for _, version := range sortedVersionEntryKeys(entry.Versions) {
		contractutil.AppendMapping(versionsNode, version, versionEntryNode(entry.Versions[version]))
	}
	contractutil.AppendMapping(node, "versions", versionsNode)

	return node
}

func versionEntryNode(entry versionEntry) *yaml.Node {
	node := contractutil.MappingNode()

	locatorNode := contractutil.MappingNode()
	contractutil.AppendMapping(locatorNode, "kind", contractutil.StringNode(entry.Locator.Kind))
	contractutil.AppendMapping(locatorNode, "repository", contractutil.StringNode(entry.Locator.Repository))
	contractutil.AppendMapping(locatorNode, "ref", contractutil.StringNode(entry.Locator.Ref))
	contractutil.AppendMapping(locatorNode, "commit", contractutil.StringNode(entry.Locator.Commit))
	contractutil.AppendMapping(node, "locator", locatorNode)

	if hasVersionValidation(entry.Validation) {
		validationNode := contractutil.MappingNode()
		contractutil.AppendMapping(validationNode, "remote_ref", contractutil.StringNode(entry.Validation.RemoteRef))
		contractutil.AppendMapping(validationNode, "manifest", contractutil.StringNode(entry.Validation.Manifest))
		contractutil.AppendMapping(validationNode, "package_manifest", contractutil.StringNode(entry.Validation.PackageManifest))

		identityNode := contractutil.MappingNode()
		contractutil.AppendMapping(identityNode, "type", contractutil.StringNode(entry.Validation.PackageIdentity.Type))
		contractutil.AppendMapping(identityNode, "name", contractutil.StringNode(entry.Validation.PackageIdentity.Name))
		contractutil.AppendMapping(validationNode, "package_identity", identityNode)

		contractutil.AppendMapping(node, "validation", validationNode)
	}

	return node
}

func hasVersionValidation(validation versionValidationEntry) bool {
	return strings.TrimSpace(validation.RemoteRef) != "" ||
		strings.TrimSpace(validation.Manifest) != "" ||
		strings.TrimSpace(validation.PackageManifest) != "" ||
		strings.TrimSpace(validation.PackageIdentity.Type) != "" ||
		strings.TrimSpace(validation.PackageIdentity.Name) != ""
}

func stringMapNode(values map[string]string) *yaml.Node {
	node := contractutil.MappingNode()
	for _, key := range sortedStringMapKeys(values) {
		contractutil.AppendMapping(node, key, contractutil.StringNode(values[key]))
	}
	return node
}

func sortedPackageEntryKeys(values map[string]packageEntry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedVersionEntryKeys(values map[string]versionEntry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// WriteEntryCandidate writes a candidate YAML file, creating parents as needed.
func WriteEntryCandidate(filename string, data []byte) error {
	cleaned := filepath.Clean(filename)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return errors.New("candidate output path must name a file")
	}
	if err := contractutil.AtomicWriteFile(cleaned, data); err != nil {
		return fmt.Errorf("write candidate file atomically: %w", err)
	}

	return nil
}

// WriteCatalogIndexEntry merges a candidate into a namespace catalog index file.
func WriteCatalogIndexEntry(filename string, candidate EntryCandidate) error {
	cleaned := filepath.Clean(filename)
	if cleaned == "." || cleaned == string(filepath.Separator) {
		return errors.New("catalog index output path must name a file")
	}

	existing, err := os.ReadFile(cleaned)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read registry namespace index: %w", err)
	}
	data, err := CatalogIndexDataWithEntryCandidate(existing, candidate)
	if err != nil {
		return err
	}
	if err := contractutil.AtomicWriteFile(cleaned, data); err != nil {
		return fmt.Errorf("write registry namespace index atomically: %w", err)
	}

	return nil
}
