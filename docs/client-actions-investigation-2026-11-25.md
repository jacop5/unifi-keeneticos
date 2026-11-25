# Client kick/block investigation — 2026-11-25

Scope: identify a native equivalent for UniFi client reconnect/kick and persistent
block/unblock without changing household WLANs. No router commands or controller
configuration changes were made in this investigation.

## Documented native boundary

The supplied [Titan 5.1 CLI manual](https://storage.googleapis.com/docs.help.keenetic.com/cli/5.1/en/cli_manual_kn-1810.pdf)
was read from its previously downloaded PDF using local text extraction.
Sections 3.32.147–148 document `interface mac access-list address` and
`interface mac access-list type` for an Access Point. Those operations support
per-AP filtering, with permit/deny/none policy. They are already used by the
WLAN ACL implementation. They do not establish immediate station eviction.

Section 3.32.86 documents `interface idle-timeout` on WiFiMaster. That is a
radio-level inactivity setting, not a targeted client kick. Section 3.178.4
documents association inspection. Searches of the extracted manual for kick,
deauthentication, disassociation and disconnect found no documented targeted
kick command. This is a bounded documentation finding, not proof that the
firmware has no such operation.

## Static firmware evidence

REA's built-in export reader inspected the existing private firmware copies,
without execution. Full export inventories contained no symbol name matching
`kick|deauth|disassoc|disconnect` (case-insensitive):

| Artifact | SHA-256 | Exports | Evidence ID |
| --- | --- | --- | --- |
| libndmWifiMtk.so | `7ade9e25187d0e691eb369eae6faa6fa4f2f1d54252b60a0c2354f3d4983e4f7` | 655 | `ev_2200eaf3484538b029d975db9061d8daab5927f8b5808f3955f0e7f58ba016c0` |
| libndmCorePack.so | `9ca15ee910ed1b00bfff73c45aa82347a65e2cbf35022066622818f95ba998ba` | 12327 | `ev_60471033d076f45fde757dc5b38abe799eddcce3071a8701fffb951f39a1c88b` |

Exports do not cover private functions, driver operations, CLI grammar or other
libraries. No absence claim follows for those surfaces. Provider-backed string
search/decompilation was unavailable: no supported analysis engine was configured.
The completed inventory evidence was checkpointed privately, the binary closed
and the investigation reset. Firmware and raw evidence are not distributed here.

## Implementation decision and next work

Keep kick/block/unblock actions unimplemented rather than substituting radio
timeouts, cycling an AP, or globally blocking a known host. A real kick requires
a verified per-client operation and a fresh association check against owned WLAN
slots; a station outside that scope must be rejected.

Persistent controller blocking needs an owned overlay on native per-AP policy,
not replacement of an operator's ACL. In particular:

- Preserve native permit/deny semantics and the controller WLAN ACL. Blocking a
  member of a permit list means exclusion, not insertion into that permit list.
- Save a private ownership/preimage record before changes. Unblock must remove
  only the overlay, preserving rules created by the operator or WLAN policy.
- Reconcile after restart, roaming, reassociation and controller-list changes;
  a one-time action against currently associated clients is insufficient.
- Guard mutation/readback/recovery with the same journal ownership as ordinary
  provisioning. Failure must not acknowledge successful enforcement.
- Test individual entries and live disconnect/reconnect behavior with a separate
  client before exposing controller action support.

These are prerequisites for general native-policy overlays. The bounded
controller-owned integration below is now implemented; immediate kick and
overlays on independently managed native ACLs remain unavailable.

## ACL composition prerequisite

`ComposeBlockedACL` now implements the pure policy operation for a separately
retained, controller-owned baseline: authorization becomes baseline authorization
intersected with the complement of the block list. Permit entries are removed;
deny entries are added. A baseline with policy `none` contributes no dormant
entries to an active deny overlay. An empty block list restores those dormant
entries and the original policy. Inputs are validated, normalized and deduplicated;
the result does not alias or mutate the baseline.

Tests check authorization across all three policies, duplicate/case-normalized
addresses, malformed inputs and exact baseline preservation. A transaction test
blocks the last permitted client, retains an empty permit list, verifies native
readback and prepares an unblock restoring that client. It checks AP quiescing
and the inverse commands, using the existing transaction machinery.

## Experimental controller-owned runtime integration

`-client-block-control` / `CLIENT_BLOCK_CONTROL=1` opts into reconciliation using
the controller's explicit ACL as the baseline. The default is off. `SLOTS` is
required; `APPLY=0` only previews the composed plan. Native enforcement additionally
requires `APPLY=1`, adoption and all ordinary apply/ownership/recovery checks.

Each configured mapped WLAN must have an explicit controller ACL request,
including the explicit globally disabled policy (`macacl.status=disabled`).
Without it, the entire plan is rejected as
`client-block.<native-target>.baseline-missing`, even with an empty block list.
No native operator policy is inferred or claimed. A controller-owned ACL can be
replaced by that controller's current policy; later native edits to it are not
independently owned or merged. Do not enable this mode for native-managed ACLs.

The existing version/controller/key-bound configuration snapshot durably retains
the baseline; the adopted state also retains the controller block list. Planning
composes those inputs afresh instead of treating the previous native overlay as
the baseline. Restart and unblock therefore do not need a second mutable baseline
file for this ownership model. Block-list changes participate in the internal
planning and confirmation identity, so changes under the same configuration
version trigger reconciliation and changes during confirmation invalidate it.

ACL application uses the existing journal, preimage, known-host registration
preflight, quiescing and native readback verification. It may briefly interrupt
every client on the mapped AP while replacing that AP's ACL. It never substitutes
a radio-wide kick. Associations outside mapped WLANs are unaffected by the plan.
Missing Keenetic known-host registrations can prevent a block from applying.
Omitted paused WLANs in scoped testing remain down; their native ACL is restored
from the explicit controller baseline when they are configured again.

`client_block_control` is recorded in current provisioning status and separately
in the last confirmed scope. Protocol receipt/action log messages are not proof
of enforcement; use the provisioning report and native/client checks. Disabling
the opt-in returns to ordinary controller ACL planning. If that controller then
omits its ACL request, no automatic cleanup of previously managed native ACLs is
claimed; restore an explicit baseline before relinquishing ownership.

Tests cover missing ownership, baseline preservation, same-version block/unblock
through the actual CLI and encrypted loopback controller, restart with a saved
baseline/block list, confirmation identity invalidation, reporting and service
switch validation. Native enforcement and live client behavior remain unverified.
This mode has not been deployed to the household router.

Clean integration candidate `85a45cb` passed the full offline release gate:
uncached race/static/tooling checks, both MIPS packages and manifests, and
offline rebuilds from bundled source. All fifteen cross-revision container
lifecycle checks also passed with agent A at `62b1285` and B at `85a45cb`, using
current packaging and block control disabled by default. Those lifecycle checks
validate default service/upgrade recovery, not native blocking. The new block
reconciliation tests used the actual CLI against an encrypted loopback controller
with application disabled; the transaction tests used synthetic running config.

The guarded application integration test subsequently uses a mutable synthetic
native ACL and an encrypted loopback controller with the runtime confirmation
identity guard. It checks successful confirmation, same-version unblock during
confirmation, HTTP 503 and controller removal. Failed confirmation restores the
exact ACL and AP state; complete recovery clears the journal. Every mutation
requires a persisted private journal and simulated watchdog arm. Successful and
reconciled no-op plans still require a fresh controller exchange. This test does
not spawn the detached watchdog or replace a real radio/client enforcement test.
