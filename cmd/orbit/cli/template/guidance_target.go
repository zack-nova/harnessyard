package orbittemplate

import "fmt"

// GuidanceTarget identifies one materialized guidance artifact target.
type GuidanceTarget string

const (
	GuidanceTargetAgents    GuidanceTarget = "agents"
	GuidanceTargetHumans    GuidanceTarget = "humans"
	GuidanceTargetBootstrap GuidanceTarget = "bootstrap"
	GuidanceTargetAll       GuidanceTarget = "all"
)

// NormalizeGuidanceTarget validates one user-facing guidance target token.
func NormalizeGuidanceTarget(target GuidanceTarget) (GuidanceTarget, error) {
	switch target {
	case "", GuidanceTargetAll:
		return GuidanceTargetAll, nil
	case GuidanceTargetAgents, GuidanceTargetHumans, GuidanceTargetBootstrap:
		return target, nil
	case "agent":
		return GuidanceTargetAgents, nil
	case "human":
		return GuidanceTargetHumans, nil
	default:
		return "", fmt.Errorf("unsupported guidance target %q", target)
	}
}

// ExpandGuidanceTargets expands one user-facing target selector into concrete artifact targets.
func ExpandGuidanceTargets(target GuidanceTarget) ([]GuidanceTarget, error) {
	normalized, err := NormalizeGuidanceTarget(target)
	if err != nil {
		return nil, err
	}

	switch normalized {
	case GuidanceTargetAll:
		return []GuidanceTarget{GuidanceTargetAgents, GuidanceTargetHumans, GuidanceTargetBootstrap}, nil
	case GuidanceTargetAgents, GuidanceTargetHumans, GuidanceTargetBootstrap:
		return []GuidanceTarget{normalized}, nil
	default:
		return nil, fmt.Errorf("unsupported guidance target %q", normalized)
	}
}
