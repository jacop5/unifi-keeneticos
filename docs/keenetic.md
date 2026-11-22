# KeeneticOS port (in development)

The Keenetic executable is `cmd/ukn`. It runs locally under Entware and
uses `ndmc -c` instead of OpenWrt's UCI, ubus and iw tools. The original
OpenWrt executable remains available in `cmd/uow`.

Verified hardware: Keenetic Titan KN-1810, KeeneticOS 5.1.9, MIPS
little-endian. A statically linked soft-float Go executable ran successfully
on the router and collected system, interface, radio and association status.
The Entware installer was also exercised on that router: binary, init script
and private configuration were installed successfully, and starting with the
default configuration correctly left the service disabled. Both immediate
failure rollback and detached recovery after parent termination were verified
using an unused disabled AP slot, with restoration checked in running-config.

The Hero DSL KN-2410 on KeeneticOS 5.1.5 was also inspected and its read-only
collector probe passed. Unlike the Titan tested above, this router's Entware
ELF binaries are **big-endian MIPS** (ELF data encoding 2); `uname -m` reports
`mips` on both routers and is insufficient to choose the build. Use
`sh tools/build-keenetic.sh mips` for the Hero DSL; the bundle is written to
`dist/keenetic-mips/`. See the live controller test results below.

## Titan controller tests (2026-11-22)

### Log-only baseline

The current build was rechecked on Titan KN-1810 in router mode, using wired
management. ELF data encoding 1 confirmed little-endian MIPS. The read-only
collector completed successfully; the executable and new private test state
were kept in a temporary directory, without replacing the installed service.

The agent connected to the existing UniFi OS Server 5.1.42 / Network 10.5.67
test controller in log-only mode. Adoption completed, AES-GCM was enabled,
and the controller reported connected. Restarting only the test agent retained
the adoption key and resumed informs without re-adoption. Firmware requests
remained in the default refuse mode. WLAN application and LED control were
not enabled.

Explicit test mappings selected AP6 on each band; at this initial stage both
slots stayed down with no configured SSID. The two serving AP0 WLANs and their shared radios
are excluded from this test session. Their names are stored only in the
private local test record, with no hard-coded deployment restriction added.
All native configuration settings matched the saved baseline after adoption
and again after the agent restart, excluding generated comment metadata.
This initial stage verified collection, controller adoption and persistence;
WLAN provisioning and client traffic were checked separately below.

### WPA2 provisioning, pause/resume and client traffic

A controller AP group containing only the Titan was created for
`UKN-Test-Titan`, with a fresh random WPA2 password kept in a private local
file. The existing Hero WLAN/profile was not changed. The controller generated
WLAN indexes 3/4 for the new SSID; the temporary mapping selected native AP6
on each band. A redacted preview was checked to contain only those AP commands
and their membership in the existing home bridge. Shared-radio and serving
AP0 commands were excluded before guarded application was enabled.

Provisioning passed native WLAN/security/bridge readback and the real
controller health check, cleared the journal, and brought both test APs up.
Pausing only this controller profile omitted its WLAN entries; its checked
plan was exactly the two AP6 `down` commands. Applying it disabled both test
APs while both serving AP0 WLANs remained up. Resume produced the exact prior
test-WLAN plan and successfully restored both test APs. Each controller change
was received and previewed in log-only mode before its guarded apply.

With the agent returned to log-only mode and Ethernet still active, the Mac
joined the test SSID. Native associations confirmed its matching Wi-Fi MAC
authenticated on `WifiMaster1/AccessPoint6` (5 GHz). The Wi-Fi interface obtained
an IPv4 lease whose DHCP server identifier matched the Titan. An HTTPS request
explicitly bound to that Wi-Fi address returned HTTP 200. Packet capture was
unavailable because macOS denied BPF access, so a second TLS request used a
socket with Darwin `IP_BOUND_IF` set to the Wi-Fi interface and checked by
`getsockopt`; it also returned HTTP 200 with certificate verification enabled.
macOS's inconsistent SSID status text was not counted as association evidence.
A separate controller band-selection test limited the profile to 2.4 GHz.
Its checked plan enabled only `WifiMaster0/AccessPoint6` and disabled the
5 GHz test AP. Native association confirmed an authenticated client on the
2.4 GHz test AP, with a Titan-issued IPv4 lease; a certificate-verified HTTPS
request using `IP_BOUND_IF` also returned HTTP 200. Both WPA2 client paths
therefore passed.

The Mac's new test preference was removed afterwards, its original Wi-Fi
power-on state restored, and Ethernet remained active. Final comparisons
confirmed that both serving WLANs and shared radios exactly matched baseline.
After excluding only test AP6 blocks, their intended bridge membership,
terminal escape formatting and generated comments, all other native settings
also matched baseline. No routing, NAT, DHCP-server or startup configuration
was changed. The test SSID remains enabled on both AP6 slots; the agent is
left log-only. A subsequent 2.4 GHz pause/resume test passed guarded
application and controller confirmation in both directions. The Mac selected
another saved WLAN during the pause and did not authenticate on test AP6
after resume. Its later HTTPS success was therefore excluded as evidence of
automatic test-SSID reconnection; that behavior remains unverified. No saved
user WLAN preference was changed to force reconnection.

