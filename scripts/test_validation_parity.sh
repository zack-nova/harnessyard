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

assert_not_contains() {
  file=$1
  unexpected=$2

  if grep -Fq "$unexpected" "$file"; then
    echo "expected ${file#$repo_root/} to not contain: $unexpected" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_text_contains() {
  text=$1
  expected=$2

  if ! printf '%s\n' "$text" | grep -Fq "$expected"; then
    echo "expected text to contain: $expected" >&2
    printf '%s\n' "$text" >&2
    exit 1
  fi
}

assert_text_not_contains() {
  text=$1
  unexpected=$2

  if printf '%s\n' "$text" | grep -Fq "$unexpected"; then
    echo "expected text to not contain: $unexpected" >&2
    printf '%s\n' "$text" >&2
    exit 1
  fi
}

assert_pinned_semver() {
  value=$1

  if ! printf '%s\n' "$value" | grep -Eq '^[0-9]+[.][0-9]+[.][0-9]+$'; then
    echo "expected pinned semver, got: $value" >&2
    exit 1
  fi
}

toml_section() {
  section=$1
  file=$2

  awk -v section="$section" '
    $0 == section {
      in_section = 1
      print
      next
    }
    in_section && /^\[/ {
      exit
    }
    in_section {
      print
    }
  ' "$file"
}

ci_workflow="$repo_root/.github/workflows/ci.yml"
mise_config="$repo_root/mise.toml"
renovate_config="$repo_root/renovate.json"
shell_validation="$repo_root/scripts/test_shell_validation.sh"
tool_version_reader="$repo_root/scripts/read_mise_tool_version.sh"
contributor_testing="$repo_root/docs/contributing/testing.md"
maintainer_testing="$repo_root/docs/maintainers/testing-strategy.md"

assert_file_exists "$ci_workflow"
assert_file_exists "$mise_config"
assert_file_exists "$renovate_config"
assert_file_exists "$shell_validation"
assert_file_exists "$tool_version_reader"
assert_file_exists "$contributor_testing"
assert_file_exists "$maintainer_testing"

golangci_lint_version=$(sh "$tool_version_reader" golangci-lint)
assert_pinned_semver "$golangci_lint_version"

assert_not_contains "$ci_workflow" "GOLANGCI_LINT_VERSION:"
assert_contains "$ci_workflow" "./scripts/read_mise_tool_version.sh golangci-lint"
assert_contains "$ci_workflow" '${{ steps.golangci_lint_version.outputs.version }}'
assert_contains "$ci_workflow" "sh ./scripts/test_shell_validation.sh"

assert_contains "$mise_config" "sh ./scripts/test_shell_validation.sh"
assert_contains "$shell_validation" "sh ./scripts/test_run_view_guidance_docs.sh"

assert_contains "$renovate_config" '"mise"'
assert_contains "$renovate_config" '"golangci-lint"'

check_task=$(toml_section "[tasks.check]" "$mise_config")
ci_task=$(toml_section "[tasks.ci]" "$mise_config")

assert_text_contains "$check_task" "description = \"Run fast local feedback checks with cached Go tests\""
assert_text_contains "$check_task" "{ task = \"lint\" }"
assert_text_contains "$check_task" "{ task = \"test:go\" }"
assert_text_not_contains "$check_task" "test:go:ci"

assert_text_contains "$ci_task" "description = \"Run full strict CI-local validation\""
assert_text_contains "$ci_task" "{ task = \"lint\" }"
assert_text_contains "$ci_task" "{ task = \"test:go:ci\" }"
assert_text_contains "$ci_task" "{ task = \"vuln\" }"
assert_text_contains "$ci_task" "{ task = \"test:scripts\" }"

assert_contains "$contributor_testing" "mise run fix"
assert_contains "$contributor_testing" "mise run check"
assert_contains "$contributor_testing" "mise run ci"
assert_contains "$contributor_testing" "fast local feedback loop"
assert_contains "$contributor_testing" "cached Go tests"
assert_contains "$contributor_testing" "full strict validation"
assert_contains "$contributor_testing" "sh ./scripts/test_release_surface_hyard.sh"

assert_contains "$maintainer_testing" "mise run fix"
assert_contains "$maintainer_testing" "mise run check"
assert_contains "$maintainer_testing" "mise run ci"
assert_contains "$maintainer_testing" "fast local feedback loop"
assert_contains "$maintainer_testing" "cached Go tests"
assert_contains "$maintainer_testing" "full strict validation"
