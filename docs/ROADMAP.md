# KeeneticOS port roadmap

Updated 2026-11-24. This is the implementation/test order, not a claim that the
port is complete. Baseline for this plan: repository revision `3985993`.
The current temporary Titan agent is `9911433`, log-only;
[credential receipt validation](credential-receipt-validation-2026-11-24.md)
confirmed a zero-change restart with both credential records reused.
[Controller validation](controller-validation-2026-11-24.md) and
[2.4 GHz/hidden-SSID checks](band-hidden-validation-2026-11-24.md) used `632f35b`.
[Recovery validation](recovery-validation-2026-11-24.md) used
`dc0fba5`. Revalidate process/network state before another live run.

## Working rules

- While the operator is away or connectivity is unstable: local code, fixtures,
  package builds and analysis only. Do not change router/controller configuration.
- Main-router testing stays on explicitly mapped test WLANs. Exclude serving
  WLANs, shared radios, WAN, routing, NAT and DHCP. Back up before live mutations
  and verify restoration afterward. Named protections belong to the test session,
  not production defaults.
- Use a spare device for physical reboot, installation/upgrade, radio/country,
  trunk/VLAN creation or broad provisioning tests.
- Batch work into a reviewed candidate and acceptance run. Do not treat another
  small passing unit test as evidence that an entire feature works on hardware.
- Keep “implemented”, “configured/read back”, “client behavior verified” and
  “deployment verified” distinct in the feature matrix.

## Priority and dependencies

| Order | Workstream | Why this comes here | Completion evidence |
| --- | --- | --- | --- |
| P0.1 | Consolidated offline candidate gate | Recent fixes span planning, verification, actions, packaging and reporting; they need one repeatable baseline | One report pins source revision/tree, passes race/static/Python checks, builds both MIPS variants, verifies manifests and rebuilds from bundled vendored source offline |
| P0.2 | Installation, upgrade and boot lifecycle | A working temporary agent is not an installable, recoverable product | On a spare device: install, upgrade, interrupted/failed upgrade, restart and physical reboot; preserve adoption/config; recover pending transactions before provisioning; restore previous package when upgrade fails |
| P1.1 | Controller compatibility/default profile | Default BSS-transition requests currently block ordinary provisioning; ignored settings must not look supported | Document a supported controller preset; audit advertised capabilities against mappings; establish per-WLAN native 802.11v semantics or retain explicit rejection; classify known omissions without silently weakening security |
| P1.2 | Core Wi-Fi acceptance candidate | Existing observations come from different revisions; prove the chosen candidate works as a whole | Complete the acceptance matrix below on both bands, with native/controller/client evidence and restoration checks |
| P2.1 | Client policy enforcement | Existing ACL/RSSI/isolation mappings need behavior tests before broader use | Individual allow/deny entries, isolation with two Wi-Fi clients, RSSI enforcement and recovery; controller block/kick only after native scope and semantics are established |
| P2.2 | VLAN data plane, then owned creation | Guest/enterprise work depends on correct network separation | First prove existing-bridge binding and tagged traffic/DHCP/isolation; then design explicit resource ownership, preimages and removal for new bridges/VLAN uplinks on a spare trunk |
| P3.1 | Real fast roaming and BSS transition | FT-enabled authentication is not FT reassociation; 802.11k is not 802.11v | Controlled handoff with association/capture evidence; matching domain/security across APs; test interoperability separately from Keenetic-only roaming |
| P3.2 | Enterprise authentication and guest portal | These need AAA/session/firewall integration, not just extra SSID commands | Local RADIUS/portal lab, authorization/expiry/revocation, failure behavior and VLAN isolation; preserve ordinary WLAN connectivity during service outages |
| P4 | Mesh, scanning, shaping and remaining radio knobs | Native feasibility and side effects are less established | Separate capability investigation and minimal proof for each; establish units/scope for transmit power, DTIM/U-APSD and shaping before translating requests |

