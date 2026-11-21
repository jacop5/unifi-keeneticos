#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
# Official UniFi OS Server in a dedicated OrbStack Linux machine.
set -eu
machine=unifi-os-test
if ! orb list | awk '$1 == "unifi-os-test" { found=1 } END { exit !found }'; then
  orb create --arch arm64 --memory 4G --cpus 2 --disk 32G ubuntu:24.04 "$machine"
fi
orb -m "$machine" -u root sh -c 'apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y podman slirp4netns passt curl ca-certificates'
orb -m "$machine" curl -fL https://fw-download.ubnt.com/data/unifi-os-server/f730-linux-arm64-5.1.42-d2bca8bd-d6fc-4d20-99e5-b78235843c2f.42-arm64 -o /tmp/unifi-os-server-5.1.42
orb -m "$machine" -u root chmod +x /tmp/unifi-os-server-5.1.42
orb -m "$machine" -u root /tmp/unifi-os-server-5.1.42 --non-interactive --network-mode pasta
