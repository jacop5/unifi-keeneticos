# WLAN/client telemetry identity validation — 2026-11-25

Clean candidate `50bf98c` passed the full offline release gate, including
race-enabled Go tests, static/tooling checks, both MIPS packages/manifests and
offline source rebuilds. No live-router changes were made for this change.

The runtime correlates controller WLAN identities using explicit native slot
ownership plus matching SSID and band. An empty mapping leaves native identities
intact. Same-band duplicate SSIDs no longer select an arbitrary controller record.
The compatibility helper without a map accepts only one-to-one SSID/band matches.

Synthetic tests use three AP slots on one band, all named `Shared`, with one
authenticated client each. Two slots have distinct explicit mappings; the third
is unowned. The test verifies:

- Mapped slots receive their own controller ID/devname, and the unowned slot
  retains its canonical native identity.
- Top-level and nested per-WLAN client rows agree on WLAN identity.
- Client counts and byte counters remain unchanged.
- A later native snapshot projection remains unmodified.
- Missing ownership, duplicate slots, duplicate aliases, SSID/band drift and
  aliases colliding with another native interface do not produce guessed labels.
- Ambiguity in either direction prevents unowned SSID-based correlation.

The implementation generates station records once per projection and shares them
between top-level and per-WLAN views, avoiding conflicting labels within one
inform payload. It sends no native configuration commands.

Same-name WLAN behavior was not exercised with live clients or inspected in the
UniFi GUI. The existing Titan temporary agent remains the previously validated
`27710ed` log-only build; this telemetry candidate has not been deployed there.
