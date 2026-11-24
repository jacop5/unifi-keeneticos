# SPDX-License-Identifier: GPL-2.0-or-later
"""Check actual staging/backup coexistence without allocating large files."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class InstallSpaceTests(unittest.TestCase):
    def test_counts_replacements_backups_and_preserves_existing_config(self):
        with tempfile.TemporaryDirectory(prefix='ukn-space-') as temp:
            base = Path(temp)
            package, root, commands = (base / n for n in ('package', 'entware', 'bin'))
            for p in (package, root, commands):
                p.mkdir()
            def sized(directory, name, size):
                p = directory / name
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_bytes(b'x' * size)
            sized(package, 'ukn', 2048)
            sized(package, 'S99unifi-keenetic', 1024)
            sized(package, 'unifi-keenetic.conf', 1024)
            sized(package, 'LICENSE', 1024)
            sized(package, 'NOTICE', 1024)
            sized(package, 'licenses/Go-BSD-3-Clause.txt', 1024)
            sized(package, 'licenses/model-catalogue-MIT.txt', 1024)
            (commands / 'df').write_text('#!/bin/sh\nprintf "Filesystem blocks Used Available Use%% Mounted\\nfixture 999999 0 %s 0%% /fixture\\n" "$UKN_FREE"\n')
            (commands / 'df').chmod(0o700)
            env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH'])
            def check(free, want):
                result = subprocess.run(['sh', '-c', '. "$1"; check_install_space "$2" "$3"',
                    'space-test', str(ROOT / 'packaging/keenetic/space-check.sh'),
                    str(package / 'ukn'), str(root)], env=env | {'UKN_FREE': str(free)},
                    capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode == 0, want, result.stdout + result.stderr)
            # Eight KiB staged for a first install, including new config.
            check(16384 + 7, False)
            check(16384 + 8, True)
            sized(root, 'sbin/ukn', 4096)
            sized(root, 'etc/init.d/S99unifi-keenetic', 1024)
            sized(root, 'share/licenses/unifi-keeneticos/NOTICE', 1024)
            # Eight KiB staged plus six KiB in old-file backups.
            check(16384 + 13, False)
            check(16384 + 14, True)
            # Existing configuration is preserved, not staged or backed up.
            sized(root, 'etc/unifi-keenetic.conf', 20480)
            check(16384 + 12, False)
            check(16384 + 13, True)
