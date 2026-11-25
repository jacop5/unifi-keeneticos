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

These are implementation prerequisites, not delivered blocking functionality.
The current session parser already retains the controller block list and emits
actions; the Keenetic runtime logs those actions without native execution.
