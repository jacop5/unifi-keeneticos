#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Run only in the disposable, network-isolated container created by the runner.
set -eu
umask 077
service=/opt/etc/init.d/S99unifi-keenetic
state=/opt/var/lib/unifi-keenetic
cleanup() {
 result=$?
 trap - 0 HUP INT TERM
 if [ "$result" != 0 ]; then
  for log in /work/*install.log /work/*upgrade.log; do
   [ ! -f "$log" ] || tail -50 "$log" >&2
  done
 fi
 if [ -x "$service" ]; then "$service" stop >/dev/null 2>&1 || true; fi
 exit "$result"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM
wait_running() {
 n=0
 until "$service" status >/dev/null 2>&1; do
  n=$((n+1)); [ "$n" -lt 50 ] || return 1
  sleep 0.1
 done
}
assert_stopped() {
 if "$service" status >/dev/null 2>&1; then return 1; else code=$?; fi
 [ "$code" = 3 ]
}
assert_clean() {
 for pending in /opt/.ukn-install.*; do
  [ ! -e "$pending" ] && [ ! -L "$pending" ] || return 1
 done
}
assert_preserved() {
 for file in etc/unifi-keenetic.conf etc/slots.json etc/networks.json var/lib/unifi-keenetic/state.json var/lib/unifi-keenetic/credential-receipts.json; do
  cmp -s "/work/preimage/$file" "/opt/$file"
 done
 [ "$(stat -c %a "$state")" = 700 ]
 [ "$(stat -c %a "$state/state.json")" = 600 ]
 [ "$(stat -c %a "$state/credential-receipts.json")" = 600 ]
 [ ! -e "$state/rollback.json" ]
}

test ! -e /opt/sbin/ukn
mkdir -p /opt/etc/init.d /work /work/preimage
ln -s /test/ndmc /usr/sbin/ndmc
/test/device-tests -test.run='^(TestStateSaveIgnoresLegacyTempSymlinkAndRepairsPermissions|TestFailedStateReplacementCleansPrivateStage|TestConcurrentStateSavesKeepCompletePrivateRecords|TestStoredConfigMatchesVersionAndStaysPrivate)$'
printf 'PASS private-atomic-adoption-state\n'
/test/ukn-tests -test.run='^TestFailedAdoptionSaveBlocksAcknowledgementAndDefersActions$'
printf 'PASS durable-adoption-before-controller-ack\n'
/test/ukn-tests -test.run='^(TestConfigIdentityChangesWithBodyAndRejectsUnboundData|TestConfigSnapshotDaemonRefreshesLegacyAndIgnoresDiagnosticFailure)$'
printf 'PASS version-bound-controller-configuration\n'
/test/ukn-tests -test.run='^(TestBlockedApplyConfirmsOrRollsBackAcrossEncryptedControllerExchange|TestDaemonReplansBlockedListWithoutConfigurationVersionChange|TestDaemonRestoresBlockedBaselineFromDurableState)$'
printf 'PASS guarded-block-policy-controller-confirmation\n'
sh /test/package-a/install.sh > /work/first-install.log 2>&1
assert_clean
assert_stopped
"$service" start > /work/disabled-start.log 2>&1
test ! -e /opt/var/run/unifi-keenetic.pid
grep -q '^ENABLED=0$' /opt/etc/unifi-keenetic.conf
printf 'PASS first-install-disabled\n'

cp /test/enabled.conf /opt/etc/unifi-keenetic.conf
cp /test/state.json "$state/state.json"
printf '{"schema_version":1,"records":[]}\n' > "$state/credential-receipts.json"
printf '{"1":"WifiMaster0/AccessPoint6"}\n' > /opt/etc/slots.json
printf '{"20":"Bridge1"}\n' > /opt/etc/networks.json
chmod 600 /opt/etc/unifi-keenetic.conf "$state/state.json" "$state/credential-receipts.json" /opt/etc/slots.json /opt/etc/networks.json
for file in etc/unifi-keenetic.conf etc/slots.json etc/networks.json var/lib/unifi-keenetic/state.json var/lib/unifi-keenetic/credential-receipts.json; do
 mkdir -p "/work/preimage/$(dirname "$file")"
 cp -p "/opt/$file" "/work/preimage/$file"
done
"$service" start
wait_running
before_pid=$(cat /opt/var/run/unifi-keenetic.pid)
test "$(readlink /proc/"$before_pid"/exe)" = /opt/sbin/ukn
assert_preserved
printf 'PASS real-agent-service-start\n'
"$service" start
[ "$(cat /opt/var/run/unifi-keenetic.pid)" = "$before_pid" ]
printf 'PASS already-running-start-keeps-process\n'

# Simulate a stale PID file that names another live instance of the same ELF.
mkdir -p /work/other-state
cp -p /test/state.json /work/other-state/state.json
start-stop-daemon -S -b -m -p /work/other.pid -x /opt/sbin/ukn -- -state /work/other-state -url http://127.0.0.1:9/inform -ssh-listen ''
sleep 1
other_pid=$(cat /work/other.pid)
test "$(readlink /proc/"$other_pid"/exe)" = /opt/sbin/ukn
cp -p /opt/var/run/unifi-keenetic.pid /work/selected.pid
cp /work/other.pid /opt/var/run/unifi-keenetic.pid
assert_stopped
if "$service" stop > /work/other-instance-stop.log 2>&1; then
 printf 'Service stop claimed an unrelated state instance\n' >&2
 exit 1
fi
kill -0 "$other_pid"
test "$(readlink /proc/"$other_pid"/exe)" = /opt/sbin/ukn
cp /work/selected.pid /opt/var/run/unifi-keenetic.pid
wait_running
kill -TERM "$other_pid"
n=0
while [ "$(readlink /proc/"$other_pid"/exe 2>/dev/null || true)" = /opt/sbin/ukn ]; do
 n=$((n+1)); [ "$n" -lt 50 ] || exit 1
 sleep 0.1
done
printf 'PASS same-executable-other-state-is-not-stopped\n'

sh /test/package-b/install.sh > /work/successful-upgrade.log 2>&1
assert_clean
assert_stopped
test "$(readlink /proc/"$before_pid"/exe 2>/dev/null || true)" != /opt/sbin/ukn
cmp -s /test/package-b/ukn /opt/sbin/ukn
assert_preserved
printf 'PASS successful-upgrade-preserves-private-data\n'

"$service" start
wait_running
assert_preserved
printf 'PASS explicit-restart-after-upgrade\n'

"$service" stop
assert_stopped
printf '{invalid-state\n' > "$state/state.json"
if "$service" start > /work/invalid-state-start.log 2>&1; then
 printf 'Service start falsely succeeded for a daemon that exits\n' >&2
 exit 1
fi
assert_stopped
cp -p /work/preimage/var/lib/unifi-keenetic/state.json "$state/state.json"
assert_preserved
"$service" start
wait_running
printf 'PASS startup-detects-immediate-daemon-exit\n'

# Fail after binary/service replacement. The real installer must restore the
# previous artifact and restart the real executable through the shipped service.
mkdir -p /work/fail-bin
cat > /work/fail-bin/mv <<'EOF'
#!/bin/sh
for last; do :; done
case "$2:$last" in
 */new/share/licenses/unifi-keeneticos/NOTICE:/opt/share/licenses/unifi-keeneticos/NOTICE)
  if [ ! -e /work/failed-once ]; then : > /work/failed-once; exit 1; fi;;
