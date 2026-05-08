# Core Model

The core model is backend-neutral. It defines what must be true for issue-driven delivery, not where or how a tracker stores those facts.

## Fact Boundary

- The selected issue tracker stores issue facts.
- The tracker contract stores the selected backend and runtime mappings.
- The tracker contract does not store consumer permissions or runtime actor roles.
- External runtime sessions and worktrees are disposable caches.
- Code-change evidence can be recorded as issue facts, but it does not decide workflow transitions after review.

## Required Dimensions

Every managed issue has:

- exactly one canonical state role,
- exactly one issue type before entering `ready-for-dev`.

Canonical state roles are unprefixed:

```text
needs-triage
needs-info
needs-split
blocked
ready-for-dev
in-progress
in-review
human-review
to-rework
to-merge
merged
cancelled
```

Issue type is also backend-neutral:

```text
bug
feature
task
docs
chore
out-of-scope
```

`out-of-scope` is reserved for long-lived catalog issues that record rejected feature concepts for deduplication and institutional memory. Ordinary rejected feature requests remain `feature` issues, enter `cancelled` with resolution `wontfix`, and reference an out-of-scope catalog entry before tracker closure. Superseded issues keep their original type, enter `cancelled` with resolution `duplicate`, and reference the superseding issue.

The issue tracker's configured issue type representation is the source of truth for issue type.

## Optional Metadata

Priority, size, and resolution are optional metadata unless the repository tracker contract makes them stricter. Resolution records why a terminal non-delivery issue was cancelled when the reason has a canonical value, such as `wontfix` or `duplicate`.

Delivery mode is optional metadata for delivery issues. It records whether a delivery slice is expected to proceed without human interaction or requires human-in-the-loop handling:

```text
afk
hitl
```

`afk` is preferred when objective repository gates are enough for implementation, review, and land. `hitl` requires a recorded note explaining why the issue cannot be delegated without human interaction, such as judgment calls, external access, design decisions, architecture decisions, or manual testing.

Delivery mode is not a state role, assignee, queue claim, runtime actor, or permission boundary.

## Issue Sections

The core defines canonical issue sections. Backend adapters decide whether those sections are comments, notes, markdown headings, fields, or another storage form.

Required canonical sections for delivery issues (`bug`, `feature`, `task`, `docs`, and `chore`):

```text
triage-notes
dev-brief
dev-workpad
review-sweep
```

Required canonical sections for `out-of-scope` catalog issues:

```text
out-of-scope-catalog
```

`out-of-scope` catalog issues do not require delivery sections because they are terminal workflow records, not development work.

Optional canonical sections:

```text
debt-notes
human-review-decision
```

`human-review-decision` becomes required only when an issue uses the `human-review` state or a `hitl` delivery mode needs a post-review decision.

## Review Artifact

A review artifact is required before a delivery issue enters review. `out-of-scope` catalog issues do not enter review and do not require review artifacts. The selected backend decides a review artifact's concrete form:

- GitHub maps it to a pull request.
- GitLab maps it to a merge request.
- Local markdown maps it to a local review artifact file.
- Other backends must define the mapping during bootstrap.
