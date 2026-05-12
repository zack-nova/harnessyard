package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// MarshalEntryCandidate returns the stable YAML representation of a registry entry candidate.
func MarshalEntryCandidate(candidate EntryCandidate) ([]byte, error) {
	data, err := contractutil.EncodeYAMLDocument(entryCandidateNode(candidate))
	if err != nil {
		return nil, fmt.Errorf("marshal registry entry candidate: %w", err)
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
	remote := strings.TrimSpace(input.SourceRemote)
	heads, err := gitpkg.ListRemoteHeads(ctx, input.RepoRoot, remote)
	if err != nil {
		return fmt.Errorf("validate source remote reachability: %w", err)
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
			return fmt.Errorf("read orbit template manifest: %w", err)
		}
		parsed, err := harnesspkg.ParseManifestFileData(data)
		if err != nil {
			return fmt.Errorf("parse orbit template manifest: %w", err)
		}
		manifest = parsed
		return nil
	}); err != nil {
		return fmt.Errorf("validate source ref resolution: %w", err)
	}
	candidate.Validation.SourceRefResolved = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s resolved to %s", input.SourceRef, candidate.Source.Commit),
	}

	if err := validateRemoteCommitReachable(ctx, input.RepoRoot, remote, candidate.Source.Commit, input.SourceRef); err != nil {
		return fmt.Errorf("validate source commit reachability: %w", err)
	}
	candidate.Validation.SourceCommitReachable = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("%s is reachable from %s", candidate.Source.Commit, input.SourceRef),
	}

	if err := validateOrbitEntryCandidateManifestIdentity(manifest, input.PackageIdentity, input.coordinate.Version); err != nil {
		return fmt.Errorf("validate package identity match: %w", err)
	}
	candidate.Validation.PackageIdentityMatch = EntryCandidateValidationCheck{
		OK:     true,
		Detail: fmt.Sprintf("remote orbit template package matches %s", input.PackageIdentity),
	}

	if err := validateOrbitEntryCandidateInstallPreview(ctx, remote, input.SourceRef, input.Now); err != nil {
		return fmt.Errorf("validate installability: %w", err)
	}
	candidate.Validation.InstallPreview = EntryCandidateValidationCheck{
		OK:     true,
		Detail: "existing orbit template install preview path succeeded",
	}

	return nil
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
