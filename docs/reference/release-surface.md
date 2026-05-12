# Release Surface

This document defines the public release contract for Harness Yard CLI.

## Public Product Name

The current release headline is:

```text
Harness Yard CLI (hyard)
```

## Canonical Binary

The canonical public binary is:

```text
hyard
```

Formal release assets distribute `hyard` only.

Public product documentation should teach the top-level `hyard` command surface.

## Installation Channels

The recommended user installation channel is Homebrew:

```bash
brew tap zack-nova/tap
brew install hyard
```

Users may also install the formula with its fully qualified name:

```bash
brew install zack-nova/tap/hyard
```

The repository install script is also supported:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | bash
```

The install script installs `hyard` from the latest release and points users to
`hyard --help`.

## Package Lifecycle Surface

The canonical top-level package lifecycle surface is:

```bash
hyard install <template-source>
hyard install <namespace>/<name>
hyard install <namespace>/<name>@<semver>
hyard install <namespace>/<name>@latest
hyard install <curated-name>
hyard install <curated-name>[@latest]
hyard uninstall orbit <orbit-package>
hyard uninstall harness <harness-package>
```

Package Handle Coordinates are case-insensitive and use
`namespace/name[@version-or-tag]` or curated `name[@version-or-tag]` syntax.
They are not npm-style `@namespace/name`. User-facing examples should prefer
package handles such as:

```bash
hyard install acme/docs
hyard install acme/docs@0.1.0
hyard install docs
```

Curated bare handles such as `docs` are reviewed aliases. `latest` is an
explicit registry dist-tag, so `acme/docs` is equivalent to `acme/docs@latest`;
documentation must not describe it as an inferred newest version or Git branch.
If fresh registry data is unavailable, stale cached bare or `latest`
resolutions may install with a warning. Mention `HYARD_CACHE_DIR` only in
troubleshooting context for the user-level registry cache.

User-facing package lifecycle documentation should prefer `uninstall`.
Scoped member-editing documentation may continue to use add/remove language, such as
`hyard orbit member add` and `hyard orbit member remove`, when it describes collection
membership rather than installed package lifecycle.

Package Uninstallation is package-manager-style removal from the current Harness
Runtime. The public `uninstall` surface deletes the active package record,
hosted OrbitSpec when no active package still owns it, unambiguous owned root
guidance, and package-owned runtime files. Retained Git history, audit output,
provenance data, detached install records, or removed hosted OrbitSpecs must not
make the package appear installed, active, reapplicable, or readiness-relevant.

## Audit Review Surface

The public read-only review command is:

```bash
hyard audit
hyard audit --json
```

Audit is scoped to the current Git worktree and reports `pass`, `warn`, `fail`,
or `not_hyard_revision`. `pass` and `warn` exit 0; `fail` and
`not_hyard_revision` exit non-zero. Public docs should describe Audit as the
standard broad review command, while keeping runtime detail on `hyard check` and
Orbit Package publish readiness on `hyard orbit prepare <package> --check --json`.

## Harness Start Demo Paths

Public demo examples should use Package Handle Coordinates once registry entries
exist. Explicit Git locators are the advanced escape hatch when no registry
entry is available.

Create a runtime, install a Harness Package by handle, and hand off to Codex:

```bash
hyard create runtime demo-runtime
cd demo-runtime
hyard install acme/frontend-lab
hyard start --with codex
```

Assemble packages into an existing Git repository with Runtime Initialization and
typed Package Installation and Package Uninstallation:

```bash
hyard init runtime
hyard install acme/frontend-lab
hyard install acme/docs --bindings .harness/vars.yaml
hyard install acme/api@latest --bindings .harness/vars.yaml
hyard install acme/ui@0.1.0 --bindings .harness/vars.yaml
hyard install docs
hyard uninstall harness frontend-lab
hyard uninstall orbit docs
```

Advanced explicit Git locator examples remain available for unpublished packages:

```bash
hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab
hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/api --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ui --bindings .harness/vars.yaml
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ops --bindings .harness/vars.yaml
```

Run View Package Installation outputs package guidance incrementally so each
newly installed package can be used immediately.

Framework Activation is the public `hyard agent plan`, `hyard agent apply`, and
`hyard agent check` surface for Agent Framework side effects. `hyard agent apply`
materializes project/global Agent Framework assets such as skills, commands,
config, hooks, and aliases, then records ownership in the activation ledger.
It must not be documented as composing root `AGENTS.md`, `HUMANS.md`, or
`BOOTSTRAP.md`. Run View Root Guidance output remains owned by Package
Installation and explicit guidance output commands. Bootstrap Guide output is a
guidance/bootstrap surface; Harness Start only discovers and executes an
existing Bootstrap Guide before bootstrap closeout.

Publish demos should make a normal Git checkpoint before publishing the current
runtime as a Harness Package:

```bash
git status --short
git add .
git commit -m "Optimize frontend lab harness"
hyard publish harness workspace
```

## Runtime View And Publication Surface

Run View is the recommended runtime-user view for a Harness Runtime. Runtime-user
documentation and examples should teach users to inspect `hyard view status`, clean
visible authoring scaffolds with `hyard view run`, and publish the current runtime
as a Harness Package.

Run View publication should use `hyard publish harness <harness-package>`.
Run View examples and next actions should not recommend Orbit Package publication
as the default way to share runtime work.

Author View is the authored-truth view. Author documentation should explain
`hyard view author`, `hyard guide render`, `hyard guide save`,
`hyard orbit content apply`, and Orbit Package publication through
`hyard publish orbit <orbit-package>`.

Orbit Package publication remains available for authoring. It should be
documented as an authoring surface, not as the recommended runtime-user
publication path.

## Registry Entry Candidate Surface

Package authors register published packages by generating a reviewable Registry
Entry Candidate and submitting the resulting YAML to the package registry
repository. Orbit and Harness candidates share the same YAML shape; validation
differs by package kind.

```bash
hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs
hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace
```

Default output is stdout. `--out <path>` writes a chosen file, and
`--registry <path>` writes under the candidate target path in a local registry
checkout. Local-only evidence can preview candidate YAML but cannot create a
submittable candidate.

Maintainer-level registry source behavior is documented in
`docs/maintainers/package-registry-source-contract.md`.

Main `hyard --help` output must stay stable across Runtime View Selection. Runtime
View Selection may affect command behavior and status/next-action output for a
runtime repository, but it must not dynamically rewrite the main CLI help surface.

## Root Guidance Marker Surface

Root guidance blocks use owner-specific marker namespaces with a single double-quoted
`workflow` attribute.

Orbit package guidance uses `orbit:` markers:

```html
<!-- orbit:begin workflow="docs" -->
<!-- orbit:end workflow="docs" -->
```

Harness package guidance uses `harness:` markers:

```html
<!-- harness:begin workflow="workspace" -->
<!-- harness:end workflow="workspace" -->
```

Root guidance marker workflow language does not rename OrbitSpec, manifest fields,
storage paths, member hints, package identity, or template branch contracts.

## Release Assets

Each release should publish platform archives that contain the `hyard` binary.

Expected archive naming pattern:

```text
hyard_${VERSION}_${GOOS}_${GOARCH}.tar.gz
```

`VERSION` is the release version without the leading `v` tag prefix.

Expected supported target matrix:

| OS | Architecture |
| --- | --- |
| linux | amd64 |
| linux | arm64 |
| darwin | amd64 |
| darwin | arm64 |

Each release should also publish:

```text
checksums.txt
```

## Version Metadata

`hyard --version` should include release metadata injected at build time:

- version
- commit
- date
- builtBy

The documentation must describe the metadata fields, but should not hard-code a specific current version.

## Homebrew Formula

The release process should update the Homebrew formula:

```text
zack-nova/homebrew-tap/Formula/hyard.rb
```

## Latest Release Downloads

The install path may rely on GitHub's latest-release download URL pattern, provided release asset names remain stable.

## Current Release Boundaries

The current release surface does not include:

- Windows prebuilt archives
- code signing
- macOS notarization
- SBOM generation
- provenance attestations

These capabilities may be added later once the release surface is stable.
