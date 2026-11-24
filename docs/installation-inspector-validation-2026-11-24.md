# Installation inspector validation — 2026-11-24

Candidate: `a31db25`. Titan KN-1810, KeeneticOS 5.1.9, little-endian MIPS.
Management was over Wi-Fi. No native configuration, service lifecycle or
controller changes were made.

The clean offline release gate passed race/static/tooling checks, both MIPS
packages and manifests, and offline rebuilds from bundled source. Targeted tests
cover mixed/new/old/changed files, corrupt backups, original absence records,
malformed or incomplete metadata, traversal, symlinks, special files, size limits,
unknown markers, output privacy and CLI conflicts without adoption-state creation.

The separately staged MIPS little-endian executable had SHA-256
`af4b1a1fe4c3650e019d2deddc4b37c5fd5ba96aaecc76aa1706da0119df2642`.
Only `-installation-status` was executed:

- Against `/opt`, it reported no pending markers, no lock and no transactions.
  This is not a claim that an installed package exists or is complete.
- Against a new private temporary synthetic Entware tree, it recognized a
  committing transaction, its matching lock and six verified backups. It
  reported the synthetic binary as matching its backup, the altered NOTICE as
  a hash mismatch, and the remaining files as matching their replacements.

The actual agent remained PID 2825 at the previously deployed `9911433` lab
executable. The staged diagnostic was not installed or launched as a daemon.
After checking that no process used it and rechecking its hash, the diagnostic
and this test's synthetic tree were removed. Temporary storage returned to
30,676 KiB available.

Actual interrupted upgrades, automatic restoration, reboot and power-loss
durability remain unverified on hardware. The inspector is a read-only snapshot;
its results do not authorize deleting markers or starting a mixed installation.
