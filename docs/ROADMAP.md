# KeeneticOS port roadmap

Updated 2026-11-24. This is the implementation/test order, not a claim that the
port is complete. Baseline for this plan: repository revision `3985993`.
The last hardware-verified temporary Titan agent was `26532b6`, log-only;
its current process/network state must be revalidated before another live run.

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

- [ ] Add failure-injection coverage around actual installer/service logic:
      corrupt package, missing notices, wrong executable, low space,
      failed copy/rename and failed startup. Check missing corresponding source
      at the distribution gate; it need not consume router installation storage.
- [x] Stage and verify replacements before stopping the service; preserve existing
      configuration and adoption files. Local failure injection covers corrupt
      packages, failed probes/copies/renames, low space, unsafe targets and caught
      termination. Ordinary failures restore old files and previous running state;
      failed recovery retains private backups and blocks reinstallation.
- [ ] Complete missing-notice/failed-restart coverage and define operational
      recovery for forced termination or power loss. No on-device upgrade claim
      follows from the local harness; successful upgrades leave the service stopped.
- [ ] Prove upgrade preserves adoption key, slot/network mappings, action policy
      and private permissions without exposing credentials in reports/logs.
- [ ] Check startup recovery, watchdog executable lifetime and journal persistence
      across agent replacement and device reboot.
- [ ] Run installation and physical reboot only on a spare router with a recovery
      path. Do not use the Titan main router as the first installed-upgrade test.

## P1 acceptance matrix

Run against one pinned candidate; do not merge observations from older builds
into a new candidate's pass record. Record device/firmware, controller version,
binary hash, plan scope, native readback, controller state, client outcome and
cleanup evidence. Keep raw configurations/credentials private.

| Case | Current evidence | Next decisive check |
| --- | --- | --- |
| WPA2, 2.4 and 5 GHz | Prior authentication/DHCP/Wi-Fi-bound HTTPS passes | Repeat on candidate with traffic demonstrably bound to Wi-Fi |
| WPA3-only | Titan 5 GHz WPA3 authentication passed | 2.4 GHz and traffic; interoperability on available clients |
| WPA2/WPA3 transition | Titan Mac negotiated WPA3 | A WPA2-only client must also connect; test both bands |
| Hidden SSID | Translation/unit coverage | Native hidden flag and explicit client join, then exact restore |
| Open / OWE | Translation and disabled-slot native command tests | Real client association and correct encryption/PMF; never claim OWE from an open connection |
| Pause/resume and schedules | Controller/native transitions passed | Repeat on candidate, including timer boundaries and restart while paused |
| Automatic reconnect | Not observed; manual same-client rejoin passed | Controlled client selection, client-side reason/state evidence and bounded observation; avoid changing main-network preferences implicitly |
| Controller interruption | Real 503 rollback/retry previously passed | Repeat on candidate with status-report outcome and history verified |
| Idempotent provisioning | Health-on-unchanged fix tested offline | No configuration writes, but operational/controller checks still required |
| Inactive WLAN / radio-off | Verification logic tested offline | Spare-device validation; no shared-radio toggling on the main router |
| Client policy | Empty allow-list blocked join and removal restored it | Individual entries and two-client isolation/RSSI behavior |
| Reboot control | Default-off policy/dispatch tests only | Spare-device reboot, service return and native startup-config behavior |

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
