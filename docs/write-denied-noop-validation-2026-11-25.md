# Real-controller no-op confirmation — 2026-11-25

Clean candidate `1b0d2cc` was tested on the Titan against actual UniFi OS Server /
Network 10.5.67 through a deliberately read-only native-command wrapper. This
checks idempotent provisioning and confirmation, not positive configuration writes
or client enforcement. The Mac remained connected through Wi-Fi.

The staged linux/mipsle softfloat binary matched its release report SHA-256
`84f0e17f0d74b6cb257359f3a576825371390cb52ec370d6092ac91dbaa48f79`.
Native build metadata confirmed the clean revision. The previous unused probe
artifact was retired only after exact hash/process checks; the existing `ef31a77`
log-only binary was retained for restoration.

## Native write boundary

A private wrapper named `ndmc` preceded the native command in the test agent's
PATH. It forwarded only an explicit list of read-only status/configuration queries
to `/bin/ndmc`; every other invocation recorded a fixed denial marker and exited.
The wrapper was self-tested for both denial and allowed read access. The agent's
actual process environment was checked for the guarded PATH and denial-log path.
No LED/reboot/client-block or other action opt-in was enabled. The wrapper would
also be inherited by any watchdog, but no transaction/watchdog was needed here.

The candidate used the real `-apply` confirmation path with `-wlan-test-only` and
the existing explicit AP6 mappings. Receipt filtering, preimage preparation,
native health/readback and encrypted-controller confirmation were unchanged.
The wrapper was a test constraint; it is not installed as a production backend.

## Controller scope and outcome

The dedicated test WLAN's AP group was freshly checked to contain only the Titan,
despite another adopted device existing elsewhere on the controller. A fresh full
profile backup stayed private. Only BSS Transition on that dedicated WLAN was
temporarily disabled to reach the conservative supported preset. No global block
command was sent.

The first attempt encountered an expired controller login before any profile
mutation. Cleanup restored the log-only agent and established native equality.
After renewing the authorized session, the repeated test established:

- Configuration `a924ebe17b58e8ae` reached `confirmed` through the real controller.
- The native plan prepared **zero changes** and reused **two credential receipts**.
- No denial marker was generated during the agent run: no native write was even
  attempted. No rollback journal was created.
- Every controller profile field matched the fresh backup after the one flag was
  restored. The received version changed again on restoration, as expected.
- The prior `ef31a77` executable was restarted in log-only mode with application
  and block control disabled. BSS Transition rejection resumed on both test bands.
- Native configuration, credential receipt bytes and selected adoption identity
  fields matched their private pre-test baselines.

Normalized native configuration SHA-256 remained
`2ca5600874df4cda00a4e6f8c150f4b393bda75006cc3cdab997f8c42d5ff77b`.
The current report retains the new historical confirmation separately from the
restored profile's unsupported/log-only state. Raw router configuration and
preimages remained on the router; the private handoff records the restored PID.

This verifies the selected candidate's real-controller, native-readback no-op path
without risking a WLAN rewrite. It does not replace positive applying-mode tests,
active client blocking, two-band client traffic acceptance or installed boot tests.
