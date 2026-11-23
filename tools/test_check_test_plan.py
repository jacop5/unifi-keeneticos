# SPDX-License-Identifier: GPL-2.0-or-later
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('checker', Path(__file__).with_name('check-test-plan.py'))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class ScopeTests(unittest.TestCase):
    def test_scope_and_redacted_target(self):
        ap = 'WifiMaster0/AccessPoint6'
        report = dict(schema_version=1, planner_supported=True, unsupported=[], preflight_passed=True,
                      commands=['<WLAN credential command>', 'interface Bridge0 include ' + ap],
                      command_targets=[ap, 'Bridge0'], access_lists=[], schedules=[])
        checker.check(report, {ap}, 'Bridge0')
        for target in ['WifiMaster0', 'WifiMaster0/AccessPoint0', 'Bridge1', '']:
            bad = copy.deepcopy(report)
            bad['command_targets'][0] = target
            with self.assertRaises(ValueError):
                checker.check(bad, {ap}, 'Bridge0')
        for field, value in [('preflight_passed', False), ('unsupported', ['security']), ('command_targets', [])]:
            bad = copy.deepcopy(report)
            bad[field] = value
            with self.assertRaises(ValueError):
                checker.check(bad, {ap}, 'Bridge0')

    def test_strict_review_requires_explicit_no_ignored_settings(self):
        ap = 'WifiMaster0/AccessPoint6'
        report = dict(schema_version=1, planner_supported=True, unsupported=[],
                      preflight_passed=True, commands=[], command_targets=[],
                      ignored_settings=[])
        checker.check(report, {ap}, 'Bridge0', require_no_ignored=True)
        for ignored in [['wireless.3.dtim_period'], None, '', False, {}]:
            report['ignored_settings'] = ignored
            checker.check(report, {ap}, 'Bridge0')  # Scope-only review remains distinct.
            with self.assertRaises(ValueError):
                checker.check(report, {ap}, 'Bridge0', require_no_ignored=True)
        del report['ignored_settings']
        with self.assertRaises(ValueError):
            checker.check(report, {ap}, 'Bridge0', require_no_ignored=True)


if __name__ == '__main__':
    unittest.main()
