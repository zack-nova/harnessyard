# Release Guide

This document describes the maintainer workflow for releasing Harness Yard CLI.

For the public release contract, see [Release Surface](../reference/release-surface.md).

## Release Summary

The official release headline is:

```text
Harness Yard CLI (hyard)
```

The canonical release binary is:

```text
hyard
```

The release process builds, packages, and uploads `hyard`. The legacy `orbit` and `harness` surfaces are preserved through:

```bash
hyard plumbing orbit
hyard plumbing harness
```

They must not be released as separate public binaries.

## Prerequisites

Before creating a release, ensure the working tree is clean:

```bash
git status --short
```

Run the standard repository checks:

```bash
mise run fix
mise run ci
```

When release-surface behavior, help output, quickstart commands, or release documentation changes, also run:

```bash
sh ./scripts/test_release_surface_hyard.sh
```

## GoReleaser Validation

Validate the GoReleaser configuration:

```bash
goreleaser check
```

Run a local snapshot release:

```bash
goreleaser release --snapshot --clean
```

The snapshot build should confirm that `hyard` is built and packaged successfully.

## Tagging

Create an annotated SemVer tag:

```bash
VERSION=vX.Y.Z
git tag -a "$VERSION" -m "$VERSION"
git push origin "$VERSION"
```

The release workflow runs after the tag is pushed.

## Release Automation

The release workflow is defined in:

```text
.github/workflows/release.yml
```

The GoReleaser configuration is defined in:

```text
.goreleaser.yaml
```

The workflow should:

- build `hyard`
- package `hyard` into platform archives
- inject version metadata
- generate checksums
- create a GitHub Release
- update the Homebrew formula

## Expected Artifact Pattern

Release archives should use this naming pattern:

```text
hyard_${VERSION}_${GOOS}_${GOARCH}.tar.gz
```

Expected targets:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
```

The release should also include:

```text
checksums.txt
```

## Post-Release Verification

After the GitHub Release is created, verify:

```bash
hyard --version
hyard --help
hyard plumbing orbit --help
hyard plumbing harness --help
```

Verify Homebrew installation:

```bash
brew update
brew install zack-nova/tap/hyard
hyard --version
```

Verify the repository install script:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | bash
hyard --version
```

## Release Surface Rules

A release is valid only if:

- `hyard` is the only public binary distributed as a release asset
- the release headline uses `Harness Yard CLI (hyard)`
- the install script installs `hyard`
- Homebrew installs `hyard`
- compatibility commands are available through `hyard plumbing orbit|harness`
- release assets follow the documented naming pattern
- checksums are published

## Out of Scope

The current release process does not yet include:

- Windows prebuilt archives
- code signing
- macOS notarization
- SBOM generation
- provenance attestations
