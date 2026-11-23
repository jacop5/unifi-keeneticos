# SPDX-License-Identifier: GPL-2.0-or-later
import importlib.util
from pathlib import Path
import unittest
from unittest import mock
import tempfile

spec = importlib.util.spec_from_file_location('stage', Path(__file__).with_name('stage-keenetic.py'))
stage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)


class StagePolicyTests(unittest.TestCase):
    def test_space_measurement(self):
        self.assertEqual(stage.available_bytes('Filesystem 1024-blocks Used Available Capacity Mounted on\ntmpfs 127236 78024 49212 61% /tmp\n'), 49212 * 1024)
        for value in ['', 'header\ninvalid', 'header\nx 1 2 unknown 50% /tmp']:
            with self.assertRaises(ValueError):
                stage.available_bytes(value)

    def test_low_space_aborts_before_upload(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'ukn'
            binary.write_bytes(b'build')
            args = ['stage', str(binary), '--host', '192.0.2.1', '--directory', '/tmp/ukn-lab']
            with mock.patch('sys.argv', args), mock.patch.object(stage.subprocess, 'check_output', return_value='Filesystem blocks Used Available Use% Mounted\ntmpfs 100 100 0 100% /tmp\n'), mock.patch.object(stage.subprocess, 'run') as upload:
                with self.assertRaisesRegex(RuntimeError, 'nothing uploaded'):
                    stage.main()
                upload.assert_not_called()

    def test_unique_scoped_destination(self):
        first = stage.destination('/tmp/ukn-test.lab')
        self.assertTrue(first.startswith('/tmp/ukn-test.lab/ukn-stage-'))
        self.assertNotEqual(first, stage.destination('/tmp/ukn-test.lab'))
        for value in ['/opt/sbin', '/tmp/ukn-test/../other', '/tmp/ukn-test;reboot', '/tmp/other']:
            with self.assertRaises(ValueError):
                stage.destination(value)


if __name__ == '__main__':
    unittest.main()
