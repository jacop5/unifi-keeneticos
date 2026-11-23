#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-or-later
"""Check a generated JSON plan's test-interface scope; execute nothing."""
import argparse
import json
import re


def check(report, aps, bridge):
    if not aps or any(not re.fullmatch(r'WifiMaster[01]/AccessPoint[1-6]', ap) for ap in aps):
        raise ValueError('explicit non-primary test APs required')
    if not re.fullmatch(r'Bridge[0-9]+', bridge):
        raise ValueError('invalid bridge')
    if report.get('schema_version') != 1 or report.get('planner_supported') is not True or report.get('unsupported') != [] or report.get('preflight_passed') is not True:
        raise ValueError('supported, preflighted schema-1 plan required')
    commands = report.get('commands')
    targets = report.get('command_targets')
    if not isinstance(commands, list) or not isinstance(targets, list) or len(commands) != len(targets):
        raise ValueError('matching command/target metadata required')
    for command, target in zip(commands, targets):
        if not isinstance(command, str) or '\n' in command or '\r' in command:
            raise ValueError('invalid command representation')
        if target in aps:
            if command != '<WLAN credential command>' and not command.startswith('interface ' + target + ' '):
                raise ValueError('command disagrees with target')
        elif target == bridge:
            parts = command.split()
            if len(parts) != 4 or parts[:3] != ['interface', bridge, 'include'] or parts[3] not in aps:
                raise ValueError('only test AP bridge membership allowed')
        else:
            raise ValueError('command targets an interface outside test scope')
    for acl in report.get('access_lists', []):
        if acl.get('target') not in aps:
            raise ValueError('ACL outside test scope')
    for schedule in report.get('schedules', []):
        if schedule.get('target') not in aps:
            raise ValueError('schedule outside test scope')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('plan')
    parser.add_argument('--ap', action='append', required=True)
    parser.add_argument('--bridge', default='Bridge0')
    args = parser.parse_args()
    with open(args.plan) as stream:
        report = json.load(stream)
    check(report, set(args.ap), args.bridge)
    print('Test-interface scope passed; no commands executed. Revalidate live ownership before applying.')


if __name__ == '__main__':
    main()
