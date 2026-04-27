# Harness Yard Quickstart

This quickstart shows the current public path for Harness Yard CLI (hyard).

`hyard` is the canonical public binary. The legacy `orbit` and `harness` command trees remain available through:

```bash
hyard plumbing orbit
hyard plumbing harness
```

## Install

Install from the latest release with Homebrew:

```bash
brew tap zack-nova/tap
brew install hyard
```

Or install with the repository install script:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | bash
```

Verify the installed CLI:

```bash
hyard --version
hyard --help
```

## Worker Path

Use this path when a runtime already exists and you need to work inside an orbit:

```bash
hyard plumbing harness inspect
hyard orbit list
hyard orbit show docs
hyard enter docs
hyard current
hyard status
hyard diff
hyard commit -m "update docs orbit"
hyard leave
```

## Runtime Author Path

Create and inspect a runtime:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard check --json
hyard ready
```

Install a reusable template or package:

```bash
hyard install <template-source>
hyard guide sync
hyard check --json
```

Manage orbit affiliation in the current harness composition:

```bash
hyard assign orbit <orbit-package>
hyard unassign orbit <orbit-package>
```

When a runtime contains multiple harness packages, select the target explicitly:

```bash
hyard assign orbit <orbit-package> --harness <harness-package>
```

Publish the current harness workspace:

```bash
hyard publish harness workspace
```

## Orbit Author Path

Create a source authoring repository for one orbit:

```bash
hyard create source docs-source --orbit docs --name "Docs Orbit" --description "Docs authoring repo"
cd docs-source
hyard orbit member add --orbit docs --key docs-content --role rule --include 'docs/**'
hyard guide save --orbit docs --target all
hyard publish orbit docs --json
```

When you need the lower-level compatibility surface:

```bash
hyard plumbing orbit template save docs --to orbit-template/docs
hyard plumbing orbit branch list --json
hyard plumbing orbit branch inspect HEAD --json
```

## Harness Author Path

Create a runtime, install content, verify readiness, and publish:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard install <template-source>
hyard plumbing harness inspect
hyard check --json
hyard guide sync
hyard publish harness workspace
```

The local raw export primitive remains available through plumbing:

```bash
hyard plumbing harness template save --to harness-template/workspace
```

## Bootstrap Completion

If an orbit bootstrap has been completed and its initialization surface should be closed:

```bash
hyard plumbing harness bootstrap complete --orbit <orbit-id>
```

If completion was accidental or the bootstrap lane needs to reopen:

```bash
hyard plumbing harness bootstrap reopen --orbit <orbit-id>
hyard plumbing harness bootstrap reopen --orbit <orbit-id> --restore-surface
```

## Acceptance Smoke Contract

The release-facing documentation surface is currently protected by:

```bash
sh ./scripts/test_release_surface_hyard.sh
```

The release-surface script validates `hyard` as the only public binary, compatibility plumbing through `hyard plumbing orbit|harness`, release asset naming, install documentation, and the separation between public release contract and maintainer release procedure.

When runtime fixtures are added to this repository, add a dedicated quickstart acceptance smoke that executes the end-to-end runtime path.

<!-- quickstart-smoke:start -->

```bash
sh ./scripts/test_release_surface_hyard.sh
```

<!-- quickstart-smoke:end -->

Documentation and behavior should not drift: when this file, CLI help, install behavior, release packaging, or branch identity behavior changes, run the relevant release-surface or quickstart smoke before merge.
