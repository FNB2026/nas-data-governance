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
from contextlib import contextmanager
import datetime
import hashlib
import http.client
import http.cookies
import ipaddress
import json
import os
import re
import secrets
import socket
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
# The repository half of the pinned reference, derived rather than repeated, so a
# candidate upgrade cannot silently point at a different registry than the
# production pin. Test data only narrows where a candidate may come from; it
# never widens the production default.
IMAGE_REPO = PINS['image'].split('@', 1)[0]
CANDIDATE_REPO = re.compile(r'^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?(?::\d{1,5})?/[a-z0-9][a-z0-9._/-]*$')
BIND_TARGETS = {'/data': False, '/config': True, '/sources/source_a': True, '/sources/source_b': True}
HEX64 = re.compile(r'^[0-9a-f]{64}$')
HEX12 = re.compile(r'^[0-9a-f]{12}$')
PROJECT = re.compile(r'^[a-z0-9][a-z0-9_-]{0,62}$')
TOOL_VERSION = 3
PRODUCTION = 'PRODUCTION'
ACCEPTANCE = 'DISPOSABLE_LINUX_ACCEPTANCE'
SCOPES = {PRODUCTION, ACCEPTANCE}
STEP_ORDER = ['compat', 'sources', 'image', 'init', 'start', 'gateway', 'browser',
              'access', 'gui', 'backup', 'upgrade', 'rollback', 'uninstall', 'reboot']
STEP_PREREQS = {
    'compat': [], 'sources': [], 'image': [],
    'init': ['compat', 'sources'],
    'gateway': ['start'], 'start': ['init', 'image'],
    'browser': ['start'], 'access': ['start'], 'gui': ['start'],
    'backup': ['access'],
    'upgrade': ['backup'], 'rollback': ['backup'],
    'uninstall': ['start'], 'reboot': ['access'],
}
MUTATING = {'init', 'gateway', 'start', 'backup', 'upgrade', 'rollback', 'uninstall'}
# Steps whose mutation is either idempotent or fully described by a
# side-effect-free postcheck; only these may be reconciled after a crash.
# A healthy endpoint cannot prove that an interrupted gateway install owns it.
RECONCILABLE = {'init', 'start', 'backup', 'uninstall', 'upgrade', 'rollback'}
TEMPLATE_REL = 'packaging/fnos/compose.lan-readonly.yaml'
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
    # The host does not have to agree with us about which digest names an image.
    # The classic overlay store reports the platform config digest; the
    # containerd store reports the index digest itself. Both are derived from
    # the same pinned index, so both prove the same identity; anything else
    # means the local image body is not the pinned content at all.
    require(image_id in (PINS['oci_index'], PINS['amd64_config']), 'image_identity_not_bound_to_pinned_index')
    identity = 'oci_index' if image_id == PINS['oci_index'] else 'amd64_config'
    require(str(record.get('Architecture', '')) == 'amd64', 'acceptance_evidence_is_amd64_only')
    return {'image_id': image_id, 'identity_kind': identity, 'pinned_repo_digest': matched[0],
            'architecture': 'amd64', 'oci_index': PINS['oci_index'], 'amd64_config': PINS['amd64_config']}


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


def confined_open(root_fd, relative, flags, reason):
    require(sys.platform == 'linux' and os.uname().machine in ('x86_64', 'aarch64'),
            'kernel_confinement_unavailable')
    import ctypes

    class OpenHow(ctypes.Structure):
        _fields_ = [('flags', ctypes.c_uint64), ('mode', ctypes.c_uint64),
                    ('resolve', ctypes.c_uint64)]

    beneath_no_symlinks_no_xdev = 0x08 | 0x04 | 0x01
    how = OpenHow(flags | os.O_CLOEXEC, 0, beneath_no_symlinks_no_xdev)
    fd = ctypes.CDLL(None, use_errno=True).syscall(437, root_fd, os.fsencode(relative),
                                                   ctypes.byref(how), ctypes.sizeof(how))
    require(fd >= 0, reason)
    return fd


