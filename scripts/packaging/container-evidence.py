#!/usr/bin/env python3
"""OCI content/identity verification. Unsigned receipts are not attestations."""
import argparse
import gzip
import hashlib
import importlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import stat
import sys
import tarfile
from urllib.parse import unquote

sys.dont_write_bytecode = True
boundary = importlib.import_module("check-fnos-package")
SHA = re.compile(r"^[0-9a-f]{64}$")
SOURCE_SHA = re.compile(r"^[0-9a-f]{40}$")
MAX_BLOB = 128 * 1024 * 1024
MAX_LAYER = 512 * 1024 * 1024
ARTIFACTS = {"image.oci.tar", "image.cyclonedx.json", "image.spdx.json", "source.cyclonedx.json", "source.spdx.json", "npm-audit.json", "govulncheck.json", "grype.json", "runtime.json", "tools.json"}


class Rejected(Exception):
    pass


def require(value, code):
    if not value:
        raise Rejected(code)


def digest(value):
    return hashlib.sha256(value).hexdigest()


def json_value(data):
    def unique(pairs):
        obj = {}
        for key, value in pairs:
            require(key not in obj, "duplicate_json_key")
            obj[key] = value
        return obj
    try:
        return json.loads(data, object_pairs_hook=unique)
    except (ValueError, UnicodeError):
        raise Rejected("invalid_json") from None


def regular_input(path, maximum):
    # Callers must retain stable owner/ACL/lease for the entire read. This checks
    # current ownership/modes/links and mount table, not hostile remount races.
    # Lexical normalization aligns relative caller inputs with absolute mount
    # table entries. Do not resolve: symlink inputs must remain visible/refused.
    path = Path(os.path.abspath(path))
    root, _ = boundary.trusted_ancestry(path.parent)
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and info.st_uid == os.getuid() and not info.st_mode & 0o022, "unsafe_evidence_file")
    require(info.st_dev == root.lstat().st_dev and path not in boundary.mountpoints(), "evidence_mount_boundary")
    require(info.st_size <= maximum, "evidence_size_limit")
    return path


