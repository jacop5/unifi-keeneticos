# SPDX-License-Identifier: GPL-2.0-or-later
"""Execute the installer in a private fake Entware root; no host /opt writes."""
import hashlib
import os
from pathlib import Path
import shutil
import signal
import time
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
FILES = ['ukn', 'install.sh', 'space-check.sh', 'S99unifi-keenetic',
         'unifi-keenetic.conf', 'LICENSE', 'NOTICE',
         'licenses/Go-BSD-3-Clause.txt', 'licenses/model-catalogue-MIT.txt']


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='ukn-installer-')
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.root = self.base / 'entware'
        self.package = self.base / 'package'
        self.bin = self.base / 'bin'
        for p in (self.root / 'etc/init.d', self.package / 'licenses', self.bin):
            p.mkdir(parents=True)
        self.events = self.base / 'events'
        self.running = self.base / 'running'
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        UKN_EVENTS=str(self.events), UKN_RUNNING=str(self.running),
                        UKN_PROBED=str(self.base / 'probed'), UKN_FAIL='', UKN_STOP='',
                        UKN_FAILED_ONCE=str(self.base / 'failed-once'), UKN_PAUSED=str(self.base / 'paused'))
        self.write_exec(self.bin / 'id', '#!/bin/sh\necho 0\n')
        self.write_exec(self.bin / 'ndmc', '#!/bin/sh\nexit 99\n')
        self.write_exec(self.bin / 'df', '''#!/bin/sh
printf 'Filesystem blocks Used Available Use%% Mounted\n'
if [ "$UKN_FAIL" = space ]; then echo 'tmpfs 100 100 0 100% /fake'; else echo 'tmpfs 999999 0 999999 0% /fake'; fi
''')
        cp, mv = shutil.which('cp'), shutil.which('mv')
        self.assertIsNotNone(cp)
        self.assertIsNotNone(mv)
        self.write_exec(self.bin / 'cp', '''#!/bin/sh
for last; do :; done
case "$UKN_FAIL:$last" in
 stage:*/new/etc/init.d/S99unifi-keenetic|backup:*/backup/sbin/ukn) exit 1;;
esac
exec "''' + cp + '''" "$@"
''')
        self.write_exec(self.bin / 'mv', '''#!/bin/sh
for last; do :; done
case "$UKN_FAIL:$1:$last" in
 commit:*:*/etc/init.d/S99unifi-keenetic|recover:*:*/etc/init.d/S99unifi-keenetic)
  case "$2" in */new/*) if [ ! -e "$UKN_FAILED_ONCE" ]; then : > "$UKN_FAILED_ONCE"; exit 1; fi;; esac;;
esac
if [ "$UKN_FAIL" = recover ]; then
 case "$2" in */restore/sbin/ukn) exit 1;; esac
fi
if [ "$UKN_FAIL" = signal ]; then
 case "$2" in */new/etc/init.d/S99unifi-keenetic)
  : > "$UKN_PAUSED"
  while :; do sleep 1; done;;
 esac
fi
exec "''' + mv + '''" "$@"
''')
        # The complete shipped script runs, with only the fixed /opt prefix
        # localized. Fixtures rehash that localized installer before execution.
        installer = (ROOT / 'packaging/keenetic/install.sh').read_text()
        (self.package / 'install.sh').write_text(installer.replace('/opt', str(self.root)))
        shutil.copyfile(ROOT / 'packaging/keenetic/space-check.sh', self.package / 'space-check.sh')
        self.write_exec(self.package / 'ukn', '''#!/bin/sh
[ "$1" = -probe ] || exit 90
: > "$UKN_PROBED"
[ "$UKN_FAIL" != probe ]
''')
        self.write_exec(self.package / 'S99unifi-keenetic', self.service('new'))
        (self.package / 'unifi-keenetic.conf').write_text('ENABLED=0\n')
        for name in FILES[5:]:
            (self.package / name).write_text('new fixture ' + name + '\n')
        self.manifest()

    @staticmethod
    def write_exec(path, text):
        path.write_text(text)
        path.chmod(0o755)

    @staticmethod
    def service(label):
        return '''#!/bin/sh
case "$1" in
 status) [ -e "$UKN_RUNNING" ] && exit 0; exit 3;;
 stop)
  echo "''' + label + '''-stop" >> "$UKN_EVENTS"
  [ "$UKN_STOP" != running ] || exit 1
  rm -f "$UKN_RUNNING"
  [ "$UKN_STOP" != stopped ] || exit 1
  ;;
 start) echo "''' + label + '''-start" >> "$UKN_EVENTS"; : > "$UKN_RUNNING";;
 *) exit 1;;
esac
'''

    def manifest(self):
        (self.package / 'SHA256SUMS').write_text(''.join(
            hashlib.sha256((self.package / name).read_bytes()).hexdigest() + '  ' + name + '\n'
            for name in FILES))

    def install_old(self, running=True):
        for name in ['sbin/ukn', 'etc/unifi-keenetic.conf',
                     'var/lib/unifi-keenetic/state.json']:
            p = self.root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('old private fixture ' + name + '\n')
            p.chmod(0o600)
        self.write_exec(self.root / 'etc/init.d/S99unifi-keenetic', self.service('old'))
        for name in ['LICENSE', 'NOTICE', 'Go-BSD-3-Clause.txt', 'model-catalogue-MIT.txt']:
            p = self.root / 'share/licenses/unifi-keeneticos' / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('old notice ' + name + '\n')
        if running:
            self.running.touch()

    def snapshot(self):
        return {str(p.relative_to(self.root)): (p.read_bytes(), p.stat().st_mode & 0o777)
                for p in self.root.rglob('*') if p.is_file() and '.ukn-install.' not in str(p)}

    def run_install(self, ok):
        result = subprocess.run(['sh', 'install.sh'], cwd=self.package, env=self.env,
                                capture_output=True, text=True, timeout=20)
        self.assertEqual(result.returncode == 0, ok, result.stdout + result.stderr)
        return result

    def assert_clean(self):
        self.assertEqual(list(self.root.glob('.ukn-install.*')), [])

    def test_first_install_stays_disabled(self):
        self.run_install(True)
        self.assertEqual((self.root / 'sbin/ukn').read_bytes(), (self.package / 'ukn').read_bytes())
        self.assertEqual((self.root / 'etc/unifi-keenetic.conf').stat().st_mode & 0o777, 0o600)
        self.assertFalse(self.events.exists())
        self.assert_clean()

    def test_preflight_and_staging_failures_preserve_running_service(self):
        for failure in ['probe', 'space', 'stage', 'backup']:
            with self.subTest(failure=failure):
                self.install_old()
                before = self.snapshot()
                self.env['UKN_FAIL'] = failure
                self.run_install(False)
                self.assertEqual(self.snapshot(), before)
                self.assertTrue(self.running.exists())
                self.assertFalse(self.events.exists())
                self.assert_clean()

    def test_commit_failure_restores_files_and_prior_running_state(self):
        self.install_old()
        before = self.snapshot()
        self.env['UKN_FAIL'] = 'commit'
        self.run_install(False)
        self.assertEqual(self.snapshot(), before)
        self.assertTrue(self.running.exists())
        self.assertEqual(self.events.read_text().splitlines(), ['old-stop', 'old-start'])
        self.assert_clean()

    def test_stopped_upgrade_remains_stopped_on_failure(self):
        self.install_old(False)
        before = self.snapshot()
        self.env['UKN_FAIL'] = 'commit'
        self.run_install(False)
        self.assertEqual(self.snapshot(), before)
        self.assertFalse(self.running.exists())
        self.assertFalse(self.events.exists())
        self.assert_clean()

    def test_success_preserves_configuration_and_adoption_without_autostart(self):
        self.install_old()
        config = (self.root / 'etc/unifi-keenetic.conf').read_bytes()
        state = (self.root / 'var/lib/unifi-keenetic/state.json').read_bytes()
        self.run_install(True)
        self.assertEqual((self.root / 'etc/unifi-keenetic.conf').read_bytes(), config)
        self.assertEqual((self.root / 'var/lib/unifi-keenetic/state.json').read_bytes(), state)
        self.assertFalse(self.running.exists())
        self.assertEqual(self.events.read_text().splitlines(), ['old-stop'])
        self.assert_clean()

    def test_stop_failure_never_replaces_files(self):
        for mode in ['running', 'stopped']:
            with self.subTest(mode=mode):
                self.install_old()
                self.events.unlink(missing_ok=True)
                self.env['UKN_STOP'] = mode
                before = self.snapshot()
                self.run_install(False)
                self.assertEqual(self.snapshot(), before)
                self.assertTrue(self.running.exists())
                self.assert_clean()

    def test_failed_recovery_retains_backup_and_blocks_next_install(self):
        self.install_old()
        original = (self.root / 'sbin/ukn').read_bytes()
        self.env['UKN_FAIL'] = 'recover'
        self.run_install(False)
        backups = [p for p in self.root.glob('.ukn-install.*') if p.is_dir()]
        self.assertEqual(len(backups), 1)
        self.assertEqual((backups[0] / 'backup/sbin/ukn').read_bytes(), original)
        self.assertEqual(backups[0].stat().st_mode & 0o777, 0o700)
        after = self.snapshot()
        self.env['UKN_FAIL'] = ''
        self.run_install(False)
        self.assertEqual(self.snapshot(), after)
        self.assertFalse(self.running.exists())

    def test_checksum_failure_precedes_probe_and_service_stop(self):
        self.install_old()
        before = self.snapshot()
        (self.package / 'ukn').write_text('corrupt')
        self.run_install(False)
        self.assertEqual(self.snapshot(), before)
        self.assertFalse((self.base / 'probed').exists())
        self.assertFalse(self.events.exists())
        self.assertTrue(self.running.exists())
        self.assert_clean()

    def test_failed_first_install_removes_new_files(self):
        self.env['UKN_FAIL'] = 'commit'
        self.run_install(False)
        self.assertEqual(self.snapshot(), {})
        self.assertFalse(self.events.exists())
        self.assert_clean()

    def test_symlink_replacement_is_refused_before_service_stop(self):
        self.install_old()
        binary = self.root / 'sbin/ukn'
        external = self.base / 'external-binary'
        external.write_text('external fixture')
        binary.unlink()
        binary.symlink_to(external)
        self.run_install(False)
        self.assertTrue(binary.is_symlink())
        self.assertEqual(external.read_text(), 'external fixture')
        self.assertFalse(self.events.exists())
        self.assertTrue(self.running.exists())
        self.assert_clean()

    def test_unrunnable_staged_executable_preserves_old_service(self):
        self.install_old()
        before = self.snapshot()
        (self.package / 'ukn').write_text('#!/missing-ukn-fixture-interpreter\n')
        self.manifest()
        self.run_install(False)
        self.assertEqual(self.snapshot(), before)
        self.assertTrue(self.running.exists())
        self.assertFalse(self.events.exists())
        self.assert_clean()

    def test_termination_during_commit_restores_previous_install(self):
        self.install_old()
        before = self.snapshot()
        self.env['UKN_FAIL'] = 'signal'
        process = subprocess.Popen(['sh', 'install.sh'], cwd=self.package, env=self.env,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                   text=True, start_new_session=True)
        try:
            deadline = time.monotonic() + 5
            while not (self.base / 'paused').exists() and process.poll() is None and time.monotonic() < deadline:
                time.sleep(0.01)
            self.assertTrue((self.base / 'paused').exists(), 'fixture never reached interrupted commit')
            os.killpg(process.pid, signal.SIGTERM)
            process.communicate(timeout=10)
            self.assertNotEqual(process.returncode, 0)
            self.assertEqual(self.snapshot(), before)
            self.assertTrue(self.running.exists())
            self.assertEqual(self.events.read_text().splitlines(), ['old-stop', 'old-start'])
            self.assert_clean()
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL)
                process.communicate(timeout=5)
