#!/usr/bin/env python3
"""Synthetic OCI corruption tests, not Docker or native-runtime acceptance."""
import gzip
import hashlib
import importlib
import io
import json
from pathlib import Path
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
evidence = importlib.import_module("container-evidence")
collector = importlib.import_module("collect-container-evidence")
SOURCE = {"commit": "a" * 40, "tree": "b" * 40, "core_version": "0.0.0-synthetic", "build_time": "2026-01-01T00:00:00Z", "channel": "web-experimental"}


def encoded(value):
    return json.dumps(value, sort_keys=True).encode()


def tar_bytes(entries):
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode="w") as tar:
        for name, value in entries.items():
            entry = tarfile.TarInfo(name)
            entry.size = len(value)
            tar.addfile(entry, io.BytesIO(value))
    return stream.getvalue()


def make_image(path, *, architecture="amd64", payload="ndg-server", corrupt_blob=False, bad_diff=False, bad_descriptor=False, link=False):
    layer = tar_bytes({payload: b"synthetic not executable"})
    packed = gzip.compress(layer, mtime=0)
    config = {"architecture": architecture, "os": "linux", "config": {"User": "10001:10001", "Entrypoint": ["/ndg-server", "-config", "/config/server.json"], "Labels": {"org.opencontainers.image.revision": SOURCE["commit"], "com.ndg.source-tree": SOURCE["tree"], "org.opencontainers.image.version": SOURCE["core_version"], "com.ndg.build-time": SOURCE["build_time"], "com.ndg.channel": SOURCE["channel"], "org.opencontainers.image.source": "https://github.com/FNB2026/nas-data-governance"}}, "rootfs": {"type": "layers", "diff_ids": ["sha256:" + ("0" * 64 if bad_diff else evidence.digest(layer))]}}
    config_data = encoded(config)

    def descriptor(data, kind):
        return {"mediaType": "application/vnd.oci.image." + kind, "digest": "sha256:" + evidence.digest(data), "size": len(data)}

    config_desc = descriptor(config_data, "config.v1+json")
    manifest = {"schemaVersion": 2, "config": config_desc, "layers": [descriptor(packed, "layer.v1.tar+gzip")]}
    if bad_descriptor:
        manifest["config"]["size"] += 1
    manifest_data = encoded(manifest)
    entries = {"oci-layout": encoded({"imageLayoutVersion": "1.0.0"}), "index.json": encoded({"schemaVersion": 2, "manifests": [descriptor(manifest_data, "manifest.v1+json")]}), "blobs/sha256/" + evidence.digest(config_data): config_data, "blobs/sha256/" + evidence.digest(manifest_data): manifest_data, "blobs/sha256/" + evidence.digest(packed): packed + (b"damage" if corrupt_blob else b"")}
    path.write_bytes(tar_bytes(entries))
    if link:
        with tarfile.open(path, "a") as tar:
            item = tarfile.TarInfo("outside-link")
            item.type = tarfile.SYMTYPE
            item.linkname = "../../synthetic-outside"
            tar.addfile(item)


class EvidenceChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ndg-oci-offline-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.archive = self.root / "image.oci.tar"
        make_image(self.archive)

    def test_valid_synthetic_graph_not_native_pass(self):
        result = evidence.image_identity(self.archive, SOURCE, "amd64")
        self.assertEqual(result["archive_sha256"], hashlib.sha256(self.archive.read_bytes()).hexdigest())
        self.assertEqual(len(result["layers"]), 1)

    def test_damaged_blob_rejected(self):
        make_image(self.archive, corrupt_blob=True)
        with self.assertRaisesRegex(evidence.Rejected, "^oci_blob_mismatch$"):
            evidence.image_identity(self.archive, SOURCE, "amd64")

    def test_wrong_descriptor_size_rejected(self):
        make_image(self.archive, bad_descriptor=True)
        with self.assertRaisesRegex(evidence.Rejected, "^oci_blob_mismatch$"):
            evidence.image_identity(self.archive, SOURCE, "amd64")

    def test_wrong_diff_id_rejected(self):
        make_image(self.archive, bad_diff=True)
        with self.assertRaisesRegex(evidence.Rejected, "^layer_diff_id_mismatch$"):
            evidence.image_identity(self.archive, SOURCE, "amd64")

    def test_source_or_architecture_mismatch_rejected(self):
        with self.assertRaisesRegex(evidence.Rejected, "^oci_architecture_mismatch$"):
            evidence.image_identity(self.archive, SOURCE, "arm64")
        changed = dict(SOURCE, tree="c" * 40)
        with self.assertRaisesRegex(evidence.Rejected, "^oci_source_identity_mismatch$"):
            evidence.image_identity(self.archive, changed, "amd64")

    def test_tar_link_and_unreviewed_payload_rejected(self):
        make_image(self.archive, link=True)
        with self.assertRaisesRegex(evidence.Rejected, "^unsafe_oci_entry$"):
            evidence.image_identity(self.archive, SOURCE, "amd64")
        for payload in ["../../synthetic-outside", "/absolute", "config/credential", ".env"]:
            with self.subTest(payload=payload):
                make_image(self.archive, payload=payload)
                with self.assertRaisesRegex(evidence.Rejected, "^unexpected_image_payload$"):
                    evidence.image_identity(self.archive, SOURCE, "amd64")

    def bundle(self):
        identity = evidence.image_identity(self.archive, SOURCE, "amd64")
        documents = {"image.cyclonedx.json": {"components": [{"name": "synthetic"}]}, "source.cyclonedx.json": {"components": [{"name": "synthetic"}]}, "image.spdx.json": {"packages": [{"name": "synthetic"}]}, "source.spdx.json": {"packages": [{"name": "synthetic"}]}, "npm-audit.json": {"metadata": {"vulnerabilities": {"high": 0, "critical": 0}}}, "govulncheck.json": [{"config": {"scanner_version": "synthetic"}}], "grype.json": {"matches": [], "descriptor": {"db": {"built": "synthetic"}}}, "runtime.json": {"status": "PASS", "loaded_config_digest": identity["config_digest"], "native_machine": "x86_64"}, "tools.json": {"fixture": "synthetic; not publisher/native evidence"}}
        for name, value in documents.items():
            (self.root / name).write_bytes(encoded(value))
        self.receipt = {"schema": 1, "scope": "experimental_ci_evidence_only", "source": SOURCE, "architecture": "amd64", "image": identity, "signed_attestation": "NOT_SIGNED", "reproducibility": "NOT_RUN", "scans": {name: {"exit_code": 0, "status": "PASS"} for name in ["npm", "govulncheck", "grype"]}, "files": {name: evidence.digest((self.root / name).read_bytes()) for name in evidence.ARTIFACTS}}
        self.seal()

    def seal(self):
        self.receipt["files"] = {name: evidence.digest((self.root / name).read_bytes()) for name in evidence.ARTIFACTS}
        (self.root / "receipt.json").write_bytes(encoded(self.receipt))

    def test_bundle_hashes_and_loaded_image_match(self):
        self.bundle()
        evidence.verify_bundle(self.root)
        runtime = json.loads((self.root / "runtime.json").read_bytes())
        runtime["loaded_config_digest"] = "sha256:" + "0" * 64
        (self.root / "runtime.json").write_bytes(encoded(runtime))
        self.seal()
        with self.assertRaisesRegex(evidence.Rejected, "^runtime_identity_mismatch$"):
            evidence.verify_bundle(self.root)

    def test_sbom_mutation_rejected(self):
        self.bundle()
        (self.root / "image.spdx.json").write_text("damaged")
        with self.assertRaisesRegex(evidence.Rejected, "^artifact_digest_mismatch$"):
            evidence.verify_bundle(self.root)

    def test_false_provenance_claim_rejected(self):
        self.bundle()
        self.receipt["signed_attestation"] = "SIGNED"
        self.seal()
        with self.assertRaisesRegex(evidence.Rejected, "^unsupported_provenance_claim$"):
            evidence.verify_bundle(self.root)

    def test_external_source_identity_required_when_supplied(self):
        self.bundle()
        with self.assertRaisesRegex(evidence.Rejected, "^receipt_commit_mismatch$"):
            evidence.verify_bundle(self.root, expected_commit="c" * 40)
        with self.assertRaisesRegex(evidence.Rejected, "^receipt_tree_mismatch$"):
            evidence.verify_bundle(self.root, expected_tree="c" * 40)
        with self.assertRaisesRegex(evidence.Rejected, "^receipt_architecture_mismatch$"):
            evidence.verify_bundle(self.root, expected_architecture="arm64")

    def test_forged_clear_vulnerability_status_rejected(self):
        self.bundle()
        for name, data, rejection in [("npm-audit.json", {"metadata": {"vulnerabilities": {"high": 1, "critical": 0}}}, "false_npm_pass"), ("govulncheck.json", [{"config": {}}, {"finding": {"osv": "SYNTHETIC-CVE"}}], "false_go_scan_pass"), ("grype.json", {"matches": [{"vulnerability": {"severity": "High"}}], "descriptor": {"db": {"built": "synthetic"}}}, "false_image_scan_pass")]:
            with self.subTest(name=name):
                self.bundle()
                (self.root / name).write_bytes(encoded(data))
                self.seal()
                with self.assertRaisesRegex(evidence.Rejected, "^" + rejection + "$"):
                    evidence.verify_bundle(self.root)

    def test_symlink_receipt_and_raw_logs_rejected(self):
        self.bundle()
        (self.root / "private.stdout").write_text("synthetic-sensitive-marker")
        with self.assertRaisesRegex(evidence.Rejected, "^unreviewed_evidence_file$"):
            evidence.verify_bundle(self.root)
        (self.root / "private.stdout").unlink()
        (self.root / "receipt.json").unlink()
        (self.root / "receipt.json").symlink_to(self.archive)
        with self.assertRaisesRegex(evidence.Rejected, "^unsafe_evidence_file$"):
            evidence.verify_bundle(self.root)

    def test_offline_collector_cannot_touch_local_docker(self):
        result = subprocess.run([sys.executable, str(Path(collector.__file__))], capture_output=True, text=True, env={"PATH": "/usr/bin:/bin"}, timeout=5)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertIn("runner_preflight_rejected", result.stderr)

    def test_sanitized_json_preserves_findings_and_rejects_secrets(self):
        target = self.root / "sanitized.json"
        value = {"source": str(collector.ROOT) + "/file.go", "cache": "/synthetic/absolute/path", "vulnerability": {"severity": "High", "id": "SYNTHETIC-CVE"}}
        collector.public_json(target, value, self.root / "private")
        text = target.read_text()
        self.assertNotIn(str(collector.ROOT), text)
        self.assertNotIn("/synthetic/absolute/path", text)
        self.assertEqual(json.loads(text)["vulnerability"]["severity"], "High")
        with mock.patch.dict(os.environ, {"SYNTHETIC_PASSWORD": "synthetic_secret_marker_12345"}):
            with self.assertRaisesRegex(evidence.Rejected, "^secret_in_tool_output$"):
                collector.public_json(target, {"value": "synthetic_secret_marker_12345"}, self.root / "private")
        with self.assertRaisesRegex(evidence.Rejected, "^redaction_key_collision$"):
            collector.public_json(target, {"/first": 1, "/second": 2}, self.root / "private")

    def test_workflow_dispatch_only_read_permission_and_fixed_actions(self):
        workflow = json.loads((Path(__file__).resolve().parents[2] / ".github/workflows/container-evidence.yml").read_text())
        self.assertEqual(workflow["on"], {"workflow_dispatch": {}})
        self.assertEqual(workflow["permissions"], {"contents": "read"})
        job = workflow["jobs"]["native-evidence"]
        self.assertEqual({row["runner"] for row in job["strategy"]["matrix"]["include"]}, {"ubuntu-24.04", "ubuntu-24.04-arm"})
        for step in job["steps"]:
            if "uses" in step:
                self.assertRegex(step["uses"], r"@[0-9a-f]{40}$")
        uploader = job["steps"][-1]
        self.assertTrue(uploader["with"]["path"].endswith("/public/"))


if __name__ == "__main__":
    unittest.main()