### Optional DHCP hostname enrichment

The collector optionally reads `show ip dhcp bindings` and attaches hostnames
from unexpired IPv4 leases to matching authenticated station MACs. Lease
expiry must be positive or `infinity`; malformed addresses, control characters,
overlong names and conflicting names for the same MAC are excluded. Registered
host descriptions are not substituted for DHCP hostnames. Missing DHCP support
is non-fatal, so external-DHCP and AP-mode operation can continue without
names. Collection, matching and unavailable-DHCP fallback have regression tests.

A log-only Titan probe collected 41 usable lease hostnames. The Mac test client
had no usable hostname in that sample; the real controller still showed its
active connection and rates without a hostname. This verifies collection and
the missing-name path, not successful named-client display. Controller ingestion
of an enriched name remains unverified. No DHCP configuration was changed.

### Client power management and rate validation

Client rows now include the native `psm` measurement as `state_pwrmgt` when
NDM explicitly reports `yes` or `no`, and publish HT state from the measured
client mode. Missing or unknown power-saving values are omitted. Rate
conversion retains decimal Mbps precision and omits unavailable, negative,
non-finite or overflowing values instead of fabricating a numeric speed.
Regression tests cover both power states and invalid rate inputs.

The new build was started log-only against the Titan controller. A temporary
Mac connection to the test SSID appeared in the controller's active-client
API with link rates and `powersave_enabled=false`, matching native `psm=no`.
The controller normalizes the power field rather than exposing the raw
`state_pwrmgt` name. Rates varied between samples, so this confirms reporting
and units rather than an exact simultaneous rate match. The test did not
exercise a client entering power save; the true-state path is unit-tested.
The Mac's test preference was removed and Wi-Fi power restored to on, with
Ethernet still active. WLAN application and LED control stayed disabled.

### Physical Ethernet status

The collector now extracts physical `Port` objects from its existing read-only
`show interface` query. Native IDs distinguish separate switches that reuse
local port indexes; sorted IDs provide deterministic one-based controller
indexes. Native labels are retained, including the Titan's port label `0`.
VLAN interfaces and aggregate switch objects are excluded. Unknown link
states are omitted; unknown speed, duplex or negotiation values are not
invented. A down port reports zero speed and no full-duplex link even if NDM
retains earlier negotiation data.

The new build was copied into the private Titan lab and started log-only,
using the existing adopted state. The real controller remained connected and
stored five physical ports in `port_stats`: native labels 1, 2 and 0 were up
at 1000 Mbit/s full duplex with auto-negotiation; labels 3 and 4 were down.
These matched native readback. The API evidence does not prove how all five
ports render in the GUI for the emulated AP model. No WAN/uplink role, PoE
capability, media type or traffic counter is inferred. Native configuration
before and after the telemetry restart matched after removing only terminal
formatting and generated comments. No WLAN apply or LED control was enabled.

## Hero DSL live controller test (2026-11-21)

Verified against UniFi OS Server 5.1.42 / Network 10.5.67 in OrbStack:

- Adoption completed, the controller key persisted, and AES-GCM informs worked.
- The AP remained adopted after restarting the daemon with the saved state.
- Controller provisioning created `UKN-Test-Hero` (WPA2) on
  `WifiMaster0/AccessPoint6` and `WifiMaster1/AccessPoint6`, joined to Bridge0.
- Post-apply WLAN and controller checks passed and cleared the recovery journal.
- The serving radio and primary AP configuration blocks had identical hashes
  before and after provisioning. Channels remained 12/40 MHz and 36/80 MHz.
- Adding controller WLAN IDs/device names to VAP telemetry removed UniFi's
  incorrect "not broadcasting" message.

The daemon was returned to log-only mode after provisioning. The test SSID
remains active in running configuration; no startup-config save was issued.
The user confirmed client connectivity on the test SSID. Two associated test
clients were subsequently observed in NDM and UniFi, including matching MACs,
5 GHz association, signal, channel width, IP address, rates and throughput.
macOS redacted SSIDs from its status scan, so that
scan was not treated as proof of beacon visibility.

Connected testing must use `-wlan-test-only` with explicit non-primary slots.
It excludes all shared-radio changes and unrelated WLANs and requires the
selected SSID names to start with `UKN-Test`. See `testing/hero-slots.json`.

### Lifecycle follow-up

- Pausing the test WLAN in UniFi removed its WLAN entries from system_cfg.
  The test planner now treats absent mapped entries as `down` commands, and
  refuses to touch mapped slots containing any non-`UKN-Test` SSID.
- Applying that paused plan disabled only AccessPoint6 on each band; both
  primary serving APs stayed up. Resuming restored both test APs successfully.
- Restarting `uosserver` caused temporary inform EOF errors. The daemon
  automatically resumed encrypted informs after the controller recovered;
  no re-adoption was required.
- `go test -race ./...` passed after these changes.

- A final daemon restart preserved the adoption key hash and resumed normal
  informs. The daemon was left in log-only mode with both test APs up. Primary
  serving AP/radio configuration hashes still matched the pre-test snapshot.
- The two test clients were not associated after the pause/resume cycle;
  automatic client reconnection was not counted as a passing test.

