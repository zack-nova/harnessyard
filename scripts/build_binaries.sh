#!/usr/bin/env sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
output_dir="${1:-$repo_root/.dist/bin}"

case "$output_dir" in
  /*) ;;
  *) output_dir="$repo_root/$output_dir" ;;
esac

mkdir -p "$output_dir"

run_go() {
  if command -v go >/dev/null 2>&1; then
    go "$@"
    return
  fi

  if command -v mise >/dev/null 2>&1; then
    mise exec -- go "$@"
    return
  fi

  echo "go toolchain not found in PATH and mise is unavailable" >&2
  exit 127
}

version="${VERSION:-dev}"
commit="${COMMIT:-$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || printf unknown)}"
build_date="${BUILD_DATE:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}"
built_by="${BUILT_BY:-build_binaries.sh}"
buildvcs="${BUILDVCS:-auto}"
cgo_enabled="${CGO_ENABLED:-0}"
ldflags="-s -w -X github.com/zack-nova/harnessyard/cmd/hyard/cli.version=$version -X github.com/zack-nova/harnessyard/cmd/hyard/cli.commit=$commit -X github.com/zack-nova/harnessyard/cmd/hyard/cli.date=$build_date -X github.com/zack-nova/harnessyard/cmd/hyard/cli.builtBy=$built_by"

(
  cd "$repo_root"
  export CGO_ENABLED="$cgo_enabled"
  run_go build \
    -trimpath \
    -buildvcs="$buildvcs" \
    -ldflags "$ldflags" \
    -o "$output_dir/hyard" \
    ./cmd/hyard
)

printf 'built hyard: %s\n' "$output_dir/hyard"
