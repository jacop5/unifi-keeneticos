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

## Adoption-state persistence extension

Clean candidate `45a7a4a` passed the full offline release gate and all nine
container checks. The added check runs the real device-package filesystem tests
as a Linux arm64 test executable before installation/lifecycle scenarios.

Adoption saves now create a unique mode-600 staging file, synchronize its data,
close it, atomically replace the destination and synchronize the parent directory.
The fixed legacy `.tmp` path is not opened or deleted. This shared device-package
change applies to both backends; the stored JSON schema remains unchanged.

Race-enabled host tests and Linux container tests verify a legacy temporary
symlink does not receive credentials or damage its target, a public destination
is replaced privately, rename failure cleans up the new stage, and concurrent
saves leave complete records without leaked stages. These are filesystem/privacy
tests, not interrupted-power tests. A directory-sync error after replacement is
returned to the caller; the new state may already be visible at that point.

No live router or controller state was changed during this validation. Hardware
storage durability remains open.

## Persistence failure and controller acknowledgement

Clean candidate `8833382` passed the full offline release gate and all ten
container checks. The additional Linux check runs the actual encrypted exchange
code against an in-process loopback controller fixture; Docker networking remains
disabled and no external controller or router is contacted.

The fixture sends a new adoption key, GCM negotiation and an LED action, then
blocks state-file replacement. The first exchange returns a persistence error;
the next attempt retries the local save without sending another HTTP inform.
Neither attempt dispatches the action or changes the inform interval. The new
key and outcome remain pending in memory.

After the filesystem obstacle is removed, the agent saves the new state before
the next encrypted inform. The controller fixture verifies the key is already
on disk when that inform arrives. The deferred action runs once, and another
successful exchange does not replay it. Adoption event logging and changed-reply
actions now occur after a successful state save. The SSH adoption path also
saves before publishing its adoption events.

The pending outcome is not durable independently of the state file. A process
crash before storage recovers can still lose the negotiated key and require
adoption recovery. This test does not establish physical power-loss behavior or
atomicity across all controller-configuration files.

## Bounded service process-establishment checks — 2026-11-25

Clean candidate `d724e0a` passed the full offline release gate and all thirteen
container checks. The shipped service now uses its existing executable/PID/zombie
criteria during enabled startup, waits up to five seconds to establish that
process and checks again after one second. A valid already-running process is
kept rather than launched again.

Two new real-agent container scenarios verify a second start preserves the same
PID and an invalid saved-state file makes startup return failure when the daemon
exits immediately. Restoring the private state fixture permits a subsequent start;
the upgrade, failed-upgrade restart and detached-watchdog checks still pass.

Option-only tests mock process observation independently of argument validation;
status tests retain simulated live/stale/unrelated/zombie PID coverage, and the
container exercises the real Linux process observer. Installation-marker tests
continue to cover admission separately from process liveness.

These checks do not wait for all native initialization or establish controller/WLAN
health, nor do they guarantee the daemon remains alive after the observation
window. Native BusyBox/MIPS startup and hardware boot validation remain open.

## Same-executable instance selection — 2026-11-25

Clean candidate `f0e999b` passed the full offline release gate and all fourteen
container checks. The service now requires its leading `-state` argument to
match the selected state directory in addition to executable/PID/zombie checks.
Later state overrides and diagnostic/recovery command lines are rejected. Stop
checks the numeric PID and signals that checked value, rather than allowing a
launcher to reread a changed PID file.

The new real-process scenario starts another agent from the same installed ELF
with a different private state directory. It deliberately points the selected
service PID file at that process. Status reports it as unselected and stop returns
failure without terminating it. The test verifies that process is still alive,
restores the selected PID file, checks the selected instance and separately stops
the fixture's second instance. All upgrade/recovery scenarios continue to pass.

Unit cases cover state paths containing spaces, missing/late/overridden state
arguments, truncated command lines, diagnostic/watchdog flags and the existing
invalid/unrelated/zombie PID cases. Newlines in `STATE_DIR` are rejected because
the shell observer decodes NUL-separated arguments into records. This selector
supports service-generated argument ordering; it is not a general command-line
parser or an authentication boundary. It does not make PID checking and signalling
atomic against process-generation changes.

No live router changes were made. Native BusyBox/MIPS service behavior and
hardware-installed lifecycle validation remain outstanding.
