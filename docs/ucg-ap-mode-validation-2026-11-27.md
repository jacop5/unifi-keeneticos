# UCG Max and Titan AP-mode validation — 2026-11-27

This run uses Titan KN-1810, KeeneticOS 5.1.10, in AP mode behind a UCG Max
running UniFi Network 11.0.104. It is a deployment acceptance run in progress,
not a claim of full feature parity or a finished installation.

## Controller session

The Entware storage and disabled old service survived the operating-mode switch.
A fresh private temporary session was used rather than reusing old-controller
adoption state. The controller initially exposed a pending AC Pro (the selected
emulated model). Its adopted flag became true, but its first encrypted inform
never arrived; last-seen remained stale and the agent had no adopted state.

The UCG has several LAN addresses. Its advertised adoption URL used the AP's
untagged LAN gateway, while the initial inform URL used another VLAN's gateway.
The listener restricts incoming adoption SSH to the configured inform host.
Using the same-LAN address removes that mismatch, but did not establish that
Network 11 delivered the key over SSH: no SSH-adoption event was observed.
Queued forget/retry requests had no observed effect; direct legacy resource
DELETE returned 404. These are recorded failed attempts, not successful cleanup.

The operator explicitly approved a temporary administrator-assisted key
bootstrap. Only the matching device's per-device key was transferred from the
controller API over encrypted SSH stdin into a private atomic temporary state
file; it was not printed or committed. After restart the controller reported
connected state 1, first-inform waiting cleared, AES-GCM was enabled and a full
configuration was received/persisted. Automatic Network 11 SSH adoption remains
an open acceptance item; this bootstrap is not proof that it works.

## Scoped WLAN application

A dedicated AP group contains only this Titan. The separate `UKN-Test-UCG`
profile uses WPA2/CCMP on both bands, ordinary untagged Default LAN,
BSS-transition/FT/WPA3/guest policies disabled. Actual WLAN indexes 1 and 2 map
to the verified-unused AP6 interfaces on 2.4/5 GHz. Country/radio writes are
excluded by `-wlan-test-only`; the two household APs are outside the slot map.

Native private configuration backups remain on the router. An unrelated SIP
block appeared between snapshots while the agent was log-only, so the older
whole configuration must not be restored over those changes. Guarded application
uses per-interface preimages and a private rollback journal/watchdog.

Candidate `6386b11` prepared 14 changes and confirmed configuration
`23a652ea160dfee0`; no rollback journal remained. Native 2.4 GHz AP6 reported
running. A Mac authenticated on that AP, obtained a UCG DHCP address in the
Default LAN, resolved DNS through the UCG with the UDP socket bound to Wi-Fi,
and completed HTTPS with curl bound to Wi-Fi (final HTTP 200). Native association
and controller telemetry agreed: mapped `ath0`, RUN/up, one client.

The 5 GHz AP was configured but pending because the shared radio was in DFS CAC
on channel 104, width 160, with maximum countdown 605 seconds. Saved pre-test
country IT and width 160 match: no radio commands were in the scoped plan.
The controller accepted the inactive configured VAP as `ath1`, INIT/down,
zero clients. This proves this inactive-row API case, not browser rendering or
all administrative pause cases. 5 GHz client acceptance was initially pending. After CAC reached zero, both
the main 5 GHz AP and test AP reported running. The test profile was temporarily
restricted to 5 GHz: the 2.4 GHz test AP went down with one native change, the
Mac authenticated on WifiMaster1/AccessPoint6 (native rates 1300/1733), and
Wi-Fi-bound HTTPS returned 200. Controller ath1 telemetry agreed (RUN/up, one
client). The profile was restored to both bands and confirmed again.
Enabling a virtual AP can coincide with shared-radio DFS unavailability; scoped
commands alone do not prove absence of a radio-wide service interruption.

## Diagnostics candidate and restart

`d491c45` adds opt-in `-debug-adoption` numeric HTTP status transitions and a
listener-start message. Request/response/body/error pass-through, transition
suppression, concurrency and secret redaction passed targeted race tests and
independent review. Clean candidate passed all nine offline release checks:
race, vet, Python tooling, both MIPS packages/manifests and offline source rebuilds.

