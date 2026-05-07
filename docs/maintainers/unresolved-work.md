# Unresolved Work

This document is the compact backlog seed for current `hyard` work. Historical
design docs and historical issue archives were removed from the working tree;
new follow-up work should update this file or create a fresh focused issue.

## Publish And Package Release

- Generated publish remote-base alignment still needs hardening. Source-to-orbit
  and runtime-to-harness generated publish flows should base `--push` output on
  the remote template branch head when it exists, rather than requiring the user
  to manually sync a stale local template branch first.
- Harness-template publish no-op detection still risks drifting from the actual
  saved payload. Payload enumeration should be centralized so new payload lanes
  are compared and written through the same tree description.
- Full-history fetch during publish preserves local template-branch continuity
  but may be too expensive for long-lived template branches. Measure real repos
  before replacing it with a bounded deepen strategy.

## Skill And Capability Lane

- Built-in agent adapters still diagnose remote skills but do not materialize,
  fetch, register, or verify them end to end.
- Remote skill truth has no reproducible pinning contract yet. A URI can drift
  over time because there is no schema-backed revision, digest, or immutable
  resolution token.
- Adapter-side remote skill fetch safety remains unfrozen. Before real fetch is
  enabled, define allowed schemes, cache placement, validation, cleanup, and
  failure reporting.
- Compatibility capability CRUD and raw URI-list plumbing still exist beside the
  canonical path/dependency model. Keep them narrow and retire or migrate them
  once dependent workflows disappear.
- Local skill overlay can classify draft local skill trees before resolver
  validation rejects malformed skills. Decide whether this draft-friendly
  behavior is intentional or should be narrowed.
- Capability glob matching still depends on current `doublestar` behavior. Add a
  focused hardening matrix for `**` and edge-case intersections.
- Capability migration still runs one orbit at a time. Multi-orbit hosted repos
  may need a preview/batch migration path.
- Member/capability overlap prevention exists for common cases, but still lacks
  a formal glob-intersection substrate and a repair command for already-invalid
  authored specs.

## Agent Activation

- The route model works for the first project/global/compatibility paths, but
  non-interactive confirmation semantics, hook activation intent, trust/restart
  notes, and deeper adapter lifecycle fields remain incomplete.
- Command-to-skill rendering does not preserve structured argument metadata.
  Freeze a framework-neutral argument schema before adding richer generated
  skill output.
- Native config generation uses a conservative TOML/JSON5 subset. Either adopt
  fuller parsers or document the supported subset clearly in validation errors.
- Package-truth agent config still has migration-era vocabulary around
  `agent.yaml` / `overlays`; the intended target is the newer config/sidecar
  model without long-lived dual-write.
- Unified hook activation supports a narrow native adapter subset. Vendor hook
  sidecars, richer native matcher syntax, capture/import, trust/restart UX, and
  mixed stdout logging/result framing remain future work.
- Package agent add-ons are readable, installable, applicable, and removable for
  first project-local outputs, but runtime enable/disable overrides and full
  readiness aggregation for cleanup-blocked states still need closure.

## Runtime, Provenance, And Ownership

- Manifest typed validation still loses some field-presence information after
  raw YAML validation. Be careful when constructing manifests in memory; pointer
  or per-kind wrappers may be needed for future mutations.
- Legacy runtime and orbit-template codecs still exist as narrow compatibility
  lanes. Keep them out of mainline paths and remove or quarantine them further
  when migration scenarios no longer require them.
- Remote install replay still depends on recorded commit reachability. When a
  remote refuses unadvertised SHA fetches, the current fallback can still be too
  weak for destructive replay.
- Some harness check/drift analysis paths stop too early behind structurally
  invalid install records. Improve read-only diagnostics without weakening
  fail-closed destructive behavior.
- Harness template remove lacks the same transaction-grade rollback guarantees
  as install paths.
- Ownership transfer and bundle-backed recomposition are only partially closed.
  Guidance, bootstrap, variable recomposition, and bundle record substrate
  expansion still trail the owner/affiliation model.
- Bundle-owned guidance has no first-class editing workflow. Users can compose
  and backfill, but editing ownership across bundle boundaries is still awkward.

## Authoring Schema And Current-Worktree Semantics

- OrbitSpec `behavior` is canonical, but legacy `rules` remains accepted input
  and some JSON/text compatibility names still leak. Remove this once downstream
  fixtures and callers have moved.
- Single-orbit target omission is shared across many authoring commands, but the
  helper still lives at the command layer instead of a neutral substrate.
- Current-worktree helpers exist, but their final package home and enforcement
  mechanism are not frozen. Future command work can still duplicate the logic.
- Branch status, template save/publish, harness template save/publish, and member
  hints mostly share current-worktree semantics, but invalid-manifest handling
  remains split across layers.
- Member-hint consume-on-success can still rewrite legacy frontmatter formatting
  noisily. The new flat-hint path is cleaner, but mixed legacy frontmatter may
  lose comments or exact style.
- Flat member hint discovery is intentionally conservative and can hide ambiguous
  author intent. Better diagnostics may be needed for users who expected a hint
  to be discovered.

## Guidance And Readiness UX

- Root guidance block markers work but remain visually heavy. A lighter anchor
  design could reduce reading friction while preserving compose, drift, and
  backfill tracking.
- `guide save` / `guide writeback` artifact statuses are more precise than some
  headline summary text. Make the headline status-aware or document that
  artifact-level status is authoritative.
- Missing root guidance blocks are handled safely in common non-interactive
  cases, but the richer interactive policy for render / skip / delete remains
  unfinished.
- `guide render --seed-empty` should stay append-if-missing and avoid confusing
  overwrite language when a selected block already exists.

## Tests And Documentation

- The release-surface smoke exists, but the full runtime quickstart smoke is
  still deferred until runtime fixtures are available in this repository.
- Public docs should continue to prefer `hyard` user-layer commands. Plumbing
  examples should appear only when the raw compatibility surface is genuinely
  required.
- When a new issue is opened, prefer one narrow issue or one concise entry here
  over restoring broad historical design archives.
