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
