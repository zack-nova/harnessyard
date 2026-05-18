# Harness Authoring

Use this guide when you want to turn a working Harness Runtime into a reusable
Harness Package.

If you only want to run an installed harness, use the
[Quickstart](../quickstart.md). If you want to author one reusable Orbit
Workflow, use [Orbit Authoring](./orbit-authoring.md).

## Goal

A Harness author composes multiple Orbit Workflows into one agent work system,
checks that the runtime is coherent, commits the versioned truth and maintained
content, and publishes a Harness Package:

```bash
hyard publish harness <harness-package>
```

The output is a reusable harness that another repository can install or clone.

## When You Are A Harness Author

You are acting as a Harness author when you:

- assemble multiple Orbit Packages into one Harness Runtime
- define how those workflows relate to each other
- tune root guidance, human orientation, variables, and package membership
- decide whether the whole runtime is ready to publish as a Harness Package

You are not acting as an Orbit author when the change belongs to one workflow's
authored truth. In that case, switch to the owning Orbit Package path and publish
that Orbit Package instead.

## First Success Path

Start from a runtime repository:

```bash
hyard init runtime
```

Install the harness or orbit packages that make up the work system:

```bash
hyard install acme/frontend-lab
hyard install acme/docs --bindings .harness/vars.yaml
hyard install acme/api@latest --bindings .harness/vars.yaml
hyard install acme/ui@0.1.0 --bindings .harness/vars.yaml
hyard install docs
```

Use Package Handle Coordinates for ordinary installs. Explicit Git locators are
still available when a package has not been registered yet, but they should be
treated as an advanced escape hatch.

Inspect package membership and readiness:

```bash
hyard orbit list
hyard view status
hyard check --json
hyard ready
```

Review the Git checkpoint:

```bash
git status --short
git diff
```

Commit the runtime baseline:

```bash
git add .
git commit -m "Optimize frontend lab harness"
```

Publish the composed Harness Package:

```bash
hyard publish harness workspace
```

Generate a reviewable Registry Entry Candidate when the published Harness
Package is ready to be registered:

```bash
hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace
```

The candidate uses the same YAML schema as Orbit candidates while validating the
Harness Package identity and install preview path. Use `--registry <path>` only
when you want to update the final namespace catalog index in a local registry
checkout.

## Composition Work

Use package lifecycle commands for installed package membership:

```bash
hyard install <namespace>/<name>
hyard install <namespace>/<name>@<semver>
hyard install <curated-name>
hyard install <package-source>
hyard uninstall orbit <orbit-package>
hyard uninstall harness <harness-package>
```

Use affiliation commands when an installed orbit should be associated with or
removed from the current harness composition:

```bash
hyard assign orbit <orbit-package>
hyard unassign orbit <orbit-package>
```

When a runtime contains multiple harness packages, select the target explicitly:

```bash
hyard assign orbit <orbit-package> --harness <harness-package>
```

Prefer `install` and `uninstall` for package lifecycle. Reserve add/remove
language for scoped collection membership such as orbit members.

Uninstall fully removes the package from the current Harness Runtime. Deleted
install records, removed hosted OrbitSpecs, and retained provenance or audit
evidence are not active package state and should not affect `hyard check` or
`hyard ready`.

## Variables

Plan Package Variables before installing a group of related packages. Runtime
owned bindings usually belong in:

```text
.harness/vars.yaml
```

Install packages with explicit bindings when variables are required:

```bash
hyard vars init acme/docs --out .harness/vars.yaml
hyard install <package-source> --bindings .harness/vars.yaml
```

Runtime Bindings use schema `2`:

```yaml
schema_version: 2
variables:
  project_name:
    value: Harness Yard Docs
  github_token:
    value_from:
      env: GITHUB_TOKEN
```

Package Variables are referenced from package-owned runtime files with strict
`{{ vars.project_name }}` Package Template References. Declaration defaults
satisfy required Package Variables without writing a Runtime Binding unless the
user explicitly asks `hyard vars init --defaults` to materialize defaults.
Sensitive Package Variables should use `value_from.env`.

Zero variables is a complete contract. Do not add placeholder bindings just to
make a file look complete.

## Root Guidance

Run View is the default runtime-user presentation. It is the normal view for a
published runtime surface.

Use Author View while editing authored truth, marked root guidance, or content
hints:

```bash
hyard view author
hyard guide render --orbit docs --target all
hyard guide save --orbit docs --target all
```

Return to Run View when you want the runtime presentation cleaned for ordinary
use:

```bash
hyard view run
hyard view status
```

If Run View cleanup reports drifted marked guidance, choose one of the reported
resolution paths before stripping owner identity: save, re-render, or strip in
place.

## Publication Checks

Before publishing a Harness Package:

- run `hyard check --json`
- run `hyard ready`
- inspect `hyard view status`
- confirm `.harness/*` truth and maintained runtime content are committed
- confirm root guidance does not contain stale authoring scaffolds unless the
  package intentionally exports them
- confirm bootstrap content is still needed before publishing it

Use:

```bash
git status --short
```

Do not publish or share a revision while knowingly leaving versioned Harness
Yard truth uncommitted.

## When To Fix An Orbit Instead

Do not solve single-workflow problems by adding harness-level instructions. Fix
the owning Orbit Package when:

- the workflow objective or scope boundary is wrong
- a skill's runtime procedure is incomplete
- member hints or content roles are wrong
- a package-owned command or skill asset is malformed
- the same fix should apply wherever that Orbit Package is installed

Then validate and publish the Orbit Package through the Orbit authoring path.

## Validation

Use these checks while authoring a harness:

| Risk | Check |
| --- | --- |
| Runtime structure and package truth | `hyard check --json` |
| Runtime publish readiness | `hyard ready` |
| View and root guidance state | `hyard view status` |
| Broad control-plane consistency | `hyard audit` |
| Documentation-only repository edits | `git diff --check` |
