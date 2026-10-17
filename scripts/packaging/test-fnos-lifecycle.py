#!/usr/bin/env python3
"""Proof for the fnOS install-chain lifecycle: evidence matrix + real execution.

Two layers are asserted here.

1. The evidence-only verifier still accepts/refuses the 2026-10-16 matrix
   exactly as before (24 checks).
2. The executable layer is proved on the current host with real directories,
   real files, a real HTTP server and a real subprocess runner:
     * no shell anywhere (AST invariant: no string command reaches
       subprocess, no os.system/os.exec/eval/exec, no shell=True);
     * a plan digest and a per-step grant are both required before any
       mutation, and neither can be widened by editing the other;
     * directory identity is re-proved (umask cannot silently filter modes,
       symlinked components are refused, the mount snapshot is re-checked);
     * the journal is append-only, and an interrupted step refuses to
       auto-continue unless its postcheck actually passes.

The real container chain (init -> start -> access -> backup -> uninstall on
the published pinned image) needs Linux + Docker + root and lives in
test-fnos-install-chain.py, not here.
"""
import ast
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

HERE = Path(__file__).resolve().parent
HELPER = HERE / 'fnos-install-lifecycle.py'
ENGINE = HERE / 'fnos_install_engine.py'
INDEX = 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1'
CONFIG = 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4'
NETWORK = 'b' * 64
IMAGE_ID = 'sha256:' + '1' * 64
CORE_IP = '10.253.188.2'

sys.path.insert(0, str(HERE))
import fnos_install_engine as engine  # noqa: E402


def run(*argv):
    return subprocess.run([sys.executable, '-B', str(HELPER), *argv],
                          capture_output=True, timeout=60)


def ok(label, *argv):
    result = run(*argv)
    assert result.returncode == 0, f'{label}: expected pass, got {result.stderr.decode()}'
    payload = json.loads(result.stdout)
    assert result.stderr == b''
    return payload


def refused(label, expected, *argv):
    result = run(*argv)
    assert result.returncode == 1 and result.stdout == b'' and result.stderr, \
        f'{label}: expected refusal, got rc={result.returncode}'
    reason = json.loads(result.stderr)['reason']
    assert reason == expected, f'{label}: expected {expected!r}, got {reason!r}'
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


def static_invariants(checks):
    """AST-level: no shell, no string command, no dynamic code execution."""
    banned_os = {'system', 'popen', 'execv', 'execve', 'execvp', 'execvpe', 'execl', 'execle',
                 'execlp', 'posix_spawn', 'posix_spawnp', 'spawnv', 'spawnve', 'spawnl',
                 'spawnlp', 'spawnvp'}
    calls = 0
    for name, path in (('cli', HELPER), ('engine', ENGINE)):
        tree = ast.parse(path.read_text())
        for node in ast.walk(tree):
            if not isinstance(node, ast.Call):
                continue
            function = node.func
            if isinstance(function, ast.Name) and function.id in {'eval', 'exec', 'compile', '__import__'}:
                assert function.id != 'compile', f'{name}: dynamic compile'
                if function.id in {'eval', 'exec'}:
                    raise AssertionError(f'{name}: {function.id} call')
            if not isinstance(function, ast.Attribute):
                continue
            owner = getattr(function.value, 'id', None)
            if owner == 'os' and function.attr in banned_os:
                raise AssertionError(f'{name}: os.{function.attr}')
            if owner == 'pty' and function.attr in {'spawn', 'fork', 'openpty'}:
                raise AssertionError(f'{name}: pty.{function.attr}')
            if owner == 'subprocess' and function.attr in {'run', 'Popen', 'call', 'check_call',
                                                           'check_output', 'getoutput',
                                                           'getstatusoutput'}:
                calls += 1
                shell = [kw for kw in node.keywords if kw.arg == 'shell']
                assert shell, f'{name}: subprocess call without explicit shell=False'
                assert isinstance(shell[0].value, ast.Constant) and shell[0].value.value is False, \
                    f'{name}: shell not literal False'
                assert node.args, f'{name}: subprocess call without argv'
                first = node.args[0]
                assert not isinstance(first, (ast.Constant, ast.JoinedStr)), \
                    f'{name}: string command reaches subprocess'
                assert not (isinstance(first, ast.BinOp) and isinstance(first.op, ast.Add)), \
                    f'{name}: concatenated command reaches subprocess'
    assert calls >= 1, 'no subprocess call site found: the runner vanished'
    checks.append('ast_no_shell_or_string_command')


