# Speedster handoff after the Titan deployment

The Titan is already installed and managing the two real WLANs. Preserve that
working deployment while adding the repeater. The accepted binary is source
`654dc52`; its two MIPS packages passed the offline release gate. Deployment and
client evidence are in [the AP-mode report](ucg-ap-mode-validation-2026-11-27.md).

## Information required when the operator returns

- Speedster model/revision, current address and SSH port/account.
- Whether Entware is installed and provides a Linux shell. Native Keenetic CLI
  login is a different interface; do not run an Entware installer blindly there.
- Whether the wireless uplink is independent or controlled by Keenetic Wi-Fi
  System, and which device owns the member WLAN configuration.

## Read-only preflight

1. Identify the actual hardware, firmware, mode, native AP/WifiStation layout,
   wired recovery path and mounted writable storage. Keep the uplink unchanged.
2. Check executable ELF byte order; do not infer it from `uname mips` or reuse a
   model assumption. Select the verified matching package and check its manifest.
3. Back up the native configuration privately on-device. Inspect Wi-Fi System
   ownership. If its manager owns the member APs, keep the port log-only until
   the configuration authority is resolved; two writers are not a tested setup.
4. Use a fresh private adoption state for this device. Never clone the Titan's
   MAC, authkey, receipts or SSH host keys. The Titan's administrator-assisted
   bootstrap was explicitly approved for that device; it is not proof of
   automatic Network 11 adoption or approval for another key transfer.

## Controlled addition

- The production UCG AP group is currently `Keenetic-Titan` and contains only the
  Titan. Adding the Speedster requires intentional controller targeting, then
  inspection of its own verified configuration IDs and native slots. Do not copy
  the Titan's numeric slot map without checking those identities.
- Start log-only. For independent WLAN ownership, use production `WLAN_ONLY=1`
  with explicit slots so the repeater's shared radio and WifiStation uplink stay
  outside provisioning. This does not remove native driver/DFS side effects.
- Current production profiles retain their separate bands and WPA2/WPA3
  transition mode. FT is off; successful Titan access does not establish roaming
  or repeater interoperability. Inspect ignored/unsupported settings first.
- Test client authentication, DHCP, DNS and interface-bound traffic on the new
  AP with native/controller association evidence. Verify the wireless uplink and
  the Titan remain healthy, then validate installed startup on the spare device.

Physical reboot acceptance and automatic Network 11 SSH adoption remain open.
No Speedster contact or configuration changes were performed overnight.
