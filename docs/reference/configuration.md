# Configuration Reference

This reference explains the user-maintainable configuration contract for
Harness Yard. It focuses on program-readable files, editing policies,
conformance requirements, and validation commands.

It does not define every internal YAML field. Advanced users may inspect YAML
directly, but user-facing documentation should point ordinary changes through
`hyard` commands and validation.

## Editing Policies

Harness Yard files are visible so users can inspect and commit them, but visible
files have different editing policies.

| Policy | Visible files | Modify with | Validate with |
| --- | --- | --- | --- |
| Tool-owned control-plane truth | `.harness/manifest.yaml`, `.harness/installs/*.yaml`, `.harness/bundles/*.yaml`, `.harness/template.yaml`, `.harness/template_members/*.yaml` | `hyard create`, `hyard init`, `hyard install`, `hyard uninstall`, `hyard publish`, `hyard assign`, `hyard unassign` | `hyard audit`, `hyard check` |
| Authored package truth | `.harness/orbits/*.yaml`, `.harness/vars.yaml` | `hyard orbit ...`, `hyard guide save`, installation and bindings commands; advanced users may hand-edit | `hyard orbit validate`, `hyard orbit prepare <package> --check`, `hyard check`, `hyard audit` |
| Editable content and presentation | `AGENTS.md`, `HUMANS.md`, `BOOTSTRAP.md`, authored docs/content, `orbit_member` hints, package command and skill assets | Edit files directly, then run `hyard guide save`, `hyard view run`, or `hyard orbit content apply` when applicable | `hyard view status`, `hyard check`, `hyard audit` |
| Repo-local state and cache | `.git/orbit/state/*` | Harness Yard commands only | Re-run the command that produced the state, or use `hyard check` / `hyard audit` |

Users should not hand-edit tool-owned control-plane truth. If advanced users
hand-edit authored package truth, they must validate before publication or
runtime use.

## Revision Identity

The single versioned revision identity host is:

```text
.harness/manifest.yaml
```

Current revision kinds are:

| Kind | Meaning |
| --- | --- |
| `runtime` | Harness Runtime |
| `source` | Source Revision |
| `orbit_template` | Orbit Template Revision |
| `harness_template` | Harness Template |

A revision has exactly one kind. Runtime View selection is separate; Run View
and Author View do not change revision identity.

## Versioned Truth Files

Harness Yard stores versioned truth under `.harness/*`.

| Path | Responsibility | Normal edit path |
| --- | --- | --- |
| `.harness/manifest.yaml` | Revision identity. | `hyard create`, `hyard init`, `hyard clone`, `hyard publish` |
| `.harness/orbits/*.yaml` | Hosted OrbitSpec authored truth. | `hyard orbit ...`, `hyard guide save`, validated advanced edits |
| `.harness/vars.yaml` | Runtime-owned package variable bindings. | Bindings helpers, install commands, validated advanced edits |
| `.harness/installs/*.yaml` | Installed package records. | `hyard install`, `hyard uninstall` |
| `.harness/bundles/*.yaml` | Harness package composition records. | `hyard publish`, `hyard assign`, `hyard unassign` |
| `.harness/template.yaml` | Template metadata. | Publish or template commands |
| `.harness/template_members/*.yaml` | Template member records. | Publish or template commands |

Do not commit `.git/orbit/state/*`. It is repo-local runtime state and cache,
not versioned truth.

## Package Identity

Package Identity is stable user-facing package identity:

```yaml
package:
  type: orbit
  name: docs
  version: 1.2.3
```

`package.type` distinguishes Orbit Packages from Harness Packages.
`package.name` is the stable package name. Display `name` and `description`
metadata are not stable references and must not be used as package identity.

Users should use short, stable, path-safe package names. Use
`hyard orbit rename <old-package> <new-package>` when renaming an Orbit Package
rather than renaming files, directories, or branches by hand.

## Runtime View Configuration

Run View is the default runtime-user presentation. Author View is selected when
a Harness author or Orbit author edits authored truth, marked guidance, or
content hints inside a runtime.

