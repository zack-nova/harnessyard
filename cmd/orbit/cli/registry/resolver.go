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

// Resolution is one exact Package Handle Coordinate resolution.
type Resolution struct {
	Coordinate      PackageHandleCoordinate
	RegistryRemote  string
	RegistryRef     string
	PackageType     string
	PackageIdentity string
	PackageStatus   PackageStatus
	SourceRemote    string
	SourceRef       string
	SourceCommit    string
	FromCache       bool
	Warnings        []string
}

// ResolveInput configures registry-backed exact-version resolution.
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

// ResolveExactPackageHandleCoordinate resolves an exact version from fresh registry data,
// falling back to a previously verified cache entry only when the registry source is unavailable.
func ResolveExactPackageHandleCoordinate(ctx context.Context, input ResolveInput) (Resolution, error) {
	if !input.Coordinate.IsExactVersion() {
		return Resolution{}, fmt.Errorf("package handle coordinate %s is not an exact SemVer version", input.Coordinate.String())
	}

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

	data, loadErr := loader.LoadNamespaceIndex(ctx, input.RepoRoot, source, input.Coordinate.Namespace)
	if loadErr != nil {
		cached, cacheErr := ReadVerifiedResolutionCache(cacheRoot, source.Remote, input.Coordinate)
		if cacheErr == nil {
			cached.RegistryRemote = source.Remote
			cached.RegistryRef = source.Ref
			cached.FromCache = true
			cached.Warnings = append(cached.Warnings, fmt.Sprintf("using cached registry resolution for %s because fresh registry data is unavailable: %v", input.Coordinate.String(), loadErr))
			return cached, nil
		}
		return Resolution{}, fmt.Errorf(
			"resolve %s from registry %s: %w",
			input.Coordinate.String(),
			source.Remote,
			errors.Join(loadErr, fmt.Errorf("no verified cache entry is available: %w", cacheErr)),
		)
	}

	resolution, err := ResolveExactVersionFromNamespaceIndex(input.Coordinate, data)
	if err != nil {
		return Resolution{}, err
	}
	resolution.RegistryRemote = source.Remote
	resolution.RegistryRef = source.Ref
	if err := WriteVerifiedResolutionCache(cacheRoot, source.Remote, resolution); err != nil {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("could not write registry resolution cache: %v", err))
	}

	return resolution, nil
}

