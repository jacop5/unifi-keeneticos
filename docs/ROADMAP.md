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
- [ ] Define operational recovery for forced termination or power loss.
      No on-device upgrade claim
      follows from the local harness; successful upgrades leave the service stopped.
- [ ] Prove upgrade preserves adoption key, slot/network mappings, action policy
      and private permissions without exposing credentials in reports/logs.
- [ ] Check startup recovery, watchdog executable lifetime and journal persistence
      across agent replacement and device reboot.
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
