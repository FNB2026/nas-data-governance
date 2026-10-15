#!/usr/bin/env python3
"""Explicit, real Linux host acceptance. Creates only nonce-owned synthetic resources.

Run as root in a new dedicated Linux VM. No remote Docker, existing state, pull,
registry, daemon configuration, volume pruning, or production source is accepted.
Raw receipts are private. Source/state/backup are intentionally retained for audit.
"""
import argparse
import errno
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import platform
import re
import secrets
import shutil
import sqlite3
import subprocess
import tempfile
import tarfile
import time
import urllib.error
import urllib.request


def run(*args, check=True, timeout=180):
    return subprocess.run(args, check=check, capture_output=True, text=True, timeout=timeout)


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute-disposable-host-test', action='store_true')
    parser.add_argument('--image', required=True)
    parser.add_argument('--source-sha', required=True)
    parser.add_argument('--expected-config', required=True)
    parser.add_argument('--keep-for-browser', action='store_true')
    args = parser.parse_args()
    assert args.execute_disposable_host_test and platform.system() == 'Linux' and os.geteuid() == 0
    assert not os.environ.get('DOCKER_HOST') and os.environ.get('DOCKER_CONTEXT', 'default') == 'default'
    assert run('docker', 'context', 'inspect', 'default', '--format', '{{.Endpoints.docker.Host}}').stdout.strip() == 'unix:///var/run/docker.sock'
    assert re.fullmatch(r'sha256:[a-f0-9]{64}', args.image) and re.fullmatch(r'[a-f0-9]{40}', args.source_sha)
    assert re.fullmatch(r'sha256:[a-f0-9]{64}', args.expected_config)
    os.umask(0o077)
    root = Path(tempfile.mkdtemp(prefix='ndg-preview-host-'))
    image = json.loads(run('docker', 'image', 'inspect', args.image).stdout)[0]
    assert image['Id'] == args.image
    assert image['Architecture'] == {'aarch64': 'arm64', 'x86_64': 'amd64'}[platform.machine()]
    assert image['Config']['Labels']['org.opencontainers.image.revision'] == args.source_sha
    assert image['Config']['Labels']['org.opencontainers.image.version'] == '0.5.0-docker-preview.1'
    # Refuse a shared host. This test is deliberately for a fresh dedicated VM.
    assert run('docker', 'ps', '-aq').stdout.strip() == '', 'dedicated_empty_host_required'
    # Docker containerd stores can expose a manifest ID through image inspect.
    # Export and verify the configuration plus every uncompressed layer instead
    # of accepting mutable labels or assuming inspect ID == OCI config digest.
    exported = root / 'runtime-image.tar'
    run('docker', 'image', 'save', '-o', str(exported), args.image)
    with tarfile.open(exported) as archive:
        manifests = json.load(archive.extractfile('manifest.json'))
        assert len(manifests) == 1
        manifest = manifests[0]
        raw = archive.extractfile(manifest['Config']).read()
        assert 'sha256:' + hashlib.sha256(raw).hexdigest() == args.expected_config
        config_value = json.loads(raw)
        observed_layers = []
        for layer in manifest['Layers']:
            h = hashlib.sha256()
            with archive.extractfile(layer) as stream:
                for block in iter(lambda: stream.read(1024 * 1024), b''):
                    h.update(block)
            observed_layers.append('sha256:' + h.hexdigest())
        assert observed_layers == config_value['rootfs']['diff_ids']
    token = secrets.token_hex(8); name = 'ndg-preview-' + token; network = name + '-net'
    label = 'com.ndg.preview-test=' + token
    source, state, config = [root / value for value in ('source', 'state', 'config')]
    for path in (source, state, config):
        path.mkdir(mode=0o700)
    for index in range(4000):
        (source / f'synthetic_private_marker_{index:05d}').write_bytes(hashlib.sha256(str(index // 2).encode()).digest() * 1024)
    before = {p.name: digest(p) for p in source.iterdir()}
    # A symlink must neither escape nor become a discovered file.
    outside = root / 'outside_private_marker'; outside.write_bytes(b'outside source boundary')
    (source / 'escape_link').symlink_to(outside)
    password = secrets.token_hex(32)
    (config / 'credential').write_text(password)
    origin = 'http://127.0.0.1:18084'
    settings = {'listen': '0.0.0.0:8080', 'origin': origin, 'state': '/data', 'credential_file': '/config/credential', 'sources': {'source_a': '/sources/source_a'}}
    (config / 'server.json').write_text(json.dumps(settings))
    for path in config.iterdir():
        path.chmod(0o600)
    for path in (root, source, state, config, *config.iterdir(), *source.iterdir()):
        os.chown(path, 10001, 10001, follow_symlinks=False)
    report = {'schema': 1, 'scope': 'independent_linux_vm_synthetic_host_acceptance', 'source_sha': args.source_sha,
              'image_id': args.image, 'verified_oci_config_digest': args.expected_config, 'exported_layer_diff_ids': observed_layers, 'architecture': image['Architecture'], 'kernel': platform.release(),
              'docker_engine': run('docker', 'version', '--format', '{{.Server.Version}}').stdout.strip(), 'checks': {},
              'root': str(root), 'container': name, 'network': network}
    report['checks']['exported_runtime_config_and_all_layers_match_oci'] = 'PASS'
    checks = report['checks']; cid = ''; success = False; base = ''; csrf = ''; mounted = False
    nested = source / 'nested-mount'
    jar = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), urllib.request.ProxyHandler({}))

    def request(path, body=None, expected=200, headers=None, authenticated=True):
        hdr = {'Host': '127.0.0.1:18084'}
        if body is not None:
            hdr.update({'Origin': origin, 'Content-Type': 'application/json', 'X-CSRF-Token': csrf})
        if headers:
            hdr.update(headers)
        req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=hdr)
        opener = client if authenticated else urllib.request.build_opener(urllib.request.ProxyHandler({}))
        try:
            response = opener.open(req, timeout=10)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            data = response.read(4 * 1024 * 1024)
            assert response.code == expected, (path, response.code, expected)
            return data, dict(response.headers)

    def api(path, body=None, **kwargs):
        return json.loads(request(path, body, **kwargs)[0])

    def inspect():
        value = json.loads(run('docker', 'inspect', name).stdout)[0]
        assert value['Id'] == cid and value['Config']['Labels']['com.ndg.preview-test'] == token
        return value

    def start(data_state):
        nonlocal cid, base, csrf
        cid = run('docker', 'run', '-d', '--name', name, '--label', label, '--network', network,
                  '--user', '10001:10001', '--read-only', '--cap-drop=ALL', '--security-opt=no-new-privileges',
                  '--pids-limit=128', '--memory=1g', '--cpus=1', '--tmpfs', '/tmp:rw,nosuid,nodev,noexec,size=64m',
                  '--mount', f'type=bind,src={source},dst=/sources/source_a,readonly',
                  '--mount', f'type=bind,src={config},dst=/config,readonly',
                  '--mount', f'type=bind,src={data_state},dst=/data', args.image).stdout.strip()
        value = inspect(); base = 'http://' + value['NetworkSettings']['Networks'][network]['IPAddress'] + ':8080'
        for _ in range(60):
            try:
                request('/healthz'); break
            except (OSError, AssertionError):
                time.sleep(0.5)
        else:
            raise AssertionError('health_timeout')
        jar.clear(); csrf = api('/api/login', {'password': password})['csrf']
        return value

    def remove():
        if cid:
            inspect()
            run('docker', 'stop', '--time', '60', cid)
            run('docker', 'rm', cid)

    def stable_snapshot():
        return api('/api/status', {'alias': 'source_a'})

    def completed():
        # Stay below the actual session rate limit; renew a session each minute.
        nonlocal csrf
        deadline = time.monotonic() + 600; renewed = time.monotonic()
        while time.monotonic() < deadline:
            if time.monotonic() - renewed > 55:
                jar.clear(); csrf = api('/api/login', {'password': password})['csrf']; renewed = time.monotonic()
            result = stable_snapshot()
            if result['state'] == 'COMPLETED':
                assert result['active'] == result['quick'] == result['full'] == 4000, result
                assert result['missing'] == result['unavailable'] == result['failed'] == 0, result
                assert result['checkpoint'] == 'completed' and len(result['groups']) == 200, result
                return result
            assert result['state'] not in ('FAILED', 'CANCELLED', 'PAUSED_AUTHORIZATION', 'PAUSED_NETWORK'), result
            time.sleep(1)
        raise AssertionError('scan_timeout')

    try:
        run('docker', 'network', 'create', '--internal', '--label', label, network)
        value = start(state)
        net = json.loads(run('docker', 'network', 'inspect', network).stdout)[0]
        assert net['Internal'] and len(value['NetworkSettings']['Networks']) == 1 and not value['HostConfig']['PortBindings']
        assert value['Image'] == args.image and value['Config']['User'] == '10001:10001'
        host = value['HostConfig']; assert host['ReadonlyRootfs'] and host['CapDrop'] == ['ALL'] and host['Privileged'] is False
        assert 'no-new-privileges' in host['SecurityOpt'] and host['PidsLimit'] == 128
        mounts = {m['Destination']: m['RW'] for m in value['Mounts']}
        assert mounts['/sources/source_a'] is False and mounts['/config'] is False and mounts['/data'] is True
        pid = value['State']['Pid']; status = Path(f'/proc/{pid}/status').read_text()
        assert re.search(r'^Uid:\s+10001\s+10001\s+10001\s+10001$', status, re.M) and 'NoNewPrivs:\t1' in status
        # Enter only our owned container's mount/root namespace and drop UID before
        # attempting to create an absent canary. EROFS is required; no source bytes change.
        probe = "import os,errno; p=int(__import__('sys').argv[1]); f=os.open(f'/proc/{p}/ns/mnt',os.O_RDONLY); r=os.open(f'/proc/{p}/root',os.O_RDONLY); os.setns(f,0); os.fchdir(r); os.chroot('.'); os.chdir('/'); os.setgroups([]); os.setgid(10001); os.setuid(10001)\ntry: os.open('/sources/source_a/readonly_canary',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)\nexcept OSError as e: raise SystemExit(0 if e.errno==errno.EROFS else 2)\nraise SystemExit(3)"
        run('python3', '-c', probe, str(pid))
        assert 'default' not in run('nsenter', '-t', str(pid), '-n', 'ip', 'route', 'show').stdout
        checks['nonroot_readonly_internal_network'] = 'PASS'
        html, headers = request('/')
        assert b'id="root"' in html and headers.get('Content-Security-Policy')
        for asset in re.findall(rb'(?:src|href)="(/assets/[^\"]+)"', html):
            assert request(asset.decode())[0]
        identity = api('/api/version'); assert identity['commit'] == args.source_sha and identity['version'] == '0.5.0-docker-preview.1'
        request('/api/sources', expected=401, authenticated=False)
        request('/api/scan', {'alias': 'source_a'}, expected=401, authenticated=False)
        request('/api/scan', {'alias': 'source_a'}, expected=403, headers={'X-CSRF-Token': ''})
        request('/api/scan', {'alias': 'source_a'}, expected=403, headers={'Origin': 'http://untrusted.invalid'})
        request('/api/sources', expected=403, headers={'Host': 'untrusted.invalid'})
        request('/api/sources?path=/etc', expected=403)
        request('/api/scan', {'alias': '../../etc'}, expected=400)
        request('/api/scan', {'alias': 'source_a', 'path': '/etc'}, expected=400)
        capabilities = api('/api/capabilities')
        assert capabilities == {'scan': True, 'resume': True, 'quarantine': False, 'restore': False, 'purge': False, 'plan_execution': False}
        for path in ('quarantine', 'restore', 'purge', 'plan', 'governance'):
            request('/api/' + path + '/execute', {}, expected=403)
        cookie = next(iter(jar)); assert cookie.has_nonstandard_attr('HttpOnly') and cookie.get_nonstandard_attr('SameSite') == 'Strict'
        checks['http_assets_auth_csrf_boundaries_execution_denied'] = 'PASS'
        api('/api/scan', {'alias': 'source_a', 'resume': False}, expected=202)
        time.sleep(0.05)
        api('/api/cancel', {'alias': 'source_a'}, expected=202)
        for _ in range(30):
            snapshot = stable_snapshot()
            if snapshot['state'] in ('CANCELLED', 'CANCELED', 'FAILED', 'COMPLETED'):
                break
            time.sleep(0.2)
        assert snapshot['checkpoint'] and snapshot['checkpoint'] != 'completed' and snapshot['state'] in ('CANCELLED', 'CANCELED'), snapshot
        checks['real_cancel_checkpoint'] = 'PASS'
        remove(); start(state)
        assert stable_snapshot()['checkpoint'] != 'completed'
        api('/api/scan', {'alias': 'source_a', 'resume': True}, expected=202)
        result = completed(); checks['container_removal_checkpoint_resume'] = 'PASS'
        run('docker', 'restart', '--time', '60', cid); time.sleep(1)
        jar.clear(); csrf = api('/api/login', {'password': password})['csrf']
        persisted = stable_snapshot(); assert persisted['active'] == 4000 and persisted['checkpoint'] == 'completed'
        checks['restart_persistence'] = 'PASS'
        logs = run('docker', 'logs', cid).stdout + run('docker', 'logs', cid).stderr
        assert all(x not in logs for x in (password, str(root), 'synthetic_private_marker', 'outside_private_marker'))
        assert 'synthetic_private_marker' not in json.dumps(result) and '/sources/' not in json.dumps(result)
        checks['bounded_log_and_response_privacy'] = 'PASS'
        remove()
        backup = root / 'backup'; restored = root / 'restored'
        shutil.copytree(state, backup); shutil.copytree(backup, restored)
        for path in (backup, restored):
            for item in (path, *path.rglob('*')):
                os.chown(item, 10001, 10001)
        assert {p.relative_to(state).as_posix(): digest(p) for p in state.rglob('*') if p.is_file()} == {p.relative_to(restored).as_posix(): digest(p) for p in restored.rglob('*') if p.is_file()}
        start(restored); snapshot = stable_snapshot()
        assert snapshot['active'] == 4000 and snapshot['full'] == 4000 and snapshot['checkpoint'] == 'completed'
        api('/api/scan', {'alias': 'source_a', 'resume': False}, expected=202); completed()
        checks['closed_backup_restore_rescan'] = 'PASS'
        remove()
        boundary = root / 'boundary'; boundary.mkdir(mode=0o700)
        marker = boundary / 'nested_private_marker'; marker.write_bytes(b'never traverse this mount')
        nested.mkdir(mode=0o700)
        for path in (boundary, marker, nested):
            os.chown(path, 10001, 10001)
        run('mount', '--bind', str(boundary), str(nested)); mounted = True
        assert source.stat().st_dev == nested.stat().st_dev, 'same_device_bind_fixture_required'
        start(restored)
        api('/api/scan', {'alias': 'source_a', 'resume': False}, expected=202)
        completed()
        remove()
        with sqlite3.connect('file:' + str(restored / 'source_a/governance.db') + '?mode=ro', uri=True) as database:
            assert database.execute('SELECT COUNT(*) FROM file_instances').fetchone()[0] == 4000
            assert database.execute('SELECT COUNT(*) FROM file_instances WHERE path LIKE ?', ('%nested_private_marker%',)).fetchone()[0] == 0
        assert marker.read_bytes() == b'never traverse this mount'
        run('umount', str(nested)); mounted = False; nested.rmdir()
        checks['real_same_device_nested_bind_not_traversed'] = 'PASS'
        start(restored)
        api('/api/scan', {'alias': 'source_a', 'resume': False}, expected=202); completed()
        after = {p.name: digest(p) for p in source.iterdir() if not p.is_symlink()}
        assert before == after and outside.read_bytes() == b'outside source boundary'
        checks['source_4000_sha256_unchanged_symlink_skipped'] = 'PASS'
        report['source_manifest_sha256'] = hashlib.sha256(json.dumps(before, sort_keys=True).encode()).hexdigest()
        report['source_files'] = len(before); report['browser'] = 'NOT_RUN'; report['fnos'] = 'NOT_RUN'
        report['container_ip'] = base.split('//')[1].split(':')[0]
        success = True
    finally:
        report['outcome'] = 'PASS' if success else 'FAIL'
        (root / 'host-receipt-private.json').write_text(json.dumps(report, indent=2) + '\n')
        if not (success and args.keep_for_browser):
            if cid:
                run('docker', 'rm', '-f', cid, check=False)
            if mounted:
                run('umount', str(nested))
            net_result = run('docker', 'network', 'inspect', network, check=False)
            if net_result.returncode == 0:
                assert json.loads(net_result.stdout)[0]['Labels']['com.ndg.preview-test'] == token
                run('docker', 'network', 'rm', network)
        print(json.dumps({'outcome': report['outcome'], 'checks': checks, 'private_receipt': str(root / 'host-receipt-private.json')}))


if __name__ == '__main__':
    main()