Safe connected-test invocation on the Hero (after copying the MIPS binary
and mapping file):

```sh
ukn -url http://MAC_WIFI_IP:8080/inform \
  -state /tmp/ukn-hero-state -slots /tmp/hero-slots.json \
  -wlan-test-only -apply
```

Omit `-apply` for log-only operation. The test state above is temporary;
production state belongs under `/opt/var/lib/unifi-keenetic`. Do not use the
unrestricted apply mode while relying on the AP's own Wi-Fi for management.

### Security feature follow-up

The planner accepts explicitly open WLANs (`aaa.N.security=open` or `none`,
without WPA/PSK settings), and maps Enhanced Open to `encryption owe` with
mandatory PMF when `wpa3.support=enabled`. Ambiguous security, WEP and EAP
remain rejected; unsupported modes are never treated as open. Personal
WLAN plans explicitly turn off OWE. WPA2 also accepts valid 64-hex raw PSKs;
raw PSKs are not accepted for SAE.

Security translation and inverse commands have regression coverage. The
`-rollback-probe-mode owe` probe sets OWE/PMF on an unused disabled AP slot,
verifies the flags while the slot is still down, forces rollback, and compares
the entire interface block to its preimage. It never enables or bridges the
probe slot. No open or OWE client association has been tested.

On Hero DSL 5.1.5, this OWE probe passed on AccessPoint5 of both WifiMaster0
and WifiMaster1. Each slot remained disabled and its complete configuration
block was restored exactly. Serving primary AP and radio configuration hashes
matched the pre-test baseline. The running management daemon was left unchanged
in log-only mode; these tests did not publish an open or OWE SSID.

```sh
ukn -rollback-probe WifiMaster0/AccessPoint5 \
  -rollback-probe-mode owe -state /tmp/ukn-security-probe-state
```

This command refuses primary APs, active slots and slots with an existing
SSID. It is a live configuration/rollback test, not a read-only probe.

The `wpa3` and `transition` probe modes use the real WLAN translator with a
random temporary passphrase. They retain only commands for the unused,
disabled AP and exclude bridge membership and all radio changes. Verification
requires encryption and WPA3 enabled, legacy WPA and OWE disabled, plus
mandatory PMF with WPA2 disabled for WPA3-only, or optional PMF and WPA2
enabled for transition mode. The key is never printed. Each probe forces a
health failure and checks that the entire native interface block is restored.

On 2026-11-21, all four combinations (WPA3-only and transition on each band)
passed on Hero DSL 5.1.5, using WifiMaster0/AccessPoint5 and
WifiMaster1/AccessPoint5. These prove native configuration and rollback,
not SAE authentication or roaming with a connected client. The management
daemon remained in log-only WLAN mode with version-only upgrade reporting.
Serving radio and primary AP configuration hashes matched the saved baseline,
and controller informs continued after the probes.

```sh
ukn -rollback-probe WifiMaster0/AccessPoint5 \
  -rollback-probe-mode wpa3 -state /tmp/ukn-personal-probe-state
# Use -rollback-probe-mode transition for the mixed WPA2/WPA3 probe.
```

Build for this device:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -o ukn ./cmd/ukn
```

`sh tools/build-keenetic.sh` assembles `dist/keenetic/` with the binary,
installer, configuration and Entware init script. Copy that directory to
the router and run `sh install.sh`. Installation runs a read-only probe,
preserves existing configuration and leaves the service disabled by default.
Set `INFORM_URL` and `ENABLED=1` in `/opt/etc/unifi-keenetic.conf`, then use
`/opt/etc/init.d/S99unifi-keenetic start` (or `stop`).

Inspect a saved controller configuration without accessing the router:
`ukn -plan system_cfg.txt -slots slots.json`. The slots JSON maps WLAN
indexes to AP interfaces, for example `{"1":"WifiMaster0/AccessPoint0"}`.
Credential commands are redacted in this output.

Run `./ukn -probe` on the router for a read-only status collection. Probe
output includes network addresses and client identifiers; treat it as private.

`./ukn -url http://CONTROLLER_IP:8080/inform` runs the inform session.
State defaults to `/opt/var/lib/unifi-keenetic`. The adoption-only SSH
listener defaults to port 2222 and accepts connections only from the
controller IP. It does not execute shell commands. Use `-ssh-listen ''`
to disable it. No controller address is bundled into the source.

## Current limits

This is not yet a finished replacement for the upstream daemon. Basic adoption
and isolated WLAN provisioning have passed the Hero DSL live test. The daemon
reports status and receives/persists controller state. `-apply` opts into
radio, WPA2/WPA3 personal, hidden SSID, PMF, peer isolation and RRM changes
using explicit slots, preimage commands, a private recovery journal, and
post-change checks. A forced-failure rollback probe passed on an unused,
disabled Titan AP slot: the temporary SSID was removed and the slot remained
disabled. Unrestricted provisioning remains untested; live tests intentionally
exclude shared radio settings. Recovery runs at daemon
startup; a separate watchdog also attempts recovery after 65 seconds if
the main daemon has not confirmed. Each journal has a unique identity so
old watchdogs do not undo later transactions. Advanced features remain under development. The Entware service
defaults to log-only mode; explicit action switches are described below.

