# State Machine

Every open issue must have exactly one canonical state role.

```text
needs-triage
  -> needs-info
  -> needs-triage
  -> needs-split
  -> ready-for-dev
  -> in-progress
  -> in-review
       -> to-rework -> in-review
       -> to-merge  -> land -> merged
       -> human-review
            -> human Decision: hold   -> human-review
            -> human Decision: rework -> to-rework -> in-review
            -> human Decision: merge  -> to-merge  -> land -> merged

any active non-terminal state
  -> blocked -> unblock -> needs-triage
  -> cancelled
```

Active non-terminal states are canonical states other than `blocked`, `merged`, and `cancelled`.

## State Meanings

- `needs-triage`: New or changed issue awaiting evaluation.
- `needs-info`: Waiting for reporter or owner information.
- `needs-split`: The issue is too large to develop safely and must be split into smaller issues.
- `blocked`: Advancement is paused by a dependency, external factor, or human decision.
- `ready-for-dev`: A complete Dev Brief exists and development can start.
- `in-progress`: active development work has been created or assigned.
- `in-review`: A review artifact exists and review evidence is being collected.
- `human-review`: Review Sweep evidence exists and human judgment is needed before state advancement.
- `to-rework`: Review evidence or a human decision requires changes, and rework should be picked up.
- `to-merge`: Review evidence or a human decision authorizes integration, and Land should be picked up.
- `merged`: The review artifact has landed.
- `cancelled`: The issue was intentionally ended without delivery.

## State-triggered Operations

- `needs-triage` triggers triage.
- `needs-split` triggers issue splitting.
- `ready-for-dev` triggers development start.
- `in-review` triggers Review Sweep evidence collection.
- `to-rework` triggers requested changes.
- `to-merge` triggers Land.
- `needs-info`, `blocked`, `in-progress`, `human-review`, `merged`, and `cancelled` do not directly trigger runtime operations.

Contract consumers must prevent duplicate operation pickup through their own manager or orchestrator claim state. The tracker contract must not add claim, running, retry, queue, or dispatcher ownership fields or sections.

## Delivery Mode

Delivery mode is optional metadata with two canonical values:

```text
afk
hitl
```

When delivery mode is absent, contract consumers should prefer AFK handling when the issue can safely advance through objective repository gates.

An issue marked `hitl` must record why it cannot be delegated without human interaction before entering `ready-for-dev`. Valid reasons include judgment calls, external access, design decisions, architecture decisions, and manual testing.

An issue marked `afk` can still enter `human-review` when a contract consumer observes that safe advancement now requires human judgment.

## Gates

An issue can enter `ready-for-dev` only when:

- exactly one issue type is set,
- one Dev Brief exists,
- the Dev Brief Type mirror exists and matches the issue type,
- delivery mode is absent or exactly one of `afk` or `hitl`,
- `hitl` delivery mode has a rationale explaining why the issue cannot be delegated without human interaction,
- no unanswered triage question remains,
- acceptance criteria are clear,
- validation plan is clear,
- out-of-scope boundaries are explicit.

An issue can enter `blocked` from any active non-terminal state when a dependency, external factor, or human decision prevents safe advancement. The blocking reason must be recorded in the appropriate issue section.

An issue can leave `blocked` only when the blocker is resolved.

An issue can enter `needs-split` when its scope is too large for one issue. The split reason and intended decomposition must be recorded in Triage Notes. After child issues are created, the original issue should move to `blocked` when it must wait for those child issues, and the child issue references must be recorded as blockers.

An issue can leave `needs-split` only after smaller issue references are recorded, or after a human maintainer narrows the original issue enough to continue in another active non-terminal state.

An issue can enter `cancelled` only when the cancellation reason is recorded.

An issue superseded by another issue can enter `cancelled` only when it records resolution `duplicate` and the superseding issue reference.

`cancelled` is a terminal workflow state. Resolution metadata describes why delivery ended without creating additional terminal state roles.

An ordinary feature request rejected as out of scope remains type `feature`, enters `cancelled`, records resolution `wontfix` when the tracker contract supports resolution metadata, and references the matching Out-of-Scope Catalog entry. Backend tracker closure remains a separate maintainer action.

An issue with type `out-of-scope` must have exactly one Out-of-Scope Catalog section before it enters `cancelled`. It must not enter delivery states such as `ready-for-dev`, `in-progress`, `in-review`, `human-review`, `to-rework`, or `to-merge`.

An issue with type `out-of-scope` must have canonical state `cancelled`. Any other state on an `out-of-scope` issue is a state conflict.

An issue can enter `in-progress` only when:

- its current canonical state role is `ready-for-dev`.

An issue can enter `in-review` only when:

- a review artifact exists,
- validation ran or was explicitly waived,
- Dev Workpad records branch/base context and validation result.

An issue can enter `human-review` only when:

- a Review Sweep exists,
- the review artifact is still available,
- the issue's current canonical state role is `in-review` or `human-review`,
- the issue is marked `hitl`, or review evidence records why human judgment is now required.

An issue can enter `to-rework` only when one of these is true:

- the issue is in `human-review` and has a valid Human Review Decision with `Decision: rework`,
- the issue is not in `human-review`, is not marked `hitl`, and Review Sweep evidence records actionable rework that can be handled without human judgment.

An issue can return from `to-rework` to `in-review` only after requested changes are applied, the review artifact is updated, and validation ran or was explicitly waived.

An issue can enter `to-merge` only when one of these is true:

- the issue is in `human-review` and has a valid Human Review Decision with `Decision: merge`,
- the issue is not in `human-review`, is not marked `hitl`, validation passed or was explicitly waived, Review Sweep evidence records no unresolved review artifact comments, and no human-dependent item remains.

An issue can land only from `to-merge`.

An issue can enter `merged` only after the review artifact lands.

If Land fails from `to-merge`, return a `hitl` issue or issue with human-dependent failure evidence to `human-review`. Otherwise return it to `to-rework` when the failure is objective and can be handled AFK.

## Review Sweep Boundary

Review Sweep records observations only. It may identify objective AFK eligibility, objective AFK rework, or the need for `human-review`, but it is not a Human Review Decision.

Review Sweep must not decide `merged`, `cancelled`, or cancellation resolutions such as `duplicate`. It must not bypass `human-review` when review evidence depends on judgment calls, external access, design decisions, architecture decisions, or manual testing.
