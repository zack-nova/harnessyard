# Root Guidance Workflow Marker Plan

This plan describes the repository-wide implementation work for the pre-release root guidance marker contract recorded in [ADR 0001](../adr/0001-root-guidance-workflow-markers.md).

## Target Contract

Root guidance blocks use owner-specific marker namespaces and a single public Workflow ID field:

```md
<!-- orbit:begin workflow="docs" -->
...
<!-- orbit:end workflow="docs" -->

<!-- harness:begin workflow="runtime-two" -->
...
<!-- harness:end workflow="runtime-two" -->
```

The marker owner kind is either `orbit` or `harness`. The `workflow` value equals the owning orbit id for orbit blocks and the owning harness package id for harness blocks. Root guidance block uniqueness is scoped by `(owner_kind, workflow_id)`.

## Non-Goals

- Do not rename OrbitSpec, orbit package identity, `.harness/orbits/*`, `.orbit-member.yaml`, `orbit_member:`, or `orbit-template/*`.
- Do not add compatibility for pre-release `orbit_id` root guidance marker attributes.
- Do not make `workflow` a separate display alias.
- Do not add arbitrary marker namespaces beyond `orbit` and `harness`.

## Implementation Slices

### 1. Core Marker Model

Update `cmd/orbit/cli/template/agents_runtime.go` to model root guidance block identity as:

- `OwnerKind`: `orbit` or `harness`
- `WorkflowID`: the stable owner package id exposed through the `workflow` marker attribute

Replace the old marker parser with a strict single-line parser for:

```text
<!-- {owner_kind}:begin workflow="{workflow_id}" -->
<!-- {owner_kind}:end workflow="{workflow_id}" -->
```

Parser rules:

- allow only `orbit` and `harness` owner kinds
- require exactly one double-quoted `workflow` attribute
- reject `orbit_id`, duplicate attributes, unknown attributes, single quotes, unquoted values, nested blocks, mismatched end markers, and duplicate `(owner_kind, workflow_id)` blocks
- validate workflow ids with the existing package-id rules

### 2. Block Operations

Update block-level helpers to accept `(OwnerKind, WorkflowID)`:

- wrap a root guidance block
- parse a root guidance document
- extract a block
- replace or append a block
- remove a block
- normalize payload content without markers

Keep user-facing diagnostics owner-specific:

- `orbit block "docs"`
- `harness block "runtime-two"`
- `malformed workflow marker` for syntax-level failures

### 3. Orbit-Owned Guidance Callers

Update orbit-owned root guidance paths to pass `OwnerKind=orbit` and the current orbit id:

- adoption root `AGENTS.md` wrapping
- `guide render/save/writeback/sync`
- brief materialize/backfill/check/export sync
- humans materialize/backfill/check
- bootstrap materialize/backfill/check/restore
- template apply/save/publish/install replay flows that touch root guidance
- package rename logic for root guidance markers

Orbit package rename should update only `orbit:` blocks whose `workflow` value matches the old orbit package id.

### 4. Harness-Owned Guidance Callers

Update harness-owned root guidance paths to pass `OwnerKind=harness` and the harness package id:

- bundle AGENTS payload apply/remove
- harness template install/remove flows that write or remove harness-owned root guidance
- runtime extraction and bundle ownership checks that inspect harness-owned root guidance
- readiness and diagnostics that report harness block state

Harness-owned writes must produce `harness:` markers, not `orbit:` markers.

### 5. Tests And Fixtures

Update tests that assert root guidance marker text from `orbit_id` to `workflow`.

High-impact test areas:

- `cmd/orbit/cli/template/agents_runtime_test.go`
- orbit guidance integration tests
- template apply/save/publish tests
- harness guidance compose/install/remove tests
- bundle AGENTS tests
- adoption tests that inspect root `AGENTS.md`
- package rename tests that update root guidance markers
- readiness tests for malformed or drifted root guidance

Add focused coverage for:

- valid orbit block parsing
- valid harness block parsing
- same workflow id allowed across different owner kinds
- duplicate block rejection within the same owner kind
- mismatched owner kind or workflow id between begin and end
- rejection of `orbit_id`
- rejection of unknown namespaces
- replacement/removal preserving unrelated blocks from the other owner kind

### 6. Documentation And Release Surface

Update user-facing documentation only where marker syntax is shown or explained.

Likely files:

- `docs/quickstart.md`
- `docs/reference/release-surface.md`
- `docs/maintainers/testing-strategy.md`
- `CONTRIBUTING.md`

Keep public package and command names stable unless they already describe root guidance blocks. Avoid broad wording that suggests OrbitSpec, template branches, or manifest fields have been renamed to workflow.

## Verification

Run focused tests first:

```bash
go test ./cmd/orbit/cli/template ./cmd/orbit/cli/harness ./cmd/hyard/cli ./cmd/harness/cli ./cmd/orbit/cli
```

Then run the standard repository checks:

```bash
mise run fix
mise run ci
```

If quickstart, help output, or release-surface docs change, also run:

```bash
sh ./scripts/test_release_surface_hyard.sh
```
