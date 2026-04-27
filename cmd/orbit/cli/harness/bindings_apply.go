package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

// BindingsApplyInput captures the harness-facing contract for reapplying current runtime vars
// to one install-backed orbit.
type BindingsApplyInput struct {
	RepoRoot string
	OrbitID  string
	Force    bool
	Now      time.Time
	Progress func(string) error
}

// BindingsApplyPreviewResult captures one dry-run preview for bindings apply.
type BindingsApplyPreviewResult struct {
	HarnessID     string
	OrbitID       string
	Forced        bool
	ChangedPaths  []string
	Warnings      []string
	DriftFindings []orbittemplate.InstallDriftFinding
}

// BindingsApplyResult captures one real bindings apply run.
type BindingsApplyResult struct {
	Preview      BindingsApplyPreviewResult
	WrittenPaths []string
}

func buildBindingsApplyPreview(
	ctx context.Context,
	input BindingsApplyInput,
) (RuntimeFile, orbittemplate.InstalledTemplateBindingsApplyPreview, error) {
	runtimeFile, _, err := loadInstallBackedBindingsTargets(input.RepoRoot, input.OrbitID, false)
	if err != nil {
		return RuntimeFile{}, orbittemplate.InstalledTemplateBindingsApplyPreview{}, err
	}

	preview, err := orbittemplate.BuildInstalledTemplateBindingsApplyPreview(ctx, orbittemplate.InstalledTemplateBindingsApplyPreviewInput{
		RepoRoot:               input.RepoRoot,
		OrbitID:                input.OrbitID,
		RuntimeInstallOrbitIDs: ActiveInstallOrbitIDs(runtimeFile),
		Now:                    input.Now,
		Progress:               input.Progress,
	})
	if err != nil {
		return RuntimeFile{}, orbittemplate.InstalledTemplateBindingsApplyPreview{}, fmt.Errorf("build bindings apply preview: %w", err)
	}

	return runtimeFile, preview, nil
}

// PreviewBindingsApply previews one bindings reapply for an install-backed orbit without mutating runtime files.
func PreviewBindingsApply(ctx context.Context, input BindingsApplyInput) (BindingsApplyPreviewResult, error) {
	runtimeFile, preview, err := buildBindingsApplyPreview(ctx, input)
	if err != nil {
		return BindingsApplyPreviewResult{}, err
	}
	if len(preview.DriftFindings) > 0 && !input.Force {
		return BindingsApplyPreviewResult{}, bindingsApplyDriftError(input.OrbitID, preview.DriftFindings)
	}

	result := BindingsApplyPreviewResult{
		HarnessID:     runtimeFile.Harness.ID,
		OrbitID:       input.OrbitID,
		Forced:        input.Force,
		ChangedPaths:  append([]string(nil), preview.ChangedPaths...),
		Warnings:      append([]string(nil), preview.Preview.Warnings...),
		DriftFindings: append([]orbittemplate.InstallDriftFinding(nil), preview.DriftFindings...),
	}
	if input.Progress != nil {
		if err := input.Progress("bindings apply complete"); err != nil {
			return BindingsApplyPreviewResult{}, err
		}
	}

	return result, nil
}

// ApplyBindings re-renders and rewrites one install-backed orbit from the current runtime vars file.
func ApplyBindings(ctx context.Context, input BindingsApplyInput) (BindingsApplyResult, error) {
	runtimeFile, preview, err := buildBindingsApplyPreview(ctx, input)
	if err != nil {
		return BindingsApplyResult{}, err
	}
	if len(preview.DriftFindings) > 0 && !input.Force {
		return BindingsApplyResult{}, bindingsApplyDriftError(input.OrbitID, preview.DriftFindings)
	}

	if input.Progress != nil {
		if err := input.Progress("writing install-owned files"); err != nil {
			return BindingsApplyResult{}, err
		}
	}
	writtenPaths, err := orbittemplate.WriteInstalledTemplateBindingsApplyPreview(input.RepoRoot, preview)
	if err != nil {
		return BindingsApplyResult{}, fmt.Errorf("apply bindings: %w", err)
	}

	result := BindingsApplyResult{
		Preview: BindingsApplyPreviewResult{
			HarnessID:     runtimeFile.Harness.ID,
			OrbitID:       input.OrbitID,
			Forced:        input.Force,
			ChangedPaths:  append([]string(nil), preview.ChangedPaths...),
			Warnings:      append([]string(nil), preview.Preview.Warnings...),
			DriftFindings: append([]orbittemplate.InstallDriftFinding(nil), preview.DriftFindings...),
		},
		WrittenPaths: append([]string(nil), writtenPaths...),
	}
	if input.Progress != nil {
		if err := input.Progress("bindings apply complete"); err != nil {
			return BindingsApplyResult{}, err
		}
	}

	return result, nil
}

func bindingsApplyDriftError(orbitID string, findings []orbittemplate.InstallDriftFinding) error {
	parts := make([]string, 0, len(findings))
	for _, finding := range findings {
		parts = append(parts, fmt.Sprintf("%s (%s)", finding.Path, finding.Kind))
	}
	return fmt.Errorf(
		"apply bindings to orbit %q: current runtime has drift: %s; review the install-backed paths and re-run with --force",
		orbitID,
		strings.Join(parts, ", "),
	)
}
