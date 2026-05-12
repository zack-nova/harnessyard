# Installation

Harness Yard installs one public CLI binary:

```text
hyard
```

## Homebrew

Homebrew is the recommended installation channel:

```bash
brew tap zack-nova/tap
brew install hyard
```

You can also install the fully qualified formula:

```bash
brew install zack-nova/tap/hyard
```

## Install Script

The repository install script downloads the latest release archive for your platform, verifies `checksums.txt`, and installs `hyard`:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | bash
```

To install into a user-writable directory:

```bash
curl -fsSL https://raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh | INSTALL_DIR="$HOME/.local/bin" bash
```

## Verify

```bash
hyard --version
hyard --help
```

## Install Packages

After `hyard` is installed, use Package Handle Coordinates for ordinary package
installation:

```bash
hyard install acme/docs
hyard install acme/docs@0.1.0
hyard install docs
```

Package Handle Coordinates are case-insensitive and use
`namespace/name[@version-or-tag]`, not npm-style `@namespace/name`. Bare handles
such as `docs` are curated aliases; `latest` is an explicit registry dist-tag,
so `acme/docs` resolves like `acme/docs@latest`.

If fresh registry data cannot be fetched, a previously verified bare or
`latest` resolution may install with a stale cached resolution warning. Set
`HYARD_CACHE_DIR` only when troubleshooting or relocating the user-level
registry cache.
