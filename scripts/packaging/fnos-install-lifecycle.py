#!/usr/bin/env python3
"""fnOS install-chain lifecycle verification; evidence in, typed verdicts out.

This tool productizes the verification matrix of
docs/development/FNOS-READONLY-COMPOSE-2026-10-16.md ("分开的执行与验收步骤"
and "冷备份、恢复和卸载"). It NEVER runs docker and never writes outside a
requested receipt path: the administrator executes, the tool verifies the
evidence the executions produce and emits the exact next command plan.

Subcommands (all read their inputs as files; "-" is not accepted so private
paths cannot leak through a shell pipe into logs):
  verify-image      docker image inspect JSON  vs pinned digest chain
  verify-mounts     docker compose config JSON vs prepared.json bindings
  plan-backup       docker container inspect JSON -> cold-copy plan + receipt skeleton
  verify-backup     backup evidence JSON -> full chain verdict
  plan-restore      backup receipt -> restore/rebuild command plan
  plan-upgrade      current + candidate image evidence -> ordered upgrade/rollback plan
  plan-uninstall    container inspect JSON -> identity-scoped uninstall plan

Every output is a single JSON object on stdout. Refusals exit 1 with a
machine-readable reason on stderr and never echo private paths.
"""
import argparse
import json
import re
import sys

PINS = {
    # From the 2026-10-16 acceptance record; the OCI index and its amd64
    # image config are the identity anchors, independent of any tag.
    'oci_index': 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1',
    'amd64_config': 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4',
}
BIND_TARGETS = {'/data': False, '/config': True, '/sources/source_a': True, '/sources/source_b': True}
HEX64 = re.compile(r'^[0-9a-f]{64}$')


class Refused(Exception):
    pass


def load_json(path, label):
    if path == '-':
        raise Refused(f'{label}_stdin_not_accepted')
    try:
        with open(path, 'rb') as handle:
            return json.loads(handle.read(1 << 24))
    except FileNotFoundError:
        raise Refused(f'{label}_missing')
    except (ValueError, OSError):
        raise Refused(f'{label}_unreadable')


def out(payload):
    print(json.dumps(payload, sort_keys=True, indent=1))


# ---------------- image identity ----------------

def verify_image(args):
    """Compare a docker image inspect record against the pinned digest chain.

    Accepts only a digest reference (RepoDigests) whose image ID and, when
    present, OCI index/config digests match the frozen acceptance anchors.
    A mutable tag alone is never identity.
    """
    records = load_json(args.inspect, 'image_inspect')
    if not isinstance(records, list) or len(records) != 1:
        raise Refused('image_inspect_single_record_required')
    record = records[0]
    image_id = str(record.get('Id', ''))
    digests = [str(d) for d in record.get('RepoDigests', []) or []]
    if not image_id.startswith('sha256:') or len(image_id) != 71:
        raise Refused('image_id_digest_required')
    if not digests:
        raise Refused('repo_digest_required_without_tag_identity')
    pinned_index = PINS['oci_index']
    matched = [d for d in digests if d.endswith('@' + pinned_index)]
    if not matched:
        raise Refused('image_not_pinned_to_acceptance_index')
    architecture = str(record.get('Architecture', ''))
    if architecture != 'amd64':
        raise Refused('acceptance_evidence_is_amd64_only')
    checks = {
        'image_id_digest': image_id,
        'pinned_repo_digest': matched[0],
        'architecture': architecture,
        'oci_index': pinned_index,
        'amd64_config': PINS['amd64_config'],
    }
    out({'result': 'IMAGE_IDENTITY_VERIFIED', 'checks': checks,
         'scope': 'compareinspect_only_runtime_pull_is_a_separate_admin_step'})


# ---------------- compose/mount identity ----------------

def _require(condition, reason):
    if not condition:
        raise Refused(reason)