Explicit radio, WLAN and AAA administrative status values must be `enabled`
or `disabled`. Unknown values reject the plan instead of falling through to
an interface `up` command. Missing values retain the controller format's
existing defaults. This validation is covered by planner regression tests;
it does not require a live radio change.

Before confirmation, native readback must match the requested encryption
flags, PMF mode, hidden-SSID state, peer isolation, RRM, and key presence,
in addition to SSID operation, bridge membership and MAC lists. A driver
acknowledgement alone is insufficient. Regression tests simulate commands
being silently ignored and verify journaled rollback. The shared checker
also passed live WPA3-only and transition probes on the disabled Hero slot
on 2026-11-21. Key contents are not compared or printed by this checker;
successful client authentication remains a separate end-to-end check.

`-model CODE` selects a model from `-models`; the Entware configuration
uses `MODEL`. Fast roaming, VLAN provisioning, guest portal, mesh, scanning,
client actions, QoS and several upstream management features are unfinished.

Fast-roaming enable requests remain unsupported. An explicit
`aaa.N.ft.status=disabled` now produces the per-AP `no ft enable` command;
an absent status preserves native FT state, and an unknown status rejects
the plan. The inverse restores the previous enable flag, and native readback
must confirm that FT is disabled before accepting the transaction. The
mobility-domain ID and over-the-DS setting are preserved. Planner, readback,
idempotence and journaled rollback tests cover this path. The Titan controller
capture contains explicit disabled statuses for both test WLAN entries, but
this change has not been applied live. It does not establish working 802.11r
roaming: the native guide specifies a two-ASCII-symbol mobility domain, whose
translation and interoperability still need verification before enablement.
A log-only controller preview subsequently confirmed that enabling fast
roaming emits only `aaa.N.ft.status=enabled` for these WLANs, without FT
mobility-domain parameters. The original disabled profile was restored and
received again by the agent; no native FT change was applied.

Personal authentication methods are matched as complete `WPA-PSK` and `SAE`
tokens across the four controller key-method entries. Unknown methods,
including FT methods, mark the plan unsupported rather than silently becoming
ordinary WPA2 or SAE. An omitted method retains the existing WPA2 fallback
for controller formats that specify `wpa=2` and a personal key without a
method entry. This parser validation has regression coverage.

NDM execution currently uses `ndmc -c`. WLAN keys are redacted from logs
and stored in private files, but can briefly appear in local process
arguments during application and recovery. The experimental terminal runner
(`-terminal-probe`) requires a native KeeneticOS login and is not used for
application. Entware root access alone does not authenticate that runner.

The collector currently expects Bridge0, WifiMaster0 and WifiMaster1 as
found on the tested Titan. Other models and interface layouts need discovery
and testing. Do not assume a different Keenetic uses the same CPU target.

