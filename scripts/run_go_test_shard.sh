#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
shard_dir="$repo_root/.github/go-test-shards"

if [ "$#" -ne 1 ]; then
  echo "usage: sh ./scripts/run_go_test_shard.sh <shard-name>" >&2
  exit 2
fi

shard_name=$1
shard_file="$shard_dir/$shard_name.txt"

if [ ! -f "$shard_file" ]; then
  echo "unknown Go test shard: $shard_name" >&2
  echo "available shards:" >&2
  find "$shard_dir" -type f -name '*.txt' -exec basename {} .txt \; | sort >&2
  exit 1
fi

set --

while IFS= read -r package || [ -n "$package" ]; do
  case "$package" in
    ""|\#*)
      continue
      ;;
  esac

  set -- "$@" "$package"
done <"$shard_file"

if [ "$#" -eq 0 ]; then
  echo "Go test shard is empty: ${shard_file#$repo_root/}" >&2
  exit 1
fi

cd "$repo_root"

timing_limit=${GO_TEST_TIMING_LIMIT:-10}
case "$timing_limit" in
  "" | 0 | *[!0-9]*)
    echo "GO_TEST_TIMING_LIMIT must be a positive integer, got: $timing_limit" >&2
    exit 2
    ;;
esac

timing_dir=${GO_TEST_TIMING_DIR:-"$repo_root/tmp/go-test-timing/$shard_name"}
events_file="$timing_dir/go-test-events-$shard_name.json"
summary_file="$timing_dir/go-test-timing-$shard_name.md"
status_file="$timing_dir/go-test-status-$shard_name.txt"

mkdir -p "$timing_dir"
rm -f "$events_file" "$summary_file" "$status_file"

set +e
go test -count=1 -json "$@" >"$events_file" 2>&1
test_status=$?
set -e

printf '%s\n' "$test_status" >"$status_file"

set +e
go run ./internal/citest/gotesttiming \
  -input "$events_file" \
  -shard "$shard_name" \
  -limit "$timing_limit" >"$summary_file"
summary_status=$?
set -e

if [ -f "$summary_file" ]; then
  cat "$summary_file"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
      printf '\n'
      cat "$summary_file"
    } >>"$GITHUB_STEP_SUMMARY"
  fi
fi

if [ "$test_status" -ne 0 ]; then
  echo "Go test shard failed; raw go test -json event log follows:"
  cat "$events_file"
  exit "$test_status"
fi

exit "$summary_status"
