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
