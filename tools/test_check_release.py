# SPDX-License-Identifier: GPL-2.0-or-later
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('release', Path(__file__).with_name('check-release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class BinaryArchitectureTests(unittest.TestCase):
    def test_reject_wrong_byte_order_machine_and_format(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'ukn'
            for arch, endian, encoding in [('mipsle', 'little', 1), ('mips', 'big', 2)]:
                header = bytearray(20)
                header[:4] = b'\x7fELF'
                header[4:6] = bytes([1, encoding])
                header[18:20] = (8).to_bytes(2, endian)
                path.write_bytes(header)
                self.assertEqual(release.validate_binary(path, arch)['endian'], endian)
                other = 'mips' if arch == 'mipsle' else 'mipsle'
                with self.assertRaises(RuntimeError):
                    release.validate_binary(path, other)
                for invalid in [b'not ELF', header[:10], header[:18] + (62).to_bytes(2, endian)]:
                    path.write_bytes(invalid)
                    with self.assertRaises(RuntimeError):
                        release.validate_binary(path, arch)
