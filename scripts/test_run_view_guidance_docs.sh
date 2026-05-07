#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
if repo_root=$(git -C "$script_dir/.." rev-parse --show-toplevel 2>/dev/null); then
  :
else
  repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
fi

assert_file_exists() {
  file=$1

  if [ ! -f "$file" ]; then
    echo "missing required file: ${file#$repo_root/}" >&2
    exit 1
  fi
}

assert_contains() {
  file=$1
  expected=$2

  if ! grep -Fq "$expected" "$file"; then
    echo "expected ${file#$repo_root/} to contain: $expected" >&2
    cat "$file" >&2
    exit 1
  fi
}

context_doc="$repo_root/CONTEXT.md"
adr_doc="$repo_root/docs/adr/0002-marked-guidance-resolution-before-run-view-cleanup.md"
architecture_doc="$repo_root/docs/maintainers/current-architecture.md"

assert_file_exists "$context_doc"
assert_file_exists "$adr_doc"
assert_file_exists "$architecture_doc"

assert_contains "$context_doc" "**Runtime View**:"
assert_contains "$context_doc" "**Run View**:"
assert_contains "$context_doc" "**Author View**:"
assert_contains "$context_doc" "**Run View Root Guidance**:"
assert_contains "$context_doc" "**Run View Cleanup**:"
assert_contains "$context_doc" "**Marked Guidance Resolution**:"
assert_contains "$context_doc" "**Runtime Check**:"
assert_contains "$context_doc" "Existing markerless **Run View Root Guidance** is presentation text"
assert_contains "$context_doc" "Markerless **Run View Root Guidance** must not create authored-truth drift"
assert_contains "$context_doc" "Standalone **Run View Guidance Output** outside **Package Installation** requires explicit user confirmation"
assert_contains "$context_doc" "**Package Installation** in **Run View** outputs guidance incrementally"

assert_contains "$adr_doc" "# Marked guidance resolution before Run View cleanup"
assert_contains "$adr_doc" "Run View cleanup removes root guidance markers"
assert_contains "$adr_doc" "interactive cleanup must ask users whether to save the current block to authored truth, re-render authored truth before cleanup, or strip markers in place"
assert_contains "$adr_doc" "Standalone Run View root guidance output outside package installation is explicit"
assert_contains "$adr_doc" "Installation output is incremental in Run View"

assert_contains "$architecture_doc" "Run View Root Guidance"
assert_contains "$architecture_doc" "Run View Cleanup"
assert_contains "$architecture_doc" "Marked Guidance Resolution"
assert_contains "$architecture_doc" "Package installation may write incremental Run View Root Guidance"
assert_contains "$architecture_doc" "Standalone Run View Guidance Output is explicit"
assert_contains "$architecture_doc" "Markerless Run View Root Guidance is presentation text"
