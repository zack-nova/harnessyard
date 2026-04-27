package orbittemplate

import "context"

// HumansBackfillInput captures the repo and orbit targeted by one explicit humans backfill.
type HumansBackfillInput struct {
	RepoRoot string
	OrbitID  string
}

// HumansBackfillResult reports the hosted definition updated by one successful humans backfill.
type HumansBackfillResult struct {
	OrbitID        string
	DefinitionPath string
	Status         GuidanceBackfillStatus
	Replacements   []ReplacementSummary
}

// BackfillOrbitHumans extracts one current revision root HUMANS block and writes it back into meta.humans_template.
func BackfillOrbitHumans(ctx context.Context, input HumansBackfillInput) (HumansBackfillResult, error) {
	result, err := backfillOrbitGuidanceTemplate(ctx, backfillOrbitGuidanceTemplateInput{
		RepoRoot:       input.RepoRoot,
		OrbitID:        input.OrbitID,
		RuntimePath:    runtimeHumansRepoPath,
		ContainerLabel: "root HUMANS.md",
		MetaField:      "humans_template",
	})
	if err != nil {
		return HumansBackfillResult{}, err
	}

	return HumansBackfillResult(result), nil
}
