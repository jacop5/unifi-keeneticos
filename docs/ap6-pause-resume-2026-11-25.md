# Guarded Titan AP6 pause/resume — 2026-11-25

Clean candidate `1ff89ef` was staged on the Titan after its full release gate.
SHA-256 was `b9a1cded2e366a29f40070cc7f79e6fa7ebcaa2472f0013246d7e1abda551d01`;
native build metadata confirmed linux/mipsle softfloat and the clean revision.
The original `ef31a77` binary was retained for restoration.

Before testing, the Mac's current router neighbor entry was correlated with
authenticated associations: its connection used `WifiMaster1/AccessPoint0`.
Both AP6 test WLANs had zero authenticated clients. Their initial native state
was up. This used the current observed neighbor identity, not an assumption from
the Mac's hardware-address listing.

## Write boundary and real-controller results

The temporary applying agent used a self-tested native-command wrapper. It
forwarded explicit read-only queries and permitted only `up`/`down` on
`WifiMaster0/AccessPoint6` and `WifiMaster1/AccessPoint6`, requiring a pending
private rollback journal. Every other write was denied and recorded. Shared
radios, AP0, credentials, ACLs, WAN and DHCP were outside the permitted writes.
Client block and native action opt-ins were off; WLAN test scope was enabled.

The dedicated test profile's AP group was freshly verified to contain only the
Titan. A fresh full profile backup remained private. Its BSS Transition flag was
temporarily disabled to reach the supported baseline; no global block command
was issued and no other device's profile was changed.

Actual UniFi OS / Network 10.5.67 and native readback established:

1. The supported preset first confirmed with zero changes and two reused
   credential receipts.
2. Disabling the test profile confirmed pause on both AP6 WLANs at configuration
   `8f181900676a6e83`: two native down changes, no credential changes.
3. Re-enabling it confirmed resume at `e1fb09a2b1b0f09b`: two native up changes,
   with both credential receipts reused.
4. The write trace contained exactly those four AP6 administrative commands.
   No denied command or manual restoration was recorded.
5. The complete controller profile matched the fresh backup after restoration.
   The original agent was restarted log-only, with application/block control off.

Both test WLANs were down for roughly a minute while telemetry was inspected;
they had no clients. Primary AP0 was not targeted and remained administratively
up. This run did not measure packet loss on the control connection.

## Inactive-row limit

The test did **not** observe two matching inactive rows through the device API;
the combined test therefore ended with an explicit unmet telemetry assertion
after successful pause/resume and cleanup. The saved paused configuration had
zero test-WLAN SSID fields and zero alias fields. The current mapper consequently
cannot map those omitted WLANs through that snapshot. On resume the API again
exposed both mapped `ath2`/`ath3` test rows as up/RUN.

These observations do not establish whether inactive native rows were filtered,
hidden or otherwise transformed by the controller. They do not validate inactive
GUI rendering. A separate case must retain the controller's enabled WLAN identity
while the native test WLAN is inactive; do not invent or reuse aliases without
validating ownership and collisions merely to satisfy this assertion.

The client-side contract also matters: a native row without an SSID produces no
VAP row, while a row retaining its SSID but lacking current controller identity
retains its native name. Native SSID presence during the paused interval was not
captured in this test. The API assertion matched test ESSID, state and counts;
missing aliases alone do not explain or prove the API's behavior.

## Restoration evidence

Full normalized native configuration matched the original baseline after cleanup
and again after all observed test watchdogs exited:
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
Credential receipt bytes and selected adoption identity fields were unchanged.
No rollback remained. Raw preimages/configuration stayed privately on the router,
and the private handoff records the restored process identity.

This is positive controller-driven pause/resume and native restoration evidence
for one candidate on both bands. Active-client traffic, block enforcement,
inactive GUI rendering and broader P1.2 acceptance remain outstanding.