@contextmanager
def confined_directory(boundary, path, label):
    strict_path(boundary, label)
    strict_path(path, label)
    require(os.path.commonpath([boundary, path]) == boundary, f'{label}_outside_boundary_rejected')
    boundary_fd = os.open(boundary, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        fd = confined_open(boundary_fd, os.path.relpath(path, boundary), os.O_RDONLY | os.O_DIRECTORY,
                           f'{label}_nested_mount_or_alias_rejected')
        try:
            yield fd
        finally:
            os.close(fd)
    finally:
        os.close(boundary_fd)


def kernel_confined(boundary, path, label):
    if sys.platform != 'linux':
        return 'kernel_confinement_not_applicable'
    with confined_directory(boundary, path, label):
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
        kernel_confined(ctx.plan['boundary_root'], ctx.plan[label], label)
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


def tree_entries(root_fd, directory='.'):
    # Resolve every descendant from the original root. A retained child FD
    # could otherwise lead outside the root after an intervening rename.
    fd = confined_open(root_fd, directory, os.O_RDONLY | os.O_DIRECTORY, 'tree_mount_or_alias_rejected')
    try:
        names = sorted(os.listdir(fd))
    finally:
        os.close(fd)
    children = []
    for name in names:
        relative = name if directory == '.' else os.path.join(directory, name)
        fd = confined_open(root_fd, relative, os.O_PATH, 'tree_mount_or_alias_rejected')
        try:
            info = os.fstat(fd)
            if stat.S_ISDIR(info.st_mode):
                children.append(relative)
                yield relative, info, None
            else:
                require(stat.S_ISREG(info.st_mode), 'non_regular_file_rejected')
                # The kernel FD refers to the regular inode that was just
                # inspected. Reopening the source pathname would race.
                with open(f'/proc/self/fd/{fd}', 'rb') as reader:
                    yield relative, info, reader
        finally:
            os.close(fd)
    for child in children:
        yield from tree_entries(root_fd, child)


def manifest(root, label='manifest', *, boundary=None):
    entries = []
    with confined_directory(boundary or root, root, label) as root_fd:
        for path, info, reader in tree_entries(root_fd):
            if reader is None:
                continue
            require(info.st_size <= (1 << 31), 'file_too_large_for_manifest')
            digest = hashlib.sha256()
            for block in iter(lambda: reader.read(1 << 20), b''):
                digest.update(block)
            entries.append({'path': path, 'sha256': digest.hexdigest(), 'size': info.st_size,
                            'mode': format(stat.S_IMODE(info.st_mode), '04o'),
                            'uid': info.st_uid, 'gid': info.st_gid})
            require(len(entries) <= 20000, 'manifest_entry_limit_exceeded')
    require(entries, 'empty_tree_rejected')
    return entries


def copy_tree(source, destination, uid=None, gid=None, *, boundary=None):
    import shutil

    strict_path(destination, 'copy_destination')
    require(os.path.commonpath([source, destination]) not in (source, destination),
            'copy_source_destination_overlap')
    destination_parent, destination_name = os.path.split(destination)
    with confined_directory(boundary or source, source, 'copy_source') as root_fd, \
            confined_directory(boundary or destination_parent, destination_parent,
                               'copy_destination') as parent_fd:
        require(not os.path.lexists(destination), 'copy_destination_must_not_exist')
        os.mkdir(destination_name, 0o700, dir_fd=parent_fd)
        destination_fd = confined_open(parent_fd, destination_name, os.O_RDONLY | os.O_DIRECTORY,
                                       'copy_destination_mount_or_alias_rejected')
        try:
            if uid is not None:
                os.fchown(destination_fd, uid, gid if gid is not None else uid)
            os.fchmod(destination_fd, stat.S_IMODE(os.fstat(root_fd).st_mode))
            for path, info, reader in tree_entries(root_fd):
                parent, name = os.path.split(path)
                output_parent = confined_open(destination_fd, parent or '.', os.O_RDONLY | os.O_DIRECTORY,
                                              'copy_destination_mount_or_alias_rejected')
                try:
                    mode = stat.S_IMODE(info.st_mode)
                    owner = uid if uid is not None else info.st_uid
                    group = gid if gid is not None else info.st_gid
                    if reader is None:
                        os.mkdir(name, 0o700, dir_fd=output_parent)
                        child = confined_open(output_parent, name, os.O_RDONLY | os.O_DIRECTORY,
                                              'copy_destination_mount_or_alias_rejected')
                        try:
                            os.fchown(child, owner, group)
                            os.fchmod(child, mode)
                            os.fsync(child)
                        finally:
                            os.close(child)
                    else:
                        flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC
                        fd = os.open(name, flags, 0o600, dir_fd=output_parent)
                        with os.fdopen(fd, 'wb') as output:
                            shutil.copyfileobj(reader, output, length=1 << 20)
                            output.flush()
                            os.fchown(output.fileno(), owner, group)
                            os.fchmod(output.fileno(), mode)
                            os.fsync(output.fileno())
                            require(stat.S_IMODE(os.fstat(output.fileno()).st_mode) == mode, 'copy_mode_not_applied')
                    os.fsync(output_parent)
                finally:
                    os.close(output_parent)
            os.fsync(destination_fd)
            os.fsync(parent_fd)
        finally:
            os.close(destination_fd)


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

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class PinnedHTTPSHandler(urllib.request.HTTPSHandler):
    def __init__(self, context, connect_ipv4, leaf_sha256):
        super().__init__(context=context)
        self.connect_ipv4 = connect_ipv4
        self.leaf_sha256 = leaf_sha256

    def https_open(self, req):
        address, expected_leaf = self.connect_ipv4, self.leaf_sha256

        class Connection(http.client.HTTPSConnection):
            def connect(self):
                self._create_connection = lambda addr, *args, **options: socket.create_connection(
                    (address, addr[1]), *args, **options)
                super().connect()
                leaf = self.sock.getpeercert(binary_form=True)
                if not leaf or hashlib.sha256(leaf).hexdigest() != expected_leaf:
                    self.close()
                    raise ssl.SSLError('gateway_tls_leaf_mismatch')

        return self.do_open(Connection, req, context=self._context)


class HTTP(object):
    def __init__(self, base, ca_file=None, connect_ipv4=None, leaf_sha256=None):
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
        require(connect_ipv4 is None or self.context is not None and isinstance(leaf_sha256, str)
                and HEX64.fullmatch(leaf_sha256), 'pinned_address_requires_https_leaf')
        https = (PinnedHTTPSHandler(self.context, connect_ipv4, leaf_sha256) if connect_ipv4
                 else urllib.request.HTTPSHandler(context=self.context))
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect(), https)

    def request(self, method, path, payload=None, headers=None, cookie=None, timeout=30):
        require(path.startswith('/'), 'request_path_required')
        url = self.base + path
        body = None
        merged = {'Accept': 'application/json'}
        merged['Origin'] = self.base
        if headers:
            merged.update(headers)
        if payload is not None:
            body = canonical(payload)
            merged['Content-Type'] = 'application/json'
        if cookie:
            merged['Cookie'] = cookie
        request = urllib.request.Request(url, data=body, method=method, headers=merged)
        try:
            with self.opener.open(request, timeout=timeout) as response:
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
               'candidate_image', 'candidate_repo', 'expected_scanned', 'gateway', 'tls_ca',
               'network_owned', 'request_sha256'}


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


