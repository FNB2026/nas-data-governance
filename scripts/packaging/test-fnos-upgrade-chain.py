#!/usr/bin/env python3
"""Real amd64 install -> upgrade -> rollback chain against a second candidate.

This is the disposable-Linux half of the fnOS upgrade gate. It proves, on real
Docker, that the production installer can move an existing installation onto a
different digest-pinned image, keep the data and the read-only contract intact,
survive an interrupted step, and roll back to the original image with the
original state.

The second image is built from this checkout and hosted in a throwaway local
registry. Nothing is pushed to GHCR, no existing tag is touched, and the
production PINS are unchanged: only the acceptance scope may name another
repository, and only together with a candidate digest.

Requires root, GitHub-hosted Linux, and a Docker daemon that trusts
127.0.0.1:5000 as an insecure registry (see fnos-web-candidate.yml).

Exit code carries the verdict. PASS here means the Linux/Docker automation
closed the loop; it is not fnOS host acceptance and not a release approval.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import time
import urllib.request

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import fnos_install_engine as engine  # noqa: E402 - needs sys.path first

HELPER = HERE / 'fnos-install-lifecycle.py'
REGISTRY = '127.0.0.1:5000'
CANDIDATE_NAME = 'ndg-readonly-candidate'
CANDIDATE_REPO = REGISTRY + '/' + CANDIDATE_NAME
CANDIDATE_TAG = 'upgrade-acceptance'
CANDIDATE_REF = CANDIDATE_REPO + ':' + CANDIDATE_TAG
REGISTRY_CONTAINER = 'ndg-acceptance-registry'
STATE = {}


def run(argv, timeout=600, check=True, cwd=None):
    completed = subprocess.run(argv, stdin=subprocess.DEVNULL, capture_output=True,
                               timeout=timeout, shell=False, cwd=cwd)
    if check and completed.returncode != 0:
        raise RuntimeError('command failed: ' + ' '.join(argv[:4]) + ' :: ' +
                           completed.stderr.decode('utf-8', 'replace')[:500])
    return completed


def cli(*argv, timeout=1200):
    completed = run([sys.executable, '-B', str(HELPER), *argv], timeout=timeout, check=False)
    if completed.returncode != 0:
        raise RuntimeError('lifecycle refused: ' + completed.stderr.decode('utf-8', 'replace')[:500])
    return json.loads(completed.stdout)


def refusal(*argv, timeout=1200, label=''):
    completed = run([sys.executable, '-B', str(HELPER), *argv], timeout=timeout, check=False)
    if completed.returncode == 0:
        raise RuntimeError((label + ': ' if label else '') +
                           'expected a typed refusal, got success: ' +
                           completed.stdout.decode('utf-8', 'replace')[:300])
    payload = json.loads(completed.stderr.decode('utf-8', 'replace'))
    if payload.get('result') != 'REFUSED' or not payload.get('reason'):
        raise RuntimeError('refusal was not machine-readable: ' + str(payload)[:300])
    return payload['reason']


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
            entries.append([os.path.relpath(path, root), info.st_ino, digest.hexdigest(), info.st_size])
    return entries


def manifest_digests(root):
    entries = []
    for current, directories, files in os.walk(root, followlinks=False):
        directories.sort()
        for name in sorted(files):
            path = os.path.join(current, name)
            entries.append([os.path.relpath(path, root),
                            hashlib.sha256(Path(path).read_bytes()).hexdigest()])
    return entries


def compose_image_line(root):
    for current, _directories, files in os.walk(root):
        if 'compose.yaml' not in files:
            continue
        for line in (Path(current) / 'compose.yaml').read_text().splitlines():
            if 'image:' in line and 'ndg-readonly-' in line:
                return line.strip()
    raise RuntimeError('installed compose.yaml image binding not found')


def preconditions():
    assert sys.platform == 'linux', 'Linux required'
    assert os.uname().machine == 'x86_64', 'amd64 required'
    assert os.geteuid() == 0, 'root required'
    assert os.environ.get('GITHUB_ACTIONS') == 'true', 'GitHub-hosted runner required'
    assert os.environ.get('RUNNER_ENVIRONMENT') == 'github-hosted', 'hosted runner required'
    assert not Path('/usr/trim').exists(), 'never run this proof on fnOS'
    run(['docker', 'version', '--format', '{{.Server.Version}}'], timeout=60)
    run(['docker', 'compose', 'version', '--short'], timeout=60)


def start_registry():
    run(['docker', 'rm', '-f', REGISTRY_CONTAINER], timeout=120, check=False)
    run(['docker', 'run', '-d', '--name', REGISTRY_CONTAINER,
         '--publish', '127.0.0.1:5000:5000', 'registry:2'], timeout=300)
    deadline = time.time() + 60
    while time.time() < deadline:
        probe = run(['docker', 'exec', REGISTRY_CONTAINER, 'wget', '-q', '-O', '-',
                     'http://127.0.0.1:5000/v2/'], timeout=30, check=False)
        if probe.returncode == 0:
            return
        time.sleep(2)
    raise RuntimeError('local registry did not become ready')


def stop_registry():
    run(['docker', 'rm', '-f', REGISTRY_CONTAINER], timeout=120, check=False)


def registry_manifest(reference, accept):
    """Read a manifest straight from the throwaway registry.

    The registry is authoritative: `docker buildx imagetools inspect --raw`
    echoes an index back when given an index digest, and parsing CLI text would
    make the evidence depend on the tool version. The registry response carries
    both the bytes and the digest it is served under.
    """
    url = 'http://127.0.0.1:5000/v2/' + CANDIDATE_NAME + '/manifests/' + reference
    request = urllib.request.Request(url, headers={'Accept': accept})
    with urllib.request.urlopen(request, timeout=60) as response:
        return response.read(), response.headers.get('Docker-Content-Digest')


def build_candidate():
    """Build the second image from this checkout and publish an OCI index to the
    throwaway registry. Returns the digest/config identity actually published,
    read back from the registry rather than assumed from the build output."""
    source_sha = run(['git', 'rev-parse', 'HEAD'], cwd=str(HERE.parent.parent)).stdout.decode().strip()
    stamp = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    run(['docker', 'buildx', 'build',
         '--platform', 'linux/amd64',
         '--build-arg', 'SOURCE_SHA=' + source_sha,
         '--build-arg', 'SOURCE_TREE=upgrade-acceptance-candidate',
         '--build-arg', 'CORE_VERSION=0.5.0-upgrade-candidate',
         '--build-arg', 'SOURCE_BUILD_TIME=' + stamp,
         '--tag', CANDIDATE_REF,
         '--file', 'packaging/docker/Dockerfile',
         '--push', '.'], timeout=2400, cwd=str(HERE.parent.parent))
    body, index_digest = registry_manifest(
        CANDIDATE_TAG, 'application/vnd.oci.image.index.v1+json,'
                       'application/vnd.docker.distribution.manifest.list.v2+json')
    assert index_digest and index_digest.startswith('sha256:') and len(index_digest) == 71, index_digest
    index = json.loads(body)
    amd64 = [m for m in index.get('manifests') or []
             if (m.get('platform') or {}).get('os') == 'linux'
             and (m.get('platform') or {}).get('architecture') == 'amd64']
    assert len(amd64) == 1, 'candidate index must carry exactly one linux/amd64 manifest'
    amd64_manifest = amd64[0]['digest']
    body, served = registry_manifest(
        amd64_manifest, 'application/vnd.oci.image.manifest.v1+json,'
                        'application/vnd.docker.distribution.manifest.v2+json')
    assert served == amd64_manifest, (served, amd64_manifest)
    config_digest = json.loads(body)['config']['digest']
    assert index_digest != engine.PINS['oci_index'], 'candidate must not equal the pinned index'
    assert config_digest != engine.PINS['amd64_config'], 'candidate must not equal the pinned config'
    return {'source_sha': source_sha,
            'source_tree': 'upgrade-acceptance-candidate',
            'build_time': stamp,
            'repo': CANDIDATE_REPO,
            'index': index_digest,
            'amd64_manifest': amd64_manifest,
            'config': config_digest,
            'platforms': [{'os': (m.get('platform') or {}).get('os'),
                           'architecture': (m.get('platform') or {}).get('architecture')}
                          for m in index.get('manifests') or []]}


def candidate_contains(root):
    for current, _directories, files in os.walk(root):
        for name in files:
            if name.endswith('.db'):
                completed = run(['python3', '-c',
                                 'import sqlite3,sys;c=sqlite3.connect(sys.argv[1]);'
                                 'print(c.execute("PRAGMA quick_check").fetchone()[0])',
                                 os.path.join(current, name)], timeout=120, check=False)
                if completed.returncode != 0 or completed.stdout.strip() != b'ok':
                    return False, os.path.join(current, name)
    return True, ''


def main():
    checks = []
    preconditions()
    os.umask(0o077)
    checks.append('github_hosted_linux_amd64_real_docker')

    start_registry()
    checks.append('throwaway_local_registry_running')

    candidate = build_candidate()
    STATE['candidate'] = candidate
    checks.append('second_candidate_index_built_and_read_back')
    print(json.dumps({'stage': 'candidate', **candidate}, sort_keys=True), file=sys.stderr)

    fixture = Path('/var/lib') / ('ndg-upgrade-' + secrets.token_hex(12))
    fixture.mkdir(mode=0o700)
    for name in ('parent', 'source_a', 'source_b', 'backups'):
        (fixture / name).mkdir(mode=0o700)
    (fixture / 'source_a' / 'alpha.txt').write_text('alpha line\n' * 64)
    (fixture / 'source_a' / 'notes one.md').write_text('# notes\n')
    (fixture / 'source_b' / 'beta.txt').write_text('beta line\n' * 32)
    for label in ('source_a', 'source_b'):
        for current, _directories, files in os.walk(fixture / label):
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
    network_name = 'ndg-upgrade-' + installation_id
    run(['docker', 'network', 'create', '--driver', 'bridge', '--internal',
         '--subnet', subnet, '--gateway', '10.253.' + str(third) + '.1',
         '--label', 'com.ndg.test.nonce=' + installation_id, network_name], timeout=120)
    network_id = run(['docker', 'network', 'inspect', '-f', '{{.Id}}', network_name],
                     timeout=60).stdout.decode().strip()
    checks.append('installation_only_internal_bridge_created')

    run(['docker', 'pull', engine.PINS['image']], timeout=900)
    checks.append('pinned_preview_image_pulled_by_digest')

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
        'candidate_image': candidate['index'],
        'candidate_repo': CANDIDATE_REPO,
    }
    request_path = fixture / 'request.json'
    request_path.write_text(json.dumps(request))
    request_path.chmod(0o600)

    # --- source restrictions that must hold before anything runs ---
    for label, patch, reason in (
            ('production rejects a non-pinned repo',
             {'scope': 'PRODUCTION', 'candidate_repo': CANDIDATE_REPO,
              'origin': 'https://ndg.example.com:8443',
              'gateway': {'unit_sha256': 'a' * 64, 'binary_sha256': 'b' * 64,
                          'cert': '/tmp/none', 'key': '/tmp/none'}},
             'production_candidate_repo_must_be_pinned_ghcr'),
            ('candidate repo without a candidate is rejected',
             {'candidate_image': None, 'candidate_repo': CANDIDATE_REPO},
             'candidate_repo_requires_candidate_image'),
            ('a repo carrying a digest is rejected',
             {'candidate_repo': CANDIDATE_REPO + '@sha256:' + 'a' * 64},
             'candidate_repo_shape_rejected')):
        probe_dir = fixture / ('neg-' + secrets.token_hex(4))
        probe_dir.mkdir(mode=0o700)
        probe = dict(request)
        probe.update(patch)
        path = probe_dir / 'request.json'
        path.write_text(json.dumps(probe))
        path.chmod(0o600)
        got = refusal('plan', str(path), '--receipt', str(probe_dir / 'receipt'), '--acceptance',
                      label=label)
        assert got == reason, (label, got)
    checks.append('candidate_source_rules_refuse_before_execution')

    planned = cli('plan', str(request_path), '--receipt', str(receipt), '--acceptance')
    digest = planned['plan_digest_prefix']
    steps = 'compat,sources,image,init,start,browser,access,gui,backup,upgrade,rollback,uninstall'
    cli('authorize', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
        '--steps', steps, '--confirm', digest, '--acceptance')
    checks.append('plan_frozen_and_twelve_steps_authorized')

    first = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'compat,sources,image,init,start,browser,access,gui',
                '--execute', '--acceptance')
    assert first['steps'][-1] == 'gui', first
    checks.append('original_pinned_image_installed_and_scanned')

    container_name = 'ndg-' + installation_id + '-core'
    original = json.loads(run(['docker', 'container', 'inspect', container_name], timeout=60).stdout)[0]
    recorded_image = json.loads((receipt / 'image.json').read_text())
    original_image = original['Image']
    assert original_image == recorded_image['image_id'], 'container is not on the recorded image identity'
    assert recorded_image['identity_kind'] in ('oci_index', 'amd64_config'), recorded_image
    binding_before = compose_image_line(fixture / 'parent')
    checks.append('original_install_bound_to_recorded_identity')

    # --- cold backup of the still-original install, before any upgrade runs ---
    cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
        '--steps', 'backup', '--execute', '--acceptance')
    pre_upgrade_state = manifest_digests(fixture / 'parent' / ('ndg-readonly-' + installation_id) / 'state')
    checks.append('cold_backup_taken_before_upgrade')

    # --- refusal: the candidate is not in the local store yet ---
    before_refusal = json.loads(run(['docker', 'container', 'inspect', container_name], timeout=60).stdout)[0]
    reason = refusal('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                     '--steps', 'upgrade', '--execute', '--acceptance')
    assert reason in ('inspect_failed', 'candidate_inspect_single_record_required'), reason
    assert compose_image_line(fixture / 'parent') == binding_before, 'refused upgrade rewrote compose'
    after_refusal = json.loads(run(['docker', 'container', 'inspect', container_name], timeout=60).stdout)[0]
    assert after_refusal['Image'] == before_refusal['Image'], 'refused upgrade changed the container image'
    assert after_refusal['State']['Running'] == before_refusal['State']['Running'], \
        'refused upgrade changed the container run state'
    checks.append('missing_candidate_refused_without_touching_the_install')

    # --- the real upgrade ---
    run(['docker', 'pull', CANDIDATE_REPO + '@' + candidate['index']], timeout=900)
    upgraded = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                   '--steps', 'upgrade', '--execute', '--acceptance')
    assert upgraded['steps'] == ['upgrade'], upgraded
    record = json.loads(run(['docker', 'container', 'inspect', container_name], timeout=60).stdout)[0]
    candidate_id = json.loads(run(['docker', 'image', 'inspect',
                                   CANDIDATE_REPO + '@' + candidate['index']], timeout=60).stdout)[0]['Id']
    assert record['Image'] == candidate_id, 'container is not on the candidate image identity'
    assert candidate_id != original_image, 'candidate identity equals the original identity'
    assert record['HostConfig']['ReadonlyRootfs'] is True, 'upgrade lost the read-only rootfs'
    assert not record['HostConfig'].get('PortBindings'), 'upgrade started publishing ports'
    assert record['Config']['User'] == '10001:10001', 'upgrade changed the run identity'
    assert compose_image_line(fixture / 'parent') == 'image: ' + CANDIDATE_REPO + '@' + candidate['index']
    checks.append('upgrade_moved_container_to_candidate_config')

    ok, bad = candidate_contains(fixture / 'parent')
    assert ok, 'database quick_check failed after upgrade: ' + bad
    for label in ('source_a', 'source_b'):
        assert tree_digest(fixture / label) == baseline[label], 'upgrade changed ' + label
    checks.append('upgrade_kept_database_quick_check_and_source_bytes')

    after_upgrade = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                        '--steps', 'access', '--execute', '--acceptance')
    assert after_upgrade['steps'] == ['access'], after_upgrade
    checks.append('login_and_state_read_back_after_upgrade')

    # --- idempotence: a second upgrade verifies and skips ---
    again = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'upgrade', '--execute', '--acceptance')
    assert again['steps'] == ['upgrade'], again
    status = cli('status', '--receipt', str(receipt))
    assert status['steps'].get('upgrade') == 'ok', status['steps']
    checks.append('second_upgrade_is_idempotent')

    # --- interrupted step: a dangling start record must reconcile, not reinstall ---
    with open(receipt / 'journal.jsonl', 'a') as handle:
        handle.write(json.dumps({'seq': 9999, 'at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                                 'step': 'upgrade', 'phase': 'start'}) + '\n')
    reconciled = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                     '--steps', 'upgrade', '--reconcile', 'upgrade', '--execute', '--acceptance')
    assert reconciled['result'] == 'RECONCILED' and reconciled['step'] == 'upgrade', reconciled
    checks.append('interrupted_upgrade_reconciled_from_postcheck')

    # --- rollback to the pre-upgrade backup ---
    rolls = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'rollback', '--execute', '--acceptance')
    assert rolls['steps'] == ['rollback'], rolls
    back = json.loads(run(['docker', 'container', 'inspect', container_name], timeout=60).stdout)[0]
    assert back['Image'] == original_image, 'rollback did not return to the recorded image identity'
    assert back['HostConfig']['ReadonlyRootfs'] is True and not back['HostConfig'].get('PortBindings')
    assert compose_image_line(fixture / 'parent') == binding_before, 'rollback rewrote compose differently'
    restored = manifest_digests(fixture / 'parent' / ('ndg-readonly-' + installation_id) / 'state')
    assert restored == pre_upgrade_state, 'rollback restored a different state than the pre-upgrade backup'
    checks.append('rollback_returned_to_pinned_config_and_pre_upgrade_state')

    after_rollback = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                         '--steps', 'access', '--execute', '--acceptance')
    assert after_rollback['steps'] == ['access'], after_rollback
    checks.append('login_and_state_read_back_after_rollback')

    for label in ('source_a', 'source_b'):
        assert tree_digest(fixture / label) == baseline[label], 'rollback changed ' + label
    checks.append('sources_unchanged_across_upgrade_and_rollback')

    final = cli('run', '--plan', str(receipt / 'plan.json'), '--receipt', str(receipt),
                '--steps', 'uninstall', '--execute', '--acceptance')
    assert final['steps'] == ['uninstall'], final
    containers = run(['docker', 'container', 'ls', '-a', '--filter',
                      'label=com.ndg.test.nonce=' + installation_id, '--format', '{{.ID}}'],
                     timeout=60).stdout.decode().strip()
    assert not containers, 'uninstall left a container behind'
    checks.append('identity_scoped_uninstall_removed_only_this_install')

    stop_registry()
    checks.append('throwaway_registry_removed')

    print(json.dumps({'result': 'PASS',
                      'scope': 'GITHUB_HOSTED_LINUX_AMD64_REAL_DOCKER_NOT_FNOS',
                      'source_sha': candidate['source_sha'],
                      'original_index': engine.PINS['oci_index'][:19] + '…',
                      'original_config': engine.PINS['amd64_config'][:19] + '…',
                      'candidate_index': candidate['index'][:19] + '…',
                      'candidate_manifest': candidate['amd64_manifest'][:19] + '…',
                      'candidate_config': candidate['config'][:19] + '…',
                      'checks': checks}, sort_keys=True))
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as error:  # noqa: BLE001 - the exit code carries the verdict
        try:
            stop_registry()
        except Exception:  # cleanup must never hide the original failure
            pass
        print(json.dumps({'result': 'FAIL', 'error': type(error).__name__ + ': ' + str(error)[:500]}),
              file=sys.stderr)
        sys.exit(1)