def image_identity(archive, source, architecture):
    require(architecture in {"amd64", "arm64"} and SOURCE_SHA.fullmatch(source["commit"]) and SOURCE_SHA.fullmatch(source["tree"]) and source["channel"] == "web-experimental", "invalid_expected_identity")
    regular_input(archive, 2 * 1024 * 1024 * 1024)
    with tarfile.open(archive, "r:") as outer:
        members = {}
        for entry in outer:
            name = entry.name.removeprefix("./").rstrip("/")
            if entry.isdir():
                require(name in {"", ".", "blobs", "blobs/sha256"}, "unsafe_oci_directory")
                continue
            require(entry.isfile() and name not in members and entry.size <= MAX_BLOB, "unsafe_oci_entry")
            require(name in {"index.json", "oci-layout", "manifest.json"} or re.fullmatch(r"blobs/sha256/[0-9a-f]{64}", name), "unsafe_oci_entry")
            members[name] = entry
            require(len(members) <= 4096, "oci_entry_limit")

        def read(name):
            require(name in members, "missing_oci_blob")
            stream = outer.extractfile(members[name])
            require(stream is not None, "missing_oci_blob")
            with stream:
                value = stream.read(MAX_BLOB + 1)
            require(len(value) <= MAX_BLOB, "oci_blob_limit")
            return value

        def blob(descriptor, media):
            identity = descriptor.get("digest", "")
            require(identity.startswith("sha256:") and SHA.fullmatch(identity[7:]) and descriptor.get("mediaType") == media and type(descriptor.get("size")) is int, "invalid_descriptor")
            value = read("blobs/sha256/" + identity[7:])
            require(len(value) == descriptor["size"] and digest(value) == identity[7:], "oci_blob_mismatch")
            return value

        require(json_value(read("oci-layout")) == {"imageLayoutVersion": "1.0.0"}, "unsupported_oci_layout")
        index_bytes = read("index.json")
        index = json_value(index_bytes)
        require(index.get("schemaVersion") == 2 and len(index.get("manifests", [])) == 1, "unsupported_oci_index")
        manifest_desc = index["manifests"][0]
        manifest = json_value(blob(manifest_desc, "application/vnd.oci.image.manifest.v1+json"))
        require(manifest.get("schemaVersion") == 2 and 1 <= len(manifest.get("layers", [])) <= 16, "unsupported_oci_manifest")
        config = json_value(blob(manifest["config"], "application/vnd.oci.image.config.v1+json"))
        require(config.get("os") == "linux" and config.get("architecture") == architecture, "oci_architecture_mismatch")
        runtime = config["config"]
        require(runtime.get("User") == "10001:10001" and runtime.get("Entrypoint") == ["/ndg-server", "-config", "/config/server.json"], "unsafe_image_configuration")
        labels = runtime.get("Labels", {})
        expected = {"org.opencontainers.image.revision": source["commit"], "com.ndg.source-tree": source["tree"], "org.opencontainers.image.version": source["core_version"], "com.ndg.build-time": source["build_time"], "com.ndg.channel": "web-experimental", "org.opencontainers.image.source": "https://github.com/FNB2026/nas-data-governance"}
        require(all(labels.get(key) == value for key, value in expected.items()), "oci_source_identity_mismatch")
        rootfs = config.get("rootfs", {})
        require(rootfs.get("type") == "layers" and len(rootfs.get("diff_ids", [])) == len(manifest["layers"]), "invalid_rootfs")
        layers = []
        allowed = {"ndg-server", "licenses", "licenses/LICENSE", "licenses/NOTICE", "licenses/THIRD_PARTY_NOTICES.md"}
        observed_files = set()
        for descriptor, diff_id in zip(manifest["layers"], rootfs["diff_ids"]):
            media = descriptor.get("mediaType")
            require(media in {"application/vnd.oci.image.layer.v1.tar+gzip", "application/vnd.oci.image.layer.v1.tar"}, "unsupported_layer_encoding")
            encoded = blob(descriptor, media)
            stream = gzip.GzipFile(fileobj=io.BytesIO(encoded)) if media.endswith("+gzip") else io.BytesIO(encoded)
            with stream:
                data = stream.read(MAX_LAYER + 1)
            require(len(data) <= MAX_LAYER and diff_id == "sha256:" + digest(data), "layer_diff_id_mismatch")
            with tarfile.open(fileobj=io.BytesIO(data), mode="r:") as layer:
                for entry in layer:
                    name = entry.name.removeprefix("./").rstrip("/")
                    require((name in allowed or (entry.isdir() and name in {"", "."})) and (entry.isdir() or entry.isfile()) and not PurePosixPath(name).is_absolute(), "unexpected_image_payload")
                    if entry.isfile():
                        require(name not in observed_files, "duplicate_image_payload")
                        observed_files.add(name)
            layers.append({"digest": descriptor["digest"], "size": descriptor["size"], "diff_id": diff_id})
        require("ndg-server" in observed_files, "missing_server_payload")
        # Reject unreferenced blobs too; OCI permits extensions, but this narrow
        # evidence format intentionally permits only the one reviewed image.
        referenced = {manifest_desc["digest"], manifest["config"]["digest"], *(item["digest"] for item in layers)}
        require({"sha256:" + name.rsplit("/", 1)[1] for name in members if name.startswith("blobs/")} == referenced, "unreferenced_oci_blob")
    hasher = hashlib.sha256()
    with archive.open("rb") as source_file:
        for chunk in iter(lambda: source_file.read(1024 * 1024), b""):
            hasher.update(chunk)
    return {"archive_sha256": hasher.hexdigest(), "index_sha256": digest(index_bytes), "manifest_digest": manifest_desc["digest"], "config_digest": manifest["config"]["digest"], "layers": layers}


