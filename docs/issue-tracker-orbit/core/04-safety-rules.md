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
- the Dev Brief Type mirror is missing or conflicts with issue type metadata,
- a required issue section is missing,
- active blocked metadata exists,
- a template target exists and differs from the proposed template,
- review state conflicts with issue state,
- validation fails and no waiver exists,
- review artifact is missing,
- review output suggests action but no Human Review Decision exists.
- a contract tries to model claim, running, retry, queue, or dispatcher ownership as issue facts.

## Prohibited

- Do not push directly to the default branch.
- Do not locally merge into the default branch.
- Do not enter `to-rework` or `to-merge` without Human Review Decision.
- Do not land unless the issue is in `to-merge`.
- Do not enter `merged` before the review artifact has landed.
- Do not create duplicate Dev Workpad sections.
- Do not treat external runtime session files or worktree notes as issue facts.
