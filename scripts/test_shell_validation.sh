#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

cd "$repo_root"

sh ./scripts/test_validation_parity.sh
sh ./scripts/test_run_golangci_lint.sh
sh ./scripts/test_build_binaries.sh
sh ./scripts/test_install_script.sh
sh ./scripts/test_run_view_guidance_docs.sh
sh ./scripts/test_release_surface_hyard.sh