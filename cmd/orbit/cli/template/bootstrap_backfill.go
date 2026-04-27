package orbittemplate

import (
	"context"
	"fmt"
)

// BootstrapBackfillInput captures the repo and orbit targeted by one explicit bootstrap backfill.
type BootstrapBackfillInput struct {
	RepoRoot string
	OrbitID  string
}

// BootstrapBackfillResult reports the hosted definition updated by one successful bootstrap backfill.
type BootstrapBackfillResult struct {
	OrbitID        string
	DefinitionPath string
	Status         GuidanceBackfillStatus
	Replacements   []ReplacementSummary
}

// BackfillOrbitBootstrap extracts one current revision root BOOTSTRAP block and writes it back into meta.bootstrap_template.
func BackfillOrbitBootstrap(ctx context.Context, input BootstrapBackfillInput) (BootstrapBackfillResult, error) {
	revisionKind, err := resolveAllowedBriefRevisionKind(input.RepoRoot, "backfill")
	if err != nil {
		return BootstrapBackfillResult{}, err
	}
	bootstrapStatus, err := InspectBootstrapOrbitForRevision(ctx, input.RepoRoot, "", input.OrbitID, revisionKind)
	if err != nil {
		return BootstrapBackfillResult{}, err
	}
	if PlanBootstrapGuidanceBackfill(bootstrapStatus, true).ReasonCode == "bootstrap_completed" {
		return BootstrapBackfillResult{}, fmt.Errorf(
			"bootstrap guidance for orbit %q is closed because bootstrap is already completed in this runtime",
			input.OrbitID,
		)
	}

	result, err := backfillOrbitGuidanceTemplate(ctx, backfillOrbitGuidanceTemplateInput{
		RepoRoot:       input.RepoRoot,
		OrbitID:        input.OrbitID,
		RuntimePath:    runtimeBootstrapRepoPath,
		ContainerLabel: "root BOOTSTRAP.md",
		MetaField:      "bootstrap_template",
	})
	if err != nil {
		return BootstrapBackfillResult{}, err
	}

	return BootstrapBackfillResult(result), nil
}
