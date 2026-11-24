# Consolidated log-only upgrade validation — 2026-11-24

Titan KN-1810, KeeneticOS 5.1.9, little-endian MIPS. Management was over Wi-Fi.
This was a manual temporary-daemon upgrade, not an installed-package upgrade,
boot test or WLAN-application test.

The clean `62b1285` build reported linux/mipsle softfloat and SHA-256
`d0826c9bb5eefac1ded07813e123ecaaf4b2dbb4f2161ab8aa31a234db110e28`.
Its implementation matches the earlier clean `a31db25` offline release gate;
the intervening commits changed documentation only. The new build was staged
under a unique private temporary filename with its hash verified before use.

Before stopping the previous `9911433` daemon, checks confirmed its process
identity and log-only arguments, controller adoption/connectivity, absence of a
rollback journal and sufficient staging headroom. Private native configuration,
adoption-state, credential-receipt and slot-map preimages were retained on the
router. Raw native configuration was not downloaded.

The previous process exited after TERM. The router's existing
`start-stop-daemon` launched the candidate with a private PID/log and the same
state, inform URL, slot map and `-wlan-test-only` flag. Application, LED control,
reboot control, experimental SAE and version-reporting overrides were absent.
A launch-failure fallback retained the old executable; it was not needed.

Observed results:

- The candidate remained running as PID 10478; UniFi still reported adopted and
  connected, with compatibility version `6.8.2.15592` and configuration version
  `ab67a4ac718cc88c`.
  A later status request still showed adoption/connection with a last-seen age
  of 6.7 seconds, and the same candidate process remained alive.
- Provisioning status reported `apply_enabled=false` and the expected unsupported
  BSS-transition requests for both test WLAN indexes. No controller profile was
  changed to hide that limitation.
- Ignored minimum data/management/beacon rates and CCK selection appeared for the
  requested 2.4 GHz WLAN fields, alongside DTIM/U-APSD diagnostics. This validates
  real-controller reporting, not native rate enforcement. Guest rejection and
  bandwidth reporting were not exercised through controller changes in this run.
- Adoption state, credential receipts and slot map remained byte-for-byte equal
  to their preimages. Adoption state and credential receipts remained mode 600.
  The earlier confirmed provisioning history was retained separately.
- The normalized native configuration was identical before and after, SHA-256
  `2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
  Normalization removes terminal escapes, CR, blank lines and generated `! $$$`
  comments. All other configuration text matched, including the main WLANs.
- No rollback journal appeared. No native application, radio restart or WLAN
  pause was requested.

The candidate remains in log-only mode. Private evidence and backups are under
`/tmp/ukn-logonly-upgrade.kDMryG`; the previous executable remains available as a
fallback. Temporary files and both agents' binaries are not persistent deployment.
Installed upgrades, physical reboot/power loss and a consolidated applying-mode
acceptance run remain outstanding.