def gateway_spec(plan):
    require(plan.get('scope') == PRODUCTION, 'gateway_installation_not_in_acceptance_scope')
    identity = plan.get('installation_id')
    require(isinstance(identity, str) and HEX12.fullmatch(identity), 'installation_id_shape_rejected')
    values = plan.get('gateway')
    require(isinstance(values, dict) and set(values) == {
        'listen_ipv4', 'binary_sha256', 'certificate_sha256', 'leaf_sha256',
    }, 'gateway_parameters_required')
    for field in ('binary_sha256', 'certificate_sha256', 'leaf_sha256'):
        require(isinstance(values[field], str) and HEX64.fullmatch(values[field]), 'gateway_digest_required')
    try:
        listen_ip = ipaddress.IPv4Address(values['listen_ipv4'])
    except (ValueError, TypeError):
        raise Refused('gateway_lan_ipv4_required') from None
    lan = any(listen_ip in ipaddress.IPv4Network(block)
              for block in ('10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16'))
    require(isinstance(values['listen_ipv4'], str) and lan, 'gateway_lan_ipv4_required')
    core = plan['core_ipv4']
    require(isinstance(core, str) and re.fullmatch(r'10(?:\.\d{1,3}){3}', core)
            and all(int(octet) <= 255 for octet in core.split('.')),
            'private_core_ipv4_required')
    require(str(listen_ip) != core, 'gateway_must_not_bind_core_ip')
    origin = _validate_origin(plan['origin'], PRODUCTION, core)
    port = origin.rsplit(':', 1)[1]
    require(str(int(port)) == port, 'origin_port_rejected')
    account = 'ndgweb-' + identity
    directory = '/etc/ndg/instances/' + identity
    binary = directory + '/ndg-fnos-https'
    template = (open(os.path.join(repo_root(), UNIT_TEMPLATE_REL), 'rb').read()).decode()
    for before, after in (
        ('User=ndgreadonlyweb\n', 'User=' + account + '\n'),
        ('Group=ndgreadonlyweb\n', 'Group=' + account + '\n'),
        ('ExecStart=/var/apps/ndg-readonly-web/cmd/ndg-fnos-https\n',
         'ExecStart=' + binary + ' --instance ' + identity + '\n'),
    ):
        require(template.splitlines(keepends=True).count(before) == 1, 'gateway_unit_template_rejected')
        template = template.replace('\n' + before, '\n' + after, 1)
    config = {'listen': str(listen_ip) + ':' + port, 'gateway': {
        'app_name': account, 'package_user': account, 'package_group': account,
        'origin': origin, 'upstream': 'http://' + core + ':8080',
    }}
    return {'unit_name': 'ndg-readonly-https-' + identity + '.service',
            'account': account, 'directory': directory, 'config': config, 'unit': template,
            'session_cookie': 'ndg_' + hashlib.sha256(account.encode()).hexdigest()[:12]}


