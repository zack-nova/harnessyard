# Contributing

Thank you for helping develop and maintain Harness Yard.

This repository uses a lightweight but explicit contribution flow: develop on a branch, run local validation before finalized work, and merge changes into `main` through a pull request. The goal is to keep the process simple while protecting the CLI main path, Git-native state boundaries, runtime/template behavior, and compatibility surface.

## Scope

This guide applies to:

- Code changes
- Documentation changes
- CLI behavior changes
- Runtime, template, and state-file changes
- Tests, linting, CI, release, and local tooling changes

Harness Yard's canonical public CLI is `hyard`. The raw `orbit` and `harness` command trees remain available through `hyard plumbing orbit|harness` for compatibility, authoring, migration, and low-level diagnostics.

## Product Boundaries

Keep changes aligned with the current layered baseline:

- v0.4: unified control plane, `.harness/*` versioned truth, `.git/orbit/state/*` repo-local state, and the `projection`, `orbit_write`, `export`, and `orchestration` surfaces.
- v0.5: runtime lifecycle and readiness semantics: `broken`, `usable`, and `ready`.
- v0.6+: guidance, capability, and agent-framework activation lanes.

Important steady-state boundaries:

- Git remains the source of history.
- `.harness/manifest.yaml` stores revision identity.
- `.harness/orbits/*.yaml` stores authored OrbitSpec truth.
- `.harness/vars.yaml`, `.harness/installs/*.yaml`, and `.harness/bundles/*.yaml` store runtime provenance.
- `.git/orbit/state/*` stores repo-local runtime state and caches.
- Root `AGENTS.md`, `HUMANS.md`, and `BOOTSTRAP.md` are materialized artifacts, not authored truth.
- `refs/orbits/*` are optional auxiliary refs, not required correctness state.

Do not introduce worktrees, databases as canonical state, web services, background daemons, auth/multitenancy, block-level orbit logic, or automatic push requirements for `refs/orbits/*`.

## Development Setup

This repository uses `mise` for local development tasks. The task scripts expect a POSIX shell environment; macOS and Linux work directly, and Windows contributors should use WSL or an equivalent POSIX shell.

Prerequisites:

- `git`
- `mise`
- Go, currently pinned by `mise.toml`
- The lint, test, and security tools installed by `mise`

Initialize your local environment:

```bash
mise trust
mise install
mise run check
```

Build the local CLI when you need to test the binary directly:

```bash
mise run build
./.dist/bin/hyard --help
```

## Git Workflow

Create a branch from `main`:

```bash
git checkout main
git pull --ff-only
git checkout -b feature/runtime-identity-scan
```

Recommended branch prefixes:

- `feature/<topic>` for new functionality
- `fix/<topic>` for bug fixes
- `docs/<topic>` for documentation changes
- `test/<topic>` for test additions or test-structure changes
- `refactor/<topic>` for behavior-preserving refactors
- `chore/<topic>` for tooling, configuration, and maintenance

Do not push feature work directly to `main`. Behavior changes should go through a pull request.

## Issue And Design Alignment

For larger features, architecture changes, or behavior changes, open a tracking issue or align on the design before implementation.

Design alignment is especially important for changes that:

- Modify public `hyard` behavior or user-facing output
- Modify `hyard install`, `hyard check`, `hyard ready`, `hyard enter`, `hyard leave`, `hyard guide`, `hyard agent`, or `hyard publish` semantics
- Modify `hyard plumbing orbit|harness` compatibility behavior
- Modify runtime, template, manifest, provenance, or `.git/orbit/state/*` structure
- Introduce migration, writeback, save, publish, or extract behavior
- Add, remove, or rename commands or flags
- Affect orbit/harness compatibility, package coordinates, or local remote publish behavior
- Affect guidance artifacts such as `AGENTS.md`, `HUMANS.md`, or `BOOTSTRAP.md`
- Affect commands, skills, hooks, or agent-framework activation
- Change quickstart acceptance smoke coverage

If implementation reveals a conflict with the active docs, update the docs first or call out the conflict explicitly before expanding the code change.

## Commit Messages

Use clear, descriptive, imperative commit messages:

```text
Add runtime identity scanner
Fix harness template writeback
Document local CI workflow
Refactor orbit compatibility checks
```

Avoid vague messages:

```text
update
fix bug
misc changes
work in progress
```

Temporary WIP commits are fine locally. Before opening a PR, clean up the history enough that the final commits explain the purpose of the change.

## Before Committing Finalized Work

Run:

```bash
mise run fix
mise run check
```

`mise run fix` runs `gofmt -s` and `go mod tidy`. `mise run check` runs lint and Go tests through the repository task entrypoints.

If `mise run fix` changes files, review the diff before committing:

```bash
git diff
git status
```

## Before Opening A Pull Request

Before opening a PR, confirm:

```bash
mise run fix
mise run check
git status
```

If the PR touches CLI behavior, runtime/state structure, template compatibility, quickstart/help/release behavior, or the MVP main path, also run:

```bash
mise run ci
```

## Before Merge Or RC

