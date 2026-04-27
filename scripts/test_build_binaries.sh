#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
target_script="$script_dir/build_binaries.sh"

tmpdir="$(mktemp -d)"
rel_dir=".dist-test-build-binaries-$$"
cleanup() {
  rm -rf "$tmpdir"
  rm -rf "$repo_root/$rel_dir"
}
trap cleanup EXIT INT TERM

assert_file_exists() {
  path=$1

  if [ ! -f "$path" ]; then
    echo "expected file to exist: $path" >&2
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

assert_contains() {
  file=$1
  pattern=$2

  if ! grep -Fq "$pattern" "$file"; then
    echo "expected output to contain: $pattern" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_not_contains() {
  file=$1
  pattern=$2

  if grep -Fq "$pattern" "$file"; then
    echo "expected output to not contain: $pattern" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_file_missing() {
  path=$1

  if [ -e "$path" ]; then
    echo "expected file to be absent: $path" >&2
    exit 1
  fi
}

output_dir="$tmpdir/bin"
VERSION="script-test" \
COMMIT="testcommit" \
BUILD_DATE="2026-04-27T00:00:00Z" \
BUILT_BY="test_build_binaries.sh" \
sh "$target_script" "$output_dir" >"$tmpdir/build.txt" 2>&1

assert_file_exists "$output_dir/hyard"
assert_file_missing "$output_dir/orbit"
assert_file_missing "$output_dir/harness"
assert_executable "$output_dir/hyard"
assert_contains "$tmpdir/build.txt" "built hyard:"
assert_not_contains "$tmpdir/build.txt" "built orbit:"
assert_not_contains "$tmpdir/build.txt" "built harness:"

(
  cd "$tmpdir"
  "$output_dir/hyard" --help >"$tmpdir/hyard-help.txt" 2>&1
  "$output_dir/hyard" --version >"$tmpdir/hyard-version.txt" 2>&1
  "$output_dir/hyard" plumbing orbit --help >"$tmpdir/plumbing-orbit-help.txt" 2>&1
  "$output_dir/hyard" plumbing harness --help >"$tmpdir/plumbing-harness-help.txt" 2>&1
  "$output_dir/hyard" completion bash >"$tmpdir/hyard-completion.txt" 2>&1
)

assert_contains "$tmpdir/hyard-help.txt" "Harness Yard CLI (hyard)"
assert_contains "$tmpdir/hyard-help.txt" "guide"
assert_contains "$tmpdir/hyard-version.txt" "hyard script-test"
assert_contains "$tmpdir/hyard-version.txt" "commit: testcommit"
assert_contains "$tmpdir/hyard-version.txt" "date: 2026-04-27T00:00:00Z"
assert_contains "$tmpdir/hyard-version.txt" "built by: test_build_binaries.sh"
assert_contains "$tmpdir/plumbing-orbit-help.txt" "orbit"
assert_contains "$tmpdir/plumbing-orbit-help.txt" "template"
assert_contains "$tmpdir/plumbing-harness-help.txt" "harness"
assert_contains "$tmpdir/plumbing-harness-help.txt" "install"
assert_contains "$tmpdir/hyard-completion.txt" "hyard"

(
  cd "$tmpdir"
  sh "$target_script" "$rel_dir" >"$tmpdir/build-relative.txt" 2>&1
)

assert_file_exists "$repo_root/$rel_dir/hyard"
assert_file_missing "$tmpdir/$rel_dir/hyard"
assert_contains "$tmpdir/build-relative.txt" "built hyard: $repo_root/$rel_dir/hyard"

echo "build_binaries.sh tests passed"
