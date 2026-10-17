#!/usr/bin/env python3
"""Real Linux amd64 install/uninstall chain on the published pinned image.

This is the independent-Linux half of R5-5: it pulls the frozen OCI index,
creates an installation-only internal bridge, and drives the executable
lifecycle end to end against real Docker:

    compat -> sources -> image -> init -> start -> browser -> access -> gui
           -> reboot -> backup -> uninstall

Then it proves the uninstall kept every source byte, the cold backup, and the
state directory. It is NOT a fnOS acceptance: no NAS, no gateway, no machine
reboot. Requires root and GitHub-hosted Linux (it creates networks and
containers, so it must never run on a user NAS).
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys

HELPER = Path(__file__).with_name('fnos-install-lifecycle.py')
INDEX = 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1'
IMAGE = 'ghcr.io/fnb2026/ndg-readonly-preview@' + INDEX
STATE = {}


def run(argv, timeout=600, check=True):
    completed = subprocess.run(argv, stdin=subprocess.DEVNULL, capture_output=True,
                              timeout=timeout, shell=False)
    if check and completed.returncode != 0:
        raise RuntimeError('command failed: ' + ' '.join(argv[:3]) + ' :: ' +
                           completed.stderr.decode('utf-8', 'replace')[:400])
    return completed


def cli(*argv, timeout=900):
    completed = run([sys.executable, '-B', str(HELPER), *argv], timeout=timeout, check=False)
    if completed.returncode != 0:
        raise RuntimeError('lifecycle refused: ' + completed.stderr.decode('utf-8', 'replace')[:400])
    return json.loads(completed.stdout)


def tree_digest(root):
    entries = []
    for current, directories, files in os.walk(root, followlinks=False):
        directories.sort()
        for name in sorted(files):
            path = os.path.join(current, name)
            info = os.lstat(path)
            assert not os.path.islink(path), 'symlink appeared in source'
            digest = hashlib.sha256()
            with open(path, 'rb') as handle:
                for block in iter(lambda: handle.read(1 << 20), b''):
                    digest.update(block)
            entries.append([os.path.relpath(path, root), info.st_ino,
                            digest.hexdigest(), info.st_size])
    return entries


def preconditions():
    assert sys.platform == 'linux', 'Linux required'
    assert os.uname().machine == 'x86_64', 'amd64 required for the frozen image'
    assert os.geteuid() == 0, 'root required'
    assert os.environ.get('GITHUB_ACTIONS') == 'true', 'GitHub-hosted runner required'
    assert os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted', 'hosted runner required'
    assert not Path('/usr/trim').exists(), 'never run this proof on fnOS'
    run(['docker', 'version', '--format', '{{.Server.Version}}'], timeout=60)
    run(['docker', 'compose', 'version', '--short'], timeout=60)


def diagnose(fixture, installation_id, core_ip):
    """Bounded failure evidence; never used to turn a failure into a pass."""
    print('--- diagnostics (bounded) ---', file=sys.stderr)
    for argv in (['docker', 'container', 'ls', '-a', '--format', '{{.ID}} {{.Names}} {{.Status}}'],
                 ['docker', 'network', 'ls', '--format', '{{.ID}} {{.Name}}']):
        completed = subprocess.run(argv, capture_output=True, timeout=60, shell=False)
        print(completed.stdout.decode('utf-8', 'replace')[:2000], file=sys.stderr)
    name = 'ndg-' + installation_id + '-core'
    for argv in (['docker', 'container', 'inspect', name, '--format',
                  '{{json .State}} {{json .NetworkSettings.Networks}}'],
                 ['docker', 'logs', '--tail', '40', name],
                 ['ip', '-4', 'route']):
        completed = subprocess.run(argv, capture_output=True, timeout=60, shell=False)
        print('$', ' '.join(argv[:3]), file=sys.stderr)
        print((completed.stdout + completed.stderr).decode('utf-8', 'replace')[:2000], file=sys.stderr)
    receipt = fixture / 'receipt'
    if (receipt / 'journal.jsonl').exists():
        print('--- journal ---', file=sys.stderr)
        print((receipt / 'journal.jsonl').read_text()[-4000:], file=sys.stderr)


def main():
    checks = []
    preconditions()
    os.umask(0o077)
    fixture = Path('/var/lib') / ('ndg-chain-' + secrets.token_hex(12))
    fixture.mkdir(mode=0o700)
    for name in ('parent', 'source_a', 'source_b', 'backups'):
        (fixture / name).mkdir(mode=0o700)
    (fixture / 'source_a' / 'alpha.txt').write_text('alpha line\n' * 64)
    (fixture / 'source_a' / 'notes one.md').write_text('# notes\n')
    (fixture / 'source_b' / 'beta.txt').write_text('beta line\n' * 32)
    # The core runs as 10001 and must be able to traverse and read sources;
    # the directories stay root-owned so the trusted-ancestor contract holds.
    for label in ('source_a', 'source_b'):
        for current, directories, files in os.walk(fixture / label):
            os.chown(current, 0, 10001)
            os.chmod(current, 0o750)
            for name in files:
                path = os.path.join(current, name)
                os.chown(path, 0, 10001)
                os.chmod(path, 0o640)
    baseline = {'source_a': tree_digest(fixture / 'source_a'),
                'source_b': tree_digest(fixture / 'source_b')}
    checks.append('fixture_created_with_known_source_bytes')

    installation_id = secrets.token_hex(6)
    third = 100 + int(installation_id[:2], 16) % 100
    subnet = '10.253.' + str(third) + '.0/24'
    core_ip = '10.253.' + str(third) + '.2'
    STATE.update(fixture=fixture, installation_id=installation_id, core_ip=core_ip)
    network_name = 'ndg-chain-' + installation_id
    run(['docker', 'network', 'create', '--driver', 'bridge', '--internal',
         '--subnet', subnet, '--gateway', '10.253.' + str(third) + '.1',
         '--label', 'com.ndg.test.nonce=' + installation_id, network_name], timeout=120)
    network_id = run(['docker', 'network', 'inspect', '-f', '{{.Id}}', network_name],
                     timeout=60).stdout.decode().strip()
    assert len(network_id) == 64 and set(network_id) <= set('0123456789abcdef'), network_id
    checks.append('installation_only_internal_bridge_created')

    run(['docker', 'pull', IMAGE], timeout=900)
    inspect = json.loads(run(['docker', 'image', 'inspect', IMAGE], timeout=60).stdout)[0]
    assert any(str(d).endswith('@' + INDEX) for d in inspect.get('RepoDigests') or []), \
        'pulled image lost its pinned repo digest'
    checks.append('published_pinned_image_pulled_by_digest')

    receipt = fixture / 'receipt'
    receipt.mkdir(mode=0o700)
    request = {
        'scope': 'DISPOSABLE_LINUX_ACCEPTANCE',
        'installation_id': installation_id,
        'parent': str(fixture / 'parent'),
        'boundary_root': str(fixture),
        'source_a': str(fixture / 'source_a'),
        'source_b': str(fixture / 'source_b'),
        'origin': 'http://' + core_ip + ':8080',
        'core_ipv4': core_ip,
        'network_id': network_id,
        'backup_dir': str(fixture / 'backups' / installation_id),
        'network_owned': True,
        'expected_scanned': 2,
    }
    request_path = fixture / 'request.json'
    request_path.write_text(json.dumps(request))
    request_path.chmod(0o600)

    planned = cli('plan', str(request_path), '--receipt', str(receipt), '--acceptance')
    digest = planned['plan_digest_prefix']
    checks.append('plan_frozen_and_digest_bound')

    granted = cli('authorize', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                  '--steps', 'compat,sources,image,init,start,browser,access,gui,backup,reboot,uninstall',
                  '--confirm', digest, '--acceptance')
    assert len(granted['steps']) == 11, granted['steps']
    checks.append('eleven_steps_explicitly_authorized')

    first = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'compat,sources,image,init,start,browser,access,gui',
                '--execute', '--acceptance')
    assert first['steps'][-1] == 'gui', first
    checks.append('install_start_browser_access_gui_executed')

    restarted = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                    '--steps', 'reboot', '--execute', '--acceptance')
    assert restarted['steps'] == ['reboot']
    checks.append('process_restart_preserved_snapshot')

    final = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'backup,uninstall', '--execute', '--acceptance')
    assert final['steps'] == ['backup', 'uninstall']
    checks.append('cold_backup_then_identity_scoped_uninstall')

    status = cli('status', '--receipt', str(receipt))
    for step in ('compat', 'sources', 'image', 'init', 'start', 'browser', 'access', 'gui',
                 'backup', 'uninstall', 'reboot'):
        assert status['steps'].get(step) == 'ok', (step, status['steps'])
    checks.append('journal_reports_every_step_ok')

    containers = run(['docker', 'container', 'ls', '-a', '--filter',
                      'label=com.ndg.test.nonce=' + installation_id, '--format', '{{.ID}}'],
                     timeout=60).stdout.decode().strip()
    assert not containers, 'uninstall left a container behind'
    gone = run(['docker', 'network', 'inspect', network_id], timeout=60, check=False)
    assert gone.returncode != 0, 'uninstall left the owned network behind'
    checks.append('container_and_owned_network_removed_by_identity')

    for label in ('source_a', 'source_b'):
        assert tree_digest(fixture / label) == baseline[label], f'uninstall changed {label}'
    state = fixture / 'parent' / ('ndg-readonly-' + installation_id) / 'state'
    assert state.is_dir(), 'uninstall removed the state directory'
    receipt_payload = json.loads((receipt / 'backup-receipt.json').read_text())
    assert receipt_payload['files'], 'backup receipt is empty'
    for entry in receipt_payload['files']:
        backed = Path(receipt_payload['backup_dir']) / 'state' / entry['path']
        assert backed.is_file(), 'backup file disappeared: ' + entry['path']
    checks.append('sources_state_and_backup_preserved')

    print(json.dumps({'result': 'PASS',
                      'scope': 'GITHUB_HOSTED_LINUX_AMD64_REAL_DOCKER_NOT_FNOS',
                      'image': INDEX[:19] + '…',
                      'checks': checks}, sort_keys=True))
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:  # noqa: BLE001 - the exit code carries the verdict
        if STATE:
            try:
                diagnose(**STATE)
            except Exception:  # diagnostics must never hide the original failure
                pass
        print(json.dumps({'result': 'FAIL', 'error': type(error).__name__ + ': ' + str(error)[:400]}),
              file=sys.stderr)
        sys.exit(1)
