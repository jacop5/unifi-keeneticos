# Titan controller validation — 2026-11-24

Candidate: `632f35be62fe30ea106c75a84c31b90754222bc5`, clean build;
SHA-256 `db68258fcd35f531866b725726022e0a0213ce1747ee1234f13450d93421e684`.
Titan KN-1810, KeeneticOS 5.1.9, MIPS little-endian softfloat;
UniFi Network 10.5.67. The full clean offline release gate passed before staging.

## Controller preset and scope

The dedicated test WLAN used WPA2 personal, WPA3/transition disabled, PMF
disabled, fast roaming disabled, no MAC filter and no schedule. BSS transition
was explicitly disabled for the test: its default enabled request is rejected
by the current translator. Existing LAN binding was retained; no VLAN, portal
or enterprise authentication was requested.

This is a tested baseline, not a claim that all other fields were translated.
DTIM and U-APSD requests were explicitly reported as ignored. The preflight
scope checker accepted only AP6 on each band and existing Bridge0 membership.
Serving AP0 WLANs and shared radios were excluded.

## Observed results

| Case | Result |
| --- | --- |
| Scoped WPA2 provisioning | Confirmed `4c832ac5d713f443`; controller remained adopted/connected; report recorded test scope and untranslated fields |
| SSID idempotence | Earlier preflight required four writes: SSID and credential per AP. The quoting fix removed both SSID writes on the same native baseline |
| Credential idempotence | Still incomplete: an already-applied profile required two credential writes. Native readback had a different stored representation; lengths/encoding shape were examined on-device without exporting values |
| 5 GHz client | Native AP6 association authenticated as WPA2-PSK; its ARP address matched the Mac's Wi-Fi address; a DHCP ACK was recorded |
| Wi-Fi data path | HTTPS explicitly bound to `en0` returned 200 using an independently resolved endpoint address. The first system-DNS attempt failed; this is not a DNS-resolution pass |
| Controller pause | Confirmed `fedac83b9b801073`; both AP6 interfaces read back down |
| Controller resume | Confirmed `d6c3a3538e1f4047`; both AP6 interfaces read back up |
| Client after pause/resume | The Mac selected main AP0 instead of returning to AP6. Post-resume Wi-Fi HTTPS therefore does not prove test-WLAN automatic reconnection |
| Cleanup | Original controller profile restored with no differing checked fields; exact native preimage restored; final agent connected in log-only mode |

Credential-bearing inverses were captured before application in a private
on-router journal by a temporary helper restricted to this session's AP6
interfaces. That helper is not included in the deployment. Restoration used the
existing recovery engine after stopping the scoped apply agent. No raw native
configuration or credential values accompany this report.

The complete native configuration compared equal to the pre-test baseline after
removing terminal formatting, blank lines and generated `! $$$` comments.
Normalized SHA-256:
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
The temporary test SSID was removed from the Mac's preferred networks; it had
already selected the main AP0 network. No main network preference was removed.

## Remaining acceptance work

Repeat client traffic on 2.4 GHz with this candidate, investigate credential
representation without guessing equality, distinguish system-DNS behavior from
the Wi-Fi data path, and test automatic return with controlled client selection.
This run does not validate WPA3/OWE, FT roaming, schedules, installation, reboot,
power-loss recovery or every feature of this candidate.

The subsequent [2.4 GHz and hidden-SSID run](band-hidden-validation-2026-11-24.md)
used the same candidate, verified the other band and a Wi-Fi-bound DNS path,
and distinguished the earlier negative DNS response from a Wi-Fi-only failure.

Follow-up inspection of the saved on-router snapshot confirmed a two-argument
`ns3` credential form on both test APs; only the syntax marker and argument count
were returned. A subsequent comparator fix rejects equivalence between this
form and a quoted literal passphrase. That follow-up is not an additional live
apply test of candidate `632f35b`, and does not resolve repeated credential writes.
