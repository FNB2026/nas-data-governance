#!/usr/bin/env python3
"""Prepare a NEW private Linux installation; never start Docker or host services.

The root-owned JSON request is an administrator authorization, not browser input.
A later execution/review step must verify the selected network, image and gateway.
"""
import argparse
import ctypes
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import stat
import sys
from urllib.parse import urlsplit


class Refused(Exception):
    pass


def require(value, category):
    if not value:
        raise Refused(category)


def unique(pairs):
    value = {}
    for key, item in pairs:
        require(key not in value, 'duplicate_request_field')
        value[key] = item
    return value


def open_directory(path):
    require(isinstance(path, str) and path.startswith('/') and (path == '/' or
            all(part not in ('', '.', '..') for part in path.split('/')[1:])), 'absolute_directory_required')
    class How(ctypes.Structure):
        _fields_ = [('flags', ctypes.c_uint64), ('mode', ctypes.c_uint64), ('resolve', ctypes.c_uint64)]
    how = How(os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW, 0, 0x04)
    libc = ctypes.CDLL(None, use_errno=True)
    # Linux x86_64/aarch64 share openat2 syscall 437. No ordinary-open fallback.
    fd = libc.syscall(437, -100, path.encode(), ctypes.byref(how), ctypes.sizeof(how))
    require(fd >= 0, 'directory_boundary_unavailable')
    return fd


def private_request(path):
    trusted_parent(str(Path(path).parent))
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    try:
        info = os.fstat(fd)
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_uid == 0 and
                stat.S_IMODE(info.st_mode) == 0o600 and info.st_size <= 65536 and
                not os.listxattr(fd), 'private_request_required')
        raw = os.read(fd, 65537)
        require(len(raw) == info.st_size, 'request_changed')
        value = json.loads(raw, object_pairs_hook=unique)
        require(isinstance(value, dict), 'request_object_required')
        return value
    finally:
        os.close(fd)


def trusted_parent(path):
    fd = open_directory(path)
    os.close(fd)
    for parent in [*reversed(Path(path).parents), Path(path)]:
        info = parent.lstat()
        require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and info.st_mode & 0o022 == 0 and
                not any(n in os.listxattr(parent, follow_symlinks=False)
                        for n in ('system.posix_acl_access', 'system.posix_acl_default')), 'trusted_parent_required')


def write_new(directory, name, raw, uid=0, gid=0):
    fd = os.open(directory / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW | os.O_CLOEXEC, 0o600)
    try:
        with os.fdopen(fd, 'wb', closefd=False) as output:
            output.write(raw)
            output.flush()
        os.fchown(fd, uid, gid)
        os.fsync(fd)
    finally:
        os.close(fd)