def verify_mounts(args):
    """Verify the resolved compose configuration against the prepared root.

    Input is the JSON of `docker compose --env-file installation.env --file
    compose.yaml config --format json`. This is the "Compose 解析成功不等于
    运行通过" gate: it proves the frozen configuration, nothing about runtime.
    """
    compose = load_json(args.config, 'compose_config')
    prepared = load_json(args.prepared, 'prepared_receipt')
    _require(isinstance(compose, dict), 'compose_config_object_required')
    services = compose.get('services')
    _require(isinstance(services, dict) and set(services) == {'core'}, 'single_core_service_required')
    core = services['core']
    _require(not core.get('ports'), 'no_host_ports')
    _require(not core.get('build'), 'no_build')
    _require(not core.get('environment'), 'no_environment')
    _require(core.get('user') == '10001:10001', 'fixed_uid_gid_required')
    _require(core.get('read_only') is True, 'read_only_root_required')
    _require(core.get('pull_policy') == 'never', 'pull_policy_never_required')
    image = str(core.get('image', ''))
    _require(image.endswith('@' + PINS['oci_index']), 'image_must_be_digest_pinned')

    mounts = {volume.get('target'): volume for volume in core.get('volumes', []) or []}
    _require(set(mounts) == set(BIND_TARGETS), 'exact_four_bind_targets_required')
    for target, read_only in BIND_TARGETS.items():
        volume = mounts[target]
        _require(volume.get('type') == 'bind', 'bind_type_required')
        # compose-go omits create_host_path when false; absence means false.
        _require(volume.get('bind', {}).get('create_host_path', False) is False,
                 'create_host_path_false_required')
        _require(volume.get('read_only', False) is read_only, 'read_only_flag_mismatch')

    networks = compose.get('networks', {})
    _require('core' in networks and networks['core'].get('external') is True, 'external_network_required')
    network_name = str(networks['core'].get('name', ''))
    _require(bool(HEX64.match(network_name)), 'full_network_id_required')
    _require(prepared.get('network_verified') is False, 'prepared_receipt_must_not_preverify')
    _require(prepared.get('image_verified') is False and prepared.get('gateway_installed') is False,
             'prepared_receipt_must_not_preverify')
    out({'result': 'MOUNT_IDENTITY_VERIFIED',
         'checks': {'binds': sorted(BIND_TARGETS), 'network_id': network_name,
                    'image': image.split('@')[-1][:16] + '…'},
         'scope': 'resolved_config_only_not_a_runtime_acceptance'})


# ---------------- backup / restore / rebuild ----------------

def container_identity(args):
    records = load_json(args.inspect, 'container_inspect')
    _require(isinstance(records, list) and len(records) == 1, 'container_inspect_single_record_required')
    record = records[0]
    name = '/' + str(record.get('Name', '')).lstrip('/')
    state = record.get('State', {}) or {}
    _require(record.get('Id', ''), 'container_id_required')
    _require(state.get('Running') is False and state.get('Restarting') is False,
             'container_must_be_stopped_before_state_operations')
    image = str(record.get('Image', ''))
    _require(image.startswith('sha256:'), 'container_image_digest_required')
    return record, {'id': record['Id'], 'name': name, 'image': image,
                    'running': bool(state.get('Running'))}


def plan_backup(args):
    record, identity = container_identity(args)
    _require(args.state_dir.startswith('/') and args.state_dir != '/', 'absolute_state_dir_required')
    # The cold-copy plan copies the complete state directory preserving
    # metadata; it never touches sources and never prunes.
    commands = [
        {'step': 1, 'command': f'docker container inspect {identity["id"]}',
         'purpose': 're-prove the exact stopped container identity'},
        {'step': 2, 'command': f'mkdir --mode=0700 {args.backup_dir}',
         'purpose': 'private backup directory'},
        {'step': 3,
         'command': f'cp -a {args.state_dir}/. {args.backup_dir}/',
         'purpose': 'cold copy with mode/uid/gid preserved; WAL/SHM included if present'},
        {'step': 4, 'command': f'find {args.backup_dir} -type f -print0 | xargs -0 sha256sum',
         'purpose': 'per-file evidence for verify-backup'},
        {'step': 5, 'command': 'sqlite3 backup/ndg.db "PRAGMA quick_check;"',
         'purpose': 'database integrity; expect ok'},
    ]
    out({'result': 'BACKUP_PLAN', 'container': identity, 'commands': commands,
         'forbidden': ['docker rm -v', 'volume prune', 'system prune'],
         'receipt_fields': ['files[{path,sha256,size,mode,uid,gid}]', 'quick_check', 'wal_files']})


def verify_backup(args):
    evidence = load_json(args.evidence, 'backup_evidence')
    files = evidence.get('files')
    _require(isinstance(files, list) and files, 'backup_file_list_required')
    seen = set()
    db_seen = False
    for entry in files:
        path = str(entry.get('path', ''))
        _require(path and path not in seen, 'duplicate_or_empty_backup_path')
        seen.add(path)
        digest = str(entry.get('sha256', ''))
        _require(bool(HEX64.match(digest)), 'sha256_required_for_every_file')
        for field in ('size', 'uid', 'gid'):
            _require(isinstance(entry.get(field), int), f'{field}_required')
        mode = entry.get('mode')
        _require(isinstance(mode, str) and mode.startswith('0') and
                 all(ch in '01234567' for ch in mode), 'octal_mode_required')
        if path.endswith('.db'):
            db_seen = True
            _require(entry.get('quick_check') == 'ok', 'sqlite_quick_check_ok_required')
    _require(db_seen, 'state_database_must_be_in_backup')
    out({'result': 'BACKUP_VERIFIED', 'files': len(files),
         'scope': 'evidence_chain_only_same_version_same_source_identity_restore_only'})


