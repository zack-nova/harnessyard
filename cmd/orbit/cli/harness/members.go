package harness

import (
	"context"
	"fmt"
	"sort"
	"time"

	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
)

// MutateMembersResult captures one successful runtime member mutation.
type MutateMembersResult struct {
	ManifestPath string
	Runtime      RuntimeFile
}

// ActiveInstallOrbitIDs returns current manifest-backed install-orbit member ids.
func ActiveInstallOrbitIDs(runtimeFile RuntimeFile) []string {
	ids := make([]string, 0, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		if member.Source != MemberSourceInstallOrbit {
			continue
		}
		ids = append(ids, member.OrbitID)
	}
	sort.Strings(ids)
	return ids
}

// AddManualMember validates one orbit definition and appends it to the harness runtime as a manual member.
func AddManualMember(ctx context.Context, repoRoot string, orbitID string, now time.Time) (MutateMembersResult, error) {
	return addMember(ctx, repoRoot, orbitID, MemberSourceManual, now)
}

// AddInstallMember validates one orbit definition and appends it to the harness runtime as an install-backed member.
func AddInstallMember(ctx context.Context, repoRoot string, orbitID string, now time.Time) (MutateMembersResult, error) {
	return addMember(ctx, repoRoot, orbitID, MemberSourceInstallOrbit, now)
}

// AddBundleMembers validates one or more orbit definitions and appends them to the harness runtime as bundle-backed members.
func AddBundleMembers(ctx context.Context, repoRoot string, ownerHarnessID string, orbitIDs []string, now time.Time) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	for _, orbitID := range orbitIDs {
		if err := validateHostedMemberDefinition(ctx, repoRoot, orbitID); err != nil {
			return MutateMembersResult{}, err
		}
	}

	existing := make(map[string]struct{}, len(runtimeFile.Members))
	for _, member := range runtimeFile.Members {
		existing[member.OrbitID] = struct{}{}
	}
	for _, orbitID := range orbitIDs {
		if _, ok := existing[orbitID]; ok {
			return MutateMembersResult{}, fmt.Errorf("member %q already exists", orbitID)
		}
		existing[orbitID] = struct{}{}
	}

	for _, orbitID := range orbitIDs {
		runtimeFile.Members = append(runtimeFile.Members, RuntimeMember{
			OrbitID:        orbitID,
			Source:         MemberSourceInstallBundle,
			OwnerHarnessID: ownerHarnessID,
			AddedAt:        mutationTime,
		})
	}
	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

// ReplaceBundleMembers replaces one bundle-backed member set with another while keeping other install units intact.
func ReplaceBundleMembers(
	ctx context.Context,
	repoRoot string,
	ownerHarnessID string,
	previousOrbitIDs []string,
	nextOrbitIDs []string,
	now time.Time,
) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	for _, orbitID := range nextOrbitIDs {
		if err := validateHostedMemberDefinition(ctx, repoRoot, orbitID); err != nil {
			return MutateMembersResult{}, err
		}
	}

	previousSet := make(map[string]struct{}, len(previousOrbitIDs))
	for _, orbitID := range previousOrbitIDs {
		previousSet[orbitID] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(nextOrbitIDs))
	for _, orbitID := range nextOrbitIDs {
		if _, ok := nextSet[orbitID]; ok {
			return MutateMembersResult{}, fmt.Errorf("member %q already exists", orbitID)
		}
		nextSet[orbitID] = struct{}{}
	}

	nextMembers := make([]RuntimeMember, 0, len(runtimeFile.Members)-len(previousSet)+len(nextSet))
	for _, member := range runtimeFile.Members {
		if _, replaced := previousSet[member.OrbitID]; replaced {
			continue
		}
		if _, conflict := nextSet[member.OrbitID]; conflict {
			return MutateMembersResult{}, fmt.Errorf("member %q already exists", member.OrbitID)
		}
		nextMembers = append(nextMembers, member)
	}
	for _, orbitID := range nextOrbitIDs {
		nextMembers = append(nextMembers, RuntimeMember{
			OrbitID:        orbitID,
			Source:         MemberSourceInstallBundle,
			OwnerHarnessID: ownerHarnessID,
			AddedAt:        mutationTime,
		})
	}

	runtimeFile.Members = nextMembers
	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