// ResolveExactVersionFromNamespaceIndex resolves an exact version from one namespace YAML index.
func ResolveExactVersionFromNamespaceIndex(coordinate PackageHandleCoordinate, data []byte) (Resolution, error) {
	if !coordinate.IsExactVersion() {
		return Resolution{}, fmt.Errorf("package handle coordinate %s is not an exact SemVer version", coordinate.String())
	}
	if coordinate.Namespace == "" {
		return Resolution{}, errors.New("exact Package Handle Coordinate resolution requires a namespace")
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
	status, err := normalizePackageStatus(entry.Status)
	if err != nil {
		return Resolution{}, fmt.Errorf("package handle %q: %w", coordinate.Handle(), err)
	}
	version, ok := versionEntryForExactVersion(entry.Versions, coordinate.Version)
	if !ok {
		return Resolution{}, fmt.Errorf("package handle %q is not registered", coordinate.String())
	}

	resolution, err := resolutionFromVersionEntry(coordinate, version)
	if err != nil {
		return Resolution{}, err
	}
	resolution.PackageStatus = status
	if status == PackageStatusDeprecated {
		resolution.Warnings = append(resolution.Warnings, fmt.Sprintf("package handle %s is deprecated", coordinate.Handle()))
	}

	return resolution, nil
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

type packageEntry struct {
	Status   string                  `yaml:"status"`
	Versions map[string]versionEntry `yaml:"versions"`
}

type versionEntry struct {
	PackageType     string             `yaml:"package_type"`
	PackageIdentity string             `yaml:"package_identity"`
	Source          versionSourceEntry `yaml:"source"`
}

type versionSourceEntry struct {
	Remote string                 `yaml:"remote"`
	Ref    string                 `yaml:"ref"`
	Commit string                 `yaml:"commit"`
	Git    *versionSourceGitEntry `yaml:"git"`
}

type versionSourceGitEntry struct {
	Remote string `yaml:"remote"`
	Ref    string `yaml:"ref"`
	Commit string `yaml:"commit"`
}

func packageEntryForName(packages map[string]packageEntry, name string) (packageEntry, bool) {
	for candidateName, entry := range packages {
		if strings.ToLower(strings.TrimSpace(candidateName)) == name {
			return entry, true
		}
	}

	return packageEntry{}, false
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

func resolutionFromVersionEntry(coordinate PackageHandleCoordinate, version versionEntry) (Resolution, error) {
	packageType := strings.ToLower(strings.TrimSpace(version.PackageType))
	switch packageType {
	case ids.PackageTypeOrbit, ids.PackageTypeHarness:
	default:
		return Resolution{}, fmt.Errorf("registry version %s package_type must be %q or %q", coordinate.String(), ids.PackageTypeOrbit, ids.PackageTypeHarness)
	}

	packageIdentity := strings.ToLower(strings.TrimSpace(version.PackageIdentity))
	if _, err := ids.NewPackageIdentity(packageType, packageIdentity, coordinate.Version); err != nil {
		return Resolution{}, fmt.Errorf("registry version %s package_identity: %w", coordinate.String(), err)
	}

	sourceRemote, sourceRef, sourceCommit := version.Source.values()
	if sourceRemote == "" {
		return Resolution{}, fmt.Errorf("registry version %s source.remote must be present", coordinate.String())
	}
	if sourceRef == "" {
		return Resolution{}, fmt.Errorf("registry version %s source.ref must be present", coordinate.String())
	}
	if sourceCommit == "" {
		return Resolution{}, fmt.Errorf("registry version %s source.commit must be present", coordinate.String())
	}
	if !commitPattern.MatchString(sourceCommit) {
		return Resolution{}, fmt.Errorf("registry version %s source.commit must be a full Git commit SHA", coordinate.String())
	}

	return Resolution{
		Coordinate:      coordinate,
		PackageType:     packageType,
		PackageIdentity: packageIdentity,
		SourceRemote:    sourceRemote,
		SourceRef:       sourceRef,
		SourceCommit:    sourceCommit,
	}, nil
}

func (source versionSourceEntry) values() (remote string, ref string, commit string) {
	remote = strings.TrimSpace(source.Remote)
	ref = strings.TrimSpace(source.Ref)
	commit = strings.ToLower(strings.TrimSpace(source.Commit))
	if source.Git != nil {
		if strings.TrimSpace(source.Git.Remote) != "" {
			remote = strings.TrimSpace(source.Git.Remote)
		}
		if strings.TrimSpace(source.Git.Ref) != "" {
			ref = strings.TrimSpace(source.Git.Ref)
		}
		if strings.TrimSpace(source.Git.Commit) != "" {
			commit = strings.ToLower(strings.TrimSpace(source.Git.Commit))
		}
	}

	return remote, ref, commit
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

// WriteVerifiedResolutionCache records a successful fresh exact-version resolution.
func WriteVerifiedResolutionCache(cacheRoot string, registryRemote string, resolution Resolution) error {
	path, err := resolutionCachePath(cacheRoot, registryRemote, resolution.Coordinate)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create registry resolution cache directory: %w", err)
	}

	entry := resolutionCacheEntry{
		SchemaVersion:   resolutionCacheSchemaVersion,
		RegistryRemote:  canonicalRegistryRemote(registryRemote),
		Namespace:       resolution.Coordinate.Namespace,
		Name:            resolution.Coordinate.Name,
		Version:         resolution.Coordinate.Version,
		PackageType:     resolution.PackageType,
		PackageIdentity: resolution.PackageIdentity,
		PackageStatus:   string(resolution.EffectivePackageStatus()),
		SourceRemote:    resolution.SourceRemote,
		SourceRef:       resolution.SourceRef,
		SourceCommit:    resolution.SourceCommit,
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

// ReadVerifiedResolutionCache reads a previously verified exact-version resolution.
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
	if canonicalRegistryRemote(entry.RegistryRemote) != canonicalRegistryRemote(registryRemote) ||
		entry.Namespace != coordinate.Namespace ||
		entry.Name != coordinate.Name ||
		entry.Version != coordinate.Version {
		return Resolution{}, errors.New("registry resolution cache entry does not match requested coordinate")
	}
	status, err := normalizePackageStatus(entry.PackageStatus)
	if err != nil {
		return Resolution{}, fmt.Errorf("cached registry resolution status: %w", err)
	}

	resolution := Resolution{
		Coordinate:      coordinate,
		RegistryRemote:  canonicalRegistryRemote(registryRemote),
		PackageType:     strings.ToLower(strings.TrimSpace(entry.PackageType)),
		PackageIdentity: strings.ToLower(strings.TrimSpace(entry.PackageIdentity)),
		PackageStatus:   status,
		SourceRemote:    strings.TrimSpace(entry.SourceRemote),
		SourceRef:       strings.TrimSpace(entry.SourceRef),
		SourceCommit:    strings.ToLower(strings.TrimSpace(entry.SourceCommit)),
	}
	if err := validateCachedResolution(resolution); err != nil {
		return Resolution{}, err
	}

	return resolution, nil
}

type resolutionCacheEntry struct {
	SchemaVersion   int    `yaml:"schema_version"`
	RegistryRemote  string `yaml:"registry_remote"`
	Namespace       string `yaml:"namespace"`
	Name            string `yaml:"name"`
	Version         string `yaml:"version"`
	PackageType     string `yaml:"package_type"`
	PackageIdentity string `yaml:"package_identity"`
	PackageStatus   string `yaml:"package_status"`
	SourceRemote    string `yaml:"source_remote"`
	SourceRef       string `yaml:"source_ref"`
	SourceCommit    string `yaml:"source_commit"`
}

func validateCachedResolution(resolution Resolution) error {
	switch resolution.PackageType {
	case ids.PackageTypeOrbit, ids.PackageTypeHarness:
	default:
		return fmt.Errorf("cached registry resolution package_type must be %q or %q", ids.PackageTypeOrbit, ids.PackageTypeHarness)
	}
	if _, err := ids.NewPackageIdentity(resolution.PackageType, resolution.PackageIdentity, resolution.Coordinate.Version); err != nil {
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

func resolutionCachePath(cacheRoot string, registryRemote string, coordinate PackageHandleCoordinate) (string, error) {
	trimmedRoot := strings.TrimSpace(cacheRoot)
	if trimmedRoot == "" {
		return "", errors.New("cache root must not be empty")
	}
	if !coordinate.IsExactVersion() {
		return "", fmt.Errorf("registry resolution cache only supports exact versions, got %s", coordinate.String())
	}

	keyBytes := sha256.Sum256([]byte(canonicalRegistryRemote(registryRemote)))
	key := hex.EncodeToString(keyBytes[:])

	return filepath.Join(trimmedRoot, "registry", "resolutions", key, coordinate.Namespace, coordinate.Name, coordinate.Version+".yaml"), nil
}
