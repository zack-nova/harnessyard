# Package Registry Source Contract

This document records the product-side contract for the official Harness Yard
Package Registry source. It is about how `hyard` understands registry data and
registry entry candidates; review policy and catalog contents live in the
official catalog repository.

Parent product work: https://github.com/zack-nova/harnessyard/issues/90

## Source Model

- The default official catalog source is `zack-nova/hyard-registry`.
- `hyard` must support any Git remote as a registry source, not only GitHub.
- Public registration is catalog-as-code: authors submit registry entry changes
  to the official catalog repository for review.
- The first public registration model does not include a hosted registration
  service, account system, OAuth flow, automatic registration API, or automatic
  pull request creation.

## Repository Boundary

The Harness Yard product repository owns:

- Package Handle Coordinate parsing and normalization.
- Registry schema and resolver behavior.
- Registry-backed package installation semantics.
- `hyard registry entry` candidate generation.
- Product documentation for package-handle install and registration.

The official catalog repository owns:

- Registry catalog entries.
- Namespace ownership records.
- Curated bare handles.
- Catalog review policy.
- Registry CI validation for submitted entries.

## Catalog Layout

Registry catalog data is YAML and includes a schema version.

The catalog uses namespace-level indexes rather than one file per package:

```text
namespaces/<namespace>.yaml
packages/<namespace>/index.yaml
curated/index.yaml
```

`namespaces/<namespace>.yaml` records namespace ownership. Owners are structured
Git-platform user or org identities, such as GitHub users or GitLab groups.

`packages/<namespace>/index.yaml` records the packages under one namespace.
Each package entry records status, `dist_tags`, version metadata, source locator
metadata, and validation evidence.

`curated/index.yaml` records curated bare handles such as `docs`. A curated
handle uses `target` to point at a namespaced Package Handle and does not copy
full version locator metadata.

Namespace package indexes use this shape:

```yaml
schema_version: 1
namespace: acme
packages:
  docs:
    handle: acme/docs
    status: active
    package:
      type: orbit
      name: docs
    source:
      repository: https://github.com/acme/docs-orbit
    dist_tags:
      latest: "0.1.0"
    versions:
      "0.1.0":
        locator:
          kind: git
          repository: https://github.com/acme/docs-orbit
          ref: orbit-template/docs
          commit: 0123456789abcdef0123456789abcdef01234567
        validation:
          remote_ref: refs/heads/orbit-template/docs
          manifest: .harness/manifest.yaml
          package_manifest: .harness/orbits/docs.yaml
          package_identity:
            type: orbit
            name: docs
```

`package.type` is `orbit` or `harness`. `package.name` is the published package
identity validated from package-owned truth. `versions.<version>.locator`
contains the commit-pinned install locator. Registry resolvers should read this
catalog index schema directly; historical version-local
`package_type` / `package_identity` / `source.remote` entries are not the current
catalog schema.

## Coordinate Rules

Supported Package Handle Coordinate forms:

```text
namespace/name
namespace/name@<semver>
namespace/name@latest
name
name@<semver>
name@latest
```

Rules:

- Coordinates are case-insensitive and normalized before resolution.
- `namespace/name` is equivalent to `namespace/name@latest`.
- `name` is a curated bare handle and is equivalent to `name@latest`.
- `latest` is an explicit registry dist-tag.
- `latest` must not be inferred from a Git branch, newest registry merge, or
  highest SemVer version.
- Namespace and handle segments follow the existing path-safe package-name
  convention: lowercase letters, digits, hyphens, or underscores, starting and
  ending with an alphanumeric character.
- npm-style `@namespace/name` syntax is not used because `@` is the version or
  dist-tag separator.

## Bare Handles

Bare handles are globally scarce names and are reserved for curated catalog
aliases.

- Ordinary authors register `namespace/name`.
- Bare handles are reviewed through the official catalog curation process.
- Bare handles point at namespaced handles.
- Bare handles do not own independent version locator metadata.

## Status And Resolution

Registry status is package-level:

- `active`: install normally.
- `deprecated`: install with a warning.
- `yanked`: require explicit user override.
- `blocked`: never install.

Each resolved install uses a commit SHA. Branches and tags may appear as
provenance or discovery inputs, but the install bridge receives the resolved
commit-pinned locator.

Registry-backed installation must not guess GitHub locators or mutable branches.

## Cache

Registry cache is user-level global cache, npm/pip style, not repo-local
`.git/orbit/state` cache.

Default cache roots:

```text
Linux:   ${XDG_CACHE_HOME:-~/.cache}/hyard
macOS:   ~/Library/Caches/hyard
Windows: %LocalAppData%/hyard/Cache
```

`HYARD_CACHE_DIR` overrides the cache root.

Cache semantics:

- Cache keys include the canonical registry remote and normalized Package Handle
  Coordinate.
- Exact-version resolutions may be cached.
- Bare and `latest` resolutions refresh from the registry when available.
- If the registry is unavailable and a previously verified cached resolution
  exists, installation may proceed with a warning.
- If there is no usable cached resolution, installation fails.
- Cached bare and `latest` resolutions are stale by definition when the registry
  is unavailable; the warning is sufficient for the first version.
- A cached entry that is already known as `blocked` still cannot install.

## Local Registry Override

Local registry override is supported for development, tests, and private catalog
experiments. It does not change the official catalog boundary.

The implementation should support local paths and Git remotes as override
sources. Exact option and config names may be chosen during implementation, but
they must not conflict with `HYARD_CACHE_DIR`.

## Registry Entry Candidates

`hyard registry entry orbit` and `hyard registry entry harness` output the same
YAML candidate schema. The package type and validation path vary by package
kind, but the entry shape does not.

Candidate behavior:

- Default output is stdout.
- `--out <path>` may write the candidate to a chosen file.
- `--registry <path>` writes or updates the final namespace catalog index under
  a local registry checkout at the intended target path, using the current
  `packages.<name>.package` and `versions.<version>.locator` schema.
- Local-only package results may produce preview output, but they cannot produce
  submittable registry entries or catalog index updates.

A submittable registry entry candidate records:

- schema version
- intended target path
- package type
- package identity
- source Git remote
- source ref
- resolved commit SHA
- package status
- validation evidence

Validation evidence must cover:

- source remote reachability
- ref resolution
- commit reachability
- package identity match
- installability through the existing package install preview path

The official catalog repository CI must revalidate submitted candidates and must
not trust local validation evidence as authoritative.

## Implementation Notes

Changing the semantic rules or catalog index field names in this document
requires updating the accepted ADR and the parent PRD issue.