def fixture(base):
    for name in ('parent', 'source_a', 'source_b', 'backups'):
        (base / name).mkdir(mode=0o700, exist_ok=True)
    (base / 'source_a' / 'alpha.txt').write_text('alpha')
    (base / 'source_b' / 'beta.txt').write_text('beta')
    return {
        'scope': 'DISPOSABLE_LINUX_ACCEPTANCE',
        'installation_id': '0123456789ab',
        'parent': str(base / 'parent'),
        'boundary_root': str(base),
        'source_a': str(base / 'source_a'),
        'source_b': str(base / 'source_b'),
        'origin': 'http://' + CORE_IP + ':8080',
        'core_ipv4': CORE_IP,
        'network_id': NETWORK,
        'backup_dir': str(base / 'backups' / 'run1'),
    }


def evidence_matrix(tmp, checks):
    # -- static invariant: the evidence layer executes nothing we did not audit --
    source = HELPER.read_text()
    for banned in ('os.system', 'os.popen', 'pty.', 'eval(', 'exec('):
        assert banned not in source, f'verifier must not contain {banned!r}'
    checks.append('verifier_source_has_no_unaudited_execution')

    prepared = write(tmp / 'prepared.json', {'network_verified': False, 'image_verified': False,
                                             'gateway_installed': False})
    good = write(tmp / 'image.json', image_record([f'registry.example/ndg@{INDEX}']))
    ok('image_pinned', 'verify-image', good)
    checks.append('image_digest_pinned_accepts')

    refused('tag_only_identity', 'repo_digest_required_without_tag_identity', 'verify-image',
            write(tmp / 'tag.json', image_record(['registry.example/ndg:latest'])))
    refused('wrong_arch', 'acceptance_evidence_is_amd64_only', 'verify-image',
            write(tmp / 'arm.json', image_record([f'registry.example/ndg@{INDEX}'], arch='arm64')))
    refused('unpinned_digest', 'image_not_pinned_to_acceptance_index', 'verify-image',
            write(tmp / 'other.json', image_record([f'registry.example/ndg@sha256:{"9" * 64}'])))
    checks.extend(['tag_only_refused', 'wrong_arch_refused', 'unpinned_digest_refused'])

    config = write(tmp / 'compose.json', compose_config())
    ok('mounts_frozen', 'verify-mounts', config, prepared)
    checks.append('frozen_mount_identity_accepts')

    binds = compose_config()['services']['core']['volumes']
    refused('create_host_path_true', 'create_host_path_false_required', 'verify-mounts',
            write(tmp / 'chp.json', compose_config(core={'volumes': [
                {**binds[0], 'bind': {'create_host_path': True}}, binds[1], binds[2], binds[3]]})), prepared)
    refused('host_port_present', 'no_host_ports', 'verify-mounts',
            write(tmp / 'port.json', compose_config(core={'ports': ['24483:24483']})), prepared)
    refused('network_name_not_id', 'full_network_id_required', 'verify-mounts',
            write(tmp / 'net.json', compose_config(networks={'core': {'external': True, 'name': 'ndg-net'}})), prepared)
    refused('missing_bind', 'exact_four_bind_targets_required', 'verify-mounts',
            write(tmp / 'few.json', compose_config(core={'volumes': binds[:3]})), prepared)
    refused('writable_config', 'read_only_flag_mismatch', 'verify-mounts',
            write(tmp / 'rw.json', compose_config(core={'volumes': [
                {**binds[1], 'read_only': False}, binds[0], binds[2], binds[3]]})), prepared)
    refused('prepared_must_not_preverify', 'prepared_receipt_must_not_preverify', 'verify-mounts', config,
            write(tmp / 'pre.json', {'network_verified': True, 'image_verified': False, 'gateway_installed': False}))
    checks.extend(['create_host_path_true_refused', 'host_port_refused', 'network_name_refused',
                   'missing_bind_refused', 'writable_config_refused', 'preverified_receipt_refused'])

    backup_dir = str(tmp / 'backup')
    stopped = write(tmp / 'container.json', container_record())
    plan = ok('backup_plan', 'plan-backup', stopped, '--state-dir', '/var/lib/ndg/state', '--backup-dir', backup_dir)
    assert any('cp -a' in step['command'] for step in plan['commands'])
    assert plan['forbidden'] == ['docker rm -v', 'volume prune', 'system prune']
    checks.append('backup_plan_cold_copy')

    refused('online_backup', 'container_must_be_stopped_before_state_operations', 'plan-backup',
            write(tmp / 'running.json', container_record(running=True)),
            '--state-dir', '/var/lib/ndg/state', '--backup-dir', backup_dir)
    checks.append('online_backup_refused')

    db_entry = {'path': 'ndg.db', 'sha256': 'a' * 64, 'size': 12, 'mode': '0600', 'uid': 10001, 'gid': 10001,
                'quick_check': 'ok'}
    wal_entry = {'path': 'ndg.db-wal', 'sha256': 'b' * 64, 'size': 4, 'mode': '0600', 'uid': 10001, 'gid': 10001}
    ok('backup_verified', 'verify-backup', write(tmp / 'evidence.json', {'files': [db_entry, wal_entry]}))
    refused('quick_check_not_ok', 'sqlite_quick_check_ok_required', 'verify-backup',
            write(tmp / 'badcheck.json', {'files': [{**db_entry, 'quick_check': 'corrupt'}, wal_entry]}))
    refused('missing_hash', 'sha256_required_for_every_file', 'verify-backup',
            write(tmp / 'nohash.json', {'files': [{'path': 'ndg.db', 'size': 12, 'mode': '0600',
                                                   'uid': 10001, 'gid': 10001, 'quick_check': 'ok'}]}))
    refused('state_without_db', 'state_database_must_be_in_backup', 'verify-backup',
            write(tmp / 'nodb.json', {'files': [{'path': 'lock', 'sha256': 'c' * 64, 'size': 1, 'mode': '0600',
                                                 'uid': 10001, 'gid': 10001}]}))
    checks.extend(['backup_chain_verified', 'corrupt_db_refused', 'missing_hash_refused', 'dbless_backup_refused'])

    receipt = write(tmp / 'receipt.json', {'backup_dir': backup_dir, 'source_state_dir': '/var/lib/ndg/state',
                                           'files': ['ndg.db']})
    restore = ok('restore_plan', 'plan-restore', receipt, '--restore-dir', '/var/lib/ndg/state-restored',
                 '--project', 'ndgtest')
    flat = ' '.join(step['command'] for step in restore['commands'])
    assert '--no-build --pull never --no-deps' in flat and 'docker rm -v' not in flat
    refused('restore_into_original', 'restore_must_target_a_fresh_empty_directory_not_the_original',
            'plan-restore', receipt, '--restore-dir', '/var/lib/ndg/state', '--project', 'ndgtest')
    checks.extend(['restore_rebuild_plan', 'restore_into_original_refused'])

    candidate = write(tmp / 'candidate.json', image_record([f'registry.example/ndg@sha256:{"8" * 64}'],
                                                           image_id='sha256:' + '2' * 64))
    upgrade = ok('upgrade_plan', 'plan-upgrade', stopped, candidate)
    assert 'backup' in upgrade['commands'][0]['command']
    refused('same_image_upgrade', 'candidate_must_be_a_different_digest_pinned_image', 'plan-upgrade',
            stopped, write(tmp / 'same.json', image_record([f'registry.example/ndg@{INDEX}'])))
    checks.extend(['upgrade_requires_backup_first', 'same_image_upgrade_refused'])

    uninstall = ok('uninstall_plan', 'plan-uninstall', stopped)
    flat = ' '.join(step['command'] for step in uninstall['commands'])
    assert ' -v' not in flat and 'prune' not in flat and 'rm ' + 'c' * 64 in flat
    assert uninstall['forbidden'] == ['-v', '--force', 'prune', 'name-prefix deletion']
    refused('online_uninstall', 'container_must_be_stopped_before_state_operations', 'plan-uninstall',
            write(tmp / 'running2.json', container_record(running=True)))
    checks.extend(['uninstall_identity_scoped', 'online_uninstall_refused'])


