# Version-bound configuration snapshot validation — 2026-11-25

Clean candidate `27710edcc568f855881b355b1152b8f7ebd87c0d` passed the full offline
release gate and all eleven isolated Linux container checks. Both MIPS packages,
manifests and offline source rebuilds passed.

## Storage and planning behavior

The authoritative controller body is now saved in `state.json` together with its
configuration version and adoption state. It is bound to the controller address
and a digest of the adoption key. Planning and configuration-dependent telemetry
read this snapshot, not the separate diagnostic `system_cfg.txt` copy.

Missing, legacy or mismatched snapshots block planning with `config-unverified`.
The Keenetic inform payload requests a fresh full configuration by advertising
configuration version `0` while preserving the received version and adoption
identity internally. After a matching snapshot is stored, it advertises the
received version normally. Body changes under an unchanged version also produce
a new internal planning identity and invalidate in-progress confirmation.

Tests cover version/controller/key mismatches, private payloads, same-version
body changes, failed state persistence and encrypted resumption. An actual agent
process against a loopback controller migrates legacy state and produces the new
plan even when a directory blocks the diagnostic-copy write. The key and complete
snapshot are committed before acknowledgement; stale diagnostic text is not an
authoritative input.

## Titan and actual UniFi OS acceptance

Management remained over Wi-Fi. The existing agent was running `62b1285` in
log-only mode with no pending rollback. Its legacy state had no configuration
snapshot. Private state, receipts, slots and normalized native preimages were
retained on the router; raw configuration and credentials were not downloaded.

The candidate was staged separately and verified before execution:

- Target: Titan KN-1810, KeeneticOS 5.1.9, linux/mipsle softfloat.
- Executable SHA-256: `8dbcf39d0f80a12e564e8391cb3bf645175286c7dacb0191e617f1f3b3d6f0ae`.
- Only the temporary agent was restarted, using the same state, inform URL and
  slot map with `-wlan-test-only`. Application, LED/reboot control, experimental
  SAE and upgrade-version reporting remained disabled.
- Actual UniFi OS automatically resent the configuration; no profile change,
  force-provision or re-adoption request was made. A snapshot appeared in the
  private state record. Adoption key/cipher/adopted flag, inform URL and reported
  software version matched the retained preimage.
- The controller remained adopted/connected at configuration version
  `ab67a4ac718cc88c`; a later read showed a last-seen age of 3.8 seconds.
- Provisioning reported the expected two BSS-transition omissions as
  `unsupported`, with `apply_enabled=false`, rather than remaining unverified.
  This path is reached only after the snapshot passes its binding checks.
- Credential receipts and slot mapping were byte-for-byte unchanged. The state
  directory remained mode 700; state, diagnostic config and receipts were mode 600.
- All normalized native configuration text matched before/after, SHA-256
  `2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
  Normalization removes terminal escapes, CR, blank lines and generated `! $$$`
  comments. The main WLANs were unchanged.

The candidate remains PID 28655 in log-only mode. Evidence and private backups
are under `/tmp/ukn-config-snapshot.ACVEpP`; the previous `62b1285` executable is
retained as a fallback. The unused older `9911433` binary was retired only after
its hash and absence from all process executable links were checked, preserving
staging headroom.

Physical reboot/power-loss durability, installed-package upgrade and applying-mode
acceptance of this candidate remain outstanding. Older agents do not enforce the
new snapshot rules. The shared device package records the snapshot for both
backends; this planning/refresh policy is enabled by the Keenetic runtime.
