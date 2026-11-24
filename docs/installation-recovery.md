# Interrupted installation recovery

Do not delete `/opt/.ukn-install.*` or start the daemon blindly after an interrupted installation. Keep private copies of the lock and work directory before attempting recovery. Backups can contain configuration secrets; do not upload them to an issue.

The installer records these files in its private work directory:

- `phase`: prepared, stopping, stopped, committing, installed, or restored. It records progress, not proof that every operation completed.
- `service-before`: absent, stopped, or running.
- `installer-pid`: historical process ID; after a reboot or PID reuse it does not establish whether an installer is running.
- `targets`: fixed paths relative to `/opt` that this transaction intended to replace.
- `backup-hashes` and `new-hashes`: SHA-256 records for backup and replacement files. An originally absent target has a corresponding `backup/<target>.absent` marker.

The lock's `work-directory` points at the transaction. Preserve both if present. Verify that no installer is still operating before recovery. Inspect every target and compare current files with the recorded replacement hashes and backup files with their recorded hashes. These manifests detect accidental changes; they are not signatures.

Recovery currently requires manual review. Restore only the listed package targets, using verified backups. Restore non-service files first and synchronize storage before restoring the service script last; synchronize again afterward. For an originally absent target, remove the installed file only after verifying that it matches this transaction's replacement and has no subsequent user changes. Stop if any path, hash, or absence marker is unexpected. Preserve existing configuration and adoption state that are not listed as replacement targets.

Retire the markers only after all files are consistent and the recovery has been reviewed. Start explicitly if the service was previously running and the configuration is appropriate. Do not set `UKN_INSTALL_RECOVERY` manually: it is reserved for the installer's completed ordinary rollback.

Local tests cover ordinary failures, caught termination, corrupt backups, hash failures, and forced termination during partial replacement. The forced-termination test verifies retained metadata and blocked service startup. Actual power loss, USB storage durability, reboot recovery, and installed-package upgrades still require spare-device validation. No automatic power-loss recovery is implemented.
