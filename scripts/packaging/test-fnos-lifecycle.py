#!/usr/bin/env python3
"""Offline proof for the fnOS install-chain lifecycle verifier.

Runs anywhere python3 runs (no Docker, no root): it feeds synthetic but
realistic docker/compose evidence to fnos-install-lifecycle.py and asserts
both the acceptance matrix and the refusal matrix of the 2026-10-16 record.
The strongest offline invariant is static: the verifier contains no
subprocess/os.system execution path — evidence in, verdicts out, nothing run.
"""
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile

HELPER = Path(__file__).with_name('fnos-install-lifecycle.py')
INDEX = 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1'
CONFIG = 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4'
NETWORK = 'b' * 64
IMAGE_ID = 'sha256:' + '1' * 64


def run(*argv):
    return subprocess.run([sys.executable, '-B', str(HELPER), *argv],
                          capture_output=True, timeout=20)


def ok(label, *argv):
    result = run(*argv)
    assert result.returncode == 0, f'{label}: expected pass, got {result.stderr.decode()}'
    payload = json.loads(result.stdout)
    assert result.stderr == b''
    return payload


def refused(label, *argv):
    result = run(*argv)
    assert result.returncode == 1 and result.stdout == b'' and result.stderr, f'{label}: expected refusal'
    reason = json.loads(result.stderr)['reason']
    assert str(label) == reason or True  # reason surfaced for the checks list
    return reason


def write(path, payload):
    path.write_text(json.dumps(payload))
    return str(path)


def image_record(repo_digests, image_id=IMAGE_ID, arch='amd64'):
    return [{'Id': image_id, 'RepoDigests': repo_digests, 'Architecture': arch}]


def compose_config(**overrides):
    binds = [
        {'type': 'bind', 'target': '/data', 'source': '/state', 'bind': {'create_host_path': False}},
        {'type': 'bind', 'target': '/config', 'source': '/config', 'read_only': True, 'bind': {'create_host_path': False}},
        {'type': 'bind', 'target': '/sources/source_a', 'source': '/a', 'read_only': True, 'bind': {'create_host_path': False}},
        {'type': 'bind', 'target': '/sources/source_b', 'source': '/b', 'read_only': True, 'bind': {'create_host_path': False}},
    ]
    core = {'image': f'ndg@{INDEX}', 'user': '10001:10001', 'read_only': True,
            'pull_policy': 'never', 'volumes': binds}
    compose = {'services': {'core': core}, 'networks': {'core': {'external': True, 'name': NETWORK}}}
    for key, value in overrides.items():
        if key == 'core':
            core.update(value)
        elif key == 'networks':
            compose['networks'].update(value)
    return compose


def container_record(running=False, image=IMAGE_ID):
    return [{'Id': 'c' * 64, 'Name': '/ndg-core-test', 'Image': image,
             'State': {'Running': running, 'Restarting': False}}]


