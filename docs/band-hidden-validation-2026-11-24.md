# Titan 2.4 GHz and hidden-SSID validation — 2026-11-24

Candidate `632f35be62fe30ea106c75a84c31b90754222bc5`, the same clean binary
as the [controller validation](controller-validation-2026-11-24.md).
SHA-256: `db68258fcd35f531866b725726022e0a0213ce1747ee1234f13450d93421e684`.
Titan KN-1810 / KeeneticOS 5.1.9 / MIPS little-endian softfloat;
UniFi Network 10.5.67; Mac Ethernet remained connected.

## Scope

Only the dedicated test WLAN was changed. Its 2.4 GHz AP6 remained enabled,
while its 5 GHz AP6 was disabled to force the client onto the tested band.
Main AP0 WLANs and shared radios stayed up. BSS transition was disabled for
the supported test preset; DTIM and U-APSD remained explicitly ignored.

Before application, the received plan passed the scope checker. Private
on-router inverses covered credentials, SSID, hiding and 5 GHz test-AP state.
A fresh complete native snapshot matched the previous restored baseline.

## Results

| Case | Evidence |
| --- | --- |
| Visible 2.4 GHz profile | Confirmed configuration `a1aca510ce29c944`; 5 GHz test AP read back down |
| Visible client | Native WPA2-PSK authentication on `WifiMaster0/AccessPoint6`, correlated by native ARP to the Mac's Wi-Fi IPv4 address; DHCP ACK recorded |
| Visible data path | DNS query for `github.com` to the DHCP-advertised DNS server succeeded through an IPv4 socket bound to `en0`; HTTPS to that returned address also bound to `en0`, used the expected source address and returned 200 |
| Hidden profile | Confirmed configuration `849049030391f2db`; native AP6 `hide-ssid` flag present |
| Hidden client | Temporary test preference removed, Mac Wi-Fi adapter cycled, explicit known-SSID join completed; native WPA2 authentication and ARP correlation again identified 2.4 GHz AP6 |
| Hidden data path | DHCP ACK, Wi-Fi-bound DNS and Wi-Fi-bound HTTPS 200 passed again |
| Cleanup | Original controller bands, BSS setting and visibility restored; full native baseline compared equal; test preference removed; temporary helper retired |

DNS binding used the installed macOS SDK's `IP_BOUND_IF` socket option, verified
with `getsockopt`, plus the Wi-Fi source address. HTTPS used `curl --interface
en0 --resolve`, with the address returned by that DNS response and normal TLS
verification. The DHCP DNS server was discovered from the recorded DHCP ACK,
not assumed to be the router. No DNS or DHCP settings were changed.

The earlier failed test endpoint returned NXDOMAIN from that DNS server over
both Wi-Fi and Ethernet. This was not a Wi-Fi-only timeout. That endpoint was
retired at the operator's request; the successful checks above used `github.com`.
The server's reason for its negative response was not investigated.

Native comparisons stayed on-router and excluded only terminal formatting,
blank lines and generated `! $$$` comments. Before/after SHA-256:
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
Controller comparison also included the hidden-SSID field, which the older
generic profile checker did not cover. No raw configurations, keys or client
identifiers are included here.

## Limits

This verifies the native hiding setting and an explicit join with a known SSID,
not passive-scan absence or beacon contents. It does not establish automatic
reconnection, roaming, cross-client isolation, WPA3/OWE or installed lifecycle
behavior. The later credential-syntax guard in `9de0efd` was not deployed for
this run; do not treat this as a live pass for that newer executable.
