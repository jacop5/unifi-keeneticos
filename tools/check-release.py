#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-or-later
"""Run local release checks; never deploy or contact a router/controller."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
INPUTS = ('cmd', 'internal', 'tools', 'packaging', 'docs', 'LICENSES',
          'go.mod', 'go.sum', 'LICENSE', 'NOTICE', 'README.md')


def tree_digest():
    digest = hashlib.sha256()
    files = []
    for name in INPUTS:
        path = ROOT / name
        files.extend(path.rglob('*') if path.is_dir() else [path])
    for path in sorted(p for p in files if p.is_file()):
        digest.update(str(path.relative_to(ROOT)).encode() + b'\0')
        digest.update(hashlib.sha256(path.read_bytes()).digest())
    return digest.hexdigest()



def validate_binary(path, arch):
    with path.open('rb') as stream:
        header = stream.read(20)
    endian = 'little' if arch == 'mipsle' else 'big'
    encoding = 1 if endian == 'little' else 2
    if (len(header) < 20 or header[:4] != b'\x7fELF' or header[4] != 1 or
            header[5] != encoding or int.from_bytes(header[18:20], endian) != 8):
        raise RuntimeError('wrong ELF architecture/byte order: ' + str(path))
    return dict(bits=32, endian=endian, machine='MIPS')


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, text=True).strip()


def write_report(path, report):
    with tempfile.NamedTemporaryFile(mode='w', dir=path.parent,
                                     prefix='.release-report-', delete=False) as stream:
        temporary = Path(stream.name)
        try:
            json.dump(report, stream, indent=2)
            stream.write('\n')
        except BaseException:
            temporary.unlink(missing_ok=True)
            raise
    try:
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', type=Path, default=ROOT / 'dist/validation/offline-release.json')
    parser.add_argument('--go', default='go')
    parser.add_argument('--require-clean', action='store_true')
    args = parser.parse_args()
    go = shutil.which(args.go)
    if not go:
        parser.error('Go executable not found')
    dirty = bool(git('status', '--porcelain'))
    if args.require_clean and dirty:
        parser.error('a clean committed tree is required')
    path = args.report.resolve()
    path.parent.mkdir(parents=True, exist_ok=True)
    logs = path.parent / (path.stem + '.logs')
    logs.mkdir(exist_ok=True, mode=0o700)
    env = dict(os.environ, GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local',
               GOWORK='off', GOFLAGS='', PYTHONDONTWRITEBYTECODE='1')
    env['PATH'] = str(Path(go).parent) + os.pathsep + env.get('PATH', '')
    for key in ('GOOS', 'GOARCH', 'GOMIPS', 'CGO_ENABLED'):
        env.pop(key, None)
    report = dict(schema_version=1, scope='offline-only', status='running',
                  started_at=datetime.now(timezone.utc).isoformat(), revision=git('rev-parse', 'HEAD'),
                  dirty=dirty, source_sha256=tree_digest(), checks=[], packages={})
    write_report(path, report)

    def run(name, command, cwd=ROOT, extra=None):
        logfile = logs / (name + '.log')
        check = dict(name=name, command=command, log=str(logfile), status='running')
        report['checks'].append(check)
        write_report(path, report)
        print('Checking ' + name, flush=True)
        started = time.monotonic()
        with logfile.open('w') as output:
            logfile.chmod(0o600)
            try:
                result = subprocess.run(command, cwd=cwd, env=env | (extra or {}),
                                        stdout=output, stderr=subprocess.STDOUT, timeout=900)
                check['exit_code'] = result.returncode
                check['status'] = 'passed' if result.returncode == 0 else 'failed'
            except (OSError, subprocess.TimeoutExpired) as error:
                check['status'] = 'failed'
                check['error'] = type(error).__name__
        check['seconds'] = round(time.monotonic() - started, 3)
        write_report(path, report)
        if check['status'] != 'passed':
            raise RuntimeError(name + ' failed; see ' + str(logfile))

    try:
        run('go-race', [go, 'test', '-race', '-count=1', './...'])
        run('go-vet', [go, 'vet', './...'])
        run('python-tools', [sys.executable, '-m', 'unittest', 'discover', '-s', 'tools', '-p', 'test_*.py'])
        for arch, directory in [('mipsle', 'keenetic'), ('mips', 'keenetic-mips')]:
            run('package-' + arch, ['sh', 'tools/build-keenetic.sh', arch])
            bundle = ROOT / 'dist' / directory
            run('manifest-' + arch, ['sha256sum', '-c', 'SHA256SUMS'], cwd=bundle)
            # Empty module cache + vendor mode makes source completeness testable.
            rebuilt = logs / ('rebuilt-ukn-' + arch)
            with tempfile.TemporaryDirectory(prefix='module-cache-' + arch + '-', dir=logs) as module_cache:
                run('source-rebuild-' + arch,
                    [go, 'build', '-mod=vendor', '-buildvcs=false', '-trimpath',
                     '-ldflags=-s -w', '-o', str(rebuilt), './cmd/ukn'],
                    cwd=bundle / 'source', extra=dict(GOOS='linux', GOARCH=arch,
                     GOMIPS='softfloat', CGO_ENABLED='0', GOMODCACHE=module_cache))
            architecture = validate_binary(bundle / 'ukn', arch)
            validate_binary(rebuilt, arch)
            report['packages'][arch] = dict(architecture=architecture,
                binary_sha256=hashlib.sha256((bundle / 'ukn').read_bytes()).hexdigest(),
                rebuilt_sha256=hashlib.sha256(rebuilt.read_bytes()).hexdigest())
        report['source_sha256_after'] = tree_digest()
        if report['revision'] != git('rev-parse', 'HEAD') or report['source_sha256_after'] != report['source_sha256']:
            raise RuntimeError('source changed during validation; rerun on a stable tree')
        if args.require_clean and git('status', '--porcelain'):
            raise RuntimeError('worktree became dirty during validation')
        report['status'] = 'passed'
    except (RuntimeError, OSError, subprocess.SubprocessError) as error:
        report['status'] = 'failed'
        report['error'] = str(error)
    finally:
        report['finished_at'] = datetime.now(timezone.utc).isoformat()
        write_report(path, report)
    print(report['status'].upper() + ': ' + str(path), flush=True)
    return 0 if report['status'] == 'passed' else 1


if __name__ == '__main__':
    sys.exit(main())
