# Wi-Fi-system scope investigation — 2026-11-24

Read-only diagnostic candidate:
`e5bf8e5003525f84512a02b43c003857bceaf51f`, clean build.
SHA-256 `a3357ec3cfae2f91eedb76f3b9333a4434c3c03b49a6a2a234f72898d297a3f9`.
The full clean offline release gate passed. A foreground invocation then ran
on the Titan KN-1810 with KeeneticOS 5.1.9; the existing `9911433` log-only
daemon was left running unchanged. The temporary diagnostic binary was retired.

## Read-only result

`ukn -wifi-system-status` resolved two native Wi-Fi-system WLANs:

| Native band | Listed AP |
| --- | --- |
| 2.4 GHz | `WifiMaster0/AccessPoint0` |
| 5 GHz | `WifiMaster1/AccessPoint0` |

Both bands explicitly reported enabled, and both WLANs explicitly reported
not disabled by schedule. No AP names remained unresolved. AP6 test interfaces
were not listed in this reply. This is an observation of manager membership,
not a claim that no other component can affect an AP.

Only `show mws wlan` and `show interface` were requested by the diagnostic.
It resolved aliases to canonical AP IDs and omitted WPS/PIN/credential fields.
A deliberately nonexistent state-directory argument remained nonexistent after
the run. No daemon restart, controller edit, WLAN mutation, shared-radio change
or Wi-Fi adapter cycle was performed. The operator's connection remained on
Wi-Fi throughout this investigation.

## Why the documented BSS command is not a direct AP translation

The supplied [KeeneticOS 5.1 CLI manual](https://storage.googleapis.com/docs.help.keenetic.com/cli/5.1/en/cli_manual_kn-1810.pdf)
establishes these scope constraints:

- Section 3.32.196 describes per-interface RRM as 802.11k neighbor reporting.
- Section 3.129.15 describes Wi-Fi-system RRM as enabling RRM and BSS Transition
  Management together; independent 802.11k/11v behavior is not established here.
- Section 3.129.1 selects APs automatically for requested bands.
- Section 3.129.3 binds WLAN objects to bridges, rebuilds a WLAN on binding changes,
  and limits same-bridge WLAN objects to non-overlapping band sets.
- Section 3.178.88 provides the read-only manager membership status used above.

Therefore, a per-AP mapping cannot simply issue a manager-wide command or create
another overlapping same-bridge object and call it equivalent. The diagnostic
does not authorize such changes. A manager backend needs explicit resource
ownership, exact runtime AP mapping, binding/band validation, reversible policy
state and tests of the requested 802.11k/11v combinations. AP-only settings and
manager policy also need an established reconciliation model.

This investigation found no established per-AP equivalent in the inspected
documentation. It does not prove absence from the firmware. The port retains
explicit rejection of enabled BSS-transition requests, and does not advertise
support based on this diagnostic.

## Software checks

Tests cover alias resolution, canonical IDs, ambiguous/unresolved names,
duplicate/invalid structure, unknown flags, nested WPS fields and omission of
synthetic secrets. CLI subprocess tests verify no state-directory creation,
only the two fixed read commands, and action/planning conflict rejection before
native access. Unknown fields are not forwarded into output.