esac
exec /usr/bin/mv "$@"
EOF
chmod 700 /work/fail-bin/mv
if PATH=/work/fail-bin:$PATH sh /test/package-a/install.sh > /work/failed-upgrade.log 2>&1; then
 printf 'Unexpected successful injected failure\n' >&2
 exit 1
fi
test -e /work/failed-once
assert_clean
wait_running
cmp -s /test/package-b/ukn /opt/sbin/ukn
assert_preserved
printf 'PASS failed-upgrade-restores-and-restarts-real-agent\n'

"$service" stop
assert_stopped
assert_preserved
test ! -e /work/forbidden-native-command
printf 'PASS clean-stop-no-native-writes\n'
normal_lock_inode=$(stat -c %i "$state/rollback.json.lock")

# The actual crash probe arms its own setsid watchdog, writes a real journal,
# changes only the synthetic AP and exits before confirmation.
: > /work/allow-rollback-probe
if /opt/sbin/ukn -state "$state" -rollback-probe WifiMaster0/AccessPoint5 -crash-probe > /work/crash-probe.log 2>&1; then
 exit 1
else
 code=$?
fi
[ "$code" = 77 ]
test -e /work/fake-ap-ssid
test -f "$state/rollback-probe.json"
[ "$(stat -c %a "$state/rollback-probe.json")" = 600 ]
cp -p "$state/rollback-probe.json" /work/saved-rollback.json
probe_lock_inode=$(stat -c %i "$state/rollback-probe.json.lock")
watch_pid=''
for command in /proc/[0-9]*/cmdline; do
 [ -r "$command" ] || continue
 args=$(tr '\000' ' ' < "$command" 2>/dev/null || true)
 case "$args" in
  "/opt/sbin/ukn -rollback-watch $state/rollback-probe.json -rollback-id "*)
   candidate=${command#/proc/}
   candidate=${candidate%/cmdline}
   [ -z "$watch_pid" ] || exit 1
   watch_pid=$candidate;;
 esac
done
test -n "$watch_pid"
[ "$(awk '{ print $6 }' /proc/"$watch_pid"/stat)" = "$watch_pid" ]
cmp -s /proc/"$watch_pid"/exe /test/package-b/ukn
sh /test/package-a/install.sh > /work/watchdog-upgrade.log 2>&1
assert_stopped
cmp -s /opt/sbin/ukn /test/package-a/ukn
[ "$(readlink /proc/"$watch_pid"/exe)" = '/opt/sbin/ukn (deleted)' ]
cmp -s /proc/"$watch_pid"/exe /test/package-b/ukn
printf 'Waiting for the actual detached watchdog deadline\n'
n=0
while [ -e "$state/rollback-probe.json" ]; do
 n=$((n+1)); [ "$n" -lt 85 ] || exit 1
 sleep 1
done
test ! -e /work/fake-ap-ssid
[ "$(stat -c %i "$state/rollback-probe.json.lock")" = "$probe_lock_inode" ]
[ "$(wc -l < /work/inverse-calls | tr -d '[:space:]')" = 1 ]
assert_clean
assert_preserved
test ! -e /work/forbidden-native-command
printf 'PASS detached-watchdog-recovers-after-executable-replacement\n'

# Reuse a real persisted journal to verify normal startup recovers before its
# first telemetry request. Telemetry before the inverse is a fixture failure.
cp -p /work/saved-rollback.json "$state/rollback.json"
: > /work/fake-ap-ssid
: > /work/require-recovery-before-telemetry
"$service" start
wait_running
n=0
while [ -e "$state/rollback.json" ]; do
 n=$((n+1)); [ "$n" -lt 50 ] || exit 1
 sleep 0.1
done
test ! -e /work/fake-ap-ssid
[ "$(stat -c %i "$state/rollback.json.lock")" = "$normal_lock_inode" ]
[ "$(wc -l < /work/inverse-calls | tr -d '[:space:]')" = 2 ]
assert_preserved
"$service" stop
assert_stopped
test ! -e /work/forbidden-native-command
printf 'PASS startup-recovers-pending-journal-before-telemetry\n'

# Artifact A is now installed and stopped. Have it produce a journal, then
# replace it with B and prove B can recover A's pending transaction. The earlier
# startup check exercised the opposite direction (B's journal recovered by A).
rm -f /work/require-recovery-before-telemetry
if /opt/sbin/ukn -state "$state" -rollback-probe WifiMaster0/AccessPoint5 -crash-probe > /work/predecessor-crash-probe.log 2>&1; then
 exit 1
else
 code=$?
fi
[ "$code" = 77 ]
test -e /work/fake-ap-ssid
test -f "$state/rollback-probe.json"
mv "$state/rollback-probe.json" "$state/rollback.json"
sh /test/package-b/install.sh > /work/predecessor-upgrade.log 2>&1
assert_clean
assert_stopped
cmp -s /test/package-b/ukn /opt/sbin/ukn
# Installation deliberately performs a read-only -probe. Restrict ordering at
# daemon startup, after that probe, as in the earlier startup recovery case.
test ! -e /work/forbidden-native-command
: > /work/require-recovery-before-telemetry
"$service" start
wait_running
n=0
while [ -e "$state/rollback.json" ]; do
 n=$((n+1)); [ "$n" -lt 50 ] || exit 1
 sleep 0.1
done
test ! -e /work/fake-ap-ssid
[ "$(stat -c %i "$state/rollback.json.lock")" = "$normal_lock_inode" ]
[ "$(wc -l < /work/inverse-calls | tr -d '[:space:]')" = 3 ]
assert_preserved
"$service" stop
assert_stopped
test ! -e /work/forbidden-native-command
printf 'PASS replacement-agent-recovers-predecessor-journal-before-telemetry\n'
