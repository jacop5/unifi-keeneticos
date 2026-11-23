#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Check before stopping the installed service or writing replacement files.
check_install_space() {
 binary_bytes=$(wc -c < "$1" | tr -d '[:space:]')
 available_kib=$(df -Pk "$2" | awk 'NR > 1 { available=$4 } END { print available }')
 case "$binary_bytes:$available_kib" in *[!0-9:]*|:*|*:) printf '%s\n' 'Cannot determine installation space' >&2; return 1 ;; esac
 required_kib=$(( (binary_bytes + 1023) / 1024 + 16384 ))
 [ "$available_kib" -ge "$required_kib" ] || {
  printf 'Insufficient installation space: %s KiB free; %s KiB required\n' "$available_kib" "$required_kib" >&2
  return 1
 }
}
