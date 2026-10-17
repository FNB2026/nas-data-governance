#!/usr/bin/env python3
"""Executable engine for the fnOS install-chain lifecycle.

The companion CLI (fnos-install-lifecycle.py) keeps the evidence-only
verification matrix; this module owns the part that actually acts. Three
artifacts must agree before anything mutates the host:

  1. a frozen plan        (structured parameters, canonical SHA-256 digest)
  2. an explicit grant    (per-step names bound to that digest, 0600)
  3. --execute            (a third, separate affirmative action)

Design rules enforced here and asserted by the tests:

  * No shell. Every external action is an argv list handed to
    subprocess.run(..., shell=False); a string command is a programming
    error, not a runtime refusal.
  * Directory identity is proved with O_NOFOLLOW opens, re-stat after
    chmod/chown (umask silently filters mkdir modes), and a mount/identity
    snapshot that every later step re-checks.
  * The journal is append-only and fsynced per record. A `start` without a
    terminal record is a crash footprint: it is never auto-continued unless
    the step declares an idempotent postcheck that actually passes.
  * Refusals carry stable tokens and never echo private paths.
"""
import datetime
import hashlib
import http.cookies
import json
import os
import re
import secrets
import sqlite3
import ssl
import stat
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

PINS = {
    'oci_index': 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1',
    'amd64_config': 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4',
    'image': 'ghcr.io/fnb2026/ndg-readonly-preview@sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1',
}
BIND_TARGETS = {'/data': False, '/config': True, '/sources/source_a': True, '/sources/source_b': True}
HEX64 = re.compile(r'^[0-9a-f]{64}$')
HEX12 = re.compile(r'^[0-9a-f]{12}$')
PROJECT = re.compile(r'^[a-z0-9][a-z0-9_-]{0,62}$')
TOOL_VERSION = 2
PRODUCTION = 'PRODUCTION'
ACCEPTANCE = 'DISPOSABLE_LINUX_ACCEPTANCE'
SCOPES = {PRODUCTION, ACCEPTANCE}
STEP_ORDER = ['compat', 'sources', 'image', 'init', 'gateway', 'start', 'browser',
              'access', 'gui', 'backup', 'upgrade', 'rollback', 'uninstall', 'reboot']
STEP_PREREQS = {
    'compat': [], 'sources': [], 'image': [],
    'init': ['compat', 'sources'],
    'gateway': ['init'], 'start': ['init'],
    'browser': ['start'], 'access': ['start'], 'gui': ['start'],
    'backup': ['access'],
    'upgrade': ['backup'], 'rollback': ['backup'],
    'uninstall': ['start'], 'reboot': ['access'],
}
MUTATING = {'init', 'gateway', 'start', 'backup', 'upgrade', 'rollback', 'uninstall'}
# Steps whose mutation is either idempotent or fully described by a
# side-effect-free postcheck; only these may be reconciled after a crash.
RECONCILABLE = {'init', 'start', 'backup', 'uninstall', 'upgrade', 'rollback', 'gateway'}
TEMPLATE_REL = 'packaging/fnos/compose.lan-readonly.yaml'
SERVICE_UNIT = 'ndg-readonly-https.service'
UNIT_TEMPLATE_REL = 'packaging/fnos/readonly-https-candidate.service'


class Refused(Exception):
    pass


def require(condition, reason):
    if not condition:
        raise Refused(reason)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True).encode()


def digest_of(value):
    return hashlib.sha256(canonical(value)).hexdigest()


def utcnow():
    return datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')


def load_json(path, label):
    require(isinstance(path, str) and path and path != '-', f'{label}_file_required')
    try:
        with open(path, 'rb') as handle:
            return json.loads(handle.read(1 << 24))
    except FileNotFoundError:
        raise Refused(f'{label}_missing')
    except (ValueError, OSError):
        raise Refused(f'{label}_unreadable')


def out(payload):
    print(json.dumps(payload, sort_keys=True, indent=1))


def repo_root():
    return os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


# ---------------- shared identity records (also used by the CLI) ----------------

def verify_image_record(record):
    require(isinstance(record, dict), 'image_inspect_single_record_required')
    image_id = str(record.get('Id', ''))
    digests = [str(d) for d in record.get('RepoDigests', []) or []]
    require(image_id.startswith('sha256:') and len(image_id) == 71, 'image_id_digest_required')
    require(digests, 'repo_digest_required_without_tag_identity')
    require(any('@sha256:' in digest for digest in digests), 'repo_digest_required_without_tag_identity')
    matched = [d for d in digests if d.endswith('@' + PINS['oci_index'])]
    require(matched, 'image_not_pinned_to_acceptance_index')
    require(str(record.get('Architecture', '')) == 'amd64', 'acceptance_evidence_is_amd64_only')
    return {'image_id': image_id, 'pinned_repo_digest': matched[0], 'architecture': 'amd64',
            'oci_index': PINS['oci_index'], 'amd64_config': PINS['amd64_config']}


def verify_mounts_payload(compose, prepared):
    require(isinstance(compose, dict), 'compose_config_object_required')
    services = compose.get('services')
    require(isinstance(services, dict) and set(services) == {'core'}, 'single_core_service_required')
    core = services['core']
    require(not core.get('ports'), 'no_host_ports')
    require(not core.get('build'), 'no_build')
    require(not core.get('environment'), 'no_environment')
    require(core.get('user') == '10001:10001', 'fixed_uid_gid_required')
    require(core.get('read_only') is True, 'read_only_root_required')
    require(core.get('pull_policy') == 'never', 'pull_policy_never_required')
    image = str(core.get('image', ''))
    require(image.endswith('@' + PINS['oci_index']), 'image_must_be_digest_pinned')
    mounts = {volume.get('target'): volume for volume in core.get('volumes', []) or []}
    require(set(mounts) == set(BIND_TARGETS), 'exact_four_bind_targets_required')
    for target, read_only in BIND_TARGETS.items():
        volume = mounts[target]
        require(volume.get('type') == 'bind', 'bind_type_required')
        require(volume.get('bind', {}).get('create_host_path', False) is False,
                'create_host_path_false_required')
        require(volume.get('read_only', False) is read_only, 'read_only_flag_mismatch')
    networks = compose.get('networks', {})
    require('core' in networks and networks['core'].get('external') is True, 'external_network_required')
    network_name = str(networks['core'].get('name', ''))
    require(bool(HEX64.match(network_name)), 'full_network_id_required')
    require(prepared.get('network_verified') is False, 'prepared_receipt_must_not_preverify')
    require(prepared.get('image_verified') is False and prepared.get('gateway_installed') is False,
            'prepared_receipt_must_not_preverify')
    return {'binds': sorted(BIND_TARGETS), 'network_id': network_name,
            'image': image.split('@')[-1][:16] + '…'}


def container_identity(record):
    require(isinstance(record, dict), 'container_inspect_single_record_required')
    state = record.get('State', {}) or {}
    require(record.get('Id', ''), 'container_id_required')
    image = str(record.get('Image', ''))
    require(image.startswith('sha256:'), 'container_image_digest_required')
    return record, {'id': record['Id'], 'name': '/' + str(record.get('Name', '')).lstrip('/'),
                    'image': image, 'running': bool(state.get('Running'))}


def verify_backup_payload(evidence):
    files = evidence.get('files')
    require(isinstance(files, list) and files, 'backup_file_list_required')
    seen = set()
    db_seen = False
    for entry in files:
        path = str(entry.get('path', ''))
        require(path and path not in seen, 'duplicate_or_empty_backup_path')
        seen.add(path)
        digest = str(entry.get('sha256', ''))
        require(bool(HEX64.match(digest)), 'sha256_required_for_every_file')
        for field in ('size', 'uid', 'gid'):
            require(isinstance(entry.get(field), int), f'{field}_required')
        mode = entry.get('mode')
        require(isinstance(mode, str) and mode.startswith('0') and
                all(ch in '01234567' for ch in mode), 'octal_mode_required')
        if path.endswith('.db'):
            db_seen = True
            require(entry.get('quick_check') == 'ok', 'sqlite_quick_check_ok_required')
    require(db_seen, 'state_database_must_be_in_backup')
    return {'files': len(files)}


# ---------------- strict path identity ----------------

def strict_path(value, label):
    require(isinstance(value, str) and 1 <= len(value) <= 4096, f'{label}_path_required')
    require('\x00' not in value and '\n' not in value and '\r' not in value,
            f'{label}_control_character_rejected')
    require(value.startswith('/'), f'{label}_absolute_required')
    parts = value.split('/')[1:]
    require(value == '/' or all(part not in ('', '.', '..') for part in parts),
            f'{label}_normalized_required')
    if value != '/':
        require(parts[-1] != '', f'{label}_normalized_required')
    current = '/'
    for part in parts:
        current = os.path.join(current, part)
        try:
            info = os.lstat(current)
        except FileNotFoundError:
            break
        require(not stat.S_ISLNK(info.st_mode), f'{label}_symlink_component_rejected')
    return value


