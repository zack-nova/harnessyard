# State Machine

Every open issue must have exactly one canonical state role.

```text
needs-triage
  -> needs-info
  -> needs-triage
  -> ready-for-dev
  -> in-progress
  -> in-review
  -> human-review
       -> human Decision: hold   -> human-review
       -> human Decision: rework -> to-rework -> in-review
       -> human Decision: merge  -> to-merge  -> land -> merged
```

## State Meanings

- `needs-triage`: New or changed issue awaiting evaluation.
- `needs-info`: Waiting for reporter or owner information.
- `ready-for-dev`: A complete Dev Brief exists and development can start.
- `in-progress`: active development work has been created or assigned.
- `in-review`: A review artifact exists and review evidence is being collected.
- `human-review`: Review Sweep evidence exists and a human review decision or state advancement is pending.
- `to-rework`: A human requested changes after review, and rework should be picked up.
- `to-merge`: A human authorized integration, and land should be picked up.
- `merged`: The review artifact has landed.

## State-triggered Operations

- `needs-triage` triggers triage.
- `ready-for-dev` triggers development start.
- `in-review` triggers Review Sweep evidence collection.
- `to-rework` triggers requested changes.
- `to-merge` triggers Land.
- `needs-info`, `in-progress`, `human-review`, and `merged` do not directly trigger runtime operations.

Contract consumers must prevent duplicate operation pickup through their own manager or orchestrator claim state. The tracker contract must not add claim, running, retry, queue, or dispatcher ownership fields or sections.

## Gates

An issue can enter `ready-for-dev` only when:

- exactly one issue type is set,
- one Dev Brief exists,
- the Dev Brief Type mirror exists and matches the issue type,
- no unanswered triage question remains,
- no active blocked metadata exists,
- acceptance criteria are clear,
- validation plan is clear,
- out-of-scope boundaries are explicit.

An issue can enter `in-progress` only when:

- its current canonical state role is `ready-for-dev`.

An issue can enter `in-review` only when:

- a review artifact exists,
- validation ran or was explicitly waived,
- Dev Workpad records branch/base context and validation result.

An issue can enter `human-review` only when:

- a Review Sweep exists,
- the review artifact is still available,
- the issue's current canonical state role is `in-review` or `human-review`.

An issue can enter `to-rework` only after a human review decision with `Decision: rework`.

An issue can return from `to-rework` to `in-review` only after requested changes are applied, the review artifact is updated, and validation ran or was explicitly waived.

An issue can enter `to-merge` only after a human review decision with `Decision: merge`.

An issue can land only from `to-merge`.

An issue can enter `merged` only after the review artifact lands.

If Land fails from `to-merge`, return the issue to `human-review`. Move it to `to-rework` only after a human confirms the failure requires code changes.

## Review Sweep Boundary

Review Sweep records observations only. It may move an issue from `in-review` to `human-review`, but it must not decide `to-rework`, `to-merge`, or `merged`.
