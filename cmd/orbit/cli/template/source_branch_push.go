package orbittemplate

import (
	"context"
	"fmt"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

// SourceBranchPushInput describes the source branch remote-readiness gate before package publish.
type SourceBranchPushInput struct {
	RepoRoot     string
	Remote       string
	SourceBranch string
	Relation     gitpkg.BranchRelation
	Prompter     SourceBranchPushPrompter
}

// SourceBranchPushResult describes the user-visible result of the source branch gate.
type SourceBranchPushResult struct {
	Status      SourceBranchStatus
	Reason      string
	NextActions []string
}

// PrepareSourceBranchForPush verifies or interactively publishes the source branch before package publish.
func PrepareSourceBranchForPush(ctx context.Context, input SourceBranchPushInput) (SourceBranchPushResult, error) {
	result := SourceBranchPushResult{
		Status: SourceBranchStatusFromRelation(input.Relation),
	}
	switch input.Relation {
	case gitpkg.BranchRelationEqual:
		return result, nil
	case gitpkg.BranchRelationAhead, gitpkg.BranchRelationMissing:
		if input.Prompter == nil {
			result.Reason = "source_branch_push_required"
			result.NextActions = SourceBranchPushNextActions(input.Remote, input.SourceBranch)
			return result, fmt.Errorf(
				"source branch %q must be pushed to %s before publishing; run `%s`",
				input.SourceBranch,
				input.Remote,
				result.NextActions[0],
			)
		}

		decision, err := input.Prompter.PromptSourceBranchPush(ctx, SourceBranchPushPrompt{
			Remote:       input.Remote,
			SourceBranch: input.SourceBranch,
			Status:       result.Status,
		})
		if err != nil {
			result.Reason = "source_branch_push_prompt_failed"
			result.NextActions = SourceBranchPushNextActions(input.Remote, input.SourceBranch)
			return result, fmt.Errorf("confirm source branch push: %w", err)
		}
		if decision != SourceBranchPushDecisionPush {
			result.Reason = "source_branch_push_declined"
			result.NextActions = SourceBranchPushNextActions(input.Remote, input.SourceBranch)
			return result, fmt.Errorf("publish canceled; source branch %q was not pushed to %s", input.SourceBranch, input.Remote)
		}
		if err := gitpkg.PushBranchSetUpstream(ctx, input.RepoRoot, input.Remote, input.SourceBranch); err != nil {
			result.Reason = "source_branch_push_failed"
			result.NextActions = SourceBranchPushNextActions(input.Remote, input.SourceBranch)
			return result, fmt.Errorf("push source branch %q to %q: %w", input.SourceBranch, input.Remote, err)
		}
		return result, nil
	case gitpkg.BranchRelationBehind:
		result.Reason = "source_branch_not_up_to_date"
		result.NextActions = SourceBranchFastForwardNextActions(input.Remote, input.SourceBranch)
		return result, fmt.Errorf(
			"remote source branch %s/%s has commits not in local %s; fast-forward manually before publishing",
			input.Remote,
			input.SourceBranch,
			input.SourceBranch,
		)
	case gitpkg.BranchRelationDiverged:
		result.Reason = "source_branch_diverged"
		result.NextActions = SourceBranchReconcileNextActions(input.Remote, input.SourceBranch)
		return result, fmt.Errorf(
			"local source branch %q has diverged from %s/%s; reconcile manually before publishing",
			input.SourceBranch,
			input.Remote,
			input.SourceBranch,
		)
	default:
		result.Reason = "source_branch_not_up_to_date"
		return result, fmt.Errorf("unsupported source branch relation %q for %s/%s", input.Relation, input.Remote, input.SourceBranch)
	}
}

// SourceBranchStatusFromRelation maps the Git-level relation to publish output vocabulary.
func SourceBranchStatusFromRelation(relation gitpkg.BranchRelation) SourceBranchStatus {
	switch relation {
	case gitpkg.BranchRelationMissing:
		return SourceBranchStatusMissing
	case gitpkg.BranchRelationAhead:
		return SourceBranchStatusAhead
	case gitpkg.BranchRelationEqual:
		return SourceBranchStatusEqual
	case gitpkg.BranchRelationBehind:
		return SourceBranchStatusBehind
	case gitpkg.BranchRelationDiverged:
		return SourceBranchStatusDiverged
	default:
		return SourceBranchStatus(relation)
	}
}

// SourceBranchPushNextActions reports the manual source branch publish fallback.
func SourceBranchPushNextActions(remote string, branch string) []string {
	return []string{
		fmt.Sprintf("git push -u %s %s", remote, branch),
		"rerun publish with --push",
	}
}

// SourceBranchFastForwardNextActions reports the manual fast-forward fallback.
func SourceBranchFastForwardNextActions(remote string, branch string) []string {
	return []string{
		fmt.Sprintf("git fetch %s %s", remote, branch),
		fmt.Sprintf("git merge --ff-only %s/%s", remote, branch),
		"rerun publish with --push",
	}
}

// SourceBranchReconcileNextActions reports the manual divergence fallback.
func SourceBranchReconcileNextActions(remote string, branch string) []string {
	return []string{
		fmt.Sprintf("git fetch %s %s", remote, branch),
		fmt.Sprintf("reconcile %s with %s/%s", branch, remote, branch),
		"rerun publish with --push",
	}
}
