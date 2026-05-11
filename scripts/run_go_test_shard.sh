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

go test -count=1 "$@"
