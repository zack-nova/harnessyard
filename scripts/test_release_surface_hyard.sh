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

  if ! grep -Fq -- "$expected" "$file"; then
    echo "expected ${file#$repo_root/} to contain: $expected" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_not_contains() {
  file=$1
  unexpected=$2

  if grep -Fq -- "$unexpected" "$file"; then
    echo "expected ${file#$repo_root/} to not contain: $unexpected" >&2
    cat "$file" >&2
    exit 1
  fi
}

assert_contains_line() {
  file=$1
  expected=$2

  if ! grep -Fxq -- "$expected" "$file"; then
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
concepts_doc="$repo_root/docs/concepts.md"
configuration_doc="$repo_root/docs/reference/configuration.md"
content_workflows_doc="$repo_root/docs/guides/content-and-workflows.md"
harness_authoring_doc="$repo_root/docs/guides/harness-authoring.md"
orbit_authoring_doc="$repo_root/docs/guides/orbit-authoring.md"
release_surface_doc="$repo_root/docs/reference/release-surface.md"
maintainer_release_doc="$repo_root/docs/maintainers/release.md"
contributor_testing_doc="$repo_root/docs/contributing/testing.md"
maintainer_testing_doc="$repo_root/docs/maintainers/testing-strategy.md"
install_script="$repo_root/install.sh"
goreleaser_config="$repo_root/.goreleaser.yaml"
hyard_install_cmd="$repo_root/cmd/hyard/cli/install.go"
hyard_registry_cmd="$repo_root/cmd/hyard/cli/registry.go"
hyard_vars_cmd="$repo_root/cmd/hyard/cli/vars.go"
harness_install_cmd="$repo_root/cmd/harness/cli/commands/install.go"

for doc in \
  "$quickstart_doc" \
  "$installation_doc" \
  "$concepts_doc" \
  "$configuration_doc" \
  "$content_workflows_doc" \
  "$harness_authoring_doc" \
  "$orbit_authoring_doc" \
  "$release_surface_doc" \
  "$maintainer_release_doc" \
  "$contributor_testing_doc" \
  "$maintainer_testing_doc" \
  "$install_script" \
  "$goreleaser_config" \
  "$hyard_install_cmd" \
  "$hyard_registry_cmd" \
  "$hyard_vars_cmd" \
  "$harness_install_cmd"
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
assert_contains "$quickstart_doc" "Run View is the default runtime-user presentation"
assert_contains "$quickstart_doc" "hyard view status"
assert_contains "$quickstart_doc" "hyard view run --check"
assert_contains "$quickstart_doc" "hyard current"
assert_contains "$quickstart_doc" "hyard enter docs"
assert_contains "$quickstart_doc" "hyard create runtime demo-repo"
assert_contains "$quickstart_doc" "hyard audit --json"
assert_contains "$quickstart_doc" 'Audit reports one of four statuses: `pass`, `warn`, `fail`, or'
assert_contains "$quickstart_doc" '`not_hyard_revision` exit non-zero'
assert_contains "$quickstart_doc" "Audit is scoped to the current Git worktree"
assert_contains "$quickstart_doc" "### Existing Repository Assembly"
assert_contains "$quickstart_doc" "hyard init runtime"
assert_contains "$quickstart_doc" "Package Handle Coordinates are the ordinary install path"
assert_contains "$quickstart_doc" "hyard install acme/frontend-lab"
assert_contains "$quickstart_doc" "hyard install acme/docs"
assert_contains "$quickstart_doc" "hyard install acme/docs@0.1.0"
assert_contains "$quickstart_doc" "hyard install docs"
assert_contains "$quickstart_doc" "### Runtime Bindings"
assert_contains "$quickstart_doc" "hyard vars init acme/docs --out .harness/vars.yaml"
assert_contains "$quickstart_doc" "schema_version: 2"
assert_contains "$quickstart_doc" "{{ vars.project_name }}"
assert_contains "$quickstart_doc" "Runtime Bindings"
assert_contains "$quickstart_doc" "Package Variables"
assert_contains "$quickstart_doc" 'Package Handle Coordinates are case-insensitive'
assert_contains "$quickstart_doc" 'Use `namespace/name[@version-or-tag]`, not npm-style `@namespace/name`.'
assert_contains "$quickstart_doc" 'Bare handles such as `docs` are curated aliases'
assert_contains "$quickstart_doc" '`latest` is an explicit registry dist-tag'
assert_contains "$quickstart_doc" 'If fresh registry data cannot be fetched'
assert_contains "$quickstart_doc" '`HYARD_CACHE_DIR`'
assert_contains "$quickstart_doc" "Explicit Git locators remain available as an advanced escape hatch"
assert_contains "$quickstart_doc" "docs/maintainers/package-registry-source-contract.md"
assert_contains "$quickstart_doc" "Each Run View Orbit Package install outputs its package guidance incrementally"
assert_contains "$quickstart_doc" "Framework Activation is the separate Agent Framework side-effect step"
assert_contains "$quickstart_doc" "hyard agent plan"
assert_contains "$quickstart_doc" "hyard agent apply --yes"
assert_contains "$quickstart_doc" "hyard agent check --json"
assert_contains "$quickstart_doc" 'It does not compose root `AGENTS.md`, `HUMANS.md`, or'
assert_contains "$quickstart_doc" "Run View Root Guidance output remains owned by package"
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
assert_contains "$quickstart_doc" "Uninstall behaves like a package manager operation"
assert_contains "$quickstart_doc" "Deleted install records and removed hosted OrbitSpecs are not treated as"
assert_contains_line "$quickstart_doc" "hyard bootstrap complete --check --json"
assert_contains_line "$quickstart_doc" "hyard bootstrap complete --yes"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup codex"
assert_contains_line "$quickstart_doc" "hyard bootstrap setup --remove"
assert_contains_line "$quickstart_doc" "hyard bootstrap reopen"
assert_contains_line "$quickstart_doc" "hyard bootstrap reopen --restore-surface"
assert_occurs_before "$quickstart_doc" 'git commit -m "Optimize frontend lab harness"' "hyard publish harness workspace"
assert_contains "$quickstart_doc" "## Authoring Next Steps"
assert_contains "$quickstart_doc" "[Harness Authoring](./guides/harness-authoring.md)"
assert_contains "$quickstart_doc" "[Orbit Authoring](./guides/orbit-authoring.md)"
assert_not_contains "$quickstart_doc" "hyard assign orbit <orbit-id> --harness <harness-id>"
assert_not_contains "$quickstart_doc" "hyard plumbing orbit list"
assert_not_contains "$quickstart_doc" "hyard plumbing harness template publish"
assert_not_contains "$quickstart_doc" "hyard plumbing"
assert_not_contains "$quickstart_doc" "Lower-Level Agent Handoff"
assert_not_contains "$quickstart_doc" "hyard remove "
assert_not_contains "$quickstart_doc" "## Author Path"
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
assert_not_contains "$quickstart_doc" ".orbit/vars.yaml"
assert_not_contains "$quickstart_doc" '$project_name'
assert_not_contains "$quickstart_doc" "--strict-bindings"
assert_not_contains "$quickstart_doc" "--allow-unresolved-bindings"

assert_contains "$concepts_doc" "# Harness Yard Concepts"
assert_contains "$concepts_doc" "A Harness Yard revision is a Git worktree revision with exactly one revision"
assert_contains "$concepts_doc" "Run View is the default runtime-user presentation"
assert_contains "$concepts_doc" "Author View is for Harness authors and Orbit authors"
assert_contains "$concepts_doc" 'Package Handle Coordinates are case-insensitive'
assert_contains "$concepts_doc" '`namespace/name[@version-or-tag]`'
assert_contains "$concepts_doc" 'not npm-style `@namespace/name`'
assert_not_contains "$concepts_doc" "hyard plumbing"

assert_contains "$configuration_doc" "# Configuration Reference"
assert_contains "$configuration_doc" ".harness/manifest.yaml"
assert_contains "$configuration_doc" ".harness/orbits/*.yaml"
assert_contains "$configuration_doc" "Deleted records and retained Git history are not active install state."
assert_contains "$configuration_doc" 'Supported fields are `name`, `description`, `role`, and `lane`'
assert_contains "$configuration_doc" "hyard orbit content apply <package> --check --json"
assert_contains "$configuration_doc" "hyard vars init"
assert_contains "$configuration_doc" "schema_version: 2"
assert_contains "$configuration_doc" "{{ vars.<name> }}"
assert_contains "$configuration_doc" "Rendering is strict"
assert_not_contains "$configuration_doc" ".orbit/vars.yaml"
assert_contains "$configuration_doc" "## Audit, Check, And Prepare"
assert_contains "$configuration_doc" 'Audit statuses are `pass`,'
assert_contains "$configuration_doc" 'A dirty but otherwise valid worktree is an'
assert_contains "$configuration_doc" '`hyard orbit prepare <package> --check --json`'

assert_contains "$content_workflows_doc" "# Content And Workflows"
assert_contains "$content_workflows_doc" "objective"
assert_contains "$content_workflows_doc" "scope boundary"
assert_contains "$content_workflows_doc" "record minimum"
assert_contains "$content_workflows_doc" "Split an Orbit Workflow only when the authored contract has split"

assert_contains "$harness_authoring_doc" "# Harness Authoring"
assert_contains "$harness_authoring_doc" "hyard publish harness workspace"
assert_contains "$harness_authoring_doc" "hyard install acme/frontend-lab"
assert_contains "$harness_authoring_doc" "hyard install acme/docs --bindings .harness/vars.yaml"
assert_contains "$harness_authoring_doc" "hyard vars init acme/docs --out .harness/vars.yaml"
assert_contains "$harness_authoring_doc" "schema_version: 2"
assert_contains "$harness_authoring_doc" "{{ vars.project_name }}"
assert_contains "$harness_authoring_doc" "Runtime Bindings"
assert_contains "$harness_authoring_doc" "Package Variables"
assert_contains "$harness_authoring_doc" "Explicit Git locators are"
assert_contains "$harness_authoring_doc" "hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace"
assert_contains "$harness_authoring_doc" "hyard assign orbit <orbit-package>"
assert_contains "$harness_authoring_doc" "hyard view author"
assert_contains "$harness_authoring_doc" "hyard guide save --orbit docs --target all"
assert_not_contains "$harness_authoring_doc" "hyard guide writeback"
assert_not_contains "$harness_authoring_doc" "hyard plumbing"

assert_contains "$orbit_authoring_doc" "# Orbit Authoring"
assert_contains "$orbit_authoring_doc" "hyard view author"
assert_contains "$orbit_authoring_doc" "hyard guide render --orbit docs --target all"
assert_contains "$orbit_authoring_doc" "hyard guide save --orbit docs --target all"
assert_contains "$orbit_authoring_doc" "hyard orbit content apply docs --check --json"
assert_contains "$orbit_authoring_doc" "hyard publish orbit docs --json"
assert_contains "$orbit_authoring_doc" "hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs"
assert_not_contains "$orbit_authoring_doc" "hyard guide writeback"
assert_not_contains "$orbit_authoring_doc" "hyard plumbing"

assert_contains "$installation_doc" "Harness Yard installs one public CLI binary"
assert_contains "$installation_doc" "brew install hyard"
assert_contains "$installation_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_contains "$installation_doc" "hyard install acme/docs"
assert_contains "$installation_doc" "hyard install acme/docs@0.1.0"
assert_contains "$installation_doc" "hyard install docs"
assert_contains "$installation_doc" 'Package Handle Coordinates are case-insensitive'
assert_contains "$installation_doc" 'not npm-style `@namespace/name`'
assert_contains "$installation_doc" 'stale cached'
assert_contains "$installation_doc" '`HYARD_CACHE_DIR`'
assert_not_contains "$installation_doc" "hyard plumbing"
assert_not_contains "$installation_doc" "harness-yard"

assert_contains "$release_surface_doc" "Harness Yard CLI (hyard)"
assert_contains "$release_surface_doc" 'Formal release assets distribute `hyard` only.'
assert_contains "$release_surface_doc" 'Public product documentation should teach the top-level `hyard` command surface.'
assert_contains "$release_surface_doc" "brew tap zack-nova/tap"
assert_contains "$release_surface_doc" "raw.githubusercontent.com/zack-nova/harnessyard/main/install.sh"
assert_contains "$release_surface_doc" "hyard install <template-source>"
assert_contains "$release_surface_doc" "hyard install <namespace>/<name>"
assert_contains "$release_surface_doc" "hyard install <namespace>/<name>@<semver>"
assert_contains "$release_surface_doc" "hyard install <curated-name>"
assert_contains "$release_surface_doc" "hyard install acme/docs"
assert_contains "$release_surface_doc" "hyard install acme/docs@0.1.0"
assert_contains "$release_surface_doc" "hyard install docs"
assert_contains "$release_surface_doc" "## Runtime Bindings Surface"
assert_contains "$release_surface_doc" "hyard vars init <package-source>"
assert_contains "$release_surface_doc" "schema_version: 2"
assert_contains "$release_surface_doc" "{{ vars.project_name }}"
assert_contains "$release_surface_doc" "Package Template Reference rendering is strict"
assert_contains "$release_surface_doc" "hyard vars init --defaults"
assert_contains "$release_surface_doc" 'Package Handle Coordinates are case-insensitive'
assert_contains "$release_surface_doc" 'not npm-style `@namespace/name`'
assert_contains "$release_surface_doc" 'Explicit Git locators are the advanced escape hatch'
assert_contains "$release_surface_doc" "hyard uninstall orbit <orbit-package>"
assert_contains "$release_surface_doc" "hyard uninstall harness <harness-package>"
assert_contains "$release_surface_doc" "hyard orbit member remove"
assert_contains "$release_surface_doc" "Package Uninstallation is package-manager-style removal"
assert_contains "$release_surface_doc" "Retained Git history, audit output"
assert_contains "$release_surface_doc" "provenance data, detached install records"
assert_contains "$release_surface_doc" "## Audit Review Surface"
assert_contains "$release_surface_doc" "hyard audit --json"
assert_contains "$release_surface_doc" "Audit is scoped to the current Git worktree"
assert_contains "$release_surface_doc" 'Orbit Package publish readiness on `hyard orbit prepare <package> --check --json`'
assert_contains "$release_surface_doc" "## Harness Start Demo Paths"
assert_contains "$release_surface_doc" "hyard clone https://github.com/acme/harness-templates.git demo-runtime --ref harness-template/frontend-lab"
assert_contains "$release_surface_doc" "hyard start --with codex"
assert_contains "$release_surface_doc" "hyard init runtime"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/harness-templates.git --ref harness-template/frontend-lab"
assert_not_contains "$release_surface_doc" "hyard plumbing"
assert_not_contains "$release_surface_doc" "compatibility"
assert_not_contains "$release_surface_doc" "writeback"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/docs --bindings .harness/vars.yaml"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/api --bindings .harness/vars.yaml"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ui --bindings .harness/vars.yaml"
assert_contains "$release_surface_doc" "hyard install https://github.com/acme/orbit-packages.git --ref orbit-template/ops --bindings .harness/vars.yaml"
assert_contains "$release_surface_doc" "Run View Package Installation outputs package guidance incrementally"
assert_contains "$release_surface_doc" 'Framework Activation is the public `hyard agent plan`, `hyard agent apply`, and'
assert_contains "$release_surface_doc" '`hyard agent apply`'
assert_contains "$release_surface_doc" "project/global Agent Framework assets"
assert_contains "$release_surface_doc" "It must not be documented as composing root"
assert_contains "$release_surface_doc" "Run View Root Guidance output remains owned by Package"
assert_contains "$release_surface_doc" "Bootstrap Guide output is a"
assert_contains "$release_surface_doc" 'git commit -m "Optimize frontend lab harness"'
assert_contains "$release_surface_doc" "hyard publish harness workspace"
assert_occurs_before "$release_surface_doc" 'git commit -m "Optimize frontend lab harness"' "hyard publish harness workspace"
assert_contains "$release_surface_doc" "Run View is the recommended runtime-user view"
assert_contains "$release_surface_doc" 'Run View publication should use `hyard publish harness <harness-package>`'
assert_contains "$release_surface_doc" "Author View is the authored-truth view"
assert_contains "$release_surface_doc" "Orbit Package publication remains available for authoring"
assert_contains "$release_surface_doc" "## Registry Entry Candidate Surface"
assert_contains "$release_surface_doc" "hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs"
assert_contains "$release_surface_doc" "hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace"
assert_contains "$release_surface_doc" "docs/maintainers/package-registry-source-contract.md"
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
assert_not_contains "$install_script" "Run: hyard plumbing"
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

assert_contains "$hyard_install_cmd" "hyard install docs"
assert_contains "$hyard_install_cmd" "hyard install acme/docs@latest"
assert_contains "$hyard_install_cmd" "hyard install acme/docs@0.1.0"
assert_contains "$hyard_install_cmd" "registry-source"
assert_contains "$hyard_install_cmd" "allow-yanked"

assert_contains "$harness_install_cmd" "Runtime Bindings YAML file"
assert_contains "$harness_install_cmd" "hideInstallBindingCompatibilityFlags(cmd)"

assert_contains "$hyard_registry_cmd" "Generate an Orbit Package Registry Entry Candidate"
assert_contains "$hyard_registry_cmd" "Generate a Harness Package Registry Entry Candidate"
assert_contains "$hyard_registry_cmd" "hyard registry entry orbit acme/docs@0.1.0"
assert_contains "$hyard_registry_cmd" "hyard registry entry harness acme/workspace@0.1.0"
assert_contains "$hyard_registry_cmd" "--out"
assert_contains "$hyard_registry_cmd" "--registry"

assert_contains "$hyard_vars_cmd" "Manage Runtime Bindings"
assert_contains "$hyard_vars_cmd" "schema_version: 2"
assert_contains "$hyard_vars_cmd" "{{ vars.<name> }}"

if command -v goreleaser >/dev/null 2>&1; then
  (
    cd "$repo_root"
    goreleaser check
  )
else
  echo "goreleaser not found; skipping goreleaser config check"
fi

echo "release surface hyard tests passed"
