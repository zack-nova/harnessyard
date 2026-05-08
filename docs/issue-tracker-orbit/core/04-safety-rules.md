# Safety Rules

## Dry-run Required

Show a dry-run before:

- batch issue edits,
- batch metadata or label changes,
- installing or overwriting backend templates,
- pushing branches,
- creating review artifacts,
- landing.

## Hard Stops

Stop and ask a human maintainer when:

- an open issue has no state,
- an open issue has multiple states,
- the Dev Brief Type mirror on a delivery issue is missing or conflicts with issue type metadata,
- delivery mode metadata has multiple values or a value other than `afk` or `hitl`,
- a required issue section is missing,
- an `out-of-scope` issue has delivery sections instead of exactly one Out-of-Scope Catalog section,
- an issue in `needs-split` advances without recorded split resolution,
- an issue in `blocked` advances without an explicit unblock transition,
- an issue is cancelled with resolution `duplicate` but no superseding issue reference,
- a template target exists and differs from the proposed template,
- review state conflicts with issue state,
- validation fails and no waiver exists,
- review artifact is missing,
- a `hitl` issue or issue in `human-review` attempts to enter `to-rework` or `to-merge` without a Human Review Decision,
- review output depends on human judgment but the issue attempts to skip `human-review`,
- a Human Review Decision is empty, placeholder text, or has multiple values,
- a contract tries to model claim, running, retry, queue, or dispatcher ownership as issue facts.

## Prohibited

- Do not push directly to the default branch.
- Do not locally merge into the default branch.
- Do not enter `to-rework` or `to-merge` from `human-review` without Human Review Decision.
- Do not use AFK review advancement when safe advancement depends on judgment calls, external access, design decisions, architecture decisions, or manual testing.
- Do not land unless the issue is in `to-merge`.
- Do not enter `merged` before the review artifact has landed.
- Do not create duplicate Dev Workpad sections.
- Do not treat external runtime session files or worktree notes as issue facts.
