#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
set -eu
cd "$(dirname "$0")"
[ "$(id -u)" = 0 ] || { printf '%s\n' 'Run as root under Entware' >&2; exit 1; }
[ -d /opt/etc/init.d ] || { printf '%s\n' 'Entware is required' >&2; exit 1; }
command -v ndmc >/dev/null || { printf '%s\n' 'Keenetic ndmc is required' >&2; exit 1; }
command -v sha256sum >/dev/null || { printf '%s\n' 'sha256sum is required' >&2; exit 1; }
[ -r SHA256SUMS ] || { printf '%s\n' 'Package integrity manifest is required' >&2; exit 1; }
for file in ukn install.sh space-check.sh S99unifi-keenetic unifi-keenetic.conf LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt; do
 awk -v expected="$file" '$2 == expected { found=1 } END { exit !found }' SHA256SUMS || { printf 'Missing package hash: %s\n' "$file" >&2; exit 1; }
done
sha256sum -c SHA256SUMS || { printf '%s\n' 'Package integrity check failed; nothing installed' >&2; exit 1; }
for file in LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt; do
 [ -r "$file" ] || { printf 'Missing required license file: %s\n' "$file" >&2; exit 1; }
done
. ./space-check.sh
check_install_space ./ukn /opt
# Prepare every replacement and backup before stopping the installed agent.
umask 077
lock=/opt/.ukn-install.lock
mkdir "$lock" 2>/dev/null || { printf '%s\n' 'Installation lock exists; inspect the previous installation before retrying' >&2; exit 1; }
work=''
committing=0
stopped=0
was_running=0
keep_backup=0
targets=''
service=/opt/etc/init.d/S99unifi-keenetic
finish_install() {
 result=$?
 trap - 0 HUP INT TERM
 set +e
 if [ "$result" != 0 ]; then
  restore_failed=0
  if [ "$committing" = 1 ]; then
   for target in $targets; do
    if [ -f "$work/backup/$target.absent" ]; then
     rm -f "/opt/$target" || restore_failed=1
    else
     mkdir -p "$work/restore/$(dirname "$target")" &&
      cp -p "$work/backup/$target" "$work/restore/$target" &&
      mv -f "$work/restore/$target" "/opt/$target" || restore_failed=1
    fi
   done
  fi
  if [ "$restore_failed" = 0 ] && [ "$stopped" = 1 ]; then
   "$service" start && "$service" status >/dev/null 2>&1 || restore_failed=1
  fi
  if [ "$restore_failed" != 0 ]; then
   keep_backup=1
   printf 'Installation recovery needs review; backups retained at %s\n' "$work" >&2
  fi
 fi
 if [ -n "$work" ] && [ "$keep_backup" = 0 ]; then rm -rf "$work"; fi
 rm -f "$lock/work-directory"
 rmdir "$lock" 2>/dev/null || true
 exit "$result"
}
trap finish_install 0
trap 'exit 1' HUP INT TERM
for previous in /opt/.ukn-install.*; do
 [ "$previous" = "$lock" ] && continue
 [ ! -e "$previous" ] || { printf 'Unfinished installation data at %s; inspect it before retrying\n' "$previous" >&2; exit 1; }
done
work=$(mktemp -d /opt/.ukn-install.XXXXXX)
printf '%s\n' "$work" > "$lock/work-directory"
# Reject links/non-files rather than replacing a different installation target.
stage_file() {
 source=$1
 target=$2
 mode=$3
 if [ -e "/opt/$target" ] || [ -L "/opt/$target" ]; then
  [ -f "/opt/$target" ] && [ ! -L "/opt/$target" ] || { printf 'Unsupported installation target: %s\n' "$target" >&2; return 1; }
 fi
 mkdir -p "$work/new/$(dirname "$target")" "$work/backup/$(dirname "$target")" "/opt/$(dirname "$target")"
 cp "$source" "$work/new/$target"
 chmod "$mode" "$work/new/$target"
 cmp -s "$source" "$work/new/$target" || { printf 'Staged copy differs: %s\n' "$target" >&2; return 1; }
 if [ -e "/opt/$target" ]; then
  cp -p "/opt/$target" "$work/backup/$target"
 else
  : > "$work/backup/$target.absent"
 fi
 targets="$targets $target"
}
stage_file ./ukn sbin/ukn 755
stage_file ./S99unifi-keenetic etc/init.d/S99unifi-keenetic 755
stage_file LICENSE share/licenses/unifi-keeneticos/LICENSE 644
stage_file NOTICE share/licenses/unifi-keeneticos/NOTICE 644
stage_file licenses/Go-BSD-3-Clause.txt share/licenses/unifi-keeneticos/Go-BSD-3-Clause.txt 644
stage_file licenses/model-catalogue-MIT.txt share/licenses/unifi-keeneticos/model-catalogue-MIT.txt 644
if [ ! -e /opt/etc/unifi-keenetic.conf ] && [ ! -L /opt/etc/unifi-keenetic.conf ]; then
 stage_file ./unifi-keenetic.conf etc/unifi-keenetic.conf 600
fi
# Probe the exact staged executable before stopping the existing service.
"$work/new/sbin/ukn" -probe >/dev/null
mkdir -p /opt/var/lib/unifi-keenetic
chmod 700 /opt/var/lib/unifi-keenetic
printf '%s\n' $targets > "$work/targets"
printf '%s\n' 'prepared' > "$work/phase"
if [ -e "$service" ]; then
 [ -x "$service" ] || { printf '%s\n' 'Existing service is not executable; inspect it before upgrading' >&2; exit 1; }
 if "$service" status >/dev/null 2>&1; then
  was_running=1
 else
  service_status=$?
  [ "$service_status" = 3 ] || { printf '%s\n' 'Cannot establish existing service status; nothing replaced' >&2; exit 1; }
 fi
fi
if [ "$was_running" = 1 ]; then
 if "$service" stop; then
  :
 else
  if "$service" status >/dev/null 2>&1; then
   :
  else
   service_status=$?
   [ "$service_status" != 3 ] || stopped=1
  fi
  printf '%s\n' 'Service stop failed; nothing replaced' >&2
  exit 1
 fi
 if "$service" status >/dev/null 2>&1; then
  printf '%s\n' 'Service is still running; nothing replaced' >&2
  exit 1
 else
  service_status=$?
  [ "$service_status" = 3 ] || { printf '%s\n' 'Service stop could not be verified; nothing replaced' >&2; exit 1; }
 fi
 stopped=1
fi
committing=1
printf '%s\n' 'committing' > "$work/phase"
for target in $targets; do mv -f "$work/new/$target" "/opt/$target"; done
printf '%s\n' 'Installed. Configure /opt/etc/unifi-keenetic.conf, then run /opt/etc/init.d/S99unifi-keenetic start.'
