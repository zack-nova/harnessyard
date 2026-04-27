package orbit

import (
	"fmt"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// EnsureHostedMemberSchema upgrades a hosted legacy definition into the hosted
// member schema while preserving the current top-level fields and path scope.
func EnsureHostedMemberSchema(spec OrbitSpec) (OrbitSpec, error) {
	if spec.HasMemberSchema() {
		return spec, nil
	}

	upgraded, err := DefaultHostedMemberSchemaSpec(spec.ID)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("build hosted member schema: %w", err)
	}

	upgraded.Name = spec.Name
	upgraded.Description = spec.Description
	upgraded.SourcePath = spec.SourcePath
	contentMember, err := defaultLegacyContentMember(spec.ID)
	if err != nil {
		return OrbitSpec{}, fmt.Errorf("build legacy content member: %w", err)
	}

	if len(spec.Include) > 0 {
		contentMember.Paths.Include = append([]string(nil), spec.Include...)
	}
	contentMember.Paths.Exclude = append([]string(nil), spec.Exclude...)
	upgraded.Members = []OrbitMember{contentMember}

	return upgraded, nil
}

func defaultLegacyContentMember(orbitID string) (OrbitMember, error) {
	if err := ids.ValidateOrbitID(orbitID); err != nil {
		return OrbitMember{}, fmt.Errorf("validate orbit id: %w", err)
	}

	return OrbitMember{
		Name: orbitID + "-content",
		Role: OrbitMemberSubject,
		Paths: OrbitMemberPaths{
			Include: []string{orbitID + "/**"},
		},
	}, nil
}
