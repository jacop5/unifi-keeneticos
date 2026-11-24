#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-or-later
"""Test the shipped installer/service and real agent in a disposable Linux container.

Requires an explicitly selected, already-present image with /bin/sh, coreutils,
start-stop-daemon, awk and sha256sum. Never pulls images or contacts a router or
controller. Native telemetry is synthetic; Docker networking is disabled.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
FILES = ['ukn', 'install.sh', 'space-check.sh', 'S99unifi-keenetic',
         'unifi-keenetic.conf', 'LICENSE', 'NOTICE',
         'licenses/Go-BSD-3-Clause.txt', 'licenses/model-catalogue-MIT.txt']
EXPECTED = ['first-install-disabled', 'real-agent-service-start',
            'successful-upgrade-preserves-private-data', 'explicit-restart-after-upgrade',
            'failed-upgrade-restores-and-restarts-real-agent', 'clean-stop-no-native-writes',
            'detached-watchdog-recovers-after-executable-replacement',
            'startup-recovers-pending-journal-before-telemetry']
NATIVE = '''#!/bin/sh
set -eu
[ "$1" = -c ] || exit 99
case "$2" in
 show*)
  if [ -e /work/require-recovery-before-telemetry ] && [ -e /work/fake-ap-ssid ]; then
   : > /work/forbidden-native-command
   exit 99
  fi;;
esac
case "$2" in
 'show version') printf 'hw_id: KN-1810\\n';;
 'show system') printf 'hostname: UKN-Container-Lab\\nuptime: 123\\n';;
 'show interface Bridge0') printf 'id: Bridge0\\naddress: 192.0.2.1\\nmac: 02:00:00:00:00:01\\nmask: 255.255.255.0\\n';;
 'show interface WifiMaster0') printf 'id: WifiMaster0\\nchannel: 6\\n';;
 'show interface WifiMaster1') printf 'id: WifiMaster1\\nchannel: 36\\n';;
 'show interface WifiMaster0/AccessPoint5')
  printf 'id: WifiMaster0/AccessPoint5\\nstate: down\\n'
  if [ -e /work/fake-ap-ssid ]; then printf 'ssid: ukn-rollback-probe\\n'; fi;;
 'show running-config')
  printf 'interface WifiMaster0/AccessPoint5\\n'
  if [ -e /work/fake-ap-ssid ]; then printf '    ssid ukn-rollback-probe\\n'; fi
  printf '    down\\n!\\n';;
 'interface WifiMaster0/AccessPoint5 ssid ukn-rollback-probe')
  [ -e /work/allow-rollback-probe ] || exit 99
  : > /work/fake-ap-ssid;;
 'interface WifiMaster0/AccessPoint5 no ssid')
  [ -e /work/allow-rollback-probe ] || exit 99
  printf 'inverse\\n' >> /work/inverse-calls
  rm -f /work/fake-ap-ssid;;
 'show associations'|'show ip arp'|'show interface'|'show ip dhcp bindings'|'show ip hotspot') :;;
 *) : > /work/forbidden-native-command; exit 99;;
esac
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--image', required=True, help='existing local Linux image; never pulled')
    parser.add_argument('--report', type=Path, default=ROOT / 'dist/validation/container-lifecycle.json')
    args = parser.parse_args()
    os.umask(0o077)
    docker, go = shutil.which('docker'), shutil.which('go')
    if not docker or not go:
        parser.error('docker and Go are required')
    image = json.loads(subprocess.check_output([docker, 'image', 'inspect', args.image], text=True))[0]
    arch = image.get('Architecture')
    if image.get('Os') != 'linux' or arch not in ('arm64', 'amd64'):
        parser.error('selected image must be Linux arm64 or amd64')
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    report = dict(schema_version=1, status='running', revision=revision,
                  modified=bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=ROOT, text=True).strip()),
                  scope='isolated-container-real-agent-synthetic-native-no-controller',
                  image_id=image['Id'], architecture=arch, checks=[],
                  limitations=['same-source artifacts with distinct build IDs, not cross-version compatibility',
                               'no MIPS runtime, physical reboot, power loss or native WLAN enforcement'])
    container = 'ukn-lifecycle-' + secrets.token_hex(8)
    started = time.monotonic()
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, indent=2) + '\n')
    with tempfile.TemporaryDirectory(prefix='ukn-container-lifecycle-') as temp:
        base = Path(temp)
        env = dict(os.environ, GOOS='linux', GOARCH=arch, CGO_ENABLED='0', GOPROXY='off', GOSUMDB='off')
        for label in ('a', 'b'):
            package = base / ('package-' + label)
            (package / 'licenses').mkdir(parents=True)
            print('Building container artifact ' + label, flush=True)
            try:
                subprocess.run([go, 'build', '-trimpath', '-ldflags=-s -w -buildid=ukn-lifecycle-' + label,
                                '-o', str(package / 'ukn'), './cmd/ukn'], cwd=ROOT, env=env, check=True, timeout=300)
            except (OSError, subprocess.SubprocessError):
                report['status'] = 'failed'
                args.report.write_text(json.dumps(report, indent=2) + '\n')
                raise
            for name in FILES[1:5]:
                shutil.copyfile(ROOT / 'packaging/keenetic' / name, package / name)
            for name in ('LICENSE', 'NOTICE'):
                shutil.copyfile(ROOT / name, package / name)
            shutil.copyfile(ROOT / 'LICENSES/Go-BSD-3-Clause.txt', package / FILES[7])
            shutil.copyfile(ROOT / 'internal/models/NOTICE', package / FILES[8])
            (package / 'SHA256SUMS').write_text(''.join(
                hashlib.sha256((package / name).read_bytes()).hexdigest() + '  ' + name + '\n' for name in FILES))
        if (base / 'package-a/ukn').read_bytes() == (base / 'package-b/ukn').read_bytes():
            raise RuntimeError('test requires distinct replacement artifacts')
        (base / 'ndmc').write_text(NATIVE)
        (base / 'ndmc').chmod(0o755)
        shutil.copyfile(ROOT / 'tools/fixtures/container-lifecycle.sh', base / 'run.sh')
        config = (ROOT / 'packaging/keenetic/unifi-keenetic.conf').read_text()
        config = config.replace('ENABLED=0', 'ENABLED=1').replace("INFORM_URL=''", "INFORM_URL='http://127.0.0.1:9/inform'")
        config = config.replace("SSH_LISTEN=':2222'", "SSH_LISTEN=''")
        config = config.replace("SLOTS=''", "SLOTS='/opt/etc/slots.json'")
        config = config.replace("NETWORKS=''", "NETWORKS='/opt/etc/networks.json'")
        (base / 'enabled.conf').write_text(config)
        (base / 'state.json').write_text(json.dumps(dict(key=secrets.token_hex(16), adopted=True,
            gcm=True, cfgversion='synthetic-adopted-config', inform_url='http://127.0.0.1:9/inform',
            version='6.8.2.15592', mgmt_cfg='', led=False, locating=False)))
        print('Testing installer/service/agent offline, including the real 65-second watchdog deadline', flush=True)
        try:
            result = subprocess.run([docker, 'run', '--rm', '--pull=never', '--name', container,
                '--network', 'none', '--no-healthcheck', '--cap-drop=ALL', '--cap-add=DAC_OVERRIDE', '--security-opt', 'no-new-privileges',
                '--pids-limit', '64', '--memory', '256m', '--cpus', '2',
                '--mount', 'type=bind,source=' + str(base) + ',target=/test,readonly',
                '--entrypoint', '/bin/sh', image['Id'], '/test/run.sh'],
                capture_output=True, text=True, timeout=180)
            report['checks'] = [line[5:] for line in result.stdout.splitlines() if line.startswith('PASS ')]
            if result.returncode != 0 or report['checks'] != EXPECTED:
                report['status'] = 'failed'
                print(result.stdout, end='')
                print(result.stderr, end='')
                raise RuntimeError('container lifecycle checks failed')
            report['status'] = 'passed'
        except (OSError, subprocess.SubprocessError):
            report['status'] = 'failed'
            raise
        finally:
            # A timeout/client interruption does not imply the container stopped.
            # Always remove our uniquely named container and all its processes.
            subprocess.run([docker, 'rm', '-f', container], stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL, timeout=30)
            remaining = subprocess.run([docker, 'container', 'ls', '--all', '--filter',
                'name=^/' + container + '$', '--format', '{{.ID}}'], capture_output=True, text=True, timeout=15)
            cleanup_ok = remaining.returncode == 0 and not remaining.stdout.strip()
            if not cleanup_ok:
                report['status'] = 'cleanup-unverified'
            report['duration_seconds'] = round(time.monotonic() - started, 2)
            args.report.parent.mkdir(parents=True, exist_ok=True)
            args.report.write_text(json.dumps(report, indent=2) + '\n')
            if not cleanup_ok:
                raise RuntimeError('cannot verify removal of the test container')
    print('PASSED: ' + str(args.report), flush=True)


if __name__ == '__main__':
    main()
