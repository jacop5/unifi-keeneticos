#!/bin/sh
# SPDX-License-Identifier: GPL-2.0-or-later
set -eu
cd "$(dirname "$0")"
[ "$(id -u)" = 0 ] || { printf '%s\n' 'Run as root under Entware' >&2; exit 1; }
[ -d /opt/etc/init.d ] || { printf '%s\n' 'Entware is required' >&2; exit 1; }
command -v ndmc >/dev/null || { printf '%s\n' 'Keenetic ndmc is required' >&2; exit 1; }
for file in LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt; do
 [ -r "$file" ] || { printf 'Missing required license file: %s\n' "$file" >&2; exit 1; }
done
# Execute only the read-only probe before installing a CPU-specific binary.
chmod 700 ./ukn
./ukn -probe >/dev/null
if [ -x /opt/etc/init.d/S99unifi-keenetic ]; then /opt/etc/init.d/S99unifi-keenetic stop; fi
mkdir -p /opt/sbin /opt/var/lib/unifi-keenetic
chmod 700 /opt/var/lib/unifi-keenetic
cp ./ukn /opt/sbin/ukn.new
chmod 755 /opt/sbin/ukn.new
mv /opt/sbin/ukn.new /opt/sbin/ukn
mkdir -p /opt/share/licenses/unifi-keeneticos
cp LICENSE NOTICE licenses/Go-BSD-3-Clause.txt licenses/model-catalogue-MIT.txt /opt/share/licenses/unifi-keeneticos/
cp ./S99unifi-keenetic /opt/etc/init.d/S99unifi-keenetic
chmod 755 /opt/etc/init.d/S99unifi-keenetic
if [ ! -e /opt/etc/unifi-keenetic.conf ]; then
 cp ./unifi-keenetic.conf /opt/etc/unifi-keenetic.conf
 chmod 600 /opt/etc/unifi-keenetic.conf
fi
printf '%s\n' 'Installed. Configure /opt/etc/unifi-keenetic.conf, then run /opt/etc/init.d/S99unifi-keenetic start.'
