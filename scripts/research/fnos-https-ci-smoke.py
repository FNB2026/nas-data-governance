#!/usr/bin/env python3
"""Ephemeral GitHub-hosted Linux test only. Never run on a NAS."""
import hashlib
import http.client
import json
import os
from pathlib import Path
import pwd
import grp
import shutil
import signal
import socket
import ssl
import stat
import struct
import subprocess
import sys
import time
import traceback


def main():
    assert os.geteuid() == 0
    assert os.environ.get('GITHUB_ACTIONS') == 'true'
    assert os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
    assert len(sys.argv) == 2
    binary = Path(sys.argv[1]).resolve(strict=True)
    assert binary.is_file()
    app = Path('/var/apps/ndg-readonly-web')
    name = 'ndghttpsci'
    assert not app.exists() and not app.is_symlink()
    try:
        pwd.getpwnam(name)
    except KeyError:
        pass
    else:
        raise AssertionError('test account exists')
    try:
        grp.getgrnam(name)
    except KeyError:
        pass
    else:
        raise AssertionError('test group exists')
    used = {u.pw_uid for u in pwd.getpwall()} | {g.gr_gid for g in grp.getgrall()}
    uid = next(n for n in range(21000, 22000) if n not in used)
    group_created = user_created = app_created = parent_created = False
    process = None
    original_inode = None

    def command(*args):
        return subprocess.run(args, check=True, capture_output=True, timeout=15)

    def start():
        return subprocess.Popen([
            '/usr/bin/setpriv', '--reuid='+str(uid), '--regid='+str(uid),
            '--clear-groups', '--no-new-privs', '--bounding-set=-all',
            '--inh-caps=-all', '--ambient-caps=-all', str(binary),
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def refused():
        p = start()
        try:
            out, err = p.communicate(timeout=4)
            assert p.returncode == 1 and out == b'' and err == b'gateway_unavailable\n'
        finally:
            if p.poll() is None:
                p.kill()
                p.communicate(timeout=3)

    try:
        command('/usr/sbin/groupadd', '--gid', str(uid), name)
        group_created = True
        command('/usr/sbin/useradd', '--uid', str(uid), '--gid', name,
                '--no-create-home', '--shell', '/usr/sbin/nologin', name)
        user_created = True
        if not app.parent.exists():
            app.parent.mkdir(mode=0o755)
            app.parent.chmod(0o755)
            parent_created = True
        app.mkdir(mode=0o755)
        app.chmod(0o755)
        app_created = True
        original_inode = app.stat().st_ino
        meta = app/'cmd'
        meta.mkdir(mode=0o755)
        meta.chmod(0o755)
        cfg = meta/'https.json'
        cert = meta/'tls.crt'
        key = meta/'tls.key'
        cfg.write_text(json.dumps({
            'gateway': {'app_name': 'ndg-readonly-web',
                        'origin': 'https://ndg.invalid:24483',
                        'upstream': 'http://10.98.45.2:8080',
                        'package_user': name, 'package_group': name},
            'listen': '127.0.0.1:24483',
        })+'\n')
        cfg.chmod(0o644)
        command('/usr/bin/openssl', 'req', '-x509', '-newkey', 'ec',
                '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-days', '1',
                '-subj', '/CN=ndg.invalid', '-addext', 'subjectAltName=DNS:ndg.invalid',
                '-keyout', str(key), '-out', str(cert))
        os.chown(key, 0, uid)
        key.chmod(0o640)
        cert.chmod(0o644)
        def snapshot():
            return {p.name: (hashlib.sha256(p.read_bytes()).hexdigest(), p.stat().st_uid,
                            p.stat().st_gid, stat.S_IMODE(p.stat().st_mode))
                    for p in [cfg, cert, key]}
        before = snapshot()
        context = ssl.create_default_context(cafile=str(cert))
        # DNS name verification and chain validation remain enabled. The test
        # client dials only loopback and supplies its generated certificate name.
        def request(host='ndg.invalid:24483', path='/'):
            raw = socket.create_connection(('127.0.0.1', 24483), timeout=2)
            tls = context.wrap_socket(raw, server_hostname='ndg.invalid')
            conn = http.client.HTTPConnection('ndg.invalid', 24483, timeout=2)
            conn.sock = tls
            try:
                conn.request('GET', path, headers={'Host': host,
                    'X-Forwarded-Proto': 'http', 'X-NDG-Edge-Proto': 'https'})
                response = conn.getresponse()
                return response.status, response.read(300000)
            finally:
                conn.close()

        process = start()
        for attempt in range(40):
            assert process.poll() is None
            try:
                status, body = request()
                break
            except (ConnectionRefusedError, ConnectionResetError):
                time.sleep(.05)
        else:
            raise AssertionError('startup timeout')
        assert status == 200 and b'NDG' in body
        assert request('wrong.invalid:24483')[0] == 403
        for route in ['/api/purge/execute', '/api/restore/execute', '/api/quarantine/execute']:
            status, body = request(path=route)
            assert status == 403 and body == b'{"error":"request_rejected"}'
        with socket.create_connection(('127.0.0.1', 24483), timeout=2) as plain:
            plain.sendall(b'GET / HTTP/1.1\r\nHost: ndg.invalid:24483\r\nX-Forwarded-Proto: https\r\nConnection: close\r\n\r\n')
            assert plain.recv(4096).startswith(b'HTTP/1.0 400')
        assert process.poll() is None
        tasks = list(Path('/proc', str(process.pid), 'task').iterdir())
        assert tasks
        for task in tasks:
            fields = dict(line.split(':', 1) for line in (task/'status').read_text().splitlines() if ':' in line)
            for label in ['Uid', 'Gid']:
                assert [int(v) for v in fields[label].split()] == [uid]*4
            assert not fields['Groups'].split()
            for label in ['CapInh', 'CapPrm', 'CapEff', 'CapAmb']:
                assert int(fields[label].strip(), 16) == 0
            assert fields['NoNewPrivs'].strip() == '1'
        process.send_signal(signal.SIGTERM)
        out, err = process.communicate(timeout=8)
        assert process.returncode == 0 and out == err == b''
        process = None
        # Real descriptor checks refuse readable-to-others keys, extended
        # access ACLs, writable metadata, symlinks and non-regular config.
        key.chmod(0o644)
        refused()
        key.chmod(0o640)
        acl = struct.pack('<I', 2) + b''.join(struct.pack('<HHI', *entry) for entry in [
            (1, 6, 0xffffffff), (2, 4, uid+1), (4, 4, 0xffffffff),
            (16, 4, 0xffffffff), (32, 0, 0xffffffff),
        ])
        os.setxattr(key, 'system.posix_acl_access', acl)
        refused()
        os.removexattr(key, 'system.posix_acl_access')
        cfg.chmod(0o664)
        refused()
        cfg.chmod(0o644)
        cert.rename(meta/'tls.crt.original')
        cert.symlink_to('tls.crt.original')
        refused()
        cert.unlink()
        (meta/'tls.crt.original').rename(cert)
        cfg.rename(meta/'https.original')
        os.mkfifo(cfg, 0o644)
        refused()
        cfg.unlink()
        (meta/'https.original').rename(cfg)
        assert before == snapshot()
    finally:
        if process is not None and process.poll() is None:
            process.terminate()
            try:
                process.communicate(timeout=8)
            except subprocess.TimeoutExpired:
                process.kill()
                process.communicate(timeout=3)
        if app_created:
            s = app.lstat()
            assert stat.S_ISDIR(s.st_mode) and s.st_uid == 0 and s.st_ino == original_inode
            shutil.rmtree(app)
        if user_created:
            assert pwd.getpwnam(name).pw_uid == uid
            command('/usr/sbin/userdel', name)
        if group_created:
            try:
                group = grp.getgrnam(name)
            except KeyError:
                pass  # userdel may already remove the empty same-name group.
            else:
                assert group.gr_gid == uid
                command('/usr/sbin/groupdel', name)
        if parent_created:
            app.parent.rmdir()

    print(json.dumps({'scope': 'GITHUB_HOSTED_SYNTHETIC_NATIVE_HTTPS_ONLY',
        'result': 'PASS', 'non_root_real_tls': True, 'startup_refusals': 5,
        'nas_runtime_acceptance': 'NOT_RUN'}))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(json.dumps({'error': 'isolated_https_smoke_failed',
            'type': type(error).__name__,
            'line': traceback.extract_tb(error.__traceback__)[-1].lineno}), file=sys.stderr)
        raise SystemExit(1) from None
