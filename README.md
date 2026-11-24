# unifi-keeneticos

Experimental KeeneticOS/Entware port of [antoniocosta/unifi-openwrt](https://github.com/antoniocosta/unifi-openwrt), making a Keenetic access point appear in UniFi Network as a UniFi AP.

This fork adds the Keenetic backend and the `ukn` daemon. It reuses the upstream UniFi inform protocol, adoption/session code, and model catalogue. **It is not feature-equivalent to the OpenWrt implementation and is not production-ready.**

README rewritten for this fork by the unifi-keeneticos contributors on **2026-11-21**.

## AI slop disclaimer

Much of this fork's Keenetic-specific code and documentation was generated with OpenAI Codex. It has had automated checks and limited live testing, but **has not undergone a comprehensive independent human review**. Expect bugs, incomplete features, questionable assumptions, and documentation that may overstate what works. Passing tests are not proof that it is safe for your network: inspect the code, back up your configuration, and use a spare router with independent management access before trusting it.

This disclaimer applies to the changes in this fork, not to the original upstream project or its authors.

## Credits

The original project is **[unifi-openwrt](https://github.com/antoniocosta/unifi-openwrt)** by **[@antoniocosta](https://github.com/antoniocosta)** and its contributors. The starting upstream revision is [a211831](https://github.com/antoniocosta/unifi-openwrt/commit/a211831). Credit for the original protocol implementation, OpenWrt backend, documentation and test infrastructure belongs to that project.

Keenetic port and integration testing are maintained in [jacop5/unifi-keeneticos](https://github.com/jacop5/unifi-keeneticos). The original OpenWrt executable and backend remain in the source tree; their documentation and test results must not be interpreted as Keenetic support claims.

Upstream also credits:

- [James Braid's unifi-emu](https://github.com/jamesbraid/unifi-emu): model catalogue data and protocol documentation, MIT.
- [Luca Becker's open-unifi](https://github.com/lucavb/open-unifi): protocol documentation and measured AP capability data, MIT.
- Other research and related projects listed in [NOTICE](NOTICE).

Their existing notices and the [model catalogue's MIT notice](internal/models/NOTICE) are retained. To support the original author, see [the upstream project](https://github.com/antoniocosta/unifi-openwrt).

## Verified hardware

| Device | KeeneticOS | Go target | Verified |
| --- | --- | --- | --- |
| Hero DSL KN-2410 | 5.1.5 | linux/mips, softfloat, **big-endian** | Adoption, encrypted informs, telemetry, isolated WPA2 WLAN provisioning, client connectivity, pause/resume, daemon and controller restart recovery |
| Titan KN-1810 (tested unit) | 5.1.9 | linux/mipsle, softfloat, **little-endian** | Adoption/reconnection, isolated WPA2 provisioning and pause/resume, 2.4/5 GHz client traffic, rollback/watchdog recovery, Ethernet telemetry and private service lifecycle tests |

Live Hero and Titan testing used **UniFi OS Server 5.1.42 / UniFi Network 10.5.67** in OrbStack. Do not infer CPU byte order from `uname -m`: both tested routers report `mips`. Verify the actual binary format/Entware architecture for your device. Other models, layouts and firmware versions are unverified.

See the prioritized [roadmap](docs/ROADMAP.md) for remaining work, dependencies and acceptance gates. Run `python3 tools/check-release.py --require-clean` on a committed candidate for offline tests, both MIPS packages, architecture/manifests and vendored-source rebuild checks. Reports/logs go to `dist/validation/`; dependencies must already be cached. This command performs no deployment.

## Feature status on Keenetic

| Feature | Status |
| --- | --- |
| Adoption, saved controller key, AES-GCM informs | Verified live on Hero and Titan; retained across agent restart; controller restart/reconnection verified on both |
| SSID, WPA2 personal, LAN bridge membership | Verified live on explicit non-primary AP slots |
| Hidden SSID | Native hiding flag and explicit 2.4 GHz WPA2 client join with DHCP, Wi-Fi-bound DNS/HTTPS and restoration verified; passive beacon/scan behavior unverified ([report](docs/band-hidden-validation-2026-11-24.md)) |
| WLAN pause/resume | Verified live; controller omission of paused WLANs handled in test-only mode |
| Client authentication, DHCP and internet access | Titan 2.4/5 GHz Mac authentication, router-issued IPv4 leases and Wi-Fi-bound HTTPS verified; automatic reconnection remains unverified |
| Client IP, band, signal, width, rates and throughput | Observed in UniFi and compared with NDM data |
| Client names, counts and power-save status | Titan controller API count consistency, DHCP hostname equality and active power-state reporting verified; sleeping-client transitions untested |
| Active-host name/IP fallback | Implemented with ARP/DHCP precedence and offline/conflict filtering; external-DHCP/AP-mode live validation outstanding |
| Uptime, memory, CPU and radio/client telemetry | Implemented; basic live reporting verified, not every field is complete |
| Physical Ethernet link status | Titan's five ports received by the controller with measured link speed/duplex; no port configuration or inferred uplink roles |
| Private rollback journal, inverse commands, watchdog, startup recovery | Implemented; immediate failure and detached crash recovery verified on Titan |
| Controller outage recovery | Real injected HTTP 503 rolled back; same-version retry confirmed after controller connectivity returned |
| WPA3 personal, transition mode and PMF | WPA3-only and WPA2/WPA3 transition-mode guarded provisioning with 5 GHz Mac WPA3 authentication verified on Titan; WPA2-only clients in transition mode remain untested |
| Peer isolation, per-AP RRM | Implemented and unit-tested; not comprehensively tested live |
| Explicit open / Enhanced Open (OWE), 64-hex WPA2 keys | Translation and rollback unit-tested; OWE/PMF commands verified only on disabled Hero slots, not with an associated client |
| Per-WLAN MAC allow/deny lists | Native list/rollback verified on disabled Hero slot; empty allow-list blocked a Titan test-client join and removing it restored authentication; individual-entry enforcement untested |
| Minimum RSSI | Per-band requests map only to owned AP slots; native threshold/rollback verified on disabled Hero slots on both bands; client enforcement not yet verified |
| WMM | Controller provisioning and native readback confirmed on both Hero test bands; disabled-slot rollback verified; traffic-priority behavior not measured |
| WLAN schedules | Native weekly timers and guarded recovery; controller blackout, automatic stop/start, and pause/resume verified on both Hero test bands |
| Radio channel, width and administrative state | Translation, explicit configuration readback and rollback tested offline; excluded from live main-router tests |
| Regulatory country | Implemented with neutral radio transition and exact country inverse; offline tests only, excluded from connected Wi-Fi tests |
| Automatic client reconnect after WLAN pause | Latest Titan/Mac test selected a main AP0 network instead of returning to test AP6; automatic return to the test WLAN remains unverified |
| Idempotent provisioning | SSID normalization verified; Titan receipt test confirmed two initial writes followed by zero prepared changes and two credential-record reuses after restart; native drift validation remains pending ([report](docs/credential-receipt-validation-2026-11-24.md)) |
| VLAN WLAN binding | Implemented with explicit `-networks` mappings to existing native bridges; automated tests only, no live VLAN traffic verification |
| Fast roaming | Explicit disable implemented; experimental opt-in for matched dual-band WPA2 with explicit native domain, guarded AP6 provisioning and 5 GHz WPA2 client authentication with FT enabled verified on Titan; actual FT reassociation/roaming unverified |
| BSS transition (802.11v) | Enabled requests remain unsupported; per-AP 802.11k is not equivalent; read-only Wi-Fi-system membership diagnostic verified ([findings](docs/wifi-system-investigation-2026-11-24.md)) |
| VLAN/uplink creation, mesh, guest policy/portal, scanning, traffic shaping | Not ported; unmapped VLANs and explicit guest WLAN requests are rejected |
| Controller LED on/off | Opt-in with `-led-control`; real controller override → native off/on readback verified on Hero DSL; original override restored |
| Controller reboot | Opt-in native dispatch with test-scope/recovery guards; mock/encrypted-controller tests pass; physical reboot unverified |
| Client block/kick, locate and other controller actions | Not implemented on Keenetic; logged rather than executed |
| Enterprise/WEP security, transmit-power control, advanced management | Not supported by the Keenetic translator |
| DTIM period and U-APSD | No Keenetic translation; ignored setting names reported in plans/logs |
| Minimum data/management/beacon rates and CCK selection | No Keenetic translation; requested setting names reported as ignored |
| Service lifecycle and status | Private Titan start/restart/stop and live/stale/unrelated PID status verified; real-agent installer upgrades/recovery pass in an isolated Linux container; hardware installed-package upgrade remains untested |
| Offline preflight | Saved configuration/rollback preparation tested against a fresh Titan snapshot and followed by guarded AP6 apply |
| Local provisioning status | Private read-only report; log-only/unsupported/applying/confirmed and confirmation retention verified on Titan |
| Executable provenance | `-build-info` verified on Titan with clean revision, architecture and matching binary hash |

Radio telemetry reports native channel widths without guessing antenna or transmit-power capabilities. Radio `ieee_mode` translation selects channel width; it does not force a PHY generation such as 802.11n/ac/ax. UniFi's device overview was verified live at 40 MHz on 2.4 GHz and 80 MHz on 5 GHz, matching the Hero's native measurements. VLAN configurations require explicit bridge mappings and complete membership information; they never fall back to Home. See [VLAN setup](docs/keenetic.md#vlan-wlan-binding).

The latest Titan client run verified counts and hostname ingestion, but its HTTPS check stopped at DNS resolution; earlier successful Wi-Fi-bound HTTPS tests are separate evidence. See the dated [live-test notes](docs/keenetic.md) for the scope of each result.

Known unimplemented DTIM, U-APSD, transmit-power and minimum-rate requests are listed as ignored in plans and logs. Enabled per-client bandwidth limits are also reported by setting name when their controller VAP matches a planned WLAN. JSON reports expose `ignored_settings`; `planner_supported` means there are no blocking unsupported settings, not that every requested setting is implemented. Unknown controller settings are not comprehensively classified. A successful adoption does not establish support for every control exposed by the selected UniFi model. This daemon never installs Ubiquiti firmware.

Keenetic sessions refuse controller firmware upgrades by default. Optional `-report-upgrade-version` acknowledges an upgrade by reporting and saving the offered version instead. Neither mode downloads firmware or simulates a reboot. The service setting is `REPORT_UPGRADE_VERSION=0` (refuse) or `1` (report only). Controller reboot requests are logged by default. Explicit `-reboot-control` (`REBOOT_CONTROL=1`) schedules a native reboot after five seconds and stops the agent cleanly. It is unavailable in WLAN-test mode and refused while this instance has pending/unreadable rollback state. It does not save runtime configuration; reboot loads the native saved startup settings. Reapplication depends on persistent service startup and controller reachability. This opt-in is separate from firmware-update policy and has not been tested by rebooting hardware. The displayed UniFi model/version is a protocol compatibility profile, not the installed Keenetic firmware version.

The controller's configuration version acknowledges receipt, independently of native application. `ukn -provision-status` reads a separate local JSON report with the latest provisioning decision, known ignored/unsupported settings, retry time and last confirmed apply, including its own recorded test scope and ignored settings. Use `-state PATH` for a custom instance. It reads no router/controller data or adoption key. The report is a historical snapshot with timestamps, not a live health check; confirmation covers the mapped plan and its configured test scope, not every controller setting. Older builds have no report and return `unknown` after upgrading until a provisioning decision is recorded.

## Requirements

- KeeneticOS with Entware and a working `ndmc` command.
- Go 1.26 or newer to build.
- Enough Entware storage for a roughly 9–12 MB static binary and private state.
- A controller reachable from the AP on TCP 8080; adoption SSH defaults to TCP 2222 on the daemon.
- The current collector assumes `Bridge0`, `WifiMaster0` and `WifiMaster1`, as found on the tested units.

Use **UniFi OS Server** for a new self-hosted test controller. The former standalone Network Server Docker stack is retained under `testing/controller/` only as an archived legacy setup.

## Build and install

Choose the target explicitly:

```sh
# Hero DSL tested unit: big-endian MIPS
sh tools/build-keenetic.sh mips
# Bundle: dist/keenetic-mips/

# Titan tested unit: little-endian MIPS
sh tools/build-keenetic.sh mipsle
# Bundle: dist/keenetic/
```

Bundles include license notices and a corresponding-source snapshot with
vendored Go dependencies. Each build recreates the source bundle so files removed from the working tree do not survive from an older build. When redistributing a binary, share the complete
bundle, including its source directory; see [the distribution review](docs/licensing.md).

Copy the matching bundle to the router. Start with the read-only probe:

```sh
chmod +x ./ukn
./ukn -probe
sh ./install.sh
```

The installer checks space for staged replacements and old-file backups plus 16 MiB headroom, verifies copies, and runs a read-only probe of the staged executable before stopping a running agent. It installs `/opt/sbin/ukn` and `/opt/etc/init.d/S99unifi-keenetic`, preserves existing configuration and adoption state, and leaves the service **stopped** (a new configuration is also **disabled**). Start it explicitly after reviewing configuration. Probe output contains network/client identifiers; do not publish it unredacted.

Ordinary installation failures and caught termination signals attempt to restore previous files and restart an agent that was running. Failed recovery retains private backups and blocks another installation pending review. An existing service must support status codes 0 (running) and 3 (stopped). The guarded service is replaced first and refuses enabled startup while installation markers remain; rollback restores it last. Forced termination or power loss may leave `/opt/.ukn-install.*` recovery data; follow the [installation recovery notes](docs/installation-recovery.md) before retrying. These paths have local failure-injection coverage; installed upgrades and power-loss recovery still require spare-device testing.

`ukn -installation-status /opt` inspects retained installer metadata and verifies
backup/current file matches without changing files or contacting router services.
Use a known-good executable. Its JSON is a diagnostic snapshot, not an automated
recovery decision or a complete package-integrity check.

For an optional isolated installer/service test, run
`python3 tools/check-container-lifecycle.py --image YOUR_LOCAL_LINUX_IMAGE`.
It requires an existing Linux arm64/amd64 image with a shell, coreutils, awk,
SHA-256 and `start-stop-daemon`; it never pulls an image. The disposable container
has networking disabled and synthetic native state. It exercises the shipped
installer/service and real agent, including failed-upgrade recovery, a detached
watchdog across executable replacement, and startup recovery before telemetry.
It includes the real 65-second watchdog delay and writes
`dist/validation/container-lifecycle.json`. It does not replace MIPS, reboot,
power-loss or real-controller acceptance tests.

Configure `/opt/etc/unifi-keenetic.conf`:

```sh
ENABLED=1
INFORM_URL='http://CONTROLLER_IP:8080/inform'
SSH_LISTEN=':2222'
MODEL='U7PG2'
```

Then start or stop it:

```sh
/opt/etc/init.d/S99unifi-keenetic start
/opt/etc/init.d/S99unifi-keenetic stop
/opt/etc/init.d/S99unifi-keenetic status
```

The packaged service starts in **log-only mode**: it reports status and receives controller configuration but does not apply it. `ukn -models` lists available identities; `-model CODE` selects one. Default persistent state is `/opt/var/lib/unifi-keenetic`. `STATE_DIR` and `PID_FILE` can select separate absolute paths; isolated instances also need distinct listener ports and controller identities. `status` checks process identity, not controller or WLAN health. `ukn -build-info` reports the actual executable build separately from its UniFi compatibility version.

Adoption-state saves use a unique mode-600 temporary file, synchronize it before
atomic replacement, and synchronize the parent directory afterward. This also
avoids following a leftover fixed temporary-file symlink. Keep the state directory
private. Physical power-loss durability still requires hardware validation.

Adopt the pending device in UniFi. For a virtualized controller, set Inform Host Override to an address reachable from the router, not an internal container address.

## Controlled provisioning tests

**Prefer wired management and an independent internet path.** A separate SSID does not isolate changes to a shared radio. Do not run unrestricted `-apply` while relying on this router's Wi-Fi for access.

Connected tests use `-wlan-test-only`, which:

- Excludes shared-radio changes and unrelated WLANs.
- Requires explicitly mapped, non-primary AP slots.
- Accepts only SSIDs beginning with `UKN-Test`.
- Refuses mapped slots already configured with a non-test SSID.

Inspect the controller's saved configuration and select genuinely unused slots. [testing/hero-slots.json](testing/hero-slots.json) is an example from the tested Hero, **not a universal mapping**. Controller WLAN indexes can change.

```sh
# Inspect a redacted plan first; run on the router or build for the host.
ukn -plan /path/to/system_cfg.txt -slots /path/to/slots.json -wlan-test-only

# Explicitly enable only the scoped provisioning test.
ukn -url http://CONTROLLER_IP:8080/inform \
  -state /opt/var/lib/unifi-keenetic \
  -slots /path/to/slots.json -wlan-test-only -apply
```

Before mutations, the daemon captures inverse commands and writes a private recovery journal. Changes must pass WLAN/controller checks before confirmation; an independent watchdog attempts recovery after 65 seconds. Rollback is a recovery mechanism, not a guarantee against losing access. Stop the foreground process to finish testing; omitting `-apply` returns to log-only operation.

Journal creation, native transaction commands, confirmation and recovery share a private persistent `.lock` file. Detached watchers check the transaction identity while holding this lock. Recovery durably marks the journal before replay, preventing the old transaction from continuing or confirming even if recovery fails. Failed recovery retains every inverse for startup retry. Keep the lock file in place even when no rollback is pending. Offline tests cover these races; [scoped Titan recovery tests](docs/recovery-validation-2026-11-24.md) also passed on candidate `dc0fba5`. Installed upgrades, reboot and mixed-version watchdog behavior still need lifecycle validation. Older binaries do not participate in this protocol.

The tested changes affect running configuration. The port does not issue a startup-config save. Reboot persistence of provisioned router settings is not verified.

WLAN credentials are redacted in logs and stored in private files, but `ndmc -c` can expose them briefly in local process arguments. The experimental interactive terminal runner is not used for provisioning.

## Development and test environment

```sh
go test ./...
go test -race ./...
go vet ./...
```

- [Keenetic implementation and live-test notes](docs/keenetic.md)
- [Official UniFi OS Server test environment in OrbStack](testing/controller/README.md)
- [License/distribution review](docs/licensing.md)
- [Inherited protocol internals](docs/internals.md) — largely describes OpenWrt behavior
- [KeeneticOS 5.1 CLI reference](https://storage.googleapis.com/docs.help.keenetic.com/cli/5.1/en/cli_manual_kn-1810.pdf)

Layout: `cmd/ukn` is the Keenetic daemon, `internal/keenetic` is the NDM backend, `packaging/keenetic` contains Entware installation files, and `tools/build-keenetic.sh` builds its bundles. The inherited `cmd/uow`, `internal/openwrt`, and OpenWrt packaging remain available separately.

## License and affiliation

This fork remains **GPL-2.0-or-later**, under the same terms as upstream. See the unchanged [LICENSE](LICENSE), [NOTICE](NOTICE), and [third-party license text](LICENSES/Go-BSD-3-Clause.txt). There is no warranty.

No Ubiquiti or Keenetic firmware is bundled. UniFi OS Server is downloaded separately from Ubiquiti by the optional environment setup script and remains subject to its own terms. This project is not affiliated with, endorsed by, or supported by Ubiquiti, Keenetic, or OpenWrt; their names and trademarks identify compatibility only.