def verify_backup_location(boundary, installation_root, source_a, source_b, backup_dir):
    require(os.path.commonpath([boundary, backup_dir]) == boundary, 'backup_dir_outside_boundary_rejected')
    for protected in (installation_root, source_a, source_b):
        require(os.path.commonpath([backup_dir, protected]) not in (backup_dir, protected),
                'backup_dir_rejected')


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
    candidate_repo = request.get('candidate_repo')
    if candidate_repo is None:
        candidate_repo = IMAGE_REPO
    else:
        require(isinstance(candidate_repo, str), 'candidate_repo_string_required')
        require('@' not in candidate_repo and CANDIDATE_REPO.fullmatch(candidate_repo) is not None,
                'candidate_repo_shape_rejected')
    # Production has exactly one allowed source: the pinned GHCR repository.
    # Only the disposable acceptance scope may name a different (local) registry,
    # and only when it also names a candidate, so a test cannot loosen the
    # production image-source contract by omission.
    if scope == PRODUCTION:
        require(candidate_repo == IMAGE_REPO, 'production_candidate_repo_must_be_pinned_ghcr')
    else:
        require(candidate_repo == IMAGE_REPO or candidate is not None,
                'candidate_repo_requires_candidate_image')
    expected = request.get('expected_scanned', 1)
    require(isinstance(expected, int) and 1 <= expected <= 1000000, 'expected_scanned_rejected')
    gateway = request.get('gateway')
    if scope == PRODUCTION:
        spec = gateway_spec(request)
    else:
        require(gateway is None, 'gateway_not_in_acceptance_scope')
    tls_ca = request.get('tls_ca')
    if tls_ca is not None:
        strict_path(tls_ca, 'tls_ca')
    network_owned = request.get('network_owned', False)
    require(isinstance(network_owned, bool), 'network_owned_boolean_required')
    root = os.path.join(parent, 'ndg-readonly-' + request['installation_id'])
    strict_path(root, 'root')
    verify_backup_location(boundary, root, source_a, source_b, backup_dir)
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
        plan['candidate_repo'] = candidate_repo
    if tls_ca:
        plan['tls_ca'] = tls_ca
    if gateway:
        plan['gateway'] = gateway
        plan['gateway_spec_sha256'] = digest_of(spec)
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
    verify_backup_location(*(plan[field] for field in ('boundary_root', 'root', 'source_a', 'source_b', 'backup_dir')))
    require(plan.get('access_base') == plan.get('origin'), 'access_origin_mismatch')
    if plan.get('candidate_repo') is not None:
        require(isinstance(plan['candidate_repo'], str) and
                CANDIDATE_REPO.fullmatch(plan['candidate_repo']) is not None,
                'candidate_repo_shape_rejected')
        if plan['scope'] == PRODUCTION:
            require(plan['candidate_repo'] == IMAGE_REPO, 'production_candidate_repo_must_be_pinned_ghcr')
    if plan['scope'] == PRODUCTION:
        frozen_gateway_spec(plan)
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
        connect_ipv4 = leaf_sha256 = None
        self.session_cookie = 'ndg_session'
        if plan['scope'] == PRODUCTION:
            self.session_cookie = frozen_gateway_spec(plan)['session_cookie']
            connect_ipv4 = plan['gateway']['listen_ipv4']
            leaf_sha256 = plan['gateway']['leaf_sha256']
        self.http = HTTP(plan['access_base'], plan.get('tls_ca'), connect_ipv4, leaf_sha256)

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
    def core_health(self, timeout=120):
        origin = self.plan['origin']
        direct = HTTP('http://' + self.plan['core_ipv4'] + ':8080')
        return self.health(timeout, direct, {'Host': origin.split('://', 1)[1], 'Origin': origin})

    def health(self, timeout=120, http=None, headers=None):
        http = http or self.http
        deadline = time.time() + timeout
        last = None
        while time.time() < deadline:
            try:
                status, _, _ = http.request('GET', '/healthz', headers=headers, timeout=5)
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
        require(self.session_cookie in cookies, 'session_cookie_missing')
        session = cookies[self.session_cookie]
        if self.plan['scope'] == PRODUCTION:
            require(session['secure'] and session['httponly'] and not session['domain']
                    and session['path'] == '/' and session['samesite'].lower() == 'strict',
                    'gateway_session_cookie_scope_rejected')
        try:
            body = json.loads(raw)
        except ValueError:
            raise Refused('login_response_unreadable')
        require(isinstance(body.get('csrf'), str) and len(body['csrf']) >= 32, 'csrf_missing')
        return self.session_cookie + '=' + session.value, body['csrf']

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
    baseline = {label: manifest(plan[label], label, boundary=plan['boundary_root']) for label in ('source_a', 'source_b')}
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


