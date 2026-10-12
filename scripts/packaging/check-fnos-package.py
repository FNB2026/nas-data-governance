#!/usr/bin/env python3
"""Offline, stdlib-only research-package checks; never contacts a daemon.

The Compose document deliberately uses the JSON subset of YAML. This checker
does not claim fnOS runtime, receipt authenticity, or fnpack equivalence.
"""
import argparse
import ctypes
import json
import os
from pathlib import Path
import re
import stat
import struct
import sys

APP = "ndg-readonly-test"
DIGEST = re.compile(r"^[a-z0-9][a-z0-9./_-]*@sha256:[0-9a-f]{64}$")
SHA = re.compile(r"^[0-9a-f]{40}$")
SOURCE = "${NDG_AUTHORIZED_SOURCE:?source authorization required}"
STATE = "${TRIM_PKGVAR:?private state required}"
USER = "${TRIM_UID:?package uid required}:${TRIM_GID:?package gid required}"


class Rejected(Exception):
    pass


def require(condition, code):
    if not condition:
        raise Rejected(code)


def document(path):
    def unique(pairs):
        value = {}
        for key, item in pairs:
            require(key not in value, "duplicate_json_key")
            value[key] = item
        return value
    try:
        require(path.is_file() and not path.is_symlink() and path.stat().st_size <= 1024 * 1024, "invalid_document_input")
        return json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=unique)
    except (OSError, ValueError, UnicodeError):
        raise Rejected("invalid_json") from None


def check_tree(root):
    check_tree_with_boundaries(root)


def linux_mountpoints(text):
    """mountinfo includes same-device bind mounts and mount-on-file entries."""
    points = set()
    for line in text.splitlines():
        fields = line.split()
        require(len(fields) >= 10 and "-" in fields[6:], "invalid_mount_boundary_source")
        raw = fields[4]
        # Kernel mountinfo escapes space, tab, newline and backslash as octal.
        require(not re.search(r"\\(?![0-7]{3})", raw), "invalid_mount_boundary_source")
        decoded = re.sub(r"\\([0-7]{3})", lambda match: chr(int(match[1], 8)), raw)
        require(decoded.startswith("/") and "\x00" not in decoded, "invalid_mount_boundary_source")
        points.add(Path(os.path.normpath(decoded)))
    require(Path("/") in points, "invalid_mount_boundary_source")
    return points


def darwin_mountpoints():
    """64-bit Darwin statfs ABI, getmntinfo with MNT_NOWAIT.

    Only mount target names are used. No mount syscall, source-volume probe,
    available-space reason, or application-runtime privacy claim is introduced.
    Unsupported ABI/symbol/error is a rejection, never an empty boundary set.
    """
    require(ctypes.sizeof(ctypes.c_void_p) == 8, "unsupported_mount_boundary_platform")
    architecture = os.uname().machine
    require(architecture in {"arm64", "x86_64"}, "unsupported_mount_boundary_platform")

    class StatFS64(ctypes.Structure):
        _fields_ = [("bsize", ctypes.c_uint32), ("iosize", ctypes.c_int32),
                    ("blocks", ctypes.c_uint64), ("bfree", ctypes.c_uint64),
                    ("bavail", ctypes.c_uint64), ("files", ctypes.c_uint64),
                    ("ffree", ctypes.c_uint64), ("fsid", ctypes.c_int32 * 2),
                    ("owner", ctypes.c_uint32), ("type", ctypes.c_uint32),
                    ("flags", ctypes.c_uint32), ("subtype", ctypes.c_uint32),
                    ("fstypename", ctypes.c_char * 16), ("mntonname", ctypes.c_char * 1024),
                    ("mntfromname", ctypes.c_char * 1024), ("flags_ext", ctypes.c_uint32),
                    ("reserved", ctypes.c_uint32 * 7)]

    require(ctypes.sizeof(StatFS64) == 2168 and StatFS64.mntonname.offset == 88, "unsupported_mount_boundary_platform")
    try:
        libc = ctypes.CDLL("/usr/lib/libSystem.B.dylib", use_errno=True)
        # Darwin arm64 only exports the 64-bit inode ABI under the plain name.
        # x86_64 has legacy ABI aliases: require the explicit INODE64 symbol.
        function = getattr(libc, "getmntinfo" if architecture == "arm64" else "getmntinfo$INODE64")
        function.argtypes = [ctypes.POINTER(ctypes.POINTER(StatFS64)), ctypes.c_int]
        function.restype = ctypes.c_int
        entries = ctypes.POINTER(StatFS64)()
        count = function(ctypes.byref(entries), 2)  # MNT_NOWAIT; no volume I/O refresh.
        require(entries and 0 < count <= 65536, "mount_boundary_unavailable")
        points = set()
        for index in range(count):
            name = bytes(entries[index].mntonname)
            require(0 < len(name) < 1024, "invalid_mount_boundary_source")
            decoded = os.fsdecode(name)
            require(decoded.startswith("/"), "invalid_mount_boundary_source")
            points.add(Path(os.path.normpath(decoded)))
        require(Path("/") in points, "invalid_mount_boundary_source")
        return points
    except (OSError, AttributeError):
        raise Rejected("mount_boundary_unavailable") from None