After staging/hash verification on Titan, its scoped applying restart retained
the encrypted session: HTTP 200, two credential receipts reused, **zero native
changes**, then controller confirmation. No hardware proof for unrelated features
is inferred from this replacement. Current instance is temporary, not installed
for boot. Regulatory country on UCG was still US at the time of these checks;
changing it to native IT was separately awaiting explicit operator approval.

## Still required

- Finish pause/resume and restoration checks on the pinned candidate.
- Resolve automatic Network 11 adoption or provide a separately reviewed pairing
  workflow; do not label a manual state bootstrap automatic adoption.
- Migrate production profiles with their actual existing passphrases, then verify
  explicit mappings and behavior. Do not use native encrypted values as passwords.
- Validate installed service lifecycle and reboot on an appropriate spare device;
  inspect Speedster repeater topology and Wi-Fi System ownership before deployment.

## Production preparation checkpoint

The agent was returned to log-only before production profiles were created.
Both household profiles are stored disabled in a Titan-only AP group, with the
operator-supplied corrected passphrases sent only to the UCG. Neither credential
is recorded here. Bands match the existing configuration (nozomi on 2.4 GHz,
mizore on 5 GHz); native readback showed WPA2 and WPA3 enabled, so the disabled
profiles were set to WPA2/WPA3 transition with optional PMF. Existing household
APs remain up. A production WLAN-only provisioning option is being implemented
and requires independent verification before native handover.

## Overnight production checkpoint

Clean production candidate `654dc52bb024c4ec18afbec5c9619c7ca223ba6a` passed all
nine offline release checks. The new production `-wlan-only` mode passed planner,
CLI, service and status tests plus lead targeted race checks. Independent static
review was accepted; reviewer probes attempted after a Smart denial were
explicitly excluded from acceptance evidence.

Both production controller profiles were enabled in a Titan-only group. Verified
controller IDs 3/4 map to existing native 2.4 GHz AP1 / 5 GHz AP0. IDs 1/2 retain
ownership of the disabled test AP6s. The refreshed controller configuration after
SAE capability advertisement preserved WPA2/WPA3: an earlier cached config had
planned WPA2-only and was not applied. A Titan-only force-provision produced the
correct transition plan. No regulatory country change was performed.

The accepted WLAN-only plan excluded every master-radio target. Guarded native
application prepared nine changes, confirmed version `1182856ed254668c`, and
cleared the rollback journal. Both household APs reported RUN/up; both test APs
reported disabled/down. Native radio country/channels/width remain owned locally.
The conservative profile disables FT/BSS transition; this run does not establish
fast-roaming support. Speedster integration is explicitly deferred until morning.

Real Mac client acceptance passed on each actual production SSID: authentication,
UCG DHCP leases and HTTPS bound to Wi-Fi (HTTP 200). Native associations and UCG
ath2/ath3 telemetry agreed with the respective bands. This proves client access
for the tested Mac, not every WPA2/WPA3 client or protocol-level SAE negotiation.

The guarded package was installed persistently. The known disabled legacy script
had no status command: its exact hash, disabled configuration, absent PID and
absence of processes using the old installed binary were verified before moving
it to a backup. Original script/config/binary are retained privately under
`/opt/var/backups/ukn-before-654dc52`. No installer status guard was weakened.

The confirmed session, receipts and SSH host keys were copied locally within the
Titan to private `/opt/var/lib/unifi-keenetic-ucg`. Service configuration uses
`ENABLED=1`, `APPLY=1`, `WLAN_ONLY=1`, explicit four-slot mapping, native SAE
advertisement enabled, and LED/reboot/block/version-spoof actions disabled.
Keenetic already has `opkg initrc /opt/etc/init.d/rc.unslung`; boot integration
is configured. Physical device reboot was not performed.

Installed service activation and a subsequent service restart both passed. PID
changed from 10049 to 10281; confirmed provisioning returned, UCG remained
connected, no rollback remained, normalized native configuration was unchanged
across restart, and Wi-Fi-bound production HTTPS still returned 200. The stable
instance now runs from `/opt/sbin/ukn`, not a temporary executable.

Tonight's production-WLAN objective is met. Automatic Network 11 SSH adoption,
physical reboot acceptance and the broader roadmap remain open; do not infer
completion of the entire port from these core deployment results.