def plan_matrix(base, checks):
    request = fixture(base)
    receipt = base / 'receipt'
    receipt.mkdir(mode=0o700)
    request_path = write(base / 'request.json', request)
    planned = ok('plan_frozen', 'plan', request_path, '--receipt', str(receipt), '--acceptance')
    assert planned['scope'] == 'DISPOSABLE_LINUX_ACCEPTANCE'
    assert planned['steps'] == engine.STEP_ORDER and len(planned['steps']) == 14
    frozen = json.loads((receipt / 'plan.json').read_text())
    digest = frozen['plan_digest']
    assert planned['plan_digest_prefix'] == digest[:12]
    assert stat.S_IMODE((receipt / 'plan.json').stat().st_mode) == 0o600
    checks.append('plan_frozen_with_14_steps')

    def variant(**changes):
        return write(base / ('request-' + str(len(list(base.glob('request-*')))) + '.json'),
                     {**request, **changes})

    refused('acceptance_requires_flag', 'acceptance_scope_requires_explicit_flag', 'plan',
            request_path, '--receipt', str(receipt))
    refused('http_origin_in_production', 'https_origin_required', 'plan',
            write(base / 'prod2.json', {**request, 'scope': 'PRODUCTION', 'origin': 'http://ndg.example.test:24483',
                                        'gateway': {'unit_sha256': 'a' * 64, 'binary_sha256': 'b' * 64,
                                                    'cert': '/etc/ndg/cert.pem', 'key': '/etc/ndg/key.pem'}}),
            '--receipt', str(receipt))
    refused('ip_origin_in_production', 'dns_origin_required', 'plan',
            write(base / 'prod3.json', {**request, 'scope': 'PRODUCTION', 'origin': 'https://10.0.0.5:24483',
                                        'gateway': {'unit_sha256': 'a' * 64, 'binary_sha256': 'b' * 64,
                                                    'cert': '/etc/ndg/cert.pem', 'key': '/etc/ndg/key.pem'}}),
            '--receipt', str(receipt))
    refused('production_needs_gateway', 'gateway_parameters_required', 'plan',
            write(base / 'prod4.json', {**request, 'scope': 'PRODUCTION',
                                        'origin': 'https://ndg.example.test:24483'}), '--receipt', str(receipt))
    refused('acceptance_forbids_gateway', 'gateway_not_in_acceptance_scope', 'plan',
            variant(gateway={'unit_sha256': 'a' * 64, 'binary_sha256': 'b' * 64,
                             'cert': '/etc/ndg/cert.pem', 'key': '/etc/ndg/key.pem'}),
            '--receipt', str(receipt), '--acceptance')
    refused('network_name_rejected', 'full_network_identity_required', 'plan',
            variant(network_id='ndg-net'), '--receipt', str(receipt), '--acceptance')
    refused('state_inside_source', 'state_inside_source_rejected', 'plan',
            variant(parent=request['source_a']), '--receipt', str(receipt), '--acceptance')
    refused('overlapping_sources', 'overlapping_sources_rejected', 'plan',
            variant(source_b=request['source_a']), '--receipt', str(receipt), '--acceptance')
    link = base / 'source-link'
    link.symlink_to(base / 'source_a')
    refused('symlinked_source', 'source_a_symlink_component_rejected', 'plan',
            variant(source_a=str(link)), '--receipt', str(receipt), '--acceptance')
    refused('parent_outside_boundary', 'parent_outside_boundary_rejected', 'plan',
            variant(parent=str(base.parent)), '--receipt', str(receipt), '--acceptance')
    refused('traversal_path', 'parent_normalized_required', 'plan',
            variant(parent=str(base) + '/../' + base.name), '--receipt', str(receipt), '--acceptance')
    refused('control_character_path', 'source_b_control_character_rejected', 'plan',
            variant(source_b=str(base / 'source_b') + '\n'), '--receipt', str(receipt), '--acceptance')
    refused('candidate_equal_to_pin', 'candidate_must_be_a_different_digest_pinned_image', 'plan',
            variant(candidate_image=INDEX), '--receipt', str(receipt), '--acceptance')
    refused('unknown_request_field', 'request_fields_rejected', 'plan',
            variant(extra='x'), '--receipt', str(receipt), '--acceptance')
    checks.extend(['production_origin_and_gateway_gated', 'acceptance_scope_explicit',
                   'network_identity_gated', 'state_inside_source_refused', 'overlap_refused',
                   'symlink_refused', 'boundary_refused', 'traversal_refused', 'control_char_refused',
                   'candidate_pin_refused', 'unknown_field_refused'])


