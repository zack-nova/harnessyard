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