def _no_acl(path):
    try:
        names = os.listxattr(path, follow_symlinks=False)
    except (AttributeError, OSError):
        return False
    return not any(name in ('system.posix_acl_access', 'system.posix_acl_default') for name in names)


def trusted_ancestors(path, label):
    """Root-owned, not group/other writable, ACL-free. Linux root only."""
    if sys.platform != 'linux' or os.geteuid() != 0:
        return 'ownership_not_verified_non_root'
    current = path
    while True:
        info = os.lstat(current)
        require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and info.st_mode & 0o022 == 0,
                f'{label}_trusted_ancestor_required')
        require(_no_acl(current), f'{label}_acl_rejected')
        if current == '/':
            break
        current = os.path.dirname(current)
    return 'ownership_verified_root'


def kernel_confined(boundary, path, label):
    """openat2(BENEATH|NO_SYMLINKS|NO_XDEV) proves containment and no nested mount."""
    if sys.platform != 'linux':
        return 'kernel_confinement_not_applicable'
    import ctypes
    fd_boundary = os.open(boundary, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        relative = os.path.relpath(path, boundary)
        require(relative != '..' and not relative.startswith('../'), f'{label}_outside_boundary_rejected')

        class How(ctypes.Structure):
            _fields_ = [('flags', ctypes.c_uint64), ('mode', ctypes.c_uint64),
                        ('resolve', ctypes.c_uint64)]

        how = How(os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW, 0, 0x08 | 0x04 | 0x01)
        fd = ctypes.CDLL(None, use_errno=True).syscall(437, fd_boundary, relative.encode(),
                                                       ctypes.byref(how), ctypes.sizeof(how))
        require(fd >= 0, f'{label}_nested_mount_or_alias_rejected')
        os.close(fd)
    finally:
        os.close(fd_boundary)
    return 'kernel_confinement_verified'


def assert_mounts_stable(ctx):
    """Every later mutation re-proves the recorded device/inode did not move."""
    require(os.path.exists(ctx.receipt_path('sources.json')), 'source_identity_snapshot_missing')
    recorded = ctx.recorded('sources.json')
    for label in ('boundary_root', 'parent', 'source_a', 'source_b'):
        try:
            info = os.lstat(ctx.plan[label])
        except OSError:
            raise Refused('source_identity_unavailable')
        expected = recorded['identities'][label]
        require((info.st_dev, info.st_ino) == (expected['device'], expected['inode']),
                'mount_identity_changed')
    return 'mount_identity_stable'


def private_dir(path, label):
    strict_path(path, label)
    try:
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    except FileNotFoundError:
        raise Refused(f'{label}_missing')
    except (NotADirectoryError, OSError):
        raise Refused(f'{label}_directory_required')
    try:
        info = os.fstat(fd)
        require(stat.S_ISDIR(info.st_mode), f'{label}_directory_required')
        require(info.st_uid == os.geteuid(), f'{label}_owned_by_operator_required')
        require(stat.S_IMODE(info.st_mode) & 0o077 == 0, f'{label}_private_mode_required')
        return {'device': info.st_dev, 'inode': info.st_ino}
    finally:
        os.close(fd)


def sync_dir(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def write_new(directory, name, data, mode=0o600, uid=None, gid=None):
    target = os.path.join(directory, name)
    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        with os.fdopen(fd, 'wb', closefd=False) as handle:
            handle.write(data)
            handle.flush()
        # mkdir/open modes are filtered by umask; chmod is not. Assert after.
        os.fchmod(fd, mode)
        if uid is not None:
            os.fchown(fd, uid, gid if gid is not None else uid)
        os.fsync(fd)
    finally:
        os.close(fd)
    info = os.lstat(target)
    require(stat.S_IMODE(info.st_mode) == mode, 'file_mode_not_applied')
    if uid is not None:
        require((info.st_uid, info.st_gid) == (uid, gid if gid is not None else uid), 'file_owner_not_applied')
    sync_dir(directory)


def ensure_dir(path, mode, uid=None, gid=None):
    os.mkdir(path, 0o700)
    os.chmod(path, mode)
    if uid is not None:
        os.chown(path, uid, gid if gid is not None else uid)
    info = os.lstat(path)
    require(stat.S_ISDIR(info.st_mode) and not stat.S_ISLNK(info.st_mode), 'directory_required')
    require(stat.S_IMODE(info.st_mode) == mode, 'directory_mode_not_applied')
    if uid is not None:
        require((info.st_uid, info.st_gid) == (uid, gid if gid is not None else uid),
                'directory_owner_not_applied')


def atomic_write(path, data, mode):
    directory = os.path.dirname(path)
    temporary = os.path.join(directory, '.' + os.path.basename(path) + '.new-' + secrets.token_hex(6))
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        with os.fdopen(fd, 'wb', closefd=False) as handle:
            handle.write(data)
            handle.flush()
        os.fchmod(fd, mode)
        os.fsync(fd)
    finally:
        os.close(fd)
    os.rename(temporary, path)
    sync_dir(directory)


def manifest(root, label='manifest'):
    strict_path(root, label)
    require(os.path.isdir(root) and not os.path.islink(root), f'{label}_directory_required')
    entries = []
    for current, directories, files in os.walk(root, followlinks=False):
        directories.sort()
        files.sort()
        for name in directories:
            path = os.path.join(current, name)
            require(not os.path.islink(path), 'symlink_in_tree_rejected')
        for name in files:
            path = os.path.join(current, name)
            info = os.lstat(path)
            require(stat.S_ISREG(info.st_mode), 'non_regular_file_rejected')
            require(info.st_size <= (1 << 31), 'file_too_large_for_manifest')
            digest = hashlib.sha256()
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
            try:
                while True:
                    block = os.read(fd, 1 << 20)
                    if not block:
                        break
                    digest.update(block)
            finally:
                os.close(fd)
            entries.append({'path': os.path.relpath(path, root), 'sha256': digest.hexdigest(),
                            'size': info.st_size, 'mode': format(stat.S_IMODE(info.st_mode), '04o'),
                            'uid': info.st_uid, 'gid': info.st_gid})
            require(len(entries) <= 20000, 'manifest_entry_limit_exceeded')
    require(entries, 'empty_tree_rejected')
    return entries


def copy_tree(source, destination, uid=None, gid=None):
    strict_path(source, 'copy_source')
    require(not os.path.exists(destination), 'copy_destination_must_not_exist')
    ensure_dir(destination, 0o700, uid, gid)
    source_info = os.lstat(source)
    os.chmod(destination, stat.S_IMODE(source_info.st_mode))
    for current, directories, files in os.walk(source, followlinks=False):
        directories.sort()
        files.sort()
        relative = os.path.relpath(current, source)
        target_dir = destination if relative == '.' else os.path.join(destination, relative)
        for name in directories:
            path = os.path.join(current, name)
            info = os.lstat(path)
            require(not stat.S_ISLNK(info.st_mode), 'symlink_in_tree_rejected')
            child = os.path.join(target_dir, name)
            ensure_dir(child, stat.S_IMODE(info.st_mode), uid if uid is not None else info.st_uid,
                       gid if gid is not None else info.st_gid)
        for name in files:
            path = os.path.join(current, name)
            info = os.lstat(path)
            require(stat.S_ISREG(info.st_mode), 'non_regular_file_rejected')
            target = os.path.join(target_dir, name)
            fd_in = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
            try:
                fd_out = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
                try:
                    with os.fdopen(fd_out, 'wb', closefd=False) as handle:
                        while True:
                            block = os.read(fd_in, 1 << 20)
                            if not block:
                                break
                            handle.write(block)
                        handle.flush()
                    os.fchmod(fd_out, stat.S_IMODE(info.st_mode))
                    os.fchown(fd_out, uid if uid is not None else info.st_uid,
                              gid if gid is not None else info.st_gid)
                    os.fsync(fd_out)
                finally:
                    os.close(fd_out)
            finally:
                os.close(fd_in)
            after = os.lstat(target)
            require(stat.S_IMODE(after.st_mode) == stat.S_IMODE(info.st_mode), 'copy_mode_not_applied')
    for current, _, _ in os.walk(destination, topdown=False):
        sync_dir(current)


# ---------------- argv-only runner ----------------

SHELLS = {'sh', 'bash', 'dash', 'zsh', 'ksh', 'fish', 'busybox', 'cmd', 'cmd.exe', 'powershell', 'pwsh'}


def assert_argv(argv):
    require(isinstance(argv, (list, tuple)) and argv, 'argv_list_required')
    tokens = []
    for token in argv:
        require(isinstance(token, str), 'argv_string_token_required')
        require(token, 'argv_empty_token_rejected')
        require(len(token) <= 4096, 'argv_token_too_long')
        require('\x00' not in token and '\n' not in token and '\r' not in token,
                'argv_control_character_rejected')
        tokens.append(token)
    head = os.path.basename(tokens[0])
    require(head not in SHELLS, 'shell_execution_rejected')
    require('eval' not in tokens and 'exec' not in tokens, 'shell_builtin_rejected')
    return tokens


class Runner(object):
    """Runs argv lists only. Recording every action keeps the receipt auditable."""

    def __init__(self, record_path=None, env=None):
        self.record_path = record_path
        self.env = env or {'PATH': '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin',
                           'LC_ALL': 'C', 'HOME': '/'}
        self.actions = []

    def run(self, argv, timeout=300):
        tokens = assert_argv(argv)
        self.actions.append(tokens)
        if self.record_path:
            with open(self.record_path, 'ab') as handle:
                handle.write(canonical({'at': utcnow(), 'argv': tokens}) + b'\n')
                handle.flush()
                os.fsync(handle.fileno())
        try:
            completed = subprocess.run(tokens, stdin=subprocess.DEVNULL, capture_output=True,
                                       timeout=timeout, shell=False, env=self.env, cwd='/')
        except FileNotFoundError:
            raise Refused('command_unavailable')
        except subprocess.TimeoutExpired:
            raise Refused('command_timed_out')
        self.actions[-1] = tokens
        return completed.returncode, completed.stdout, completed.stderr

    def must(self, argv, timeout=300, reason='command_failed'):
        code, stdout, stderr = self.run(argv, timeout=timeout)
        require(code == 0, reason)
        return stdout

    def json(self, argv, timeout=60):
        raw = self.must(argv, timeout=timeout, reason='inspect_failed')
        try:
            return json.loads(raw)
        except ValueError:
            raise Refused('inspect_output_unreadable')


# ---------------- HTTP (TLS verified, no redirects) ----------------

class HTTP(object):
    def __init__(self, base, ca_file=None):
        require(isinstance(base, str) and base.startswith(('http://', 'https://')), 'access_base_required')
        self.base = base.rstrip('/')
        require(self.base == base, 'access_base_trailing_slash_rejected')
        self.context = None
        if self.base.startswith('https://'):
            self.context = ssl.create_default_context(cafile=ca_file)
            self.context.check_hostname = True
            self.context.verify_mode = ssl.CERT_REQUIRED
        else:
            require(ca_file is None, 'ca_file_requires_https')

    def request(self, method, path, payload=None, headers=None, cookie=None, timeout=30):
        require(path.startswith('/'), 'request_path_required')
        url = self.base + path
        body = None
        merged = {'Accept': 'application/json'}
        if headers:
            merged.update(headers)
        if payload is not None:
            body = canonical(payload)
            merged['Content-Type'] = 'application/json'
        if cookie:
            merged['Cookie'] = cookie
        merged['Origin'] = self.base
        request = urllib.request.Request(url, data=body, method=method, headers=merged)
        try:
            with urllib.request.urlopen(request, timeout=timeout, context=self.context) as response:
                raw = response.read(1 << 20)
                return response.status, dict(response.headers.items()), raw
        except urllib.error.HTTPError as error:
            raw = error.read(1 << 20)
            return error.code, dict(error.headers.items()), raw
        except (urllib.error.URLError, ssl.SSLError, OSError):
            raise Refused('access_unreachable')


def http_json(http, method, path, payload=None, cookie=None, csrf=None, accept=(200,)):
    headers = {'X-CSRF-Token': csrf} if csrf else None
    status, _, raw = http.request(method, path, payload=payload, headers=headers, cookie=cookie)
    require(status in accept, f'access_status_{status}')
    try:
        return json.loads(raw)
    except ValueError:
        raise Refused('access_response_unreadable')


# ---------------- plan ----------------

PLAN_FIELDS = {'schema', 'scope', 'installation_id', 'root', 'parent', 'boundary_root', 'source_a',
               'source_b', 'origin', 'network_id', 'core_ipv4', 'project', 'backup_dir',
               'candidate_image', 'expected_scanned', 'gateway', 'tls_ca', 'network_owned', 'request_sha256'}


def _validate_origin(origin, scope, core_ipv4):
    require(isinstance(origin, str) and origin, 'origin_required')
    require('\n' not in origin and '\r' not in origin and '\x00' not in origin, 'origin_control_character_rejected')
    match = re.fullmatch(r'(https?)://([a-z0-9.-]+):(\d{1,5})', origin)
    require(match, 'origin_shape_rejected')
    scheme, host, port = match.group(1), match.group(2), int(match.group(3))
    require(1024 <= port <= 65535, 'origin_port_rejected')
    if scope == PRODUCTION:
        require(scheme == 'https', 'https_origin_required')
        require(not re.fullmatch(r'\d+\.\d+\.\d+\.\d+', host), 'dns_origin_required')
        require('.' in host and all(re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label)
                                    for label in host.split('.')), 'dns_origin_required')
    else:
        require(scheme == 'http' and host == core_ipv4 and port == 8080,
                'acceptance_origin_must_be_core_plaintext')
    return origin


def build_plan(request, acceptance):
    require(isinstance(request, dict), 'request_object_required')
    unknown = set(request) - (PLAN_FIELDS - {'schema', 'project', 'request_sha256', 'root'})
    require(not unknown, 'request_fields_rejected')
    scope = request.get('scope')
    require(scope in SCOPES, 'scope_required')
    require(scope != ACCEPTANCE or acceptance, 'acceptance_scope_requires_explicit_flag')
    for field in ('installation_id', 'parent', 'boundary_root', 'source_a', 'source_b', 'origin',
                  'network_id', 'core_ipv4', 'backup_dir'):
        require(isinstance(request.get(field), str), 'request_strings_required')
    require(bool(HEX12.match(request['installation_id'])), 'installation_id_shape_rejected')
    require(bool(HEX64.match(request['network_id'])), 'full_network_identity_required')
    require(request['network_id'] != '0' * 64, 'full_network_identity_required')
    boundary = strict_path(request['boundary_root'], 'boundary_root')
    parent = strict_path(request['parent'], 'parent')
    source_a = strict_path(request['source_a'], 'source_a')
    source_b = strict_path(request['source_b'], 'source_b')
    backup_dir = strict_path(request['backup_dir'], 'backup_dir')
    require(os.path.isdir(boundary), 'boundary_root_directory_required')
    require(os.path.isdir(parent), 'parent_directory_required')
    for label, path in (('source_a', source_a), ('source_b', source_b)):
        require(os.path.isdir(path), f'{label}_directory_required')
    require(os.path.commonpath([boundary, parent]) == boundary, 'parent_outside_boundary_rejected')
    for label, path in (('source_a', source_a), ('source_b', source_b)):
        require(os.path.commonpath([boundary, path]) == boundary, f'{label}_outside_boundary_rejected')
        require(os.path.commonpath([parent, path]) != path, 'state_inside_source_rejected')
    require(source_a != source_b, 'overlapping_sources_rejected')
    require(os.path.commonpath([source_a, source_b]) not in (source_a, source_b),
            'overlapping_sources_rejected')
    require(backup_dir != parent and os.path.commonpath([backup_dir, source_a]) != backup_dir and
            os.path.commonpath([backup_dir, source_b]) != backup_dir, 'backup_dir_rejected')
    require(os.path.commonpath([boundary, backup_dir]) == boundary, 'backup_dir_outside_boundary_rejected')
    trusted_ancestors(boundary, 'boundary_root')
    trusted_ancestors(parent, 'parent')
    for label, path in (('source_a', source_a), ('source_b', source_b)):
        trusted_ancestors(path, label)
    address = request['core_ipv4']
    require(bool(re.fullmatch(r'\d{1,3}(?:\.\d{1,3}){3}', address)), 'core_ipv4_shape_rejected')
    octets = [int(part) for part in address.split('.')]
    require(all(0 <= value <= 255 for value in octets), 'core_ipv4_shape_rejected')
    require(octets[0] == 10 and not (octets[1] == 0 and octets[2] == 0 and octets[3] == 0),
            'private_core_ipv4_required')
    _validate_origin(request['origin'], scope, address)
    project = request.get('project') or 'ndg-' + request['installation_id']
    require(bool(PROJECT.match(project)), 'project_name_rejected')
    candidate = request.get('candidate_image')
    if candidate is not None:
        require(isinstance(candidate, str) and re.fullmatch(r'sha256:[0-9a-f]{64}', candidate),
                'candidate_image_digest_required')
        require(candidate != PINS['oci_index'], 'candidate_must_be_a_different_digest_pinned_image')
    expected = request.get('expected_scanned', 1)
    require(isinstance(expected, int) and 1 <= expected <= 1000000, 'expected_scanned_rejected')
    gateway = request.get('gateway')
    if scope == PRODUCTION:
        require(isinstance(gateway, dict) and set(gateway) == {'unit_sha256', 'binary_sha256', 'cert', 'key'},
                'gateway_parameters_required')
        require(bool(HEX64.match(str(gateway.get('unit_sha256', '')))) and
                bool(HEX64.match(str(gateway.get('binary_sha256', '')))), 'gateway_digest_required')
        for field in ('cert', 'key'):
            strict_path(gateway[field], 'gateway_' + field)
    else:
        require(gateway is None, 'gateway_not_in_acceptance_scope')
    tls_ca = request.get('tls_ca')
    if tls_ca is not None:
        strict_path(tls_ca, 'tls_ca')
    network_owned = request.get('network_owned', False)
    require(isinstance(network_owned, bool), 'network_owned_boolean_required')
    root = os.path.join(parent, 'ndg-readonly-' + request['installation_id'])
    strict_path(root, 'root')
    template = os.path.join(repo_root(), TEMPLATE_REL)
    require(os.path.isfile(template), 'compose_template_missing')
    template_bytes = open(template, 'rb').read()
    plan = {
        'schema': 1, 'scope': scope, 'tool_version': TOOL_VERSION,
        'installation_id': request['installation_id'],
        'root': root, 'parent': parent, 'boundary_root': boundary,
        'source_a': source_a, 'source_b': source_b,
        'origin': request['origin'], 'access_base': request['origin'],
        'network_id': request['network_id'], 'core_ipv4': address,
        'project': project, 'backup_dir': backup_dir,
        'network_owned': network_owned,
        'expected_scanned': expected,
        'template_sha256': hashlib.sha256(template_bytes).hexdigest(),
        'pins': dict(PINS),
        'created_at': utcnow(),
    }
    if candidate:
        plan['candidate_image'] = candidate
    if tls_ca:
        plan['tls_ca'] = tls_ca
    if gateway:
        plan['gateway'] = gateway
    plan['request_sha256'] = hashlib.sha256(canonical(request)).hexdigest()
    plan['plan_digest'] = digest_of({key: value for key, value in plan.items() if key != 'plan_digest'})
    return plan


def plan_digest(plan):
    return digest_of({key: value for key, value in plan.items() if key != 'plan_digest'})


def load_plan(path, acceptance):
    plan = load_json(path, 'plan')
    require(isinstance(plan, dict), 'plan_object_required')
    require(plan.get('schema') == 1 and plan.get('tool_version') == TOOL_VERSION, 'plan_version_rejected')
    require(plan.get('plan_digest') == plan_digest(plan), 'plan_digest_mismatch')
    require(plan.get('scope') in SCOPES, 'plan_scope_rejected')
    require(plan.get('scope') != ACCEPTANCE or acceptance, 'acceptance_scope_requires_explicit_flag')
    for field in ('root', 'parent', 'boundary_root', 'source_a', 'source_b', 'backup_dir'):
        strict_path(plan.get(field, ''), field)
    return plan


# ---------------- authorization ----------------

def authorization_path(receipt):
    return os.path.join(receipt, 'authorization.json')


def load_authorization(receipt):
    path = authorization_path(receipt)
    try:
        info = os.lstat(path)
    except FileNotFoundError:
        raise Refused('authorization_missing')
    require(stat.S_ISREG(info.st_mode) and not stat.S_ISLNK(info.st_mode), 'authorization_regular_file_required')
    require(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) & 0o077 == 0,
            'authorization_private_required')
    authorization = load_json(path, 'authorization')
    require(isinstance(authorization, dict), 'authorization_object_required')
    require(set(authorization) == {'schema', 'plan_digest', 'steps', 'confirmed_at', 'tool_version'},
            'authorization_fields_rejected')
    require(authorization['schema'] == 1 and authorization['tool_version'] == TOOL_VERSION,
            'authorization_version_rejected')
    require(bool(re.fullmatch(r'[0-9a-f]{64}', str(authorization['plan_digest']))), 'authorization_digest_rejected')
    require(isinstance(authorization['steps'], list) and authorization['steps'], 'authorization_steps_required')
    require(all(step in STEP_ORDER for step in authorization['steps']), 'authorization_step_unknown')
    require(authorization['steps'] == [s for s in STEP_ORDER if s in set(authorization['steps'])],
            'authorization_step_order_rejected')
    return authorization


