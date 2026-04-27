#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
target_script="$repo_root/install.sh"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

assert_contains() {
  file=$1
  expected=$2

  if ! grep -Fq "$expected" "$file"; then
    echo "expected $file to contain: $expected" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_executable() {
  path=$1

  if [ ! -x "$path" ]; then
    echo "expected executable file: $path" >&2
    exit 1
  fi
}

sha256_file() {
  path=$1

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$path" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$path" | awk '{print $1}'
  else
    echo "no SHA256 tool found for test" >&2
    exit 1
  fi
}

detect_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')

  case "$os" in
    linux)
      printf '%s\n' linux
      ;;
    darwin)
      printf '%s\n' darwin
      ;;
    *)
      echo "unsupported test os: $os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  arch=$(uname -m)

  case "$arch" in
    x86_64 | amd64)
      printf '%s\n' amd64
      ;;
    arm64 | aarch64)
      printf '%s\n' arm64
      ;;
    *)
      echo "unsupported test arch: $arch" >&2
      exit 1
      ;;
  esac
}

bash -n "$target_script"

fake_bin="$tmpdir/bin"
fixture_dir="$tmpdir/fixture"
install_dir="$tmpdir/install"
mkdir -p "$fake_bin" "$fixture_dir" "$install_dir"

os=$(detect_os)
arch=$(detect_arch)
asset="hyard_1.2.3_${os}_${arch}.tar.gz"
bundle_dir="${asset%.tar.gz}"
mkdir -p "$fixture_dir/$bundle_dir"
printf '#!/bin/sh\nprintf "hyard fixture\\n"\n' >"$fixture_dir/$bundle_dir/hyard"
chmod +x "$fixture_dir/$bundle_dir/hyard"
tar -czf "$fixture_dir/$asset" -C "$fixture_dir" "$bundle_dir"

checksum=$(sha256_file "$fixture_dir/$asset")
regex_like_asset="hyard_1x2x3_${os}_${arch}xtarxgz"
{
  printf '%s  %s\n' "000000" "$regex_like_asset"
  printf '%s  %s\n' "$checksum" "$asset"
} >"$fixture_dir/checksums.txt"

cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
set -eu

out=""
url=""
saw_retry=0
saw_retry_delay=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      out=$2
      shift 2
      ;;
    --retry)
      if [ "$2" != "3" ]; then
        echo "unexpected retry count: $2" >&2
        exit 1
      fi
      saw_retry=1
      shift 2
      ;;
    --retry-delay)
      if [ "$2" != "1" ]; then
        echo "unexpected retry delay: $2" >&2
        exit 1
      fi
      saw_retry_delay=1
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url=$1
      shift
      ;;
  esac
done

if [ "$saw_retry" -ne 1 ] || [ "$saw_retry_delay" -ne 1 ]; then
  echo "curl retry flags were not provided" >&2
  exit 1
fi

case "$url" in
  *"/releases/latest")
    printf '{\n  "tag_name": "v1.2.3"\n}\n'
    ;;
  *"/checksums.txt")
    cp "$FIXTURE_DIR/checksums.txt" "$out"
    ;;
  *".tar.gz")
    cp "$FIXTURE_DIR/${url##*/}" "$out"
    ;;
  *)
    echo "unexpected url: $url" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$fake_bin/curl"

output_file="$tmpdir/install-output.txt"
FIXTURE_DIR="$fixture_dir" PATH="$fake_bin:$PATH" INSTALL_DIR="$install_dir" VERSION=latest bash "$target_script" >"$output_file"

assert_executable "$install_dir/hyard"
assert_contains "$output_file" "Downloading ${asset}..."
assert_contains "$output_file" "Verifying checksum..."
assert_contains "$output_file" "Installed:"
assert_contains "$output_file" "  ${install_dir}/hyard"
assert_contains "$output_file" "Note: ${install_dir} is not on your PATH."
assert_contains "$output_file" "Run: hyard --help"
assert_contains "$output_file" "Run: hyard plumbing orbit --help"
assert_contains "$output_file" "Run: hyard plumbing harness --help"

echo "install.sh tests passed"
