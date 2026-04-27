#!/usr/bin/env bash
set -euo pipefail

OWNER="${OWNER:-zack-nova}"
REPO="${REPO:-harnessyard}"
PROJECT="hyard"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
BIN="hyard"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

download() {
  curl -fsSL --retry 3 --retry-delay 1 "$@"
}

need_cmd awk
need_cmd curl
need_cmd head
need_cmd install
need_cmd mktemp
need_cmd sed
need_cmd tar
need_cmd tr
need_cmd uname

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  linux)
    os="linux"
    ;;
  darwin)
    os="darwin"
    ;;
  *)
    echo "unsupported os: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64 | amd64)
    arch="amd64"
    ;;
  arm64 | aarch64)
    arch="arm64"
    ;;
  *)
    echo "unsupported arch: $arch" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  release_json="$(download "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest")"
  tag="$(printf '%s\n' "$release_json" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$tag" ]; then
    echo "could not determine latest release tag" >&2
    exit 1
  fi
else
  case "$VERSION" in
    v*)
      tag="$VERSION"
      ;;
    *)
      tag="v$VERSION"
      ;;
  esac
fi
asset_version="${tag#v}"
asset="${PROJECT}_${asset_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/${OWNER}/${REPO}/releases/download/${tag}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading ${asset}..."
download "${base_url}/${asset}" -o "${tmpdir}/${asset}"
download "${base_url}/checksums.txt" -o "${tmpdir}/checksums.txt"

echo "Verifying checksum..."
if ! expected="$(awk -v asset="$asset" '$2 == asset { print $1; found=1; exit } END { exit !found }' "${tmpdir}/checksums.txt")"; then
  echo "checksum not found for ${asset}" >&2
  exit 1
fi
if [ -z "$expected" ]; then
  echo "checksum not found for ${asset}" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmpdir}/${asset}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${tmpdir}/${asset}" | awk '{print $1}')"
else
  echo "no SHA256 tool found; install sha256sum or shasum and retry" >&2
  exit 1
fi

if [ "$expected" != "$actual" ]; then
  echo "checksum mismatch for ${asset}" >&2
  exit 1
fi

echo "Extracting..."
tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"
bundle_dir="${tmpdir}/${asset%.tar.gz}"
binary_path="${bundle_dir}/${BIN}"
if [ ! -f "$binary_path" ]; then
  binary_path="${tmpdir}/${BIN}"
fi
if [ ! -f "$binary_path" ]; then
  echo "archive did not contain ${BIN}" >&2
  exit 1
fi

echo "Installing to ${INSTALL_DIR}..."
if mkdir -p "$INSTALL_DIR" 2>/dev/null && [ -w "$INSTALL_DIR" ]; then
  install -m 0755 "$binary_path" "${INSTALL_DIR}/${BIN}"
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "cannot write to ${INSTALL_DIR}, and sudo is unavailable" >&2
    exit 1
  fi
  sudo mkdir -p "$INSTALL_DIR"
  sudo install -m 0755 "$binary_path" "${INSTALL_DIR}/${BIN}"
fi

echo
echo "Installed:"
echo "  ${INSTALL_DIR}/${BIN}"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo
    echo "Note: ${INSTALL_DIR} is not on your PATH."
    echo "Add it to your shell profile to run '${BIN}' directly."
    ;;
esac

echo
echo "Run: hyard --help"
echo "Run: hyard plumbing orbit --help"
echo "Run: hyard plumbing harness --help"
