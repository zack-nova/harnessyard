package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"gopkg.in/yaml.v3"
)

var (
	// ErrYankedPackageRequiresOverride identifies a yanked package refused without explicit override.
	ErrYankedPackageRequiresOverride = errors.New("registry package is yanked and requires explicit override")
	// ErrBlockedPackageInstallRefused identifies a blocked package refused by the registry status gate.
	ErrBlockedPackageInstallRefused = errors.New("registry package is blocked and cannot be installed")
)

const (
	DefaultRegistryRemote = "https://github.com/zack-nova/hyard-registry.git"
	DefaultRegistryRef    = "HEAD"

	resolutionCacheSchemaVersion = 1
	namespaceIndexSchemaVersion  = 1
)

// PackageStatus is the package-level install status recorded by the registry.
type PackageStatus string

const (
	PackageStatusActive     PackageStatus = "active"
	PackageStatusDeprecated PackageStatus = "deprecated"
	PackageStatusYanked     PackageStatus = "yanked"
	PackageStatusBlocked    PackageStatus = "blocked"
)

// Source identifies a Git-backed package registry source.
type Source struct {
	Remote string
	Ref    string
}

// Resolution is one Package Handle Coordinate resolution.
type Resolution struct {
	Coordinate         PackageHandleCoordinate
	ResolvedCoordinate PackageHandleCoordinate
	RegistryRemote     string
	RegistryRef        string
	PackageType        string
	PackageIdentity    string
	PackageStatus      PackageStatus
	SourceRemote       string
	SourceRef          string
	SourceCommit       string
	FromCache          bool
	Warnings           []string
}

// ResolveInput configures registry-backed Package Handle Coordinate resolution.
type ResolveInput struct {
	RepoRoot       string
	Coordinate     PackageHandleCoordinate
	RegistrySource Source
	CacheRoot      string
	Loader         NamespaceIndexLoader
}

// InstallGateOptions controls package-status install gating.
type InstallGateOptions struct {
	AllowYanked bool
}

// RequireInstallableResolution enforces registry package-status gates before
// a resolved locator enters the package install bridge.
func RequireInstallableResolution(resolution Resolution, options InstallGateOptions) error {
	switch resolution.EffectivePackageStatus() {
	case PackageStatusActive, PackageStatusDeprecated:
		return nil
	case PackageStatusYanked:
		if options.AllowYanked {
			return nil
		}
		return fmt.Errorf("%w: package handle %s is yanked; pass --allow-yanked to install it anyway", ErrYankedPackageRequiresOverride, resolution.Coordinate.Handle())
	case PackageStatusBlocked:
		return fmt.Errorf("%w: package handle %s is blocked by the registry and has no override", ErrBlockedPackageInstallRefused, resolution.Coordinate.Handle())
	default:
		return fmt.Errorf("package handle %s has unsupported registry status %q", resolution.Coordinate.Handle(), resolution.PackageStatus)
	}
}

// EffectivePackageStatus returns active for older status-less resolution data.
func (resolution Resolution) EffectivePackageStatus() PackageStatus {
	if resolution.PackageStatus == "" {
		return PackageStatusActive
	}

	return resolution.PackageStatus
}

// NamespaceIndexLoader loads one namespace index from a registry source.
type NamespaceIndexLoader interface {
	LoadNamespaceIndex(ctx context.Context, repoRoot string, source Source, namespace string) ([]byte, error)
}

// CuratedIndexLoader loads the curated bare-handle index from a registry source.
type CuratedIndexLoader interface {
	LoadCuratedIndex(ctx context.Context, repoRoot string, source Source) ([]byte, error)
}

// NamespaceIndexLoaderFunc adapts a function into a NamespaceIndexLoader.
type NamespaceIndexLoaderFunc func(ctx context.Context, repoRoot string, source Source, namespace string) ([]byte, error)