def sync_directory(path):
    fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def initialize(request_path):
    require(sys.platform == 'linux' and os.geteuid() == 0 and os.uname().machine == 'x86_64',
            'linux_administrator_required')
    os.umask(0o077)
    request = private_request(request_path)
    require(set(request) == {'parent', 'origin', 'network_id', 'core_ipv4', 'source_a', 'source_b', 'boundary_root'}, 'request_fields_rejected')
    require(all(isinstance(value, str) for value in request.values()), 'request_strings_required')
    origin = urlsplit(request['origin'])
    require(origin.scheme == 'https' and origin.hostname and not origin.username and not origin.password and
            origin.path == '' and origin.query == '' and origin.fragment == '' and
            origin.netloc == origin.hostname + ':' + str(origin.port) and
            origin.port is not None and 1024 <= origin.port <= 65535 and len(origin.hostname) <= 253 and
            '.' in origin.hostname and all(re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', label)
                                          for label in origin.hostname.split('.')), 'https_origin_required')
    try:
        ipaddress.ip_address(origin.hostname)
    except ValueError:
        pass
    else:
        raise Refused('dns_origin_required')
    require(re.fullmatch(r'[a-f0-9]{64}', request['network_id']), 'full_network_identity_required')
    address = ipaddress.IPv4Address(request['core_ipv4'])
    require(address.is_private and not address.is_loopback and not address.is_link_local and
            not address.is_unspecified and not address.is_multicast, 'private_core_ipv4_required')
    trusted_parent(request['boundary_root'])
    trusted_parent(request['parent'])
    boundary_fd = open_directory(request['boundary_root'])
    def confined(path):
        require(path.startswith('/') and (path == '/' or all(part not in ('', '.', '..') for part in path.split('/')[1:])), 'absolute_directory_required')
        relative = os.path.relpath(path, request['boundary_root'])
        require(relative != '..' and not relative.startswith('../'), 'outside_boundary_rejected')
        class How(ctypes.Structure):
            _fields_ = [('flags', ctypes.c_uint64), ('mode', ctypes.c_uint64), ('resolve', ctypes.c_uint64)]
        how = How(os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW, 0, 0x08 | 0x04 | 0x01)
        fd = ctypes.CDLL(None, use_errno=True).syscall(437, boundary_fd, relative.encode(), ctypes.byref(how), ctypes.sizeof(how))
        require(fd >= 0, 'nested_mount_or_alias_rejected')
        return fd
    try:
        parent_fd = confined(request['parent'])
        os.close(parent_fd)
        source_fds = []
        try:
            for alias in ('source_a', 'source_b'):
                source_fds.append(confined(request[alias]))
        finally:
            for fd in source_fds:
                os.close(fd)
    finally:
        os.close(boundary_fd)
    for alias in ('source_a', 'source_b'):
        # The shared configured boundary plus NO_XDEV excludes bind aliases;
        # textual containment is used only after that kernel restriction.
        require(os.path.commonpath([request['parent'], request[alias]]) != request[alias], 'state_inside_source_rejected')
    require(os.path.commonpath([request['source_a'], request['source_b']]) not in
            (request['source_a'], request['source_b']), 'overlapping_sources_rejected')
    template = Path(__file__).resolve().parents[2] / 'packaging/fnos/compose.lan-readonly.yaml'
    raw_template = template.read_bytes()
    require(len(raw_template) <= 65536, 'template_rejected')
    nonce = secrets.token_hex(6)
    root = Path(request['parent']) / ('ndg-readonly-' + nonce)
    require(all(not any(char in value for char in '\r\n\x00$\'"\\') for value in request.values()), 'dotenv_value_rejected')
    # No adoption/overwrite/automatic cleanup of an existing or partial root.
    root.mkdir(mode=0o700)
    try:
        for name in ('config', 'state'):
            child = root / name
            child.mkdir(mode=0o700)
            os.chown(child, 10001, 10001)
        settings = {'listen': '0.0.0.0:8080', 'origin': request['origin'], 'state': '/data',
                    'credential_file': '/config/credential',
                    'sources': {'source_a': '/sources/source_a', 'source_b': '/sources/source_b'}}
        write_new(root / 'config', 'credential', (secrets.token_hex(32) + '\n').encode(), 10001, 10001)
        write_new(root / 'config', 'server.json', (json.dumps(settings, indent=2) + '\n').encode(), 10001, 10001)
        # Literal single-quoted dotenv values; reject interpolation/escape characters.
        variables = {'NDG_CORE_CONTAINER': 'ndg-readonly-' + nonce + '-core', 'NDG_INSTALLATION_ID': nonce,
                     'NDG_CORE_IPV4': request['core_ipv4'], 'NDG_INSTALL_ROOT': str(root),
                     'NDG_SOURCE_A': request['source_a'], 'NDG_SOURCE_B': request['source_b'],
                     'NDG_INTERNAL_NETWORK_ID': request['network_id']}
        require(all(not any(char in value for char in '\r\n\x00$\'"\\') for value in variables.values()), 'dotenv_value_rejected')
        env = ''.join(key + "='" + value + "'\n" for key, value in variables.items())
        write_new(root, 'installation.env', env.encode())
        write_new(root, 'compose.yaml', raw_template)
        receipt = {'schema': 1, 'installation_id': nonce, 'status': 'PREPARED_NOT_STARTED',
                   'template_sha256': hashlib.sha256(raw_template).hexdigest(),
                   'network_verified': False, 'image_verified': False, 'gateway_installed': False,
                   'scope': 'NEW_READONLY_CORE_CONFIGURATION_ONLY',
                   'boundary_contract': 'SINGLE_CONFIGURED_MOUNT_NO_NESTED_MOUNTS_TRUSTED_ADMIN_NO_CONCURRENT_REMOUNTS'}
        write_new(root, 'prepared.json', (json.dumps(receipt, indent=2) + '\n').encode())
        for path in (root / 'config', root / 'state', root, root.parent):
            sync_directory(path)
    except BaseException:
        # The random root is left intact for explicit inspection; never recursive rm.
        raise Refused('partial_installation_preserved') from None
    return {'status': 'PREPARED_NOT_STARTED', 'installation_id': nonce, 'root': str(root)}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--create-new-installation', required=True, metavar='PRIVATE_REQUEST_JSON')
    args = parser.parse_args()
    try:
        print(json.dumps(initialize(args.create_new_installation), sort_keys=True))
    except Refused as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(1)
    except (OSError, ValueError, TypeError, KeyError):
        print('initialization_refused_inspect_private_request_or_partial_root', file=sys.stderr)
        raise SystemExit(1)
