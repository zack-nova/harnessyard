# Current Architecture

This is the compact current-reference document for the `hyard` codebase. It
replaces the old design archive as the first place to look for current behavior.

## Public Surface

`hyard` is the canonical public CLI and the only public release binary.

The compatibility command trees remain reachable through:

```bash
hyard plumbing orbit
hyard plumbing harness
```

The public `hyard` root currently exposes:

- repository bootstrap: `create`, `init`, `clone`
- install/remove/publish: `install`, `remove`, `publish`
- runtime health: `check`, `ready`
- guidance: `guide render`, `guide save`, `guide writeback`, `guide sync`
- agent activation: `agent detect`, `agent inspect`, `agent plan`, `agent apply`,
  `agent check`, `agent remove`, `agent use`, `agent recommend`, `hooks`
- current worktree flow: `current`, `enter`, `leave`, `status`, `diff`, `log`,
  `commit`, `restore`
- authored orbit surface: `orbit create`, `orbit list`, `orbit show`,
  `orbit files`, `orbit set`, `orbit validate`, `orbit prepare`,
  `orbit checkpoint`, `orbit content apply`, `orbit member`, `orbit skill`,
  `orbit agent`
- harness affiliation: `assign orbit`, `unassign orbit`

`save` commands are lower-level export primitives. User-facing publication uses
`hyard publish orbit <package>` or `hyard publish harness <package>`.

## Revision Identity

The single versioned revision identity host is:

```text
.harness/manifest.yaml
```

Current manifest kinds are:

- `source`
- `runtime`
- `orbit_template`
- `harness_template`

The manifest schema version is currently `1`. Runtime and template identity use
package-style user-facing names while preserving internal orbit/harness fields
needed by compatibility code.

## Versioned Truth

Current versioned hosts are:

```text
.harness/manifest.yaml
.harness/orbits/*.yaml
.harness/vars.yaml
.harness/installs/*.yaml
.harness/bundles/*.yaml
.harness/template.yaml
.harness/template_members/*.yaml
```

`.harness/orbits/*.yaml` stores hosted OrbitSpec authored truth. Current
OrbitSpec writes the package/content shape:

- `package`
- `name`
- `description`
- `meta`
- `capabilities`
- `agent_addons`
- `content`
- `behavior`

The old top-level `rules` block is accepted as legacy input only and normalizes
to `behavior` on write. Member roles remain:

- `meta`
- `subject`
- `rule`
- `process`

`lane: bootstrap` is lifecycle metadata, not a fifth role.

## Repo-Local State

Repo-local state belongs under `.git/orbit/state` and is not history:

```text
.git/orbit/state/current_orbit.json
.git/orbit/state/resolved_scope/*.txt
.git/orbit/state/orbits/<orbit>/file_inventory.json
.git/orbit/state/orbits/<orbit>/runtime_state.json
.git/orbit/state/orbits/<orbit>/git_state.json
.git/orbit/state/warnings.json
.git/orbit/state/last_status.json
.git/orbit/state/transactions/*
```

Projection caches and ledgers are observational. After a primary Git or
filesystem mutation succeeds, late ledger refresh failures should be warnings
unless the specific command contract requires the state write to be durable.

## Projection And Scoped Work

The current worktree model is still Git-native:

- sparse-checkout controls what is visible after `hyard enter`
- `hyard leave` restores the full tracked view
- scoped read/write operations use current orbit pathspecs
- scoped commits must not silently include out-of-scope changes
- dirty tracked files that would be hidden must block projection changes

The main surfaces remain distinct:

- `projection`: files visible in the worktree
- `orbit_write`: files targeted by current scoped operations
- `export`: files saved or published into package/template payloads
- `orchestration`: guidance materialized into root artifacts

## Guidance

Root guidance artifacts are materialized outputs:

```text
AGENTS.md
HUMANS.md
BOOTSTRAP.md
```

They are editable containers, not canonical authored truth. The authored source
is hosted OrbitSpec guidance fields and package/runtime composition records.

Runtime View selection changes how these artifacts are presented and published;
it does not change package identity or canonical authored truth. In Run View,
root guidance is Run View Root Guidance: runtime-facing presentation text for the
composed Harness Runtime.

Markerless Run View Root Guidance is presentation text, not an authored backfill
lane. Runtime checks must not treat markerless presentation edits as package
truth drift merely because they differ from OrbitSpec guidance templates.

Package installation may write incremental Run View Root Guidance for the newly
installed package and then apply Run View Cleanup when cleanup is safe. Existing
markerless guidance should not be recomposed, reordered, or deduplicated because
it no longer carries owner identity.

Standalone Run View Guidance Output is explicit and separate from package
installation output. Non-interactive standalone output should use output
language rather than force language because replacing markerless presentation
text can discard user edits.

Run View Cleanup removes visible authoring markers and consumed hints from the
runtime-facing artifacts. Before cleanup removes a drifted marked block's owner
identity, Marked Guidance Resolution must choose one of the preserved facts:
save the current block to authored truth, re-render authored truth, or strip
markers in place and keep the current text as Run View Root Guidance.

Use:

- `hyard guide render` to materialize editable root blocks
- `hyard guide save` to write edited blocks back to hosted truth
- `hyard guide sync --output` to explicitly output runtime-wide root guidance artifacts

## Readiness

Runtime readiness is derived, not separately persisted. Current status values:

- `broken`
- `usable`
- `ready`

Readiness combines runtime structure, orbit/member checks, bindings, guidance
composition, and agent activation state. Agent-related readiness reasons include
missing/stale activation, pending hooks, required remote dependencies,
unsupported required events, cleanup blockers, invalid agent truth, invalid
activation ledgers, and ownership conflicts.

## Agent And Capability Model

Orbit capabilities are framework-agnostic authored truth:

- command path scopes
- local skill path scopes
- remote skill dependencies

Local commands and skills are package assets. Remote skill dependencies are
links; activation remains a runtime agent operation.

Agent activation is project-first. Global writes or hybrid hook activation must
remain explicit and visible. Current supported framework adapters include Codex,
Claude Code, and OpenClaw through the shared `agent` command surface and the
lower-level framework implementation package.

Package-scoped agent add-ons currently include hook declarations under
OrbitSpec `agent_addons`.

## Code Boundaries

Important package responsibilities:

- `cmd/hyard/cli`: public user-facing wrappers and error wording
- `cmd/harness/cli/commands`: runtime compatibility commands
- `cmd/orbit/cli/commands`: orbit compatibility commands
- `cmd/orbit/cli/orbit`: OrbitSpec parsing, normalization, validation, scope,
  capability resolution, member hints
- `cmd/orbit/cli/harness`: manifests, installs, bundles, readiness, runtime
  remove/extract, agent activation, native config/hooks
- `cmd/orbit/cli/template`: template save/publish/apply, source manifests,
  guidance materialize/backfill, bootstrap state
- `cmd/orbit/cli/view`: projection state and sparse-checkout entry/exit
- `cmd/orbit/cli/scoped`: scoped files/diff/log/commit/restore
- `cmd/orbit/cli/git`: system Git adapters
- `cmd/orbit/cli/state`: `.git/orbit/state` persistence
- `cmd/orbit/cli/ids`: orbit IDs, package identities, package coordinates

Command code should stay thin and route durable behavior into these packages.

## Release And Tests

Release-facing docs are:

- `docs/quickstart.md`
- `docs/installation.md`
- `docs/reference/release-surface.md`
- `docs/maintainers/release.md`

Standard checks:

```bash
mise run fix
mise run ci
```

Release-surface check:

```bash
sh ./scripts/test_release_surface_hyard.sh
```
