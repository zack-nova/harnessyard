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

assert_contains_line() {
  file=$1
  expected=$2

  if ! grep -Fxq "$expected" "$file"; then
    echo "expected ${file#$repo_root/} to contain line: $expected" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_occurs_before() {
  file=$1
  first=$2
  second=$3

  first_line=$(grep -Fn "$first" "$file" | head -n 1 | cut -d: -f1 || true)
  second_line=$(grep -Fn "$second" "$file" | head -n 1 | cut -d: -f1 || true)

  if [ -z "$first_line" ]; then
    echo "expected ${file#$repo_root/} to contain before-check text: $first" >&2
    cat "$file" >&2
    exit 1
  fi
  if [ -z "$second_line" ]; then
    echo "expected ${file#$repo_root/} to contain after-check text: $second" >&2
    cat "$file" >&2
    exit 1
  fi
  if [ "$first_line" -ge "$second_line" ]; then
    echo "expected ${file#$repo_root/} to place '$first' before '$second'" >&2
    cat "$file" >&2
    exit 1
  fi
}

quickstart_doc="$repo_root/docs/quickstart.md"
installation_doc="$repo_root/docs/installation.md"
release_surface_doc="$repo_root/docs/reference/release-surface.md"
maintainer_release_doc="$repo_root/docs/maintainers/release.md"
contributor_testing_doc="$repo_root/docs/contributing/testing.md"
maintainer_testing_doc="$repo_root/docs/maintainers/testing-strategy.md"
install_script="$repo_root/install.sh"
goreleaser_config="$repo_root/.goreleaser.yaml"

for doc in \
  "$quickstart_doc" \
  "$installation_doc" \
  "$release_surface_doc" \
  "$maintainer_release_doc" \
  "$contributor_testing_doc" \
  "$maintainer_testing_doc" \
  "$install_script" \
  "$goreleaser_config"
do
  assert_file_exists "$doc"
done

assert_contains "$quickstart_doc" "# Harness Yard Quickstart"
assert_contains "$quickstart_doc" "<!-- quickstart-smoke:start -->"
assert_contains "$quickstart_doc" "<!-- quickstart-smoke:end -->"
assert_contains "$quickstart_doc" "sh ./scripts/test_release_surface_hyard.sh"
assert_contains "$quickstart_doc" "When runtime fixtures are added"
assert_contains "$quickstart_doc" "brew install hyard"
assert_contains "$quickstart_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_contains "$quickstart_doc" "hyard --version"
assert_contains "$quickstart_doc" "## Runtime User Path"
assert_contains "$quickstart_doc" "hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab"
assert_contains "$quickstart_doc" "hyard start --with codex"
assert_contains "$quickstart_doc" "### Lower-Level Agent Handoff"
assert_contains "$quickstart_doc" "hyard agent use codex"
assert_contains "$quickstart_doc" "hyard agent apply --project-only --yes"
assert_contains "$quickstart_doc" "Run View is the default runtime-user presentation"
assert_contains "$quickstart_doc" "hyard view status"
assert_contains "$quickstart_doc" "hyard view run --check"
assert_contains "$quickstart_doc" "hyard current"
assert_contains "$quickstart_doc" "hyard enter docs"
assert_contains "$quickstart_doc" "hyard create runtime demo-repo"
assert_contains "$quickstart_doc" "### Existing Repository Assembly"
assert_contains "$quickstart_doc" "hyard init runtime"
assert_contains "$quickstart_doc" "hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab"
assert_contains "$quickstart_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml"
assert_contains "$quickstart_doc" "hyard orbit list"
assert_contains "$quickstart_doc" "hyard publish harness workspace"
assert_contains "$quickstart_doc" "hyard assign orbit <orbit-package>"
assert_contains "$quickstart_doc" "hyard unassign orbit <orbit-package>"
assert_contains "$quickstart_doc" "git status --short"
assert_contains "$quickstart_doc" "git add ."
assert_contains "$quickstart_doc" 'git commit -m "Optimize frontend lab harness"'
assert_contains "$quickstart_doc" "hyard install <template-source>"
assert_contains "$quickstart_doc" "hyard uninstall orbit <orbit-package>"
assert_contains "$quickstart_doc" "hyard uninstall harness frontend-lab"
assert_contains "$quickstart_doc" "hyard uninstall orbit docs"
assert_contains "$quickstart_doc" "## Author Path"
assert_contains "$quickstart_doc" "hyard view author"
assert_contains "$quickstart_doc" "hyard guide render --orbit docs --target all"
assert_contains "$quickstart_doc" "hyard guide save --orbit docs --target all"
assert_contains "$quickstart_doc" "hyard guide writeback --orbit docs --target all"
assert_contains "$quickstart_doc" "hyard orbit content apply docs --check --json"
assert_contains "$quickstart_doc" "hyard publish orbit docs --json"
assert_contains "$quickstart_doc" "hyard plumbing orbit branch list --json"
assert_contains_line "$quickstart_doc" "hyard bootstrap complete --check --json"
assert_contains_line "$quickstart_doc" "hyard bootstrap complete --yes"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup codex"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup --remove"
assert_contains_line "$quickstart_doc" "hyard bootstrap reopen"
assert_contains_line "$quickstart_doc" "hyard bootstrap reopen --restore-surface"
assert_occurs_before "$quickstart_doc" 'git commit -m "Optimize frontend lab harness"' "hyard publish harness workspace"
assert_not_contains "$quickstart_doc" "hyard assign orbit <orbit-id> --harness <harness-id>"
assert_not_contains "$quickstart_doc" "hyard plumbing orbit list"
assert_not_contains "$quickstart_doc" "hyard plumbing harness template publish"
assert_not_contains "$quickstart_doc" "hyard remove "
assert_not_contains "$quickstart_doc" "# Orbit / Harness Quickstart"
assert_not_contains "$quickstart_doc" "## Worker Path"
assert_not_contains "$quickstart_doc" "## Runtime Author Path"
assert_not_contains "$quickstart_doc" "## Orbit Author Path"
assert_not_contains "$quickstart_doc" "## Harness Author Path"
assert_not_contains "$quickstart_doc" "Install Or Build"
assert_not_contains "$quickstart_doc" "scripts/build_binaries.sh"
assert_not_contains "$quickstart_doc" 'export HYARD_BIN="$ORBIT_BIN_DIR/hyard"'
assert_not_contains "$quickstart_doc" 'export ORBIT_BIN="$ORBIT_BIN_DIR/orbit"'
assert_not_contains "$quickstart_doc" 'export HARNESS_BIN="$ORBIT_BIN_DIR/harness"'
assert_not_contains "$quickstart_doc" '"$ORBIT_BIN" branch list --json'
assert_not_contains "$quickstart_doc" '"$HARNESS_BIN" install "$TEMPLATE_REPO"'

assert_contains "$installation_doc" "Harness Yard is released as a single public CLI binary"
assert_contains "$installation_doc" "brew install hyard"
assert_contains "$installation_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_contains "$installation_doc" "hyard plumbing orbit --help"
assert_contains "$installation_doc" "hyard plumbing harness --help"
assert_not_contains "$installation_doc" "harness-yard"

assert_contains "$release_surface_doc" "Harness Yard CLI (hyard)"
assert_contains "$release_surface_doc" 'Formal release assets must distribute `hyard` only.'
assert_contains "$release_surface_doc" "brew tap zack-nova/tap"
assert_contains "$release_surface_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_contains "$release_surface_doc" "hyard install <template-source>"
assert_contains "$release_surface_doc" "hyard uninstall orbit <orbit-package>"
assert_contains "$release_surface_doc" "hyard uninstall harness <harness-package>"
assert_contains "$release_surface_doc" "hyard orbit member remove"
assert_contains "$release_surface_doc" "## Harness Start Demo Paths"
assert_contains "$release_surface_doc" "hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab"
assert_contains "$release_surface_doc" "hyard start --with codex"
assert_contains "$release_surface_doc" "hyard init runtime"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml"
assert_contains "$release_surface_doc" "hyard agent use codex"
assert_contains "$release_surface_doc" "hyard agent apply --project-only --yes"
assert_contains "$release_surface_doc" 'git commit -m "Optimize frontend lab harness"'
assert_contains "$release_surface_doc" "hyard publish harness workspace"
assert_occurs_before "$release_surface_doc" 'git commit -m "Optimize frontend lab harness"' "hyard publish harness workspace"
assert_contains "$release_surface_doc" "Run View is the recommended runtime-user view"
assert_contains "$release_surface_doc" 'Run View publication should use `hyard publish harness <harness-package>`'
assert_contains "$release_surface_doc" "Author View is the authored-truth view"
assert_contains "$release_surface_doc" "Orbit Package publication remains available for authoring and compatibility"
assert_contains "$release_surface_doc" 'Main `hyard --help` output must stay stable across Runtime View Selection'
assert_contains "$release_surface_doc" "<!-- orbit:begin workflow=\"docs\" -->"
assert_contains "$release_surface_doc" "<!-- orbit:end workflow=\"docs\" -->"
assert_contains "$release_surface_doc" "<!-- harness:begin workflow=\"workspace\" -->"
assert_contains "$release_surface_doc" "<!-- harness:end workflow=\"workspace\" -->"
assert_contains "$release_surface_doc" "Root guidance marker workflow language does not rename OrbitSpec"
assert_contains "$release_surface_doc" "storage paths, member hints, package identity, or template branch contracts."
assert_contains "$release_surface_doc" 'hyard_${VERSION}_${GOOS}_${GOARCH}.tar.gz'
assert_contains "$release_surface_doc" "zack-nova/homebrew-tap/Formula/hyard.rb"
assert_contains "$release_surface_doc" "checksums.txt"
assert_not_contains "$release_surface_doc" "v0.4.0"
assert_not_contains "$release_surface_doc" "hyard_0.4.0_linux_amd64.tar.gz"
assert_not_contains "$release_surface_doc" 'install `hyard`, `orbit`, and `harness`'
assert_not_contains "$release_surface_doc" "orbit_id=\""
assert_not_contains "$release_surface_doc" "orbit:block"
assert_not_contains "$release_surface_doc" "harness:block"
assert_not_contains "$release_surface_doc" "hyard remove "
assert_not_contains "$release_surface_doc" "harness-yard"

assert_contains "$maintainer_release_doc" "goreleaser check"
assert_contains "$maintainer_release_doc" "goreleaser release --snapshot --clean"
assert_contains "$maintainer_release_doc" "VERSION=vX.Y.Z"
assert_contains "$maintainer_release_doc" "../reference/release-surface.md"
assert_contains "$maintainer_release_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_not_contains "$maintainer_release_doc" "git tag -a v0.4.0"
assert_not_contains "$maintainer_release_doc" "hyard_0.4.0"
assert_not_contains "$maintainer_release_doc" "harness-yard"

assert_contains "$contributor_testing_doc" "Until that task exists in this repository"
assert_contains "$maintainer_testing_doc" "## 2. MVP Test Pyramid"
assert_contains "$maintainer_testing_doc" "## 3. Minimum Coverage Matrix"
assert_contains "$maintainer_testing_doc" "## 4. Test Harness Rules"

assert_contains "$install_script" "PROJECT=\"hyard\""
assert_contains "$install_script" "REPO=\"\${REPO:-harnessyard}\""
assert_contains "$install_script" "asset_version=\"\${tag#v}\""
assert_contains "$install_script" "Run: hyard --help"
assert_contains "$install_script" "Run: hyard plumbing orbit --help"
assert_contains "$install_script" "Run: hyard plumbing harness --help"
assert_not_contains "$install_script" "BINS=(hyard orbit harness)"
assert_not_contains "$install_script" "Run: orbit --help"
assert_not_contains "$install_script" "Run: harness --help"

assert_contains "$goreleaser_config" "project_name: hyard"
assert_contains "$goreleaser_config" "name: harnessyard"
assert_contains "$goreleaser_config" "  - id: hyard"
assert_contains "$goreleaser_config" "    binary: hyard"
assert_contains "$goreleaser_config" 'name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"'
assert_contains "$goreleaser_config" "      - hyard"
assert_not_contains "$goreleaser_config" "  - id: orbit"
assert_not_contains "$goreleaser_config" "  - id: harness"
assert_not_contains "$goreleaser_config" "      - orbit"
assert_not_contains "$goreleaser_config" "      - harness"

if command -v goreleaser >/dev/null 2>&1; then
  (
    cd "$repo_root"
    goreleaser check
  )
else
  echo "goreleaser not found; skipping goreleaser config check"
fi

echo "release surface hyard tests passed"
