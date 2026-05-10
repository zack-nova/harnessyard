---
status: accepted
---

# 0004 Package Registry Product Integration

Harness Yard resolves Package Handle Coordinates such as `acme/docs@0.1.0`
through a Package Registry into installable Orbit Package or Harness Package
locators. The Harness Yard CLI source repository owns the registry schema,
resolver behavior, product documentation, `hyard registry entry` candidate
generation, and Package Installation semantics. The official catalog repository
owns catalog entries, namespace ownership records, curated handles, and registry
review policy.

The concrete source contract for the official catalog repository is maintained
in `docs/maintainers/package-registry-source-contract.md`.

We chose this split so the product can support package-handle installation
without mixing third-party catalog submissions into the CLI source repository.
The public registration model is catalog-as-code: authors publish a package to a
Git ref or commit, generate a registry entry candidate, and submit it to the
official catalog repository for review. The first version intentionally does not
include a hosted registration service, OAuth, account system, automatic
registration API, or automatic pull request creation.

## Decisions

- The official catalog source is `zack-nova/hyard-registry`; the resolver must
  support any Git remote as a registry source.
- Registry catalog data is YAML and includes a schema version.
- The catalog uses namespace-level indexes rather than one file per package.
  Namespace ownership is explicit and records owners as Git-platform user or org
  identities.
- Package Handle Coordinates use `namespace/name[@version-or-tag]` or curated
  `name[@version-or-tag]` syntax, not npm-style `@namespace/name`.
- Package Handle Coordinates are case-insensitive and are normalized before
  resolution. Namespace and handle segments follow the existing path-safe
  package-name convention: lowercase letters, digits, hyphens, or underscores,
  starting and ending with an alphanumeric character.
- Bare handles such as `docs` are reserved for curated catalog aliases. Ordinary
  package authors register `namespace/name`; curated aliases point at namespaced
  package handles and do not copy full version locator metadata.
- Bare `namespace/name` is equivalent to `namespace/name@latest`, and bare
  curated handles are equivalent to the curated target at `latest`.
- `latest` is an explicit registry dist-tag. The resolver must not infer it from
  a Git branch, newest registry merge, or highest SemVer version.
- Registry status is package-level. Deprecated packages warn, yanked packages
  require explicit override, and blocked packages cannot be installed.
- Each resolved install uses a commit SHA. Branches and tags are provenance and
  discovery inputs, not the final installation locator.
- Registry cache is a user-level global cache, following npm and pip style
  rather than repo-local state. Harness Yard uses an OS-native cache directory
  with `HYARD_CACHE_DIR` as the override.
- Exact-version resolutions may be cached. Bare and `latest` resolutions should
  refresh from the registry when available, but if the registry is unavailable
  and a previously verified cached resolution exists, installation may proceed
  with a warning.
- Local registry override is supported for development, tests, and private
  catalog experiments without changing the official catalog boundary.
- `hyard registry entry orbit` and `hyard registry entry harness` output the
  same YAML candidate schema. The package type and validation path vary by
  package kind, but the registry entry shape does not.
- A registry entry candidate records its intended target path, package type,
  package identity, source repository, ref, commit SHA, status, and validation
  evidence. Submittable candidates require remote repository, ref, commit
  reachability, package identity, and installability validation.
- Local-only package results may produce preview output, but they cannot produce
  a submittable registry entry.
- Registry repository CI must revalidate submitted candidates and must not trust
  local validation evidence as authoritative.

## Consequences

Registry-backed installation never guesses mutable GitHub locators. If registry
resolution cannot fetch fresh catalog data, the resolver may use a previously
verified cached resolution with a warning; without such a cache entry it fails.
Cached bare and `latest` resolutions are stale by definition when the registry is
unavailable, so the warning is part of the user-visible contract.

Registry entry candidate generation remains a separate `hyard registry entry`
command so package publication and registry submission remain independently
inspectable. Review and curation stay in the official catalog repository, while
the product repository keeps the parser, schema, resolver, installer bridge, and
candidate-generation behavior.
