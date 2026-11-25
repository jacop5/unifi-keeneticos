# Controller preset log-only check — 2026-11-25

Actual UniFi OS Server / Network 10.5.67 and a Titan KN-1810 running KeeneticOS
5.1.9 were used. The Mac was connected through Wi-Fi. The temporary agent was
revision `27710ed`, not the newer repository candidate `2cdca7b`; this run does
not validate deployment of the newer telemetry or rekey-reporting changes.

Before mutation, the process executable and command line were inspected. The
agent had `-wlan-test-only`, the explicit two-band AP6 mapping and no `-apply`.
Provisioning status independently reported application disabled. The dedicated
controller AP group contained only the Titan.

The existing dedicated test WLAN used WPA2 Personal, WPA3 off, PMF disabled and
fast roaming off. Both mapped WLANs were rejected solely for BSS Transition.
A fresh complete profile backup was retained privately, including its secret;
only `bss_transition` was changed, from true to false.

- The agent received configuration `e5e98d5c897c1123`, entered `log-only` and
  reported no unsupported settings. Application stayed disabled.
- Eight ignored fields remained visible: DTIM/U-APSD on both bands, plus minimum
  data/management/beacon rates and CCK-rate selection on the first band.
- A `finally` restoration changed the flag back to its freshly saved value.
  Comparing every profile field against that backup found no differences.
- The agent returned to explicit BSS Transition rejection on both test bands,
  with application still disabled.
- Native running configuration, credential receipt bytes and selected adoption
  identity fields matched the retained pre-test baseline. No pending rollback
  journal existed. Raw private records remained on the router.

The normalized native configuration SHA-256 remained
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
Normalization removes terminal escapes, carriage returns, empty lines and
generated `! $$$` comments; it does not discard configuration commands.

This establishes controller-to-planner compatibility for the selected profile,
not native provisioning, client connectivity, roaming or complete feature parity.
The baseline predates the brief profile mutation; the final equality covers the
whole interval, including intervening local-only development.
