#!/usr/bin/env python3
"""Private GitHub-hosted Writer probe, not a product/approval/NAS/power-loss test.

Every mutation targets newly created synthetic fixtures in our private mount
namespace. No network, Docker, service, source cleanup or recovery-clear action.
"""
import ctypes
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import pwd
import grp
import signal
import sqlite3
import subprocess
import sys
import uuid

spec = importlib.util.spec_from_file_location('reader_probe_helpers', Path(__file__).with_name('managed-reader-kernel.py'))
helpers = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helpers)
run, actor, line, identity, private = helpers.run, helpers.actor, helpers.line, helpers.identity, helpers.private

TARGET = b'synthetic target object'
KEEPER = b'synthetic keeper object'
SOURCE = b'synthetic copy input'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def stamp(path):
    st = path.lstat()
    return {'device': st.st_dev, 'inode': st.st_ino, 'mode': st.st_mode, 'uid': st.st_uid,
            'gid': st.st_gid, 'links': st.st_nlink, 'size': st.st_size,
            'mtime_ns': st.st_mtime_ns, 'ctime_ns': st.st_ctime_ns, 'sha256': sha(path)}


def opaque(value):
    return isinstance(value, str) and len(value) == 64 and all(c in '0123456789abcdef' for c in value)


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
    uid, outsider = [n for n in range(23000, 24000) if n not in used][:2]
    base = Path('/var/lib') / ('ndg-managed-writer-' + uuid.uuid4().hex)
    base.mkdir(mode=0o700); os.chown(base, uid, uid)
    results = []

    def launch(cfg):
        config = Path(cfg['area']) / ('config-' + uuid.uuid4().hex + '.json')
        private(config, json.dumps(cfg).encode(), uid)
        env = {**os.environ, 'NDG_MANAGED_WRITER_PROBE': str(config)}
        return subprocess.Popen(actor(uid, str(binary), '-test.run=^TestManagedWriterKernelProcess$', '-test.timeout=30s'),
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env, bufsize=0)

    def finish(process):
        remaining, errors = process.communicate(timeout=15)
        assert process.returncode == 0 and remaining == b'PASS\n' and errors == b'', 'child test failed'

    def prepared():
        area = base / uuid.uuid4().hex
        area.mkdir(mode=0o700); os.chown(area, uid, uid)
        private(area / 'source', SOURCE, uid)
        cfg = {'area': str(area), 'mode': 'prepare', 'source_identity': stamp(area / 'source')}
        process = launch(cfg)
        try:
            result = line(process)
            assert result.pop('state') == 'PASS'
            assert opaque(result['target']['object_id']) and opaque(result['keeper']['object_id'])
            assert opaque(result['root']['root_id']) and opaque(result['root']['store_id'])
            cfg.update(result)
            finish(process)
        finally:
            if process.poll() is None:
                process.kill(); process.communicate(timeout=5)
        return area, cfg

    def reopens(cfg, expected):
        process = launch({**cfg, 'mode': 'reopen', 'expected': expected})
        try:
            assert line(process) == {'state': 'PASS', 'category': expected}
            finish(process)
        finally:
            if process.poll() is None:
                process.kill(); process.communicate(timeout=5)

    def protected(area, cfg, selected=None):
        root = selected or area / 'root'
        assert (area / 'source').read_bytes() == SOURCE
        assert stamp(area / 'source') == cfg['source_identity']
        keeper = root / 'objects' / cfg['keeper']['object_id']
        assert keeper.read_bytes() == KEEPER
        st = keeper.lstat()
        proof = cfg['keeper']
        assert (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns, st.st_nlink,
                st.st_uid, st.st_gid, st.st_mode) == (proof['device'], proof['inode'], proof['size'],
                proof['mtime_sec'] * 10**9 + proof['mtime_nsec'], proof['ctime_sec'] * 10**9 + proof['ctime_nsec'],
                1, uid, uid, 0o100600)

    def state(area, selected=None):
        root = selected or area / 'root'
        control = json.loads((root / 'control.json').read_bytes())
        db = sqlite3.connect('file:' + str(root / 'catalog.sqlite') + '?mode=ro', uri=True)
        try:
            epoch, pending = db.execute('SELECT epoch,pending FROM identity WHERE id=1').fetchone()
            ops = db.execute('SELECT status,count(*) FROM operations GROUP BY status').fetchall()
            assert db.execute('PRAGMA quick_check').fetchone() == ('ok',)
        finally:
            db.close()
        return {'root_epoch': control['epoch'], 'root_pending': control['pending'] is not None,
                'catalog_epoch': epoch, 'catalog_pending': bool(pending), 'operations': dict(ops)}

    for mode in ('seal', 'purge'):
        area, cfg = prepared()
        process = launch({**cfg, 'mode': mode, 'expected': 'success'})
        try:
            assert line(process) == {'state': 'READY'}
            identity(process.pid, uid)
            other = launch({**cfg, 'mode': 'lease', 'expected': 'busy'})
            try:
                assert line(other) == {'state': 'PASS', 'category': 'busy'}
                finish(other)
            finally:
                if other.poll() is None:
                    other.kill(); other.communicate(timeout=5)
            denied = run(*actor(outsider, 'python3', '-c',
                "import errno,os,sys\ntry: os.open(sys.argv[1],os.O_WRONLY|os.O_TRUNC|os.O_NOFOLLOW)\nexcept OSError as e: sys.exit(0 if e.errno==errno.EACCES else 2)\nsys.exit(3)",
                str(area / 'root' / 'objects' / cfg['keeper']['object_id'])))
            assert denied.stdout == denied.stderr == b''
            process.stdin.write(b'execute\n'); process.stdin.flush()
            assert line(process) == {'state': 'PASS', 'category': 'success'}
            finish(process)
        finally:
            if process.poll() is None:
                process.kill(); process.communicate(timeout=5)
        reopens(cfg, 'ready')
        protected(area, cfg)
        current = state(area)
        assert current == {'root_epoch': 3, 'root_pending': False, 'catalog_epoch': 3,
                           'catalog_pending': False, 'operations': {'COMPLETE': 3}}
        target = area / 'root' / 'objects' / cfg['target']['object_id']
        assert target.exists() == (mode == 'seal')
        assert len(list((area / 'root' / 'objects').iterdir())) == (3 if mode == 'seal' else 1)
        assert not list((area / 'root' / 'staging').iterdir())
        results.append({'case': mode + '-normal-reopen-and-process-lease', 'result': 'PASS', 'state': current})

    # Actual same-device AND same-inode bind mounts after all Writer/SQLite
    # descriptors are open. This tests admission refusal, not immunity to
    # privileged remounts during an excluded final mutation window.
    for case in ('late-objects-bind', 'late-staging-bind', 'late-file-bind', 'late-lock-bind',
                 'late-catalog-bind', 'late-wal-bind', 'late-shm-bind', 'late-root-bind', 'late-root-replacement',
                 'late-dirty-wal-root-replacement'):
        area, cfg = prepared()
        dirty = case == 'late-dirty-wal-root-replacement'
        process = launch({**cfg, 'mode': 'purge', 'expected': 'refused',
                          'stop_at': 'catalog.begin.after' if dirty else ''})
        mount = None
        guards = {}
        try:
            assert line(process) == {'state': 'READY'}
            identity(process.pid, uid)
            root = area / 'root'
            main_before = None
            wal_before = None
            if dirty:
                process.stdin.write(b'execute\n'); process.stdin.flush()
                assert line(process) == {'state': 'BOUNDARY', 'at': 'catalog.begin.after'}
                wal_before = (root / 'catalog.sqlite-wal').stat().st_size
                assert wal_before > 0, 'dirty WAL was not exercised'
                assert state(area) == {'root_epoch': 3, 'root_pending': True, 'catalog_epoch': 3,
                                       'catalog_pending': True, 'operations': {'COMPLETE': 2, 'PENDING': 1}}
                # The worker is paused after commit, with no other writer or
                # auto-checkpoint. This diagnostic view deliberately ignores
                # WAL to prove that the committed intent is not in main yet.
                main_only = sqlite3.connect('file:' + str(root / 'catalog.sqlite') + '?mode=ro&immutable=1', uri=True)
                try:
                    assert main_only.execute('SELECT epoch,pending FROM identity WHERE id=1').fetchone() == (2, '')
                    assert main_only.execute('SELECT status,count(*) FROM operations GROUP BY status').fetchall() == [('COMPLETE', 2)]
                finally:
                    main_only.close()
                main_before = sha(root / 'catalog.sqlite')
            if case in ('late-root-replacement', 'late-dirty-wal-root-replacement'):
                root.rename(area / 'original-root')
                root.mkdir(mode=0o700); os.chown(root, uid, uid)
                for name in ('catalog.sqlite', 'catalog.sqlite-wal', 'catalog.sqlite-shm', 'control.json', 'writer.lock'):
                    private(root / name, b'synthetic replacement keeper must stay unchanged', uid)
                    guards[root / name] = stamp(root / name)
            else:
                target = {'late-objects-bind': root / 'objects', 'late-staging-bind': root / 'staging',
                          'late-file-bind': root / 'objects' / cfg['target']['object_id'],
                          'late-lock-bind': root / 'writer.lock', 'late-catalog-bind': root / 'catalog.sqlite',
                          'late-wal-bind': root / 'catalog.sqlite-wal', 'late-shm-bind': root / 'catalog.sqlite-shm',
                          'late-root-bind': root}[case]
                before = target.stat()
                run('mount', '--bind', str(target), str(target)); mount = target
                assert (target.stat().st_dev, target.stat().st_ino) == (before.st_dev, before.st_ino)
            process.stdin.write(b'continue\n' if dirty else b'execute\n'); process.stdin.flush()
            assert line(process) == {'state': 'PASS', 'category': 'refused'}
            finish(process)
            assert all(stamp(path) == expected for path, expected in guards.items()), 'Close touched replacement keepers'
            selected = area / 'original-root' if guards else root
            protected(area, cfg, selected)
            assert (selected / 'objects' / cfg['target']['object_id']).read_bytes() == TARGET
            result = {'case': case, 'result': 'PASS', 'source_keeper_target_unchanged': True,
                      'replacement_close_guards_unchanged': bool(guards)}
            if guards:
                assert set(root.iterdir()) == set(guards), 'Close changed replacement namespace'
            if dirty:
                current = state(area, selected)
                assert current == {'root_epoch': 3, 'root_pending': True, 'catalog_epoch': 3,
                                   'catalog_pending': True, 'operations': {'COMPLETE': 2, 'PENDING': 1}}
                result.update({'state': current, 'pre_close_wal_epoch': 3, 'pre_close_main_epoch': 2,
                               'wal_bytes_before_close': wal_before,
                               'wal_bytes_after_close': (selected / 'catalog.sqlite-wal').stat().st_size,
                               'original_catalog_checkpoint_observed': sha(selected / 'catalog.sqlite') != main_before})
            results.append(result)
        finally:
            if process.poll() is None:
                process.kill(); process.communicate(timeout=5)
            if mount is not None:
                run('umount', str(mount))

    common = ['root.pending.create', 'root.pending.write', 'root.pending.sync', 'root.pending.close',
              'root.pending.rename', 'root.pending.dirsync', 'catalog.begin', 'catalog.begin.commit',
              'catalog.staged', 'catalog.staged.commit', 'catalog.complete', 'catalog.complete.commit',
              'root.ready.create', 'root.ready.write', 'root.ready.sync', 'root.ready.close',
              'root.ready.rename', 'root.ready.dirsync']
    phases = {'seal': ['object.create', 'object.copy', 'object.sync', 'object.close', 'stage.copy.dirsync',
                       'seal.rename', 'objects.seal.dirsync', 'stage.seal.dirsync'],
              'purge': ['purge.rename', 'objects.purge.dirsync', 'stage.purge.dirsync', 'catalog.irreversible',
                        'catalog.irreversible.commit', 'purge.unlink', 'stage.unlink.dirsync']}
    for mode in ('seal', 'purge'):
        for label in common + phases[mode]:
            for edge in ('before', 'after'):
                stop = label + '.' + edge
                area, cfg = prepared()
                process = launch({**cfg, 'mode': mode, 'expected': 'success', 'stop_at': stop})
                try:
                    assert line(process) == {'state': 'READY'}
                    identity(process.pid, uid)
                    process.stdin.write(b'execute\n'); process.stdin.flush()
                    assert line(process) == {'state': 'BOUNDARY', 'at': stop}
                    os.kill(process.pid, signal.SIGKILL)
                    output, errors = process.communicate(timeout=10)
                    assert process.returncode == -signal.SIGKILL and output == errors == b''
                finally:
                    if process.poll() is None:
                        process.kill(); process.communicate(timeout=5)
                complete = stop == 'root.pending.create.before' or stop == 'root.ready.rename.after' or label == 'root.ready.dirsync'
                reopens(cfg, 'ready' if complete else 'recovery')
                protected(area, cfg)
                current = state(area)
                if complete:
                    expected = 2 if stop == 'root.pending.create.before' else 3
                    assert current == {'root_epoch': expected, 'root_pending': False, 'catalog_epoch': expected,
                                       'catalog_pending': False, 'operations': {'COMPLETE': expected}}
                results.append({'case': mode + '-sigkill-' + stop, 'result': 'PASS',
                                'fresh_process': 'ready_complete' if complete else 'recovery_refused', 'state': current})
    print(json.dumps({'result': 'PASS', 'scope': 'DISPOSABLE_NATIVE_PRIVATE_WRITER_NOT_PRODUCT_OR_NAS_OR_POWER_LOSS_ACCEPTANCE',
                      'checks': results, 'fixtures_retained_until_runner_disposal': True}, sort_keys=True))


if __name__ == '__main__':
    main()
