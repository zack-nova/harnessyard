# Issue Sections

Issue sections are canonical structured parts of an issue. They are backend-neutral; storage is defined by the selected backend adapter.

## Triage Notes

`triage-notes` records missing information, confirmed facts, and why an issue cannot advance yet.

## Dev Brief

`dev-brief` is the development contract. It records:

- issue type as a mirror of tracker metadata,
- optional delivery mode mirror and rationale,
- summary,
- current behavior or context,
- desired behavior,
- key interfaces and constraints,
- acceptance criteria,
- validation plan,
- out-of-scope boundaries.

Development must not expand beyond Dev Brief scope without updating the issue or creating a follow-up issue.

The issue tracker's configured issue type representation remains the source of truth. The Dev Brief Type mirror uses the canonical type value, not backend-specific label syntax. If the Dev Brief Type mirror is missing or conflicts with tracker metadata, stop advancement until a human maintainer resolves the mismatch.

The issue tracker's configured delivery mode representation remains the source of truth when the repository contract defines it.

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

If a technical debt observation should block or change the current issue's delivery, promote it into review evidence. Require a Human Review Decision when the issue is in `human-review` or the observation depends on human judgment.

## Out-of-Scope Catalog

`out-of-scope-catalog` records rejected feature concepts for deduplication and institutional memory.

Each catalog entry records:

- concept name,
- stable kebab-case slug,
- decision,
- durable reason,
- related but allowed concepts when useful,
- prior request issue references.

An `out-of-scope` issue must have exactly one Out-of-Scope Catalog section and does not require Dev Brief, Dev Workpad, Review Sweep, or Human Review Decision sections. Ordinary rejected feature requests remain type `feature`, are cancelled with resolution `wontfix`, and are recorded as prior requests inside a matching catalog entry.

Do not use multiple `## Out-of-Scope Catalog` sections inside one issue for categories. Use one section with multiple entries, or split entries into separate Out-of-Scope Catalog issues when maintenance pressure appears.

Prior requests should use an issue reference with a title snapshot, such as `#42 — "Add dark mode support"`. Do not expand prior requests into a duplicate issue database.

A contract consumer may create a new catalog entry only after a maintainer has decided that the feature request is out of scope. Similarity matching alone is not a maintainer decision.

Repositories should start with one general Out-of-Scope Catalog issue. Split into category-specific catalog issues only when the catalog becomes hard to browse, matching requires reading too many unrelated entries, entries form stable categories, or edit conflicts become common. Splitting moves entries without changing their stable slugs or prior request references.

Out-of-Scope Catalog issue titles should use the `Out-of-Scope Catalog: <Category>` pattern. The category is a human-readable title segment, not a canonical tracker metadata value.

Contract consumers discover Out-of-Scope Catalog issues through canonical issue type `out-of-scope`.

Creating the first Out-of-Scope Catalog issue is lazy. Backend templates should provide an `Out-of-Scope Catalog: Overall` template, but bootstrap does not need to create an empty catalog issue unless the maintainer explicitly enables the catalog or the first out-of-scope decision needs to be recorded.

## Review Sweep

`review-sweep` records **Review Sweep Producer** observations about checks, reviews, comments, and potentially actionable items.

It is not a human decision.

Review Sweep may record whether the observed review evidence is AFK-eligible, requires AFK rework, or requires `human-review`. That classification is review evidence, not a Human Review Decision.

## Human Review Decision

`human-review-decision` is required only when an issue is in `human-review` or when `hitl` delivery mode requires a post-review decision.

When required, `human-review-decision` is the only source for post-review human decisions:

```text
hold
rework
merge
```

`hold` keeps the issue in `human-review`. `rework` allows transition to `to-rework`. `merge` allows transition to `to-merge`; `to-merge` triggers Land, and Land success moves the issue to `merged`.

A Human Review Decision is valid only when a human reviewer or maintainer records exactly one decision value. Placeholder, empty, or multi-value decision text is not a decision.

AFK review advancement must not create or simulate a Human Review Decision. It advances from `in-review` using objective Review Sweep evidence and the state machine gates.
