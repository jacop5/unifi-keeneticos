#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Check before stopping the installed service or writing replacement files.
check_install_space() {
 binary_bytes=$(wc -c < "$1" | tr -d '[:space:]')
 case "$binary_bytes" in ''|*[!0-9]*) printf '%s\n' 'Cannot determine executable size' >&2; return 1 ;; esac
 package_dir=$(dirname "$1")
 staged_bytes=$binary_bytes
 backup_bytes=0
 # During preparation both the staged replacements and copies of existing
 # files coexist with the installed package. Leave headroom after BOTH copies.
 for file in S99unifi-keenetic LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt; do
  if [ -f "$package_dir/$file" ]; then
   file_bytes=$(wc -c < "$package_dir/$file" | tr -d '[:space:]')
   case "$file_bytes" in ''|*[!0-9]*) return 1 ;; esac
   staged_bytes=$((staged_bytes + file_bytes))
  fi
 done
 if [ ! -e "$2/etc/unifi-keenetic.conf" ] && [ ! -L "$2/etc/unifi-keenetic.conf" ] && [ -f "$package_dir/unifi-keenetic.conf" ]; then
  file_bytes=$(wc -c < "$package_dir/unifi-keenetic.conf" | tr -d '[:space:]')
  case "$file_bytes" in ''|*[!0-9]*) return 1 ;; esac
  staged_bytes=$((staged_bytes + file_bytes))
 fi
 for target in sbin/ukn etc/init.d/S99unifi-keenetic share/licenses/unifi-keeneticos/LICENSE share/licenses/unifi-keeneticos/NOTICE share/licenses/unifi-keeneticos/Go-BSD-3-Clause.txt share/licenses/unifi-keeneticos/model-catalogue-MIT.txt; do
  if [ -f "$2/$target" ] && [ ! -L "$2/$target" ]; then
   file_bytes=$(wc -c < "$2/$target" | tr -d '[:space:]')
   case "$file_bytes" in ''|*[!0-9]*) return 1 ;; esac
   backup_bytes=$((backup_bytes + file_bytes))
  fi
 done
 available_kib=$(df -Pk "$2" | awk 'NR > 1 { available=$4 } END { print available }')
 case "$binary_bytes:$available_kib" in *[!0-9:]*|:*|*:) printf '%s\n' 'Cannot determine installation space' >&2; return 1 ;; esac
 required_kib=$(( (staged_bytes + backup_bytes + 1023) / 1024 + 16384 ))
 [ "$available_kib" -ge "$required_kib" ] || {
  printf 'Insufficient installation space: %s KiB free; %s KiB required\n' "$available_kib" "$required_kib" >&2
  return 1
 }
}