// LoadNamespaceIndex implements NamespaceIndexLoader.
func (fn NamespaceIndexLoaderFunc) LoadNamespaceIndex(ctx context.Context, repoRoot string, source Source, namespace string) ([]byte, error) {
	return fn(ctx, repoRoot, source, namespace)
}

// GitNamespaceIndexLoader loads registry namespace indexes through Git plumbing.
type GitNamespaceIndexLoader struct{}

// LoadNamespaceIndex loads packages/<namespace>/index.yaml from a Git registry source.
func (GitNamespaceIndexLoader) LoadNamespaceIndex(ctx context.Context, repoRoot string, source Source, namespace string) ([]byte, error) {
	normalizedSource, err := normalizeRegistrySource(source)
	if err != nil {
		return nil, err
	}
	if err := validateHandleSegment("namespace", namespace, false); err != nil {
		return nil, err
	}

	registryPath := fmt.Sprintf("packages/%s/index.yaml", namespace)
	data, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, normalizedSource.Remote, normalizedSource.Ref, registryPath)
	if err != nil {
		return nil, fmt.Errorf("load registry namespace index %s from %s at %s: %w", registryPath, normalizedSource.Remote, normalizedSource.Ref, err)
	}

	return data, nil
}

// LoadCuratedIndex loads curated/index.yaml from a Git registry source.
func (GitNamespaceIndexLoader) LoadCuratedIndex(ctx context.Context, repoRoot string, source Source) ([]byte, error) {
	normalizedSource, err := normalizeRegistrySource(source)
	if err != nil {
		return nil, err
	}

	const registryPath = "curated/index.yaml"
	data, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, normalizedSource.Remote, normalizedSource.Ref, registryPath)
	if err != nil {
		return nil, fmt.Errorf("load registry curated index %s from %s at %s: %w", registryPath, normalizedSource.Remote, normalizedSource.Ref, err)
	}

	return data, nil
}

// ResolveExactPackageHandleCoordinate resolves an exact version from fresh registry data,
// falling back to a previously verified cache entry only when the registry source is unavailable.
func ResolveExactPackageHandleCoordinate(ctx context.Context, input ResolveInput) (Resolution, error) {
	if !input.Coordinate.IsExactVersion() {
		return Resolution{}, fmt.Errorf("package handle coordinate %s is not an exact SemVer version", input.Coordinate.String())
	}

	return ResolvePackageHandleCoordinate(ctx, input)
}

// ResolvePackageHandleCoordinate resolves a Package Handle Coordinate from fresh registry data,
// falling back to a previously verified cache entry only when the registry source is unavailable.
func ResolvePackageHandleCoordinate(ctx context.Context, input ResolveInput) (Resolution, error) {
	source, err := normalizeRegistrySource(input.RegistrySource)
	if err != nil {
		return Resolution{}, err
	}
	cacheRoot := input.CacheRoot
	if strings.TrimSpace(cacheRoot) == "" {
		cacheRoot, err = DefaultCacheRoot()
		if err != nil {
			return Resolution{}, err
		}
	}
	loader := input.Loader
	if loader == nil {
		loader = GitNamespaceIndexLoader{}
	}

	requestedCoordinate := input.Coordinate
	resolutionCoordinate := requestedCoordinate
	if requestedCoordinate.Namespace == "" {
		curatedLoader, ok := loader.(CuratedIndexLoader)
		if !ok {
			return Resolution{}, fmt.Errorf("package handle coordinate %s requires a curated handle resolver", requestedCoordinate.String())
		}
		data, loadErr := curatedLoader.LoadCuratedIndex(ctx, input.RepoRoot, source)
		if loadErr != nil {
			return cachedResolutionForUnavailableRegistry(cacheRoot, source, requestedCoordinate, loadErr)
		}
		resolutionCoordinate, err = ResolveCuratedHandleFromIndex(requestedCoordinate, data)
		if err != nil {
			return Resolution{}, err
		}
	}

	data, loadErr := loader.LoadNamespaceIndex(ctx, input.RepoRoot, source, resolutionCoordinate.Namespace)
	if loadErr != nil {
		return cachedResolutionForUnavailableRegistry(cacheRoot, source, requestedCoordinate, loadErr)
	}

	resolution, err := ResolveFromNamespaceIndex(resolutionCoordinate, data)
	if err != nil {
		return Resolution{}, err
	}
	resolution.Coordinate = requestedCoordinate
	resolution.RegistryRemote = source.Remote
	resolution.RegistryRef = source.Ref
	if err := WriteVerifiedResolutionCache(cacheRoot, source.Remote, resolution); err != nil {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("could not write registry resolution cache: %v", err))
	}

	return resolution, nil
}

