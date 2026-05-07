package harness

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeriveAuditStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		findings []AuditFinding
		expected string
	}{
		{
			name:     "no findings pass",
			findings: []AuditFinding{},
			expected: AuditStatusPass,
		},
		{
			name: "warning findings warn",
			findings: []AuditFinding{
				{Severity: AuditStatusWarn, Kind: "warning"},
			},
			expected: AuditStatusWarn,
		},
		{
			name: "failure findings fail",
			findings: []AuditFinding{
				{Severity: AuditStatusWarn, Kind: "warning"},
				{Severity: AuditStatusFail, Kind: "failure"},
			},
			expected: AuditStatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expected, DeriveAuditStatus(tt.findings))
		})
	}
}

func TestAuditFindingsFromRuntimeCheckMapsFindingSeverity(t *testing.T) {
	t.Parallel()

	findings := auditFindingsFromRuntimeCheck([]CheckFinding{
		{
			Kind:    CheckFindingMissingDefinition,
			OrbitID: "docs",
			Path:    ".harness/orbits/docs.yaml",
			Message: "definition is missing",
		},
		{
			Kind:    CheckFindingUnresolvedBindings,
			OrbitID: "api",
			Path:    ".harness/installs/api.yaml",
			Message: "bindings are unresolved",
		},
	})

	require.Equal(t, []AuditFinding{
		{
			Severity: AuditStatusFail,
			Kind:     "runtime_check_missing_definition",
			Package:  "docs",
			Path:     ".harness/orbits/docs.yaml",
			Message:  "definition is missing",
		},
		{
			Severity: AuditStatusWarn,
			Kind:     "runtime_check_unresolved_bindings",
			Package:  "api",
			Path:     ".harness/installs/api.yaml",
			Message:  "bindings are unresolved",
		},
	}, findings)
}

func TestAuditFindingsFromRuntimeReadinessMapsReasonSeverity(t *testing.T) {
	t.Parallel()

	findings := auditFindingsFromRuntimeReadiness([]ReadinessReason{
		{
			Code:     ReadinessReasonInvalidOrbitSpec,
			Severity: ReadinessReasonSeverityBlocking,
			Message:  "runtime contains a missing or invalid orbit definition",
			OrbitIDs: []string{"docs"},
		},
		{
			Code:     ReadinessReasonAgentsNotComposed,
			Severity: ReadinessReasonSeverityAdvisory,
			Message:  "root AGENTS.md has not been composed for this orbit",
			OrbitIDs: []string{"api"},
		},
	})

	require.Equal(t, []AuditFinding{
		{
			Severity: AuditStatusFail,
			Kind:     "runtime_readiness_invalid_orbit_spec",
			Package:  "docs",
			Message:  "runtime contains a missing or invalid orbit definition",
		},
		{
			Severity: AuditStatusWarn,
			Kind:     "runtime_readiness_agents_not_composed",
			Package:  "api",
			Message:  "root AGENTS.md has not been composed for this orbit",
		},
	}, findings)
}