def authorize(plan_path, receipt, steps, confirm, acceptance):
    plan = load_plan(plan_path, acceptance)
    private_dir(receipt, 'receipt')
    require(steps, 'authorization_steps_required')
    if steps == ['all']:
        expanded = list(STEP_ORDER)
    else:
        unknown = [step for step in steps if step not in STEP_ORDER]
        require(not unknown, 'authorization_step_unknown')
        expanded = [step for step in STEP_ORDER if step in set(steps)]
    require(confirm == plan['plan_digest'][:12], 'confirmation_digest_mismatch')
    if os.path.exists(authorization_path(receipt)):
        raise Refused('authorization_already_exists')
    payload = {'schema': 1, 'plan_digest': plan['plan_digest'], 'steps': expanded,
               'confirmed_at': utcnow(), 'tool_version': TOOL_VERSION}
    write_new(receipt, 'authorization.json', (json.dumps(payload, indent=1) + '\n').encode(), 0o600)
    return {'result': 'AUTHORIZED', 'installation_id': plan['installation_id'],
            'plan_digest_prefix': plan['plan_digest'][:12], 'steps': expanded}


# ---------------- journal ----------------

class Journal(object):
    def __init__(self, receipt):
        self.path = os.path.join(receipt, 'journal.jsonl')
        self.receipt = receipt

    def append(self, step, phase, detail=None):
        require(step in STEP_ORDER, 'journal_unknown_step')
        require(phase in {'start', 'ok', 'skip', 'reconciled_ok', 'failed', 'refused'},
                'journal_unknown_phase')
        record = {'seq': len(self.entries()), 'at': utcnow(), 'step': step, 'phase': phase}
        if detail:
            record['detail'] = detail
        existed = os.path.exists(self.path)
        fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
        try:
            os.write(fd, canonical(record) + b'\n')
            os.fsync(fd)
        finally:
            os.close(fd)
        if not existed:
            sync_dir(self.receipt)

    def entries(self):
        if not os.path.exists(self.path):
            return []
        entries = []
        with open(self.path, 'rb') as handle:
            for line in handle:
                line = line.strip()
                if not line:
                    continue
                try:
                    entries.append(json.loads(line))
                except ValueError:
                    raise Refused('journal_unreadable')
        return entries

    def state(self):
        state = {}
        for entry in self.entries():
            step, phase = entry.get('step'), entry.get('phase')
            require(step in STEP_ORDER, 'journal_unknown_step')
            if phase == 'start':
                state[step] = 'start'
            elif phase in ('ok', 'skip', 'reconciled_ok'):
                state[step] = 'ok'
            elif phase in ('failed', 'refused'):
                state[step] = 'failed'
            else:
                raise Refused('journal_unknown_phase')
        return state