def install_new_gateway_unit(runner, unit_dir, unit_name, raw_unit):
    load_state = runner.must(
        ['systemctl', 'show', '--property=LoadState', '--value', unit_name],
        timeout=30, reason='gateway_unit_state_unavailable').decode().strip()
    require(load_state == 'not-found', 'gateway_unit_already_exists')
    try:
        write_new(unit_dir, unit_name, raw_unit, 0o644)
    except FileExistsError:
        raise Refused('gateway_unit_already_exists') from None


def frozen_gateway_spec(plan):
    spec = gateway_spec(plan)
    require(digest_of(spec) == plan.get('gateway_spec_sha256'), 'gateway_spec_changed')
    return spec


def read_gateway_file(directory, name, mode, max_bytes, gid=0):
    path = os.path.join(directory, name)
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(fd, 'rb') as handle:
        info = os.fstat(handle.fileno())
        require(stat.S_ISREG(info.st_mode) and info.st_uid == 0 and info.st_gid == gid
                and info.st_nlink == 1 and stat.S_IMODE(info.st_mode) == mode,
                'gateway_material_identity_rejected')
        require(0 < info.st_size <= max_bytes and _no_acl(path), 'gateway_material_rejected')
        data = handle.read(max_bytes + 1)
        require(len(data) == info.st_size, 'gateway_material_rejected')
    return data


def gateway_leaf_sha256(pem):
    match = re.search(rb'-----BEGIN CERTIFICATE-----[\r\n]+[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----', pem)
    require(match is not None, 'gateway_certificate_invalid')
    return hashlib.sha256(ssl.PEM_cert_to_DER_cert(match.group().decode())).hexdigest()


def checked_gateway_material(plan, spec):
    import grp
    import pwd
    directory = spec['directory']
    trusted_ancestors(directory, 'gateway_directory')
    kernel_confined('/', directory, 'gateway_directory')
    account = spec['account']
    try:
        user, group = pwd.getpwnam(account), grp.getgrnam(account)
    except KeyError:
        raise Refused('gateway_identity_missing') from None
    groups = grp.getgrall()
    require(user.pw_uid > 0 and group.gr_gid > 0 and user.pw_gid == group.gr_gid
            and user.pw_uid != 10001 and group.gr_gid != 10001
            and {u.pw_name for u in pwd.getpwall() if u.pw_uid == user.pw_uid or u.pw_gid == group.gr_gid} == {account}
            and {g.gr_name for g in groups if g.gr_gid == group.gr_gid} == {account}
            and set(group.gr_mem) <= {account}
            and all(account not in g.gr_mem for g in groups if g.gr_name != account),
            'gateway_identity_not_exclusive')
    config = read_gateway_file(directory, 'https.json', 0o644, 4096)
    require(config == canonical(spec['config']) + b'\n', 'gateway_config_does_not_match_plan')
    cert = read_gateway_file(directory, 'tls.crt', 0o644, 65536)
    require(hashlib.sha256(cert).hexdigest() == plan['gateway']['certificate_sha256'], 'gateway_certificate_changed')
    require(gateway_leaf_sha256(cert) == plan['gateway']['leaf_sha256'], 'gateway_leaf_does_not_match_plan')
    binary = read_gateway_file(directory, 'ndg-fnos-https', 0o755, 64 << 20)
    require(hashlib.sha256(binary).hexdigest() == plan['gateway']['binary_sha256'], 'gateway_binary_changed')
    # The runtime verifies TLS key ownership/mode, pair and hostname before binding.
    key_path = os.path.join(directory, 'tls.key')
    key = os.lstat(key_path)
    require(stat.S_ISREG(key.st_mode) and key.st_uid == 0 and key.st_gid == group.gr_gid
            and stat.S_IMODE(key.st_mode) == 0o640 and key.st_nlink == 1 and _no_acl(key_path),
            'gateway_key_identity_rejected')


