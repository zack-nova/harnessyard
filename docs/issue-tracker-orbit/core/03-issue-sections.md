# Issue Sections

Issue sections are canonical structured parts of an issue. They are backend-neutral; storage is defined by the selected backend adapter.

## Triage Notes

`triage-notes` records missing information, confirmed facts, and why an issue cannot advance yet.

## Dev Brief

`dev-brief` is the development contract. It records:

- issue type as a mirror of tracker metadata,
- summary,
- current behavior or context,
- desired behavior,
- key interfaces and constraints,
- acceptance criteria,
- validation plan,
- out-of-scope boundaries.

Development must not expand beyond Dev Brief scope without updating the issue or creating a follow-up issue.

The issue tracker's configured issue type representation remains the source of truth. The Dev Brief Type mirror uses the canonical type value, not backend-specific label syntax. If the Dev Brief Type mirror is missing or conflicts with tracker metadata, stop advancement until a human maintainer resolves the mismatch.

## Dev Workpad

`dev-workpad` is the issue-scoped execution record. It records:

- branch,
- base ref,
- plan,
- validation result,
- review artifact link or path,
- notes and confusions.

It is an auditable issue fact, not an external runtime session cache.

## Debt Notes

`debt-notes` is an optional issue-scoped section for technical debt observations discovered while working an issue.

Each entry records:

- observation,
- impact,
- follow-up issue as `none` or an issue reference.

When a Debt Notes entry is promoted to a managed issue, update its follow-up issue from `none` to the issue reference. The original entry does not mirror follow-up issue state.

Debt Notes are not lifecycle gates, review decisions, hidden follow-up issues, or a second status workflow.

If a technical debt observation should block or change the current issue's delivery, promote it into review evidence and require a Human Review Decision instead of leaving it only in Debt Notes.

## Review Sweep

`review-sweep` records **Review Sweep Producer** observations about checks, reviews, comments, and potentially actionable items.

It is not a human decision.

## Human Review Decision

`human-review-decision` is the only source for post-review decisions:

```text
hold
rework
merge
```

`hold` keeps the issue in `human-review`. `rework` allows transition to `to-rework`. `merge` allows transition to `to-merge`; `to-merge` triggers Land, and Land success moves the issue to `merged`.