func cachedResolutionForUnavailableRegistry(cacheRoot string, source Source, coordinate PackageHandleCoordinate, loadErr error) (Resolution, error) {
	cached, cacheErr := ReadVerifiedResolutionCache(cacheRoot, source.Remote, coordinate)
	if cacheErr == nil {
		cached.RegistryRemote = source.Remote
		cached.RegistryRef = source.Ref
		cached.FromCache = true
		warning := "using cached registry resolution"
		if !coordinate.IsExactVersion() {
			warning = "using stale cached registry resolution"
		}
		cached.Warnings = append(cached.Warnings, fmt.Sprintf("%s for %s because fresh registry data is unavailable: %v", warning, coordinate.String(), loadErr))
		return cached, nil
	}
	return Resolution{}, fmt.Errorf(
		"resolve %s from registry %s: %w",
		coordinate.String(),
		source.Remote,
		errors.Join(loadErr, fmt.Errorf("no verified cache entry is available: %w", cacheErr)),
	)
}

// ResolveFromNamespaceIndex resolves an exact version or dist-tag from one namespace YAML index.
func ResolveFromNamespaceIndex(coordinate PackageHandleCoordinate, data []byte) (Resolution, error) {
	switch coordinate.Kind {
	case HandleCoordinateExactVersion:
		return ResolveExactVersionFromNamespaceIndex(coordinate, data)
	case HandleCoordinateDistTag:
		return ResolveDistTagFromNamespaceIndex(coordinate, data)
	default:
		return Resolution{}, fmt.Errorf("package handle coordinate %s has unsupported selector kind %q", coordinate.String(), coordinate.Kind)
	}
}

// ResolveExactVersionFromNamespaceIndex resolves an exact version from one namespace YAML index.
func ResolveExactVersionFromNamespaceIndex(coordinate PackageHandleCoordinate, data []byte) (Resolution, error) {
	if !coordinate.IsExactVersion() {
		return Resolution{}, fmt.Errorf("package handle coordinate %s is not an exact SemVer version", coordinate.String())
	}
	if coordinate.Namespace == "" {
		return Resolution{}, errors.New("exact Package Handle Coordinate resolution requires a namespace")
	}

	return resolveVersionFromNamespaceIndex(coordinate, coordinate, data)
}

// ResolveDistTagFromNamespaceIndex resolves an explicit dist-tag from one namespace YAML index.
func ResolveDistTagFromNamespaceIndex(coordinate PackageHandleCoordinate, data []byte) (Resolution, error) {
	if coordinate.Kind != HandleCoordinateDistTag {
		return Resolution{}, fmt.Errorf("package handle coordinate %s is not a dist-tag", coordinate.String())
	}
	if coordinate.Namespace == "" {
		return Resolution{}, errors.New("dist-tag Package Handle Coordinate resolution requires a namespace")
	}

	var index namespaceIndexFile
	if err := yaml.Unmarshal(data, &index); err != nil {
		return Resolution{}, fmt.Errorf("parse registry namespace index for %s: %w", coordinate.Namespace, err)
	}
	if index.SchemaVersion != namespaceIndexSchemaVersion {
		return Resolution{}, fmt.Errorf("registry namespace index schema_version must be %d", namespaceIndexSchemaVersion)
	}
	if strings.ToLower(strings.TrimSpace(index.Namespace)) != coordinate.Namespace {
		return Resolution{}, fmt.Errorf("registry namespace index namespace must be %q", coordinate.Namespace)
	}

	entry, ok := packageEntryForName(index.Packages, coordinate.Name)
	if !ok {
		return Resolution{}, fmt.Errorf("package handle %q is not registered", coordinate.Handle())
	}
	version, ok := distTagVersion(entry.DistTags, coordinate.Tag)
	if !ok {
		return Resolution{}, fmt.Errorf("package handle %q has no registry dist-tag %q", coordinate.Handle(), coordinate.Tag)
	}
	resolvedCoordinate := coordinate
	resolvedCoordinate.Kind = HandleCoordinateExactVersion
	resolvedCoordinate.Version = version
	resolvedCoordinate.Tag = ""

	return resolvePackageEntryVersion(coordinate, resolvedCoordinate, entry)
}

