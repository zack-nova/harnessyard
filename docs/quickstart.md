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

## Runtime User Path

Use this path when you want to run, inspect, update, or publish a Harness Runtime.
Run View is the default runtime-user presentation: it keeps authored scaffolding out
of the ordinary working tree and makes current runtime publication the recommended
sharing path.

Create and inspect a runtime:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard check --json
hyard ready
hyard view status
```

Install a reusable template or package:

```bash
hyard install <template-source>
hyard check --json
hyard view run --check
```

Review installed orbit packages and work inside the current runtime:

```bash
hyard orbit list
hyard orbit show docs
hyard enter docs
hyard current
hyard status
hyard diff
hyard commit -m "update runtime docs"
hyard leave
```

Uninstall an installed orbit package when it is no longer needed:

```bash
hyard uninstall orbit <orbit-package>
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

Publish the current runtime as a Harness Package:

```bash
hyard publish harness workspace
```

## Author Path

Use this path when you are editing authored truth, guide blocks, or package content
intended to become an Orbit Package. Author View makes authored scaffolds explicit;
Orbit Package publication remains available for authoring and compatibility, but it
is not the recommended runtime-user publication path.

When you are authoring inside a Harness Runtime, select Author View before
materializing editable guide artifacts:

```bash
hyard view author
```

Render editable guide artifacts, save edited guidance back into authored truth, or
use the writeback alias when following older authoring instructions:

```bash
hyard guide render --orbit docs --target all
hyard guide save --orbit docs --target all
hyard guide writeback --orbit docs --target all
```

Apply tracked content hints before publishing:

```bash
hyard orbit content apply docs --check --json
hyard orbit content apply docs
```

Publish the authored orbit as an Orbit Package:

```bash
hyard publish orbit docs --json
```

Create a source authoring repository when you need a standalone starting point for
one orbit package:

```bash
hyard create source docs-source --orbit docs --name "Docs Orbit" --description "Docs authoring repo"
cd docs-source
hyard orbit member add --orbit docs --key docs-content --role rule --include 'docs/**'
```

Rename an authored orbit package when the package identity needs to change:

```bash
hyard orbit rename docs api
```

When you need the lower-level compatibility surface:

```bash
hyard plumbing orbit template save docs --to orbit-template/docs
hyard plumbing orbit branch list --json
hyard plumbing orbit branch inspect HEAD --json
```

Authoring a reusable Harness Package follows the runtime publication path after the
runtime content is reviewed:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard install <template-source>
hyard plumbing harness inspect
hyard check --json
hyard view status
hyard publish harness workspace
```

The local raw export primitive remains available through plumbing:

```bash
hyard plumbing harness template save --to harness-template/workspace
```

## Bootstrap Completion

To install the repository-level agent skill that guides a pending runtime bootstrap:

```bash
hyard bootstrap setup
hyard bootstrap setup codex
hyard bootstrap setup --remove
```

If the repository bootstrap has been completed and its initialization surface should be closed:

```bash
hyard bootstrap complete --check --json
hyard bootstrap complete --yes
```

Bootstrap closeout treats bootstrap-lane runtime files as closeout artifacts:
`--check` lists tracked, modified, staged, and untracked matches, and `--yes`
removes the listed paths.

If completion was accidental or the bootstrap lane needs to reopen:

```bash
hyard bootstrap reopen
hyard bootstrap reopen --restore-surface
```

The lower-level per-orbit compatibility commands remain available under
`hyard plumbing harness bootstrap` when maintainers need targeted recovery.

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
