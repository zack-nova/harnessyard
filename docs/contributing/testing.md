# Testing for Contributors

This page describes the tests contributors should run before opening a pull request.

For the complete testing strategy and coverage matrix, see [Testing Strategy](../maintainers/testing-strategy.md).

## Standard Checks

Run the standard repository checks:

```bash
mise run fix
mise run ci
```

These checks should pass before a pull request is opened.

## Quickstart Acceptance Smoke

When the dedicated quickstart acceptance smoke is added, run it when a change touches any of the following:

- `docs/quickstart.md`
- release-surface documentation
- CLI help output
- install behavior
- release packaging
- branch identity behavior
- `hyard plumbing orbit|harness`
- single-control-plane routing
- user-visible command flows

Until that task exists in this repository, keep quickstart examples aligned with the release-surface check and call out any manual verification in the pull request.

## Release Surface Checks

Run the release-surface test when a change affects:

- `.goreleaser.yaml`
- `.github/workflows/release.yml`
- `install.sh`
- Homebrew formula behavior
- public binary naming
- release archive naming
- `hyard --version`
- `hyard plumbing orbit|harness`

Command:

```bash
sh ./scripts/test_release_surface_hyard.sh
```

## Test Expectations

Tests should prefer real CLI boundaries over mocked behavior when validating user-visible workflows.

Git-related tests should use isolated temporary repositories and must configure:

```bash
git config user.name "Test User"
git config user.email "test@example.com"
```

Tests should avoid relying on the contributor's global Git configuration.

## Documentation Drift

User-facing documentation is part of the product surface.

When a command sequence appears in the quickstart, automated checks should either execute it directly or verify the same flow through a stable script.

Documentation and behavior must not drift.