def authorization_matrix(base, checks):
    receipt = base / 'receipt'
    plan_path = str(receipt / 'plan.json')
    frozen = json.loads((receipt / 'plan.json').read_text())
    digest = frozen['plan_digest']

    refused('authorization_needs_digest', 'confirmation_digest_mismatch', 'authorize',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'all', '--confirm', '0' * 12,
            '--acceptance')
    refused('authorization_unknown_step', 'authorization_step_unknown', 'authorize',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'compat,teleport',
            '--confirm', digest[:12], '--acceptance')
    granted = ok('authorization', 'authorize', '--plan', plan_path, '--receipt', str(receipt),
                 '--steps', 'reboot,access,compat,init', '--confirm', digest[:12], '--acceptance')
    assert granted['steps'] == ['compat', 'init', 'access', 'reboot'], 'grant must be stored in canonical order'
    authorization = json.loads((receipt / 'authorization.json').read_text())
    assert authorization['plan_digest'] == digest
    assert stat.S_IMODE((receipt / 'authorization.json').stat().st_mode) == 0o600
    refused('authorization_is_immutable', 'authorization_already_exists', 'authorize',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'all', '--confirm', digest[:12],
            '--acceptance')
    checks.extend(['authorization_digest_bound', 'authorization_canonical_order',
                   'authorization_private', 'authorization_immutable'])


