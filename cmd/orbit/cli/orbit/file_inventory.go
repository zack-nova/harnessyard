package orbit

import (
	"fmt"
	"time"

	statepkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/state"
)

// BuildFileInventorySnapshot projects one role-aware orbit runtime plan onto the
// local file inventory ledger shape used under .git/orbit/state/orbits/<id>/.
func BuildFileInventorySnapshot(
	config RepositoryConfig,
	spec OrbitSpec,
	plan ProjectionPlan,
	generatedAt time.Time,
) (statepkg.FileInventorySnapshot, error) {
	if err := ValidateRepositoryConfig(config.Global, config.Orbits); err != nil {
		return statepkg.FileInventorySnapshot{}, fmt.Errorf("validate repository config: %w", err)
	}
	if err := validateOrbitSpecForHost(spec); err != nil {
		return statepkg.FileInventorySnapshot{}, fmt.Errorf("validate orbit spec: %w", err)
	}

	paths := mergeSortedUniquePaths(plan.MetaPaths, plan.SubjectPaths, plan.RulePaths, plan.ProcessPaths, plan.CapabilityPaths)
	entries := make([]statepkg.FileInventoryEntry, 0, len(paths))

	for _, path := range paths {
		classification, err := ClassifyOrbitPath(config, spec, plan, path, true)
		if err != nil {
			return statepkg.FileInventorySnapshot{}, fmt.Errorf("classify inventory path %q: %w", path, err)
		}

		memberName, err := fileInventoryMemberName(spec, path, classification.Role)
		if err != nil {
			return statepkg.FileInventorySnapshot{}, fmt.Errorf("resolve member name for %q: %w", path, err)
		}

		entries = append(entries, statepkg.FileInventoryEntry{
			Path:          path,
			MemberName:    memberName,
			Role:          classification.Role,
			Projection:    classification.Projection,
			OrbitWrite:    classification.OrbitWrite,
			Export:        classification.Export,
			Orchestration: classification.Orchestration,
		})
	}

	return statepkg.FileInventorySnapshot{
		Orbit:       spec.ID,
		GeneratedAt: generatedAt,
		Files:       entries,
	}, nil
}

func fileInventoryMemberName(spec OrbitSpec, normalizedPath string, role string) (string, error) {
	if role == PathRoleMeta || role == PathRoleCapability || !spec.HasMemberSchema() {
		return "", nil
	}

	matchedName := ""
	for _, member := range spec.Members {
		matches, err := pathMatchesMember(member, normalizedPath)
		if err != nil {
			return "", fmt.Errorf("match member %q: %w", orbitMemberIdentityName(member), err)
		}
		if !matches {
			continue
		}
		if matchedName != "" {
			return "", fmt.Errorf("multiple members match path %q", normalizedPath)
		}
		matchedName = orbitMemberIdentityName(member)
	}

	return matchedName, nil
}