// UpsertInstallMember ensures one install-backed member exists for the target orbit.
// Existing install-backed members are preserved in place while the runtime updated_at timestamp is refreshed.
func UpsertInstallMember(ctx context.Context, repoRoot string, orbitID string, now time.Time) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	if err := validateHostedMemberDefinition(ctx, repoRoot, orbitID); err != nil {
		return MutateMembersResult{}, err
	}

	found := false
	for index, member := range runtimeFile.Members {
		if member.OrbitID != orbitID {
			continue
		}
		if member.Source != MemberSourceInstallOrbit {
			return MutateMembersResult{}, fmt.Errorf("member %q already exists", orbitID)
		}
		runtimeFile.Members[index].Source = MemberSourceInstallOrbit
		found = true
		break
	}
	if !found {
		runtimeFile.Members = append(runtimeFile.Members, RuntimeMember{
			OrbitID: orbitID,
			Source:  MemberSourceInstallOrbit,
			AddedAt: mutationTime,
		})
	}

	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

// ReplaceMemberWithInstall rewrites one existing runtime member in place as an install-backed member.
func ReplaceMemberWithInstall(ctx context.Context, repoRoot string, orbitID string, now time.Time) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	if err := validateHostedMemberDefinition(ctx, repoRoot, orbitID); err != nil {
		return MutateMembersResult{}, err
	}

	found := false
	for index, member := range runtimeFile.Members {
		if member.OrbitID != orbitID {
			continue
		}
		runtimeFile.Members[index] = RuntimeMember{
			OrbitID: orbitID,
			Source:  MemberSourceInstallOrbit,
			AddedAt: mutationTime,
		}
		found = true
		break
	}
	if !found {
		return MutateMembersResult{}, fmt.Errorf("member %q does not exist", orbitID)
	}

	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

func addMember(ctx context.Context, repoRoot string, orbitID string, source string, now time.Time) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	if err := validateHostedMemberDefinition(ctx, repoRoot, orbitID); err != nil {
		return MutateMembersResult{}, err
	}

	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			return MutateMembersResult{}, fmt.Errorf("member %q already exists", orbitID)
		}
	}

	runtimeFile.Members = append(runtimeFile.Members, RuntimeMember{
		OrbitID: orbitID,
		Source:  source,
		AddedAt: mutationTime,
	})
	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

func validateHostedMemberDefinition(ctx context.Context, repoRoot string, orbitID string) error {
	if _, err := orbitpkg.LoadHostedOrbitSpec(ctx, repoRoot, orbitID); err != nil {
		return fmt.Errorf("load orbit definition %q: %w", orbitID, err)
	}

	return nil
}

// RemoveMember removes one declared member from the harness runtime without deleting related definitions or install records.
func RemoveMember(repoRoot string, orbitID string, now time.Time) (MutateMembersResult, error) {
	runtimeFile, err := LoadRuntimeFile(repoRoot)
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("load harness runtime: %w", err)
	}
	mutationTime := resolveMutationTime(now)

	nextMembers := make([]RuntimeMember, 0, len(runtimeFile.Members))
	removed := false
	for _, member := range runtimeFile.Members {
		if member.OrbitID == orbitID {
			removed = true
			continue
		}
		nextMembers = append(nextMembers, member)
	}
	if !removed {
		return MutateMembersResult{}, fmt.Errorf("member %q not found", orbitID)
	}

	runtimeFile.Members = nextMembers
	runtimeFile.Harness.UpdatedAt = mutationTime

	manifestPath, err := WriteManifestFile(repoRoot, ManifestFileFromRuntimeFile(runtimeFile))
	if err != nil {
		return MutateMembersResult{}, fmt.Errorf("write harness manifest: %w", err)
	}

	return MutateMembersResult{
		ManifestPath: manifestPath,
		Runtime:      runtimeFile,
	}, nil
}

func resolveMutationTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}

	return now.UTC()
}
