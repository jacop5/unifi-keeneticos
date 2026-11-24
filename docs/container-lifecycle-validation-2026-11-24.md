# Container package lifecycle validation — 2026-11-24

Clean candidate `0d0063ffde8578c498eb0baa4c2e1f14145b5959` passed both
`tools/check-release.py --require-clean` and the separately invoked
`tools/check-container-lifecycle.py`.

The lifecycle runner used the already-present arm64 Linux image
`andrius/asterisk:latest`, pinned at execution to image ID
`sha256:96cefb56ddc0f57934e63b3ad3f6f12beefa3101f5bf181f948279d27fe22821`.
Its normal entrypoint was bypassed. Docker networking was disabled; no ports,
controller connections or router access were used. The only host mount was the
private synthetic test input directory, read-only. Container cleanup was verified
by the runner and by a subsequent empty lifecycle-container inventory.

The installer, space checker and service were copied unchanged from the shipped
packaging. They used their actual `/opt` paths inside the disposable container.
The agent was the real Linux arm64 executable built from the current source.
Two artifacts had different Go build IDs so byte-level replacement/restoration
could be checked; this does not validate interoperability between source versions.
The synthetic native runner accepted only specified read-only telemetry commands
and recorded any other invocation as a failure.

All six checks passed:

1. First installation remained disabled and stopped.
2. The shipped service started the real executable and established process identity.
3. Successful upgrade stopped the previous agent, installed the new artifact and
   preserved configuration, synthetic adopted state, credential-cache fixture,
   slot/network mappings and private permissions. It left the service stopped.
4. Explicit start after upgrade succeeded.
5. A rename failure injected after binary/service replacement restored the prior
   artifact and restarted the real agent through the shipped service. No unfinished
   installation markers remained after recovery.
6. Clean stop succeeded and no native write command was attempted.

The fixture's initial native-command path was corrected to match the service's
restricted PATH before the clean candidate run. No production workaround or
service-path change was needed.

The installer space check was also corrected: it previously budgeted one new
binary plus headroom, omitting concurrent backup copies. It now counts all staged
replacement files and existing replacement-target backups before leaving 16 MiB.
Boundary tests cover first install, upgrade and an existing configuration that
must be preserved rather than copied. The full installer/space suite passed
18 tests; the release gate also passed both MIPS packages/manifests and offline
source rebuilds.

Reports are local ignored artifacts at `dist/validation/container-lifecycle.json`
and `dist/validation/offline-release.json`. No firmware or real adoption credentials
were used. The main router was inspected for capacity only and was not used for
this lifecycle test. Its temporary agent remains in log-only mode.

MIPS runtime behavior, hardware-installed upgrades, physical reboot, interrupted
power/storage writes and cross-version recovery/watchdog lifetime remain open.

## Detached watchdog and startup recovery extension

Clean candidate `9b98230` subsequently passed all eight container checks and the
full offline release gate. The runner now disables inherited image healthchecks
as well as Docker networking. The two new scenarios use narrowly allowed,
file-backed synthetic AP mutations; no real interface or router is involved.

The actual `-rollback-probe` crash path created a mode-600 journal, armed its
watchdog in a separate session, changed the simulated AP and exited with code 77.
The test verified the watcher's session identity and executable against artifact B.
It then installed artifact A through the shipped installer. Linux reported the
old watcher's executable as `/opt/sbin/ukn (deleted)`; its bytes still matched B.

After the real 65-second watchdog delay, the old watcher replayed the inverse
exactly once, removed the journal and restored the simulated AP. Its persistent
journal lock retained the same inode across installation and recovery.

A second scenario reused the real persisted journal as normal startup recovery
input. The simulated native runner rejected and recorded any telemetry request
while the tentative AP state still existed. Starting the new agent replayed the
inverse before telemetry, removed the journal and retained the original normal
journal lock inode. Adopted-state, cache and mapping fixtures and configuration
permissions remained intact. The agent then stopped cleanly; container removal
was verified by the runner.

These results establish process/file lifetime and ordering for the tested Linux
arm64 environment and same-source artifacts. They do not establish compatibility
with older journal formats, a MIPS kernel, reboot or interrupted storage writes.