def verify_frontend_sbom_inventory(root, frontend_lock):
    """Bind both source formats to every lock entry, including dev/optional deps.

    This verifies known source inventory; an unsigned receipt is not provenance.
    Caller supplies the lock from the independently verified source checkout.
    """
    raw = regular_input(Path(frontend_lock), MAX_BLOB).read_bytes()
    lock = json_value(raw)
    require(lock.get("lockfileVersion") == 3 and isinstance(lock.get("packages"), dict), "unsupported_frontend_lock")
    expected = set()
    entries, dev_entries = 0, 0
    for path, item in lock["packages"].items():
        if path == "":
            continue
        require(isinstance(path, str) and path.startswith("node_modules/") and isinstance(item, dict), "invalid_frontend_lock_entry")
        name = item.get("name") or path.rsplit("node_modules/", 1)[1]
        version = item.get("version")
        require(isinstance(name, str) and re.fullmatch(r"(?:@[A-Za-z0-9_.-]+/)?[A-Za-z0-9_.-]+", name) and isinstance(version, str) and len(version) <= 128 and bool(version), "invalid_frontend_lock_identity")
        expected.add((name, version))
        entries += 1
        dev_entries += item.get("dev") is True
    require(bool(expected), "empty_frontend_lock_inventory")
    for name, field in [("source.cyclonedx.json", "components"), ("source.spdx.json", "packages")]:
        data = json_value(regular_input(Path(root) / name, MAX_BLOB).read_bytes())
        require(isinstance(data.get(field), list), "invalid_source_sbom")
        observed = set()
        for item in data[field]:
            require(isinstance(item, dict), "invalid_source_sbom")
            purls = [item.get("purl")] if field == "components" else [ref.get("referenceLocator") for ref in item.get("externalRefs", []) if isinstance(ref, dict) and ref.get("referenceType") == "purl"]
            for purl in purls:
                if not isinstance(purl, str) or not purl.startswith("pkg:npm/"):
                    continue
                package, separator, version = purl[8:].split("?", 1)[0].split("#", 1)[0].rpartition("@")
                require(bool(separator) and bool(package) and bool(version), "invalid_npm_purl")
                observed.add((unquote(package), unquote(version)))
        require(expected <= observed, "incomplete_frontend_sbom_inventory")
    return {"scope": "frontend_lock_all_entries_including_dev_and_optional", "lock_sha256": digest(raw), "lock_entries": entries, "dev_entries": dev_entries, "unique_packages": len(expected), "status": "PASS"}