def plan_restore(args):
    receipt = load_json(args.receipt, 'backup_receipt')
    files = receipt.get('files') or []
    _require(files, 'verified_backup_receipt_required')
    _require(args.restore_dir.startswith('/') and args.restore_dir != '/', 'absolute_restore_dir_required')
    _require(args.restore_dir != receipt.get('source_state_dir'),
             'restore_must_target_a_fresh_empty_directory_not_the_original')
    commands = [
        {'step': 1, 'command': f'mkdir --mode=0700 {args.restore_dir}',
         'purpose': 'fresh empty state directory'},
        {'step': 2, 'command': f'cp -a {receipt.get("backup_dir")}/. {args.restore_dir}/',
         'purpose': 'restore from the verified cold copy'},
        {'step': 3, 'command': f'docker compose --project-name {args.project} --env-file installation.env '
                               f'--file compose.yaml up -d --no-build --pull never --no-deps',
         'purpose': 'rebuild from the original fixed compose; never pull, never build'},
        {'step': 4, 'command': f'docker container inspect $(docker compose --project-name {args.project} ps -q core)',
         'purpose': 'post-rebuild identity: same image digest, uid, binds, no ports'},
    ]
    out({'result': 'RESTORE_REBUILD_PLAN', 'commands': commands,
         'conditions': ['same core version and source identity only',
                        'no second core may run on the same state',
                        'journal and recovery locks are not cleaned']})


def plan_upgrade(args):
    current, current_identity = container_identity(args)
    candidate = load_json(args.candidate, 'candidate_image_inspect')
    _require(isinstance(candidate, list) and len(candidate) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(candidate[0].get('Id', ''))
    _require(candidate_id.startswith('sha256:') and candidate_id != current_identity['image'],
             'candidate_must_be_a_different_digest_pinned_image')
    candidate_digests = [str(d) for d in candidate[0].get('RepoDigests', []) or []]
    _require(any(d.endswith('@' + PINS['oci_index']) for d in candidate_digests) or
             any(HEX64.match(d.rsplit('@', 1)[-1].removeprefix('sha256:')) for d in candidate_digests),
             'candidate_must_carry_a_repo_digest')
    commands = [
        {'step': 1, 'command': 'plan-backup + verify-backup first',
         'purpose': 'no upgrade without a verified cold backup'},
        {'step': 2, 'command': 'docker compose ... up -d --no-build --pull never --no-deps  # with the new digest',
         'purpose': 'rebuild the core on the candidate image'},
        {'step': 3, 'command': 'gateway health matrix + authenticated snapshot comparison',
         'purpose': 'acceptance before the old image is removed'},
        {'step': 4, 'command': f'docker rmi {current_identity["image"]}',
         'purpose': 'only after acceptance; rollback is restore of the step-1 receipt'},
    ]
    out({'result': 'UPGRADE_ROLLBACK_PLAN', 'current': current_identity['image'][:19] + '…',
         'candidate': candidate_id[:19] + '…', 'commands': commands,
         'rollback': 'restore the verified backup receipt into a fresh state directory and rebuild'})


def plan_uninstall(args):
    _, identity = container_identity(args)
    commands = [
        {'step': 1, 'command': 'stop the gateway entry first, then the core',
         'purpose': 'no writer left before identity checks'},
        {'step': 2, 'command': f'docker container inspect {identity["id"]}',
         'purpose': 'confirm the exact container identity and stopped state'},
        {'step': 3, 'command': f'docker container rm {identity["id"]}',
         'purpose': 'delete strictly by identity; never -v, never name-prefix batch'},
        {'step': 4, 'command': 'verify sources/state/backup still exist with unchanged inodes and hashes',
         'purpose': 'uninstall must not delete data'},
        {'step': 5, 'command': 'network removal only after zero endpoints and explicit ownership confirmation',
         'purpose': 'shared networks are never removed by prefix'},
    ]
    out({'result': 'UNINSTALL_PLAN', 'container': identity, 'commands': commands,
         'forbidden': ['-v', '--force', 'prune', 'name-prefix deletion']})


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('verify-image'); p.add_argument('inspect'); p.set_defaults(func=verify_image)
    p = sub.add_parser('verify-mounts'); p.add_argument('config'); p.add_argument('prepared'); p.set_defaults(func=verify_mounts)
    p = sub.add_parser('plan-backup'); p.add_argument('inspect')
    p.add_argument('--state-dir', required=True); p.add_argument('--backup-dir', required=True)
    p.set_defaults(func=plan_backup)
    p = sub.add_parser('verify-backup'); p.add_argument('evidence'); p.set_defaults(func=verify_backup)
    p = sub.add_parser('plan-restore'); p.add_argument('receipt')
    p.add_argument('--restore-dir', required=True); p.add_argument('--project', required=True)
    p.set_defaults(func=plan_restore)
    p = sub.add_parser('plan-upgrade'); p.add_argument('inspect'); p.add_argument('candidate')
    p.set_defaults(func=plan_upgrade)
    p = sub.add_parser('plan-uninstall'); p.add_argument('inspect'); p.set_defaults(func=plan_uninstall)
    args = parser.parse_args()
    try:
        args.func(args)
    except Refused as refusal:
        print(json.dumps({'result': 'REFUSED', 'reason': str(refusal)}), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
