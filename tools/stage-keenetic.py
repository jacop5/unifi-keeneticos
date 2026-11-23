#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-or-later
"""Stage, but never execute, a build in an existing private Keenetic lab directory."""
import argparse
import hashlib
from pathlib import Path, PurePosixPath
import re
import shlex
import subprocess
import uuid

HEADROOM = 16 * 1024 * 1024


def destination(directory):
    path = PurePosixPath(directory)
    if not directory.startswith('/tmp/ukn-') or '..' in path.parts or not re.fullmatch(r'/tmp/ukn-[A-Za-z0-9._/-]+', directory):
        raise ValueError('destination must be an existing /tmp/ukn-* lab directory without traversal')
    return str(path / ('ukn-stage-' + uuid.uuid4().hex))


def available_bytes(output):
    lines = output.strip().splitlines()
    if len(lines) < 2:
        raise ValueError('unrecognised df output')
    fields = lines[-1].split()
    if len(fields) < 6 or not fields[3].isdigit():
        raise ValueError('unrecognised free-space measurement')
    return int(fields[3]) * 1024


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=Path)
    parser.add_argument('--host', required=True)
    parser.add_argument('--directory', required=True)
    parser.add_argument('--user', default='root')
    parser.add_argument('--port', type=int, default=222)
    args = parser.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.:-]*', args.host) or not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_-]*', args.user) or not 1 <= args.port <= 65535:
        parser.error('invalid SSH endpoint')
    target = destination(args.directory)
    size = args.binary.stat().st_size
    if not args.binary.is_file() or size <= 0:
        parser.error('binary must be a nonempty regular file')
    endpoint = args.user + '@' + args.host
    def remote(command):
        return subprocess.check_output(['ssh', '-p', str(args.port), '-o', 'ConnectTimeout=8', endpoint, command], text=True)
    quoted = shlex.quote(args.directory)
    free = available_bytes(remote(f'test -d {quoted} && test ! -L {quoted} && df -Pk {quoted}'))
    if free < size + HEADROOM:
        raise RuntimeError(f'insufficient space: {free} bytes free; need {size + HEADROOM}; nothing uploaded')
    with args.binary.open('rb') as stream:
        digest = hashlib.file_digest(stream, 'sha256').hexdigest()
    scp_host = '[' + args.host + ']' if ':' in args.host else args.host
    print(f'Staging {size} bytes with {HEADROOM} bytes required headroom: {target}', flush=True)
    try:
        subprocess.run(['scp', '-O', '-P', str(args.port), '-o', 'ConnectTimeout=8', str(args.binary.resolve()), args.user + '@' + scp_host + ':' + shlex.quote(target)], check=True)
        actual = remote('sha256sum ' + shlex.quote(target)).split()[0]
        if actual != digest:
            raise RuntimeError('uploaded hash does not match')
    except Exception:
        print(f'Staging failed. Do not execute the candidate; inspect this unique path: {target}', flush=True)
        raise
    print(f'Hash verified: {digest}\nStaged only; not executed or installed: {target}')


if __name__ == '__main__':
    main()
