# Cross-revision agent recovery — 2026-11-25

The extended container lifecycle suite passed all fifteen checks using agent A
from `62b1285bd26bdac3499f2e5bd2da426a14d5f292` and agent B from the current
`dcd8467` source. Only the runner, fixture and documentation were modified during
this run; the report records `modified: true`. Both artifacts used the current
packaging scripts, so this is agent compatibility evidence, not proof of an
upgrade from an entire historical installed package.

Command (requires the already-present image and cached Go dependencies):

```sh
python3 tools/check-container-lifecycle.py \
  --image andrius/asterisk:latest \
  --previous-agent-revision 62b1285 \
  --report dist/validation/cross-revision-lifecycle.json
```

The runner resolves the historical commit from local Git, exports it privately,
builds its agent with dependency downloads disabled and records both full commit
IDs and executable SHA-256 values. Packaging comes from the current tree.
The default invocation still builds two same-source artifacts with different
build IDs. No repository checkout or router files are changed.

The actual installer/service and Linux arm64 agents ran in a disposable container
with networking disabled, using synthetic native telemetry and an in-process
loopback controller fixture. The existing fourteen lifecycle checks passed across
the selected agent revisions, including replacement/restoration, private-state
preservation, detached-watchdog recovery after executable replacement, and recovery
of B's persisted journal by A before daemon telemetry.

The fifteenth check exercised forward journal compatibility: A created an actual
crash-probe journal; the fixture moved it to the normal startup journal location,
installed B, then started B. B restored the simulated AP before native telemetry,
removed the journal and performed the inverse exactly once. Persistent lock inode,
configuration, adoption fixture, mappings and credential receipt bytes were
preserved. This is a synthetic journal handoff, not a power interruption during
provisioning or an automatic downgrade scenario.

An initial fixture attempt enabled the telemetry-order sentinel before the
installer's deliberate read-only `-probe`, incorrectly rejecting that probe.
Moving the sentinel immediately before daemon start corrected the fixture; no
production behavior was weakened. The final complete run passed and verified
container removal. Its ignored local report is the path shown above.

Remaining: historical service/package migrations, MIPS execution of these cases,
real storage power-loss behavior, physical boot and native WLAN enforcement.
The selected pair does not establish compatibility with all past/future versions.
The household router and local UniFi OS configuration were untouched by this run.
