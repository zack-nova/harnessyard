#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
target_script="$script_dir/run_golangci_lint.sh"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

assert_contains() {
  file=$1
  pattern=$2

  if ! grep -Fq -- "$pattern" "$file"; then
    echo "expected output to contain: $pattern" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_not_contains() {
  file=$1
  pattern=$2

  if grep -Fq -- "$pattern" "$file"; then
    echo "expected output to not contain: $pattern" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_file_equals() {
  file=$1
  expected=$2

  actual=$(cat "$file")
  if [ "$actual" != "$expected" ]; then
    echo "expected $file to equal: $expected" >&2
    echo "actual: $actual" >&2
    exit 1
  fi
}

copy_lint_script() {
  test_root=$1

  mkdir -p "$test_root/repo/scripts"
  cp "$target_script" "$test_root/repo/scripts/run_golangci_lint.sh"
}

run_timeout_fallback_test() {
  test_root="$tmpdir/timeout"
  mkdir -p "$test_root/bin" "$test_root/repo"

  cat <<'EOF' >"$test_root/bin/golangci-lint"
#!/bin/sh
set -eu

case "${1:-}" in
  version)
    echo "golangci-lint has version 2.10.1"
    exit 0
    ;;
  config)
    if [ "${2:-}" != "verify" ]; then
      echo "unexpected config args: $*" >&2
      exit 98
    fi
    cat <<'ERR' >&2
Get "https://golangci-lint.run/jsonschema/golangci.jsonschema.json": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
ERR
    exit 1
    ;;
  run)
    printf '%s\n' "$@" >"$TEST_ROOT/run-args.txt"
    echo "lint ok"
    exit 0
    ;;
esac

echo "unexpected args: $*" >&2
exit 99
EOF
  chmod +x "$test_root/bin/golangci-lint"

  cat <<'EOF' >"$test_root/repo/go.mod"
module example.com/test

go 1.26.2
EOF

  (
    cd "$test_root/repo"
    TEST_ROOT="$test_root" PATH="$test_root/bin:$PATH" sh "$target_script"
  ) >"$test_root/output.txt" 2>&1

  assert_contains "$test_root/output.txt" "Skipping golangci-lint config verify because remote schema lookup is unavailable"
  assert_contains "$test_root/output.txt" "lint ok"
  assert_contains "$test_root/run-args.txt" "--timeout=10m"
  assert_contains "$test_root/run-args.txt" "--modules-download-mode=readonly"
  assert_contains "$test_root/run-args.txt" "./..."
}

run_go_env_passthrough_test() {
  test_root="$tmpdir/go-env"
  mkdir -p "$test_root/bin" "$test_root/repo"

  cat <<'EOF' >"$test_root/bin/golangci-lint"
#!/bin/sh
set -eu

case "${1:-}" in
  version)
    echo "golangci-lint has version 2.10.1"
    exit 0
    ;;
  config)
    exit 0
    ;;
  run)
    printf '%s' "$(pwd)" >"$TEST_ROOT/pwd.txt"
    printf '%s|%s' "${GOPROXY-<unset>}" "${GOSUMDB-<unset>}" >"$TEST_ROOT/go-env.txt"
    echo "lint ok"
    exit 0
    ;;
esac

echo "unexpected args: $*" >&2
exit 99
EOF
  chmod +x "$test_root/bin/golangci-lint"

  cat <<'EOF' >"$test_root/repo/go.mod"
module example.com/test

go 1.26.2
EOF

  (
    mkdir -p "$test_root/repo/nested"
    cd "$test_root/repo/nested"
    unset GOPROXY GOSUMDB
    TEST_ROOT="$test_root" PATH="$test_root/bin:$PATH" sh "$target_script"
  ) >"$test_root/output.txt" 2>&1

  assert_contains "$test_root/output.txt" "lint ok"
  assert_file_equals "$test_root/go-env.txt" "<unset>|<unset>"
  assert_file_equals "$test_root/pwd.txt" "$repo_root"
}

