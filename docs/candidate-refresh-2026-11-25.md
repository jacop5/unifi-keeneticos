# Titan log-only candidate refresh — 2026-11-25

The temporary Titan agent was refreshed from `27710ed` to clean candidate
`ef31a776e35c9129adf12be9077514ee502c0de3`, after its offline release and
sixteen-check Linux container validation. This was a temporary process refresh,
not an installed-package upgrade or boot test.

The MIPS little-endian softfloat executable was uploaded to a unique private
test path. Its SHA-256 matched the local release report:
`099d6b10c124189b3c753bef102c3b07b371d0da51f202490dd303888b56fedb`.
On-router `-build-info` reported that revision, Go 1.27.4, linux/mipsle softfloat
and `modified: false` before the current daemon was stopped.

Staging retained 16 MiB headroom. The older unused `62b1285` test artifact was
retired only after its exact path/hash and absence from process executables were
checked. The active `27710ed` artifact was retained as fallback. Startup included
a fallback restart path; it was not needed. Only the temporary agent process was
replaced. The native configuration was not applied or saved.

The candidate command line retained the same state, controller endpoint, explicit
AP6 slot map and `-wlan-test-only`. It had no `-apply`, `-client-block-control`,
LED/reboot actions, experimental SAE or version-reporting opt-in. Status confirmed
application and block control disabled. The controller reported adopted/online
with the same received configuration version as the agent (`9c3f82f50974a49b`).

As expected for the restored controller profile, both BSS Transition requests
remained explicitly unsupported. Ten ignored fields were visible, now including
`aaa.3.wpa.group_rekey` and `aaa.4.wpa.group_rekey`. This is native execution of
the new reporting path; no block enforcement was requested or tested.

Post-refresh comparison established:

- Exact normalized native configuration equality; SHA-256 remained
  `2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
- Credential receipt bytes and selected adoption identity fields unchanged.
- No pending rollback journal, state directory mode 700, and adoption/receipt
  files mode 600.
- New process executable matched the staged candidate. The private local handoff
  was updated with the new PID, path, revision, log and retained fallback.

Raw configuration and private preimages remained on the router. This run validates
temporary startup, adoption continuity and log-only planning against actual UniFi
OS / Network 10.5.67. Applying-mode acceptance, client blocking and installed
upgrade/reboot behavior remain separate requirements.
