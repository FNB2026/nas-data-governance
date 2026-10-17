#!/usr/bin/env python3
"""fnOS install-chain lifecycle: evidence verification plus an executable chain.

Two layers live here on purpose:

  * verify-* / plan-*   evidence in, typed verdicts out. Nothing runs.
  * plan / authorize / run / status   the executable lifecycle. It acts only
    through an argv-only runner and only after a frozen plan, an explicit
    per-step grant bound to that plan's digest, and --execute all agree.

Subcommands (inputs are files; "-" is refused so private paths cannot leak
through a shell pipe into logs):
  verify-image      docker image inspect JSON  vs pinned digest chain
  verify-mounts     docker compose config JSON vs prepared.json bindings
  plan-backup       docker container inspect JSON -> cold-copy plan + receipt skeleton
  verify-backup     backup evidence JSON -> full chain verdict
  plan-restore      backup receipt -> restore/rebuild command plan
  plan-upgrade      current + candidate image evidence -> ordered upgrade/rollback plan
  plan-uninstall    container inspect JSON -> identity-scoped uninstall plan
  plan              freeze structured installation parameters into a digest
  preflight         read-only survey of the host and of any existing NDG instance
  authorize         grant named steps against one plan digest
  run               execute only granted steps, journaled and resumable
  status            read the append-only journal back

Every output is a single JSON object on stdout. Refusals exit 1 with a
machine-readable reason on stderr and never echo private paths.
"""
import argparse
import json
import os
import sys

import fnos_install_engine as engine
from fnos_install_engine import PINS, BIND_TARGETS, HEX64, Refused, load_json, out, require


# ---------------- image identity ----------------

def verify_image(args):
    """Compare a docker image inspect record against the pinned digest chain."""
    records = load_json(args.inspect, 'image_inspect')
    require(isinstance(records, list) and len(records) == 1, 'image_inspect_single_record_required')
    detail = engine.verify_image_record(records[0])
    checks = {
        'image_id_digest': detail['image_id'],
        'pinned_repo_digest': detail['pinned_repo_digest'],
        'architecture': detail['architecture'],
        'oci_index': detail['oci_index'],
        'amd64_config': detail['amd64_config'],
    }
    out({'result': 'IMAGE_IDENTITY_VERIFIED', 'checks': checks,
         'scope': 'compareinspect_only_runtime_pull_is_a_separate_admin_step'})


# ---------------- compose/mount identity ----------------

def verify_mounts(args):
    compose = load_json(args.config, 'compose_config')
    prepared = load_json(args.prepared, 'prepared_receipt')
    detail = engine.verify_mounts_payload(compose, prepared)
    out({'result': 'MOUNT_IDENTITY_VERIFIED',
         'checks': {'binds': detail['binds'], 'network_id': detail['network_id'],
                    'image': detail['image']},
         'scope': 'resolved_config_only_not_a_runtime_acceptance'})


# ---------------- backup / restore / rebuild ----------------

def container_identity(args):
    records = load_json(args.inspect, 'container_inspect')
    require(isinstance(records, list) and len(records) == 1, 'container_inspect_single_record_required')
    record, identity = engine.container_identity(records[0])
    require((record.get('State', {}) or {}).get('Running') is False and
            (record.get('State', {}) or {}).get('Restarting') is False,
            'container_must_be_stopped_before_state_operations')
    return record, identity


