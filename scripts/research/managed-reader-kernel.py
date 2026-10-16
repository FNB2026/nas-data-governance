#!/usr/bin/env python3
"""GitHub-hosted, private mount-namespace reader proof. Never run on a NAS.

No product writer, approval token, SQLite durability or source unlink is tested.
All files are new artificial fixtures; no network, Docker or host service change.
"""
import ctypes
import hashlib
import json
import os
from pathlib import Path
import pwd
import grp
import select
import stat
import subprocess
import sys
import uuid


def run(*args):
    return subprocess.run(args, capture_output=True, check=True, timeout=15)


def private(path, raw, uid):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        with os.fdopen(fd, 'wb', closefd=False) as stream:
            stream.write(raw)
            stream.flush()
        os.fchown(fd, uid, uid)
    finally:
        os.close(fd)


def actor(uid, *args):
    return ['setpriv', '--reuid=' + str(uid), '--regid=' + str(uid), '--clear-groups',
            '--no-new-privs', '--bounding-set=-all', '--inh-caps=-all', '--ambient-caps=-all', *args]


def line(process):
    assert select.select([process.stdout], [], [], 10)[0], 'probe handshake timeout'
    raw = process.stdout.readline(8193)
    assert len(raw) <= 8192 and raw.endswith(b'\n'), 'probe output bound'
    return json.loads(raw)


def identity(pid, uid):
    for task in Path('/proc/' + str(pid) + '/task').iterdir():
        fields = dict(text.split(':', 1) for text in (task / 'status').read_text().splitlines() if ':' in text)
        for name in ('Uid', 'Gid'):
            assert fields[name].split() == [str(uid)] * 4
        assert fields['Groups'].strip() == '' and fields['NoNewPrivs'].strip() == '1'
        assert all(int(fields[key].strip(), 16) == 0 for key in ('CapInh', 'CapPrm', 'CapEff', 'CapBnd', 'CapAmb'))