# ---------------- context and step helpers ----------------

class Context(object):
    def __init__(self, plan, receipt, runner):
        self.plan = plan
        self.receipt = receipt
        self.runner = runner
        self.journal = Journal(receipt)
        self.http = HTTP(plan['access_base'], plan.get('tls_ca'))

    # -- paths --
    def path(self, name):
        return os.path.join(self.plan['root'], name)

    def receipt_path(self, name):
        return os.path.join(self.receipt, name)

    # -- records --
    def record(self, name, payload):
        atomic_write(self.receipt_path(name), (json.dumps(payload, indent=1, sort_keys=True) + '\n').encode(), 0o600)

    def recorded(self, name):
        return load_json(self.receipt_path(name), name.replace('.json', ''))

    # -- docker --
    def compose(self, *arguments, timeout=300):
        require(os.path.isfile(self.path('compose.yaml')), 'installation_compose_missing')
        require(os.path.isfile(self.path('installation.env')), 'installation_env_missing')
        argv = ['docker', 'compose', '--project-name', self.plan['project'], '--ansi', 'never',
                '--env-file', self.path('installation.env'), '--file', self.path('compose.yaml'),
                *arguments]
        return self.runner.must(argv, timeout=timeout, reason='compose_command_failed')

    def resolved_config(self):
        raw = self.compose('config', '--format', 'json')
        try:
            return json.loads(raw)
        except ValueError:
            raise Refused('compose_config_unreadable')

    def container_id(self):
        raw = self.compose('ps', '-a', '-q', 'core')
        identifier = raw.decode().strip()
        if not identifier:
            raise Refused('core_container_missing')
        require(bool(re.fullmatch(r'[0-9a-f]{12,64}', identifier)), 'container_id_shape_rejected')
        return identifier

    def inspect_container(self, identifier=None):
        identifier = identifier or self.container_id()
        records = self.runner.json(['docker', 'container', 'inspect', identifier])
        require(isinstance(records, list) and len(records) == 1, 'container_inspect_single_record_required')
        return container_identity(records[0])[0]

    def network(self):
        records = self.runner.json(['docker', 'network', 'inspect', self.plan['network_id']])
        require(isinstance(records, list) and len(records) == 1, 'network_inspect_single_record_required')
        return records[0]

    # -- health / access --
    def health(self, timeout=120):
        deadline = time.time() + timeout
        last = None
        while time.time() < deadline:
            try:
                status, _, _ = self.http.request('GET', '/healthz', timeout=5)
                if status == 200:
                    return True
            except Refused as refusal:
                last = refusal
            time.sleep(1)
        if last:
            raise last
        raise Refused('health_timeout')

    def login(self):
        credential = self.path('config/credential')
        info = os.lstat(credential)
        require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o600, 'credential_mode_rejected')
        secret = open(credential, 'r', encoding='utf-8').read().strip()
        require(len(secret) >= 32, 'credential_shape_rejected')
        status, headers, raw = self.http.request('POST', '/api/login', payload={'password': secret},
                                                 headers={'Content-Type': 'application/json'})
        require(status == 200, 'login_rejected')
        cookies = http.cookies.SimpleCookie()
        for value in headers.get('Set-Cookie', '').split(','):
            try:
                cookies.load(value)
            except http.cookies.CookieError:
                continue
        require('ndg_session' in cookies, 'session_cookie_missing')
        try:
            body = json.loads(raw)
        except ValueError:
            raise Refused('login_response_unreadable')
        require(isinstance(body.get('csrf'), str) and len(body['csrf']) >= 32, 'csrf_missing')
        return 'ndg_session=' + cookies['ndg_session'].value, body['csrf']

    def snapshot(self, cookie, csrf):
        return http_json(self.http, 'POST', '/api/status', payload={'alias': 'source_a', 'resume': False},
                         cookie=cookie, csrf=csrf)


