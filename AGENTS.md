# Harness Yard Agent Notes

This file is a thin agent entry point. Keep durable product, architecture,
testing, and release rules in `docs/`; keep this root artifact as an index plus
hard boundaries.

## Startup Reading

Read this file first. Do not pre-read the full documentation set for every
task; use the deeper docs below only when the task touches that area.

## Task-Triggered Reading

- For project overview, setup, or navigation, read `README.md`,
  `docs/README.md`, `docs/quickstart.md`, and `docs/installation.md` as needed.
- For implementation, architecture, package responsibility, or known maintainer
  gaps, read `docs/maintainers/current-architecture.md` and
  `docs/maintainers/unresolved-work.md`.
- For tests or validation strategy, read `docs/contributing/testing.md` and
  `docs/maintainers/testing-strategy.md`.
- For public command behavior, help text, install flow, release surface, or
  release process changes, read `docs/reference/release-surface.md` and
  `docs/maintainers/release.md`.

Historical design docs and historical issue archives were intentionally removed
from the working tree. Do not recreate them as active references. If old context
is needed, recover it from external backup and extract only the current decision
or still-open problem into the maintainer docs above.

## Always-On Boundaries

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

Keep command files thin; route durable behavior into the existing packages. Use
the system `git` through explicit argument lists, validate identifiers before
using them in paths or ref names, and prefer NUL-delimited Git I/O for path
lists.

## Testing

For normal code changes, run `mise run fix` and `mise run ci`.

For release-surface or public command/help/install changes, also run
`sh ./scripts/test_release_surface_hyard.sh`.

Detailed testing rules live in `docs/contributing/testing.md`; maintainer
testing strategy lives in `docs/maintainers/testing-strategy.md`.

# AGENTS.md - Issue Tracker Contract Orbit

This repository uses the Issue Tracker Contract Orbit for issue-driven delivery.

## When To Read

- When handling issue state, metadata, issue sections, review artifacts,
  templates, or tracker safety rules, read
  `docs/issue-tracker-orbit/tracker-contract.md` first. If it is still
  `pending-bootstrap`, run `BOOTSTRAP.md` first.
- Do not assume issues live in GitHub. Use the backend and mappings selected in
  the tracker contract.
- Do not develop directly on the default branch, and do not locally merge the default branch.
- If rules, templates, state, or fact sources conflict, stop and ask the human maintainer for a decision.

## Documentation Entry Points

- Runtime contract: `docs/issue-tracker-orbit/tracker-contract.md`
- Documentation index: `docs/issue-tracker-orbit/INDEX.md`

After the tracker contract is loaded, read core or adapter docs only as needed.

# AGENTS.md - Issue Discovery Orbit

This orbit turns PRD synthesis and issue slicing into traceable work in the
issue tracker.

## When To Read

- Before generating PRDs, slicing issues, or publishing issue candidates, read
  `docs/issue-discovery-orbit/discovery-rules.md`.
- When publishing a PRD or issues, follow the target repository's Repository
  Publishing Rules; if rules are missing or conflicting, output candidates only.

# Design Memory Constitution

This repository uses the Design Memory Orbit to preserve long-lived project language and architecture decisions from human-agent design discussions.

## When To Read

- When discussing project language, context boundaries, architecture decisions,
  `CONTEXT.md`, `CONTEXT-MAP.md`, or ADRs, read
  `docs/design-memory-orbit/INDEX.md` first.
- Do not invent domain terms, context boundaries, or architecture decisions;
  uncertain content must remain an open question.
- If any output conflicts with an existing ADR, state the conflict clearly and
  wait for a human decision.

# AGENTS.md - Development Discipline Orbit

This orbit selects and runs TDD, diagnosis, or review-commit feedback discipline
for the target codebase.

## When To Read

- Before running a TDD, diagnosis, or review-commit skill, apply the currently
  available orbit or repository discipline rules.
- When reporting completion, include the feedback loop used, observed failure
  signals, passing signals, commit or technical-debt recording result, and any
  still-missing evidence.