WEP remains unsupported; investigate only if a concrete legacy requirement is
identified. It is not a prerequisite for modern WPA2/WPA3 acceptance. Locate is
also pending a native operation with equivalent semantics; do not substitute a
global LED shutdown and call it Locate.

Explicit `aaa.N.is_guest=true` requests are rejected as `wireless.N.guest-policy`
until guest enforcement is implemented. Peer isolation and existing VLAN binding
alone do not implement guest authorization, restricted networks, or a portal.
Local tests cover normal/scoped planning, paused guest requests, malformed flags,
and refusal before apply preparation. This is a compatibility guard, not guest support.

Minimum data/management/beacon rates, CCK-rate selection, and enabled per-client
bandwidth requests matched by VAP are now reported as ignored setting names in
normal and scoped test plans. Their values are not retained in diagnostics and
they produce no native commands. Native rate and shaping support remains open.

The [rate-control investigation](rate-control-investigation-2026-11-24.md)
records documented radio/interface/host scope and hash-verified firmware export
inventories. No minimum-rate/DTIM/U-APSD mapping was established. Host-wide shaping
needs resource ownership and association-lifetime handling before VAP translation;
power requests need a justified conversion to native percentage units.

## P0 tasks

### P0.1 — One offline command and candidate evidence

- [x] Add `tools/check-release.py`: local checks only, no router/Docker/controller
      access; fail on a failed check; retain logs and a machine-readable report.
- [x] Pin Git revision and source-tree fingerprint; compare before/after snapshots.
- [x] Run uncached Go race tests, vet and Python tooling tests with dependency downloads off.
- [x] Build both MIPS packages, check ELF machine/byte order and verify manifests.
- [x] Rebuild each executable from its bundled source/vendor tree with a fresh empty
      module-cache location and dependency downloads off.
- [x] Run the gate on a clean committed candidate. A passing offline gate is not
      a live-test pass, a signature, or proof of bit-identical builds.

### P0.2 — Upgrade/recovery readiness

- [x] Harden shared adoption-state persistence: unique private stages, file sync,
      atomic rename and directory sync; cover legacy temp symlinks, failed
      replacement and concurrent complete-record writes. The Linux container
      harness runs these state tests as well as the real-agent lifecycle suite.
      This is not physical power-loss proof.
      Clean `45a7a4a` passed the full offline gate and all nine container checks.
- [x] Gate outgoing informs and changed-reply actions on successful adoption-state
      persistence. Retain pending outcomes for local retry; exercise failed saves,
      no acknowledgement during failure, deferred actions and encrypted resumption
      with the new key through a loopback controller test.
      Clean `8833382` passed the full offline gate and all ten container checks.
- [x] Bind received configuration text, version and controller/key identity in
      the atomic adoption-state record. Plan only from that snapshot; treat the
      separate text file as diagnostic. Request a fresh full config for legacy
      or mismatched records; detect same-version body changes during planning
      and confirmation. Test blocked diagnostic writes and encrypted migration.
      Real-controller refresh interoperability still needs acceptance validation.
- [x] Add a separately invoked network-isolated Linux container harness using
      the actual installer/service and agent. Cover disabled installation,
      start/stop, successful upgrade, explicit restart, failed-upgrade restoration
      and private state/config/mapping preservation. Native telemetry is synthetic;
      same-source artifacts have different build IDs. Hardware/cross-version
      behavior is still unverified by this harness.
      Clean candidate `0d0063f` passed all six checks and the full offline gate;
      see [container lifecycle validation](container-lifecycle-validation-2026-11-24.md).
- [x] Include all staged files and existing replacement-target backups in the
      install space check before retaining 16 MiB headroom. Boundary tests cover
      first install, upgrade and preserved existing configuration.
- [x] Execute the shipped installer in a private fake Entware root with injected
      corrupt packages, missing notices, unrunnable executables, low space,
      copy/rename failures, unknown service status and failed recovery restart.
      Service behavior is simulated; this does not validate the native service.
      Check missing corresponding source at the distribution gate; it need not
      consume router installation storage.
