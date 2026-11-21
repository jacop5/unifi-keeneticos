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

## Optional controller LED actions

`ukn -led-control` enables the controller's LED on/off requests independently
of WLAN provisioning. It changes the device-wide LED shutdown setting using
`system led power shutdown all` and `system no led power shutdown`; it does
not save startup-config. Leave this flag absent for a fully log-only daemon.
Existing LED schedules still apply. Locate/blink, reboot, and client actions
remain rejected.

On 2026-11-21, both native LED commands were exercised on the Hero DSL and
the original non-shutdown state was restored. Automated tests cover the
encrypted controller reply through to the NDM command allowlist. A real
controller UI LED round trip remains unverified. The updated telemetry daemon
also exchanged informs successfully in log-only mode, but the controller
initially displayed configured widths of 20/40 MHz while native measurements
were 40/80 MHz. This was subsequently fixed by reporting the native operating
width in each `vap_table[].bw` entry, in addition to the radio measurements.
On 2026-11-21, the UniFi OS test controller's device overview showed channel
12 at 40 MHz and channel 36 at 80 MHz, matching native measurements, with
the daemon still in log-only WLAN mode. Configured width preferences and
operating measurements remain separate; no radio setting was changed.