def step_gateway(ctx):
    spec = frozen_gateway_spec(ctx.plan)
    checked_gateway_material(ctx.plan, spec)
    verify_start(ctx)
    unit = spec['unit_name']
    install_new_gateway_unit(ctx.runner, '/etc/systemd/system', unit, spec['unit'].encode())
    ctx.runner.must(['systemctl', 'daemon-reload'], timeout=60, reason='systemd_reload_failed')
    verify_gateway_unit(ctx, spec)
    try:
        ctx.runner.must(['systemctl', 'start', unit], timeout=120, reason='gateway_start_failed')
        deadline = time.monotonic() + 15
        while True:
            try:
                verify_gateway(ctx)
                break
            except (Refused, OSError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.5)
    except (Refused, OSError):
        # The exact new fragment was confirmed before we attempted start.
        # Keep its file and the journal for inspection, but close its listener.
        verify_gateway_unit(ctx, spec)
        ctx.runner.must(['systemctl', 'stop', unit], timeout=60, reason='gateway_stop_failed')
        raise
    return {'service': unit, 'state': 'active', 'boot_enable': 'separate_host_step'}


def planned_bind_sources(plan):
    return {'/data': os.path.join(plan['root'], 'state'),
            '/config': os.path.join(plan['root'], 'config'),
            '/sources/source_a': plan['source_a'], '/sources/source_b': plan['source_b']}


def verify_core_binding(plan, record):
    mounts = {mount.get('Destination'): mount for mount in record.get('Mounts', []) or []}
    require(set(mounts) == set(BIND_TARGETS), 'container_bind_set_mismatch')
    sources = planned_bind_sources(plan)
    for target, read_only in BIND_TARGETS.items():
        require(mounts[target].get('Type') == 'bind' and mounts[target].get('Source') == sources[target],
                'container_bind_source_mismatch')
        require(bool(mounts[target].get('RW')) is not read_only, 'container_bind_mode_mismatch')
    networks = (record.get('NetworkSettings') or {}).get('Networks') or {}
    require(len(networks) == 1, 'single_container_network_required')
    attached = next(iter(networks.values()))
    require(attached.get('NetworkID') == plan['network_id'] and attached.get('IPAddress') == plan['core_ipv4'],
            'core_network_binding_mismatch')


def step_start(ctx):
    plan = ctx.plan
    prepared = load_json(ctx.path('prepared.json'), 'prepared_receipt')
    assert_mounts_stable(ctx)
    config = ctx.resolved_config()
    verify_mounts_payload(config, prepared)
    variables = config['services']['core']
    require({v['target']: v['source'] for v in variables['volumes']} == planned_bind_sources(plan),
            'compose_bind_source_mismatch')
    require(set(variables['networks']) == {'core'} and
            variables['networks']['core'].get('ipv4_address') == plan['core_ipv4'] and
            config['networks']['core']['name'] == plan['network_id'], 'compose_network_binding_mismatch')

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
    ctx.core_health(timeout=180)
    record = ctx.inspect_container()
    expected_image = ctx.recorded('image.json')['image_id']
    require(record.get('Image', '') == expected_image, 'container_image_mismatch')
    require(str((record.get('Config', {}) or {}).get('User', '')) == '10001:10001', 'container_user_mismatch')
    require((record.get('HostConfig', {}) or {}).get('ReadonlyRootfs') is True, 'container_root_not_readonly')
    require(not (record.get('HostConfig', {}) or {}).get('PortBindings'), 'container_publishes_ports')
    verify_core_binding(plan, record)
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
        require(manifest(path, label, boundary=ctx.plan['boundary_root']) == baseline[label], 'source_mutated_by_scan')
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
    verify_backup_location(*(plan[field] for field in ('boundary_root', 'root', 'source_a', 'source_b', 'backup_dir')))
    require(not os.path.lexists(plan['backup_dir']), 'backup_directory_must_be_new')
    backup_parent = os.path.dirname(plan['backup_dir'])
    trusted_ancestors(backup_parent, 'backup_parent')
    kernel_confined(plan['boundary_root'], backup_parent, 'backup_parent')
    record = ctx.inspect_container()
    require((record.get('State', {}) or {}).get('Running') is True, 'core_must_be_running_before_cold_backup')
    ctx.runner.must(['docker', 'container', 'stop', record['Id']], timeout=180, reason='container_stop_failed')
    stopped = ctx.inspect_container(record['Id'])
    require((stopped.get('State', {}) or {}).get('Running') is False, 'container_still_running')
    ensure_dir(plan['backup_dir'], 0o700, os.geteuid(), os.getegid())
    target = os.path.join(plan['backup_dir'], 'state')
    copy_tree(ctx.path('state'), target, 10001, 10001, boundary=plan['boundary_root'])
    files = manifest(target, 'backup', boundary=ctx.plan['boundary_root'])
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


def candidate_reference(plan):
    """Full repository@digest reference for the upgrade candidate.

    A bare index digest is not a valid `docker image inspect` target on a
    multi-architecture image: Docker resolves digests per repository. Every
    inspect and every compose rewrite must therefore use the same fully
    qualified reference, taken from one place.
    """
    repo = plan.get('candidate_repo') or IMAGE_REPO
    return repo + '@' + plan['candidate_image']


