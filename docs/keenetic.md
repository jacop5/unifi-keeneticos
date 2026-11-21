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

## Optional controller LED actions

The native session disables the upstream emulator's simulated upgrade and
reboot state changes. Both top-level firmware requests and `cmd upgrade`
are rejected without downloading an image, adopting the requested version,
or moving the boot clock. Reboot/restart requests reach the backend action
allowlist, where they remain unsupported; they cannot be enabled by
`-led-control`. This behavior has automated coverage. No live firmware or
reboot test was performed on the Wi-Fi-connected Hero.

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
continued to display configured widths of 20/40 MHz while native measurements
were 40/80 MHz; do not treat the width display as verified.
