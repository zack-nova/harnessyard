# Release Surface

This document defines the public release contract for Harness Yard CLI.

## Public Product Name

The official release headline is:

```text
Harness Yard CLI (hyard)
```

## Canonical Binary

The canonical public binary is:

```text
hyard
```

Formal release assets must distribute `hyard` only.

The legacy `orbit` and `harness` command surfaces remain available through the compatibility plumbing interface:

```bash
hyard plumbing orbit
hyard plumbing harness
```

They are compatibility surfaces, not separately distributed release binaries.

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

The install script installs `hyard` from the latest release and points users to the compatibility plumbing interface when they need legacy `orbit` or `harness` behavior.

## Package Lifecycle Surface

The canonical top-level package lifecycle surface is:

```bash
hyard install <template-source>
hyard uninstall orbit <orbit-package>
hyard uninstall harness <harness-package>
```

User-facing package lifecycle documentation should prefer `uninstall`.
Scoped member-editing documentation may continue to use add/remove language, such as
`hyard orbit member add` and `hyard orbit member remove`, when it describes collection
membership rather than installed package lifecycle.

## Harness Start Demo Paths

Public demo examples should use explicit GitHub locators until registry-backed
package handles are part of the public release surface.

Clone a Harness Template and hand off to Codex:

```bash
hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab
cd demo-runtime
hyard start --with codex
```

Assemble packages into an existing Git repository with Runtime Initialization and
typed Package Installation and Package Uninstallation:

```bash
hyard init runtime
hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab
hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml
hyard uninstall harness frontend-lab
hyard uninstall orbit docs
```

The lower-level explanatory path for `hyard start --with codex` is repo-local Agent
Framework selection plus project-only Framework Activation:

```bash
hyard agent use codex
hyard agent apply --project-only --yes
hyard bootstrap setup codex
hyard start --print-prompt
```

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
`hyard view author`, `hyard guide render`, `hyard guide save`, the `hyard guide
writeback` compatibility alias, `hyard orbit content apply`, and Orbit Package
publication through `hyard publish orbit <orbit-package>`.

Orbit Package publication remains available for authoring and compatibility. It
should be documented as an authoring surface or compatibility surface, not as the
recommended runtime-user publication path.

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
