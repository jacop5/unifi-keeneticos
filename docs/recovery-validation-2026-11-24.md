# Titan recovery validation — 2026-11-24

Candidate: `dc0fba525306fea1ae287fd94d024293c7ec7861`, clean build.
Executable SHA-256:
`0ba6cb6bc5ca817a1c1bb26411566217daeac2bc2cd61cfc3c8fed59a9636e16`.

Environment: Titan KN-1810, KeeneticOS 5.1.9, Linux MIPS little-endian
softfloat; UniFi Network 10.5.67 (`atag_10.5.67_35187`) in the existing
local UniFi OS test deployment. The Mac used Ethernet.

## Scope and results

The full clean offline release gate passed before staging: race tests, vet,
Python tooling tests, both MIPS packages/manifests and offline rebuilds from
their bundled source. Live tests used a temporary executable, not an installed
upgrade. The normal adopted agent remained in log-only WLAN test mode.

| Check | Observed result |
| --- | --- |
| Read-only native collection | Candidate probe completed on the Titan |
| Controller reconnection | After correcting the test agent's saved controller address for the Mac's changed Ethernet IP, the existing device remained adopted and returned to connected state |
| Immediate rollback, both bands | Disabled, SSID-free AP5 slots passed the SSID rollback probe; interface preimages were restored |
| Detached crash recovery, 5 GHz | Probe exited intentionally before confirmation; a separate watcher and pending journal were observed at 23 seconds; the journal was cleared by the observation at 91 seconds |
| Startup recovery, 2.4 GHz | A separate crash-probe journal was moved to the daemon's expected journal filename in a private test state directory; startup recovered it before rejecting an intentionally malformed adoption file |
| Competing daemon | A second process using the adopted agent's state directory was refused |
| Private files | Recovery lock and test adoption file had mode 0600 |
| Restoration | Complete native configuration matched the pre-test baseline; controller test-profile comparison reported no changed fields |

Only the disabled AP5 test slots were mutated. Serving WLANs, test WLAN AP6
profiles, shared radios, WAN, routing, NAT and DHCP were not provisioned.
The controller-address change affected private test-agent state; a reverse
substitution compared equal to its on-router backup before replacement.

Configuration comparisons stayed entirely on the router. Normalization removed
terminal escape sequences, carriage returns, blank lines and generated
`! $$$` comment metadata. All remaining configuration text compared equal;
its before/after SHA-256 was
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
No raw configurations or adoption keys accompany this report.

## Limits

The detached deadline was observed, not measured precisely; 91 seconds is the
later observation time, not a claim that rollback took 91 seconds. The startup
test deliberately moved the probe journal, so its original watcher subsequently
had no journal to replay. It does not establish a simultaneous startup/watcher
race on hardware; cross-process exclusion and ownership races have offline tests.

These results do not verify installed upgrades, physical reboot/power loss,
mixed-version watchers, active-client traffic or all controller provisioning
features. Default BSS-transition requests still cause explicit rejection of the
test profile. DTIM and U-APSD remain reported as ignored settings. The agent was
left connected with application disabled, retaining historical confirmations.
