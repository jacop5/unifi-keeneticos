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
This specific preflight failure now retries the same controller request after
one minute, doubling up to five minutes, so an external registration can unblock
application without a controller version change. The agent still does not create
registrations. Each attempt prepares the complete plan before any mutation;
other permanent configuration/ownership/recovery failures remain non-retryable.
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

Clean candidate `ef31a77` passed the full release gate and all sixteen expanded
Linux container checks. The added container check runs the guarded application
test and the actual CLI same-version/restart block-policy tests on Linux arm64.
The separate lifecycle checks still exercise the actual detached watchdog and
cross-revision journals. Native blocking/client enforcement remains unverified.

## Read-only block-list planning

Use `-plan-blocked /private/path/blocked.json` together with `-plan` and
`-plan-json` to compose an explicitly supplied JSON array of client MAC strings.
An empty array previews unblock back to the explicit controller baseline.
The input is capped at 64 KiB. This mode never loads adoption state or contacts a
router/controller; it also works with `-preflight-running` for saved native
rollback preparation. It rejects live/action modes before dispatch. JSON output
contains ACL policies and counts, not client addresses or credentials.

This input was added after a read-only check found two adopted devices on the
local test controller. No global block-list command was issued: a site-wide test
would have exceeded the verified single-device scope. The Titan's running agent
remains log-only with block control disabled. The offline CLI test checks both
bands, block/unblock, malformed input, action conflicts, secret redaction and a
native-command sentinel using a deliberately unusable adoption-state path.

### Titan offline execution

Clean candidate `d36e003` passed the release gate and was staged separately on the
Titan, without replacing the running `ef31a77` daemon. Its hash was
`4c154480b956f60ca623101c89c23f2549368a464c768ae5129d5d22566bf8f8`;
native build metadata confirmed linux/mipsle softfloat and the clean revision.
The earlier unused `27710ed` artifact was retired after hash/process checks to
retain staging headroom; the active tested executable remained untouched.

The saved controller configuration had `macacl.status=disabled`. A private copy
had BSS Transition disabled for the two test WLANs; the original controller
profile/cache was unchanged. Native offline planning then established:

- A supplied synthetic blocked address produced a supported deny ACL with one
  client on each AP6 test band. This was planning, not successful preflight.
- An empty list restored policy `none` on both bands and passed saved-native
  preflight, with two total prepared changes in the complete WLAN plan.
- Preflight with the synthetic block was rejected for missing Keenetic known-host
  registration. No registration was created and no forward/inverse command ran.

Post-run normalized native configuration matched exactly (SHA-256
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`);
credential receipt bytes and selected adoption identity fields were unchanged.
The original daemon PID/executable remained selected and no rollback was pending.
Raw inputs/results stayed privately on the router; only redacted policy/count and
preflight fields were inspected locally. This proves native diagnostic execution,
not live blocking, client eviction or applying-mode acceptance.

The retry regression test uses mutable synthetic registration state and an
encrypted controller exchange: the first attempt returns the typed missing-host
condition before journal/watchdog/mutation or confirmation; an external simulated
registration makes the same request eligible at its retry deadline, after which
ordinary guarded application and confirmation succeed. Error classification uses
the typed condition, not message text. No native retry behavior has been deployed
or observed on the household router yet.
Clean retry candidate `1b0d2cc` passed the full release gate and all seventeen
historical-package Linux lifecycle checks, including the expanded registration-
arrival integration case. The report recorded an unmodified source tree.

## Disabled-slot block ACL rollback probe

`-rollback-probe-mode blocked-acl` extends the existing unused-slot probe. It
requires a non-primary AP, administratively down, with no SSID in either status
or running configuration. It reuses one existing valid known-host registration;
it never creates a global registration. The composed one-client deny ACL and
temporary probe SSID remain on that disabled AP only. Native ACL readback and
continued down state are checked, then an intentional health failure exercises
the normal journal rollback and exact AP preimage restoration.

Tests cover registration selection, reversible target-only commands without `up`,
and actual CLI refusal of primary, enabled or configured slots before mutation.
This probe cannot establish client eviction or blocking on an active WLAN.

### Titan native rollback result

Clean `4cc2052` passed the full offline release gate, then its MIPS little-endian
softfloat executable was staged and hash-verified on the Titan:
`73e5d81ac6352c73e77759e2eb3f04c4cd37d27de8ce9133081170c68c07cf02`.
The unused `d36e003` diagnostic artifact was retired after hash/process checks;
the running `ef31a77` log-only agent was retained unchanged.

The `blocked-acl` probe passed on `WifiMaster0/AccessPoint5`. Its guards verified
the slot was disabled and SSID-free. It reused an existing known-host registration,
prepared three reversible changes, verified the composed deny ACL while the AP
remained down, deliberately failed health and restored the exact AP preimage.
No global host registration, controller block command or shared-radio operation
was issued. Neither household WLAN was targeted.

Full normalized native configuration matched the pre-test baseline immediately
after rollback. The actual detached watchdog was observed, then allowed to finish
its normal deadline. A second full comparison after its exit still matched:
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
The probe journal was absent, credential receipts and selected adoption identity
fields were unchanged, and the original log-only agent executable remained active.
Private raw preimages stayed on the router.

This establishes native deny-ACL composition, readback and journal restoration on
a disabled Titan AP. It does not establish active-client enforcement, immediate
kick or end-to-end controller blocking on a serving WLAN.
