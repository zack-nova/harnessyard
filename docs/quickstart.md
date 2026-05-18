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

Package Handle Coordinates are the ordinary clone/install path for published
Harness Packages and Orbit Packages. Use `namespace/name[@version-or-tag]`, not
npm-style `@namespace/name`.
Package Handle Coordinates are case-insensitive and normalize before resolution.

Clone a published Harness Package by handle and start the agent handoff:

```bash
hyard clone acme/frontend-lab demo-runtime
cd demo-runtime
hyard start --with codex
```

Create and inspect a runtime:

```bash
hyard create runtime demo-repo
cd demo-repo
hyard check --json
hyard audit --json
hyard ready
hyard view status
```

### Existing Repository Assembly

Use this path when you already have a Git repository and want to assemble a
Harness Runtime around it:

```bash
hyard init runtime
hyard install acme/frontend-lab
hyard install acme/docs --bindings .harness/vars.yaml
hyard install acme/docs@0.1.0 --bindings .harness/vars.yaml
hyard install acme/api@latest --bindings .harness/vars.yaml
hyard install docs
hyard check --json
hyard audit --json
```

Bare handles such as `docs` are curated aliases reviewed by the package
registry. `latest` is an explicit registry dist-tag, so `acme/docs` is the same
request as `acme/docs@latest`; it is not inferred from a Git branch, newest
catalog merge, or highest SemVer version.

If fresh registry data cannot be fetched, Harness Yard may use a previously
verified bare or `latest` resolution with a stale cached resolution warning.
Set `HYARD_CACHE_DIR` only when you need to relocate or inspect the user-level
registry cache while troubleshooting.

### Runtime Bindings

Packages may declare Package Variables that must be supplied by the runtime.
Runtime Bindings live in `.harness/vars.yaml` and use schema `2`.

Create a skeleton from a package before installing when required values are
missing:

```bash
hyard vars init acme/docs --out .harness/vars.yaml
hyard vars validate
hyard install acme/docs --bindings .harness/vars.yaml
```

The skeleton uses the public Runtime Bindings shape:

```yaml
schema_version: 2
variables:
  project_name:
    value: Harness Yard Docs
  github_token:
    value_from:
      env: GITHUB_TOKEN
```

Package-owned runtime files reference Package Variables with strict Package
Template References such as `{{ vars.project_name }}`. Missing required Runtime
Bindings block installation before package-owned runtime files are written.

Maintainer-level registry behavior is documented in
`docs/maintainers/package-registry-source-contract.md`.

Use `hyard audit` as the standard read-only review command for the current
worktree. Audit reports one of four statuses: `pass`, `warn`, `fail`, or
`not_hyard_revision`. `pass` and `warn` exit 0; `fail` and
`not_hyard_revision` exit non-zero. Audit is scoped to the current Git worktree
and does not publish packages, install templates, initialize runtimes, or rewrite
authored truth.

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

Uninstall behaves like a package manager operation for the current Harness
Runtime. It removes the active package record, hosted package truth, unambiguous
owned root guidance, and package-owned runtime files; retained Git history or
audit evidence does not keep the package installed, active, reapplicable, or
readiness-relevant.

Install a reusable template or package:

```bash
hyard clone <namespace>/<harness-name> [repo-name]
hyard clone <curated-harness-name> [repo-name]
hyard install <namespace>/<name>
hyard install <namespace>/<name>@<semver>
hyard install <curated-name>
hyard install <template-source>
hyard check --json
hyard view run --check
```

Explicit Git locators remain available as an advanced escape hatch when no
registry entry exists yet:

```bash
hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab
hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml
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

After uninstall, `hyard check --json` and `hyard ready` evaluate the remaining
runtime. Deleted install records and removed hosted OrbitSpecs are not treated as
active package state.

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