- [x] Stage and verify replacements before stopping the service; preserve existing
      configuration and adoption files. Local failure injection covers corrupt
      packages, failed probes/copies/renames, low space, unsafe targets and caught
      termination. Ordinary failures restore old files and previous running state;
      failed recovery retains private backups and blocks reinstallation.
- [x] Document manual inspection/recovery for forced termination or power loss
      in [installation-recovery.md](installation-recovery.md). Record synchronized
      phases and backup/replacement hashes; replace the guarded service first and
      restore it last. Local forced-termination tests verify retained evidence
      and blocked startup; completed rollback bypass is scoped to its transaction.
      No on-device upgrade or power-loss durability claim
      follows from the local harness; successful upgrades leave the service stopped.
- [x] Add `-installation-status /opt`: read-only structural inspection and
      backup/current hash comparison with fixed status labels, no configuration
      contents or hashes in output. Offline tests cover malformed/partial data,
      unsafe paths, symlinks, special files, changed files and state-free CLI use.
      Native Titan execution and synthetic mixed-file hashing passed; see
      [installation inspector validation](installation-inspector-validation-2026-11-24.md).
      This does not implement automatic restoration or prove a real upgrade.
- [ ] Prove upgrade preserves adoption key, slot/network mappings, action policy
      and private permissions without exposing credentials in reports/logs.
      A [temporary log-only upgrade](logonly-upgrade-validation-2026-11-24.md)
      from `9911433` to `62b1285` preserved adoption/cache/slots byte-for-byte and
      remained connected with application disabled. Installed-package and
      applying-mode lifecycle coverage is still required.
- [ ] Check startup recovery, watchdog executable lifetime and journal persistence
      across agent replacement and device reboot.
      The container harness now exercises the actual crash-probe watcher across
      binary replacement and startup replay before telemetry. Hardware reboot
      and cross-source-version compatibility remain separate requirements.
      Clean `9b98230` passed all eight container checks, persistent lock-inode
      assertions and the full offline release gate.
      Offline recovery tests cover interrupted replay, retained journals and fresh
      identities for repeated changes. Empty inverse entries are rejected before
      any replay. Journal creation, clearing and recovery now share a persistent
      kernel lock; detached watchers check identity inside that lock. Offline
      tests cover cross-process exclusion, canceled waits and stale identities.
      Native transaction commands and confirmation also check journal ownership
      under that lock. Recovery marks the journal before replay, fencing out old
      commands even on failure; offline tests cover health-check races, replacement
      journals and forward deadlines. Still validate native timing and migration
      from older binaries whose watchers do not use the journal lock.
      Normal daemon startup now locks state and recovers pending changes before
      telemetry collection or adoption-state loading. CLI subprocess tests cover
      telemetry/adoption failures, failed recovery, competing daemons and a
      read-only probe that leaves the journal untouched.
      Candidate `dc0fba5` passed immediate disabled-slot rollback on both Titan
      bands, detached crash recovery on 5 GHz, startup recovery before malformed
      adoption state on 2.4 GHz and native refusal of a competing daemon. Full
      configuration restoration was verified on-device. Installed upgrade and
      reboot lifecycle remain open; see the linked validation report above.
- [ ] Run installation and physical reboot only on a spare router with a recovery
      path. Do not use the Titan main router as the first installed-upgrade test.

## P1 acceptance matrix

Run against one pinned candidate; do not merge observations from older builds
into a new candidate's pass record. Record device/firmware, controller version,
binary hash, plan scope, native readback, controller state, client outcome and
cleanup evidence. Keep raw configurations/credentials private.

