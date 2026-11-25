# Historical package upgrade — 2026-11-25

The lifecycle runner can now select `--previous-package-revision COMMIT`, mutually
exclusive with the agent-only history option. Artifact A takes its agent source,
installer, space checker, service, default configuration and notices from that
local commit. Artifact B uses the current tree. Both agents are rebuilt with the
current compiler; these are not preserved original release binaries. Package
manifests are generated from the selected files and their hashes are recorded.

The first full historical-package run passed all seventeen checks with A at
`f0e999b` and B at `e928e36`. The runner/fixture were modified for this run and
the report explicitly records `modified: true`; production agent and packaging
sources were unchanged. Both agent and packaging commit maps are in the report.
The agent, service and default-configuration hashes differed between A and B,
confirming this was not another old-agent/current-packaging run.

```sh
python3 tools/check-container-lifecycle.py \
  --image andrius/asterisk:latest \
  --previous-package-revision f0e999b \
  --report dist/validation/historical-package-lifecycle.json
```

The initial enabled configuration is derived from A's historical template, rather
than the current template. Installation upgrades must preserve it byte-for-byte,
along with adoption, credential receipts, mappings and private permissions.
After upgrading and explicitly starting B, a new check confirms its command line
does not enable client block control and its provisioning report records that
control disabled. The newer service supplies a safe default for the option absent
from this selected historical configuration.

The suite also passed successful replacement, injected failed replacement with
restoration/restart, malformed-state startup failure, state-specific PID ownership,
the encrypted block-policy tests, detached watchdog survival across executable
replacement and startup journal recovery in both source-version directions.
The runner verified removal of the disposable container and its processes.

This used Linux arm64, actual installer/service paths and real agent executables
with synthetic native state and loopback controller fixtures. Docker networking
was disabled. The household router and actual UniFi OS controller were untouched.
The ignored local report is the path shown above; no private production state was
used. This establishes the selected source-package pair, not arbitrary historical
versions, MIPS runtime behavior, physical boot or storage power-loss durability.