// ResolveCuratedHandleFromIndex resolves a curated bare handle to a namespaced Package Handle Coordinate.
func ResolveCuratedHandleFromIndex(coordinate PackageHandleCoordinate, data []byte) (PackageHandleCoordinate, error) {
	if coordinate.Namespace != "" {
		return PackageHandleCoordinate{}, fmt.Errorf("package handle coordinate %s is already namespaced", coordinate.String())
	}

	var index curatedIndexFile
	if err := yaml.Unmarshal(data, &index); err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("parse registry curated index: %w", err)
	}
	if index.SchemaVersion != namespaceIndexSchemaVersion {
		return PackageHandleCoordinate{}, fmt.Errorf("registry curated index schema_version must be %d", namespaceIndexSchemaVersion)
	}
	entry, ok := curatedEntryForName(index.Curated, coordinate.Name)
	if !ok {
		return PackageHandleCoordinate{}, fmt.Errorf("curated package handle %q is not registered", coordinate.Name)
	}

	target := strings.ToLower(strings.TrimSpace(entry.Target))
	if strings.Contains(target, "@") {
		return PackageHandleCoordinate{}, fmt.Errorf("curated package handle %q target must be a namespaced Package Handle without version or dist-tag", coordinate.Name)
	}
	targetCoordinate, err := ParsePackageHandleCoordinate(target)
	if err != nil {
		return PackageHandleCoordinate{}, fmt.Errorf("curated package handle %q target: %w", coordinate.Name, err)
	}
	if targetCoordinate.Namespace == "" {
		return PackageHandleCoordinate{}, fmt.Errorf("curated package handle %q target must be namespaced", coordinate.Name)
	}
	targetCoordinate.Kind = coordinate.Kind
	targetCoordinate.Version = coordinate.Version
	targetCoordinate.Tag = coordinate.Tag

	return targetCoordinate, nil
}

func resolveVersionFromNamespaceIndex(coordinate PackageHandleCoordinate, resolvedCoordinate PackageHandleCoordinate, data []byte) (Resolution, error) {
	var index namespaceIndexFile
	if err := yaml.Unmarshal(data, &index); err != nil {
		return Resolution{}, fmt.Errorf("parse registry namespace index for %s: %w", coordinate.Namespace, err)
	}
	if index.SchemaVersion != namespaceIndexSchemaVersion {
		return Resolution{}, fmt.Errorf("registry namespace index schema_version must be %d", namespaceIndexSchemaVersion)
	}
	if strings.ToLower(strings.TrimSpace(index.Namespace)) != coordinate.Namespace {
		return Resolution{}, fmt.Errorf("registry namespace index namespace must be %q", coordinate.Namespace)
	}

	entry, ok := packageEntryForName(index.Packages, coordinate.Name)
	if !ok {
		return Resolution{}, fmt.Errorf("package handle %q is not registered", coordinate.Handle())
	}
	return resolvePackageEntryVersion(coordinate, resolvedCoordinate, entry)
}

