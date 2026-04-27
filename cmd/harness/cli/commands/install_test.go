package commands

import "testing"

func TestInferRemoteOrbitInstallTargetID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		wantOrbit string
		wantOK    bool
	}{
		{
			name:      "template branch name",
			requested: "orbit-template/docs",
			wantOrbit: "docs",
			wantOK:    true,
		},
		{
			name:      "template full ref",
			requested: "refs/heads/orbit-template/docs",
			wantOrbit: "docs",
			wantOK:    true,
		},
		{
			name:      "source alias cannot infer target",
			requested: "main",
			wantOrbit: "",
			wantOK:    false,
		},
		{
			name:      "harness template ref is not an orbit target",
			requested: "harness-template/workspace",
			wantOrbit: "",
			wantOK:    false,
		},
		{
			name:      "empty orbit id is invalid",
			requested: "orbit-template/",
			wantOrbit: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotOrbit, gotOK := inferRemoteOrbitInstallTargetID(tt.requested)
			if gotOrbit != tt.wantOrbit {
				t.Fatalf("orbit id = %q, want %q", gotOrbit, tt.wantOrbit)
			}
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestClassifyExplicitRemoteTemplateRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		want      explicitRemoteTemplateRefKind
	}{
		{
			name:      "orbit template branch name",
			requested: "orbit-template/docs",
			want:      explicitRemoteTemplateRefOrbit,
		},
		{
			name:      "orbit template full ref",
			requested: "refs/heads/orbit-template/docs",
			want:      explicitRemoteTemplateRefOrbit,
		},
		{
			name:      "harness template branch name",
			requested: "harness-template/workspace",
			want:      explicitRemoteTemplateRefHarness,
		},
		{
			name:      "harness template full ref",
			requested: "refs/heads/harness-template/workspace",
			want:      explicitRemoteTemplateRefHarness,
		},
		{
			name:      "source alias remains unknown",
			requested: "main",
			want:      explicitRemoteTemplateRefUnknown,
		},
		{
			name:      "empty remains unknown",
			requested: "",
			want:      explicitRemoteTemplateRefUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := classifyExplicitRemoteTemplateRef(tt.requested)
			if got != tt.want {
				t.Fatalf("kind = %v, want %v", got, tt.want)
			}
		})
	}
}