# ---------------- steps ----------------

def step_compat(ctx):
    require(sys.platform == 'linux', 'linux_administrator_required')
    require(os.uname().machine == 'x86_64', 'acceptance_evidence_is_amd64_only')
    require(os.geteuid() == 0, 'linux_administrator_required')
    release = os.uname().release
    kernel = re.match(r'^(\d+)\.(\d+)', release)
    require(kernel is not None, 'kernel_version_unreadable')
    require((int(kernel.group(1)), int(kernel.group(2))) >= (5, 10), 'kernel_too_old')
    require(sys.version_info >= (3, 9), 'python_too_old')
    server = ctx.runner.must(['docker', 'version', '--format', '{{.Server.Version}}'],
                             timeout=30, reason='docker_unavailable').decode().strip()
    require(bool(re.match(r'^\d+\.\d+', server)), 'docker_version_unreadable')
    compose = ctx.runner.must(['docker', 'compose', 'version', '--short'], timeout=30,
                              reason='docker_compose_required').decode().strip()
    require(bool(re.match(r'^\d+\.\d+', compose)), 'compose_version_unreadable')
    return {'kernel': release, 'docker': server, 'compose': compose}


def step_sources(ctx):
    plan = ctx.plan
    for label in ('boundary_root', 'parent', 'source_a', 'source_b'):
        trusted_ancestors(strict_path(plan[label], label), label)
    confined = {label: kernel_confined(plan['boundary_root'], plan[label], label)
                for label in ('parent', 'source_a', 'source_b')}
    identities = {}
    for label in ('boundary_root', 'parent', 'source_a', 'source_b'):
        info = os.lstat(plan[label])
        require(stat.S_ISDIR(info.st_mode), 'directory_required')
        identities[label] = {'device': info.st_dev, 'inode': info.st_ino,
                             'mode': format(stat.S_IMODE(info.st_mode), '04o'), 'uid': info.st_uid}
    baseline = {label: manifest(plan[label], label) for label in ('source_a', 'source_b')}
    ctx.record('sources.json', {'identities': identities, 'baseline': baseline,
                                'kernel_confinement': confined})
    return {'trusted_ancestors': trusted_ancestors(plan['boundary_root'], 'boundary_root'),
            'kernel_confinement': confined,
            'source_a_files': len(baseline['source_a']), 'source_b_files': len(baseline['source_b'])}


def step_image(ctx):
    records = ctx.runner.json(['docker', 'image', 'inspect', PINS['image']])
    require(isinstance(records, list) and len(records) == 1, 'image_inspect_single_record_required')
    detail = verify_image_record(records[0])
    ctx.record('image.json', detail)
    return detail


def step_init(ctx):
    plan = ctx.plan
    root = plan['root']
    require(not os.path.exists(root), 'installation_root_already_exists')
    require(os.path.isdir(plan['parent']), 'parent_directory_required')
    # Re-prove the sources did not move between planning and mutation.
    assert_mounts_stable(ctx)
    template = os.path.join(repo_root(), TEMPLATE_REL)
    raw_template = open(template, 'rb').read()
    require(hashlib.sha256(raw_template).hexdigest() == plan['template_sha256'], 'compose_template_changed')
    os.mkdir(root, 0o700)
    try:
        os.chmod(root, 0o700)
        ensure_dir(os.path.join(root, 'config'), 0o700, 10001, 10001)
        ensure_dir(os.path.join(root, 'state'), 0o700, 10001, 10001)
        settings = {'listen': '0.0.0.0:8080', 'origin': plan['origin'], 'state': '/data',
                    'credential_file': '/config/credential',
                    'sources': {'source_a': '/sources/source_a', 'source_b': '/sources/source_b'}}
        write_new(os.path.join(root, 'config'), 'credential',
                  (secrets.token_hex(32) + '\n').encode(), 0o600, 10001, 10001)
        write_new(os.path.join(root, 'config'), 'server.json',
                  (json.dumps(settings, indent=2) + '\n').encode(), 0o600, 10001, 10001)
        variables = {'NDG_CORE_CONTAINER': plan['project'] + '-core',
                     'NDG_INSTALLATION_ID': plan['installation_id'],
                     'NDG_CORE_IPV4': plan['core_ipv4'],
                     'NDG_INSTALL_ROOT': root,
                     'NDG_SOURCE_A': plan['source_a'], 'NDG_SOURCE_B': plan['source_b'],
                     'NDG_INTERNAL_NETWORK_ID': plan['network_id']}
        require(all(not any(ch in value for ch in '\r\n\x00$\'"\\') for value in variables.values()),
                'dotenv_value_rejected')
        environment = ''.join(key + "='" + value + "'\n" for key, value in variables.items())
        write_new(root, 'installation.env', environment.encode(), 0o600)
        write_new(root, 'compose.yaml', raw_template, 0o644)
        prepared = {'schema': 1, 'installation_id': plan['installation_id'],
                    'status': 'PREPARED_NOT_STARTED', 'template_sha256': plan['template_sha256'],
                    'network_verified': False, 'image_verified': False, 'gateway_installed': False,
                    'scope': plan['scope']}
        write_new(root, 'prepared.json', (json.dumps(prepared, indent=2) + '\n').encode(), 0o600)
        for path in (os.path.join(root, 'config'), os.path.join(root, 'state'), root, plan['parent']):
            sync_dir(path)
    except BaseException:
        raise Refused('partial_installation_preserved')
    info = os.lstat(root)
    ctx.record('installation.json', {'root_device': info.st_dev, 'root_inode': info.st_ino,
                                     'root_mode': format(stat.S_IMODE(info.st_mode), '04o')})
    return {'status': 'PREPARED_NOT_STARTED', 'config_mode': '0700', 'credential_mode': '0600'}