func resolvePackageEntryVersion(coordinate PackageHandleCoordinate, resolvedCoordinate PackageHandleCoordinate, entry packageEntry) (Resolution, error) {
	status, err := normalizePackageStatus(entry.Status)
	if err != nil {
		return Resolution{}, fmt.Errorf("package handle %q: %w", coordinate.Handle(), err)
	}
	if err := validatePackageEntryHandle(resolvedCoordinate, entry); err != nil {
		return Resolution{}, fmt.Errorf("package handle %q: %w", coordinate.Handle(), err)
	}
	version, ok := versionEntryForExactVersion(entry.Versions, resolvedCoordinate.Version)
	if !ok {
		return Resolution{}, fmt.Errorf("package handle %q is not registered", resolvedCoordinate.String())
	}

	resolution, err := resolutionFromVersionEntry(resolvedCoordinate, entry, version)
	if err != nil {
		return Resolution{}, err
	}
	resolution.Coordinate = coordinate
	resolution.ResolvedCoordinate = resolvedCoordinate
	resolution.PackageStatus = status
	if status == PackageStatusDeprecated {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("package handle %s is deprecated", coordinate.Handle()))
	}

	return resolution, nil
}

func validatePackageEntryHandle(coordinate PackageHandleCoordinate, entry packageEntry) error {
	rawHandle := strings.ToLower(strings.TrimSpace(entry.Handle))
	if rawHandle == "" {
		return nil
	}
	handleCoordinate, err := ParsePackageHandleCoordinate(rawHandle)
	if err != nil {
		return fmt.Errorf("registry package handle field: %w", err)
	}
	if handleCoordinate.Namespace == "" {
		return fmt.Errorf("registry package handle field must be namespaced")
	}
	if handleCoordinate.Handle() != coordinate.Handle() {
		return fmt.Errorf("registry package handle field must be %q, got %q", coordinate.Handle(), handleCoordinate.Handle())
	}

	return nil
}

func normalizePackageStatus(raw string) (PackageStatus, error) {
	status := PackageStatus(strings.ToLower(strings.TrimSpace(raw)))
	switch status {
	case "":
		return PackageStatusActive, nil
	case PackageStatusActive, PackageStatusDeprecated, PackageStatusYanked, PackageStatusBlocked:
		return status, nil
	default:
		return "", fmt.Errorf("unsupported registry status %q", raw)
	}
}

type namespaceIndexFile struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Namespace     string                  `yaml:"namespace"`
	Packages      map[string]packageEntry `yaml:"packages"`
}

type curatedIndexFile struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Curated       map[string]curatedEntry `yaml:"curated"`
}

type curatedEntry struct {
	Target string `yaml:"target"`
}

type packageEntry struct {
	Handle   string                  `yaml:"handle"`
	Status   string                  `yaml:"status"`
	Package  packageDescriptorEntry  `yaml:"package"`
	DistTags map[string]string       `yaml:"dist_tags"`
	Versions map[string]versionEntry `yaml:"versions"`
}

type packageDescriptorEntry struct {
	Type string `yaml:"type"`
	Name string `yaml:"name"`
}

type versionEntry struct {
	Locator versionLocatorEntry `yaml:"locator"`
}

type versionLocatorEntry struct {
	Kind       string `yaml:"kind"`
	Repository string `yaml:"repository"`
	Ref        string `yaml:"ref"`
	Commit     string `yaml:"commit"`
}

func packageEntryForName(packages map[string]packageEntry, name string) (packageEntry, bool) {
	for candidateName, entry := range packages {
		if strings.ToLower(strings.TrimSpace(candidateName)) == name {
			return entry, true
		}
	}

	return packageEntry{}, false
}

func curatedEntryForName(curated map[string]curatedEntry, name string) (curatedEntry, bool) {
	for candidateName, entry := range curated {
		if strings.ToLower(strings.TrimSpace(candidateName)) == name {
			return entry, true
		}
	}

	return curatedEntry{}, false
}

func versionEntryForExactVersion(versions map[string]versionEntry, version string) (versionEntry, bool) {
	for candidateVersion, entry := range versions {
		normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(candidateVersion)), "v")
		if normalized == version {
			return entry, true
		}
	}

	return versionEntry{}, false
}