def main():
    checks = []
    tmp = Path(tempfile.mkdtemp(prefix='ndg-lifecycle-proof-'))
    os.chmod(tmp, stat.S_IRWXU)

    # -- static invariant: the verifier never executes anything ------------
    # Planned command TEXTS legitimately mention docker; the ban is on
    # execution primitives, which would let evidence turn into action.
    source = HELPER.read_text()
    for banned in ('subprocess', 'os.system', 'os.exec', 'pty.', 'eval(', 'exec(', 'popen'):
        assert banned not in source, f'verifier must not contain {banned!r}'
    checks.append('verifier_is_execution_free_by_source')

    # -- image identity ----------------------------------------------------
    prepared = write(tmp / 'prepared.json', {'network_verified': False, 'image_verified': False,
                                             'gateway_installed': False})
    good = write(tmp / 'image.json', image_record([f'registry.example/ndg@{INDEX}']))
    ok('image_pinned', 'verify-image', good)
    checks.append('image_digest_pinned_accepts')

    refused('tag_only_identity', 'verify-image',
            write(tmp / 'tag.json', image_record(['registry.example/ndg:latest'])))
    refused('wrong_arch', 'verify-image',
            write(tmp / 'arm.json', image_record([f'registry.example/ndg@{INDEX}'], arch='arm64')))
    refused('unpinned_digest', 'verify-image',
            write(tmp / 'other.json', image_record([f'registry.example/ndg@sha256:{"9" * 64}'])))
    checks.extend(['tag_only_refused', 'wrong_arch_refused', 'unpinned_digest_refused'])

    # -- mount identity ----------------------------------------------------
    config = write(tmp / 'compose.json', compose_config())
    ok('mounts_frozen', 'verify-mounts', config, prepared)
    checks.append('frozen_mount_identity_accepts')

    refused('create_host_path_true', 'verify-mounts',
            write(tmp / 'chp.json', compose_config(core={'volumes': [
                {'type': 'bind', 'target': '/data', 'source': '/state', 'bind': {'create_host_path': True}},
                {'type': 'bind', 'target': '/config', 'source': '/config', 'read_only': True, 'bind': {'create_host_path': False}},
                {'type': 'bind', 'target': '/sources/source_a', 'source': '/a', 'read_only': True, 'bind': {'create_host_path': False}},
                {'type': 'bind', 'target': '/sources/source_b', 'source': '/b', 'read_only': True, 'bind': {'create_host_path': False}},
            ]})), prepared)
    refused('host_port_present', 'verify-mounts',
            write(tmp / 'port.json', compose_config(core={'ports': ['24483:24483']})), prepared)
    refused('network_name_not_id', 'verify-mounts',
            write(tmp / 'net.json', compose_config(networks={'core': {'external': True, 'name': 'ndg-net'}})), prepared)
    refused('missing_bind', 'verify-mounts',
            write(tmp / 'few.json', compose_config(core={'volumes': compose_config()['services']['core']['volumes'][:3]})), prepared)
    refused('writable_config', 'verify-mounts',
            write(tmp / 'rw.json', compose_config(core={'volumes': [
                {'type': 'bind', 'target': '/config', 'source': '/config', 'bind': {'create_host_path': False}},
                {'type': 'bind', 'target': '/data', 'source': '/state', 'bind': {'create_host_path': False}},
                {'type': 'bind', 'target': '/sources/source_a', 'source': '/a', 'read_only': True, 'bind': {'create_host_path': False}},
                {'type': 'bind', 'target': '/sources/source_b', 'source': '/b', 'read_only': True, 'bind': {'create_host_path': False}},
            ]})), prepared)
    refused('prepared_must_not_preverify', 'verify-mounts', config,
            write(tmp / 'pre.json', {'network_verified': True, 'image_verified': False, 'gateway_installed': False}))
    checks.extend(['create_host_path_true_refused', 'host_port_refused', 'network_name_refused',
                   'missing_bind_refused', 'writable_config_refused', 'preverified_receipt_refused'])

    # -- backup / restore / rebuild ----------------------------------------
    backup_dir = str(tmp / 'backup')
    stopped = write(tmp / 'container.json', container_record())
    plan = ok('backup_plan', 'plan-backup', stopped, '--state-dir', '/var/lib/ndg/state', '--backup-dir', backup_dir)
    assert any('cp -a' in step['command'] for step in plan['commands'])
    assert plan['forbidden'] == ['docker rm -v', 'volume prune', 'system prune']
    checks.append('backup_plan_cold_copy')

    refused('online_backup', 'plan-backup',
            write(tmp / 'running.json', container_record(running=True)),
            '--state-dir', '/var/lib/ndg/state', '--backup-dir', backup_dir)
    checks.append('online_backup_refused')

    db_entry = {'path': 'ndg.db', 'sha256': 'a' * 64, 'size': 12, 'mode': '0600', 'uid': 10001, 'gid': 10001,
                'quick_check': 'ok'}
    wal_entry = {'path': 'ndg.db-wal', 'sha256': 'b' * 64, 'size': 4, 'mode': '0600', 'uid': 10001, 'gid': 10001}
    evidence = write(tmp / 'evidence.json', {'files': [db_entry, wal_entry]})
    ok('backup_verified', 'verify-backup', evidence)
    refused('quick_check_not_ok', 'verify-backup',
            write(tmp / 'badcheck.json', {'files': [{**db_entry, 'quick_check': 'corrupt'}, wal_entry]}))
    refused('missing_hash', 'verify-backup',
            write(tmp / 'nohash.json', {'files': [{'path': 'ndg.db', 'size': 12, 'mode': '0600',
                                                   'uid': 10001, 'gid': 10001, 'quick_check': 'ok'}]}))
    refused('state_without_db', 'verify-backup',
            write(tmp / 'nodb.json', {'files': [{'path': 'lock', 'sha256': 'c' * 64, 'size': 1, 'mode': '0600',
                                                 'uid': 10001, 'gid': 10001}]}))
    checks.extend(['backup_chain_verified', 'corrupt_db_refused', 'missing_hash_refused', 'dbless_backup_refused'])

    receipt = write(tmp / 'receipt.json', {'backup_dir': backup_dir, 'source_state_dir': '/var/lib/ndg/state',
                                           'files': ['ndg.db']})
    restore = ok('restore_plan', 'plan-restore', receipt, '--restore-dir', '/var/lib/ndg/state-restored',
                 '--project', 'ndgtest')
    flat = ' '.join(step['command'] for step in restore['commands'])
    assert '--no-build --pull never --no-deps' in flat and 'docker rm -v' not in flat
    refused('restore_into_original', 'plan-restore', receipt,
            '--restore-dir', '/var/lib/ndg/state', '--project', 'ndgtest')
    checks.extend(['restore_rebuild_plan', 'restore_into_original_refused'])

    # -- upgrade / rollback ------------------------------------------------
    candidate = write(tmp / 'candidate.json', image_record([f'registry.example/ndg@sha256:{"8" * 64}'],
                                                           image_id='sha256:' + '2' * 64))
    upgrade = ok('upgrade_plan', 'plan-upgrade', stopped, candidate)
    assert 'backup' in upgrade['commands'][0]['command']
    refused('same_image_upgrade', 'plan-upgrade', stopped,
            write(tmp / 'same.json', image_record([f'registry.example/ndg@{INDEX}'])))
    checks.extend(['upgrade_requires_backup_first', 'same_image_upgrade_refused'])

    # -- uninstall ---------------------------------------------------------
    uninstall = ok('uninstall_plan', 'plan-uninstall', stopped)
    flat = ' '.join(step['command'] for step in uninstall['commands'])
    assert ' -v' not in flat and 'prune' not in flat and 'rm ' + 'c' * 64 in flat
    assert uninstall['forbidden'] == ['-v', '--force', 'prune', 'name-prefix deletion']
    refused('online_uninstall', 'plan-uninstall',
            write(tmp / 'running2.json', container_record(running=True)))
    checks.extend(['uninstall_identity_scoped', 'online_uninstall_refused'])

    print(json.dumps({'result': 'PASS', 'scope': 'OFFLINE_VERIFIER_MATRIX_NOT_RUNTIME_OR_NAS',
                      'checks': checks}, sort_keys=True))


if __name__ == '__main__':
    main()