def step_gateway(ctx):
    plan = ctx.plan
    require(plan['scope'] == PRODUCTION, 'gateway_installation_not_in_acceptance_scope')
    gateway = plan['gateway']
    unit_template = os.path.join(repo_root(), UNIT_TEMPLATE_REL)
    raw_unit = open(unit_template, 'rb').read()
    require(hashlib.sha256(raw_unit).hexdigest() == gateway['unit_sha256'], 'gateway_unit_changed')
    for field, mode in (('cert', 0o644), ('key', 0o600)):
        info = os.lstat(gateway[field])
        require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == mode,
                f'gateway_{field}_mode_rejected')
    binary = '/var/apps/ndg-readonly-web/cmd/ndg-fnos-https'
    info = os.lstat(binary)
    require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) & 0o111 != 0,
            'gateway_binary_not_executable')
    fd = os.open(binary, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        binary_digest = hashlib.sha256()
        while True:
            block = os.read(fd, 1 << 20)
            if not block:
                break
            binary_digest.update(block)
    finally:
        os.close(fd)
    require(binary_digest.hexdigest() == gateway['binary_sha256'], 'gateway_binary_changed')
    unit_dir = '/etc/systemd/system'
    atomic_write(os.path.join(unit_dir, SERVICE_UNIT), raw_unit, 0o644)
    ctx.runner.must(['systemctl', 'daemon-reload'], timeout=60, reason='systemd_reload_failed')
    ctx.runner.must(['systemctl', 'enable', '--now', SERVICE_UNIT], timeout=120, reason='gateway_enable_failed')
    active = ctx.runner.must(['systemctl', 'is-active', SERVICE_UNIT], timeout=30,
                             reason='gateway_inactive').decode().strip()
    require(active == 'active', 'gateway_inactive')
    ctx.health(timeout=60)
    return {'service': SERVICE_UNIT, 'state': active}


def step_start(ctx):
    plan = ctx.plan
    prepared = load_json(ctx.path('prepared.json'), 'prepared_receipt')
    assert_mounts_stable(ctx)
    config = ctx.resolved_config()
    verify_mounts_payload(config, prepared)
    network = ctx.network()
    require(str(network.get('Id', '')) == plan['network_id'], 'network_identity_mismatch')
    require(network.get('Internal') is True, 'internal_network_required')
    require(not (network.get('Containers') or {}), 'network_must_be_empty_before_start')
    subnets = [entry.get('Subnet') for entry in (network.get('IPAM', {}) or {}).get('Config', []) or []]
    require(len(subnets) == 1, 'single_subnet_required')
    import ipaddress
    subnet = ipaddress.ip_network(subnets[0])
    address = ipaddress.ip_address(plan['core_ipv4'])
    require(address in subnet, 'core_ipv4_outside_network')
    require(address not in (subnet.network_address, subnet.broadcast_address), 'core_ipv4_reserved')
    gateway_address = (network.get('IPAM', {}) or {}).get('Config', [{}])[0].get('Gateway')
    require(str(address) != str(gateway_address), 'core_ipv4_must_not_be_network_gateway')
    ctx.compose('up', '-d', '--no-build', '--pull', 'never', '--no-deps', timeout=600)
    ctx.health(timeout=180)
    record = ctx.inspect_container()
    expected_image = ctx.recorded('image.json')['image_id']
    require(record.get('Image', '') == expected_image, 'container_image_mismatch')
    require(str((record.get('Config', {}) or {}).get('User', '')) == '10001:10001', 'container_user_mismatch')
    require((record.get('HostConfig', {}) or {}).get('ReadonlyRootfs') is True, 'container_root_not_readonly')
    require(not (record.get('HostConfig', {}) or {}).get('PortBindings'), 'container_publishes_ports')
    mounts = {mount.get('Destination'): mount for mount in record.get('Mounts', []) or []}
    require(set(mounts) == set(BIND_TARGETS), 'container_bind_set_mismatch')
    for target, read_only in BIND_TARGETS.items():
        require(bool(mounts[target].get('RW')) is not read_only, 'container_bind_mode_mismatch')
    networks = ((record.get('NetworkSettings', {}) or {}).get('Networks') or {})
    require(len(networks) == 1, 'single_container_network_required')
    ctx.record('container.json', {'id': record['Id'], 'image': record.get('Image'),
                                  'network': plan['network_id'], 'ipv4': plan['core_ipv4']})
    return {'container': record['Id'][:12], 'health': 'ok', 'network': plan['network_id'][:12]}


def step_browser(ctx):
    status, headers, raw = ctx.http.request('GET', '/')
    require(status == 200, 'browser_page_status_rejected')
    lowered = {key.lower(): value for key, value in headers.items()}
    require('content-security-policy' in lowered, 'browser_csp_required')
    require(lowered.get('x-content-type-options') == 'nosniff', 'browser_nosniff_required')
    page = raw.decode('utf-8', 'replace')
    require('id="root"' in page and '<!doctype html' in page.lower(), 'gui_app_shell_missing')
    return {'status': 200, 'csp': True, 'app_shell': 'root'}


def step_access(ctx):
    cookie, csrf = ctx.login()
    version = http_json(ctx.http, 'GET', '/api/version', cookie=cookie)
    require(isinstance(version, dict) and version.get('version'), 'version_endpoint_rejected')
    sources = http_json(ctx.http, 'GET', '/api/sources', cookie=cookie)
    require(isinstance(sources, list) and sources, 'sources_endpoint_rejected')
    http_json(ctx.http, 'POST', '/api/scan', payload={'alias': 'source_a', 'resume': False},
              cookie=cookie, csrf=csrf, accept=(200, 202))
    deadline = time.time() + 300
    snapshot = None
    expected = ctx.plan['expected_scanned']
    while time.time() < deadline:
        snapshot = ctx.snapshot(cookie, csrf)
        if snapshot.get('checkpoint') == 'completed' and int(snapshot.get('scanned', 0)) >= expected:
            break
        if snapshot.get('state') in ('failed', 'cancelled'):
            raise Refused('scan_failed')
        time.sleep(2)
    require(isinstance(snapshot, dict), 'snapshot_unavailable')
    require(snapshot.get('checkpoint') == 'completed', 'checkpoint_not_completed')
    require(int(snapshot.get('scanned', 0)) >= expected, 'scan_did_not_cover_sources')
    baseline = ctx.recorded('sources.json')['baseline']
    for label, path in (('source_a', ctx.plan['source_a']), ('source_b', ctx.plan['source_b'])):
        require(manifest(path, label) == baseline[label], 'source_mutated_by_scan')
    http_json(ctx.http, 'POST', '/api/logout', payload={}, cookie=cookie, csrf=csrf)
    ctx.record('access.json', {'snapshot': snapshot, 'version': version.get('version')})
    return {'scanned': snapshot.get('scanned'), 'checkpoint': snapshot.get('checkpoint'),
            'state': snapshot.get('state')}


def step_gui(ctx):
    cookie, csrf = ctx.login()
    capabilities = http_json(ctx.http, 'GET', '/api/capabilities', cookie=cookie)
    require(isinstance(capabilities, dict), 'capabilities_unreadable')
    require(capabilities.get('scan') is True, 'scan_capability_must_be_enabled')
    for capability in ('quarantine', 'restore', 'purge'):
        require(capabilities.get(capability) is False,
                'destructive_capability_must_stay_disabled_until_release_approval')
    status, _, raw = ctx.http.request('GET', '/', cookie=cookie)
    require(status == 200, 'gui_page_status_rejected')
    page = raw.decode('utf-8', 'replace')
    for capability in ('quarantine', 'restore', 'purge'):
        enabled = 'data-ndg-capability="' + capability + '" data-ndg-enabled="true"'
        disabled = 'data-ndg-capability="' + capability + '" data-ndg-enabled="false"'
        require(enabled not in page, 'fake_enabled_control_detected')
        if 'data-ndg-capability="' + capability + '"' in page:
            require(disabled in page, 'disabled_capability_must_say_so')
    http_json(ctx.http, 'POST', '/api/logout', payload={}, cookie=cookie, csrf=csrf)
    return {'capabilities': {key: capabilities.get(key) for key in
                             ('scan', 'resume', 'quarantine', 'restore', 'purge', 'plan_execution')}}


def _quick_check(path):
    """Check a cold-copied database with its WAL, on a throwaway copy."""
    directory = tempfile.mkdtemp(prefix='quick-check-', dir=os.path.dirname(path))
    base = os.path.basename(path)
    copied = 0
    try:
        for suffix in ('', '-wal', '-shm'):
            source = path + suffix
            if not os.path.exists(source):
                continue
            target = os.path.join(directory, base + suffix)
            fd_in = os.open(source, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
            try:
                fd_out = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                try:
                    with os.fdopen(fd_out, 'wb', closefd=False) as handle:
                        while True:
                            block = os.read(fd_in, 1 << 20)
                            if not block:
                                break
                            handle.write(block)
                        handle.flush()
                    os.fsync(fd_out)
                finally:
                    os.close(fd_out)
            finally:
                os.close(fd_in)
            copied += 1
        require(copied > 0, 'sqlite_database_missing')
        try:
            connection = sqlite3.connect(os.path.join(directory, base))
        except sqlite3.Error:
            raise Refused('sqlite_open_rejected')
        try:
            rows = connection.execute('PRAGMA quick_check;').fetchall()
        except sqlite3.Error:
            raise Refused('sqlite_quick_check_failed')
        finally:
            connection.close()
    finally:
        for name in os.listdir(directory):
            os.remove(os.path.join(directory, name))
        os.rmdir(directory)
    require(rows and all(row[0] == 'ok' for row in rows), 'sqlite_quick_check_ok_required')


def step_backup(ctx):
    plan = ctx.plan
    assert_mounts_stable(ctx)
    record = ctx.inspect_container()
    require((record.get('State', {}) or {}).get('Running') is True, 'core_must_be_running_before_cold_backup')
    ctx.runner.must(['docker', 'container', 'stop', record['Id']], timeout=180, reason='container_stop_failed')
    stopped = ctx.inspect_container(record['Id'])
    require((stopped.get('State', {}) or {}).get('Running') is False, 'container_still_running')
    require(not os.path.exists(plan['backup_dir']), 'backup_directory_must_be_new')
    ensure_dir(plan['backup_dir'], 0o700, os.geteuid(), os.getegid())
    target = os.path.join(plan['backup_dir'], 'state')
    copy_tree(ctx.path('state'), target, 10001, 10001)
    files = manifest(target, 'backup')
    for entry in files:
        if entry['path'].endswith('.db'):
            _quick_check(os.path.join(target, entry['path']))
            entry['quick_check'] = 'ok'
    evidence = {'schema': 1, 'scope': plan['scope'], 'source_state_dir': ctx.path('state'),
                'backup_dir': plan['backup_dir'], 'files': files,
                'container': record['Id'], 'created_at': utcnow()}
    verify_backup_payload(evidence)
    atomic_write(ctx.receipt_path('backup-receipt.json'),
                 (json.dumps(evidence, indent=1, sort_keys=True) + '\n').encode(), 0o600)
    atomic_write(ctx.receipt_path('compose-before-upgrade.yaml'),
                 open(ctx.path('compose.yaml'), 'rb').read(), 0o600)
    return {'files': len(files), 'backup': 'verified', 'container_stopped': record['Id'][:12]}


def step_upgrade(ctx):
    plan = ctx.plan
    candidate = plan.get('candidate_image')
    require(candidate, 'candidate_image_required')
    records = ctx.runner.json(['docker', 'image', 'inspect', candidate])
    require(isinstance(records, list) and len(records) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(records[0].get('Id', ''))
    require(candidate_id.startswith('sha256:') and
            candidate_id != ctx.recorded('image.json')['image_id'],
            'candidate_must_be_a_different_digest_pinned_image')
    current = open(ctx.path('compose.yaml'), 'rb').read().decode()
    replaced, count = re.subn(r'(?m)^(\s*image:\s*)\S+$',
                              lambda match: match.group(1) + 'ghcr.io/fnb2026/ndg-readonly-preview@' + candidate,
                              current)
    require(count == 1, 'compose_image_binding_not_unique')
    atomic_write(ctx.path('compose.yaml'), replaced.encode(), 0o644)
    ctx.compose('up', '-d', '--no-build', '--pull', 'never', '--no-deps', timeout=600)
    ctx.health(timeout=180)
    upgraded = ctx.inspect_container()
    require(upgraded.get('Image') == candidate_id, 'upgrade_image_mismatch')
    return {'candidate': candidate[:19] + '…', 'container': upgraded['Id'][:12]}


def step_rollback(ctx):
    plan = ctx.plan
    receipt = load_json(ctx.receipt_path('backup-receipt.json'), 'backup_receipt')
    verify_backup_payload(receipt)
    assert_mounts_stable(ctx)
    record = ctx.inspect_container()
    if (record.get('State', {}) or {}).get('Running'):
        ctx.runner.must(['docker', 'container', 'stop', record['Id']], timeout=180,
                        reason='container_stop_failed')
    original = open(ctx.receipt_path('compose-before-upgrade.yaml'), 'rb').read()
    failed = ctx.path('state') + '.failed-' + utcnow().replace(':', '').replace('-', '')
    os.rename(ctx.path('state'), failed)
    copy_tree(os.path.join(receipt['backup_dir'], 'state'), ctx.path('state'), 10001, 10001)
    restored = manifest(ctx.path('state'), 'restored')
    expected = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in receipt['files']]
    actual = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in restored]
    require(actual == expected, 'restored_state_differs_from_backup')
    atomic_write(ctx.path('compose.yaml'), original, 0o644)
    ctx.compose('up', '-d', '--no-build', '--pull', 'never', '--no-deps', timeout=600)
    ctx.health(timeout=180)
    record = ctx.inspect_container()
    require(record.get('Image') == plan['pins']['amd64_config'], 'rollback_image_mismatch')
    return {'state_preserved_at': failed.replace(plan['root'] + '/', ''),
            'container': record['Id'][:12]}


def step_uninstall(ctx):
    plan = ctx.plan
    baseline = ctx.recorded('sources.json')['baseline']
    record = ctx.inspect_container()
    identifier = record['Id']
    ctx.runner.must(['docker', 'container', 'stop', identifier], timeout=180, reason='container_stop_failed')
    ctx.runner.must(['docker', 'container', 'rm', identifier], timeout=120, reason='container_rm_failed')
    remaining = ctx.runner.must(['docker', 'container', 'ls', '-a', '--filter',
                                 'label=com.ndg.test.nonce=' + plan['installation_id'],
                                 '--format', '{{.ID}}'], timeout=60,
                                reason='container_list_failed').decode().strip()
    require(not remaining, 'core_container_still_present')
    if plan['network_owned']:
        network = ctx.network()
        require(not (network.get('Containers') or {}), 'network_still_has_endpoints')
        ctx.runner.must(['docker', 'network', 'rm', plan['network_id']], timeout=120,
                        reason='network_rm_failed')
    for label, path in (('source_a', plan['source_a']), ('source_b', plan['source_b'])):
        require(manifest(path, label) == baseline[label], 'uninstall_mutated_sources')
    receipt = load_json(ctx.receipt_path('backup-receipt.json'), 'backup_receipt')
    backup = manifest(os.path.join(receipt['backup_dir'], 'state'), 'backup')
    require([{'path': e['path'], 'sha256': e['sha256']} for e in backup] ==
            [{'path': e['path'], 'sha256': e['sha256']} for e in receipt['files']],
            'uninstall_mutated_backup')
    require(os.path.isdir(ctx.path('state')), 'uninstall_must_keep_state')
    return {'container_removed': identifier[:12], 'network_removed': bool(plan['network_owned']),
            'sources_unchanged': True, 'backup_unchanged': True}


def step_reboot(ctx):
    record = ctx.inspect_container()
    policy = ((record.get('HostConfig', {}) or {}).get('RestartPolicy') or {}).get('Name')
    require(policy == 'unless-stopped', 'restart_policy_must_survive_reboot')
    before = ctx.recorded('access.json')['snapshot']
    ctx.runner.must(['docker', 'restart', record['Id']], timeout=180, reason='container_restart_failed')
    ctx.health(timeout=180)
    cookie, csrf = ctx.login()
    after = ctx.snapshot(cookie, csrf)
    http_json(ctx.http, 'POST', '/api/logout', payload={}, cookie=cookie, csrf=csrf)
    for field in ('checkpoint', 'scanned', 'state'):
        require(after.get(field) == before.get(field), 'snapshot_changed_after_restart')
    return {'restart_policy': policy, 'snapshot': 'stable',
            'scope': 'process_restart_not_machine_reboot'}


def verify_sources(ctx):
    recorded = ctx.recorded('sources.json')
    for label in ('source_a', 'source_b'):
        require(manifest(ctx.plan[label], label) == recorded['baseline'][label], 'source_mutated')
    return {'mounts': assert_mounts_stable(ctx)}


def verify_init(ctx):
    root = ctx.plan['root']
    require(os.path.isdir(root), 'installation_root_missing')
    info = os.lstat(root)
    require(stat.S_ISDIR(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o700,
            'installation_root_mode_rejected')
    for name in ('config', 'state'):
        child = os.lstat(ctx.path(name))
        require(stat.S_IMODE(child.st_mode) == 0o700 and (child.st_uid, child.st_gid) == (10001, 10001),
                'installation_directory_identity_rejected')
    for name in ('credential', 'server.json'):
        child = os.lstat(os.path.join(root, 'config', name))
        require(stat.S_IMODE(child.st_mode) == 0o600 and child.st_uid == 10001,
                'installation_file_identity_rejected')
    prepared = load_json(ctx.path('prepared.json'), 'prepared_receipt')
    require(prepared.get('network_verified') is False and prepared.get('image_verified') is False and
            prepared.get('gateway_installed') is False, 'prepared_receipt_must_not_preverify')
    compose = open(ctx.path('compose.yaml'), 'rb').read()
    require(hashlib.sha256(compose).hexdigest() == ctx.plan['template_sha256'], 'compose_template_changed')
    return {'layout': 'verified'}


def verify_gateway(ctx):
    active = ctx.runner.must(['systemctl', 'is-active', SERVICE_UNIT], timeout=30,
                             reason='gateway_inactive').decode().strip()
    require(active == 'active', 'gateway_inactive')
    ctx.health(timeout=60)
    return {'service': SERVICE_UNIT, 'state': active}


def verify_start(ctx):
    assert_mounts_stable(ctx)
    record = ctx.inspect_container()
    require(record.get('Image') == ctx.recorded('image.json')['image_id'], 'container_image_mismatch')
    require((record.get('State', {}) or {}).get('Running') is True, 'core_not_running')
    ctx.health(timeout=60)
    return {'container': record['Id'][:12], 'health': 'ok'}


def verify_access(ctx):
    cookie, csrf = ctx.login()
    snapshot = ctx.snapshot(cookie, csrf)
    http_json(ctx.http, 'POST', '/api/logout', payload={}, cookie=cookie, csrf=csrf)
    require(snapshot.get('checkpoint') == 'completed', 'checkpoint_not_completed')
    return {'scanned': snapshot.get('scanned'), 'checkpoint': snapshot.get('checkpoint')}


def verify_backup(ctx):
    receipt = load_json(ctx.receipt_path('backup-receipt.json'), 'backup_receipt')
    verify_backup_payload(receipt)
    target = os.path.join(receipt['backup_dir'], 'state')
    current = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in manifest(target, 'backup')]
    expected = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in receipt['files']]
    require(current == expected, 'backup_manifest_changed')
    for entry in receipt['files']:
        if entry['path'].endswith('.db'):
            _quick_check(os.path.join(target, entry['path']))
    return {'files': len(receipt['files']), 'backup': 'verified'}