def verify_bundle(root, *, expected_commit=None, expected_tree=None, expected_architecture=None, expected_frontend_lock=None):
    root = Path(os.path.abspath(root))
    boundary.trusted_ancestry(root)
    names = {entry.name for entry in root.iterdir()}
    require(names == ARTIFACTS | {"receipt.json"}, "unreviewed_evidence_file")
    receipt = json_value(regular_input(root / "receipt.json", 1024 * 1024).read_bytes())
    require(receipt.get("schema") == 1 and receipt.get("scope") == "experimental_ci_evidence_only", "invalid_receipt_scope")
    require(expected_commit is None or receipt.get("source", {}).get("commit") == expected_commit, "receipt_commit_mismatch")
    require(expected_tree is None or receipt.get("source", {}).get("tree") == expected_tree, "receipt_tree_mismatch")
    require(expected_architecture is None or receipt.get("architecture") == expected_architecture, "receipt_architecture_mismatch")
    require(receipt.get("signed_attestation") == "NOT_SIGNED" and receipt.get("reproducibility") == "NOT_RUN", "unsupported_provenance_claim")
    require(set(receipt.get("files", {})) == ARTIFACTS, "incomplete_artifact_receipt")
    for name, sha in receipt["files"].items():
        require(SHA.fullmatch(sha), "invalid_artifact_digest")
        member = regular_input(root / name, 2 * 1024 * 1024 * 1024 if name == "image.oci.tar" else MAX_BLOB)
        hasher = hashlib.sha256()
        with member.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                hasher.update(chunk)
        require(hasher.hexdigest() == sha, "artifact_digest_mismatch")
    identity = image_identity(root / "image.oci.tar", receipt["source"], receipt["architecture"])
    require(identity == receipt["image"], "receipt_image_mismatch")
    runtime = json_value((root / "runtime.json").read_bytes())
    require(runtime == {"status": "PASS", "loaded_config_digest": identity["config_digest"], "native_machine": {"amd64": "x86_64", "arm64": "aarch64"}[receipt["architecture"]]}, "runtime_identity_mismatch")
    for name, field in [("image.cyclonedx.json", "components"), ("image.spdx.json", "packages"), ("source.cyclonedx.json", "components"), ("source.spdx.json", "packages")]:
        require(bool(json_value((root / name).read_bytes()).get(field)), "empty_or_invalid_sbom")
    require(set(receipt.get("scans", {})) == {"npm", "govulncheck", "grype"}, "missing_scan_receipt")
    npm = json_value((root / "npm-audit.json").read_bytes())
    counts = npm.get("metadata", {}).get("vulnerabilities", {})
    require(type(counts.get("high")) is int and type(counts.get("critical")) is int, "missing_npm_severity_evidence")
    govuln = json_value((root / "govulncheck.json").read_bytes())
    require(isinstance(govuln, list) and govuln and any("config" in item for item in govuln), "missing_go_scan_evidence")
    grype = json_value((root / "grype.json").read_bytes())
    require(isinstance(grype.get("matches"), list) and bool(grype.get("descriptor", {}).get("db")), "missing_image_scan_evidence")
    for scan in receipt["scans"].values():
        require(scan.get("status") in {"PASS", "BLOCKED"} and type(scan.get("exit_code")) is int, "invalid_scan_receipt")
    if receipt["scans"]["npm"]["status"] == "PASS":
        require(receipt["scans"]["npm"]["exit_code"] == 0 and counts["high"] == counts["critical"] == 0, "false_npm_pass")
    if receipt["scans"]["govulncheck"]["status"] == "PASS":
        require(receipt["scans"]["govulncheck"]["exit_code"] == 0 and not any("finding" in item for item in govuln), "false_go_scan_pass")
    if receipt["scans"]["grype"]["status"] == "PASS":
        require(receipt["scans"]["grype"]["exit_code"] == 0 and not any(str(item.get("vulnerability", {}).get("severity", "")).lower() in {"high", "critical"} for item in grype["matches"]), "false_image_scan_pass")
    if expected_frontend_lock is not None:
        inventory = verify_frontend_sbom_inventory(root, expected_frontend_lock)
        require(receipt.get("source_sbom_inventory") == inventory, "source_sbom_inventory_receipt_mismatch")
    return receipt


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("bundle", type=Path)
    parser.add_argument("--expected-commit")
    parser.add_argument("--expected-tree")
    parser.add_argument("--expected-architecture", choices=["amd64", "arm64"])
    parser.add_argument("--frontend-lock", type=Path, help="lock from independently verified source checkout; required to certify complete frontend inventory")
    args = parser.parse_args()
    try:
        verify_bundle(args.bundle, expected_commit=args.expected_commit, expected_tree=args.expected_tree, expected_architecture=args.expected_architecture, expected_frontend_lock=args.frontend_lock)
    except Exception as error:
        print("container-evidence: " + (str(error) if isinstance(error, Rejected) else "invalid_or_unreadable_evidence"), file=sys.stderr)
        return 1
    print("container-evidence: content_identity_verified_unsigned_not_release")
    return 0


if __name__ == "__main__":
    sys.exit(main())
