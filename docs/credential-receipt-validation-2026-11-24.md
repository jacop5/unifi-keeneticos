# Titan credential receipt validation — 2026-11-24

Candidate `9911433cdc3dae5eb3c671180f294d9325693a0d`, clean build.
Executable SHA-256:
`16b9cbb709f090a23e9d9718752252417ae3cc448dc256dd4017fec591844da9`.
Titan KN-1810, KeeneticOS 5.1.9, MIPS little-endian softfloat;
UniFi Network 10.5.67. The clean offline release gate passed before staging.

## Scope and observations

The dedicated WPA2 test WLAN used AP6 on both bands, with BSS transition disabled
for the supported preset. Plan scope and fresh native ownership were checked;
private AP6 credential/SSID inverses were captured before application. Main
WLANs, shared radios, WAN, NAT and DHCP were excluded.

| Observation | Result |
| --- | --- |
| Before first application | No receipt file; conservative and receipt-aware preflight both required two credential writes |
| First apply | Daemon logged `changes=2 credential_receipts_reused=0`; configuration `2bc9c26aee2051b1` confirmed |
| Stored records | Two records, schema 1, file mode 0600; no credential values returned by the inspection helper |
| Preflight after confirmation | Conservative preflight still reported two changes; receipt-aware preflight reported zero changes and zero credential writes |
| Agent restart | Daemon logged `changes=0 credential_receipts_reused=2`; configuration `f0d3e253495470af` confirmed after normal native/controller health checks |
| Native restoration check | Complete normalized native configuration equaled the fresh pre-test baseline; no WLAN restoration commands were needed |
| Controller cleanup | The changed BSS setting was restored; checked test-profile fields matched the original profile |

The prepared counts are emitted by the same apply path that uses the records.
The zero-change path runs health checks without beginning a mutation transaction.
Offline runner tests independently check that no native writes occur in that path.
This run did not collect a system-call trace of every `ndmc` invocation.

## Connection change and final state

The operator reported that the Mac was now using Wi-Fi and being controlled
through the mobile app. The route was rechecked: Wi-Fi was active and native
association/ARP correlation identified main 5 GHz AP0 as the serving connection.
The test daemon was stopped, and the planned deliberate SSID drift test was
deferred. The full native configuration already matched baseline.

Only the private agent's stored controller URL was updated for the Mac's new
address; reverse substitution matched its on-router state backup. The AP6-scoped
zero-change restart then confirmed both record reuse and controller health.
The candidate was subsequently left connected in log-only mode, with no pending
agent rollback. The private test undo file and unused helper were retired.
No main Wi-Fi outage, radio change or Wi-Fi adapter cycle was performed in this
receipt test.

Comparisons stayed on-router. Normalization removed terminal formatting, blank
lines and generated `! $$$` comments. Before/after SHA-256:
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.

## Limits

These records attest to a confirmed native write and matching later readback;
they do not decrypt the native credential or independently verify a new client
handshake. Client connectivity was tested separately on the earlier candidate.
Live password/SSID/native-key drift, adoption-key rotation, cache-write failure,
power loss and installed upgrade behavior remain unverified. Offline tests
cover drift, restart reuse, invalid files, rollback, cancellation, late readback
changes, authentication removal and pause/resume. No forbidden endpoint was
requested during this run.