Before merging MVP behavior changes, release-candidate work, demo-critical work, quickstart/help/release changes, or larger runtime/template changes, run:

```bash
mise run ci
```

`mise run ci` runs lint, uncached Go tests, `govulncheck`, and shell validation tests.

When the dedicated quickstart acceptance smoke is added, it should exercise the current `hyard` main path in isolated temp repos, including:

- `runtime`, `source`, `orbit_template`, and `harness_template` branch identity
- `hyard plumbing orbit|harness` compatibility surface
- `hyard install`
- `hyard check`
- `hyard enter`
- `hyard leave`
- clean runtime and migrated runtime writeback
- harness template publish to a local remote

This smoke does not replace unit tests or `go test`; it confirms that Harness Yard still works as an integrated CLI.

## Testing Expectations

New behavior should normally include tests. Bug fixes should include a regression test whenever practical.

Before opening a pull request, run:

```bash
mise run fix
mise run ci
```

When your change touches user-visible command flows, quickstart documentation, release packaging, install behavior, branch identity, or compatibility plumbing, also run the matching shell validation script or release-surface check:

```bash
sh ./scripts/test_release_surface_hyard.sh
```

For contributor-facing testing instructions, see [docs/contributing/testing.md](docs/contributing/testing.md).

For the full testing strategy and coverage matrix, see [docs/maintainers/testing-strategy.md](docs/maintainers/testing-strategy.md).

If a behavior change does not include new tests, explain why in the PR. Good reasons include existing coverage, unstable automation at the current stage, follow-up work tracked in an issue, or documentation-only/tooling-only scope.

## Code Style

Go code must pass:

```bash
gofmt -s
golangci-lint run
go test ./...
```

Use the repository task entrypoints:

```bash
mise run fix
mise run check
```

Default code expectations:

- Keep command files thin: flags, arguments, stdout/stderr, and JSON output only.
- Keep Git interaction in the Git adapter layer; use system `git` with explicit argument lists.
- Do not use `sh -c` for normal Git operations.
- Prefer repo-root-relative paths and NUL-delimited Git/pathspec I/O.
- Validate identifiers before using them in paths, refs, or state filenames.
- Keep `.git/orbit/state/*` writes in the state layer.
- Use atomic writes for state files.
- Fail closed when a change could hide dirty tracked paths, include scope-outside changes, or corrupt state.
- Keep CLI text stable, clear, and diagnosable.
- Keep `--json` output stable and free of prose-only warnings.
- Handle errors explicitly; do not silently discard them.
- Use readable names over short abbreviations.
- Add focused comments only when they clarify non-obvious logic.

## Documentation Expectations

Update documentation when a change affects:

- Commands, flags, examples, or user-facing output
- Default behavior or error behavior
- Runtime, template, manifest, provenance, or state-file formats
- Readiness, guidance, command/skill capability, or framework activation behavior
- Migration, writeback, save, publish, clone, install, extract, assign, or remove behavior
- Quickstart, demo, release, or smoke-test flows

Possible docs to update include `README.md`, `docs/quickstart.md`, user guides, technical specs, local issues, release notes, and this file.

## Pull Request Expectations

A PR description should include:

- Purpose
- Main changes
- Local validation
- User-visible or state-format impact

Suggested format:

````md
## Purpose

Describe the problem this PR solves.

## Changes

- Change 1
- Change 2
- Change 3

## Validation

Ran:

```bash
mise run fix
mise run check
```

Also ran, if applicable:

```bash
mise run ci
```

## Impact

Does this affect:

- CLI commands
- CLI output
- runtime/state files
- orbit/harness compatibility
- guidance artifacts
- package/template publish or install behavior
- quickstart, help, release, or demo flows

## Notes

Anything reviewers should know about constraints, tradeoffs, or follow-up work.
````

## Review And Merge Policy

Recommended repository rules:

- `main` should use branch protection or repository rules.
- Behavior-changing PRs should have at least one reviewer who is not the author.
- Required checks should pass before merge.
- Failed checks should not be bypassed without a clear emergency note.
- Small and medium PRs should usually use squash merge.
- Larger refactors may keep merge commits when the branch history is meaningful.
- Before merge, confirm there is no obvious CI drift or unresolved conflict with `main`.

Emergency fixes can use a shorter path, but the PR should state why the fix is urgent, which checks were skipped, and how validation will be completed afterward.

## Security Issues

Do not disclose suspected security vulnerabilities in public issues, discussions, or PR descriptions.

Follow [SECURITY.md](SECURITY.md). The current process uses GitHub private vulnerability reporting:

<https://github.com/zack-nova/harnessyard/security/advisories/new>

Security reports should include the affected version or commit, operating system and architecture, reproduction steps, expected impact, any known workaround, and relevant logs or commands.

## Dependency And Tooling Changes

For dependency, tooling, lint, CI, release, or `mise` task changes, explain:

- Why the change is needed
- Whether it affects local setup
- Whether it affects CI time
- Whether it affects Go module files
- Whether docs or quickstart flows need updates

Common examples include Go version changes, `golangci-lint` updates, new test tooling, generated-code workflows, release config changes, and quickstart acceptance smoke changes.
