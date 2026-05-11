# Harness Yard Quickstart

This quickstart shows the current public path for Harness Yard CLI (hyard).

`hyard` is the public command surface.

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

Clone a published Harness Template from an explicit GitHub locator and start the
agent handoff:

```bash
hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab
cd demo-runtime
hyard start --with codex
```

Create and inspect a runtime:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard check --json
hyard ready
hyard view status
```

### Existing Repository Assembly

Use this path when you already have a Git repository and want to assemble a
Harness Runtime around it:

```bash
hyard init runtime
hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/api --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ui --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ops --bindings .harness/vars.yaml
hyard check --json
```

Each Run View Orbit Package install outputs its package guidance incrementally.
You can start using the newly installed guidance immediately; standalone
runtime-wide guidance output remains an explicit `hyard guide sync --output`
operation.

Framework Activation is the separate Agent Framework side-effect step. Preview
it before agent handoff:

```bash
hyard agent plan
hyard agent apply --yes
hyard agent check --json
```

`hyard agent apply` materializes project/global Agent Framework side effects such
as skills, commands, config, hooks, and aliases, then records ownership in the
activation ledger. It does not compose root `AGENTS.md`, `HUMANS.md`, or
`BOOTSTRAP.md`; Run View Root Guidance output remains owned by package
installation and explicit `hyard guide sync --output` guidance commands.
Harness Start runs project-local Framework Activation before handing control to
the selected Agent Framework, while Bootstrap Guide output remains a separate
guidance/bootstrap concern.

Uninstall package content with the typed package lifecycle commands:

```bash
hyard uninstall harness frontend-lab
hyard uninstall orbit docs
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

Publish the current runtime as a Harness Package after a normal Git checkpoint:

```bash
git status --short
git add .
git commit -m "Optimize frontend lab harness"
hyard publish harness workspace
```

## Authoring Next Steps

This quickstart is the Runtime User path. If the next job is to author reusable
work, use the role-specific guides:

- [Harness Authoring](./guides/harness-authoring.md): compose multiple Orbit
  Workflows into a reusable Harness Package.
- [Orbit Authoring](./guides/orbit-authoring.md): author and publish one Orbit
  Package.
- [Content And Workflows](./guides/content-and-workflows.md): decide what each
  maintained content file should contain.

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

## Acceptance Smoke Contract

The release-facing documentation surface is currently protected by:

```bash
sh ./scripts/test_release_surface_hyard.sh
```

The release-surface script validates `hyard` as the public command surface,
release asset naming, install documentation, and the separation between public
release contract and maintainer release procedure.

When runtime fixtures are added to this repository, add a dedicated quickstart acceptance smoke that executes the end-to-end runtime path.

<!-- quickstart-smoke:start -->

```bash
sh ./scripts/test_release_surface_hyard.sh
```

<!-- quickstart-smoke:end -->

Documentation and behavior should not drift: when this file, CLI help, install behavior, release packaging, or branch identity behavior changes, run the relevant release-surface or quickstart smoke before merge.
