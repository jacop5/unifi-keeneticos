# Conservative controller preset

This is a starting profile for the currently implemented personal-WLAN mapping,
not a production certification. Validate the generated plan before enabling
application. Controller UI labels may differ between Network versions.

| Setting | Starting value | Reason |
| --- | --- | --- |
| Security | WPA2 Personal, AES/CCMP | Avoids enterprise, multiple-key and experimental SAE requirements |
| WPA3 / transition mode | Off | Keeps this baseline separate from transition-mode testing |
| Protected management frames | Disabled | Conservative baseline; optional/required PMF needs separate acceptance |
| BSS Transition (802.11v) | Off | Enabled requests are explicitly rejected; 802.11k is a different feature |
| Fast roaming (802.11r) | Off | Real client reassociation remains an acceptance task |
| Guest policy / portal | Off | Guest authorization and firewall integration are not implemented |
| Network | Ordinary LAN, native Bridge0 | Creating VLANs, uplinks or DHCP services is outside this preset |
| Bands | Explicitly selected, with an owned slot for each | A controller WLAN does not grant ownership of an existing native WLAN |

Use a dedicated AP group containing only the intended test device and a separate
test SSID. Map controller WLAN IDs to unused native AP slots explicitly. Do not
copy the test slot numbers from another device without checking ownership.
The named household WLAN protections used in our lab are test-session rules,
not hardcoded deployment restrictions.

## Verify before applying

1. Start with application disabled and inspect `-provision-status`. Confirm
   `apply_enabled` is false and check both `unsupported` and `ignored` settings.
2. In Wi-Fi-connected main-router tests, use `-wlan-test-only` with only the
   dedicated slots mapped. Exclude serving WLANs and shared-radio changes.
3. An empty unsupported list means the planner accepts the selected scope. It
   does not prove native application, client connectivity or every requested
   setting. Review ignored settings independently.
4. Before an applying test, retain a native configuration backup and verify
   rollback readiness. Validate native readback, client traffic and restoration
   separately. Use a spare device for shared-radio or installation/boot tests.

Known ignored requests include DTIM, U-APSD, minimum data/management/beacon rates,
CCK-rate selection and matched per-client bandwidth limits. They produce no
corresponding native commands. Per-WLAN group-key rekey timing is also reported
as ignored; the documented shared-radio command is not used as a substitute.
This preset does not establish support for other unmapped options. Do not interpret
an accepted plan as full controller/native feature parity.

The current dedicated Titan profile was inspected on 2026-11-25: WPA2 Personal,
WPA3 off, PMF disabled and fast roaming off. BSS Transition was enabled and was
the only reported unsupported setting, on both test bands. Temporarily disabling
that one flag produced an accepted log-only plan on the live Titan agent; the
exact profile was restored afterward. Native application and client traffic
were not tested in this run. See the [live check](preset-validation-2026-11-25.md).
The synthetic `compatible-wpa2.cfg` regression fixture checks both owned bands,
WPA2 with WPA3/PMF/FT disabled, LAN membership, credential redaction, the exact
DTIM/U-APSD omissions and rejection when BSS Transition is enabled. It also
checks that commands stay within the two owned APs and their LAN membership.
Existing transition-mode evidence is recorded separately in
[the Keenetic notes](keenetic.md); it is not evidence for this baseline preset.
