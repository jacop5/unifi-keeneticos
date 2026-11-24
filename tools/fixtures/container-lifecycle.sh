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
