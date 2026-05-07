# Harness Yard Agent Notes

This file is a thin agent entry point. Keep durable product, architecture,
testing, and release rules in `docs/`; keep this root artifact as an index plus
hard boundaries.

## Read First

Use these docs as the current working set:

1. `README.md`
2. `docs/quickstart.md`
3. `docs/installation.md`
4. `docs/README.md`
5. `docs/reference/release-surface.md`
6. `docs/maintainers/current-architecture.md`
7. `docs/maintainers/unresolved-work.md`
8. `docs/contributing/testing.md`
9. `docs/maintainers/testing-strategy.md`
10. `docs/maintainers/release.md`

Historical design docs and historical issue archives were intentionally removed
from the working tree. Do not recreate them as active references. If old context
is needed, recover it from external backup and extract only the current decision
or still-open problem into the maintainer docs above.

## Hard Boundaries

- Treat `hyard` as the public product surface and the only public release
  binary. Historical `orbit` and `harness` command trees are compatibility
  surfaces reached through `hyard plumbing orbit` and `hyard plumbing harness`.
- Preserve the Git-native model: one repository, one working tree, normal Git
  commits as history, sparse-checkout for projection, and pathspec-limited
  scoped operations.
- Keep `.harness/*` as versioned product truth and `.git/orbit/state/*` as
  repo-local runtime state and cache.
- Treat root `AGENTS.md`, `HUMANS.md`, and `BOOTSTRAP.md` as materialized
  guidance artifacts, not canonical authored truth.
- Do not introduce worktrees, services, HTTP APIs, background daemons, databases
  as canonical state, auth/multitenancy, semantic/block-level file ownership, or
  required pushes to auxiliary refs.

## Implementation Notes

Prefer the package responsibilities documented in
`docs/maintainers/current-architecture.md`. Keep command files thin; route
durable behavior into the existing packages. Use the system `git` through
explicit argument lists, validate identifiers before using them in paths or ref
names, and prefer NUL-delimited Git I/O for path lists.

## Testing

For normal code changes, run `mise run fix` and `mise run ci`.

For release-surface or public command/help/install changes, also run
`sh ./scripts/test_release_surface_hyard.sh`.

Detailed testing rules live in `docs/contributing/testing.md`; maintainer
testing strategy lives in `docs/maintainers/testing-strategy.md`.

# AGENTS.md - Issue Tracker Contract Orbit

This repository uses the Issue Tracker Contract Orbit to manage issue-driven delivery.

## Non-Negotiable Rules

- Read `docs/issue-tracker-orbit/tracker-contract.md` first. If it is still `pending-bootstrap`, run `BOOTSTRAP.md` first.
- Do not assume issues live in GitHub. Use the backend mapping selected in the tracker contract.
- Every open issue must have exactly one canonical state role.
- Before an issue enters `ready-for-dev`, it must have exactly one issue type and a complete `Dev Brief`.
- Before an issue enters `in-progress`, it must already be in canonical state role `ready-for-dev`.
- `Dev Workpad` is an issue-scoped execution record, not an external runtime session.
- `Review Sweep` records only **Review Sweep Producer** observations; it does not decide `rework` or `merge`.
- `to-rework` and `to-merge` must be decided by `Human Review Decision`.
- After `to-rework` work is completed, the issue returns to `in-review`; `to-merge` can enter `merged` only after Land succeeds.
- Do not develop directly on the default branch, and do not locally merge the default branch.
- If rules, templates, state, or fact sources conflict, stop and ask the human maintainer for a decision.

## Documentation Entry Points

- Runtime contract: `docs/issue-tracker-orbit/tracker-contract.md`
- Documentation index: `docs/issue-tracker-orbit/INDEX.md`

When handling state, metadata, issue sections, review artifacts, templates, or safety rules, read the tracker contract first, then read core or adapter docs as needed.

# AGENTS.md - Issue Discovery Orbit

You are working in the Issue Discovery Orbit. This orbit turns PRD synthesis and issue slicing into traceable work in the issue tracker.

## Required Rules

1. Before running any skill from this orbit, read `docs/issue-discovery-orbit/discovery-rules.md`.
2. When publishing a PRD or issues, follow the target repository's Repository Publishing Rules; if rules are missing or conflicting, output candidates only.

# Design Memory Constitution

This repository uses the Design Memory Orbit to preserve long-lived project language and architecture decisions from human-agent design discussions.

## Non-Negotiable Rules

- Read `docs/design-memory-orbit/INDEX.md` first.
- Project memory consists of `CONTEXT.md`, `CONTEXT-MAP.md`, `docs/adr/`, and same-named files inside context directories.
- Do not invent domain terms, context boundaries, or architecture decisions; uncertain content must remain an open question.
- Missing project memory is not an error; create memory lazily only after a term or decision is confirmed.
- If any output conflicts with an existing ADR, state the conflict clearly and wait for a human decision.

See `docs/design-memory-orbit/INDEX.md` for more rules and templates.

# AGENTS.md - Development Discipline Orbit

You are working in the Development Discipline Orbit. This orbit selects and runs TDD, diagnosis, or review-commit feedback discipline for the target codebase.

## Required Rules

1. Before running any skill from this orbit, apply the currently available orbit or repository discipline rules.
2. When reporting completion, include the feedback loop used, observed failure signals, passing signals, commit or technical-debt recording result, and any still-missing evidence.
