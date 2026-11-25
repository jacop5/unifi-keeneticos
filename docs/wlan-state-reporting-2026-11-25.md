# WLAN state reporting — 2026-11-25

The collector previously retained only administratively enabled APs, then marked
every VAP row `up=true` and `state=RUN`. Configured disabled WLANs disappeared,
and an administratively enabled AP with an explicit down link was still reported
running. Authenticated association records could also survive an inactive AP in
the independently collected station list.

The collector now retains configured APs whose administrative state is up or
down. VAP rows report `up=false`/`INIT` when native administrative state or native
link is explicitly down; otherwise they retain `RUN`. Missing link measurements
retain the administrative-state fallback. Unknown administrative states are not
newly interpreted as configured active WLANs by collection.

Known inactive AP IDs are also retained in `Snapshot.InactiveAPs`, including
SSID-free unused APs that have no VAP row. Their stale association records are
excluded from the common station source, keeping device, radio and nested WLAN
client counts/traffic consistent. Unknown APs without negative state evidence
retain the existing station validation behavior. Slot/SSID/band identity checks
still determine controller aliases independently of operational state.

Read-only queries on the Titan observed top-level `state=up`, `link=up` for the
serving and test APs, and `state=down`, `link=down` for the unused AP. Nested
summary link values were `running`/`disabled`; the existing first-field parser
preserves the top-level measurement. No radio/WLAN state was changed to collect
these observations.

The regression fixture runs complete collection and mapped inform conversion.
It covers active, enabled-but-unlinked, configured paused and unused inactive APs,
stale authenticated records, nested summary fields, controller aliases, counts
and traffic totals. A separate test checks missing-link fallback. These are
synthetic transition cases, not live disabled-WLAN GUI acceptance; no candidate
containing this change has been deployed on the household router yet.

Clean candidate `1ff89ef` passed the full offline release gate, including race
tests, static/tooling checks, both MIPS packages/manifests and offline rebuilds
from bundled vendored source. This does not establish controller UI rendering
of the newly retained inactive rows.
