#!/usr/bin/env python3
"""Offline synthetic bytes and mocked Docker only; not NAS runtime evidence."""
import contextlib
import copy
import hashlib
import importlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
import urllib.error
from unittest import mock

sys.dont_write_bytecode = True
smoke = importlib.import_module("nas-disposable-smoke")
fixtures = importlib.import_module("test-container-evidence")


class FakeDocker:
    def __init__(self, nonce, image):
        self.calls = []
        self.network = "1" * 64
        self.container = "2" * 64
        self.values = {
            ("network", self.network): [{"Id": self.network, "Labels": {smoke.LABEL: nonce}, "Internal": True, "Driver": "bridge", "Containers": {}}],
            ("container", self.container): [{"Id": self.container, "Config": {"Labels": {smoke.LABEL: nonce}}, "Image": image}],
            ("image", image): [{"Id": image, "RepoTags": ["ndg-smoke-local:" + nonce]}],
        }

    def value(self, args):
        self.calls.append(args)
        return copy.deepcopy(self.values[(args[0], args[2])])

    def call(self, args, **kwargs):
        self.calls.append(args)
        output = self.network if args[:2] == ["network", "create"] else self.container if args[:2] == ["container", "create"] else ""
        return subprocess.CompletedProcess(args, 0, output.encode(), b"")


class SmokeChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ndg-smoke-offline-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        (self.root / "private").mkdir(mode=0o700)
        self.nonce = "a" * 32
        self.image = "sha256:" + "b" * 64
        self.docker = FakeDocker(self.nonce, self.image)
        self.resources = smoke.Resources(self.docker, self.nonce, self.image, self.root)
        container = self.docker.values[("container", self.docker.container)][0]
        container["HostConfig"] = {"NetworkMode": self.docker.network, "ReadonlyRootfs": True, "Privileged": False, "Memory": 384 * 1024 * 1024, "NanoCpus": 1_000_000_000, "PidsLimit": 128, "CapDrop": ["ALL"]}
        container["Mounts"] = [{"Type": "bind", "Source": str(self.root / name), "Destination": target, "RW": rw} for name, target, rw in [("source", "/sources/source_a", False), ("config", "/config", False), ("state", "/data", True)]]

    def synthetic_bundle(self):
        case = fixtures.EvidenceChecks()
        case.setUp()
        self.addCleanup(case.doCleanups)
        case.bundle()
        identity = case.receipt["image"]
        expected = {"commit": fixtures.SOURCE["commit"], "tree": fixtures.SOURCE["tree"], "archive_sha256": identity["archive_sha256"], "config_digest": identity["config_digest"]}
        return case, expected

    def test_default_protocol_never_calls_docker_or_creates_files(self):
        with mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("Docker forbidden")), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("write forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--bundle", "/synthetic-does-not-exist"]), 0)
        self.assertEqual(output.getvalue(), "nas-smoke: protocol_only_no_docker_no_files_no_credentials\n")

    def test_frozen_conversion_preserves_config_and_uncompressed_layers(self):
        case, expected = self.synthetic_bundle()
        target = self.root / "docker.tar"
        receipt, sha = smoke.docker_archive(case.root, expected, target, "ndg-smoke-local:" + self.nonce)
        self.assertEqual(sha, hashlib.sha256(target.read_bytes()).hexdigest())
        with tarfile.open(target) as archive:
            manifest = json.load(archive.extractfile("manifest.json"))[0]
            self.assertEqual(manifest["RepoTags"], ["ndg-smoke-local:" + self.nonce])
            config = archive.extractfile(manifest["Config"]).read()
            self.assertEqual("sha256:" + hashlib.sha256(config).hexdigest(), expected["config_digest"])
            self.assertEqual(set(archive.getnames()), {"manifest.json", manifest["Config"], *manifest["Layers"]})
            for name, layer in zip(manifest["Layers"], receipt["image"]["layers"]):
                self.assertEqual("sha256:" + hashlib.sha256(archive.extractfile(name).read()).hexdigest(), layer["diff_id"])

    def test_wrong_artifact_scope_or_existing_destination_rejects_before_write(self):
        case, expected = self.synthetic_bundle()
        target = self.root / "docker.tar"
        for update in [{"commit": "c" * 40}, {"tree": "c" * 40}, {"archive_sha256": "c" * 64}, {"config_digest": "sha256:" + "c" * 64}]:
            with self.subTest(update=update), self.assertRaises(Exception):
                smoke.docker_archive(case.root, dict(expected, **update), target, "ndg-smoke-local:" + self.nonce)
            self.assertFalse(target.exists())
        target.write_bytes(b"independent existing bytes")
        with self.assertRaises(FileExistsError):
            smoke.docker_archive(case.root, expected, target, "ndg-smoke-local:" + self.nonce)
        self.assertEqual(target.read_bytes(), b"independent existing bytes")

    def test_archive_symlink_and_unreviewed_payload_rejected(self):
        case, expected = self.synthetic_bundle()
        case.archive.unlink()
        case.archive.symlink_to(self.root / "outside")
        with self.assertRaises(Exception):
            smoke.docker_archive(case.root, expected, self.root / "target", "ndg-smoke-local:" + self.nonce)
        self.assertFalse((self.root / "target").exists())

    def test_no_existing_tag_or_invalid_nonce_reference(self):
        case, expected = self.synthetic_bundle()
        with self.assertRaisesRegex(smoke.Blocked, "invalid_private_tag"):
            smoke.docker_archive(case.root, expected, self.root / "target", "ndg-disposable:test")
        self.assertFalse((self.root / "target").exists())

    def test_fixture_is_seven_disposable_files_and_private_credentials(self):
        settings, password, marker = smoke.fixture(self.root, 12345)
        before = smoke.source_hashes(self.root)
        self.assertEqual(len(before), 7)
        self.assertEqual(len(set(before.values())), 4)
        self.assertEqual((self.root / "config" / "credential").read_text(), password)
        self.assertEqual((self.root / "config" / "credential").stat().st_mode & 0o777, 0o600)
        self.assertEqual(settings["origin"], "http://127.0.0.1:12345")
        self.assertEqual(before, smoke.source_hashes(self.root))
        self.assertTrue(all(marker in name for name in before))

    def test_source_symlink_and_membership_changes_are_rejected(self):
        smoke.fixture(self.root, 12345)
        member = next((self.root / "source").iterdir())
        member.unlink()
        member.symlink_to(self.root / "config" / "credential")
        with self.assertRaisesRegex(smoke.Blocked, "fixture_source_changed"):
            smoke.source_hashes(self.root)

    def test_known_ids_internal_network_loopback_and_limited_container_only(self):
        self.resources.create_network()
        identifier = self.resources.create_container(self.root, 12345)
        self.assertEqual(identifier, self.docker.container)
        call = next(args for args in self.docker.calls if args[:2] == ["container", "create"])
        self.assertIn("--read-only", call)
        self.assertIn("--memory=384m", call)
        self.assertIn("--cpus=1", call)
        self.assertIn("--pids-limit=128", call)
        self.assertIn("127.0.0.1:12345:8080", call)
        self.assertIn("type=bind,src=" + str(self.root / "source") + ",dst=/sources/source_a,readonly", call)
        self.assertIn("type=bind,src=" + str(self.root / "config") + ",dst=/config,readonly", call)
        self.resources.transition(identifier, "start")
        self.resources.transition(identifier, "stop")
        self.resources.cleanup()
        mutators = [a for a in self.docker.calls if a[:2] in [["container", "rm"], ["network", "rm"]]]
        self.assertEqual(mutators, [["container", "rm", "--force", self.docker.container], ["network", "rm", self.docker.network]])

    def test_foreign_fixture_and_transition_denied_without_docker_side_effect(self):
        for operation in [lambda: self.resources.create_container(self.root / "foreign"), lambda: self.resources.transition("c" * 64, "stop")]:
            with self.assertRaises(smoke.Blocked):
                operation()
        self.assertEqual(self.docker.calls, [])

    def test_ownership_change_retains_everything_and_never_prefix_cleans(self):
        self.resources.create_network()
        self.resources.create_container(self.root)
        self.docker.values[("container", self.docker.container)][0]["Config"]["Labels"][smoke.LABEL] = "foreign"
        self.docker.calls.clear()
        with self.assertRaisesRegex(smoke.Blocked, "resource_ownership_unknown"):
            self.resources.cleanup()
        self.assertFalse(any(args[:2] in [["container", "rm"], ["network", "rm"], ["image", "rm"]] for args in self.docker.calls))

    def test_foreign_network_endpoint_blocks_network_delete(self):
        self.resources.create_network()
        self.docker.values[("network", self.docker.network)][0]["Containers"] = {"foreign": {}}
        with self.assertRaisesRegex(smoke.Blocked, "network_has_foreign_endpoint"):
            self.resources.cleanup()
        self.assertNotIn(["network", "rm", self.docker.network], self.docker.calls)

    def test_existing_image_never_removed_new_nonce_tag_has_fail_closed_cleanup(self):
        self.resources.cleanup()
        self.assertEqual(self.docker.calls, [])
        self.resources.image_created = True
        self.docker.values[("image", self.image)][0]["RepoTags"].append("existing-production:keep")
        with self.assertRaisesRegex(smoke.Blocked, "image_ownership_unknown"):
            self.resources.cleanup()
        self.assertFalse(any(args[:2] == ["image", "rm"] for args in self.docker.calls))

    def test_unrelated_container_state_change_is_blocked_not_repaired(self):
        before = {"d" * 64: {"Status": "running", "StartedAt": "synthetic-before"}}
        smoke.check_existing(before, dict(before, **{"e" * 64: {"Status": "running"}}))
        for after in [{}, {"d" * 64: {"Status": "exited", "StartedAt": "synthetic-before"}}, {"d" * 64: {"Status": "running", "StartedAt": "synthetic-after"}}]:
            with self.assertRaisesRegex(smoke.Blocked, "existing_container_state_changed"):
                smoke.check_existing(before, after)

    def test_mount_and_resource_changes_refuse_before_start_or_cleanup(self):
        self.resources.create_network()
        for area, key, value in [("HostConfig", "ReadonlyRootfs", False), ("HostConfig", "Privileged", True), ("HostConfig", "NanoCpus", 2_000_000_000)]:
            with self.subTest(key=key):
                item = self.docker.values[("container", self.docker.container)][0]
                old = item[area][key]
                item[area][key] = value
                with self.assertRaisesRegex(smoke.Blocked, "container_isolation_changed"):
                    self.resources.own("container", self.docker.container)
                item[area][key] = old
        item["Mounts"][0]["RW"] = True
        with self.assertRaisesRegex(smoke.Blocked, "container_mounts_changed"):
            self.resources.own("container", self.docker.container)
        self.assertFalse(any(a[:2] == ["container", "start"] for a in self.docker.calls))

    def test_explicit_flag_without_complete_preflight_never_calls_docker(self):
        with mock.patch.object(smoke.subprocess, "run", side_effect=AssertionError("Docker forbidden")), mock.patch.object(smoke.tempfile, "mkdtemp", side_effect=AssertionError("write forbidden")), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(smoke.main(["--execute-disposable"]), 1)
        self.assertNotIn(str(self.root), output.getvalue())

    def test_runtime_mock_classifies_complete_and_real_byte_change(self):
        self.runtime("PASS")

    def test_runtime_mock_refuses_dangerous_api_success(self):
        self.runtime("dangerous_api_accepted")

    def test_runtime_mock_checks_error_body_privacy(self):
        self.runtime("http_privacy_window_failed")

    def test_runtime_mock_detects_source_changes(self):
        self.runtime("source_bytes_changed")

    def runtime(self, mode):
        settings, password, marker = smoke.fixture(self.root, 12345)
        own = {"State": {"Running": True, "OOMKilled": False}, "NetworkSettings": {"Ports": {"8080/tcp": [{"HostIp": "127.0.0.1", "HostPort": "12345"}]}, "Networks": {"synthetic-network": {"NetworkID": self.docker.network}}}}
        runtime = mock.Mock()
        runtime.containers = [self.docker.container]
        runtime.network = self.docker.network
        runtime.own.return_value = own
        runtime.create_container.side_effect = [self.docker.container, "3" * 64]
        runtime.docker.call.side_effect = lambda args, **kwargs: subprocess.CompletedProcess(args, 0, b"1" if args[:2] == ["container", "wait"] else b"safe synthetic", b"")
        source = fixtures.SOURCE
        status_calls = []
        requests = []
        def response(request, **kwargs):
            path = request.full_url.split("12345", 1)[1]
            requests.append(path)
            if path == "/api/login":
                value = {"csrf": "synthetic-session"}
            elif path == "/api/version":
                value = {"commit": source["commit"], "version": source["core_version"], "build_time": source["build_time"], "channel": source["channel"]}
            elif path == "/api/scan" and requests.count(path) == 1:
                raise urllib.error.HTTPError(request.full_url, 401, "synthetic", {}, io.BytesIO(b'{"error":"request_rejected"}'))
            elif any(path.startswith("/api/" + area + "/") for area in ["quarantine", "restore", "purge"]):
                if mode == "dangerous_api_accepted":
                    value = {"unexpected": "accepted"}
                else:
                    raw = json.dumps({"error": marker if mode == "http_privacy_window_failed" else "request_rejected"}).encode()
                    raise urllib.error.HTTPError(request.full_url, 403, "synthetic", {}, io.BytesIO(raw))
            elif path == "/api/status":
                status_calls.append(path)
                if mode == "source_bytes_changed" and len(status_calls) == 2:
                    next((self.root / "source").iterdir()).write_bytes(b"artificial change")
                value = {"state": "COMPLETED", "failed": 0, "active": 7, "quick": 7, "full": 6, "groups": [{"copies": 2, "bytes": 300}] * 3, "checkpoint": "completed", "missing": 0, "unavailable": 0}
            else:
                value = {"state": "synthetic"}
            return io.BytesIO(json.dumps(value).encode())
        client = mock.Mock()
        client.open.side_effect = response
        with mock.patch.object(smoke.urllib.request, "build_opener", return_value=client), mock.patch.object(smoke.time, "sleep"):
            if mode == "PASS":
                value = smoke.exercise(runtime, self.root, 12345, settings, password, marker, source)
                self.assertTrue(value["source_sha_unchanged"])
                self.assertTrue(value["single_writer_refused"])
                self.assertEqual(len([path for path in requests if any(path.startswith("/api/" + area + "/") for area in ["quarantine", "restore", "purge"])]), 12)
            else:
                with self.assertRaisesRegex(smoke.Blocked, "^" + mode + "$"):
                    smoke.exercise(runtime, self.root, 12345, settings, password, marker, source)


if __name__ == "__main__":
    unittest.main()