def run_matrix(base, checks):
    receipt = base / 'receipt'
    plan_path = str(receipt / 'plan.json')
    refused('run_requires_execute', 'explicit_execute_required', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--acceptance')
    refused('run_requires_authorized_step', 'step_not_authorized', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'start', '--execute',
            '--acceptance')
    refused('run_requires_prior_step', 'previous_step_incomplete', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'init', '--execute',
            '--acceptance')
    refused('run_unknown_step', 'run_step_unknown', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'teleport', '--execute',
            '--acceptance')
    refused('reconcile_needs_interruption', 'reconciliation_requires_interrupted_step', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--reconcile', 'compat', '--execute',
            '--acceptance')

    # A real (expected) failure must be journaled as failed and exit non-zero.
    result = run('run', '--plan', plan_path, '--receipt', str(receipt), '--steps', 'compat',
                 '--execute', '--acceptance')
    journal = [json.loads(line) for line in (receipt / 'journal.jsonl').read_text().splitlines() if line]
    if result.returncode == 0:
        assert [entry['phase'] for entry in journal] == ['start', 'ok']
    else:
        reason = json.loads(result.stderr)['reason']
        assert [entry['phase'] for entry in journal] == ['start', 'failed']
        assert journal[-1]['detail']['reason'] == reason
    checks.append('real_refusal_journaled_and_exit_nonzero')

    # A tampered plan must not authorize anything.
    tampered = json.loads((receipt / 'plan.json').read_text())
    tampered['root'] = tampered['parent'] + '/elsewhere'
    write(receipt / 'plan.json', tampered)
    refused('tampered_plan', 'plan_digest_mismatch', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'compat', '--execute',
            '--acceptance')
    tampered['root'] = tampered['parent'] + '/ndg-readonly-' + tampered['installation_id']
    write(receipt / 'plan.json', tampered)
    checks.append('tampered_plan_refused')

    # Journal: an interrupted non-reconcilable step must not auto-continue.
    journal_path = receipt / 'journal.jsonl'
    with open(journal_path, 'ab') as handle:
        for step in ('sources', 'image'):
            handle.write(engine.canonical({'seq': 0, 'at': 'now', 'step': step, 'phase': 'ok'}) + b'\n')
        handle.write(engine.canonical({'seq': 0, 'at': 'now', 'step': 'init', 'phase': 'ok'}) + b'\n')
        for step in ('gateway', 'start'):
            handle.write(engine.canonical({'seq': 0, 'at': 'now', 'step': step, 'phase': 'ok'}) + b'\n')
        handle.write(engine.canonical({'seq': 0, 'at': 'now', 'step': 'access', 'phase': 'start'}) + b'\n')
        handle.flush()
        os.fsync(handle.fileno())
    refused('interrupted_step_needs_human', 'manual_reconciliation_required', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--steps', 'access', '--execute',
            '--acceptance')
    refused('non_reconcilable_step_refused', 'step_not_reconcilable', 'run',
            '--plan', plan_path, '--receipt', str(receipt), '--reconcile', 'access', '--execute',
            '--acceptance')
    checks.extend(['interrupted_step_not_auto_continued', 'non_reconcilable_reconcile_refused'])

    status = ok('status', 'status', '--receipt', str(receipt))
    assert status['plan_digest_prefix'] and status['steps']['access'] == 'start'
    assert 'parent' not in json.dumps(status), 'status must not echo private paths'
    checks.append('status_reads_journal_without_paths')


def filesystem_matrix(base, checks):
    tree = base / 'tree'
    tree.mkdir(mode=0o700)
    (tree / 'nested').mkdir(mode=0o750)
    (tree / 'a file with spaces.txt').write_text('spaces are one argument')
    (tree / 'unicode-数据.bin').write_bytes(b'\x00\x01\x02')
    (tree / 'nested' / 'empty').write_bytes(b'')
    os.chmod(tree / 'nested' / 'empty', 0o640)

    copied = base / 'copy'
    engine.copy_tree(str(tree), str(copied))
    assert engine.manifest(str(tree)) == engine.manifest(str(copied)), 'copy must preserve every byte and mode'
    checks.append('copy_tree_preserves_bytes_modes_and_odd_names')

    link = tree / 'link'
    link.symlink_to(tree / 'a file with spaces.txt')
    try:
        engine.manifest(str(tree))
    except engine.Refused as refusal:
        assert str(refusal) == 'non_regular_file_rejected'
    else:
        raise AssertionError('symlink in tree must be refused')
    link.unlink()
    checks.append('tree_symlink_refused')

    # umask must not be able to filter the intended modes.
    previous = os.umask(0o777)
    try:
        private = base / 'umask-dir'
        engine.ensure_dir(str(private), 0o700)
        assert stat.S_IMODE(private.stat().st_mode) == 0o700
        engine.write_new(str(private), 'secret', b'value', 0o600)
        assert stat.S_IMODE((private / 'secret').stat().st_mode) == 0o600
    finally:
        os.umask(previous)
    checks.append('modes_asserted_after_umask')

    engine.atomic_write(str(base / 'atomic'), b'first', 0o600)
    engine.atomic_write(str(base / 'atomic'), b'second', 0o600)
    assert (base / 'atomic').read_bytes() == b'second'
    assert not [name for name in os.listdir(base) if name.startswith('.atomic.new-')], 'temp file leaked'
    checks.append('atomic_write_replaces_without_temp_leak')

    for bad, reason in ((str(tree) + '/../x', 'path_normalized_required'),
                        ('relative/path', 'path_absolute_required'),
                        (str(tree) + '\n', 'path_control_character_rejected')):
        try:
            engine.strict_path(bad, 'path')
        except engine.Refused as refusal:
            assert str(refusal) == reason, (bad, str(refusal))
        else:
            raise AssertionError(f'{bad!r} must be refused')
    checks.append('strict_path_rejections')

    journal_receipt = base / 'journal-receipt'
    journal_receipt.mkdir(mode=0o700)
    journal = engine.Journal(str(journal_receipt))
    journal.append('compat', 'start')
    journal.append('compat', 'ok', {'k': 'v'})
    journal.append('sources', 'failed', {'reason': 'x'})
    assert journal.state() == {'compat': 'ok', 'sources': 'failed'}
    assert [entry['phase'] for entry in journal.entries()] == ['start', 'ok', 'failed']
    try:
        journal.append('compat', 'teleport')
    except engine.Refused as refusal:
        assert str(refusal) == 'journal_unknown_phase'
    else:
        raise AssertionError('unknown phase must be refused')
    checks.append('journal_append_only_and_typed')


def runner_matrix(base, checks):
    runner = engine.Runner(record_path=str(base / 'actions.jsonl'))
    code, stdout, _ = runner.run(['/bin/echo', 'one token with spaces', 'two'])
    assert code == 0 and stdout.strip() == b'one token with spaces two', 'argv must not be split'
    recorded = [json.loads(line)['argv'] for line in (base / 'actions.jsonl').read_text().splitlines()]
    assert recorded[-1] == ['/bin/echo', 'one token with spaces', 'two']
    checks.append('runner_preserves_argv_without_shell')

    for bad in ('echo hi', ['sh', '-c', 'echo hi'], ['/bin/bash', '-c', 'id'],
                ['/bin/echo', 'eval'], ['/bin/echo', 'bad\nvalue']):
        try:
            engine.assert_argv(bad)
        except engine.Refused:
            pass
        else:
            raise AssertionError(f'{bad!r} must be refused')
    checks.append('runner_refuses_shell_forms')

    class Handler(BaseHTTPRequestHandler):
        seen = []

        def do_GET(self):
            Handler.seen.append((self.path, dict(self.headers)))
            body = b'{"state":"alive"}'
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    server = HTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        client = engine.HTTP('http://127.0.0.1:' + str(server.server_port))
        status, _, raw = client.request('GET', '/healthz')
        assert status == 200 and json.loads(raw) == {'state': 'alive'}
        assert Handler.seen[-1][1].get('Origin') == 'http://127.0.0.1:' + str(server.server_port)
    finally:
        server.shutdown()
        server.server_close()
    checks.append('http_real_request_with_origin')


def main():
    checks = []
    tmp = Path(tempfile.mkdtemp(prefix='ndg-lifecycle-proof-'))
    tmp = Path(os.path.realpath(tmp))
    os.chmod(tmp, stat.S_IRWXU)
    os.environ['no_proxy'] = '*'
    os.environ['NO_PROXY'] = '*'

    static_invariants(checks)
    evidence_matrix(tmp, checks)
    base = tmp / 'chain'
    base.mkdir(mode=0o700)
    plan_matrix(base, checks)
    authorization_matrix(base, checks)
    run_matrix(base, checks)
    fsbase = tmp / 'fs'
    fsbase.mkdir(mode=0o700)
    filesystem_matrix(fsbase, checks)
    runner_matrix(tmp, checks)

    print(json.dumps({'result': 'PASS',
                      'scope': 'OFFLINE_MATRIX_PLUS_REAL_LOCAL_FILES_AND_HTTP_NOT_CONTAINER_CHAIN',
                      'checks': checks}, sort_keys=True))


if __name__ == '__main__':
    main()
