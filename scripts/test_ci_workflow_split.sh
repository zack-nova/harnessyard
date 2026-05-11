#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

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

assert_not_contains() {
  file=$1
  unexpected=$2

  if grep -Fq "$unexpected" "$file"; then
    echo "expected ${file#$repo_root/} to not contain: $unexpected" >&2
    cat "$file" >&2
    exit 1
  fi
}

workflow="$repo_root/.github/workflows/ci.yml"
mise_config="$repo_root/mise.toml"
shell_entrypoint="$repo_root/scripts/test_shell_validation.sh"
shard_test="$repo_root/scripts/test_go_test_shards.sh"
tool_version_reader="$repo_root/scripts/read_mise_tool_version.sh"

assert_file_exists "$workflow"
assert_file_exists "$mise_config"
assert_file_exists "$shell_entrypoint"
assert_file_exists "$shard_test"
assert_file_exists "$tool_version_reader"

assert_contains "$workflow" "  lint:"
assert_contains "$workflow" "  go-tests:"
assert_contains "$workflow" "  govulncheck:"
assert_contains "$workflow" "  shell-validation:"
assert_contains "$workflow" 'name: Go tests (${{ matrix.shard }})'
assert_contains "$workflow" 'sh ./scripts/run_go_test_shard.sh "${{ matrix.shard }}"'
assert_contains "$workflow" "./scripts/read_mise_tool_version.sh golangci-lint"
assert_contains "$workflow" '${{ steps.golangci_lint_version.outputs.version }}'
assert_contains "$workflow" "sh ./scripts/test_shell_validation.sh"

assert_not_contains "$workflow" "GOLANGCI_LINT_VERSION:"
assert_not_contains "$workflow" "Run Go tests"
assert_not_contains "$workflow" "go test -count=1 ./..."

assert_contains "$mise_config" 'run = "sh ./scripts/test_shell_validation.sh"'
assert_contains "$shell_entrypoint" "sh ./scripts/test_validation_parity.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_go_test_shards.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_ci_workflow_split.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_run_golangci_lint.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_build_binaries.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_install_script.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_run_view_guidance_docs.sh"
assert_contains "$shell_entrypoint" "sh ./scripts/test_release_surface_hyard.sh"

echo "CI workflow split tests passed"
