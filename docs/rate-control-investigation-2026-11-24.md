# Rate-control investigation — 2026-11-24

Scope: minimum WLAN data/management/beacon rates, DTIM/U-APSD, transmit power
and per-client shaping on the Titan KN-1810 running KeeneticOS 5.1.9. Management
was over Wi-Fi. Only firmware-library inventory, hashes and copies were read
from the router. No native settings or services were changed.

## Documented controls and scope

Source: the supplied [KeeneticOS 5.1 Titan CLI manual](https://storage.googleapis.com/docs.help.keenetic.com/cli/5.1/en/cli_manual_kn-1810.pdf),
read from the cached PDF/text.

| Control | Documented semantics | Consequence for this port |
| --- | --- | --- |
| 3.32.183 `interface power` | Radio-wide percentage of hardware/regulatory maximum, 1–100; no `no` form | No established conversion from controller power requests; retain ignored reporting rather than assume numeric equivalence |
| 3.32.227 `interface traffic-shape` | Interface-wide download/upload cap, in Kbps | Not established as a per-client WLAN limit |
| 3.89 `ip traffic-shape host` | Known-host MAC download/upload cap, optionally scheduled; no WLAN selector in the documented syntax | A candidate for a separate host-policy feature, not a direct VAP-scoped mapping. Ownership, movement between WLANs, existing operator limits and recovery need design first |
| 3.32.192 `interface rekey-interval` | Wi-Fi rekey interval in seconds; example addresses `WifiMaster0` | Do not infer a per-WLAN control from the command name; its scope needs confirmation before translating per-WLAN requests |

Searches of this manual for DTIM, U-APSD/APSD and minimum/basic/beacon rate
controls did not identify a documented mapping. This is a bounded documentation
search, not proof that the firmware or driver lacks those capabilities.

## Static firmware evidence

The copies below matched SHA-256 values measured on the router. They remain
private local analysis inputs; firmware is not committed or redistributed.

| File | Bytes | SHA-256 | REA parent Evidence |
| --- | ---: | --- | --- |
| `libndmWifiMtk.so` | 1,021,620 | `7ade9e25187d0e691eb369eae6faa6fa4f2f1d54252b60a0c2354f3d4983e4f7` | `ev_2200eaf3484538b029d975db9061d8daab5927f8b5808f3955f0e7f58ba016c0` |
| `libndmCorePack.so` | 11,002,432 | `9ca15ee910ed1b00bfff73c45aa82347a65e2cbf35022066622818f95ba998ba` | `ev_60471033d076f45fde757dc5b38abe799eddcce3071a8701fffb951f39a1c88b` |

REA's built-in ELF export reader reported complete dynamic-export inventories:
655 and 12,327 exports respectively. These are 32-bit little-endian MIPS files.
The Wi-Fi library exports `WifiMaster::SetTxPower` at `0x4cc7c` and
`WifiMaster::SetRekeyInterval` at `0x50294`. The core
library exports radio TX-power boundary symbols and generic network/policy rate
limit setters. Selected Wi-Fi export-name searches did not find a minimum/basic
rate, DTIM or U-APSD setter.

Export inventories exclude local, hidden and undefined symbols. Names and
addresses do not establish implementation semantics, CLI registration, call
paths, driver behavior or runtime reachability. No decompilation or runtime
tracing was performed: the connected REA session reported no configured native
analysis engine. Do not turn this limited negative result into a claim that
these features cannot be implemented.

The retained Evidence bundle was checkpointed privately at
`/tmp/ukn-native-rates.T2x89e/evidence.json` before resetting the investigation.
It contains the export records and earlier retained web-investigation evidence;
it is not a public release artifact. The corresponding local firmware copies
are in that same private directory.

## Implementation decision and next evidence

Existing minimum-rate and bandwidth diagnostics remain accurate: these requests
are not applied. No speculative native mapping was added.

For rate/DTIM/U-APSD work, the next useful evidence is a command/driver registration
and implementation trace with a suitable native analysis engine, followed by
readback, exact preimage/inverse handling and spare-device behavior tests. For
host shaping, first define explicit ownership and WLAN-association lifetime so
a guest WLAN's limit cannot silently become a permanent host-wide policy.