def plan_backup(args):
    record, identity = container_identity(args)
    require(args.state_dir.startswith('/') and args.state_dir != '/', 'absolute_state_dir_required')
    commands = [
        {'step': 1, 'command': f'docker container inspect {identity["id"]}',
         'purpose': 're-prove the exact stopped container identity'},
        {'step': 2, 'command': f'mkdir --mode=0700 {args.backup_dir}',
         'purpose': 'private backup directory'},
        {'step': 3, 'command': f'cp -a {args.state_dir}/. {args.backup_dir}/',
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
    detail = engine.verify_backup_payload(evidence)
    out({'result': 'BACKUP_VERIFIED', 'files': detail['files'],
         'scope': 'evidence_chain_only_same_version_same_source_identity_restore_only'})


def plan_restore(args):
    receipt = load_json(args.receipt, 'backup_receipt')
    files = receipt.get('files') or []
    require(files, 'verified_backup_receipt_required')
    require(args.restore_dir.startswith('/') and args.restore_dir != '/', 'absolute_restore_dir_required')
    require(args.restore_dir != receipt.get('source_state_dir'),
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
    require(isinstance(candidate, list) and len(candidate) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(candidate[0].get('Id', ''))
    require(candidate_id.startswith('sha256:') and candidate_id != current_identity['image'],
            'candidate_must_be_a_different_digest_pinned_image')
    candidate_digests = [str(d) for d in candidate[0].get('RepoDigests', []) or []]
    require(any(d.endswith('@' + PINS['oci_index']) for d in candidate_digests) or
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


# ---------------- executable lifecycle ----------------

def freeze_plan(args):
    request = load_json(args.request, 'installation_request')
    plan = engine.build_plan(request, args.acceptance)
    engine.private_dir(args.receipt, 'receipt')
    target = os.path.join(args.receipt, 'plan.json')
    require(not os.path.exists(target), 'plan_already_exists')
    engine.atomic_write(target, (json.dumps(plan, indent=1, sort_keys=True) + '\n').encode(), 0o600)
    out({'result': 'PLAN_FROZEN', 'scope': plan['scope'],
         'installation_id': plan['installation_id'],
         'plan_digest_prefix': plan['plan_digest'][:12],
         'steps': engine.STEP_ORDER,
         'next': 'authorize --plan <receipt>/plan.json --receipt <receipt> --steps ... --confirm '
                 + plan['plan_digest'][:12]})


# ---------------- read-only preflight survey ----------------

PREFLIGHT_PROGRAMS = frozenset({'docker', 'docker-compose', 'systemctl', 'uname'})


def _trim(raw, limit=512):
    text = (raw or b'').decode('utf-8', 'replace').strip()
    return text[:limit] if len(text) > limit else text


def _kernel_ok(release):
    parts = release.split('.', 2)[:2]
    try:
        major, minor = int(parts[0]), int(parts[1])
    except (ValueError, IndexError):
        return None
    return (major, minor) >= (5, 10)


def _listening_ports():
    """Which IPv4 ports are already bound. Read-only, Linux only."""
    listeners = {}
    for name in ('/proc/net/tcp', '/proc/net/tcp6'):
        try:
            with open(name) as handle:
                rows = handle.read().splitlines()[1:]
        except OSError:
            continue
        for row in rows:
            fields = row.split()
            if len(fields) < 4 or fields[3] != '0A':  # 0A == LISTEN
                continue
            listeners[int(fields[1].split(':')[1], 16)] = name.split('/')[-1]
    return listeners


def preflight(args):
    """Read-only environment and service identity survey.

    This is investigation only: nothing is written (the Runner gets no
    receipt path), nothing is installed, started, restarted or removed,
    and no public/private boundary is changed. Anything that could not be
    determined is reported under `not_checked` instead of being guessed.
    A preflight report authorises nothing.
    """
    runner = engine.Runner()  # no record_path: no file is ever written
    checks, blocking, not_checked = {}, [], []

    host = os.uname()
    if hasattr(os, 'geteuid'):
        euid = os.geteuid()
        checks['euid'] = euid
        if euid != 0:
            blocking.append('root_required_for_any_install_step')
    else:
        not_checked.append('euid_token_unavailable_on_this_platform')
    # The 5.10 floor is a Linux claim; never evaluate it for another kernel.
    kernel_ok = _kernel_ok(host.release) if host.sysname == 'Linux' else None
    checks['host'] = {'sysname': host.sysname, 'release': host.release,
                      'machine': host.machine, 'kernel_at_least_5_10': kernel_ok}
    if host.sysname != 'Linux':
        blocking.append('surveyed_platform_is_not_linux')
        not_checked.append('kernel_floor_only_defined_for_linux')
    elif kernel_ok is None:
        not_checked.append('kernel_version_unparseable')
    elif not kernel_ok:
        blocking.append('kernel_below_5_10')

    def probe(label, argv, timeout=60):
        try:
            code, stdout, stderr = runner.run(argv, timeout=timeout)
        except Refused as refusal:
            return {'status': 'unavailable', 'reason': str(refusal)}
        return {'status': 'ok' if code == 0 else 'failed', 'exit': code,
                'stdout': _trim(stdout), 'stderr': _trim(stderr)}

    docker = probe('docker_version', ['docker', 'version', '--format', '{{.Server.Version}}'])
    checks['docker_version'] = docker
    if docker.get('status') != 'ok':
        blocking.append('docker_server_unavailable')
        not_checked.extend(['existing_containers', 'existing_networks'])
    else:
        listing = probe('containers', ['docker', 'container', 'ls', '--all', '--no-trunc',
                                       '--format', '{{.Names}}\t{{.Image}}\t{{.Status}}'])
        named = [row for row in listing.get('stdout', '').splitlines()
                 if 'ndg' in row.lower() or PINS['image'].split('@')[0].split('/')[-1] in row.lower()]
        checks['existing_containers'] = {'seen': named, 'total_rows': len(listing.get('stdout', '').splitlines())}
        networks = probe('networks', ['docker', 'network', 'ls', '--format',
                                      '{{.ID}}\t{{.Name}}\t{{.Driver}}\t{{.Internal}}'])
        checks['existing_networks'] = {'rows': networks.get('stdout', '').splitlines()}

    compose = probe('compose', ['docker', 'compose', 'version', '--short'])
    if compose.get('status') != 'ok':
        compose = probe('compose_legacy', ['docker-compose', 'version', '--short'])
    checks['compose_version'] = compose
    if compose.get('status') != 'ok':
        blocking.append('compose_unavailable')

    units = probe('units', ['systemctl', 'list-units', '--all', '--no-legend', '--type=service'])
    if units.get('status') == 'ok':
        checks['existing_units'] = [row for row in units['stdout'].splitlines() if 'ndg' in row.lower()]
    else:
        checks['existing_units'] = {'status': units.get('status')}
        not_checked.append('systemd_units_or_systemd_is_absent')

    listeners = _listening_ports()
    if listeners:
        checks['listening_tcp_ports'] = sorted(listeners)
        for port in (443, 8080, 24483):
            if port in listeners:
                checks.setdefault('ports_in_use_for_ndg', []).append({'port': port, 'family': listeners[port]})
    else:
        not_checked.append('proc_net_tcp_unavailable')

    paths = {}
    for label, value in (('boundary_root', args.root), *[('source', item) for item in args.source]):
        if not value.startswith('/') or value == '/':
            paths[label] = {'path_reported': False, 'finding': 'absolute_path_required'}
            blocking.append(f'{label}_not_absolute')
            continue
        try:
            engine.strict_path(value, label)
            stat = os.lstat(value)
            paths[label] = {'exists': True, 'mode': oct(stat.st_mode & 0o777), 'uid': stat.st_uid,
                            'gid': stat.st_gid, 'identity': 'strict_path_accepted'}
        except Refused as refusal:
            paths[label] = {'exists': os.path.exists(value), 'identity': str(refusal)}
        except FileNotFoundError:
            paths[label] = {'exists': False, 'identity': 'not_present_yet'}
    checks['requested_paths'] = paths

    if args.origin:
        try:
            http = engine.HTTP(args.origin)
            status, _, _ = http.request('GET', '/healthz', timeout=15)
            checks['origin_healthz'] = {'origin': args.origin, 'status': status}
        except Refused as refusal:
            checks['origin_healthz'] = {'origin': args.origin, 'status': 'unchecked',
                                        'reason': str(refusal)}
    else:
        not_checked.append('existing_https_origin_not_supplied')

    executed = [list(tokens) for tokens in runner.actions]
    unexpected = sorted({tokens[0].rsplit('/', 1)[-1] for tokens in executed} - PREFLIGHT_PROGRAMS)
    require(not unexpected, 'preflight_ran_an_unexpected_program')
    out({'result': 'PREFLIGHT_READ_ONLY',
         'checks': checks,
         'blocking': sorted(set(blocking)),
         'not_checked': sorted(set(not_checked)),
         'commands_executed': executed,
         'mutating_actions_performed': 0,
         'declares': 'nothing_installed_nothing_started_nothing_written_nothing_removed',
         'authorises': 'nothing_awaiting_owner_authorisation_still_required'})


def authorize_steps(args):
    steps = [step.strip() for step in args.steps.split(',') if step.strip()]
    out(engine.authorize(args.plan, args.receipt, steps, args.confirm, args.acceptance))


def run_chain(args):
    steps = [step.strip() for step in args.steps.split(',') if step.strip()] if args.steps else None
    out(engine.run(args.plan, args.receipt, steps, args.acceptance, args.execute, args.reconcile))


def lifecycle_status(args):
    out(engine.status(args.receipt))


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest='command', required=True)
    p = sub.add_parser('verify-image'); p.add_argument('inspect'); p.set_defaults(func=verify_image)
    p = sub.add_parser('verify-mounts'); p.add_argument('config'); p.add_argument('prepared')
    p.set_defaults(func=verify_mounts)
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

    p = sub.add_parser('plan'); p.add_argument('request'); p.add_argument('--receipt', required=True)
    p.add_argument('--acceptance', action='store_true'); p.set_defaults(func=freeze_plan)
    p = sub.add_parser('preflight')
    p.add_argument('--root', default='', help='candidate boundary_root to inspect read-only')
    p.add_argument('--source', action='append', default=[], help='candidate source directory (repeatable)')
    p.add_argument('--origin', default='', help='existing https origin to probe /healthz on')
    p.set_defaults(func=preflight)
    p = sub.add_parser('authorize'); p.add_argument('--plan', required=True)
    p.add_argument('--receipt', required=True); p.add_argument('--steps', required=True)
    p.add_argument('--confirm', required=True); p.add_argument('--acceptance', action='store_true')
    p.set_defaults(func=authorize_steps)
    p = sub.add_parser('run'); p.add_argument('--plan', required=True)
    p.add_argument('--receipt', required=True); p.add_argument('--steps')
    p.add_argument('--execute', action='store_true'); p.add_argument('--acceptance', action='store_true')
    p.add_argument('--reconcile'); p.set_defaults(func=run_chain)
    p = sub.add_parser('status'); p.add_argument('--receipt', required=True)
    p.set_defaults(func=lifecycle_status)

    args = parser.parse_args()
    try:
        args.func(args)
    except Refused as refusal:
        print(json.dumps({'result': 'REFUSED', 'reason': str(refusal)}), file=sys.stderr)
        sys.exit(1)
    except (OSError, ValueError, KeyError, PermissionError):
        print(json.dumps({'result': 'REFUSED', 'reason': 'lifecycle_refused_inspect_receipt'}),
              file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