def verify_upgrade(ctx):
    candidate = ctx.plan.get('candidate_image')
    require(candidate, 'candidate_image_required')
    records = ctx.runner.json(['docker', 'image', 'inspect', candidate])
    require(isinstance(records, list) and len(records) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(records[0].get('Id', ''))
    record = ctx.inspect_container()
    require(record.get('Image') == candidate_id, 'upgrade_image_mismatch')
    ctx.health(timeout=60)
    return {'candidate': candidate[:19] + '…'}


def verify_rollback(ctx):
    record = ctx.inspect_container()
    require(record.get('Image') == ctx.recorded('image.json')['image_id'], 'rollback_image_mismatch')
    ctx.health(timeout=60)
    return {'container': record['Id'][:12], 'image': 'original'}


def verify_uninstall(ctx):
    remaining = ctx.runner.must(['docker', 'container', 'ls', '-a', '--filter',
                                 'label=com.ndg.test.nonce=' + ctx.plan['installation_id'],
                                 '--format', '{{.ID}}'], timeout=60,
                                reason='container_list_failed').decode().strip()
    require(not remaining, 'core_container_still_present')
    baseline = ctx.recorded('sources.json')['baseline']
    for label, path in (('source_a', ctx.plan['source_a']), ('source_b', ctx.plan['source_b'])):
        require(manifest(path, label) == baseline[label], 'uninstall_mutated_sources')
    require(os.path.isdir(ctx.path('state')), 'uninstall_must_keep_state')
    return {'container_removed': True, 'sources_unchanged': True}


def verify_reboot(ctx):
    record = ctx.inspect_container()
    policy = ((record.get('HostConfig', {}) or {}).get('RestartPolicy') or {}).get('Name')
    require(policy == 'unless-stopped', 'restart_policy_must_survive_reboot')
    require((record.get('State', {}) or {}).get('Running') is True, 'core_not_running')
    before = ctx.recorded('access.json')['snapshot']
    cookie, csrf = ctx.login()
    after = ctx.snapshot(cookie, csrf)
    http_json(ctx.http, 'POST', '/api/logout', payload={}, cookie=cookie, csrf=csrf)
    for field in ('checkpoint', 'scanned', 'state'):
        require(after.get(field) == before.get(field), 'snapshot_changed_after_restart')
    return {'restart_policy': policy, 'snapshot': 'stable'}


STEPS = {
    'compat': (step_compat, step_compat),
    'sources': (step_sources, verify_sources),
    'image': (step_image, step_image),
    'init': (step_init, verify_init),
    'gateway': (step_gateway, verify_gateway),
    'start': (step_start, verify_start),
    'browser': (step_browser, step_browser),
    'access': (step_access, verify_access),
    'gui': (step_gui, step_gui),
    'backup': (step_backup, verify_backup),
    'upgrade': (step_upgrade, verify_upgrade),
    'rollback': (step_rollback, verify_rollback),
    'uninstall': (step_uninstall, verify_uninstall),
    'reboot': (step_reboot, verify_reboot),
}


# ---------------- run ----------------

def run(plan_path, receipt, steps, acceptance, execute, reconcile=None):
    require(execute, 'explicit_execute_required')
    plan = load_plan(plan_path, acceptance)
    private_dir(receipt, 'receipt')
    authorization = load_authorization(receipt)
    require(authorization['plan_digest'] == plan['plan_digest'], 'authorization_plan_mismatch')
    if steps:
        unknown = [step for step in steps if step not in STEP_ORDER]
        require(not unknown, 'run_step_unknown')
        requested = [step for step in STEP_ORDER if step in set(steps)]
    else:
        requested = list(authorization['steps'])
    missing = [step for step in requested if step not in authorization['steps']]
    require(not missing, 'step_not_authorized')
    journal = Journal(receipt)
    state = journal.state()
    if reconcile:
        require(reconcile in STEP_ORDER, 'run_step_unknown')
        require(state.get(reconcile) == 'start', 'reconciliation_requires_interrupted_step')
        require(reconcile in RECONCILABLE, 'step_not_reconcilable')
        ctx = Context(plan, receipt, Runner(os.path.join(receipt, 'actions.jsonl')))
        try:
            detail = STEPS[reconcile][1](ctx)
        except Refused as refusal:
            journal.append(reconcile, 'failed', {'reconcile': str(refusal)})
            raise
        journal.append(reconcile, 'reconciled_ok', detail)
        return {'result': 'RECONCILED', 'step': reconcile, 'detail': detail}
    for step in requested:
        for prerequisite in STEP_PREREQS[step]:
            require(state.get(prerequisite) == 'ok', 'previous_step_incomplete')
        if state.get(step) == 'ok':
            ctx = Context(plan, receipt, Runner(os.path.join(receipt, 'actions.jsonl')))
            detail = STEPS[step][1](ctx)
            journal.append(step, 'skip', detail)
            continue
        if state.get(step) == 'start':
            require(step in RECONCILABLE, 'manual_reconciliation_required')
            ctx = Context(plan, receipt, Runner(os.path.join(receipt, 'actions.jsonl')))
            try:
                detail = STEPS[step][1](ctx)
            except Refused:
                raise Refused('manual_reconciliation_required')
            journal.append(step, 'reconciled_ok', detail)
            state[step] = 'ok'
            continue
        journal.append(step, 'start', {'scope': plan['scope']})
        ctx = Context(plan, receipt, Runner(os.path.join(receipt, 'actions.jsonl')))
        try:
            detail = STEPS[step][0](ctx)
            journal.append(step, 'ok', detail)
            state[step] = 'ok'
        except Refused as refusal:
            journal.append(step, 'failed', {'reason': str(refusal)})
            raise
    return {'result': 'CHAIN_STEPS_COMPLETE', 'steps': requested, 'scope': plan['scope']}


def status(receipt):
    private_dir(receipt, 'receipt')
    journal = Journal(receipt)
    state = journal.state()
    authorization = load_authorization(receipt) if os.path.exists(authorization_path(receipt)) else None
    return {'result': 'LIFECYCLE_STATUS', 'steps': state,
            'plan_digest_prefix': authorization['plan_digest'][:12] if authorization else None,
            'authorized_steps': authorization['steps'] if authorization else []}
