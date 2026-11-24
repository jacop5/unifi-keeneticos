#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
set -eu
cd "$(dirname "$0")"
[ "$(id -u)" = 0 ] || { printf '%s\n' 'Run as root under Entware' >&2; exit 1; }
[ -d /opt/etc/init.d ] || { printf '%s\n' 'Entware is required' >&2; exit 1; }
command -v ndmc >/dev/null || { printf '%s\n' 'Keenetic ndmc is required' >&2; exit 1; }
command -v sha256sum >/dev/null || { printf '%s\n' 'sha256sum is required' >&2; exit 1; }
command -v sync >/dev/null || { printf '%s\n' 'sync is required' >&2; exit 1; }
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
rollback_targets=''
service=/opt/etc/init.d/S99unifi-keenetic
record_phase() {
 printf '%s\n' "$1" > "$work/phase.new" || return 1
 mv -f "$work/phase.new" "$work/phase" || return 1
 sync
}
record_hash() {
 hash_line=$(sha256sum "$1") || return 1
 digest=${hash_line%% *}
 case "$digest" in ''|*[!0123456789abcdef]*) return 1 ;; esac
 [ "${#digest}" = 64 ] || return 1
 printf '%s  %s\n' "$digest" "$2" >> "$3"
}
finish_install() {
 result=$?
 trap - 0 HUP INT TERM
 set +e
 if [ "$result" != 0 ]; then
  restore_failed=0
  if [ "$committing" = 1 ]; then
   # Keep the guarded service in place until all other old files are durable.
   for target in $rollback_targets; do
    if [ "$target" = etc/init.d/S99unifi-keenetic ]; then
     [ "$restore_failed" = 0 ] || continue
     sync || { restore_failed=1; continue; }
    fi
    if [ -f "$work/backup/$target.absent" ]; then
     rm -f "/opt/$target" || restore_failed=1
    else
     mkdir -p "$work/restore/$(dirname "$target")" &&
      cp -p "$work/backup/$target" "$work/restore/$target" &&
      mv -f "$work/restore/$target" "/opt/$target" &&
      cmp -s "$work/backup/$target" "/opt/$target" || restore_failed=1
    fi
   done
  fi
  if [ "$restore_failed" = 0 ] && { [ "$committing" = 1 ] || [ "$stopped" = 1 ]; }; then
   record_phase restored || restore_failed=1
  fi
  if [ "$restore_failed" = 0 ] && [ "$stopped" = 1 ]; then
   UKN_INSTALL_RECOVERY="$work" "$service" start && "$service" status >/dev/null 2>&1 || restore_failed=1
  fi
  if [ "$restore_failed" != 0 ]; then
   keep_backup=1
   printf 'Installation recovery needs review; backups retained at %s\n' "$work" >&2
  fi
 fi
 if [ -n "$work" ] && [ "$keep_backup" = 0 ]; then rm -rf "$work"; fi
 rm -f "$lock/work-directory" "$lock/pid"
 rmdir "$lock" 2>/dev/null || true
 if ! sync; then
  printf '%s\n' 'Installation cleanup could not be synchronized; inspect installation markers before reboot' >&2
  [ "$result" != 0 ] || result=1
 fi
 exit "$result"
}
trap finish_install 0
trap 'exit 1' HUP INT TERM
printf '%s\n' "$$" > "$lock/pid"
for previous in /opt/.ukn-install.*; do
 [ "$previous" = "$lock" ] && continue
 [ ! -e "$previous" ] || { printf 'Unfinished installation data at %s; inspect it before retrying\n' "$previous" >&2; exit 1; }
done
work=$(mktemp -d /opt/.ukn-install.XXXXXX)
printf '%s\n' "$work" > "$lock/work-directory"
printf '%s\n' "$$" > "$work/installer-pid"
: > "$work/backup-hashes"
: > "$work/new-hashes"
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
  cmp -s "/opt/$target" "$work/backup/$target" || { printf 'Backup copy differs: %s\n' "$target" >&2; return 1; }
  record_hash "$work/backup/$target" "$target" "$work/backup-hashes"
 else
  : > "$work/backup/$target.absent"
 fi
 record_hash "$work/new/$target" "$target" "$work/new-hashes"
 targets="$targets $target"
 rollback_targets="$target $rollback_targets"
}
# Commit the guarded service first, before any binary/license replacement.
stage_file ./S99unifi-keenetic etc/init.d/S99unifi-keenetic 755
stage_file ./ukn sbin/ukn 755
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
service_before=absent
if [ -e "$service" ]; then
 [ -x "$service" ] || { printf '%s\n' 'Existing service is not executable; inspect it before upgrading' >&2; exit 1; }
 if "$service" status >/dev/null 2>&1; then
  was_running=1
  service_before=running
 else
  service_status=$?
  [ "$service_status" = 3 ] || { printf '%s\n' 'Cannot establish existing service status; nothing replaced' >&2; exit 1; }
  service_before=stopped
 fi
fi
printf '%s\n' "$service_before" > "$work/service-before"
record_phase prepared
if [ "$was_running" = 1 ]; then
 record_phase stopping
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
 record_phase stopped
fi
committing=1
record_phase committing
for target in $targets; do mv -f "$work/new/$target" "/opt/$target"; done
record_phase installed
printf '%s\n' 'Installed. Configure /opt/etc/unifi-keenetic.conf, then run /opt/etc/init.d/S99unifi-keenetic start.'