def main():
    assert os.geteuid() == 0 and len(sys.argv) == 2
    assert os.environ.get('GITHUB_ACTIONS') == 'true' and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
    assert not Path('/usr/trim').exists(), 'never run on fnOS'
    binary = Path(sys.argv[1]).resolve(strict=True)
    assert binary.is_file() and not binary.is_symlink()
    assert ctypes.CDLL(None, use_errno=True).unshare(0x00020000) == 0
    run('mount', '--make-rprivate', '/')
    os.umask(0o077)
    used = {x.pw_uid for x in pwd.getpwall()} | {x.gr_gid for x in grp.getgrall()}
    for p in Path('/proc').iterdir():
        if p.name.isdecimal():
            try:
                for text in (p / 'status').read_text().splitlines():
                    if text.startswith(('Uid:', 'Gid:')):
                        used.update(int(v) for v in text.split()[1:])
            except (FileNotFoundError, ProcessLookupError):
                pass
    uid, outsider = [n for n in range(22000, 23000) if n not in used][:2]
    base = Path('/var/lib') / ('ndg-managed-reader-' + uuid.uuid4().hex)
    base.mkdir(mode=0o700); os.chown(base, uid, uid)
    results = []
    for case in ('normal', 'static-directory-bind', 'static-symlink-root', 'late-directory-bind', 'late-file-bind', 'late-root-bind', 'late-root-replacement'):
        area = base / case; area.mkdir(mode=0o700); os.chown(area, uid, uid)
        root = area / 'root'; root.mkdir(mode=0o700); os.chown(root, uid, uid)
        objects = root / 'objects'; objects.mkdir(mode=0o700); os.chown(objects, uid, uid)
        ids = {'root_id': uuid.uuid4().hex + uuid.uuid4().hex, 'store_id': uuid.uuid4().hex + uuid.uuid4().hex}
        private(root / 'root.json', (json.dumps({'version': 1, **ids}, separators=(',', ':')) + '\n').encode(), uid)
        object_id = uuid.uuid4().hex + uuid.uuid4().hex
        member = objects / object_id; payload = b'synthetic private object before kernel mount probe'
        private(member, payload, uid)
        root_stat = root.stat(); file_stat = member.stat()
        proof = {'root': {**ids, 'device': root_stat.st_dev, 'inode': root_stat.st_ino},
                 'object_id': object_id, 'device': file_stat.st_dev, 'inode': file_stat.st_ino,
                 'size': file_stat.st_size, 'mtime_sec': file_stat.st_mtime_ns // 10**9, 'mtime_nsec': file_stat.st_mtime_ns % 10**9,
                 'ctime_sec': file_stat.st_ctime_ns // 10**9, 'ctime_nsec': file_stat.st_ctime_ns % 10**9,
                 'sha256': hashlib.sha256(payload).hexdigest()}
        keeper = os.open(member, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW)
        # A genuine foreign UID may not obtain a writable FD, not merely a
        # mock errno. An unexpected write can affect only this artificial file.
        code = "import errno,os,sys\ntry: os.open(sys.argv[1],os.O_WRONLY|os.O_TRUNC|os.O_NOFOLLOW)\nexcept OSError as e: sys.exit(0 if e.errno==errno.EACCES else 2)\nsys.exit(3)"
        denied = run(*actor(outsider, 'python3', '-c', code, str(member)))
        assert denied.stdout == denied.stderr == b''
        expected = 'match' if case == 'normal' else 'constructor_refused' if case.startswith('static-') else 'refused'
        selected = root
        mount = None
        if case == 'static-directory-bind':
            old = objects.stat(); run('mount', '--bind', str(objects), str(objects)); mount = objects
            assert (objects.stat().st_dev, objects.stat().st_ino) == (old.st_dev, old.st_ino)
        if case == 'static-symlink-root':
            selected = area / 'root-link'; selected.symlink_to(root)
        config = area / 'input.json'
        private(config, json.dumps({'path': str(selected), 'proof': proof, 'expected': expected}).encode(), uid)
        env = {**os.environ, 'NDG_MANAGED_READER_PROBE': str(config)}
        process = subprocess.Popen(actor(uid, str(binary), '-test.run=^TestManagedReaderKernelProcess$', '-test.timeout=15s'),
                                   stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, bufsize=0)
        try:
            first = line(process)
            if expected == 'constructor_refused':
                assert first == {'state': 'PASS', 'category': expected}
            else:
                assert first == {'state': 'READY'}
                identity(process.pid, uid)
                if case in ('late-directory-bind', 'late-file-bind', 'late-root-bind'):
                    target = objects if case == 'late-directory-bind' else member if case == 'late-file-bind' else root
                    old = target.stat(); run('mount', '--bind', str(target), str(target)); mount = target
                    assert (target.stat().st_dev, target.stat().st_ino) == (old.st_dev, old.st_ino)
                if case == 'late-root-replacement':
                    root.rename(area / 'original-root'); root.mkdir(mode=0o700); os.chown(root, uid, uid)
                process.stdin.write(b'inspect\n'); process.stdin.flush()
                assert line(process) == {'state': 'PASS', 'category': expected}
            remaining, errors = process.communicate(timeout=10)
            assert process.returncode == 0 and remaining == b'PASS\n' and errors == b''
            os.lseek(keeper, 0, os.SEEK_SET)
            assert os.read(keeper, len(payload) + 1) == payload
            assert os.fstat(keeper).st_ino == file_stat.st_ino
            results.append({'case': case, 'result': 'PASS', 'different_uid_write_refused': True, 'object_bytes_unchanged': True})
        finally:
            if process.poll() is None:
                process.kill(); process.communicate(timeout=5)
            if mount is not None:
                run('umount', str(mount))
            os.close(keeper)
    print(json.dumps({'result': 'PASS', 'scope': 'DISPOSABLE_NATIVE_PRIVATE_READER_NOT_WRITER_OR_PRODUCT_ACCEPTANCE',
                      'checks': results, 'fixtures_retained_until_runner_disposal': True}, sort_keys=True))


if __name__ == '__main__':
    main()