def mountpoints():
    if sys.platform.startswith("linux"):
        try:
            with open("/proc/self/mountinfo", encoding="utf-8", errors="surrogateescape") as source:
                text = source.read(4 * 1024 * 1024 + 1)
            require(len(text) <= 4 * 1024 * 1024, "invalid_mount_boundary_source")
            return linux_mountpoints(text)
        except OSError:
            raise Rejected("mount_boundary_unavailable") from None
    if sys.platform == "darwin":
        return darwin_mountpoints()
    raise Rejected("unsupported_mount_boundary_platform")


def trusted_ancestry(root):
    """Owner/mode/identity preflight, not proof of ACLs or future stability.

    Caller must retain an exclusive lease for this stable checkout throughout
    validation. System-owned sticky temp ancestors are permitted; private root
    itself must be caller-owned and cannot be a link or writable by another UID.
    """
    root = Path(os.path.abspath(root))
    identities = []
    for ancestor in reversed((root, *root.parents)):
        info = ancestor.lstat()
        require(stat.S_ISDIR(info.st_mode) and not stat.S_ISLNK(info.st_mode), "untrusted_package_ancestry")
        require(info.st_uid in {0, os.getuid()}, "untrusted_package_owner")
        sticky_system = info.st_uid == 0 and bool(info.st_mode & stat.S_ISVTX)
        require(not info.st_mode & 0o022 or sticky_system, "untrusted_package_ancestry")
        identities.append((ancestor, info.st_dev, info.st_ino))
    require(root.lstat().st_uid == os.getuid(), "untrusted_package_owner")
    return root, identities


def check_tree_with_boundaries(root, *, known_mountpoints=None):
    root, identities = trusted_ancestry(root)
    boundaries = mountpoints() if known_mountpoints is None else known_mountpoints
    require(isinstance(boundaries, set) and all(isinstance(point, Path) and point.is_absolute() for point in boundaries), "invalid_mount_boundary_source")
    device = root.lstat().st_dev
    pending = [root]
    while pending:
        directory = pending.pop()
        # No child enumeration before that directory's own boundary check.
        info = directory.lstat()
        require(info.st_dev == device and (directory == root or directory not in boundaries), "package_mount_boundary")
        require(stat.S_ISDIR(info.st_mode) and not stat.S_ISLNK(info.st_mode), "package_symlink")
        for member in directory.iterdir():
            info = member.lstat()
            require(not stat.S_ISLNK(info.st_mode), "package_symlink")
            # Check files as well: Linux bind mounts can replace a single file.
            require(info.st_dev == device and member not in boundaries, "package_mount_boundary")
            require(stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode), "special_package_entry")
            require(not member.name.startswith("."), "hidden_package_entry")
            require(member.suffix.lower() not in {".fpk", ".db", ".sqlite", ".log", ".pem", ".key"}, "private_or_generated_entry")
            require(not info.st_mode & 0o022, "unsafe_package_mode")
            require(info.st_size <= 1024 * 1024, "oversized_package_entry")
            if stat.S_ISDIR(info.st_mode):
                pending.append(member)
    # Detect changed ancestry observed across this preflight; caller must still
    # prohibit later replacements/remounts until all document reads finish.
    for ancestor, expected_device, inode in identities:
        current = ancestor.lstat()
        require(current.st_dev == expected_device and current.st_ino == inode and stat.S_ISDIR(current.st_mode), "package_ancestry_changed")


def manifest(path):
    result = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.partition("=")
        key, value = key.strip(), value.strip()
        require(separator and key not in result, "invalid_manifest")
        result[key] = value
    expected = {"appname": APP, "version": "0.0.0-research", "source": "thirdparty",
                "platform": "x86", "desktop_uidir": "ui", "desktop_applaunchname": APP + ".main",
                "ctl_stop": "true", "checkport": "true", "service_port": "18080",
                "disable_authorization_path": "false"}
    require(set(result) == set(expected) | {"display_name", "maintainer", "maintainer_url", "distributor", "desc", "changelog"}, "unreviewed_manifest_field")
    for key, value in expected.items():
        require(result[key] == value, "unsafe_manifest")
    require(all(result.values()), "empty_manifest_value")
    return result


def check_png(path, dimension):
    data = path.read_bytes()
    require(24 <= len(data) <= 1024 * 1024 and data[:8] == b"\x89PNG\r\n\x1a\n", "invalid_icon")
    require(data[12:16] == b"IHDR" and struct.unpack(">II", data[16:24]) == (dimension, dimension), "wrong_icon_size")


