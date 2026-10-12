#!/usr/bin/env python3
"""Offline, stdlib-only research-package checks; never contacts a daemon.

The Compose document deliberately uses the JSON subset of YAML. This checker
does not claim fnOS runtime, receipt authenticity, or fnpack equivalence.
"""
import argparse
import json
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
    # The checkout itself is a trusted stable caller-owned research input. No
    # security claim is made for roots concurrently replaced by another writer.
    require(root.is_dir() and not root.is_symlink(), "invalid_package_root")
    for member in root.rglob("*"):
        info = member.lstat()
        require(not stat.S_ISLNK(info.st_mode), "package_symlink")
        require(stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode), "special_package_entry")
        require(not member.name.startswith("."), "hidden_package_entry")
        require(member.suffix.lower() not in {".fpk", ".db", ".sqlite", ".log", ".pem", ".key"}, "private_or_generated_entry")
        require(not info.st_mode & 0o022, "unsafe_package_mode")
        require(info.st_size <= 1024 * 1024, "oversized_package_entry")


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
