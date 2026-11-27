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
