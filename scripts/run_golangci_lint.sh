#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

is_remote_schema_network_failure() {
  output_file=$1

  if ! grep -Eqi '(https?://[^[:space:]]+|golangci-lint\.run|jsonschema/.+\.json|raw\.githubusercontent\.com/.+jsonschema)' "$output_file"; then
    return 1
  fi

  if grep -Eqi '(no such host|i/o timeout|dial tcp|context deadline exceeded|client\.timeout exceeded|tls handshake timeout|temporary failure in name resolution|connection timed out|network is unreachable|connection reset by peer)' "$output_file"; then
    return 0
  fi

  return 1
}

run_golangci_lint() {
  if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint "$@"
    return
  fi

  if command -v mise >/dev/null 2>&1; then
    mise exec -- golangci-lint "$@"
    return
  fi

  echo "golangci-lint not found in PATH and mise is unavailable" >&2
  exit 127
}

cd "$repo_root"

if [ ! -f go.mod ]; then
  echo "No go.mod yet; skipping golangci-lint"
  exit 0
fi

run_golangci_lint version

verify_output_file="$(mktemp)"
cleanup() {
  rm -f "$verify_output_file"
}
trap cleanup EXIT INT TERM

if run_golangci_lint config verify >"$verify_output_file" 2>&1; then
  cat "$verify_output_file"
else
  verify_status=$?
  cat "$verify_output_file"
  if [ "${GOLANGCI_LINT_ALLOW_SCHEMA_FALLBACK:-1}" = "1" ] && is_remote_schema_network_failure "$verify_output_file"; then
    echo "Skipping golangci-lint config verify because remote schema lookup is unavailable"
  else
    exit "$verify_status"
  fi
fi

run_golangci_lint run \
  --timeout="${GOLANGCI_LINT_TIMEOUT:-10m}" \
  --modules-download-mode="${GOLANGCI_LINT_MODULES_DOWNLOAD_MODE:-readonly}" \
  ./...
