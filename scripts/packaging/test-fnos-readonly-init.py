#!/usr/bin/env python3
"""Real GitHub-hosted preparation/refusal proof; never a NAS/runtime acceptance."""
import ctypes
import hashlib
import json
import os
from pathlib import Path
import secrets
import stat
import subprocess
import sys


def main():
    assert os.geteuid() == 0 and os.uname().machine == 'x86_64'
    assert os.environ.get('GITHUB_ACTIONS') == 'true' and os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted'
    assert not Path('/usr/trim').exists(), 'never run this proof on fnOS'
    assert ctypes.CDLL(None, use_errno=True).unshare(0x00020000) == 0
    subprocess.run(['mount', '--make-rprivate', '/'], check=True, capture_output=True)
    os.umask(0o077)
    fixture = Path('/var/lib') / ('ndg-init-proof-' + secrets.token_hex(12))
    fixture.mkdir(mode=0o700)
    source_a, source_b = fixture / 'source_a', fixture / 'source_b'
    for source in (source_a, source_b):
        source.mkdir(mode=0o700)
        (source / 'keeper').write_bytes(b'synthetic source must remain untouched')
    before = {str(path): (path.stat().st_ino, hashlib.sha256(path.read_bytes()).hexdigest())
              for path in (source_a / 'keeper', source_b / 'keeper')}
    helper = Path(__file__).with_name('init-fnos-readonly.py')
    request = fixture / 'request.json'
    initial = {'boundary_root': str(fixture), 'parent': str(fixture), 'origin': 'https://ndg.example.test:24483',
               'network_id': 'a' * 64, 'core_ipv4': '10.253.188.2',
               'source_a': str(source_a), 'source_b': str(source_b)}

    def call(value, raw=None, mode=0o600, path=None):
        request.write_text(json.dumps(value) if raw is None else raw)
        request.chmod(mode)
        return subprocess.run([sys.executable, '-B', str(helper), '--create-new-installation', str(path or request)],
                              capture_output=True, timeout=10)

    checks = []
    def refusal(label, value=None, **kwargs):
        previous = set(fixture.glob('ndg-readonly-*'))
        result = call(initial if value is None else value, **kwargs)
        assert result.returncode == 1 and result.stdout == b'' and result.stderr
        assert set(fixture.glob('ndg-readonly-*')) == previous
        assert all(private.encode() not in result.stderr for private in (str(source_a), str(source_b), str(request)))
        checks.append(label)

    refusal('duplicate_fields', raw=json.dumps(initial)[:-1] + ',"origin":"https://other.test:24483"}')
    refusal('http_origin', {**initial, 'origin': 'http://ndg.example.test:24483'})
    refusal('ip_origin', {**initial, 'origin': 'https://192.168.31.193:24483'})
    refusal('network_name_instead_of_id', {**initial, 'network_id': 'arbitrary-existing-network'})
    refusal('loopback_upstream', {**initial, 'core_ipv4': '127.0.0.1'})
    refusal('world_readable_request', mode=0o644)
    request_link = fixture / 'request-link'; request_link.symlink_to(request)
    refusal('symlink_request', path=request_link)
    source_link = fixture / 'source-link'; source_link.symlink_to(source_a)
    refusal('symlink_source', {**initial, 'source_a': str(source_link)})
    refusal('state_inside_source', {**initial, 'parent': str(source_a)})
    refusal('overlapping_sources', {**initial, 'source_b': str(source_a)})
    unsafe = fixture / 'unsafe-parent'; unsafe.mkdir(mode=0o700); unsafe.chmod(0o702)
    refusal('writable_parent', {**initial, 'parent': str(unsafe)})
    unusual = fixture / 'source\nnewline'; unusual.mkdir(mode=0o700)
    refusal('dotenv_control_character', {**initial, 'source_a': str(unusual)})
    # Actual same-device bind aliases; refusal must precede any source write.
    nested = source_a / 'nested'; nested.mkdir(mode=0o700)
    parent_alias = fixture / 'parent-alias'; parent_alias.mkdir(mode=0o700)
    source_alias = fixture / 'source-alias'; source_alias.mkdir(mode=0o700)
    subprocess.run(['mount', '--bind', str(nested), str(parent_alias)], check=True, capture_output=True)
    try:
        assert (nested.stat().st_dev, nested.stat().st_ino) == (parent_alias.stat().st_dev, parent_alias.stat().st_ino)
        refusal('real_same_device_parent_bind_inside_source', {**initial, 'parent': str(parent_alias)})
        assert list(nested.iterdir()) == []
    finally:
        subprocess.run(['umount', str(parent_alias)], check=True, capture_output=True)
    subprocess.run(['mount', '--bind', str(source_a), str(source_alias)], check=True, capture_output=True)
    try:
        assert (source_a.stat().st_dev, source_a.stat().st_ino) == (source_alias.stat().st_dev, source_alias.stat().st_ino)
        refusal('real_same_device_overlapping_source_bind', {**initial, 'source_b': str(source_alias)})
    finally:
        subprocess.run(['umount', str(source_alias)], check=True, capture_output=True)
    result = call(initial)
    assert result.returncode == 0 and result.stderr == b''
    public = json.loads(result.stdout); root = Path(public['root'])
    assert public['status'] == 'PREPARED_NOT_STARTED' and root.parent == fixture
    assert stat.S_IMODE(root.stat().st_mode) == 0o700 and root.stat().st_uid == 0
    for name in ('config', 'state'):
        info = (root / name).stat()
        assert (info.st_uid, info.st_gid, stat.S_IMODE(info.st_mode)) == (10001, 10001, 0o700)
    for name in ('credential', 'server.json'):
        info = (root / 'config' / name).stat()
        assert (info.st_uid, info.st_gid, stat.S_IMODE(info.st_mode)) == (10001, 10001, 0o600)
    credential = (root / 'config/credential').read_text().strip()
    assert len(credential) == 64 and set(credential) <= set('0123456789abcdef') and credential.encode() not in result.stdout
    receipt = json.loads((root / 'prepared.json').read_bytes())
    assert receipt['network_verified'] is False and receipt['image_verified'] is False and receipt['gateway_installed'] is False
    command = ['docker', 'compose', '--project-name', 'ndginitproof', '--env-file', str(root / 'installation.env'),
               '--file', str(root / 'compose.yaml'), 'config', '--format', 'json']
    parsed = subprocess.run(command, capture_output=True, timeout=30)
    assert parsed.returncode == 0
    compose = json.loads(parsed.stdout); core = compose['services']['core']
    assert set(compose['services']) == {'core'} and not core.get('ports') and not core.get('build') and not core.get('environment')
    assert core['user'] == '10001:10001' and core['read_only'] and core['pull_policy'] == 'never'
    assert core['image'].endswith('@sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1')
    mounts = {v['target']: v for v in core['volumes']}
    assert set(mounts) == {'/data', '/config', '/sources/source_a', '/sources/source_b'}
    # compose-go serializes CreateHostPath with omitempty: false is absent.
    # Require all four explicit false inputs, then prove the parser preserves true.
    assert (root / 'compose.yaml').read_text().count('create_host_path: false') == 4
    assert all(v['type'] == 'bind' and v['bind'].get('create_host_path', False) is False for v in mounts.values())
    control = fixture / 'create-path-control.yaml'
    control.write_text((root / 'compose.yaml').read_text().replace('create_host_path: false', 'create_host_path: true'))
    control_command = list(command); control_command[control_command.index('--file') + 1] = str(control)
    control_run = subprocess.run(control_command, capture_output=True, timeout=30)
    assert control_run.returncode == 0
    control_mounts = json.loads(control_run.stdout)['services']['core']['volumes']
    assert all(v['bind']['create_host_path'] is True for v in control_mounts)
    checks.append('real_compose_parser_true_control')
    assert all(mounts[key]['read_only'] for key in ('/config', '/sources/source_a', '/sources/source_b'))
    assert not mounts['/data'].get('read_only', False)
    assert compose['networks']['core']['external'] and compose['networks']['core']['name'] == 'a' * 64
    assert {str(path): (path.stat().st_ino, hashlib.sha256(path.read_bytes()).hexdigest()) for path in
            (source_a / 'keeper', source_b / 'keeper')} == before
    checks.extend(['new_private_state_and_credential_permissions', 'real_compose_parser_boundary', 'source_inode_and_sha_unchanged'])
    print(json.dumps({'result': 'PASS', 'scope': 'GITHUB_HOSTED_PREPARATION_ONLY_NOT_RUNTIME_OR_NAS',
                      'checks': checks, 'fixture_retained_until_runner_disposal': True}, sort_keys=True))


if __name__ == '__main__':
    main()
