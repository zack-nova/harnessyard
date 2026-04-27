# Testing Strategy

This document defines the testing strategy for the Harness Yard CLI mainline.

The product is released as `hyard`. The legacy `orbit` and `harness` command surfaces remain available through:

```bash
hyard plumbing orbit
hyard plumbing harness
```

## 1. Goals

The test suite must protect these invariants:

1. Harness Yard must preserve normal Git semantics.
2. Scoped operations must not silently hide dirty worktree changes.
3. Scoped commits must not include out-of-scope changes.
4. Control-plane files, user-visible projection state, and runtime state must remain separated.
5. User-visible commands must produce stable behavior and stable output.
6. Branch identity must be resolved through the single control plane.
7. The public quickstart path must be replayable by automated acceptance smoke tests.
8. The release surface must continue to publish `hyard` as the only canonical public binary.

## 2. MVP Test Pyramid

### Layer A: Unit Tests

Unit tests validate pure logic, local data transformations, Git adapter behavior, and local state read/write logic.

Unit tests should cover:

- ID validation
- repository-relative path normalization
- control-plane loading
- OrbitSpec resolution
- projection scope calculation
- scoped-operation scope calculation
- state file serialization
- atomic writes
- lock behavior
- pathspec assembly
- porcelain parsing
- stable sorting

Use `t.Parallel()` by default unless the test mutates process-wide state such as the current working directory, environment variables, or shared temporary resources.

### Layer B: CLI Integration Tests

CLI integration tests validate workflows through the real command boundary.

Each test should use an isolated temporary Git repository. Each repository must explicitly configure:

```bash
git config user.name "Test User"
git config user.email "test@example.com"
```

Integration tests should cover initialization, branch identity detection, orbit creation, harness creation, installation, validation, list/show behavior, enter/leave behavior, current state behavior, status classification, scoped diff/log/commit/restore, template save/publish behavior, runtime writeback behavior, and fail-closed behavior for invalid control-plane state.

### Layer C: Quickstart Acceptance Smoke

The quickstart acceptance smoke validates the public happy path documented in:

```text
docs/quickstart.md
```

It must cover:

- building `hyard`
- using `hyard` as the only public binary
- using `hyard plumbing orbit|harness` for compatibility flows
- creating runtime, source, orbit-template, and harness-template branches
- installing a harness
- checking branch identity
- entering and leaving an orbit projection
- verifying clean runtime writeback
- verifying migrated runtime writeback
- verifying that legacy `.orbit/config.yaml` is not restored as the steady-state runtime host

The acceptance entrypoint should be added once the runtime fixtures are present in this repository. When the quickstart, help output, release surface, or branch identity routing changes, this smoke test should be run before merge.

### Layer D: Deferred Tests

The following tests are deferred until product complexity or scale requires them:

- performance benchmarks
- large-repository stress tests
- sparse-checkout performance regression tests
- concurrency stress tests
- long-running end-to-end scenario tests
- build-tag-specific integration layers

## 3. Minimum Coverage Matrix

Current mainline coverage should include:

- Git and path safety: non-Git failure, subdirectory invocation, ID validation, path normalization, and NUL-delimited path handling.
- Init and config: idempotent initialization, stable defaults, missing-directory creation, and no fake current-orbit state.
- Validation: malformed YAML, duplicate IDs, filename/ID mismatch, invalid patterns, empty includes, empty-scope warnings, and fail-closed control-plane conflicts.
- Control loading and scope resolution: sparse-hidden control fallback, include/exclude/shared/projection-only semantics, control-plane exclusions, stable sorting, and cache recovery.
- Branch identity: stable handling for `runtime`, `source`, `orbit_template`, and `harness_template` revisions through `.harness/manifest.yaml`.
- Enter and leave: sparse-checkout mutation, current state writes, hidden-dirty gates, outside-untracked warnings, and full-view restoration.
- Status, diff, and log: current-orbit failure modes, in-scope/out-of-scope classification, path-limited reads, `--outside`, and stable JSON output.
- Commit and restore: scoped pathspec boundaries, preservation of out-of-scope changes, commit trailers, fail-closed restore rules, normal Git commits, and best-effort auxiliary refs.
- State and locking: atomic writes, lock behavior, warnings snapshots, status snapshots, and damaged-state diagnostics.
- Quickstart smoke: doc-derived happy path, `hyard`-only build, compatibility plumbing, branch identity, and clean/migrated runtime writeback.
- Release surface: `hyard` as canonical binary, release archive contents, archive naming, version metadata, install docs, Homebrew docs, and compatibility plumbing.

## 4. Test Harness Rules

- Use isolated temporary repositories for Git tests.
- Configure Git user name and email explicitly.
- Do not depend on the contributor's global Git configuration.
- Prefer real Git commands when preparing repositories.
- Prefer real CLI entrypoints for user-visible workflows.
- Use stable, repo-root-relative paths in assertions.
- Sort file lists before comparing them.
- Assert stable fields and important warnings, not incidental whitespace.
- Run quickstart acceptance smoke when documentation or public command behavior changes.
- Shell-based acceptance should reuse existing `scripts/` and `mise` entrypoints.

## Contract-Bearing Documentation

These files are treated as contract-bearing documentation:

```text
docs/quickstart.md
docs/reference/release-surface.md
docs/contributing/testing.md
docs/maintainers/release.md
docs/maintainers/testing-strategy.md
```

Contract-bearing documentation should avoid hard-coded current versions.

Use placeholders instead:

```text
vX.Y.Z
$VERSION
<version>
hyard_${VERSION}_${GOOS}_${GOARCH}.tar.gz
```

Concrete released versions belong in:

```text
CHANGELOG.md
Git tags
GitHub Releases
generated release notes
generated artifacts
checksums.txt
```
