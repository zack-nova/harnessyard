#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
shard_dir="$repo_root/.github/go-test-shards"

if [ ! -d "$shard_dir" ]; then
  echo "missing Go test shard directory: ${shard_dir#$repo_root/}" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

shards_file="$tmpdir/shards.txt"
expected_file="$tmpdir/expected.txt"
actual_file="$tmpdir/actual.txt"
actual_sorted_file="$tmpdir/actual-sorted.txt"
actual_unique_file="$tmpdir/actual-unique.txt"
duplicates_file="$tmpdir/duplicates.txt"
membership_file="$tmpdir/membership.txt"
required_shards_file="$tmpdir/required-shards.txt"

find "$shard_dir" -type f -name '*.txt' | sort >"$shards_file"

if [ ! -s "$shards_file" ]; then
  echo "no Go test shard files found in ${shard_dir#$repo_root/}" >&2
  exit 1
fi

: >"$actual_file"
: >"$membership_file"

cd "$repo_root"

while IFS= read -r shard_file; do
  shard_name=${shard_file##*/}
  shard_name=${shard_name%.txt}
  package_count=0

  while IFS= read -r package || [ -n "$package" ]; do
    case "$package" in
      ""|\#*)
        continue
        ;;
    esac

    case "$package" in
      ./*)
        ;;
      *)
        echo "Go test shard ${shard_file#$repo_root/} must use repo-relative package paths, got: $package" >&2
        exit 1
        ;;
    esac

    package_import_path=$(go list "$package")
    printf '%s\n' "$package_import_path" >>"$actual_file"
    printf '%s %s\n' "$package_import_path" "$shard_name" >>"$membership_file"
    package_count=$((package_count + 1))
  done <"$shard_file"

  if [ "$package_count" -eq 0 ]; then
    echo "Go test shard ${shard_file#$repo_root/} is empty" >&2
    exit 1
  fi
done <"$shards_file"

go list ./... | sort >"$expected_file"
sort "$actual_file" >"$actual_sorted_file"
uniq -d "$actual_sorted_file" >"$duplicates_file"

if [ -s "$duplicates_file" ]; then
  echo "Go test shard package lists contain duplicate packages:" >&2
  cat "$duplicates_file" >&2
  exit 1
fi

sort -u "$actual_file" >"$actual_unique_file"

if ! diff -u "$expected_file" "$actual_unique_file"; then
  echo "Go test shard package coverage does not match go list ./..." >&2
  exit 1
fi

: >"$required_shards_file"

for package in \
  ./cmd/hyard/cli \
  ./cmd/harness/cli \
  ./cmd/orbit/cli \
  ./cmd/orbit/cli/template \
  ./cmd/orbit/cli/harness
do
  package_import_path=$(go list "$package")
  matched_shards=$(awk -v package="$package_import_path" '$1 == package { print $2 }' "$membership_file")
  matched_count=$(printf '%s\n' "$matched_shards" | sed '/^$/d' | wc -l | tr -d ' ')

  if [ "$matched_count" -ne 1 ]; then
    echo "expected $package to appear in exactly one Go test shard, found $matched_count" >&2
    exit 1
  fi

  matched_shard=$(printf '%s\n' "$matched_shards" | sed '/^$/d' | head -n 1)
  if grep -Fxq "$matched_shard" "$required_shards_file"; then
    echo "expected slow package $package to have a distinct shard, but $matched_shard is reused" >&2
    exit 1
  fi

  printf '%s\n' "$matched_shard" >>"$required_shards_file"
done

echo "Go test shard coverage tests passed"
