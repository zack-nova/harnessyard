# Package uninstallation fully removes installed packages

Package uninstallation means removing an installed Orbit Package or Harness Package from the current Harness Runtime, not detaching an active member while retaining hosted package truth. We chose package-manager-style uninstall semantics because users expect `hyard install <package>` followed by `hyard uninstall <package>` to leave the runtime as though that package is no longer installed, while Git history remains the evidence trail for committed installs and uninstalls.

## Consequences

`hyard uninstall` deletes the package's active install record, hosted OrbitSpec, marked root guidance block, and package-owned runtime files. Deletion is based on package ownership, not Git tracked state or projection/write/export/orchestration scope; process-role files are deleted when they are package-owned. Markerless Run View root guidance is presentation text without owner identity, so uninstall leaves it untouched. Local Git changes on package-owned files trigger warning and confirmation in interactive use, `--yes` confirms deletion non-interactively, and `--dry-run` previews the deletion set. Lower-level detach, shrink, or member removal behavior may remain as plumbing or maintainer operations, but the public `uninstall` and compatibility `remove` surfaces share the full package uninstallation semantics.