| Convention | Applies to | Visible files | Modify with | Default | Recommended | Validate with |
| --- | --- | --- | --- | --- | --- | --- |
| Runtime presentation | Harness Runtime | `AGENTS.md`, `HUMANS.md`, `BOOTSTRAP.md` | `hyard view run`, direct presentation edits | Run View for runtime users | Runtime users stay in Run View | `hyard view status`, `hyard check` |
| Authoring presentation | Harness Runtime | marked root guidance, member hints, authored package truth | `hyard view author`, `hyard guide render`, `hyard guide save`, `hyard orbit content apply` | Not selected by default | Harness authors and Orbit authors select Author View while editing authored truth | `hyard view status`, `hyard orbit validate`, `hyard check` |

Markerless Run View root guidance is presentation text. Users may edit it
directly, but those edits do not automatically update Orbit Package authored
truth.

## Root Guidance Markers

Marked root guidance blocks preserve ownership for save and cleanup workflows.
Markers use owner-specific namespaces and one double-quoted `workflow`
attribute:

```html
<!-- orbit:begin workflow="docs" -->
<!-- orbit:end workflow="docs" -->

<!-- harness:begin workflow="workspace" -->
<!-- harness:end workflow="workspace" -->
```

The `workflow` value equals the owning package name for that owner kind. It is
marker syntax, not a separate Package Identity.

When Run View cleanup finds drifted marked guidance, users must resolve it by
choosing one of the reported paths: save the current block to authored truth,
re-render authored truth, or strip markers in place and keep the current text as
Run View root guidance.

## Package Variables

Packages may declare Package Variables. Absence of variables means the package
needs no user-provided values.

Runtime users usually provide bindings with:

```bash
hyard install <package-source> --bindings .harness/vars.yaml
```

Reusable runtime-owned bindings belong in `.harness/vars.yaml` when the runtime
owns them. This document locates bindings files and commands; it does not define
the complete bindings YAML schema.

## Member Hints

Orbit authors may use member hints as temporary authoring input for ordinary
installable content. Hints are not used for local skills, prompt commands, or
root guidance files.

Markdown files may declare nested `orbit_member` frontmatter:

```yaml
---
orbit_member:
  name: docs-rules
  description: Documentation rules
  role: rule
  lane: bootstrap
---
```

Directories may use `.orbit-member.yaml`:

```yaml
orbit_member:
  name: docs-process
  role: process
```

Supported fields are `name`, `description`, `role`, and `lane`. Supported roles
are `subject`, `rule`, and `process`. A directory marker defaults to `process`
when `role` is omitted. The only canonical lane value currently defined is
`bootstrap`.

Run content hint checks before publishing:

```bash
hyard orbit content apply <package> --check --json
hyard orbit content apply <package>
```

`--with-spec` authoring bootstrap does not use member hints. It writes the
generated `spec` rule member to authored package truth directly and creates
`docs/<orbit-id>/README.md` without `orbit_member` hint metadata.

## Package Assets

Local commands and local skills are package assets when declared by an Orbit
Package. Recommended positions are:

```text
commands/<orbit-id>/**/*.md
skills/<orbit-id>/*
```

Project-local activation is the default Harness Start path. Global or hybrid
agent activation requires explicit user choice. Users should apply and inspect
agent activation through:

```bash
hyard agent ...
hyard start
hyard hooks ...
```

Remote skill dependencies may be diagnosed, but unresolved remote skill pinning
or full materialization is not a stable user contract in this reference.

## Validation Matrix

Use the validation command that matches the revision and risk.

| Revision or surface | Validate with |
| --- | --- |
| Harness Runtime | `hyard check`, `hyard ready`, `hyard audit` |
| Harness Template | `hyard audit` |
| Orbit Template Revision | `hyard orbit validate`, `hyard audit` |
| Source Revision | `hyard orbit validate`, `hyard orbit prepare <package> --check`, `hyard audit` |
| Run View / Author View state | `hyard view status`, `hyard check` |
| Member hints | `hyard orbit content apply <package> --check --json` |

For documentation-only edits in this repository, also run:

```bash
git diff --check
```

## Prohibited User Actions

- Do not rely on display names as Package Identity.
- Do not publish or share a revision while knowingly leaving versioned Harness
  Yard truth uncommitted.
- Do not use Source Revisions or Orbit Template Revisions to compose multiple
  Orbit Packages.
- Do not treat markerless Run View guidance as Orbit Package authored truth.
- Do not hand-edit generated native agent files as the source of truth when
  Harness Yard owns those files.
- Do not hand-edit tool-owned control-plane YAML unless you are deliberately
  doing advanced recovery and will validate afterward.
