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
ready-for-dev
in-progress
in-review
human-review
to-rework
to-merge
merged
```

Issue type is also backend-neutral:

```text
bug
feature
task
docs
chore
```

The issue tracker's configured issue type representation is the source of truth for issue type. The Type line in `dev-brief` is a human-readable mirror of that issue fact and uses the canonical type value, not backend-specific label syntax.

## Optional Metadata

Priority, size, blocked reason, and resolution are optional metadata unless the repository tracker contract makes them stricter.

Blocked reason prevents unsafe advancement. It is not a lifecycle state.

## Issue Sections

The core defines canonical issue sections. Backend adapters decide whether those sections are comments, notes, markdown headings, fields, or another storage form.

Required canonical sections:

```text
triage-notes
dev-brief
dev-workpad
review-sweep
human-review-decision
```

Optional canonical sections:

```text
debt-notes
```

## Review Artifact

A review artifact is required before an issue enters review. The selected backend decides its concrete form:

- GitHub maps it to a pull request.
- GitLab maps it to a merge request.
- Local markdown maps it to a local review artifact file.
- Other backends must define the mapping during bootstrap.