def validate(root, release=False, receipt=None):
    check_tree(root)
    meta = manifest(root / "manifest")
    policy = document(root / "build-policy.json")
    require(policy == {"schema": 1, "implementation_status": "scaffold_only", "channel": "fnos-research",
                       "accepted_source_sha": "", "accepted_image_digest": "", "fnos_runtime_acceptance": "NOT_RUN"}, "unreviewed_build_policy")
    require(document(root / "config/privilege") == {"defaults": {"run-as": "package"}, "username": APP, "groupname": APP}, "unsafe_package_privilege")
    require(document(root / "config/resource") == {"docker-project": {"projects": [{"name": APP, "path": "docker"}]}}, "unsafe_resources")
    ui = document(root / "app/ui/config")
    require(ui == {".url": {APP + ".main": {"title": "NDG Research (Not Installable)", "icon": "images/icon_{0}.png", "type": "url", "protocol": "http", "port": meta["service_port"], "url": "/", "allUsers": False, "control": {"accessPerm": "readonly"}}}}, "unsafe_ui_entry")
    config = document(root / "app/docker/docker-compose.yaml")
    require(set(config) == {"services"} and set(config["services"]) == {"ndg"}, "unsafe_compose_topology")
    service = config["services"]["ndg"]
    keys = {"image", "container_name", "restart", "user", "read_only", "cap_drop", "security_opt", "ports", "tmpfs", "pids_limit", "mem_limit", "cpus", "stop_grace_period", "labels", "volumes", "environment", "healthcheck"}
    require(set(service) == keys, "unreviewed_compose_field")
    image = service["image"]
    require(isinstance(image, str) and (image == "" or DIGEST.fullmatch(image)), "unfrozen_image")
    require(service["container_name"] == APP + "-web" and service["restart"] == "unless-stopped", "unsafe_container_identity")
    require(service["user"] == USER and service["read_only"] is True and service["cap_drop"] == ["ALL"] and service["security_opt"] == ["no-new-privileges:true"], "unsafe_container_privilege")
    require(service["ports"] == ["127.0.0.1:18080:8080"], "public_or_unreviewed_listener")
    require(service["volumes"] == [{"type": "bind", "source": STATE, "target": "/data", "read_only": False}, {"type": "bind", "source": SOURCE, "target": "/sources/source", "read_only": True}], "unsafe_volumes")
    require(service["environment"] == {"NDG_WRITE_POLICY": "disabled"}, "unsafe_environment")
    require(service["labels"] == {"com.ndg.fnos.app": APP, "com.ndg.fnos.channel": "fnos-research"}, "unsafe_labels")
    require(service["healthcheck"] == {"test": ["CMD", "/usr/local/bin/ndg-server", "healthcheck"], "interval": "10s", "timeout": "3s", "retries": 3}, "unreviewed_healthcheck")
    require(service["tmpfs"] == ["/tmp:rw,nosuid,nodev,noexec,size=128m"] and service["pids_limit"] == 128 and service["mem_limit"] == "512m" and service["cpus"] == "1.0" and service["stop_grace_period"] == "60s", "unsafe_runtime_limits")
    for name, size in [("ICON.PNG", 64), ("ICON_256.PNG", 256), ("app/ui/images/icon_64.png", 64), ("app/ui/images/icon_256.png", 256)]:
        check_png(root / name, size)
    for name in ["install", "uninstall"]:
        wizard = document(root / "wizard" / name)
        require(isinstance(wizard, list) and len(wizard) == 1 and set(wizard[0]) == {"stepTitle", "items"}, "invalid_wizard")
        require(all(set(item) == {"type", "helpText"} and item["type"] == "tips" for item in wizard[0]["items"]), "unreviewed_wizard_input")
    for name in ["main", "install_init", "install_callback", "upgrade_init", "upgrade_callback", "config_init", "config_callback", "uninstall_init", "uninstall_callback"]:
        file = root / "cmd" / name
        require(file.is_file() and file.stat().st_mode & 0o111, "missing_lifecycle_script")
    if release:
        require(DIGEST.fullmatch(image or ""), "accepted_oci_digest_required")
        require(receipt is not None, "acceptance_receipt_required")
        require(set(receipt) == {"schema", "source_sha", "image_digest", "docker_acceptance", "fnos_acceptance", "architectures"}, "invalid_acceptance_receipt")
        require(receipt["schema"] == 1 and isinstance(receipt["source_sha"], str) and SHA.fullmatch(receipt["source_sha"]), "invalid_receipt_source")
        require(receipt["image_digest"] == image and receipt["docker_acceptance"] == "PASS" and receipt["fnos_acceptance"] == "PASS" and receipt["architectures"] == ["linux/amd64"], "acceptance_not_complete")
        # This is an explicit hard stop, not a runtime certification based on an
        # editable JSON receipt. Installation, export, upgrade and proxy/auth
        # integration require a separately reviewed implementation and device evidence.
        raise Rejected("fnos_runtime_integration_not_implemented")
    return "research_structure_pass_not_installable"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2] / "packaging/fnos" / APP)
    parser.add_argument("--release", action="store_true")
    parser.add_argument("--receipt", type=Path)
    args = parser.parse_args()
    try:
        receipt = document(args.receipt) if args.receipt else None
        result = validate(args.root, args.release, receipt)
    except (Rejected, OSError, TypeError, KeyError, AttributeError, ValueError) as error:
        code = str(error) if isinstance(error, Rejected) else "malformed_package_input"
        print("fnos_check: " + code, file=sys.stderr)
        return 1
    print("fnos_check: " + result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