func distTagVersion(distTags map[string]string, tag string) (string, bool) {
	for candidateTag, candidateVersion := range distTags {
		if strings.ToLower(strings.TrimSpace(candidateTag)) != tag {
			continue
		}
		version := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(candidateVersion)), "v")
		if handleSemverPattern.MatchString(version) {
			return version, true
		}
		return "", false
	}

	return "", false
}

func resolutionFromVersionEntry(coordinate PackageHandleCoordinate, entry packageEntry, version versionEntry) (Resolution, error) {
	packageType := strings.ToLower(strings.TrimSpace(entry.Package.Type))
	switch packageType {
	case ids.PackageTypeOrbit, ids.PackageTypeHarness:
	default:
		return Resolution{}, fmt.Errorf("registry package %s package.type must be %q or %q", coordinate.Handle(), ids.PackageTypeOrbit, ids.PackageTypeHarness)
	}

	packageIdentity := strings.ToLower(strings.TrimSpace(entry.Package.Name))
	if _, err := ids.NewPackageIdentity(packageType, packageIdentity, coordinate.Version); err != nil {
		return Resolution{}, fmt.Errorf("registry package %s package.name: %w", coordinate.Handle(), err)
	}

	sourceRemote, sourceRef, sourceCommit, err := version.Locator.values(coordinate)
	if err != nil {
		return Resolution{}, err
	}
	if sourceRemote == "" {
		return Resolution{}, fmt.Errorf("registry version %s locator.repository must be present", coordinate.String())
	}
	if sourceRef == "" {
		return Resolution{}, fmt.Errorf("registry version %s locator.ref must be present", coordinate.String())
	}
	if sourceCommit == "" {
		return Resolution{}, fmt.Errorf("registry version %s locator.commit must be present", coordinate.String())
	}
	if !commitPattern.MatchString(sourceCommit) {
		return Resolution{}, fmt.Errorf("registry version %s locator.commit must be a full Git commit SHA", coordinate.String())
	}

	return Resolution{
		Coordinate:         coordinate,
		ResolvedCoordinate: coordinate,
		PackageType:        packageType,
		PackageIdentity:    packageIdentity,
		SourceRemote:       sourceRemote,
		SourceRef:          sourceRef,
		SourceCommit:       sourceCommit,
	}, nil
}

func (locator versionLocatorEntry) values(coordinate PackageHandleCoordinate) (remote string, ref string, commit string, err error) {
	kind := strings.ToLower(strings.TrimSpace(locator.Kind))
	if kind != "" && kind != "git" {
		return "", "", "", fmt.Errorf("registry version %s locator.kind must be %q", coordinate.String(), "git")
	}
	remote = strings.TrimSpace(locator.Repository)
	ref = strings.TrimSpace(locator.Ref)
	commit = strings.ToLower(strings.TrimSpace(locator.Commit))

	return remote, ref, commit, nil
}

var commitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func normalizeRegistrySource(source Source) (Source, error) {
	remote := strings.TrimSpace(source.Remote)
	if remote == "" {
		return Source{}, errors.New("registry source remote must not be empty")
	}
	ref := strings.TrimSpace(source.Ref)
	if ref == "" {
		ref = DefaultRegistryRef
	}

	return Source{Remote: canonicalRegistryRemote(remote), Ref: ref}, nil
}

func canonicalRegistryRemote(remote string) string {
	trimmed := strings.TrimSpace(remote)
	if filepath.IsAbs(trimmed) {
		return filepath.Clean(trimmed)
	}

	return trimmed
}

