# Explicit network membership guard — 2026-11-26

An offline audit found that a nonempty `-networks` mapping did not always enforce
the documented requirement for explicit WLAN bridge membership. The check ran
only when the controller also declared at least one VLAN. A VLAN-less or partial
snapshot without membership could therefore produce a supported plan targeting
Home (`Bridge0`).

The planner now requires explicit membership whenever network mappings are
enabled, independent of the presence of controller VLAN declarations. Missing
membership is reported as `wireless.<id>.network`; ordinary preimage/application
guards reject the entire unsupported plan before native mutation. Diagnostic
plan commands are not themselves evidence that a plan is supported.

Explicit untagged Home membership remains valid. The ordinary default Home
behavior with no network mappings is unchanged, as are complete tagged-network
plans. A stale mapping alone is not proof that a WLAN belongs to that VLAN; the
controller must supply complete membership information.

## Evidence

- The audit worker used private Go source overlays, without modifying source or
  contacting a router/controller. The lead independently reproduced the missing
  membership case and distinguished explicit Home membership as a valid control.
- The new regression checks both absent membership and an unrelated bridge port,
  asserts the unsupported marker, and verifies application never reaches native
  mutation or health confirmation. It also preserves the two untagged controls.
- An overlay of the original planner with the new test failed at the intended
  assertion: missing membership returned an empty unsupported list.
- The fixed package passed `go test -race ./internal/keenetic -count=1`.
- A separate Smart worker independently reviewed the two-line behavioral change,
  the tests and downstream execution guard, finding no counterexample. Its
  approval is source-review evidence, not hardware proof.

Other audit observations, including extra unused mapping entries, duplicate JSON
keys and speculative controller status fields, are not addressed by this fix.
The existence/semantics of those status fields was not established offline.

No router/controller access or live changes occurred in this work. VLAN/uplink
creation, installed hardware behavior and actual tagged traffic/isolation remain
separate acceptance tasks.

Clean candidate `41f2f9a` passed the full offline release gate: uncached race,
static and tooling checks, both MIPS packages/manifests and bundled-source
offline rebuilds. It has not been deployed to the router.
