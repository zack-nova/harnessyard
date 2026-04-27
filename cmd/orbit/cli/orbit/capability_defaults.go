package orbit

import (
	"fmt"
	"path"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

// SeedDefaultCapabilityTruth applies the default v0.66 command/local-skill capability
// paths to a newly created hosted orbit without overwriting authored capability blocks.
func SeedDefaultCapabilityTruth(spec OrbitSpec) (OrbitSpec, error) {
	if err := ids.ValidateOrbitID(spec.ID); err != nil {
		return OrbitSpec{}, fmt.Errorf("validate orbit id: %w", err)
	}

	next := spec
	if next.Capabilities == nil {
		next.Capabilities = &OrbitCapabilities{}
	}
	if next.Capabilities.Commands == nil {
		next.Capabilities.Commands = &OrbitCommandCapabilityPaths{
			Paths: OrbitMemberPaths{
				Include: []string{path.Join("commands", spec.ID, "**", "*.md")},
			},
		}
	}
	if next.Capabilities.Skills == nil {
		next.Capabilities.Skills = &OrbitSkillCapabilities{}
	}
	if next.Capabilities.Skills.Local == nil {
		next.Capabilities.Skills.Local = &OrbitLocalSkillCapabilityPaths{
			Paths: OrbitMemberPaths{
				Include: []string{path.Join("skills", spec.ID, "*")},
			},
		}
	}
	if err := ValidateHostedOrbitSpec(next); err != nil {
		return OrbitSpec{}, fmt.Errorf("validate default capability truth: %w", err)
	}

	return next, nil
}