def step_upgrade(ctx):
    plan = ctx.plan
    candidate = plan.get('candidate_image')
    require(candidate, 'candidate_image_required')
    reference = candidate_reference(plan)
    records = ctx.runner.json(['docker', 'image', 'inspect', reference])
    require(isinstance(records, list) and len(records) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(records[0].get('Id', ''))
    require(candidate_id.startswith('sha256:') and
            candidate_id != ctx.recorded('image.json')['image_id'],
            'candidate_must_be_a_different_digest_pinned_image')
    current = open(ctx.path('compose.yaml'), 'rb').read().decode()
    replaced, count = re.subn(r'(?m)^(\s*image:\s*)\S+$',
                              lambda match: match.group(1) + reference,
                              current)
    require(count == 1, 'compose_image_binding_not_unique')
    atomic_write(ctx.path('compose.yaml'), replaced.encode(), 0o644)
    ctx.compose('up', '-d', '--no-build', '--pull', 'never', '--no-deps', timeout=600)
    ctx.core_health(timeout=180)
    upgraded = ctx.inspect_container()
    require(upgraded.get('Image') == candidate_id, 'upgrade_image_mismatch')
    return {'candidate': candidate[:19] + '…', 'container': upgraded['Id'][:12]}


def step_rollback(ctx):
    plan = ctx.plan
    receipt = load_json(ctx.receipt_path('backup-receipt.json'), 'backup_receipt')
    verify_backup(ctx)
    assert_mounts_stable(ctx)
    record = ctx.inspect_container()
    if (record.get('State', {}) or {}).get('Running'):
        ctx.runner.must(['docker', 'container', 'stop', record['Id']], timeout=180,
                        reason='container_stop_failed')
    original = open(ctx.receipt_path('compose-before-upgrade.yaml'), 'rb').read()
    failed = ctx.path('state') + '.failed-' + utcnow().replace(':', '').replace('-', '')
    os.rename(ctx.path('state'), failed)
    copy_tree(os.path.join(receipt['backup_dir'], 'state'), ctx.path('state'), 10001, 10001,
              boundary=plan['boundary_root'])
    restored = manifest(ctx.path('state'), 'restored', boundary=plan['boundary_root'])
    expected = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in receipt['files']]
    actual = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in restored]
    require(actual == expected, 'restored_state_differs_from_backup')
    atomic_write(ctx.path('compose.yaml'), original, 0o644)
    ctx.compose('up', '-d', '--no-build', '--pull', 'never', '--no-deps', timeout=600)
    ctx.core_health(timeout=180)
    record = ctx.inspect_container()
    # Compare against the identity this installation actually recorded, not a
    # config-digest constant: the host may name the image by its index digest.
    require(record.get('Image') == ctx.recorded('image.json')['image_id'], 'rollback_image_mismatch')
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
        require(manifest(path, label, boundary=ctx.plan['boundary_root']) == baseline[label], 'uninstall_mutated_sources')
    receipt = load_json(ctx.receipt_path('backup-receipt.json'), 'backup_receipt')
    backup = manifest(os.path.join(receipt['backup_dir'], 'state'), 'backup', boundary=ctx.plan['boundary_root'])
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
    ctx.core_health(timeout=180)
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
        require(manifest(ctx.plan[label], label, boundary=ctx.plan['boundary_root']) == recorded['baseline'][label], 'source_mutated')
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


def verify_gateway_unit(ctx, spec):
    unit = spec['unit_name']
    expected = '/etc/systemd/system/' + unit
    fragment = ctx.runner.must(['systemctl', 'show', '--property=FragmentPath', '--value', unit],
                               timeout=30, reason='gateway_unit_state_unavailable').decode().strip()
    dropins = ctx.runner.must(['systemctl', 'show', '--property=DropInPaths', '--value', unit],
                              timeout=30, reason='gateway_unit_state_unavailable').decode().strip()
    require(fragment == expected and not dropins
            and read_gateway_file('/etc/systemd/system', unit, 0o644, 16384) == spec['unit'].encode(),
            'gateway_loaded_unit_mismatch')


def gateway_main_pid(ctx, unit):
    raw = ctx.runner.must(['systemctl', 'show', '--property=MainPID', '--value', unit],
                          timeout=30, reason='gateway_unit_state_unavailable').decode().strip()
    require(bool(re.fullmatch(r'[1-9][0-9]{0,9}', raw)), 'gateway_main_pid_missing')
    return raw