run_schema_error_test() {
  test_root="$tmpdir/schema"
  mkdir -p "$test_root/bin" "$test_root/repo"

  cat <<'EOF' >"$test_root/bin/golangci-lint"
#!/bin/sh
set -eu

case "${1:-}" in
  version)
    echo "golangci-lint has version 2.10.1"
    exit 0
    ;;
  config)
    if [ "${2:-}" != "verify" ]; then
      echo "unexpected config args: $*" >&2
      exit 98
    fi
    cat <<'ERR' >&2
jsonschema: "/linters/default" does not validate with "/properties/linters/properties/default/type": expected string or array, but got number
ERR
    exit 1
    ;;
  run)
    echo "lint should not run" >&2
    exit 91
    ;;
esac

echo "unexpected args: $*" >&2
exit 99
EOF
  chmod +x "$test_root/bin/golangci-lint"

  cat <<'EOF' >"$test_root/repo/go.mod"
module example.com/test

go 1.26.2
EOF

  set +e
  (
    cd "$test_root/repo"
    PATH="$test_root/bin:$PATH" sh "$target_script"
  ) >"$test_root/output.txt" 2>&1
  status=$?
  set -e

  if [ "$status" -eq 0 ]; then
    echo "expected schema validation failure to stop the script" >&2
    cat "$test_root/output.txt" >&2
    exit 1
  fi

  assert_contains "$test_root/output.txt" "jsonschema: \"/linters/default\" does not validate"
  assert_not_contains "$test_root/output.txt" "Skipping golangci-lint config verify because remote schema lookup is unavailable"
}

run_strict_schema_network_failure_test() {
  test_root="$tmpdir/strict-schema"
  mkdir -p "$test_root/bin" "$test_root/repo"

  cat <<'EOF' >"$test_root/bin/golangci-lint"
#!/bin/sh
set -eu

case "${1:-}" in
  version)
    echo "golangci-lint has version 2.10.1"
    exit 0
    ;;
  config)
    cat <<'ERR' >&2
Get "https://golangci-lint.run/jsonschema/golangci.jsonschema.json": dial tcp: i/o timeout
ERR
    exit 1
    ;;
  run)
    echo "lint should not run" >&2
    exit 91
    ;;
esac

echo "unexpected args: $*" >&2
exit 99
EOF
  chmod +x "$test_root/bin/golangci-lint"

  cat <<'EOF' >"$test_root/repo/go.mod"
module example.com/test

go 1.26.2
EOF

  set +e
  (
    cd "$test_root/repo"
    GOLANGCI_LINT_ALLOW_SCHEMA_FALLBACK=0 PATH="$test_root/bin:$PATH" sh "$target_script"
  ) >"$test_root/output.txt" 2>&1
  status=$?
  set -e

  if [ "$status" -eq 0 ]; then
    echo "expected strict schema verification to fail on network lookup failure" >&2
    cat "$test_root/output.txt" >&2
    exit 1
  fi

  assert_contains "$test_root/output.txt" "i/o timeout"
  assert_not_contains "$test_root/output.txt" "Skipping golangci-lint config verify because remote schema lookup is unavailable"
}

run_missing_tool_test() {
  test_root="$tmpdir/missing-tool"
  copy_lint_script "$test_root"

  cat <<'EOF' >"$test_root/repo/go.mod"
module example.com/test

go 1.26.2
EOF

  set +e
  PATH="/usr/bin:/bin" sh "$test_root/repo/scripts/run_golangci_lint.sh" >"$test_root/output.txt" 2>&1
  status=$?
  set -e

  if [ "$status" -ne 127 ]; then
    echo "expected missing golangci-lint to exit 127, got $status" >&2
    cat "$test_root/output.txt" >&2
    exit 1
  fi

  assert_contains "$test_root/output.txt" "golangci-lint not found in PATH and mise is unavailable"
}

run_no_go_mod_test() {
  test_root="$tmpdir/no-go-mod"
  copy_lint_script "$test_root"

  PATH="/usr/bin:/bin" sh "$test_root/repo/scripts/run_golangci_lint.sh" >"$test_root/output.txt" 2>&1

  assert_contains "$test_root/output.txt" "No go.mod yet; skipping golangci-lint"
}

run_timeout_fallback_test
run_go_env_passthrough_test
run_schema_error_test
run_strict_schema_network_failure_test
run_missing_tool_test
run_no_go_mod_test

echo "run_golangci_lint.sh tests passed"
