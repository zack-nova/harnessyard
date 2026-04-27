# Installation

Harness Yard is released as a single public CLI binary:

```text
hyard
```

The legacy `orbit` and `harness` command surfaces remain available through:

```bash
hyard plumbing orbit
hyard plumbing harness
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
hyard plumbing orbit --help
hyard plumbing harness --help
```