// DefaultCacheRoot returns the OS-native hyard user cache root.
func DefaultCacheRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv("HYARD_CACHE_DIR")); override != "" {
		return filepath.Clean(override), nil
	}

	switch runtime.GOOS {
	case "windows":
		localAppData := strings.TrimSpace(os.Getenv("LocalAppData"))
		if localAppData == "" {
			localAppData = strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		}
		if localAppData == "" {
			return "", errors.New("LocalAppData is not set")
		}
		return filepath.Join(localAppData, "hyard", "Cache"), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		return filepath.Join(home, "Library", "Caches", "hyard"), nil
	default:
		if xdgCacheHome := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdgCacheHome != "" {
			return filepath.Join(xdgCacheHome, "hyard"), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		return filepath.Join(home, ".cache", "hyard"), nil
	}
}

// WriteVerifiedResolutionCache records a successful fresh registry resolution.
func WriteVerifiedResolutionCache(cacheRoot string, registryRemote string, resolution Resolution) error {
	path, err := resolutionCachePath(cacheRoot, registryRemote, resolution.Coordinate)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create registry resolution cache directory: %w", err)
	}

	exactCoordinate := resolution.ExactCoordinate()
	entry := resolutionCacheEntry{
		SchemaVersion:     resolutionCacheSchemaVersion,
		RegistryRemote:    canonicalRegistryRemote(registryRemote),
		Coordinate:        resolution.Coordinate.String(),
		Namespace:         resolution.Coordinate.Namespace,
		Name:              resolution.Coordinate.Name,
		Version:           resolution.Coordinate.Version,
		Tag:               resolution.Coordinate.Tag,
		ResolvedNamespace: exactCoordinate.Namespace,
		ResolvedName:      exactCoordinate.Name,
		ResolvedVersion:   exactCoordinate.Version,
		PackageType:       resolution.PackageType,
		PackageIdentity:   resolution.PackageIdentity,
		PackageStatus:     string(resolution.EffectivePackageStatus()),
		SourceRemote:      resolution.SourceRemote,
		SourceRef:         resolution.SourceRef,
		SourceCommit:      resolution.SourceCommit,
	}
	data, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal registry resolution cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write registry resolution cache: %w", err)
	}

	return nil
}

// ReadVerifiedResolutionCache reads a previously verified registry resolution.
func ReadVerifiedResolutionCache(cacheRoot string, registryRemote string, coordinate PackageHandleCoordinate) (Resolution, error) {
	path, err := resolutionCachePath(cacheRoot, registryRemote, coordinate)
	if err != nil {
		return Resolution{}, err
	}

	//nolint:gosec // Cache path is derived from the configured cache root and normalized coordinate components.
	data, err := os.ReadFile(path)
	if err != nil {
		return Resolution{}, fmt.Errorf("read registry resolution cache: %w", err)
	}
	var entry resolutionCacheEntry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return Resolution{}, fmt.Errorf("parse registry resolution cache: %w", err)
	}
	if entry.SchemaVersion != resolutionCacheSchemaVersion {
		return Resolution{}, fmt.Errorf("registry resolution cache schema_version must be %d", resolutionCacheSchemaVersion)
	}
	if canonicalRegistryRemote(entry.RegistryRemote) != canonicalRegistryRemote(registryRemote) {
		return Resolution{}, errors.New("registry resolution cache entry does not match requested registry remote")
	}
	if !cacheEntryMatchesCoordinate(entry, coordinate) {
		return Resolution{}, errors.New("registry resolution cache entry does not match requested coordinate")
	}
	status, err := normalizePackageStatus(entry.PackageStatus)
	if err != nil {
		return Resolution{}, fmt.Errorf("cached registry resolution status: %w", err)
	}

	resolvedCoordinate := coordinate
	if entry.ResolvedVersion != "" {
		resolvedCoordinate.Namespace = entry.ResolvedNamespace
		resolvedCoordinate.Name = entry.ResolvedName
		resolvedCoordinate.Kind = HandleCoordinateExactVersion
		resolvedCoordinate.Version = entry.ResolvedVersion
		resolvedCoordinate.Tag = ""
	}
	resolution := Resolution{
		Coordinate:         coordinate,
		ResolvedCoordinate: resolvedCoordinate,
		RegistryRemote:     canonicalRegistryRemote(registryRemote),
		PackageType:        strings.ToLower(strings.TrimSpace(entry.PackageType)),
		PackageIdentity:    strings.ToLower(strings.TrimSpace(entry.PackageIdentity)),
		PackageStatus:      status,
		SourceRemote:       strings.TrimSpace(entry.SourceRemote),
		SourceRef:          strings.TrimSpace(entry.SourceRef),
		SourceCommit:       strings.ToLower(strings.TrimSpace(entry.SourceCommit)),
	}
	if err := validateCachedResolution(resolution); err != nil {
		return Resolution{}, err
	}

	return resolution, nil
}