CLI reference: [KeeneticOS 5.1 Titan command guide](https://storage.googleapis.com/docs.help.keenetic.com/cli/5.1/en/cli_manual_kn-1810.pdf).
The planner uses documented `interface … channel`, `channel width`, `ssid`,
`authentication wpa-psk`, and `encryption` commands. Interface slot mappings
are explicit; the planner does not infer ownership from existing SSIDs.
## WLAN schedule development

Schedule-enabled WLAN requests use native weekly timers in the normal guarded
application path. Schedule changes and WLAN security/bridge changes share one
journal and quiesce barriers. A redacted `-plan` preview includes the compiled
native schedule. Unsupported settings still reject the entire plan; scheduled
WLANs must not silently become always-on WLANs. Native probe results below are
separate from the real controller roundtrip results recorded below.

The compiler translates `wireless.N.schedule_*` into native weekly
`schedule NAME action start|stop MIN HOUR DOW` actions, with Sunday numbered
zero. Overlapping windows are merged before transitions are generated;
matching times on multiple days share one native action. Overnight windows
carry into the following day, including Saturday into Sunday. `2400` is valid
only as an end time. The controller's `2359` end-of-day sentinel also means
midnight, so `0000-2359` covers a full day without an invented one-minute gap.
Other equal start/end times are empty windows. Normal windows allow airtime;
`schedule_invert=enabled` makes them blackout windows instead. An enabled
non-inverted schedule with no windows is a valid full-week blackout, not an
always-on WLAN. Invalid times/days/indexes are refused.

Legacy controllers may repeat unindexed keys such as
`wireless.1.schedule_sun=0000-1100` and
`wireless.1.schedule_sun=1200-2359`. Both full and test-mode planning normalize
these windows before map parsing can discard duplicates. Indexed windows are
also supported; mixed indexed/unindexed encodings for one day, duplicate
indexed keys, and more than 32 legacy windows per day are rejected.

Compilation and weekly-state evaluation have automated coverage. Native
creation, grouped weekday action syntax, action removal, and deletion were
checked using an unbound test schedule on Hero DSL; no AP was bound to it.
The test object was removed afterwards. Later binding and transition probes
are described below; these initial resource-only checks did not establish
operational WLAN scheduling.

Unbound schedule resources now have a separate transaction layer. Names are
derived from the explicit AP target, existing objects require the exact
backend ownership marker, and any reference from an AP or another service
blocks a resource-only update. New-resource inverse commands are conditional
on existence, so recovery also works when a crash preceded creation. The
ownership marker is retained until deletion succeeds. Recovery refuses a
resource whose marker or references have changed. Existing owned definitions
are updated by action deltas; unrelated schedules are preserved.

Tests cover each forward-command crash boundary for both creation and updates,
immediate rollback, unchanged definitions, collisions, references, and invalid
recovery commands. A native `-rollback-probe-mode schedule` test creates only
an unbound resource, verifies its marker/actions, and forces rollback. It does
not apply the WLAN commands used to compile the draft and never binds an AP.
It retains the standard unused/disabled/no-SSID AP-slot prerequisites.

On 2026-11-22, the unbound-resource probe passed on Hero DSL 5.1.5: creation,
ownership/action readback, forced rollback, and deletion. An independent
`-crash-probe` run terminated the probe after verification; the detached
watchdog subsequently removed the resource and its private journal. Empty
native schedules were also checked to remain visible in configuration
readback before their marker/actions are added. Serving radio/primary AP
configuration hashes matched the baseline and controller informs continued.
No AP was bound to any of these test schedules.

A separate `-rollback-probe-mode schedule-binding` development probe binds a
new owned schedule to an unused, disabled AP1–AP6 with no SSID and no existing
schedule. It accepts only stop actions, so the schedule cannot enable the AP.
It verifies the native binding and disabled status, then deliberately rolls
back. Recovery detaches the AP before the guarded resource inverses run; a
journal written before any forward command is also recoverable. Tests exercise
every forward boundary and reject serving, primary, reserved, or enabled slots.
This development probe accepts only stopped schedules. Normal provisioning
uses the coupled lifecycle and health checks described below.

On 2026-11-22 the stopped-binding probe passed on both Hero DSL 5.1.5 bands
(`WifiMaster0/AccessPoint5` and `WifiMaster1/AccessPoint5`), including exact
interface restoration after forced rollback. A deliberate process crash on the
2.4 GHz slot also recovered through the detached watchdog: binding, resource,
and journal were removed. The primary radio/AP blocks and both test-slot
blocks matched the saved preimage byte for byte; controller informs continued.
These results cover disabled binding and recovery, not active timed broadcasts.

`PrepareScheduleTransition` now prepares creation, replacement, pause, schedule
removal, and WLAN removal. It detaches an
old owned binding before quiescing the AP, updates only an unreferenced owned
resource, then binds and administratively enables the AP. The schedule gates
its operational state independently; leaving an off-window AP administratively
down would prevent its next scheduled start. Paused/deleted
WLANs lose their binding so an old timer cannot restart them. Removed schedules
remain as dormant owned resources; they are not deleted or left attached.
Unchanged transitions generate no commands. Foreign bindings and schedules
shared with another service are refused. Tests recover each forward-command
boundary for creation, replacement, pause, and both removal scenarios and verify exact preimage recovery.
Normal provisioning now couples this lifecycle to security/bridge changes:
all affected APs are detached and quiesced before changing any timer or WLAN
setting, and binding/enablement happens last. Recovery reverses that order,
restoring timer actions and credentials before original bindings/enablement.
Tests cover every crash boundary for creation, timer replacement with security
changes, and security changes with an unchanged timer. Immediate rollback stops
at its first failed inverse so it cannot re-enable an incompletely restored AP;
the retained journal supports an ordered retry.

`RouterCivilTime` validates native `show clock date` fields, calendar dates and
weekday agreement, rejecting unset clocks. It carries the router-local calendar
without converting it through the agent's timezone or the Mac's clock.
Enabled schedules require a valid native clock before any mutation. Pausing a
WLAN does not depend on a valid clock. Health checks bracket status collection
with native clock reads and retry if a timer boundary was crossed. They check
the scheduled operational state separately from administrative enablement,
verify binding/action readback, and reject residual bindings on paused or
unscheduled WLANs. A legitimate off window may leave no active mapped APs;
controller communication must still pass. Full-mode planning now disables
explicitly mapped WLANs missing from controller configuration, as test mode
already did, so deletion cannot leave a bound timer behind.
If the health-check inform receives a newer controller configuration, the
current transaction rolls back and the newer version is planned next; it is
not accidentally marked as already applied.

The experimental `schedule-active` and `schedule-start` rollback probes create
an isolated `UKN-Test-Timed-Probe` AP with a random WPA2 password. They require
an unused, unaliased, unbridged, down/no-SSID slot and a new schedule resource.
They apply no bridge or shared-radio commands. The native clock must show
seconds 20–40 at entry, leaving time to observe the next-minute transition
within the watchdog deadline. `schedule-active` observes a native timed stop;
`schedule-start` observes a native timed start. Both force rollback afterwards
and require exact interface restoration and resource removal. They do not
support crash injection and do not verify client authentication or traffic.

On 2026-11-22, Hero DSL 5.1.5 `WifiMaster0/AccessPoint5` passed the native timed
stop and timed start probes, with WPA2 configuration readback and exact slot
restoration after each successful test. An initial start probe correctly rolled
back when the AP was left administratively down: a native schedule does not
override administrative disablement. Keeping scheduled WLANs administratively
up fixed the test; the native engine then gated operational down/up state at
the expected router-local minute. This result is currently limited to the
2.4 GHz test slot; it does not prove normal controller schedule provisioning.

The integrated `ApplyPlanWatched` path subsequently passed both native timed
start and stop probes on `WifiMaster1/AccessPoint5` (5 GHz), including security
readback, native binding/administrative-state verification, clock-bracketed
operational collection before and after the transition, and exact forced
rollback. Primary radio/AP and both test-slot configuration blocks matched
the baseline afterwards; the journal and temporary schedule were removed,
and controller informs continued from the separate log-only daemon. At that
stage the live daemon had not yet been switched to the new schedule build.

The subsequent real Network 10.5.67 controller test exposed two compiler bugs:
the controller emitted repeated unindexed allowed-window keys, and the initial
implementation interpreted the invert flag backwards. The corrected backend
preserves every repeated window and treats normal windows as allowed airtime.
The controller's full-week blackout is enabled with no allowed windows; it now
compiles to a constant native stop. The controller generator also uses `2359`
as an end-of-day sentinel, now handled without a one-minute nightly outage.
Tests include the captured legacy encoding, full blackouts, indexed inversion,
daily boundaries, and ambiguous/excessive window rejection.

On 2026-11-22 the corrected daemon applied the controller's full-week blackout
to `UKN-Test-Hero` on both AP6 bands and confirmed native readback/controller
health. Both test APs were operationally down while the primary AP0 remained
up. A real controller one-minute Sunday blackout then generated allowed
windows `0000-1057` and `1058-2359`, plus full-day windows on the other days.
Native observations showed both AP6 bands up before 10:57 router-local time,
down throughout the 10:57–10:58 blackout, and automatically up again at 10:58.
The Mac's association was verified on primary `WifiMaster0/AccessPoint0` before
testing; no shared-radio or primary-WLAN settings were included in application.
Controller pause then removed both native schedule bindings and disabled both
test APs; controller resume reattached the timers and brought the test APs up.
Restoring the original schedule-free profile removed the bindings again and
passed guarded confirmation. An API comparison found no unrelated WLAN field
changes. The now-unbound, backend-owned test schedule resources were explicitly
removed after stopping the applying daemon. The corrected build was restarted
in log-only mode with version-only upgrade reporting still enabled. Client
reconnection after a timed outage was not exercised by this test.
Final native radio, primary/test AP, and home-bridge configuration blocks
matched the pre-test baseline byte for byte, and controller informs resumed
from the corrected log-only build.

```sh
ukn -rollback-probe WifiMaster0/AccessPoint5 \
  -rollback-probe-mode schedule -state /tmp/ukn-schedule-resource-state
# Add -crash-probe to test detached recovery; wait for watchdog cleanup.
# Use -rollback-probe-mode schedule-binding for the constant-off binding test.
```

The current live topology is Mac over Wi-Fi → Hero DSL in router mode
(NAT and DHCP) → Huawei 5G gateway on its own LAN. No router/AP mode change
has been made. AP-mode operation remains unverified; changing mode requires
a separate wired maintenance test and rediscovery of the management address.

## WMM

Explicit `wireless.N.wmm=enabled` and `disabled` requests map to per-AP `wmm`
and `no wmm`. An absent request preserves the native setting; unknown values
are rejected before application. The controller's normal configuration sends
`enabled` on both test WLANs, while their old native AP6 configurations had no
WMM flag. Only explicitly mapped APs are changed, including in connected test
mode; no shared-radio command is generated. The inverse restores the native
flag, unchanged settings generate no delta, and confirmation verifies readback.

`-rollback-probe-mode wmm` tests enablement on an unused disabled/no-SSID
non-primary slot, verifies that it stays down, then forces rollback and checks
exact interface restoration. This setting does not implement U-APSD, DTIM,
minimum-rate policy, or traffic shaping; traffic-priority behavior still needs
an associated-client test.

Both disabled AP5 band probes passed on Hero DSL 5.1.5 on 2026-11-22. A fresh
controller provisioning request confirmed `wireless.1.wmm=enabled` and
`wireless.2.wmm=enabled`. The first scoped apply lost its controller connection
during the health check and correctly rolled WMM back to its original state.

Transient network/deadline failures and HTTP 408/429/5xx responses now schedule
an apply retry for the same controller version after one minute, doubling to
a five-minute cap. A successful inform is required before starting an attempt.
New controller versions bypass old backoff; success clears it. Unsupported
settings, ownership/recovery failures, and other permanent errors are not
automatically retried. Tests cover retry classification, deadlines, backoff,
and version changes. Even an empty delta cannot be confirmed while an old
rollback journal remains; recovery must finish first.

The corrected build subsequently passed guarded provisioning against the real
controller on both AP6 test WLANs: native readback showed `wmm` and `up`,
controller health passed, and the journal was cleared. Comparing complete
configuration settings, excluding generated comments and only the two intended
AP6 WMM flags, matched the baseline exactly. WMM was left enabled on the test
WLANs to honor their controller configuration. The daemon was then returned to
log-only mode. The retry/backoff policy was subsequently exercised against
the real controller as described below.

## Controller outage recovery

`tools/inform-fault-proxy.py` is a temporary test relay for the standard Go
`HTTP_PROXY` setting. It accepts encrypted inform packets only from one
configured client and only for the exact configured HTTP inform URL. It
forwards packets unchanged to the real controller, injects HTTP 503 for one
selected request, logs only request counts/statuses, and stops after a bounded
lifetime. `--fail-request 0` runs without injection for recovery. It does not
decrypt packets, record credentials, change adoption state, or change the
saved inform URL. Use it only with a separately mapped test WLAN and a
nonempty guarded configuration delta; otherwise request 2 may be an ordinary
inform rather than the post-change health check.

On 2026-11-22, the Hero DSL used this relay to reach the real UniFi OS test
controller. Both test AP6 WMM flags were temporarily cleared while the agent
was stopped. The first inform reached the controller successfully. The next
controller health-check request received the injected HTTP 503: the agent
rolled both flags back to their immediate preimage, cleared its journal, and
scheduled a one-minute retry. Both test APs remained up during backoff.

The relay then became unreachable and expired, so successful controller
communication was unavailable past the initial retry deadline. A pass-through
relay was started, without restarting the agent or issuing a new controller
configuration. The same agent process automatically retried, restored WMM on
both test APs, passed the real controller health check, and cleared the journal.
The saved controller configuration version matched before and after the test.
All native settings matched the original pre-test baseline after excluding
generated comments. The relay was stopped, and the agent returned to direct,
log-only operation with no proxy environment and its saved inform URL intact.

## Minimum RSSI

The controller's `stamgr.N.radio` selects a band (`ng` or `na`), and
`minrssi.status=true` with `minrssi.rssi` sets native `rssi-threshold` only
on explicitly mapped AP slots on that band. Negative values are dBm; positive
values use the upstream controller conversion (value minus 95). Zero or an
explicit per-band disable clears the threshold. Global `stamgr.status=disabled`
clears thresholds on all mapped APs, overriding stale per-band entries.
Absent requests preserve native thresholds. Invalid ranges or ambiguous
band entries are refused. Load balancing/max-client requests remain unsupported.

The native AP setting controls client rejection/disconnection; the daemon
does not run a separate timed client-kick loop. It records the old threshold
for rollback and verifies the new value before confirming. Connected Wi-Fi
test mode still excludes shared-radio commands and all non-test AP slots.

On 2026-11-21, a -75 dBm command, native readback, forced failure, and exact
configuration restoration passed on both Hero DSL AccessPoint5 slots while
they remained disabled. Client enforcement and a controller UI round trip
remain unverified. `-rollback-probe-mode rssi` runs that disabled-slot probe.

## Regulatory country changes

The controller's numeric `radio.N.countrycode` is mapped through the upstream
ISO table to native `interface WifiMasterN country-code XX`; a per-radio code
takes precedence over `radio.countrycode`. Unknown codes prevent application.
Country requests are completely excluded by `-wlan-test-only`.

Changing a domain requires an explicit native country setting in running-config,
an administrative-state preimage, and explicit desired channel and width.
NDM documents no `no country-code` command, so an implicit firmware default is
not guessed as an inverse. The transaction first pauses the radio and selects
20 MHz/automatic channel, changes country, applies the requested channel/width,
and finally applies the requested administrative state (or restores the old one).
Recovery returns to the neutral state before restoring the old country and its
channel/width. Confirmation requires matching native country readback.

Offline tests cover code mapping, rejection, disabled-radio preservation, and
recovery at every forward-command crash boundary under a model that rejects
channels invalid in its current domain. This is not native hardware validation
of a regulatory transition. No live country change has been attempted while
the Hero carries the management Wi-Fi connection; wired management is required
for that test.

## VLAN WLAN binding

The Entware service exposes `APPLY`, `WLAN_TEST_ONLY`, `LED_CONTROL`, `SLOTS`,
and `NETWORKS` in `/opt/etc/unifi-keenetic.conf`. All action switches default
to `0`. `APPLY=1` requires an explicit readable `SLOTS` JSON file. While using
the device's Wi-Fi for management, keep `WLAN_TEST_ONLY=1`, use only the
dedicated test slots, and leave `NETWORKS` empty. `LED_CONTROL=1` is an
independent opt-in for device-wide LED on/off; it is not restricted by WLAN
test mode. Restarting the service loads the edited settings.

The optional `-networks` file maps UniFi VLAN IDs to **existing**, operator-
provisioned Keenetic bridges. For example, `{"77":"Bridge2"}` maps VLAN 77
to Bridge2. Create and verify that bridge and its tagged Ethernet uplink
separately, with wired management available. The daemon does not configure
switch ports, create VLAN interfaces or bridges, start DHCP, or change routing.
It cannot validate the external switch or prove that the operator's Bridge2
really reaches VLAN 77.

Preview a saved controller configuration before enabling application:

```sh
ukn -plan system_cfg.txt -slots slots.json -networks networks.json
```

The controller must supply a matching `vlan.N.id`/`devname`, tagged bridge
name, tagged uplink member, and explicit WLAN device membership. Partial or
ambiguous configurations are refused. Each VLAN needs a distinct native
bridge; Bridge0 is prohibited as a tagged-network mapping. With VLAN mappings
enabled, every WLAN needs an explicit controller bridge membership so an
incomplete request cannot silently land on Home.

The native bridge and AP slot must already exist. A slot already belonging to
another bridge is refused, even when applying a change from tagged to untagged;
move it deliberately during separate network maintenance. Newly added bridge
membership has an inverse in the rollback journal. Confirmation checks actual
bridge membership in addition to the SSID and AP state. This verifies the
local binding, not end-to-end VLAN traffic or isolation.

VLAN mappings cannot be used with `-wlan-test-only`. This feature has automated
plan, rejection, inverse, and membership-health coverage; no live VLAN change
has been made on the Wi-Fi-connected Hero DSL.

## WLAN MAC allow/deny lists

The native planner translates the controller's `macacl` section by WLAN
device name, not by assuming its indexes equal WLAN indexes. `allow` maps to
Keenetic's `permit`; `deny` maps directly. Empty permit lists intentionally
deny all clients; empty deny lists disable filtering. An absent global ACL
request preserves native rules. An explicit disabled request clears filtering
and its address list on the mapped AP.

NDM requires addresses to be registered as Keenetic known hosts. The daemon
checks the running configuration before making changes and refuses missing
registrations; it does not register, rename, or delete hosts automatically.
This also checks addresses needed for rollback. Normal `-apply` and explicit
WLAN slots are required. Connected test mode only targets its dedicated test
slots, even when the controller supplies ACLs for other WLANs.

Changed lists briefly pause an active mapped AP while policy and address
entries are replaced, then restore its administrative state before continuing
the WLAN plan. Unchanged lists do not pause it. Each change is journaled with
its inverse, and confirmation checks the resulting policy and complete list.
Only MAC lists are supported here; controller-wide block/kick actions remain
unimplemented.

On 2026-11-21 a native deny-list probe on Hero DSL's unused, disabled
WifiMaster0/AccessPoint5 passed verification and forced-failure rollback.
The complete AP configuration matched its preimage afterwards. A temporary
test-host registration was created for that test and removed afterwards.
The final binary also rejected the unregistered address before application.
Serving radio/primary AP configuration hashes matched the saved baseline,
and the existing log-only daemon continued exchanging informs.
The probe never enabled the AP. This proves native command/rollback behavior,
not enforcement against an associated client or a controller UI round trip.
`-rollback-probe-mode acl` uses test MAC `02:00:00:00:00:fe` and requires that
test address to be registered before running; the probe itself never changes
the known-host registry.

## Controller firmware requests

The native session disables the upstream emulator's simulated reboot state
changes. Both top-level firmware requests and `cmd upgrade` are refused by
default. `-report-upgrade-version` instead reports the offered version and
saves it in private state; it remains reported across restarts. Disabling the
option refuses future updates without undoing the last reported version.
In the Entware service, set
`REPORT_UPGRADE_VERSION=0` to refuse or `1` to report only. Neither mode
downloads an image, installs firmware, or moves the boot clock.
Reboot/restart requests reach the backend action
allowlist, where they remain unsupported; they cannot be enabled by
`-led-control`. This behavior has automated coverage. No live firmware or
reboot test was performed on the Wi-Fi-connected Hero.
Regression tests cover top-level `reboot` and both `cmd reboot`/`cmd restart`:
all preserve the physical session's boot clock while the upstream emulator
continues simulating its reboot. The top-level path previously missed that
guard; it is now fixed. No native reboot command is enabled by this change.

## Optional controller LED actions

`ukn -led-control` enables the controller's LED on/off requests independently
of WLAN provisioning. It changes the device-wide LED shutdown setting using
`system led power shutdown all` and `system no led power shutdown`; it does
not save startup-config. Leave this flag absent for a fully log-only daemon.
Existing LED schedules still apply. Locate/blink, reboot, and client actions
remain rejected.

On 2026-11-21, both native LED commands were exercised on the Hero DSL and
the original non-shutdown state was restored. Automated tests cover the
encrypted controller reply through to the NDM command allowlist. The updated telemetry daemon
also exchanged informs successfully in log-only mode, but the controller
initially displayed configured widths of 20/40 MHz while native measurements
were 40/80 MHz. This was subsequently fixed by reporting the native operating
width in each `vap_table[].bw` entry, in addition to the radio measurements.

On 2026-11-22 a real Network 10.5.67 controller API override was tested with
WLAN application disabled and `-led-control` temporarily enabled. Override
`off` produced `led:off` and native `led power shutdown all`; override `on`
produced `led:on` and removed that shutdown setting. The original controller
override (`default`) was restored and the controller still reported connected.
Native configuration settings matched their baseline after excluding generated
comment metadata. This verifies the controller/inform/native configuration
roundtrip, not a visual observation of the physical lights. The corrected
daemon was returned to fully log-only mode with LED control absent.
On 2026-11-21, the UniFi OS test controller's device overview showed channel
12 at 40 MHz and channel 36 at 80 MHz, matching native measurements, with
the daemon still in log-only WLAN mode. Configured width preferences and
operating measurements remain separate; no radio setting was changed.