def verify_gateway_listener(spec, pid):
    address, port = spec['config']['listen'].split(':')
    socket_key = socket.inet_aton(address)[::-1].hex().upper() + ':' + format(int(port), '04X')
    proc = '/proc/' + pid
    binary = os.stat(spec['directory'] + '/ndg-fnos-https', follow_symlinks=False)
    executing = os.stat(proc + '/exe')
    require((binary.st_dev, binary.st_ino) == (executing.st_dev, executing.st_ino), 'gateway_executable_mismatch')
    inodes = set()
    with open(proc + '/net/tcp', encoding='ascii') as records:
        for row in records:
            fields = row.split()
            if len(fields) >= 10 and fields[1] == socket_key and fields[3] == '0A':
                inodes.add('socket:[' + fields[9] + ']')
    require(len(inodes) == 1, 'gateway_listener_identity_missing')
    for name in os.listdir(proc + '/fd'):
        try:
            if os.readlink(proc + '/fd/' + name) in inodes:
                return
        except FileNotFoundError:
            continue
    raise Refused('gateway_listener_not_owned_by_unit')


def verify_gateway(ctx):
    spec = frozen_gateway_spec(ctx.plan)
    unit = spec['unit_name']
    verify_gateway_unit(ctx, spec)
    active = ctx.runner.must(['systemctl', 'is-active', unit], timeout=30,
                             reason='gateway_inactive').decode().strip()
    require(active == 'active', 'gateway_inactive')
    pid = gateway_main_pid(ctx, unit)
    verify_gateway_listener(spec, pid)
    # The gateway's UI loads even if its upstream is down. Require the
    # allowlisted session endpoint to return the core's unauthenticated 401.
    status, _, _ = ctx.http.request('GET', '/api/session', timeout=10)
    require(status == 401, 'gateway_upstream_unavailable')
    require(pid == gateway_main_pid(ctx, unit), 'gateway_main_pid_changed')
    verify_gateway_listener(spec, pid)
    return {'service': unit, 'state': active}


def verify_start(ctx):
    assert_mounts_stable(ctx)
    record = ctx.inspect_container()
    require(record.get('Image') == ctx.recorded('image.json')['image_id'], 'container_image_mismatch')
    require((record.get('State', {}) or {}).get('Running') is True, 'core_not_running')
    current = ctx.recorded('container.json')
    require(record['Id'] == current['id'], 'core_container_changed')
    verify_core_binding(ctx.plan, record)
    ctx.core_health(timeout=60)
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
    require(receipt['backup_dir'] == ctx.plan['backup_dir'], 'backup_plan_location_mismatch')
    target = os.path.join(receipt['backup_dir'], 'state')
    current = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in manifest(target, 'backup', boundary=ctx.plan['boundary_root'])]
    expected = [{'path': entry['path'], 'sha256': entry['sha256']} for entry in receipt['files']]
    require(current == expected, 'backup_manifest_changed')
    for entry in receipt['files']:
        if entry['path'].endswith('.db'):
            _quick_check(os.path.join(target, entry['path']))
    return {'files': len(receipt['files']), 'backup': 'verified'}


def verify_upgrade(ctx):
    candidate = ctx.plan.get('candidate_image')
    require(candidate, 'candidate_image_required')
    records = ctx.runner.json(['docker', 'image', 'inspect', candidate_reference(ctx.plan)])
    require(isinstance(records, list) and len(records) == 1, 'candidate_inspect_single_record_required')
    candidate_id = str(records[0].get('Id', ''))
    record = ctx.inspect_container()
    require(record.get('Image') == candidate_id, 'upgrade_image_mismatch')
    ctx.core_health(timeout=60)
    return {'candidate': candidate[:19] + '…'}


def verify_rollback(ctx):
    record = ctx.inspect_container()
    require(record.get('Image') == ctx.recorded('image.json')['image_id'], 'rollback_image_mismatch')
    ctx.core_health(timeout=60)
    return {'container': record['Id'][:12], 'image': 'original'}


def verify_uninstall(ctx):
    remaining = ctx.runner.must(['docker', 'container', 'ls', '-a', '--filter',
                                 'label=com.ndg.test.nonce=' + ctx.plan['installation_id'],
                                 '--format', '{{.ID}}'], timeout=60,
                                reason='container_list_failed').decode().strip()
    require(not remaining, 'core_container_still_present')
    baseline = ctx.recorded('sources.json')['baseline']
    for label, path in (('source_a', ctx.plan['source_a']), ('source_b', ctx.plan['source_b'])):
        require(manifest(path, label, boundary=ctx.plan['boundary_root']) == baseline[label], 'uninstall_mutated_sources')
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
        if plan['scope'] == PRODUCTION and step in ('browser', 'access', 'gui'):
            require(state.get('gateway') == 'ok', 'previous_step_incomplete')
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