| Case | Current evidence | Next decisive check |
| --- | --- | --- |
| WPA2, 2.4 and 5 GHz | `632f35b`: native authentication, DHCP ACK and Wi-Fi-bound HTTPS on both bands; 2.4 GHz DNS also bound to Wi-Fi succeeded | Continue security-mode/client coverage; do not generalize to every DNS endpoint or controller option |
| WPA3-only | Titan 5 GHz WPA3 authentication passed | 2.4 GHz and traffic; interoperability on available clients |
| WPA2/WPA3 transition | Titan Mac negotiated WPA3 | A WPA2-only client must also connect; test both bands |
| Hidden SSID | `632f35b`: native hidden flag, explicit known-SSID 2.4 GHz WPA2 join, DHCP/DNS/HTTPS and exact restoration passed | Passive scan/beacon contents and 5 GHz hidden-client coverage |
| Open / OWE | Translation and disabled-slot native command tests | Real client association and correct encryption/PMF; never claim OWE from an open connection |
| Pause/resume and schedules | `632f35b`: controller pause/resume confirmed on both bands; earlier schedule transitions passed | Repeat schedules on candidate, including timer boundaries and restart while paused |
| Automatic reconnect | Latest Mac run selected main AP0 after pause, rather than returning to test AP6 | Controlled client selection and bounded observation; preserve main-network preferences |
| Controller interruption | Real 503 rollback/retry previously passed | Repeat on candidate with status-report outcome and history verified |
| Idempotent provisioning | `9911433`: two initial writes, then zero prepared changes and two records reused after restart; health/controller checks confirmed | Live drift invalidation, re-adoption and installed lifecycle; retain health checks on no-op plans |
| Inactive WLAN / radio-off | Verification logic tested offline | Spare-device validation; no shared-radio toggling on the main router |
| Client policy | Empty allow-list blocked join and removal restored it | Individual entries and two-client isolation/RSSI behavior |
| Reboot control | Default-off policy/dispatch tests only | Spare-device reboot, service return and native startup-config behavior |

Credential readback investigation confirmed the native two-argument `ns3` form.
The comparator excludes equivalence with a quoted literal passphrase. New
receipt-based optimization binds device identity, the exact requested
credential, AP/SSID, adoption key and observed native representation after a
confirmed write. It rechecks drift after health, uses private atomic storage,
and does not learn from failed/rolled-back, canceled or log-only operations.
Offline tests cover restart reuse, drift, pause/resume, invalid cache files,
pending recovery and failed confirmation. Initial write and restart reuse passed
on Titan; native drift and installed lifecycle validation remain open;
ordinary offline CLI preflight remains conservative without receipt state.

The [Wi-Fi-system investigation](wifi-system-investigation-2026-11-24.md) adds
a read-only WLAN/band/AP membership diagnostic, verified on Titan. The documented
manager BSS control has broader scope than an AP command, with automatic AP
allocation and bridge/band constraints. Default BSS requests remain rejected;
future manager support needs explicit ownership, exact membership and rollback
semantics, including combined 802.11k/11v behavior. No main Wi-Fi changes were
used for this investigation.

## Investigation prerequisites

- Interactive native command discovery needs the separate KeeneticOS account;
  Entware SSH credentials did not authenticate the attempted native login.
  This does not block offline development or the existing noninteractive NDM path.
- Isolation needs two Wi-Fi clients. WPA2-only transition compatibility needs
  a client that can actually exercise WPA2 rather than automatically choosing SAE.
- FT needs a controlled handoff and protocol-level evidence. Current successful
  authentication with FT enabled does not close that task.
- VLAN creation/mesh/radio work needs a spare device/port and a documented topology.
- Controller/BSS defaults are a high-priority compatibility issue. If native
  802.11v is global-only, do not enable it globally to satisfy a per-WLAN request.

## Useful work during the current hour away

1. Write this roadmap and freeze a candidate acceptance checklist.
2. Implement and run P0.1, keeping a report and logs for review when connectivity
   is stable. Resolve offline failures before another hardware deployment.
3. Use remaining time for P0.2 installer failure analysis/harness design and the
   supported-controller preset. Do not start speculative mesh/portal work or
   repeat native-login attempts while the prerequisite is unresolved.

## Next session at home

After the operator confirms a stable test environment, revalidate Ethernet,
router identity, controller connectivity and backups first.
Review the candidate diff and its offline report, then stage it with space/hash
checks and start log-only. Use the isolated SSID for the P1 cases. Finish each
run with journal checks, full native comparison and controller-profile restoration.
Installation/reboot/radio/trunk tests wait for a spare test device.
