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

## Feature status on Keenetic

| Feature | Status |
| --- | --- |
| Adoption, saved controller key, AES-GCM informs | Verified live on Hero and Titan; retained across agent restart; controller restart/reconnection verified on both |
| SSID, WPA2 personal, LAN bridge membership | Verified live on explicit non-primary AP slots |
| WLAN pause/resume | Verified live; controller omission of paused WLANs handled in test-only mode |
| Client authentication, DHCP and internet access | Titan 2.4/5 GHz Mac authentication, router-issued IPv4 leases and Wi-Fi-bound HTTPS verified; automatic reconnection remains unverified |
| Client IP, band, signal, width, rates and throughput | Observed in UniFi and compared with NDM data |
| Client names, counts and power-save status | Titan controller API count consistency, DHCP hostname equality and active power-state reporting verified; sleeping-client transitions untested |
| Active-host name/IP fallback | Implemented with ARP/DHCP precedence and offline/conflict filtering; external-DHCP/AP-mode live validation outstanding |
| Uptime, memory, CPU and radio/client telemetry | Implemented; basic live reporting verified, not every field is complete |
| Physical Ethernet link status | Titan's five ports received by the controller with measured link speed/duplex; no port configuration or inferred uplink roles |
| Private rollback journal, inverse commands, watchdog, startup recovery | Implemented; immediate failure and detached crash recovery verified on Titan |
| Controller outage recovery | Real injected HTTP 503 rolled back; same-version retry confirmed after controller connectivity returned |
| WPA3 personal, transition mode and PMF | WPA3-only guarded provisioning and 5 GHz Mac WPA3 authentication verified on Titan with experimental SAE advertisement; transition-mode client authentication remains unverified |
| Hidden SSID, peer isolation, RRM | Implemented and unit-tested; not comprehensively tested live |
| Explicit open / Enhanced Open (OWE), 64-hex WPA2 keys | Translation and rollback unit-tested; OWE/PMF commands verified only on disabled Hero slots, not with an associated client |
| Per-WLAN MAC allow/deny lists | Implemented with registered Keenetic hosts; native list and rollback verified on an unused disabled Hero slot; client enforcement not yet verified |
| Minimum RSSI | Per-band requests map only to owned AP slots; native threshold/rollback verified on disabled Hero slots on both bands; client enforcement not yet verified |
| WMM | Controller provisioning and native readback confirmed on both Hero test bands; disabled-slot rollback verified; traffic-priority behavior not measured |
| WLAN schedules | Native weekly timers and guarded recovery; controller blackout, automatic stop/start, and pause/resume verified on both Hero test bands |
| Radio channel, width and administrative state | Implemented; deliberately excluded from connected Wi-Fi tests |
| Regulatory country | Implemented with neutral radio transition and exact country inverse; offline tests only, excluded from connected Wi-Fi tests |
| Automatic client reconnect after WLAN pause | Not verified |
| VLAN WLAN binding | Implemented with explicit `-networks` mappings to existing native bridges; automated tests only, no live VLAN traffic verification |
| Fast roaming | Explicit disable implemented with readback/rollback; enable remains unsupported |
| BSS transition (802.11v) | Enabled requests explicitly unsupported; per-AP 802.11k RRM is not treated as equivalent |
| VLAN/uplink creation, mesh, guest portal, scanning, traffic shaping | Not ported; unmapped VLANs are explicitly rejected |
| Controller LED on/off | Opt-in with `-led-control`; real controller override → native off/on readback verified on Hero DSL; original override restored |
| Client block/kick, locate/reboot and other controller actions | Not implemented on Keenetic; logged rather than executed |
| Enterprise/WEP security, transmit-power control, advanced management | Not supported by the Keenetic translator |
| DTIM period and U-APSD | No Keenetic translation; controller values currently ignored |
| Service lifecycle and status | Private Titan start/restart/stop and live/stale/unrelated PID status verified; installed-package upgrade remains untested |
| Offline preflight | Saved configuration/rollback preparation tested against a fresh Titan snapshot and followed by guarded AP6 apply |
| Executable provenance | `-build-info` verified on Titan with clean revision, architecture and matching binary hash |

Radio telemetry reports native channel widths without guessing antenna or transmit-power capabilities. UniFi's device overview was verified live at 40 MHz on 2.4 GHz and 80 MHz on 5 GHz, matching the Hero's native measurements. VLAN configurations require explicit bridge mappings and complete membership information; they never fall back to Home. See [VLAN setup](docs/keenetic.md#vlan-wlan-binding).

The latest Titan client run verified counts and hostname ingestion, but its HTTPS check stopped at DNS resolution; earlier successful Wi-Fi-bound HTTPS tests are separate evidence. See the dated [live-test notes](docs/keenetic.md) for the scope of each result.

Unknown controller settings are not comprehensively classified. A successful adoption does not establish support for every control exposed by the selected UniFi model. This daemon never installs Ubiquiti firmware.

Keenetic sessions refuse controller firmware upgrades by default. Optional `-report-upgrade-version` acknowledges an upgrade by reporting and saving the offered version instead. Neither mode downloads firmware or simulates a reboot. The service setting is `REPORT_UPGRADE_VERSION=0` (refuse) or `1` (report only). A controller reboot request is logged; it does not reset uptime or restart the router. The displayed UniFi model/version is a protocol compatibility profile, not the installed Keenetic firmware version.

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
vendored Go dependencies. When redistributing a binary, share the complete
bundle, including its source directory; see [the distribution review](docs/licensing.md).

Copy the matching bundle to the router. Start with the read-only probe:

```sh
chmod +x ./ukn
./ukn -probe
sh ./install.sh
```

The installer runs a read-only probe, installs `/opt/sbin/ukn` and `/opt/etc/init.d/S99unifi-keenetic`, preserves an existing configuration, and leaves a new service **disabled**. Probe output contains network/client identifiers; do not publish it unredacted.

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
