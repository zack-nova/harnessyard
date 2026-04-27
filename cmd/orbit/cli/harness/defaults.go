package harness

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

const defaultHarnessID = "harness"

// DefaultHarnessIDForPath derives a stable harness id from the repo root directory name.
func DefaultHarnessIDForPath(repoRoot string) string {
	base := strings.TrimSpace(filepath.Base(filepath.Clean(repoRoot)))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return defaultHarnessID
	}

	var builder strings.Builder
	lastWasSeparator := false
	for _, value := range strings.ToLower(base) {
		switch {
		case value >= 'a' && value <= 'z':
			builder.WriteRune(value)
			lastWasSeparator = false
		case value >= '0' && value <= '9':
			builder.WriteRune(value)
			lastWasSeparator = false
		case value == '-' || value == '_':
			builder.WriteRune(value)
			lastWasSeparator = false
		default:
			if !lastWasSeparator && builder.Len() > 0 {
				builder.WriteRune('-')
				lastWasSeparator = true
			}
		}
	}

	candidate := strings.Trim(builder.String(), "-_")
	if candidate == "" {
		return defaultHarnessID
	}
	if err := ids.ValidateOrbitID(candidate); err != nil {
		return defaultHarnessID
	}

	return candidate
}

// DefaultRuntimeFile builds the default runtime control file for harness init/create.
func DefaultRuntimeFile(repoRoot string, now time.Time) (RuntimeFile, error) {
	harnessID := DefaultHarnessIDForPath(repoRoot)
	if err := ids.ValidateOrbitID(harnessID); err != nil {
		return RuntimeFile{}, fmt.Errorf("build default harness id: %w", err)
	}

	name := strings.TrimSpace(filepath.Base(filepath.Clean(repoRoot)))
	file := RuntimeFile{
		SchemaVersion: runtimeSchemaVersion,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        harnessID,
			CreatedAt: now.UTC(),
			UpdatedAt: now.UTC(),
		},
		Members: []RuntimeMember{},
	}
	if name != "" && name != "." && name != string(filepath.Separator) {
		file.Harness.Name = name
	}

	return file, nil
}

// DefaultRuntimeManifestFile builds the default single-control-plane runtime manifest for harness init/create.
func DefaultRuntimeManifestFile(repoRoot string, now time.Time) (ManifestFile, error) {
	runtimeFile, err := DefaultRuntimeFile(repoRoot, now)
	if err != nil {
		return ManifestFile{}, err
	}

	return ManifestFileFromRuntimeFile(runtimeFile), nil
}

// ManifestFileFromRuntimeFile converts the legacy runtime control document into the single-control-plane runtime manifest.
func ManifestFileFromRuntimeFile(file RuntimeFile) ManifestFile {
	manifest := ManifestFile{
		SchemaVersion: manifestSchemaVersion,
		Kind:          ManifestKindRuntime,
		Runtime: &ManifestRuntimeMetadata{
			Package:   ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: file.Harness.ID},
			ID:        file.Harness.ID,
			Name:      file.Harness.Name,
			CreatedAt: file.Harness.CreatedAt,
			UpdatedAt: file.Harness.UpdatedAt,
		},
		Members: make([]ManifestMember, 0, len(file.Members)),
	}

	for _, member := range file.Members {
		manifestMember := ManifestMember{
			Package:        ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: member.OrbitID},
			OrbitID:        member.OrbitID,
			AddedAt:        member.AddedAt,
			OwnerHarnessID: member.OwnerHarnessID,
		}
		if member.OwnerHarnessID != "" {
			manifestMember.IncludedIn = &ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: member.OwnerHarnessID}
		}
		manifestMember.LastStandaloneOrigin = cloneTemplateSource(member.LastStandaloneOrigin)
		switch member.Source {
		case MemberSourceManual:
			manifestMember.Source = ManifestMemberSourceManual
		case MemberSourceInstallBundle:
			manifestMember.Source = ManifestMemberSourceInstallBundle
		default:
			manifestMember.Source = ManifestMemberSourceInstallOrbit
		}
		manifest.Members = append(manifest.Members, manifestMember)
	}

	return manifest
}

// RuntimeFileFromManifestFile converts a runtime manifest into the legacy runtime control document used by transitional commands.
func RuntimeFileFromManifestFile(file ManifestFile) (RuntimeFile, error) {
	if err := ValidateRuntimeManifestFile(file); err != nil {
		return RuntimeFile{}, fmt.Errorf("validate runtime manifest file: %w", err)
	}

	runtimeFile := RuntimeFile{
		SchemaVersion: runtimeSchemaVersion,
		Kind:          RuntimeKind,
		Harness: RuntimeMetadata{
			ID:        ensureHarnessPackageIdentity(file.Runtime.Package, file.Runtime.ID).Name,
			Name:      file.Runtime.Name,
			CreatedAt: file.Runtime.CreatedAt,
			UpdatedAt: file.Runtime.UpdatedAt,
		},
		Members: make([]RuntimeMember, 0, len(file.Members)),
	}

	for _, member := range file.Members {
		ownerHarnessID := member.OwnerHarnessID
		if member.IncludedIn != nil {
			ownerHarnessID = member.IncludedIn.Name
		}
		runtimeMember := RuntimeMember{
			OrbitID:        ensureOrbitPackageIdentity(member.Package, member.OrbitID).Name,
			OwnerHarnessID: ownerHarnessID,
			AddedAt:        member.AddedAt,
		}
		runtimeMember.LastStandaloneOrigin = cloneTemplateSource(member.LastStandaloneOrigin)
		switch member.Source {
		case ManifestMemberSourceManual:
			runtimeMember.Source = MemberSourceManual
		case ManifestMemberSourceInstallOrbit:
			runtimeMember.Source = MemberSourceInstallOrbit
		case ManifestMemberSourceInstallBundle:
			runtimeMember.Source = MemberSourceInstallBundle
		default:
			return RuntimeFile{}, fmt.Errorf("members for runtime manifest must use only manual, install_orbit, or install_bundle sources")
		}
		runtimeFile.Members = append(runtimeFile.Members, runtimeMember)
	}

	return runtimeFile, nil
}
