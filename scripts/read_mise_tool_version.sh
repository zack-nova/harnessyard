#!/bin/sh

set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <tool-name>" >&2
  exit 64
fi

tool_name=$1
case "$tool_name" in
  "" | *[!A-Za-z0-9._:-]*)
    echo "invalid mise tool name: $tool_name" >&2
    exit 64
    ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
mise_config="$repo_root/mise.toml"

if [ ! -f "$mise_config" ]; then
  echo "missing mise config: mise.toml" >&2
  exit 1
fi

version=$(
  awk -v target="$tool_name" '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }

    {
      line = $0
      sub(/[[:space:]]+#.*$/, "", line)
      line = trim(line)
      if (line == "") {
        next
      }
      if (line ~ /^\[/) {
        in_tools = (line == "[tools]")
        next
      }
      if (!in_tools) {
        next
      }

      equals = index(line, "=")
      if (equals == 0) {
        next
      }

      key = trim(substr(line, 1, equals - 1))
      value = trim(substr(line, equals + 1))
      if (key ~ /^".*"$/) {
        key = substr(key, 2, length(key) - 2)
      }
      if (value ~ /^".*"$/) {
        value = substr(value, 2, length(value) - 2)
      }

      if (key == target) {
        print value
        found = 1
        exit
      }
    }

    END {
      if (!found) {
        exit 1
      }
    }
  ' "$mise_config"
) || {
  echo "missing mise tool version for $tool_name" >&2
  exit 1
}

version=${version#v}
if [ "$version" = "latest" ] || ! printf '%s\n' "$version" | grep -Eq '^[0-9]+[.][0-9]+[.][0-9]+$'; then
  echo "mise tool $tool_name must use a pinned semver version, got: $version" >&2
  exit 1
fi

printf '%s\n' "$version"