type resolutionCacheEntry struct {
	SchemaVersion     int    `yaml:"schema_version"`
	RegistryRemote    string `yaml:"registry_remote"`
	Coordinate        string `yaml:"coordinate"`
	Namespace         string `yaml:"namespace"`
	Name              string `yaml:"name"`
	Version           string `yaml:"version"`
	Tag               string `yaml:"tag"`
	ResolvedNamespace string `yaml:"resolved_namespace"`
	ResolvedName      string `yaml:"resolved_name"`
	ResolvedVersion   string `yaml:"resolved_version"`
	PackageType       string `yaml:"package_type"`
	PackageIdentity   string `yaml:"package_identity"`
	PackageStatus     string `yaml:"package_status"`
	SourceRemote      string `yaml:"source_remote"`
	SourceRef         string `yaml:"source_ref"`
	SourceCommit      string `yaml:"source_commit"`
}

func validateCachedResolution(resolution Resolution) error {
	switch resolution.PackageType {
	case ids.PackageTypeOrbit, ids.PackageTypeHarness:
	default:
		return fmt.Errorf("cached registry resolution package_type must be %q or %q", ids.PackageTypeOrbit, ids.PackageTypeHarness)
	}
	if _, err := ids.NewPackageIdentity(resolution.PackageType, resolution.PackageIdentity, resolution.ExactCoordinate().Version); err != nil {
		return fmt.Errorf("cached registry resolution package identity: %w", err)
	}
	if resolution.SourceRemote == "" {
		return errors.New("cached registry resolution source_remote must be present")
	}
	if resolution.SourceRef == "" {
		return errors.New("cached registry resolution source_ref must be present")
	}
	if !commitPattern.MatchString(resolution.SourceCommit) {
		return errors.New("cached registry resolution source_commit must be a full Git commit SHA")
	}

	return nil
}

// ExactCoordinate returns the resolved exact namespaced coordinate used for installation.
func (resolution Resolution) ExactCoordinate() PackageHandleCoordinate {
	if resolution.ResolvedCoordinate.IsExactVersion() {
		return resolution.ResolvedCoordinate
	}

	return resolution.Coordinate
}

func resolutionCachePath(cacheRoot string, registryRemote string, coordinate PackageHandleCoordinate) (string, error) {
	trimmedRoot := strings.TrimSpace(cacheRoot)
	if trimmedRoot == "" {
		return "", errors.New("cache root must not be empty")
	}
	keyBytes := sha256.Sum256([]byte(canonicalRegistryRemote(registryRemote)))
	remoteKey := hex.EncodeToString(keyBytes[:])
	coordinateBytes := sha256.Sum256([]byte(coordinate.String()))
	coordinateKey := hex.EncodeToString(coordinateBytes[:])

	return filepath.Join(trimmedRoot, "registry", "resolutions", remoteKey, coordinateKey+".yaml"), nil
}

func cacheEntryMatchesCoordinate(entry resolutionCacheEntry, coordinate PackageHandleCoordinate) bool {
	if strings.TrimSpace(entry.Coordinate) != "" {
		return entry.Coordinate == coordinate.String()
	}

	return entry.Namespace == coordinate.Namespace &&
		entry.Name == coordinate.Name &&
		entry.Version == coordinate.Version &&
		entry.Tag == coordinate.Tag
}
